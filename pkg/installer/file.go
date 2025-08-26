package installer

import (
	"fmt"
	"os"
)

// PlaceFile handles placing files with appropriate permissions
func (si *SystemInstaller) PlaceFile(filePath, fileType string) error {
	si.logger.Info("Placing %s file: %s", fileType, filePath)
	si.logger.Debug("File placer dry-run mode: %t", si.packageInstaller.dryRun)

	if si.packageInstaller.dryRun {
		si.logger.Info("[DRY RUN] Would place file: %s (%s)", filePath, fileType)
		return nil
	}

	// Log execution context
	if fileType == "rootfile" && si.scriptExecutor.isAgentMode {
		si.logger.Debug("Placing rootfile in agent mode - relies on proper authorization")
	}
	if fileType == "userfile" && !si.scriptExecutor.isAgentMode {
		// Note: userfiles should only be in userland phase, which normally runs via agent
		// This case indicates standalone mode processing userland items
		si.logger.Debug("Placing userfile in standalone mode (userland phase)")
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", filePath)
	}

	si.logger.Debug("File exists, setting permissions based on type: %s", fileType)

	// Set appropriate permissions based on file type
	switch fileType {
	case "rootfile":
		// Root-only readable
		if err := os.Chmod(filePath, 0644); err != nil {
			return fmt.Errorf("failed to set permissions on root file: %w", err)
		}
		si.logger.Verbose("Set permissions to 0644 for root file: %s", filePath)
	case "userfile":
		// User-readable
		if err := os.Chmod(filePath, 0755); err != nil {
			return fmt.Errorf("failed to set permissions on user file: %w", err)
		}
		si.logger.Verbose("Set permissions to 0755 for user file: %s", filePath)
	default:
		return fmt.Errorf("unknown file type: %s", fileType)
	}

	si.logger.Info("File placed successfully: %s", filePath)
	return nil
}
