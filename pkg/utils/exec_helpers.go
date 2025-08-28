package utils

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-installapplications/pkg/config"
)

// RunCommandCapture runs a command and returns trimmed stdout or an error
func RunCommandCapture(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("no command provided")
	}
	cmd := exec.Command(args[0], args[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// GetConsoleUserUID returns the current GUI console user UID
func GetConsoleUserUID() (string, error) {
	out, err := RunCommandCapture([]string{"stat", "-f", "%u", "/dev/console"})
	if err != nil {
		return "", err
	}
	return out, nil
}

// IsRootUser checks if the current process is running with root privileges
func IsRootUser() bool {
	return os.Geteuid() == 0
}

// Exit handles program exit with cleanup and optional message
func Exit(cfg *config.Config, logger *Logger, exitCode int, message string) {
	if message != "" {
		logger.Info("Exiting with code %d: %s", exitCode, message)
	}

	// Always call cleanup (cleanup handles flag logic)
	Cleanup(cfg, logger, "exit")

	os.Exit(exitCode)
}

// Cleanup performs system cleanup (plists, services, reboot) - file cleanup is handled by components
func Cleanup(cfg *config.Config, logger *Logger, cleanupType string) {
	logger.Debug("Performing system cleanup (plists, services, reboot)")

	// Build paths
	daemonPlist := "/Library/LaunchDaemons/" + cfg.LaunchDaemonIdentifier + ".plist"
	agentPlist := "/Library/LaunchAgents/" + cfg.LaunchAgentIdentifier + ".plist"

	// Remove LaunchDaemon plist file
	logger.Debug("Removing LaunchDaemon plist: %s", daemonPlist)
	if err := os.Remove(daemonPlist); err != nil && !os.IsNotExist(err) {
		logger.Debug("Failed to remove LaunchDaemon plist: %v", err)
	}

	// Remove LaunchAgent plist file
	logger.Debug("Removing LaunchAgent plist: %s", agentPlist)
	if err := os.Remove(agentPlist); err != nil && !os.IsNotExist(err) {
		logger.Debug("Failed to remove LaunchAgent plist: %v", err)
	}

	// Boot out LaunchAgent from user context
	logger.Debug("Booting out LaunchAgent from user context")
	uid, err := GetConsoleUserUID()
	if err != nil || uid == "" {
		logger.Debug("Could not determine console user UID, defaulting to gui/501: %v", err)
		uid = "501"
	}
	guiDomain := "gui/" + uid

	cmd := exec.Command("launchctl", "bootout", guiDomain, agentPlist)
	if err := cmd.Run(); err != nil {
		logger.Debug("Failed to boot out LaunchAgent (may not be running): %v", err)
	}

	// Remove entire installation directory
	logger.Debug("Removing installation directory: %s", cfg.InstallPath)
	if err := os.RemoveAll(cfg.InstallPath); err != nil {
		logger.Debug("Failed to remove installation directory: %v", err)
	}

	// Boot out LaunchDaemon
	logger.Debug("Booting out LaunchDaemon")
	cmd = exec.Command("launchctl", "bootout", "system", daemonPlist)
	if err := cmd.Run(); err != nil {
		logger.Debug("Failed to boot out LaunchDaemon (may not be running): %v", err)
	}

	// Handle reboot if configured
	if cfg.Reboot {
		logger.Info("🔄 Reboot flag is set; system will reboot in 5 seconds")
		time.Sleep(5 * time.Second)
		cmd := exec.Command("/sbin/shutdown", "-r", "now")
		if err := cmd.Start(); err != nil {
			logger.Error("Failed to initiate reboot: %v", err)
		}
	}

	logger.Info("✅ %s cleanup completed", cleanupType)
}
