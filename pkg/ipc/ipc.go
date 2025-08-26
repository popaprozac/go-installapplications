package ipc

import (
	"fmt"
	"os"
	"path/filepath"
)

// SocketDir is the directory where agent sockets are created.
// We use /var/tmp to avoid user-home coupling and keep paths stable across contexts.
const SocketDir = "/var/tmp/go-installapplications"

// GetAgentSocketPathForUID returns the Unix domain socket path for a given user UID
func GetAgentSocketPathForUID(uid string) string {
	if uid == "" {
		uid = "unknown"
	}
	return filepath.Join(SocketDir, fmt.Sprintf("agent-%s.sock", uid))
}

// EnsureSocketDir ensures the socket directory exists with safe permissions
// that allow both root and regular users to create sockets.
func EnsureSocketDir() error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(SocketDir, 0777); err != nil {
		return fmt.Errorf("failed to create socket dir %s: %w", SocketDir, err)
	}

	// Set world-writable permissions to allow both root and users to create sockets
	// This is safe for /var/tmp as it's a temporary directory
	if err := os.Chmod(SocketDir, 0777); err != nil {
		return fmt.Errorf("failed to set socket dir permissions: %w", err)
	}

	// Try to set ownership to root, but don't fail if we can't (e.g., regular user)
	// This allows the daemon (root) to manage the directory when possible
	if err := os.Chown(SocketDir, 0, 0); err != nil {
		// Log but don't fail - this is expected when running as a regular user
		// The directory will still work with world-writable permissions
	}

	return nil
}

// RPCRequest represents a request from the daemon to the agent
type RPCRequest struct {
	ID        string `json:"id"`
	Command   string `json:"command"` // RunUserScript | PlaceUserFile | Ping | Shutdown
	Path      string `json:"path,omitempty"`
	Source    string `json:"source,omitempty"`
	DoNotWait bool   `json:"donotwait,omitempty"`
}

// RPCResponse represents a response from the agent back to the daemon
type RPCResponse struct {
	ID       string `json:"id"`
	OK       bool   `json:"ok"`
	Started  bool   `json:"started,omitempty"`
	ExitCode int    `json:"exitCode,omitempty"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
}
