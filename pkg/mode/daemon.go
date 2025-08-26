package mode

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/go-installapplications/pkg/config"
	"github.com/go-installapplications/pkg/download"
	"github.com/go-installapplications/pkg/installer"
	"github.com/go-installapplications/pkg/ipc"
	"github.com/go-installapplications/pkg/phases"
	"github.com/go-installapplications/pkg/retry"
	"github.com/go-installapplications/pkg/utils"
)

// RunDaemon executes the daemon mode workflow
func RunDaemon(cfg *config.Config, logger *utils.Logger) {
	logger.Info("Starting daemon mode")

	// Check retry logic
	if shouldRetry, err := retry.ShouldRetry(); !shouldRetry {
		logger.Error("Maximum retry attempts exceeded: %v", err)
		os.Exit(0) // Successful exit = don't restart
	}

	logger.Info("Daemon attempt: %s", retry.GetRetryInfo())

	if err := retry.IncrementRetryCount("daemon started"); err != nil {
		logger.Error("Failed to update retry count: %v", err)
	}

	// Get bootstrap from either JSON URL or embedded mobile config
	bootstrap, err := getBootstrap(cfg, logger)
	if err != nil {
		logger.Error("Failed to get bootstrap: %v", err)
		retry.IncrementRetryCount(fmt.Sprintf("bootstrap failed: %v", err))
		os.Exit(1)
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
	manager := phases.NewManager(downloader, systemInstaller, cfg, logger)

	// Process preflight phase
	if len(bootstrap.Preflight) > 0 {
		logger.Info("Starting preflight phase")
		if err := manager.ProcessItems(bootstrap.Preflight, "preflight"); err != nil {
			logger.Error("Preflight phase failed: %v", err)
			retry.IncrementRetryCount(fmt.Sprintf("preflight failed: %v", err))
			os.Exit(1)
		}
		logger.Info("Preflight phase completed successfully")
	} else {
		logger.Debug("No preflight items to process")
	}

	// Process setupassistant phase
	if len(bootstrap.SetupAssistant) > 0 {
		logger.Info("Starting setupassistant phase")
		if err := manager.ProcessItems(bootstrap.SetupAssistant, "setupassistant"); err != nil {
			logger.Error("Setupassistant phase failed: %v", err)
			retry.IncrementRetryCount(fmt.Sprintf("setupassistant failed: %v", err))
			os.Exit(1)
		}
		logger.Info("Setupassistant phase completed successfully")
	} else {
		logger.Debug("No setupassistant items to process")
	}

	// Pre-download userland items (daemon will orchestrate userland in order)
	if len(bootstrap.Userland) > 0 {
		logger.Info("Pre-downloading %d userland items", len(bootstrap.Userland))
		cleanupFailed := cfg.CleanupOnFailure && !cfg.KeepFailedFiles
		if !cleanupFailed && cfg.CleanupOnFailure {
			logger.Debug("KeepFailedFiles=true: preserving failed downloads for troubleshooting")
		}
		results := downloader.DownloadMultipleWithCleanup(bootstrap.Userland, cfg.DownloadMaxConcurrency, cleanupFailed)

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
			logger.Error("Failed to download %d userland items", len(downloadErrors))
			retry.IncrementRetryCount(fmt.Sprintf("userland downloads failed: %d errors", len(downloadErrors)))
			os.Exit(1)
		}

		logger.Info("Successfully pre-downloaded all %d userland items", successCount)

		// Orchestrate userland phase in order with the daemon as the single orchestrator
		logger.Info("Waiting for GUI login and agent readiness to process userland phase")
		// Wait for agent socket using configured timeout
		sockPath, err := waitForAgentSocket(logger, cfg.WaitForAgentTimeout)
		if err != nil {
			logger.Error("Agent readiness wait failed: %v", err)
			os.Exit(1)
		}

		logger.Info("Starting ordered userland processing")
		// Create installer for root-context items
		systemInstaller := installer.NewSystemInstaller(cfg.DryRun, logger, false)
		successCount = 0
		var backgroundProcessCount int

		for i, item := range bootstrap.Userland {
			logger.Info("Userland item %d/%d: %s (%s)", i+1, len(bootstrap.Userland), item.Name, item.Type)
			switch item.Type {
			case "userscript":
				// Change ownership of user scripts to console user so agent can execute them
				if err := changeFileOwnershipToConsoleUser(item.File, logger); err != nil {
					logger.Error("Failed to change ownership of user script %s: %v", item.Name, err)
					os.Exit(1)
				}

				// Delegate to agent via IPC
				resp, err := callAgent(logger, sockPath, ipc.RPCRequest{Command: "RunUserScript", Path: item.File, DoNotWait: item.DoNotWait}, cfg.AgentRequestTimeout)
				if err != nil || !resp.OK {
					logger.Error("Agent userscript failed for %s: %v %s", item.Name, err, resp.Error)
					os.Exit(1)
				}
				if item.DoNotWait {
					backgroundProcessCount++
					logger.Info("✅ User script delegated (background): %s", item.Name)
				} else {
					logger.Info("✅ User script completed: %s", item.Name)
				}
			case "userfile":
				// Change ownership of user files to console user so agent can modify them
				if err := changeFileOwnershipToConsoleUser(item.File, logger); err != nil {
					logger.Error("Failed to change ownership of user file %s: %v", item.Name, err)
					os.Exit(1)
				}

				resp, err := callAgent(logger, sockPath, ipc.RPCRequest{Command: "PlaceUserFile", Path: item.File}, cfg.AgentRequestTimeout)
				if err != nil || !resp.OK {
					logger.Error("Agent userfile failed for %s: %v %s", item.Name, err, resp.Error)
					os.Exit(1)
				}
				logger.Info("✅ User file placed: %s", item.Name)
			case "package":
				// Optional: pkg_required check is handled in phase manager; perform simple install here
				if item.PkgRequired {
					isInstalled, checkErr := utils.CheckPackageReceipt(item.PackageID, item.Version, logger)
					if checkErr != nil {
						logger.Error("Package receipt check failed for %s: %v", item.Name, checkErr)
						os.Exit(1)
					}
					if isInstalled {
						logger.Info("⏭️  Package %s already installed - skipping", item.Name)
						continue
					}
				}
				if err := systemInstaller.InstallPackage(item.File, "/"); err != nil {
					logger.Error("Failed to install package %s: %v", item.Name, err)
					os.Exit(1)
				}
				logger.Info("✅ Package installed: %s", item.Name)
			case "rootscript":
				if err := systemInstaller.ExecuteScript(item.File, "rootscript", item.DoNotWait, cfg.TrackBackgroundProcesses); err != nil {
					logger.Error("Failed to execute root script %s: %v", item.Name, err)
					os.Exit(1)
				}
				if item.DoNotWait {
					backgroundProcessCount++
					logger.Info("✅ Root script started in background: %s", item.Name)
				} else {
					logger.Info("✅ Root script executed: %s", item.Name)
				}
			case "rootfile":
				if err := systemInstaller.PlaceFile(item.File, "rootfile"); err != nil {
					logger.Error("Failed to place root file %s: %v", item.Name, err)
					os.Exit(1)
				}
				logger.Info("✅ Root file placed: %s", item.Name)
			default:
				logger.Info("⚠️  Unknown item type: %s for %s", item.Type, item.Name)
			}
			successCount++
		}

		if backgroundProcessCount > 0 && cfg.TrackBackgroundProcesses {
			logger.Info("Waiting for %d background processes to complete", backgroundProcessCount)
			errors := systemInstaller.WaitForBackgroundProcesses(cfg.BackgroundTimeout)
			if len(errors) > 0 {
				logger.Error("Background process errors in userland:")
				for _, e := range errors {
					logger.Error("  - %v", e)
				}
				os.Exit(1)
			}
			logger.Info("All background processes completed successfully")
		}

		logger.Info("Userland processing completed")
		// Request agent shutdown
		if _, err := callAgent(logger, sockPath, ipc.RPCRequest{Command: "Shutdown"}, cfg.AgentRequestTimeout); err != nil {
			logger.Debug("Agent shutdown request failed (non-fatal): %v", err)
		}
	} else {
		logger.Debug("No userland items present")
	}

	// Success!
	logger.Info("Daemon completed all phases successfully!")

	// Clear retry counter
	if err := retry.ClearRetryCount(); err != nil {
		logger.Error("Failed to clear retry count: %v", err)
	}

	// Optional reboot after successful completion
	if cfg.Reboot {
		logger.Info("Reboot flag is set; system will reboot in 5 seconds")
		time.Sleep(5 * time.Second)
		cmd := exec.Command("/sbin/shutdown", "-r", "now")
		if err := cmd.Start(); err != nil {
			logger.Error("Failed to initiate reboot: %v", err)
		}
	}

	logger.Info("Daemon exiting with success")
	os.Exit(0)
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
	uid, err := getConsoleUserUID()
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
