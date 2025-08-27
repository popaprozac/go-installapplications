package config

import (
	"fmt"
	"time"
)

// Config represents the main configuration for go-installapplications
type Config struct {
	JSONURL     string `json:"jsonurl"`
	InstallPath string `json:"install_path"`
	Debug       bool   `json:"debug"`
	Verbose     bool   `json:"verbose"`
	Reboot      bool   `json:"reboot"`

	// Retry settings
	MaxRetries int `json:"max_retries"`
	RetryDelay int `json:"retry_delay"` // seconds

	// Cleanup settings
	CleanupOnFailure bool `json:"cleanup_on_failure"`
	KeepFailedFiles  bool `json:"keep_failed_files"`  // For debugging
	CleanupOnSuccess bool `json:"cleanup_on_success"` // Remove installed artifacts after success (match original)

	// Execution settings
	DryRun bool `json:"dry_run"` // Don't actually install/execute anything

	TrackBackgroundProcesses bool          `json:"track_background_processes"` // New enhancement!
	BackgroundTimeout        time.Duration `json:"background_timeout"`         // How long to wait for background processes
	// Download concurrency
	DownloadMaxConcurrency int `json:"download_max_concurrency"`
	// IPC and coordination
	WaitForAgentTimeout time.Duration `json:"wait_for_agent_timeout"` // How long daemon waits for agent socket
	AgentRequestTimeout time.Duration `json:"agent_request_timeout"`  // How long daemon waits for a single agent RPC

	// HTTP Authentication settings (backwards compatibility with original InstallApplications)
	HTTPAuthUser        string            `json:"http_auth_user,omitempty"`
	HTTPAuthPassword    string            `json:"http_auth_password,omitempty"`
	HTTPHeaders         map[string]string `json:"http_headers,omitempty"`         // Custom headers
	HeaderAuthorization string            `json:"header_authorization,omitempty"` // for --headers convenience

	// Remote log shipping (generic)
	LogDestination string            `json:"log_destination,omitempty"`
	LogProvider    string            `json:"log_provider,omitempty"` // e.g., "generic", "datadog"
	LogHeaders     map[string]string `json:"log_headers,omitempty"`
	LogFilePath    string            `json:"log_file_path,omitempty"` // optional: force logging to this file (also logs to console)

	// Mode settings
	Mode string `json:"mode"` // "daemon", "agent", or "standalone"

	// Backwards-compat flags from original InstallApplications
	FollowRedirects        bool   `json:"follow_redirects"`
	SkipValidation         bool   `json:"skip_validation"`
	LaunchAgentIdentifier  string `json:"launch_agent_identifier"`
	LaunchDaemonIdentifier string `json:"launch_daemon_identifier"`

	RetainLogFiles bool `json:"retain_log_files"` // Retain log files from previous runs

	WithPreflight    bool `json:"with_preflight"`      // Run preflight phase in standalone mode
	NoRestartOnError bool `json:"no_restart_on_error"` // Exit 0 on errors to prevent restart

	// Bootstrap configuration (can be set from top-level or mode-specific sections)
	bootstrapConfig interface{} `json:"-"` // Internal field for bootstrap configuration

	DefaultBootstrapPath string `json:"default_bootstrap_path"`

	DefaultDaemonLogPath     string `json:"default_daemon_log_path"`
	DefaultAgentLogPath      string `json:"default_agent_log_path"`
	DefaultStandaloneLogPath string `json:"default_standalone_log_path"`
}

// NewConfig creates a new Config with defaults
func NewConfig() *Config {
	return &Config{
		JSONURL:                  "",
		InstallPath:              "/Library/go-installapplications",
		Debug:                    false,
		Verbose:                  false,
		Reboot:                   false,
		MaxRetries:               3,
		RetryDelay:               5,
		CleanupOnFailure:         true, // Clean up by default
		CleanupOnSuccess:         true,
		KeepFailedFiles:          false,           // Don't keep corrupted files
		DryRun:                   false,           // Actually run by default
		TrackBackgroundProcesses: false,           // Backward compatible default
		BackgroundTimeout:        time.Minute * 5, // 5 minute timeout for background processes
		DownloadMaxConcurrency:   4,
		WaitForAgentTimeout:      time.Hour * 24, // Wait up to 24h for agent
		AgentRequestTimeout:      time.Hour * 2,  // Per-request timeout
		Mode:                     "standalone",   // Default to standalone for testing

		// Remote log shipping defaults
		LogDestination: "",
		LogProvider:    "", // empty means disabled
		LogHeaders:     map[string]string{},
		LogFilePath:    "",

		// Compatibility defaults
		FollowRedirects:        false,
		SkipValidation:         false,
		LaunchAgentIdentifier:  "com.github.go-installapplications.agent",
		LaunchDaemonIdentifier: "com.github.go-installapplications.daemon",

		RetainLogFiles: false, // Create a new log file for each run

		WithPreflight:    false,
		NoRestartOnError: false,

		DefaultBootstrapPath: "/Library/go-installapplications/bootstrap.json",

		DefaultDaemonLogPath:     "/var/log/go-installapplications/go-installapplications.daemon.log",
		DefaultAgentLogPath:      "/var/log/go-installapplications/go-installapplications.agent.log",
		DefaultStandaloneLogPath: "/var/log/go-installapplications/go-installapplications.standalone.log",
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.JSONURL == "" {
		return fmt.Errorf("JSONURL is required")
	}
	return nil
}

// RedactedForLogging returns a redacted, human-friendly snapshot of the
// effective configuration suitable for debug logs. Sensitive values are masked
// and durations are rendered as strings.
func (c *Config) RedactedForLogging() map[string]interface{} {
	mask := func(s string) string {
		if s == "" {
			return ""
		}
		return "***redacted***"
	}
	maskMap := func(in map[string]string) map[string]string {
		if in == nil {
			return nil
		}
		out := make(map[string]string, len(in))
		for k := range in {
			out[k] = "***redacted***"
		}
		return out
	}

	snapshot := map[string]interface{}{
		// Core
		"Mode":        c.Mode,
		"JSONURL":     c.JSONURL,
		"InstallPath": c.InstallPath,
		// Logging
		"Debug":          c.Debug,
		"Verbose":        c.Verbose,
		"LogDestination": c.LogDestination,
		"LogProvider":    c.LogProvider,
		"LogHeaders":     maskMap(c.LogHeaders),
		"LogFilePath":    c.LogFilePath,
		// Execution
		"Reboot": c.Reboot,
		"DryRun": c.DryRun,
		// Retries
		"MaxRetries": c.MaxRetries,
		"RetryDelay": c.RetryDelay,
		// Cleanup
		"CleanupOnFailure": c.CleanupOnFailure,
		"CleanupOnSuccess": c.CleanupOnSuccess,
		"KeepFailedFiles":  c.KeepFailedFiles,
		// Concurrency & background
		"TrackBackgroundProcesses": c.TrackBackgroundProcesses,
		"BackgroundTimeout":        c.BackgroundTimeout.String(),
		"DownloadMaxConcurrency":   c.DownloadMaxConcurrency,
		// IPC timeouts
		"WaitForAgentTimeout": c.WaitForAgentTimeout.String(),
		"AgentRequestTimeout": c.AgentRequestTimeout.String(),
		// HTTP auth & headers (redacted)
		"HTTPAuthUser":        c.HTTPAuthUser,
		"HTTPAuthPassword":    mask(c.HTTPAuthPassword),
		"HTTPHeaders":         maskMap(c.HTTPHeaders),
		"HeaderAuthorization": mask(c.HeaderAuthorization),
		// Compatibility
		"FollowRedirects":        c.FollowRedirects,
		"SkipValidation":         c.SkipValidation,
		"LaunchAgentIdentifier":  c.LaunchAgentIdentifier,
		"LaunchDaemonIdentifier": c.LaunchDaemonIdentifier,
		// Bootstrap
		"withPreflight": c.WithPreflight,
	}

	return snapshot
}
