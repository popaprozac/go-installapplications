package manager

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

	// Validate phase restrictions
	if err := m.validatePhaseRestrictions(items, phaseName); err != nil {
		return err
	}

	m.logger.Info("📋 Processing %s phase", phaseName)

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
			m.logger.Debug("✅ Download success: %s", result.Item.Name)
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
		m.logger.Debug("Processing item %d/%d: %s (%s)", i+1, len(successfulItems), item.Name, item.Type)

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
			if err := m.handlePackageInstallation(item); err != nil {
				return err
			}

		case "rootscript":
			if phaseName == "preflight" {
				return m.handlePreflightScript(item)
			} else {
				if err := m.handleRootScript(item, &backgroundProcessCount); err != nil {
					return err
				}
			}

		case "userscript":
			if err := m.handleUserScript(item, &backgroundProcessCount); err != nil {
				return err
			}

		case "rootfile":
			if err := m.handleFilePlacement(item, "rootfile"); err != nil {
				return err
			}

		case "userfile":
			if err := m.handleFilePlacement(item, "userfile"); err != nil {
				return err
			}

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

	m.logger.Info("✅ Completed %s phase", phaseName)

	// Cleanup on success, if configured
	if m.config.CleanupOnSuccess {
		m.logger.Debug("CleanupOnSuccess=true: removing downloaded artifacts for %s phase", phaseName)
		if err := m.cleanupTracker.CleanupAll(); err != nil {
			m.logger.Debug("CleanupOnSuccess encountered errors: %v", err)
		}
	}
	return nil
}

// handleRootScript handles root script execution (non-preflight)
func (m *Manager) handleRootScript(item config.Item, backgroundProcessCount *int) error {
	// Normal script execution for non-preflight phases
	err := m.installer.ExecuteScript(item.File, "rootscript", item.DoNotWait, m.config.TrackBackgroundProcesses)
	if err != nil {
		// Normal error handling for non-preflight phases
		if shouldStopOnError := m.handleItemError(item, err, "script execution"); shouldStopOnError {
			return fmt.Errorf("failed to execute root script %s: %w", item.Name, err)
		}
		return nil // Continue with next item
	}

	// Log success based on execution mode
	if item.DoNotWait {
		if m.config.TrackBackgroundProcesses {
			*backgroundProcessCount++
			m.logger.Info("✅ Root script started in background: %s", item.Name)
		} else {
			m.logger.Info("✅ Root script started (fire-and-forget): %s", item.Name)
		}
	} else {
		m.logger.Info("✅ Root script executed: %s", item.Name)
	}
	return nil
}

// handleFilePlacement handles file placement for both root and user files
func (m *Manager) handleFilePlacement(item config.Item, fileType string) error {
	err := m.installer.PlaceFile(item.File, fileType)
	if err != nil {
		if shouldStopOnError := m.handleItemError(item, err, "file placement"); shouldStopOnError {
			return fmt.Errorf("failed to place %s %s: %w", fileType, item.Name, err)
		}
		return nil // Continue with next item
	}
	m.logger.Info("✅ %s placed: %s", fileType, item.Name)
	return nil
}

// handleUserScript handles user script execution
func (m *Manager) handleUserScript(item config.Item, backgroundProcessCount *int) error {
	err := m.installer.ExecuteScript(item.File, "userscript", item.DoNotWait, m.config.TrackBackgroundProcesses)
	if err != nil {
		if shouldStopOnError := m.handleItemError(item, err, "script execution"); shouldStopOnError {
			return fmt.Errorf("failed to execute user script %s: %w", item.Name, err)
		}
		return nil // Continue with next item
	}

	// Log success based on execution mode
	if item.DoNotWait {
		if m.config.TrackBackgroundProcesses {
			*backgroundProcessCount++
			m.logger.Info("✅ User script started in background: %s", item.Name)
		} else {
			m.logger.Info("✅ User script started (fire-and-forget): %s", item.Name)
		}
	} else {
		m.logger.Info("✅ User script executed: %s", item.Name)
	}
	return nil
}

// handlePackageInstallation installs a package. Skips if already installed (version >= required) unless pkg_required is true.
func (m *Manager) handlePackageInstallation(item config.Item) error {
	// When pkg_required is false (default): skip if already installed with satisfied version (loose >=).
	// When pkg_required is true: always install, no skip based on receipt.
	if !item.PkgRequired && item.PackageID != "" {
		alreadySatisfied, err := utils.CheckPackageReceipt(item.PackageID, item.Version, m.logger)
		if err != nil {
			if shouldStopOnError := m.handleItemError(item, err, "package receipt check"); shouldStopOnError {
				return fmt.Errorf("failed to check package receipt for %s: %w", item.Name, err)
			}
			return nil
		}
		if alreadySatisfied {
			m.logger.Info("⏭️  Skipping %s - already installed.", item.Name)
			return nil
		}
	}

	err := m.installer.InstallPackage(item.File, "/")
	if err != nil {
		if shouldStopOnError := m.handleItemError(item, err, "package installation"); shouldStopOnError {
			return fmt.Errorf("failed to install package %s: %w", item.Name, err)
		}
		return nil // Continue with next item
	}
	m.logger.Info("✅ Package installed: %s", item.Name)
	// On success, we can mark the file as preserved for now; cleanup-all will remove it later if enabled
	return nil
}

// handlePreflightScript handles the special case of preflight rootscript execution
// Returns PreflightSuccessError on exit code 0, nil on exit code 1+, or error on execution failure
func (m *Manager) handlePreflightScript(item config.Item) error {
	// Use the preflight-specific method that handles exit codes internally
	err := m.installer.ExecuteScriptForPreflight(item.File, "rootscript", item.DoNotWait, m.config.TrackBackgroundProcesses)

	// Check if this is a preflight success signal
	if _, ok := err.(*installer.PreflightSuccessError); ok {
		m.logger.Info("✅ Preflight script %s passed (exit code 0) - performing full cleanup and exiting", item.Name)

		// Perform complete cleanup (files, services, reboot if configured)
		m.Cleanup("preflight success")

		return err // Return the PreflightSuccessError to signal preflight success
	} else if err != nil {
		// Script execution failed (e.g., script not found, permission denied)
		// Note: Preflight ignores fail_policy - only execution errors stop the process
		m.logger.Error("❌ Preflight script execution failed for %s: %v", item.Name, err)
		return fmt.Errorf("failed to execute preflight script %s: %w", item.Name, err)
	} else {
		// Script executed but returned non-zero exit code (err is nil, continue with bootstrap)
		m.logger.Info("⚠️  Preflight script %s failed (non-zero exit code) - continuing with bootstrap", item.Name)
		// Continue with setupassistant and userland phases
		return nil
	}
}

// Cleanup performs manager's own cleanup (files, based on flags)
func (m *Manager) Cleanup(cleanupType string) {
	m.logger.Info("🧹 Performing %s cleanup", cleanupType)

	// Always clean up files if either flag is true
	if m.config.CleanupOnSuccess || m.config.CleanupOnFailure {
		m.logger.Debug("Cleanup flags enabled: removing downloaded artifacts")
		if err := m.cleanupTracker.CleanupAll(); err != nil {
			m.logger.Debug("File cleanup encountered errors: %v", err)
		}
	} else {
		m.logger.Debug("Cleanup flags disabled: preserving downloaded artifacts")
	}
}

// handleItemError processes errors according to the item's fail policy
// Returns true if the phase should stop, false if it should continue
func (m *Manager) handleItemError(item config.Item, err error, operation string) bool {
	stop := item.ShouldStopOnError(operation)
	policy := item.GetEffectiveFailPolicy()
	if stop {
		m.logger.Error("❌ %s failed for %s (fail_policy: %s): %v", operation, item.Name, policy, err)
	} else {
		m.logger.Info("⚠️  %s failed for %s (fail_policy: %s): %v - continuing", operation, item.Name, policy, err)
	}
	return stop
}

// validatePhaseRestrictions validates that items are appropriate for the given phase
func (m *Manager) validatePhaseRestrictions(items []config.Item, phaseName string) error {
	for _, item := range items {
		switch item.Type {
		case "userscript", "userfile":
			if phaseName == "setupassistant" {
				return fmt.Errorf("userscript and userfile items are not allowed in setupassistant phase: %s", item.Name)
			}
			if phaseName == "preflight" {
				return fmt.Errorf("userscript and userfile items are not allowed in preflight phase: %s", item.Name)
			}
			// userland phase allows userscripts/userfiles (handled by daemon via IPC)
			// standalone mode allows userscripts/userfiles (handled directly by manager)
		}
	}
	return nil
}
