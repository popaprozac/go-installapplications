package installer

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-installapplications/pkg/utils"
)

// ScriptExecutor handles script execution
type ScriptExecutor struct {
	dryRun         bool
	logger         *utils.Logger
	processTracker *utils.ProcessTracker
	isAgentMode    bool // true if running as agent (user context), false if daemon (root context)
}

// NewScriptExecutor creates a new script executor
func NewScriptExecutor(dryRun bool, logger *utils.Logger, isAgentMode bool) *ScriptExecutor {
	return &ScriptExecutor{
		dryRun:         dryRun,
		logger:         logger,
		processTracker: utils.NewProcessTracker(logger),
		isAgentMode:    isAgentMode,
	}
}

// detectScriptInterpreter reads the shebang line to determine script type
func (se *ScriptExecutor) detectScriptInterpreter(scriptPath string) (string, error) {
	file, err := os.Open(scriptPath)
	if err != nil {
		return "", fmt.Errorf("failed to open script: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		firstLine := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(firstLine, "#!") {
			interpreter := strings.TrimSpace(firstLine[2:]) // Remove #!
			se.logger.Verbose("Detected interpreter from shebang: %s", interpreter)

			// Extract just the interpreter name for logging
			parts := strings.Fields(interpreter)
			if len(parts) > 0 {
				interpreterName := filepath.Base(parts[0])
				switch {
				case strings.Contains(interpreterName, "bash"):
					return "bash", nil
				case strings.Contains(interpreterName, "sh"):
					return "shell", nil
				case strings.Contains(interpreterName, "python"):
					return "python", nil
				case strings.Contains(interpreterName, "ruby"):
					return "ruby", nil
				case strings.Contains(interpreterName, "perl"):
					return "perl", nil
				case strings.Contains(interpreterName, "node"):
					return "node.js", nil
				default:
					return interpreterName, nil
				}
			}
		}
	}

	// No shebang found - assume shell script
	se.logger.Debug("No shebang found, assuming shell script")
	return "shell", nil
}

// ExecuteScript runs a script with appropriate permissions and donotwait support
func (se *ScriptExecutor) ExecuteScript(scriptPath, scriptType string, doNotWait bool, trackBackgroundProcesses bool) error {
	se.logger.Info("Executing %s script: %s", scriptType, scriptPath)
	se.logger.Debug("Script executor dry-run mode: %t, donotwait: %t, track-bg: %t", se.dryRun, doNotWait, trackBackgroundProcesses)

	if se.dryRun {
		if doNotWait {
			se.logger.Info("[DRY RUN] Would execute in background: %s (%s)", scriptPath, scriptType)
		} else {
			se.logger.Info("[DRY RUN] Would execute: %s (%s)", scriptPath, scriptType)
		}
		return nil
	}

	// Check if script exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return fmt.Errorf("script does not exist: %s", scriptPath)
	}

	se.logger.Debug("Script exists, setting permissions")

	// Make script executable
	if err := os.Chmod(scriptPath, 0755); err != nil {
		return fmt.Errorf("failed to make script executable: %w", err)
	}
	se.logger.Verbose("Set script permissions to 0755: %s", scriptPath)

	// Detect script interpreter from shebang
	interpreter, err := se.detectScriptInterpreter(scriptPath)
	if err != nil {
		se.logger.Debug("Failed to detect interpreter: %v", err)
		interpreter = "unknown"
	}
	se.logger.Debug("Script interpreter: %s", interpreter)

	var cmd *exec.Cmd

	switch scriptType {
	case "rootscript":
		// Root-context scripts
		// - Daemon: executes directly as root
		// - Agent: allowed if binary/flow grants proper authorization (should be rare)
		se.logger.Debug("Running rootscript (mode: %s)", func() string {
			if se.isAgentMode {
				return "agent"
			} else {
				return "daemon"
			}
		}())
		cmd = exec.Command(scriptPath)
	case "userscript":
		// User-context scripts
		// - Daemon/agent flow: userscripts are executed by the Agent only (daemon delegates via IPC)
		// - Standalone mode: process runs as root and must switch to the logged-in user via launchctl asuser
		if se.isAgentMode {
			se.logger.Debug("Running userscript as user (agent mode)")
			cmd = exec.Command(scriptPath)
		} else {
			se.logger.Debug("Running userscript as logged-in user via launchctl asuser (standalone/root context)")
			return se.executeAsLoggedInUser(scriptPath, scriptType)
		}
	default:
		return fmt.Errorf("unknown script type: %s", scriptType)
	}

	// Set working directory to script's directory
	cmd.Dir = filepath.Dir(scriptPath)
	se.logger.Debug("Setting working directory: %s", cmd.Dir)
	se.logger.Verbose("Executing command: %s", cmd.String())

	// Handle donotwait behavior
	if doNotWait {
		if trackBackgroundProcesses {
			// Modern mode: Track the background process
			se.logger.Info("Starting script in background (tracked): %s", scriptPath)
			return se.processTracker.StartBackgroundProcess(cmd, fmt.Sprintf("%s (%s)", scriptPath, scriptType))
		} else {
			// Legacy mode: Fire and forget
			se.logger.Info("Starting script in background (fire-and-forget): %s", scriptPath)
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("failed to start background script: %w", err)
			}
			se.logger.Info("Background script started: %s", scriptPath)
			return nil
		}
	}

	// Normal execution: wait for completion
	output, err := cmd.CombinedOutput()
	if err != nil {
		se.logger.Error("Script execution failed: %v", err)
		se.logger.Debug("Script output: %s", string(output))
		return fmt.Errorf("script execution failed: %w, output: %s", err, string(output))
	}

	se.logger.Info("Script executed successfully: %s", scriptPath)
	if len(output) > 0 {
		se.logger.Debug("Script output: %s", string(output))
	} else {
		se.logger.Verbose("Script produced no output")
	}

	return nil
}

// WaitForBackgroundProcesses waits for all background processes to complete
func (se *ScriptExecutor) WaitForBackgroundProcesses(timeout time.Duration) []error {
	return se.processTracker.WaitForCompletion(timeout)
}

// GetBackgroundProcessCount returns the number of active background processes
func (se *ScriptExecutor) GetBackgroundProcessCount() int {
	return se.processTracker.GetActiveCount()
}

// executeAsLoggedInUser executes a script as the currently logged-in user using launchctl asuser
func (se *ScriptExecutor) executeAsLoggedInUser(scriptPath, scriptType string) error {
	// Get the currently logged-in user
	userUID, err := se.getCurrentLoggedInUserUID()
	if err != nil {
		return fmt.Errorf("failed to get logged-in user: %w", err)
	}

	// Log the execution context
	se.logger.Info("Executing %s as user (UID: %s) via launchctl asuser", scriptType, userUID)

	// Use launchctl asuser to execute the script in the user's context
	cmd := exec.Command("launchctl", "asuser", userUID, scriptPath)
	cmd.Dir = filepath.Dir(scriptPath)

	// Capture output for logging
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		se.logger.Info("%s output: %s", scriptType, string(output))
	}

	if err != nil {
		return fmt.Errorf("%s execution failed: %w", scriptType, err)
	}

	se.logger.Info("%s completed successfully", scriptType)
	return nil
}

// getCurrentLoggedInUserUID returns the UID of the currently logged-in user
func (se *ScriptExecutor) getCurrentLoggedInUserUID() (string, error) {
	// Use stat on /dev/console to get the owner (logged-in user)
	cmd := exec.Command("stat", "-f", "%u", "/dev/console")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get console user UID: %w", err)
	}

	uid := strings.TrimSpace(string(output))
	if uid == "" || uid == "0" {
		return "", fmt.Errorf("no user logged in or root owns console")
	}

	// Validate that the UID is a number
	if _, err := strconv.Atoi(uid); err != nil {
		return "", fmt.Errorf("invalid UID format: %s", uid)
	}

	return uid, nil
}
