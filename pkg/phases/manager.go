package phases

import (
	"fmt"

	"github.com/go-installapplications/pkg/config"
	"github.com/go-installapplications/pkg/download"
	"github.com/go-installapplications/pkg/installer"
	"github.com/go-installapplications/pkg/utils"
)

// Manager orchestrates the three-phase installation process
type Manager struct {
	downloader     download.Downloader
	installer      installer.Installer
	config         *config.Config
	logger         *utils.Logger
	cleanupTracker *download.CleanupTracker
}

// NewManager creates a new phase manager
func NewManager(downloader download.Downloader, installer installer.Installer, cfg *config.Config, logger *utils.Logger) *Manager {
	return &Manager{
		downloader:     downloader,
		installer:      installer,
		config:         cfg,
		logger:         logger,
		cleanupTracker: download.NewCleanupTracker(),
	}
}

// ProcessItems downloads and installs a list of items with cleanup
func (m *Manager) ProcessItems(items []config.Item, phaseName string) error {
	if len(items) == 0 {
		return nil
	}

	m.logger.Info("=== Processing %s phase ===", phaseName)

	// Filter items based on skip_if criteria
	var filteredItems []config.Item
	var skippedCount int

	for _, item := range items {
		if utils.ShouldSkipItem(item.SkipIf, m.logger) {
			m.logger.Info("⏭️  Skipping %s: matches skip_if criteria '%s'", item.Name, item.SkipIf)
			skippedCount++
		} else {
			filteredItems = append(filteredItems, item)
		}
	}

	m.logger.Info("Processing %d items (%d skipped)", len(filteredItems), skippedCount)

	if len(filteredItems) == 0 {
		m.logger.Info("No items to process after filtering")
		return nil
	}

	m.logger.Info("Starting parallel downloads for %d filtered items", len(filteredItems))

	// Download filtered items in parallel (respect config concurrency and KeepFailedFiles)
	maxConcurrency := m.config.DownloadMaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 4
	}
	cleanupFailed := m.config.CleanupOnFailure && !m.config.KeepFailedFiles
	if !cleanupFailed && m.config.CleanupOnFailure {
		m.logger.Debug("KeepFailedFiles=true: preserving failed downloads for troubleshooting")
	}
	// Track all target file paths for potential cleanup-on-success
	for _, item := range filteredItems {
		if item.File != "" {
			m.cleanupTracker.TrackFile(item.File)
		}
	}

	results := m.downloader.DownloadMultipleWithCleanup(filteredItems, maxConcurrency, cleanupFailed)

	// Check download results and install successful items
	var downloadErrors []error
	var successfulItems []config.Item

	for _, result := range results {
		if result.Error != nil {
			m.logger.Error("❌ Download failed: %s - %v", result.Item.Name, result.Error)
			downloadErrors = append(downloadErrors, result.Error)
		} else {
			m.logger.Info("✅ Download success: %s", result.Item.Name)
			successfulItems = append(successfulItems, result.Item)
		}
	}

	// If any downloads failed, stop here
	if len(downloadErrors) > 0 {
		return fmt.Errorf("failed to download %d items in %s phase, first error: %w", len(downloadErrors), phaseName, downloadErrors[0])
	}

	// Install/execute successful downloads
	m.logger.Info("Installing %d successfully downloaded items", len(successfulItems))

	var backgroundProcessCount int

	for i, item := range successfulItems {
		m.logger.Info("Processing item %d/%d: %s (%s)", i+1, len(successfulItems), item.Name, item.Type)

		// Log donotwait behavior if enabled
		if item.DoNotWait {
			if m.config.TrackBackgroundProcesses {
				m.logger.Debug("Item marked as donotwait with background tracking")
			} else {
				m.logger.Debug("Item marked as donotwait with fire-and-forget")
			}
		}

		switch item.Type {
		case "package":
			// Check pkg_required before installation
			if item.PkgRequired {
				m.logger.Debug("Checking if package %s is already installed (pkg_required=true)", item.Name)
				isInstalled, err := utils.CheckPackageReceipt(item.PackageID, item.Version, m.logger)
				if err != nil {
					if shouldStopOnError := m.handleItemError(item, err, "package receipt check"); shouldStopOnError {
						return fmt.Errorf("failed to check package receipt for %s: %w", item.Name, err)
					}
					continue
				}
				if isInstalled {
					m.logger.Info("⏭️  Package %s already installed - skipping", item.Name)
					continue
				}
				m.logger.Debug("Package %s not installed or version mismatch - proceeding with installation", item.Name)
			}

			err := m.installer.InstallPackage(item.File, "/")
			if err != nil {
				if shouldStopOnError := m.handleItemError(item, err, "package installation"); shouldStopOnError {
					return fmt.Errorf("failed to install package %s: %w", item.Name, err)
				}
				continue
			}
			m.logger.Info("✅ Package installed: %s", item.Name)
			// On success, we can mark the file as preserved for now; cleanup-all will remove it later if enabled

		case "rootscript":
			err := m.installer.ExecuteScript(item.File, "rootscript", item.DoNotWait, m.config.TrackBackgroundProcesses)
			if err != nil {
				if shouldStopOnError := m.handleItemError(item, err, "script execution"); shouldStopOnError {
					return fmt.Errorf("failed to execute root script %s: %w", item.Name, err)
				}
				continue
			}
			if item.DoNotWait {
				if m.config.TrackBackgroundProcesses {
					backgroundProcessCount++
					m.logger.Info("✅ Root script started in background: %s", item.Name)
				} else {
					m.logger.Info("✅ Root script started (fire-and-forget): %s", item.Name)
				}
			} else {
				m.logger.Info("✅ Root script executed: %s", item.Name)
			}

		case "userscript":
			err := m.installer.ExecuteScript(item.File, "userscript", item.DoNotWait, m.config.TrackBackgroundProcesses)
			if err != nil {
				if shouldStopOnError := m.handleItemError(item, err, "script execution"); shouldStopOnError {
					return fmt.Errorf("failed to execute user script %s: %w", item.Name, err)
				}
				continue
			}
			if item.DoNotWait {
				if m.config.TrackBackgroundProcesses {
					backgroundProcessCount++
					m.logger.Info("✅ User script started in background: %s", item.Name)
				} else {
					m.logger.Info("✅ User script started (fire-and-forget): %s", item.Name)
				}
			} else {
				m.logger.Info("✅ User script executed: %s", item.Name)
			}

		case "rootfile":
			err := m.installer.PlaceFile(item.File, "rootfile")
			if err != nil {
				if shouldStopOnError := m.handleItemError(item, err, "file placement"); shouldStopOnError {
					return fmt.Errorf("failed to place root file %s: %w", item.Name, err)
				}
				continue
			}
			m.logger.Info("✅ Root file placed: %s", item.Name)

		case "userfile":
			err := m.installer.PlaceFile(item.File, "userfile")
			if err != nil {
				if shouldStopOnError := m.handleItemError(item, err, "file placement"); shouldStopOnError {
					return fmt.Errorf("failed to place user file %s: %w", item.Name, err)
				}
				continue
			}
			m.logger.Info("✅ User file placed: %s", item.Name)

		default:
			m.logger.Info("⚠️  Unknown item type: %s for %s", item.Type, item.Name)
		}
	}

	// Wait for background processes started in THIS PHASE ONLY
	if backgroundProcessCount > 0 && m.config.TrackBackgroundProcesses {
		m.logger.Info("Waiting for %d background processes from %s phase to complete", backgroundProcessCount, phaseName)
		errors := m.installer.WaitForBackgroundProcesses(m.config.BackgroundTimeout)

		if len(errors) > 0 {
			m.logger.Error("Background process errors in %s phase:", phaseName)
			for _, err := range errors {
				m.logger.Error("  - %v", err)
			}
			return fmt.Errorf("background processes failed in %s phase: %d errors", phaseName, len(errors))
		}

		m.logger.Info("All background processes from %s phase completed successfully", phaseName)
	}

	m.logger.Info("=== Completed %s phase ===", phaseName)

	// Cleanup on success, if configured
	if m.config.CleanupOnSuccess {
		m.logger.Debug("CleanupOnSuccess=true: removing downloaded artifacts for %s phase", phaseName)
		if err := m.cleanupTracker.CleanupAll(); err != nil {
			m.logger.Debug("CleanupOnSuccess encountered errors: %v", err)
		}
	}
	return nil
}

// handleItemError processes errors according to the item's fail policy
// Returns true if the phase should stop, false if it should continue
func (m *Manager) handleItemError(item config.Item, err error, operation string) bool {
	policy := item.GetEffectiveFailPolicy()

	switch policy {
	case "failure_is_not_an_option":
		// Stop entire phase on any failure (default behavior)
		m.logger.Error("❌ %s failed for %s (fail_policy: %s): %v", operation, item.Name, policy, err)
		return true

	case "failable":
		// Log error but continue with phase (all failures are ignored)
		m.logger.Info("⚠️  %s failed for %s (fail_policy: %s): %v - continuing", operation, item.Name, policy, err)
		return false

	case "failable_execution":
		// Allow script execution failures, but not download/install failures
		if operation == "script execution" {
			m.logger.Info("⚠️  %s failed for %s (fail_policy: %s): %v - continuing", operation, item.Name, policy, err)
			return false
		} else {
			// Download/install failures still stop the phase
			m.logger.Error("❌ %s failed for %s (fail_policy: %s): %v", operation, item.Name, policy, err)
			return true
		}

	default:
		// Should never happen due to validation, but be safe
		m.logger.Error("❌ Unknown fail_policy '%s' for %s: %v", policy, item.Name, err)
		return true
	}
}
