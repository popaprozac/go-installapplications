package utils

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// LooseVersionCompare compares two version strings. Returns -1 if a < b, 0 if a == b, 1 if a > b.
// E.g. "10.6" and "10.6.0" are equal.
func LooseVersionCompare(a, b string) int {
	partsA := parseLooseVersion(a)
	partsB := parseLooseVersion(b)
	maxLen := len(partsA)
	if len(partsB) > maxLen {
		maxLen = len(partsB)
	}
	for i := 0; i < maxLen; i++ {
		var va, vb int
		if i < len(partsA) {
			va = partsA[i]
		}
		if i < len(partsB) {
			vb = partsB[i]
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	return 0
}

// parseLooseVersion splits a version string into numeric (and optionally trailing string) components.
func parseLooseVersion(s string) []int {
	if s == "" {
		return nil
	}
	// Match digit sequences and optional non-digit suffix per segment (e.g. "1.0.0b1" -> 1, 0, 0)
	re := regexp.MustCompile(`(\d+)|([a-zA-Z]+)|\.`)
	parts := re.FindAllString(s, -1)
	var result []int
	for _, p := range parts {
		if p == "." {
			continue
		}
		if n, err := strconv.Atoi(p); err == nil {
			result = append(result, n)
		} else {
			// Treat non-numeric as 0 for comparison (e.g. "b" in "1.0b1")
			result = append(result, 0)
		}
	}
	return result
}

// CheckPackageReceipt reports whether the package is installed and satisfies the required version (loose: installed >= required).
// Caller skips install when true and pkg_required is false.
func CheckPackageReceipt(packageID, version string, logger *Logger) (bool, error) {
	if packageID == "" {
		logger.Debug("No package ID provided - skipping receipt check")
		return false, nil // Not "already satisfied" so caller will proceed
	}

	logger.Debug("Checking package receipt for: %s", packageID)

	cmd := exec.Command("pkgutil", "--pkg-info", packageID)
	output, err := cmd.CombinedOutput()

	if err != nil {
		logger.Debug("Package %s not found in receipts", packageID)
		return false, nil
	}

	outputStr := strings.TrimSpace(string(output))
	logger.Verbose("Package receipt info for %s: %s", packageID, outputStr)

	if version == "" {
		logger.Debug("Package %s found in receipts (no version check)", packageID)
		return true, nil // Exists, consider satisfied when no version required
	}

	installedVersion, err := extractVersionFromPkgInfo(outputStr)
	if err != nil {
		logger.Debug("Could not extract version from package receipt: %v", err)
		return false, nil // Can't parse -> don't skip, let install attempt proceed
	}

	cmp := LooseVersionCompare(installedVersion, version)
	if cmp >= 0 {
		logger.Debug("Package %s installed version %s >= required %s (loose)", packageID, installedVersion, version)
		return true, nil
	}
	logger.Debug("Package %s installed version %s < required %s", packageID, installedVersion, version)
	return false, nil
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
