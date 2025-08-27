package installer

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/go-installapplications/pkg/utils"
)

// PackageInstaller handles macOS package installation
type PackageInstaller struct {
	dryRun      bool
	logger      *utils.Logger
	isAgentMode bool
}

// NewPackageInstaller creates a new package installer
func NewPackageInstaller(dryRun bool, logger *utils.Logger, isAgentMode bool) *PackageInstaller {
	return &PackageInstaller{
		dryRun:      dryRun,
		logger:      logger,
		isAgentMode: isAgentMode,
	}
}

// InstallPackage installs a .pkg file using the macOS installer command
func (pi *PackageInstaller) InstallPackage(pkgPath, target string) error {
	if target == "" {
		target = "/" // Default to root volume
	}

	pi.logger.Info("Installing package: %s to %s", pkgPath, target)
	pi.logger.Debug("Package installer dry-run mode: %t", pi.dryRun)

	// Log execution context
	if pi.isAgentMode {
		pi.logger.Debug("Installing package in agent mode - relies on proper authorization")
	}

	if pi.dryRun {
		pi.logger.Info("[DRY RUN] Would install: %s", pkgPath)
		return nil
	}

	// Build installer command
	// Both daemon and agent can install packages
	// Agent relies on proper authorization/signing to run installer
	cmd := exec.Command("installer", "-pkg", pkgPath, "-target", target)
	pi.logger.Debug("Executing installer (mode: %s): %s", func() string {
		if pi.isAgentMode {
			return "agent"
		} else {
			return "daemon/standalone"
		}
	}(), cmd.String())
	pi.logger.Verbose("Command args: %v", cmd.Args)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		pi.logger.Error("Installer command failed: %v", err)
		pi.logger.Debug("Installer output: %s", string(output))
		return fmt.Errorf("installer failed: %w, output: %s", err, string(output))
	}

	outputStr := strings.TrimSpace(string(output))
	pi.logger.Info("Package installed successfully")
	pi.logger.Debug("Installer output: %s", outputStr)
	return nil
}
