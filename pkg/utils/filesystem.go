package utils

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnsureDir creates a directory and all parent directories if they don't exist
func EnsureDir(dirPath string) error {
	if dirPath == "" {
		return fmt.Errorf("directory path cannot be empty")
	}

	// Check if directory already exists
	if info, err := os.Stat(dirPath); err == nil {
		if info.IsDir() {
			return nil // Directory already exists
		}
		return fmt.Errorf("path %s exists but is not a directory", dirPath)
	}

	// Create directory and all parents
	err := os.MkdirAll(dirPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dirPath, err)
	}

	fmt.Printf("Created directory: %s\n", dirPath)
	return nil
}

// EnsureDirForFile creates the directory needed for a file path
func EnsureDirForFile(filePath string) error {
	dir := filepath.Dir(filePath)
	return EnsureDir(dir)
}
