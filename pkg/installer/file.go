package installer

import (
	"fmt"
	"os"

	"github.com/go-installapplications/pkg/utils"
)

// FilePlacer handles file placement with appropriate permissions
type FilePlacer struct {
	dryRun      bool
	logger      *utils.Logger
	isAgentMode bool
}

// NewFilePlacer creates a new file placer
func NewFilePlacer(dryRun bool, logger *utils.Logger, isAgentMode bool) *FilePlacer {
	return &FilePlacer{
		dryRun:      dryRun,
		logger:      logger,
		isAgentMode: isAgentMode,
	}
}

// PlaceFile handles placing files with appropriate permissions
func (fp *FilePlacer) PlaceFile(filePath, fileType string) error {
	fp.logger.Info("Placing %s file: %s", fileType, filePath)
	fp.logger.Debug("File placer dry-run mode: %t", fp.dryRun)

	if fp.dryRun {
		fp.logger.Info("[DRY RUN] Would place file: %s (%s)", filePath, fileType)
		return nil
	}

	// Log execution context
	if fileType == "rootfile" && fp.isAgentMode {
		fp.logger.Debug("Placing rootfile in agent mode - relies on proper authorization")
	}
	if fileType == "userfile" && !fp.isAgentMode {
		// Note: userfiles should only be in userland phase, which normally runs via agent
		// This case indicates standalone mode processing userland items
		fp.logger.Debug("Placing userfile in standalone mode (userland phase)")
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", filePath)
	}

	fp.logger.Debug("File exists, setting permissions based on type: %s", fileType)

	// Set appropriate permissions based on file type
	switch fileType {
	case "rootfile":
		// Root-only readable
		if err := os.Chmod(filePath, 0644); err != nil {
			return fmt.Errorf("failed to set permissions on root file: %w", err)
		}
		fp.logger.Verbose("Set permissions to 0644 for root file: %s", filePath)
	case "userfile":
		// User-readable
		if err := os.Chmod(filePath, 0755); err != nil {
			return fmt.Errorf("failed to set permissions on user file: %w", err)
		}
		fp.logger.Verbose("Set permissions to 0755 for user file: %s", filePath)
	default:
		return fmt.Errorf("unknown file type: %s", fileType)
	}

	fp.logger.Info("File placed successfully: %s", filePath)
	return nil
}
