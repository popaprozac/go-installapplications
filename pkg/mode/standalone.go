package mode

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/go-installapplications/pkg/config"
	"github.com/go-installapplications/pkg/download"
	"github.com/go-installapplications/pkg/installer"
	"github.com/go-installapplications/pkg/manager"
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
		return
	}

	// Step 3: Run complete bootstrap process
	logger.Info("Step 2: Running complete bootstrap process")
	if err := runCompleteBootstrap(cfg, logger); err != nil {
		logger.Error("Bootstrap process failed: %v", err)
		logger.Error("⚠️  Manual intervention may be required")
		return
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
	uid, err := getConsoleUserUID()
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

	// Get bootstrap using the same logic as daemon/agent modes (server or MDM only)
	var bootstrap *config.Bootstrap
	var err error

	if cfg.JSONURL != "" {
		// Download from URL (same logic as daemon/agent)
		logger.Info("Downloading bootstrap from: %s", cfg.JSONURL)

		bootstrapPath := filepath.Join(cfg.InstallPath, "bootstrap.json")

		// Create authenticated downloader if needed
		var downloader *download.Client
		if cfg.HTTPAuthUser != "" || len(cfg.HTTPHeaders) > 0 {
			downloader = download.NewClientWithAuth(logger, cfg.HTTPAuthUser, cfg.HTTPAuthPassword, cfg.HTTPHeaders)
			logger.Debug("Using authenticated download client")
		} else {
			downloader = download.NewClient(logger)
		}
		downloader.SetFollowRedirects(cfg.FollowRedirects)
		downloader.SetRetryDefaults(cfg.MaxRetries, cfg.RetryDelay)

		if err := downloader.DownloadFile(cfg.JSONURL, bootstrapPath, ""); err != nil {
			return fmt.Errorf("failed to download bootstrap: %w", err)
		}

		if cfg.SkipValidation {
			logger.Debug("SkipValidation=true: loading bootstrap without validation")
			bootstrap, err = config.LoadBootstrapWithOptions(bootstrapPath, false)
		} else {
			bootstrap, err = config.LoadBootstrap(bootstrapPath)
		}
		if err != nil {
			return fmt.Errorf("failed to parse downloaded bootstrap: %w", err)
		}
	} else {
		// Load from embedded mobileconfig
		logger.Info("Loading bootstrap from embedded mobileconfig")
		bootstrap, err = cfg.LoadBootstrapFromProfile(config.DefaultProfileDomain)
		if err != nil {
			return fmt.Errorf("failed to load bootstrap from mobileconfig: %w", err)
		}
	}

	// Validate bootstrap
	if !cfg.SkipValidation {
		if err := config.ValidateBootstrap(bootstrap); err != nil {
			return fmt.Errorf("bootstrap validation failed: %w", err)
		}
	} else {
		logger.Debug("SkipValidation=true: skipping bootstrap structural validation")
	}

	logger.Info("Bootstrap loaded successfully")
	logger.Debug("Preflight: %d, SetupAssistant: %d, Userland: %d items",
		len(bootstrap.Preflight), len(bootstrap.SetupAssistant), len(bootstrap.Userland))

	// Create components for processing
	var downloader *download.Client
	if cfg.HTTPAuthUser != "" || len(cfg.HTTPHeaders) > 0 {
		downloader = download.NewClientWithAuth(logger, cfg.HTTPAuthUser, cfg.HTTPAuthPassword, cfg.HTTPHeaders)
	} else {
		downloader = download.NewClient(logger)
	}
	downloader.SetRetryDefaults(cfg.MaxRetries, cfg.RetryDelay)

	// Standalone mode runs as root but can handle both root and user items (recovery scenario)
	systemInstaller := installer.NewSystemInstaller(cfg.DryRun, logger, false) // false = daemon context, but allows user items
	manager := manager.NewManager(downloader, systemInstaller, cfg, logger)

	// Run all phases in order (like the complete daemon + agent flow)
	if len(bootstrap.Preflight) > 0 {
		logger.Info("Starting preflight phase")
		if err := manager.ProcessItems(bootstrap.Preflight, "preflight"); err != nil {
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
	if cfg.Reboot {
		logger.Info("Reboot flag is set; system will reboot in 5 seconds")
		time.Sleep(5 * time.Second)
		cmd := exec.Command("/sbin/shutdown", "-r", "now")
		if err := cmd.Start(); err != nil {
			logger.Error("Failed to initiate reboot: %v", err)
		}
	}
	return nil
}
