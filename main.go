package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-installapplications/pkg/config"
	"github.com/go-installapplications/pkg/mode"
	"github.com/go-installapplications/pkg/retry"
	"github.com/go-installapplications/pkg/utils"
)

func main() {
	// Normalize boolean flags so forms like "--reboot false" are treated as "--reboot=false"
	os.Args = utils.NormalizeBooleanFlags(os.Args, map[string]struct{}{
		"debug":                      {},
		"verbose":                    {},
		"reboot":                     {},
		"cleanup-on-failure":         {},
		"keep-failed-files":          {},
		"dry-run":                    {},
		"track-background-processes": {},
		"reset-retries":              {},
	})

	// Create a new config with defaults
	cfg := config.NewConfig()

	// Define command-line flags - use empty/false defaults so we can detect if they were set
	jsonURL := flag.String("jsonurl", "", "URL to bootstrap JSON file")
	installPath := flag.String("installpath", "", "Installation path (default: /Library/go-installapplications)")
	compat := flag.Bool("compat", false, "Use original InstallApplications layout for internal paths (/Library/installapplications). Mutually exclusive with --installpath")
	debug := flag.Bool("debug", false, "Enable debug logging (default: false)")
	verbose := flag.Bool("verbose", false, "Enable verbose logging (default: false)")
	reboot := flag.Bool("reboot", false, "Reboot after completion (default: false)")

	maxRetries := flag.Int("max-retries", 3, "Maximum number of retries for failed installs")
	retryDelay := flag.Int("retry-delay", 5, "Delay between retries in seconds")

	cleanupOnFailure := flag.Bool("cleanup-on-failure", false, "Cleanup on failure (default: true, set to false to disable)")
	cleanupOnSuccess := flag.Bool("cleanup-on-success", false, "Cleanup on success (default: true, set to false to disable)")
	keepFailedFiles := flag.Bool("keep-failed-files", false, "Keep failed files (default: false, set to true to keep)")

	dryRun := flag.Bool("dry-run", false, "Dry run - don't actually install anything (default: false)")

	trackBgProcesses := flag.Bool("track-background-processes", false, "Track and wait for background processes (default: false, set to true to enable)")
	backgroundTimeout := flag.Int("background-timeout", 300, "Timeout for background processes in seconds")

	modeFlag := flag.String("mode", "", "Operating mode: daemon, agent, standalone (default: standalone)")
	resetRetries := flag.Bool("reset-retries", false, "Clear retry state before running (useful for testing)")
	profileDomain := flag.String("profile-domain", config.DefaultProfileDomain, "macOS preference domain to read from")

	// Download and IPC settings
	downloadMaxConcurrency := flag.Int("download-max-concurrency", 4, "Maximum concurrent downloads")
	waitForAgentTimeout := flag.Int("wait-for-agent-timeout", 86400, "How long daemon waits for agent socket (seconds)")
	agentRequestTimeout := flag.Int("agent-request-timeout", 7200, "Timeout per agent RPC request (seconds)")

	// Backwards-compat flags matching original InstallApplications
	followRedirects := flag.Bool("follow-redirects", false, "Follow HTTP redirects (default: false)")
	headersAuth := flag.String("headers", "", "Authorization header value (e.g., 'Basic xxx' or 'Bearer yyy')")
	laIdentifier := flag.String("laidentifier", "", "LaunchAgent identifier")
	ldIdentifier := flag.String("ldidentifier", "", "LaunchDaemon identifier")
	skipValidation := flag.Bool("skip-validation", false, "Skip bootstrap.json validation")

	// HTTP Authentication (mobile config only, but CLI for testing)
	httpAuthUser := flag.String("http-auth-user", "", "HTTP Basic Auth username")
	httpAuthPassword := flag.String("http-auth-password", "", "HTTP Basic Auth password")

	// Remote logging NOT YET IMPLEMENTED
	// logDestination := flag.String("log-destination", "", "Remote log destination URL (optional)")
	// logProvider := flag.String("log-provider", "", "Remote log provider: generic|datadog (optional)")
	// var logHeaders utils.MultiValueHeader
	// flag.Var(&logHeaders, "log-header", "Header for remote logs in Name=Value form (repeatable)")
	logFilePath := flag.String("log-file", "", "Force logs to also go to this file (in addition to console)")

	retainLogFiles := flag.Bool("retain-log-files", false, "Retain log files from previous runs (default: false, set to true to retain)")

	// Parse the command-line arguments
	flag.Parse()

	// Handle retry reset first if requested
	if *resetRetries {
		if err := retry.ClearRetryCount(); err != nil {
			fmt.Printf("Warning: failed to clear retry state: %v\n", err)
		} else {
			fmt.Printf("Retry state cleared\n")
		}
	}

	// Apply mode from command line early if provided, otherwise use default
	if *modeFlag != "" {
		cfg.Mode = *modeFlag
	}

	// Try to read from mobile config with graceful fallback
	var profileResult *config.ProfileResult
	if result, err := cfg.ReadFromProfile(*profileDomain); err != nil {
		// Mobile config reading failed - log and continue with defaults
		profileResult = &config.ProfileResult{ConfigFound: false, BootstrapSource: "none"}
		fmt.Printf("Warning: mobile config reading failed (continuing with defaults): %v\n", err)
	} else {
		profileResult = result
	}

	// Create a map to track which flags were explicitly set
	flagsSet := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		flagsSet[f.Name] = true
	})

	// Only override mobile config with command line flags that were explicitly set
	if flagsSet["jsonurl"] {
		// Command-line should take precedence over embedded profile settings
		if profileResult.BootstrapSource == "embedded" {
			fmt.Printf("Warning: --jsonurl overrides embedded bootstrap section from mobile config\n")
		}
		cfg.JSONURL = *jsonURL
	}
	// Handle compatibility and install path
	if flagsSet["compat"] && flagsSet["installpath"] {
		fmt.Println("Error: --compat cannot be used together with --installpath; choose one")
		os.Exit(1)
	}
	if flagsSet["compat"] && *compat {
		cfg.InstallPath = "/Library/installapplications"
	} else if flagsSet["installpath"] {
		cfg.InstallPath = *installPath
	}
	if flagsSet["debug"] {
		cfg.Debug = *debug
	}
	if flagsSet["verbose"] {
		cfg.Verbose = *verbose
	}
	if flagsSet["log-file"] {
		cfg.LogFilePath = *logFilePath
	}
	if flagsSet["reboot"] {
		cfg.Reboot = *reboot
	}
	if flagsSet["max-retries"] {
		cfg.MaxRetries = *maxRetries
	}
	if flagsSet["retry-delay"] {
		cfg.RetryDelay = *retryDelay
	}
	// Compat flags
	if flagsSet["follow-redirects"] {
		cfg.FollowRedirects = *followRedirects
	}
	if flagsSet["headers"] {
		cfg.HeaderAuthorization = *headersAuth
		if cfg.HTTPHeaders == nil {
			cfg.HTTPHeaders = map[string]string{}
		}
		if *headersAuth != "" {
			cfg.HTTPHeaders["Authorization"] = *headersAuth
		}
	}
	if flagsSet["laidentifier"] {
		cfg.LaunchAgentIdentifier = *laIdentifier
	}
	if flagsSet["ldidentifier"] {
		cfg.LaunchDaemonIdentifier = *ldIdentifier
	}
	if flagsSet["skip-validation"] {
		cfg.SkipValidation = *skipValidation
	}
	if flagsSet["cleanup-on-failure"] {
		cfg.CleanupOnFailure = *cleanupOnFailure
	}
	if flagsSet["cleanup-on-success"] {
		cfg.CleanupOnSuccess = *cleanupOnSuccess
	}
	if flagsSet["keep-failed-files"] {
		cfg.KeepFailedFiles = *keepFailedFiles
	}
	if flagsSet["dry-run"] {
		cfg.DryRun = *dryRun
	}
	if flagsSet["track-background-processes"] {
		cfg.TrackBackgroundProcesses = *trackBgProcesses
	}
	if flagsSet["background-timeout"] {
		cfg.BackgroundTimeout = time.Duration(*backgroundTimeout) * time.Second
	}
	if flagsSet["retain-log-files"] {
		cfg.RetainLogFiles = *retainLogFiles
	}

	// Download and IPC settings
	if flagsSet["download-max-concurrency"] {
		cfg.DownloadMaxConcurrency = *downloadMaxConcurrency
	}
	if flagsSet["wait-for-agent-timeout"] {
		cfg.WaitForAgentTimeout = time.Duration(*waitForAgentTimeout) * time.Second
	}
	if flagsSet["agent-request-timeout"] {
		cfg.AgentRequestTimeout = time.Duration(*agentRequestTimeout) * time.Second
	}

	// HTTP Authentication
	if flagsSet["http-auth-user"] {
		cfg.HTTPAuthUser = *httpAuthUser
	}
	if flagsSet["http-auth-password"] {
		cfg.HTTPAuthPassword = *httpAuthPassword
	}

	// Create logger (with file logging for standalone mode)
	var logger *utils.Logger
	var err error

	if cfg.Mode == "standalone" {
		// Standalone mode
		var logFilePath string
		if cfg.LogFilePath != "" {
			logFilePath = cfg.LogFilePath
		} else {
			logFilePath = cfg.DefaultStandaloneLogPath
		}

		if !cfg.RetainLogFiles {
			if err := os.Remove(logFilePath); err != nil && !os.IsNotExist(err) {
				fmt.Printf("Warning: Failed to delete standalone log file: %v\n", err)
			}
		}

		logger, err = utils.NewLoggerWithFile(cfg.Debug, cfg.Verbose, logFilePath)
		if err != nil {
			fmt.Printf("Warning: Failed to create file logger: %v\nUsing console-only logging\n", err)
			logger = utils.NewLogger(cfg.Debug, cfg.Verbose)
		} else {
			mode := "appending"
			if !cfg.RetainLogFiles {
				mode = "fresh"
			}
			fmt.Printf("Logging to: %s (%s)\n", logFilePath, mode)
		}
	} else {
		// Daemon/agent modes: use console logging by default; optionally tee to a file
		if cfg.LogFilePath != "" {
			if !cfg.RetainLogFiles {
				if err := os.Remove(cfg.LogFilePath); err != nil && !os.IsNotExist(err) {
					fmt.Printf("Warning: Failed to delete log file: %v\n", err)
				}
			}

			if cfg.Mode == "daemon" {
				dir := filepath.Dir(cfg.LogFilePath)
				if err := utils.EnsureDir(dir); err == nil {
					_ = os.Chmod(dir, 0o1777)
				}
			}

			logger, err = utils.NewLoggerWithFile(cfg.Debug, cfg.Verbose, cfg.LogFilePath)
			if err != nil {
				fmt.Printf("Warning: Failed to create file logger: %v\nUsing console-only logging\n", err)
				logger = utils.NewLogger(cfg.Debug, cfg.Verbose)
			} else {
				mode := "appending"
				if !cfg.RetainLogFiles {
					mode = "fresh"
				}
				fmt.Printf("Logging to: %s (and console, %s)\n", cfg.LogFilePath, mode)
			}
		} else {
			logger = utils.NewLogger(cfg.Debug, cfg.Verbose)
		}
	}

	// Remote log shipping temporarily disabled for initial release
	// if cfg.LogDestination != "" {
	// 	// Default provider to generic if unspecified but destination is set
	// 	provider := cfg.LogProvider
	// 	if provider == "" {
	// 		provider = "generic"
	// 	}
	// 	logger.EnableRemoteShipping(cfg.LogDestination, cfg.LogHeaders, provider)
	// }

	// Log configuration source with details
	if profileResult.ConfigFound {
		logger.Info("Starting go-installapplications in %s mode (mobile config found)", cfg.Mode)
		logger.Debug("Profile domain: %s", *profileDomain)
		logger.Debug("Bootstrap source: %s", profileResult.BootstrapSource)
		logger.Debug("Config hierarchy: defaults → shared → %s → command line", cfg.Mode)

		// Log which command line flags were explicitly set
		if len(flagsSet) > 0 {
			var setFlags []string
			for flagName := range flagsSet {
				if flagName != "profile-domain" { // Don't clutter with internal flags
					setFlags = append(setFlags, flagName)
				}
			}
			if len(setFlags) > 0 {
				logger.Debug("Command line overrides: %v", setFlags)
			}
		}
	} else {
		logger.Info("Starting go-installapplications in %s mode (using defaults + command line)", cfg.Mode)
		logger.Debug("No mobile config found at domain: %s", *profileDomain)
	}

	logger.Debug("System architecture: %s", utils.GetArchitectureInfo())
	if cfg.TrackBackgroundProcesses {
		logger.Debug("Background process tracking enabled (timeout: %v)", cfg.BackgroundTimeout)
	} else {
		logger.Debug("Background process tracking disabled")
	}

	// Log full final configuration (with sensitive fields redacted)
	if cfg.Debug {
		if b, err := json.MarshalIndent(cfg.RedactedForLogging(), "", "  "); err == nil {
			logger.Debug("Final configuration:\n%s", string(b))
		} else {
			logger.Debug("Final configuration: %v", cfg.RedactedForLogging())
		}
	}

	// Route to appropriate mode handler
	switch cfg.Mode {
	case "daemon":
		mode.RunDaemon(cfg, logger)
	case "agent":
		mode.RunAgent(cfg, logger)
	case "standalone":
		mode.RunStandalone(cfg, logger)
	default:
		logger.Error("Unknown mode: %s", cfg.Mode)
		fmt.Printf("Valid modes: daemon, agent, standalone\n")
		os.Exit(1)
	}
}
