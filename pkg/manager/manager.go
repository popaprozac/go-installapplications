package manager

import (
	"fmt"
	"sync"

	"github.com/go-installapplications/pkg/config"
	"github.com/go-installapplications/pkg/download"
	"github.com/go-installapplications/pkg/installer"
	"github.com/go-installapplications/pkg/utils"
)

// itemResult carries the outcome of a single item execution so callers can
// apply fail_policy uniformly across sequential and parallel paths.
type itemResult struct {
	item        config.Item
	err         error
	operation   string // for handleItemError ("script execution", "package installation", ...)
	startedBg   bool   // true if a tracked background process was started
}

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

	// Preflight is a single-item phase with bespoke control flow; route it
	// directly through the preflight handler regardless of parallel_group.
	if phaseName == "preflight" {
		for _, item := range successfulItems {
			if item.Type == "rootscript" {
				return m.handlePreflightScript(item)
			}
		}
	}

	batches := config.BatchByParallelGroup(successfulItems)
	for batchIdx, batch := range batches {
		if len(batch) == 1 {
			item := batch[0]
			m.logger.Debug("Processing item %d/%d (batch %d): %s (%s)", batchIdx+1, len(batches), batchIdx+1, item.Name, item.Type)
			if item.DoNotWait {
				if m.config.TrackBackgroundProcesses {
					m.logger.Debug("Item marked as donotwait with background tracking")
				} else {
					m.logger.Debug("Item marked as donotwait with fire-and-forget")
				}
			}
			res := m.runItem(item, phaseName)
			if res.startedBg {
				backgroundProcessCount++
			}
			if res.err != nil {
				if m.handleItemError(item, res.err, res.operation) {
					return fmt.Errorf("%s failed in %s phase for %s: %w", res.operation, phaseName, item.Name, res.err)
				}
			}
			continue
		}

		// Parallel batch — every item in the batch shares the same non-empty group.
		groupName := batch[0].ParallelGroup
		m.logger.Info("🔀 parallel_group %q: running %d items concurrently", groupName, len(batch))

		results := make([]itemResult, len(batch))
		var wg sync.WaitGroup
		for i := range batch {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				results[i] = m.runItem(batch[i], phaseName)
			}(i)
		}
		wg.Wait()

		// Apply fail_policy to each result in the batch's declared order so
		// log output remains deterministic.
		for _, res := range results {
			if res.startedBg {
				backgroundProcessCount++
			}
			if res.err != nil {
				if m.handleItemError(res.item, res.err, res.operation) {
					return fmt.Errorf("parallel_group %q: %s failed for %s: %w", groupName, res.operation, res.item.Name, res.err)
				}
			}
		}
		m.logger.Info("✅ parallel_group %q complete", groupName)
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

// runItem executes one item and returns the outcome WITHOUT consulting
// fail_policy — the caller (which may have been parallel or sequential)
// decides what to do with the error. This is the unifying primitive used by
// both the singleton and parallel-batch paths.
func (m *Manager) runItem(item config.Item, phaseName string) itemResult {
	switch item.Type {
	case "package":
		return m.runPackage(item)
	case "rootscript":
		// Preflight is handled separately at the ProcessItems level.
		_ = phaseName
		return m.runRootScript(item)
	case "userscript":
		return m.runUserScript(item)
	case "rootfile":
		return m.runFilePlacement(item, "rootfile")
	case "userfile":
		return m.runFilePlacement(item, "userfile")
	default:
		m.logger.Info("⚠️  Unknown item type: %s for %s", item.Type, item.Name)
		return itemResult{item: item, operation: "dispatch"}
	}
}

func (m *Manager) runRootScript(item config.Item) itemResult {
	err := m.installer.ExecuteScript(item.File, "rootscript", item.DoNotWait, m.config.TrackBackgroundProcesses)
	res := itemResult{item: item, operation: "script execution", err: err}
	if err == nil {
		if item.DoNotWait {
			if m.config.TrackBackgroundProcesses {
				res.startedBg = true
				m.logger.Info("✅ Root script started in background: %s", item.Name)
			} else {
				m.logger.Info("✅ Root script started (fire-and-forget): %s", item.Name)
			}
		} else {
			m.logger.Info("✅ Root script executed: %s", item.Name)
		}
	}
	return res
}

func (m *Manager) runFilePlacement(item config.Item, fileType string) itemResult {
	err := m.installer.PlaceFile(item.File, fileType)
	res := itemResult{item: item, operation: "file placement", err: err}
	if err == nil {
		m.logger.Info("✅ %s placed: %s", fileType, item.Name)
	}
	return res
}

func (m *Manager) runUserScript(item config.Item) itemResult {
	err := m.installer.ExecuteScript(item.File, "userscript", item.DoNotWait, m.config.TrackBackgroundProcesses)
	res := itemResult{item: item, operation: "script execution", err: err}
	if err == nil {
		if item.DoNotWait {
			if m.config.TrackBackgroundProcesses {
				res.startedBg = true
				m.logger.Info("✅ User script started in background: %s", item.Name)
			} else {
				m.logger.Info("✅ User script started (fire-and-forget): %s", item.Name)
			}
		} else {
			m.logger.Info("✅ User script executed: %s", item.Name)
		}
	}
	return res
}

func (m *Manager) runPackage(item config.Item) itemResult {
	if !item.PkgRequired && item.PackageID != "" {
		alreadySatisfied, err := utils.CheckPackageReceipt(item.PackageID, item.Version, m.logger)
		if err != nil {
			return itemResult{item: item, operation: "package receipt check", err: err}
		}
		if alreadySatisfied {
			m.logger.Info("⏭️  Skipping %s - already installed.", item.Name)
			return itemResult{item: item, operation: "package installation"}
		}
	}
	err := m.installer.InstallPackage(item.File, "/")
	res := itemResult{item: item, operation: "package installation", err: err}
	if err == nil {
		m.logger.Info("✅ Package installed: %s", item.Name)
	}
	return res
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
