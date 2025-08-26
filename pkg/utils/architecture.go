package utils

import (
	"os/exec"
	"runtime"
	"strings"
)

// GetArchitecture returns the current system architecture
func GetArchitecture() string {
	return runtime.GOARCH
}

// IsAppleSilicon returns true if we're running on Apple Silicon
func IsAppleSilicon() bool {
	if runtime.GOOS != "darwin" {
		return false
	}

	arch := runtime.GOARCH
	if arch == "arm64" {
		return true
	}

	if arch == "amd64" {
		// Check if we're running under Rosetta on Apple Silicon
		// Use sysctl to check if we're on Apple Silicon hardware
		cmd := exec.Command("sysctl", "-n", "machdep.cpu.brand_string")
		output, err := cmd.Output()
		if err != nil {
			// Fallback: check uname for ARM64 in version
			cmd = exec.Command("uname", "-v")
			if versionOutput, err := cmd.Output(); err == nil {
				return strings.Contains(string(versionOutput), "ARM64")
			}
			return false
		}

		// Look for Apple Silicon indicators in CPU brand
		brandString := strings.ToLower(string(output))
		return strings.Contains(brandString, "apple")
	}

	return false
}

// IsIntel returns true if we're running natively on Intel
func IsIntel() bool {
	if runtime.GOOS != "darwin" {
		return runtime.GOARCH == "amd64"
	}

	return runtime.GOARCH == "amd64" && !IsAppleSilicon()
}

// ShouldSkipItem checks if an item should be skipped based on skip_if criteria
func ShouldSkipItem(skipIf string, logger *Logger) bool {
	if skipIf == "" {
		return false // No skip criteria, don't skip
	}

	skipIf = strings.ToLower(skipIf)
	logger.Debug("Checking skip_if criteria: %s", skipIf)

	var shouldSkip bool
	switch skipIf {
	case "arm64", "apple_silicon":
		shouldSkip = IsAppleSilicon()
		logger.Debug("Is Apple Silicon: %t", shouldSkip)
	case "x86_64", "intel":
		shouldSkip = IsIntel()
		logger.Debug("Is Intel: %t", shouldSkip)
	default:
		logger.Debug("Unknown skip_if criteria '%s', not skipping", skipIf)
		return false
	}

	if shouldSkip {
		logger.Debug("Item should be skipped based on architecture")
	}

	return shouldSkip
}

// GetArchitectureInfo returns human-readable architecture information
func GetArchitectureInfo() string {
	arch := runtime.GOARCH
	if IsAppleSilicon() {
		return "Apple Silicon (arm64)"
	}
	if IsIntel() {
		return "Intel (amd64)"
	}
	return arch
}
