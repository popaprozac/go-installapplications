package signal

import (
	"os"
)

const (
	SignalDir         = "/var/tmp/go-installapplications"
	UserlandReadyFile = SignalDir + "/.userland-ready"
)

// CreateUserlandReady creates the userland ready signal file (empty touchfile)
func CreateUserlandReady() error {
	// Ensure signal directory exists
	if err := os.MkdirAll(SignalDir, 0777); err != nil {
		return err
	}

	// Create empty signal file (like original InstallApplications)
	file, err := os.Create(UserlandReadyFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// Set permissions so agent can delete it
	return os.Chmod(UserlandReadyFile, 0666)
}

// CheckUserlandReady checks if userland signal file exists
func CheckUserlandReady() bool {
	_, err := os.Stat(UserlandReadyFile)
	return err == nil
}

// RemoveUserlandReady removes the userland ready signal file
func RemoveUserlandReady() error {
	return os.Remove(UserlandReadyFile)
}

// CleanupAllSignals removes signal directory and all files
func CleanupAllSignals() error {
	return os.RemoveAll(SignalDir)
}
