package utils

import (
	"fmt"
	"os/exec"
	"strings"
)

// CheckPackageReceipt checks if a package is installed using pkgutil
func CheckPackageReceipt(packageID, version string, logger *Logger) (bool, error) {
	if packageID == "" {
		logger.Debug("No package ID provided - skipping receipt check")
		return true, nil // If no packageID specified, assume it's okay
	}

	logger.Debug("Checking package receipt for: %s", packageID)

	// Check if package is installed
	cmd := exec.Command("pkgutil", "--pkg-info", packageID)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Package not installed
		logger.Debug("Package %s not found in receipts", packageID)
		return false, nil
	}

	outputStr := strings.TrimSpace(string(output))
	logger.Verbose("Package receipt info for %s: %s", packageID, outputStr)

	// If no version specified, just check existence
	if version == "" {
		logger.Debug("Package %s found in receipts (no version check)", packageID)
		return true, nil
	}

	// Check version if specified
	installedVersion, err := extractVersionFromPkgInfo(outputStr)
	if err != nil {
		logger.Debug("Could not extract version from package receipt: %v", err)
		return true, nil // If we can't parse version, assume it's okay
	}

	if installedVersion == version {
		logger.Debug("Package %s version %s matches required version", packageID, version)
		return true, nil
	} else {
		logger.Debug("Package %s installed version %s does not match required version %s", packageID, installedVersion, version)
		return false, nil
	}
}

// extractVersionFromPkgInfo extracts the version from pkgutil --pkg-info output
func extractVersionFromPkgInfo(output string) (string, error) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "version:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}
	return "", fmt.Errorf("version not found in package info")
}
