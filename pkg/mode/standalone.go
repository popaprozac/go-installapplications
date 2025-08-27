package mode

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/go-installapplications/pkg/config"
	"github.com/go-installapplications/pkg/installer"
	"github.com/go-installapplications/pkg/utils"
)

// RunStandalone executes the standalone mode workflow
// Cleans existing state and runs complete bootstrap process using standard configuration hierarchy
// Only supports server-based (jsonurl) or MDM-embedded bootstrap sources
func RunStandalone(cfg *config.Config, logger *utils.Logger) {
	logger.Info("Starting standalone mode")

	// Step 1: Clean existing state (but preserve binary)
	logger.Info("Step 1: Cleaning existing installation state")
	if err := cleanInstallationState(cfg, logger); err != nil {
		logger.Error("Failed to clean installation state: %v", err)
		// No cleanup needed - we haven't started bootstrap yet
		return
	}

	// Step 2: Check if we have a valid bootstrap source (server-based or MDM-embedded only)
	hasBootstrapSource := false
	if cfg.JSONURL != "" {
		logger.Info("Bootstrap source: JSON URL (%s)", cfg.JSONURL)
		hasBootstrapSource = true
	} else {
		// Check for embedded bootstrap in mobileconfig
		_, err := cfg.LoadBootstrapFromProfile(config.DefaultProfileDomain)
		if err == nil {
			logger.Info("Bootstrap source: Embedded in mobileconfig")
			hasBootstrapSource = true
		}
	}

	if !hasBootstrapSource {
		logger.Error("❌ MISSING BOOTSTRAP SOURCE")
		logger.Error("Standalone mode requires a server-based or MDM-managed bootstrap source:")
		logger.Error("  1. Remote URL: --jsonurl https://company.com/bootstrap.json")
		logger.Error("  2. Embedded in mobileconfig (deployed via MDM)")
		// No cleanup needed - we haven't started bootstrap yet
		return
	}

	// Step 3: Run complete bootstrap process
	logger.Info("Step 2: Running complete bootstrap process")
	if err := runCompleteBootstrap(cfg, logger); err != nil {
		logger.Error("Bootstrap process failed: %v", err)
		logger.Error("⚠️  Manual intervention may be required")
		// Cleanup needed since bootstrap process was started
		// We need to create a temporary manager for cleanup
		if _, _, _, manager, setupErr := setupBootstrapAndComponents(cfg, logger); setupErr == nil {
			manager.Cleanup("standalone bootstrap failure")
			utils.Exit(cfg, logger, 1, "bootstrap process failed")
		} else {
			// If we can't create manager, just exit without cleanup
			utils.Exit(cfg, logger, 1, "bootstrap process failed")
		}
	}

}

// cleanInstallationState cleans existing state but preserves the binary
func cleanInstallationState(cfg *config.Config, logger *utils.Logger) error {
	logger.Info("🧹 Cleaning installation state (preserving binary)")

	// Stop all running services
	logger.Debug("Stopping LaunchDaemon and LaunchAgent services")
	if err := stopInstallApplicationsServices(cfg, logger); err != nil {
		logger.Debug("Failed to stop services (may not be running): %v", err)
	}

	// Clean signal files and temp directories
	logger.Debug("Cleaning signal files and temporary directories")
	if err := cleanSignalFiles(cfg, logger); err != nil {
		logger.Debug("Failed to clean signal files: %v", err)
	}

	// Reset any cached state (but preserve binary)
	logger.Debug("Clearing cached application state")
	if err := clearCachedState(cfg, logger); err != nil {
		logger.Debug("Failed to clear cached state: %v", err)
	}

	logger.Info("✅ Installation state cleaned successfully")
	return nil
}

// stopInstallApplicationsServices stops any running LaunchDaemon/LaunchAgent services
func stopInstallApplicationsServices(cfg *config.Config, logger *utils.Logger) error {
	// Build plist paths from identifiers
	daemonPlist := "/Library/LaunchDaemons/" + cfg.LaunchDaemonIdentifier + ".plist"
	agentPlist := "/Library/LaunchAgents/" + cfg.LaunchAgentIdentifier + ".plist"

	// Determine current console user's GUI domain for agent bootout
	uid, err := utils.GetConsoleUserUID()
	if err != nil || uid == "" {
		logger.Debug("Could not determine console user UID, defaulting to gui/501: %v", err)
		uid = "501"
	}
	guiDomain := "gui/" + uid

	services := []struct {
		label string
		cmd   []string
	}{
		{label: "LaunchDaemon", cmd: []string{"launchctl", "bootout", "system", daemonPlist}},
		{label: "LaunchAgent", cmd: []string{"launchctl", "bootout", guiDomain, agentPlist}},
	}

	for _, svc := range services {
		logger.Debug("Stopping %s service", svc.label)
		cmd := exec.Command(svc.cmd[0], svc.cmd[1:]...)
		if err := cmd.Run(); err != nil {
			logger.Debug("%s service stop failed (may not be running): %v", svc.label, err)
		} else {
			logger.Info("✅ Stopped %s service", svc.label)
		}
	}

	return nil
}

// cleanSignalFiles removes signal files that track installation state
func cleanSignalFiles(cfg *config.Config, logger *utils.Logger) error {
	signalDirs := []string{
		cfg.InstallPath,
	}

	for _, dir := range signalDirs {
		if dir == "" {
			continue
		}

		logger.Debug("Cleaning signal directory: %s", dir)
		if err := os.RemoveAll(dir); err != nil {
			logger.Debug("Failed to remove %s: %v", dir, err)
		} else {
			logger.Verbose("Cleaned signal directory: %s", dir)
		}

		// Recreate the directory for future use
		if err := os.MkdirAll(dir, 0755); err != nil {
			logger.Debug("Failed to recreate %s: %v", dir, err)
		}
	}

	return nil
}

// clearCachedState removes any cached application or download state (but preserves binary)
func clearCachedState(cfg *config.Config, logger *utils.Logger) error {
	// Clear any cached downloads
	cacheDir := filepath.Join(cfg.InstallPath, "cache")
	if err := os.RemoveAll(cacheDir); err != nil {
		logger.Debug("Failed to clear cache directory %s: %v", cacheDir, err)
	} else {
		logger.Verbose("Cleared cache directory: %s", cacheDir)
	}

	// Clear any bootstrap files from previous runs
	bootstrapFiles := []string{
		cfg.DefaultBootstrapPath,
	}

	for _, file := range bootstrapFiles {
		if err := os.Remove(file); err != nil {
			logger.Debug("Failed to remove bootstrap file %s: %v", file, err)
		} else {
			logger.Verbose("Removed cached bootstrap file: %s", file)
		}
	}

	// Note: We intentionally preserve the binary at cfg.InstallPath/go-installapplications
	// so it can be reused for recovery operations
	logger.Verbose("Preserved binary in InstallPath for reuse")

	return nil
}

// runCompleteBootstrap executes the full bootstrap process using standard logic
func runCompleteBootstrap(cfg *config.Config, logger *utils.Logger) error {
	logger.Info("🔄 Starting complete bootstrap process")

	// Get bootstrap and create components using shared logic
	bootstrap, _, _, manager, err := setupBootstrapAndComponents(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to setup bootstrap and components: %w", err)
	}

	// Run all phases in order (like the complete daemon + agent flow)
	if len(bootstrap.Preflight) > 0 && cfg.WithPreflight {
		logger.Info("Starting preflight phase")
		if err := manager.ProcessItems(bootstrap.Preflight, "preflight"); err != nil {
			// Check if this is a preflight success signal
			if _, ok := err.(*installer.PreflightSuccessError); ok {
				logger.Info("Preflight script passed - cleaning up and exiting")
				return nil // Return success to indicate completion
			}
			// Actual error occurred
			return fmt.Errorf("preflight phase failed: %w", err)
		}
		logger.Info("Preflight phase completed successfully")
	}

	if len(bootstrap.SetupAssistant) > 0 {
		logger.Info("Starting setupassistant phase")
		if err := manager.ProcessItems(bootstrap.SetupAssistant, "setupassistant"); err != nil {
			return fmt.Errorf("setupassistant phase failed: %w", err)
		}
		logger.Info("Setupassistant phase completed successfully")
	}

	if len(bootstrap.Userland) > 0 {
		logger.Info("Starting userland phase")
		if err := manager.ProcessItems(bootstrap.Userland, "userland"); err != nil {
			return fmt.Errorf("userland phase failed: %w", err)
		}
		logger.Info("Userland phase completed successfully")
	}

	logger.Info("All phases completed successfully")

	// Perform cleanup and exit
	manager.Cleanup("standalone completion")
	// Perform manager cleanup, then exit with system cleanup
	manager.Cleanup("standalone completion")
	utils.Exit(cfg, logger, 0, "standalone successful completion")
	return nil // This line will never be reached due to os.Exit
}
