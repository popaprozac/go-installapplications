package config

import (
	"testing"
	"time"
)

// applySettingsMap is the entry point used by both shared and mode-specific
// mobileconfig sections. Cover every supported key with an explicit assertion
// so adding new keys doesn't silently regress existing ones.
func TestApplySettingsMap_AllKeys(t *testing.T) {
	cfg := NewConfig()
	settings := map[string]interface{}{
		"JSONURL":                  "https://server.example/bootstrap.json",
		"InstallPath":              "/Library/custom-iapath",
		"Debug":                    true,
		"Verbose":                  true,
		"Reboot":                   true,
		"MaxRetries":               int64(7),
		"RetryDelay":               int64(11),
		"CleanupOnFailure":         false,
		"CleanupOnSuccess":         false,
		"KeepFailedFiles":          true,
		"DryRun":                   true,
		"TrackBackgroundProcesses": true,
		"BackgroundTimeout":        int64(120),
		"DownloadMaxConcurrency":   int64(8),
		"WaitForAgentTimeout":      int64(3600),
		"AgentRequestTimeout":      int64(900),
		"HTTPAuthUser":             "alice",
		"HTTPAuthPassword":         "s3cret",
		"FollowRedirects":          true,
		"SkipValidation":           true,
		"LaunchAgentIdentifier":    "com.example.agent",
		"LaunchDaemonIdentifier":   "com.example.daemon",
		"LogFilePath":              "/var/log/example.log",
		"RetainLogFiles":           true,
		"WithPreflight":            true,
		"NoRestartOnError":         true,
	}
	if err := cfg.applySettingsMap(settings); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if cfg.JSONURL != "https://server.example/bootstrap.json" ||
		cfg.InstallPath != "/Library/custom-iapath" ||
		!cfg.Debug || !cfg.Verbose || !cfg.Reboot ||
		cfg.MaxRetries != 7 || cfg.RetryDelay != 11 ||
		cfg.CleanupOnFailure || cfg.CleanupOnSuccess ||
		!cfg.KeepFailedFiles || !cfg.DryRun || !cfg.TrackBackgroundProcesses ||
		cfg.BackgroundTimeout != 120*time.Second ||
		cfg.DownloadMaxConcurrency != 8 ||
		cfg.WaitForAgentTimeout != 3600*time.Second ||
		cfg.AgentRequestTimeout != 900*time.Second ||
		cfg.HTTPAuthUser != "alice" || cfg.HTTPAuthPassword != "s3cret" ||
		!cfg.FollowRedirects || !cfg.SkipValidation ||
		cfg.LaunchAgentIdentifier != "com.example.agent" ||
		cfg.LaunchDaemonIdentifier != "com.example.daemon" ||
		cfg.LogFilePath != "/var/log/example.log" ||
		!cfg.RetainLogFiles || !cfg.WithPreflight || !cfg.NoRestartOnError {
		t.Fatalf("settings not fully applied: %+v", cfg)
	}
}

func TestApplySettingsMap_RebootStringForm(t *testing.T) {
	cases := map[string]bool{
		"true":  true,
		"True":  true,
		"false": false,
	}
	for in, want := range cases {
		cfg := NewConfig()
		if err := cfg.applySettingsMap(map[string]interface{}{"Reboot": in}); err != nil {
			t.Fatalf("apply: %v", err)
		}
		if cfg.Reboot != want {
			t.Fatalf("Reboot=%q -> %v, want %v", in, cfg.Reboot, want)
		}
	}
}

func TestApplySettingsMap_RejectsEmptyJSONURL(t *testing.T) {
	cfg := NewConfig()
	err := cfg.applySettingsMap(map[string]interface{}{"JSONURL": ""})
	if err == nil {
		t.Fatalf("empty-string JSONURL should be rejected (omit the key instead)")
	}
}

func TestApplySettingsMap_HTTPHeadersArrayForm(t *testing.T) {
	cfg := NewConfig()
	settings := map[string]interface{}{
		"HTTPHeaders": []interface{}{
			map[string]interface{}{"name": "X-API-Key", "value": "abc123"},
			map[string]interface{}{"name": "User-Agent", "value": "test"},
		},
	}
	if err := cfg.applySettingsMap(settings); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if cfg.HTTPHeaders["X-API-Key"] != "abc123" || cfg.HTTPHeaders["User-Agent"] != "test" {
		t.Fatalf("array-form headers not applied: %+v", cfg.HTTPHeaders)
	}
}

func TestRedactedForLogging_MasksSecrets(t *testing.T) {
	cfg := NewConfig()
	cfg.HTTPAuthPassword = "real-password"
	cfg.HeaderAuthorization = "Bearer real-token"
	cfg.HTTPHeaders = map[string]string{"X-API-Key": "real-key"}

	snap := cfg.RedactedForLogging()
	if snap["HTTPAuthPassword"] == "real-password" {
		t.Fatalf("HTTPAuthPassword leaked: %v", snap["HTTPAuthPassword"])
	}
	if snap["HeaderAuthorization"] == "Bearer real-token" {
		t.Fatalf("HeaderAuthorization leaked: %v", snap["HeaderAuthorization"])
	}
	headers, ok := snap["HTTPHeaders"].(map[string]string)
	if !ok {
		t.Fatalf("HTTPHeaders type %T", snap["HTTPHeaders"])
	}
	if headers["X-API-Key"] == "real-key" {
		t.Fatalf("X-API-Key leaked")
	}
}
