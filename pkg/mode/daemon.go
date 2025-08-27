package mode

import (
	"fmt"
	"os"

	"github.com/go-installapplications/pkg/config"
	"github.com/go-installapplications/pkg/download"
	"github.com/go-installapplications/pkg/installer"
	"github.com/go-installapplications/pkg/ipc"
	"github.com/go-installapplications/pkg/manager"
	"github.com/go-installapplications/pkg/retry"
	"github.com/go-installapplications/pkg/utils"
)

// RunDaemon executes the daemon mode workflow
func RunDaemon(cfg *config.Config, logger *utils.Logger) {
	logger.Info("Starting daemon mode")

	// Check retry logic
	if shouldRetry, err := retry.ShouldRetry(); !shouldRetry {
		logger.Error("Maximum retry attempts exceeded: %v", err)
		utils.Exit(cfg, logger, 0, "max retries exceeded")
	}

	logger.Info("Daemon attempt: %s", retry.GetRetryInfo())

	if err := retry.IncrementRetryCount("daemon started"); err != nil {
		logger.Error("Failed to update retry count: %v", err)
	}

	// Get bootstrap and create components
	bootstrap, downloader, systemInstaller, manager, err := setupBootstrapAndComponents(cfg, logger)
	if err != nil {
		logger.Error("Failed to setup bootstrap and components: %v", err)
		retry.IncrementRetryCount(fmt.Sprintf("setup failed: %v", err))
		// Exit without cleanup (no components created yet)
		utils.Exit(cfg, logger, 1, "setup failed")
	}

	// Process preflight and setupassistant phases
	if err := processSystemPhases(bootstrap, manager, cfg, logger); err != nil {
		// Check if this is a preflight success signal
		if _, ok := err.(*installer.PreflightSuccessError); ok {
			logger.Info("Preflight script passed - cleaning up and exiting")
			// Perform manager cleanup, then exit with system cleanup
			manager.Cleanup("preflight success")
			utils.Exit(cfg, logger, 0, "preflight success")
		}
		// Actual error occurred
		retry.IncrementRetryCount(fmt.Sprintf("system phases failed: %v", err))
		// Perform manager cleanup, then exit with system cleanup
		manager.Cleanup("system phases error")
		utils.Exit(cfg, logger, 1, "system phases failed")
	}

	// Process userland phase
	if len(bootstrap.Userland) > 0 {
		if err := processUserlandPhase(bootstrap.Userland, downloader, systemInstaller, cfg, logger); err != nil {
			retry.IncrementRetryCount(fmt.Sprintf("userland failed: %v", err))
			// Perform manager cleanup, then exit with system cleanup
			manager.Cleanup("userland error")
			utils.Exit(cfg, logger, 1, "userland phase failed")
		}
		logger.Info("Userland phase completed successfully")
	} else {
		logger.Debug("No userland items present")
	}

	// Success!
	logger.Info("Daemon completed all phases successfully!")

	// Clear retry counter
	if err := retry.ClearRetryCount(); err != nil {
		logger.Error("Failed to clear retry count: %v", err)
	}

	// Perform manager cleanup, then exit with system cleanup
	manager.Cleanup("daemon completion")
	utils.Exit(cfg, logger, 0, "daemon successful completion")
}

// setupBootstrapAndComponents loads bootstrap and creates all necessary components
func setupBootstrapAndComponents(cfg *config.Config, logger *utils.Logger) (*config.Bootstrap, *download.Client, *installer.SystemInstaller, *manager.Manager, error) {
	// Get bootstrap from either JSON URL or embedded mobile config
	bootstrap, err := getBootstrap(cfg, logger)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to get bootstrap: %w", err)
	}

	logger.Info("Bootstrap loaded successfully")
	logger.Debug("Preflight items: %d, SetupAssistant items: %d, Userland items: %d",
		len(bootstrap.Preflight), len(bootstrap.SetupAssistant), len(bootstrap.Userland))

	// Create components with authentication support
	var downloader *download.Client
	if cfg.HTTPAuthUser != "" || len(cfg.HTTPHeaders) > 0 {
		downloader = download.NewClientWithAuth(logger, cfg.HTTPAuthUser, cfg.HTTPAuthPassword, cfg.HTTPHeaders)
		logger.Debug("Created authenticated download client")
	} else {
		downloader = download.NewClient(logger)
	}
	downloader.SetRetryDefaults(cfg.MaxRetries, cfg.RetryDelay)

	systemInstaller := installer.NewSystemInstaller(cfg.DryRun, logger, false) // false = daemon mode (root)
	manager := manager.NewManager(downloader, systemInstaller, cfg, logger)

	return bootstrap, downloader, systemInstaller, manager, nil
}

// processSystemPhases processes preflight and setupassistant phases
func processSystemPhases(bootstrap *config.Bootstrap, manager *manager.Manager, cfg *config.Config, logger *utils.Logger) error {
	// Process preflight phase
	if len(bootstrap.Preflight) > 0 {
		logger.Info("Starting preflight phase")
		if err := manager.ProcessItems(bootstrap.Preflight, "preflight"); err != nil {
			return err
		}
		logger.Info("Preflight phase completed successfully")
	} else {
		logger.Debug("No preflight items to process")
	}

	// Process setupassistant phase
	if len(bootstrap.SetupAssistant) > 0 {
		logger.Info("Starting setupassistant phase")
		if err := manager.ProcessItems(bootstrap.SetupAssistant, "setupassistant"); err != nil {
			return err
		}
		logger.Info("Setupassistant phase completed successfully")
	} else {
		logger.Debug("No setupassistant items to process")
	}

	return nil
}

// getBootstrap retrieves bootstrap configuration from either JSON URL or embedded mobile config
func getBootstrap(cfg *config.Config, logger *utils.Logger) (*config.Bootstrap, error) {
	// First check if we have a JSON URL
	if cfg.JSONURL != "" {
		logger.Info("Loading bootstrap from JSON URL: %s", cfg.JSONURL)

		// Download bootstrap to consistent path
		bootstrapPath := cfg.InstallPath + "/bootstrap.json"
		logger.Debug("Bootstrap destination: %s", bootstrapPath)

		// Create authenticated downloader if needed
		var downloader *download.Client
		if cfg.HTTPAuthUser != "" || len(cfg.HTTPHeaders) > 0 {
			downloader = download.NewClientWithAuth(logger, cfg.HTTPAuthUser, cfg.HTTPAuthPassword, cfg.HTTPHeaders)
			logger.Debug("Using authenticated download client for bootstrap")
		} else {
			downloader = download.NewClient(logger)
		}
		downloader.SetRetryDefaults(cfg.MaxRetries, cfg.RetryDelay)

		// honor follow-redirects compat flag
		downloader.SetFollowRedirects(cfg.FollowRedirects)
		if err := downloader.DownloadFile(cfg.JSONURL, bootstrapPath, ""); err != nil {
			return nil, fmt.Errorf("failed to download bootstrap: %w", err)
		}

		// Load and parse bootstrap
		var bootstrap *config.Bootstrap
		var err error
		if cfg.SkipValidation {
			logger.Debug("SkipValidation=true: loading bootstrap without validation")
			bootstrap, err = config.LoadBootstrapWithOptions(bootstrapPath, false)
		} else {
			bootstrap, err = config.LoadBootstrap(bootstrapPath)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse bootstrap: %w", err)
		}

		return bootstrap, nil
	}

	// No JSON URL, try to load from mobile config
	logger.Info("Loading bootstrap from embedded mobile config")
	bootstrap, err := cfg.LoadBootstrapFromProfile(config.DefaultProfileDomain)
	if err != nil {
		return nil, fmt.Errorf("failed to load bootstrap from mobile config: %w", err)
	}

	return bootstrap, nil
}

// changeFileOwnershipToConsoleUser changes the ownership of a file to the current console user
// so that the agent (running as that user) can modify the file's permissions
func changeFileOwnershipToConsoleUser(filePath string, logger *utils.Logger) error {
	// Get the console user UID
	uid, err := utils.GetConsoleUserUID()
	if err != nil {
		return fmt.Errorf("failed to get console user UID: %w", err)
	}

	// Convert UID string to int
	var uidInt int
	if _, err := fmt.Sscanf(uid, "%d", &uidInt); err != nil {
		return fmt.Errorf("failed to parse UID %s: %w", uid, err)
	}

	// Change ownership to the console user
	if err := os.Chown(filePath, uidInt, -1); err != nil {
		return fmt.Errorf("failed to change ownership of %s to UID %d: %w", filePath, uidInt, err)
	}

	logger.Debug("Changed ownership of %s to UID %d", filePath, uidInt)
	return nil
}

// processUserlandPhase handles the complete userland phase including downloads and execution
func processUserlandPhase(userlandItems []config.Item, downloader *download.Client, systemInstaller *installer.SystemInstaller, cfg *config.Config, logger *utils.Logger) error {
	// Pre-download userland items
	logger.Info("Pre-downloading %d userland items", len(userlandItems))
	cleanupFailed := cfg.CleanupOnFailure && !cfg.KeepFailedFiles
	if !cleanupFailed && cfg.CleanupOnFailure {
		logger.Debug("KeepFailedFiles=true: preserving failed downloads for troubleshooting")
	}
	results := downloader.DownloadMultipleWithCleanup(userlandItems, cfg.DownloadMaxConcurrency, cleanupFailed)

	var downloadErrors []error
	var successCount int

	for _, result := range results {
		if result.Error != nil {
			logger.Error("Failed to download userland item '%s': %v", result.Item.Name, result.Error)
			downloadErrors = append(downloadErrors, result.Error)
		} else {
			logger.Debug("Pre-downloaded userland item: %s", result.Item.Name)
			successCount++
		}
	}

	if len(downloadErrors) > 0 {
		return fmt.Errorf("failed to download %d userland items: %d download errors", len(downloadErrors), len(downloadErrors))
	}

	logger.Info("Successfully pre-downloaded all %d userland items", successCount)

	// Wait for agent socket
	logger.Info("Waiting for GUI login and agent readiness to process userland phase")
	sockPath, err := waitForAgentSocket(logger, cfg.WaitForAgentTimeout)
	if err != nil {
		return fmt.Errorf("agent readiness wait failed: %w", err)
	}

	// Process userland items
	logger.Info("Starting ordered userland processing")
	successCount = 0
	var backgroundProcessCount int

	for i, item := range userlandItems {
		logger.Info("Userland item %d/%d: %s (%s)", i+1, len(userlandItems), item.Name, item.Type)
		switch item.Type {
		case "userscript":
			if err := processUserScript(item, sockPath, cfg, logger); err != nil {
				return fmt.Errorf("userscript failed for %s: %w", item.Name, err)
			}
			if item.DoNotWait {
				backgroundProcessCount++
				logger.Info("✅ User script delegated (background): %s", item.Name)
			} else {
				logger.Info("✅ User script completed: %s", item.Name)
			}
		case "userfile":
			if err := processUserFile(item, sockPath, cfg, logger); err != nil {
				return fmt.Errorf("userfile failed for %s: %w", item.Name, err)
			}
			logger.Info("✅ User file placed: %s", item.Name)
		case "package":
			if err := processPackage(item, systemInstaller, logger); err != nil {
				return fmt.Errorf("package failed for %s: %w", item.Name, err)
			}
			logger.Info("✅ Package installed: %s", item.Name)
		case "rootscript":
			if err := systemInstaller.ExecuteScript(item.File, "rootscript", item.DoNotWait, cfg.TrackBackgroundProcesses); err != nil {
				return fmt.Errorf("rootscript failed for %s: %w", item.Name, err)
			}
			if item.DoNotWait {
				backgroundProcessCount++
				logger.Info("✅ Root script started in background: %s", item.Name)
			} else {
				logger.Info("✅ Root script executed: %s", item.Name)
			}
		case "rootfile":
			if err := systemInstaller.PlaceFile(item.File, "rootfile"); err != nil {
				return fmt.Errorf("rootfile failed for %s: %w", item.Name, err)
			}
			logger.Info("✅ Root file placed: %s", item.Name)
		default:
			logger.Info("⚠️  Unknown item type: %s for %s", item.Type, item.Name)
		}
		successCount++
	}

	// Wait for background processes
	if backgroundProcessCount > 0 && cfg.TrackBackgroundProcesses {
		logger.Info("Waiting for %d background processes to complete", backgroundProcessCount)
		errors := systemInstaller.WaitForBackgroundProcesses(cfg.BackgroundTimeout)
		if len(errors) > 0 {
			logger.Error("Background process errors in userland:")
			for _, e := range errors {
				logger.Error("  - %v", e)
			}
			return fmt.Errorf("background processes failed: %d errors", len(errors))
		}
		logger.Info("All background processes completed successfully")
	}

	logger.Info("Userland processing completed")

	// Request agent shutdown
	if _, err := callAgent(logger, sockPath, ipc.RPCRequest{Command: "Shutdown"}, cfg.AgentRequestTimeout); err != nil {
		logger.Debug("Agent shutdown request failed (non-fatal): %v", err)
	}

	return nil
}

// processUserScript handles userscript execution via agent IPC
func processUserScript(item config.Item, sockPath string, cfg *config.Config, logger *utils.Logger) error {
	// Change ownership of user scripts to console user so agent can execute them
	if err := changeFileOwnershipToConsoleUser(item.File, logger); err != nil {
		return fmt.Errorf("failed to change ownership of user script %s: %w", item.Name, err)
	}

	// Delegate to agent via IPC
	resp, err := callAgent(logger, sockPath, ipc.RPCRequest{Command: "RunUserScript", Path: item.File, DoNotWait: item.DoNotWait}, cfg.AgentRequestTimeout)
	if err != nil || !resp.OK {
		return fmt.Errorf("agent userscript failed: %v %s", err, resp.Error)
	}
	return nil
}

// processUserFile handles userfile placement via agent IPC
func processUserFile(item config.Item, sockPath string, cfg *config.Config, logger *utils.Logger) error {
	// Change ownership of user files to console user so agent can modify them
	if err := changeFileOwnershipToConsoleUser(item.File, logger); err != nil {
		return fmt.Errorf("failed to change ownership of user file %s: %w", item.Name, err)
	}

	resp, err := callAgent(logger, sockPath, ipc.RPCRequest{Command: "PlaceUserFile", Path: item.File}, cfg.AgentRequestTimeout)
	if err != nil || !resp.OK {
		return fmt.Errorf("agent userfile failed: %v %s", err, resp.Error)
	}
	return nil
}

// processPackage handles package installation with optional receipt checking
func processPackage(item config.Item, systemInstaller *installer.SystemInstaller, logger *utils.Logger) error {
	// Optional: pkg_required check is handled in phase manager; perform simple install here
	if item.PkgRequired {
		isInstalled, checkErr := utils.CheckPackageReceipt(item.PackageID, item.Version, logger)
		if checkErr != nil {
			return fmt.Errorf("package receipt check failed: %w", checkErr)
		}
		if isInstalled {
			logger.Info("⏭️  Package %s already installed - skipping", item.Name)
			return nil
		}
	}
	if err := systemInstaller.InstallPackage(item.File, "/"); err != nil {
		return fmt.Errorf("failed to install package: %w", err)
	}
	return nil
}
