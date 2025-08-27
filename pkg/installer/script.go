package installer

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-installapplications/pkg/utils"
)

// PreflightSuccessError is a special error type that signals preflight success
// This allows the caller to distinguish between actual errors and preflight success
type PreflightSuccessError struct{}

func (e *PreflightSuccessError) Error() string {
	return "preflight script passed - cleaning up and exiting"
}

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
	return se.executeScript(scriptPath, scriptType, doNotWait, trackBackgroundProcesses, false)
}

// ExecuteScriptForPreflight runs a script with special preflight exit code handling
func (se *ScriptExecutor) ExecuteScriptForPreflight(scriptPath, scriptType string, doNotWait bool, trackBackgroundProcesses bool) error {
	return se.executeScript(scriptPath, scriptType, doNotWait, trackBackgroundProcesses, true)
}

// executeScript is the internal implementation that handles both normal and preflight scripts
func (se *ScriptExecutor) executeScript(scriptPath, scriptType string, doNotWait bool, trackBackgroundProcesses bool, isPreflight bool) error {
	se.logger.Info("Executing %s script: %s", scriptType, scriptPath)
	se.logger.Debug("Script executor dry-run mode: %t, donotwait: %t, track-bg: %t", se.dryRun, doNotWait, trackBackgroundProcesses)

	if se.dryRun {
		return se.handleDryRunExecution(scriptPath, scriptType, doNotWait)
	}

	// Validate and prepare script
	if err := se.validateAndPrepareScript(scriptPath); err != nil {
		return err
	}

	// Create and configure command
	cmd, err := se.createScriptCommand(scriptPath, scriptType)
	if err != nil {
		return err
	}

	// Handle background execution
	if doNotWait && !isPreflight {
		return se.handleBackgroundExecution(cmd, scriptPath, scriptType, trackBackgroundProcesses)
	}

	// Execute and handle result
	return se.executeAndHandleResult(cmd, scriptPath, scriptType, isPreflight)
}

// WaitForBackgroundProcesses waits for all background processes to complete
func (se *ScriptExecutor) WaitForBackgroundProcesses(timeout time.Duration) []error {
	return se.processTracker.WaitForCompletion(timeout)
}

// GetBackgroundProcessCount returns the number of active background processes
func (se *ScriptExecutor) GetBackgroundProcessCount() int {
	return se.processTracker.GetActiveCount()
}

// getCurrentLoggedInUserUID returns the UID of the currently logged-in user
func (se *ScriptExecutor) getCurrentLoggedInUserUID() (string, error) {
	return utils.GetConsoleUserUID()
}

// handleDryRunExecution handles script execution in dry-run mode
func (se *ScriptExecutor) handleDryRunExecution(scriptPath, scriptType string, doNotWait bool) error {
	if doNotWait {
		se.logger.Info("[DRY RUN] Would execute in background: %s (%s)", scriptPath, scriptType)
	} else {
		se.logger.Info("[DRY RUN] Would execute: %s (%s)", scriptPath, scriptType)
	}
	return nil
}

// validateAndPrepareScript validates script exists and sets permissions
func (se *ScriptExecutor) validateAndPrepareScript(scriptPath string) error {
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

	return nil
}

// createScriptCommand creates and configures the appropriate command for script execution
func (se *ScriptExecutor) createScriptCommand(scriptPath, scriptType string) (*exec.Cmd, error) {
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
				return "daemon/standalone"
			}
		}())
		cmd = exec.Command(scriptPath)
	case "userscript":
		// User-context scripts
		if se.isAgentMode {
			se.logger.Debug("Running userscript as user (agent mode)")
			cmd = exec.Command(scriptPath)
		} else {
			// Standalone mode: use launchctl asuser to execute as logged-in user
			se.logger.Debug("Running userscript as logged-in user via launchctl asuser (standalone mode)")
			userUID, err := se.getCurrentLoggedInUserUID()
			if err != nil {
				return nil, fmt.Errorf("failed to get user UID for userscript: %w", err)
			}
			cmd = exec.Command("launchctl", "asuser", userUID, scriptPath)
		}
	default:
		return nil, fmt.Errorf("unknown script type: %s", scriptType)
	}

	// Set working directory to script's directory
	cmd.Dir = filepath.Dir(scriptPath)
	se.logger.Debug("Setting working directory: %s", cmd.Dir)
	se.logger.Verbose("Executing command: %s", cmd.String())

	return cmd, nil
}

// handleBackgroundExecution handles script execution in background mode
func (se *ScriptExecutor) handleBackgroundExecution(cmd *exec.Cmd, scriptPath, scriptType string, trackBackgroundProcesses bool) error {
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

// executeAndHandleResult executes the command and handles the result based on context
func (se *ScriptExecutor) executeAndHandleResult(cmd *exec.Cmd, scriptPath, scriptType string, isPreflight bool) error {
	// Normal execution: wait for completion
	output, err := cmd.CombinedOutput()

	// Handle preflight exit code behavior (matches original InstallApplications)
	if isPreflight && scriptType == "rootscript" {
		return se.handlePreflightResult(err, output)
	}

	// Normal script execution (non-preflight)
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

// handlePreflightResult handles the special preflight exit code logic
func (se *ScriptExecutor) handlePreflightResult(err error, output []byte) error {
	if err != nil {
		// Script failed (non-zero exit code) - all treated the same
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode := exitErr.ExitCode()
			se.logger.Info("⚠️  Preflight script failed (exit code %d) - continuing with bootstrap", exitCode)
			se.logger.Debug("Script output: %s", string(output))
			return nil // Return nil to continue with bootstrap (all non-zero exit codes)
		} else {
			// Non-exit error (e.g., script not found, permission denied)
			se.logger.Error("Preflight script execution failed: %v", err)
			se.logger.Debug("Script output: %s", string(output))
			return fmt.Errorf("preflight script execution failed: %w, output: %s", err, string(output))
		}
	} else {
		// Script succeeded (exit code 0)
		se.logger.Info("✅ Preflight script passed (exit code 0) - signaling cleanup and exit")
		se.logger.Debug("Script output: %s", string(output))
		return &PreflightSuccessError{} // Special error to signal preflight success
	}
}
