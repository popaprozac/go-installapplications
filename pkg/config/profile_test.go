package config

import "testing"

func TestApplySettingsMap_HeadersAndCompat(t *testing.T) {
	cfg := NewConfig()
	settings := map[string]interface{}{
		"HTTPHeaders":            map[string]interface{}{"X-Test": "v"},
		"HeaderAuthorization":    "Bearer abc",
		"FollowRedirects":        true,
		"SkipValidation":         true,
		"LaunchAgentIdentifier":  "com.example.agent",
		"LaunchDaemonIdentifier": "com.example.daemon",
	}
	if err := cfg.applySettingsMap(settings); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPHeaders["X-Test"] != "v" {
		t.Fatalf("missing header")
	}
	if cfg.HTTPHeaders["Authorization"] != "Bearer abc" {
		t.Fatalf("missing auth header")
	}
	if !cfg.FollowRedirects || !cfg.SkipValidation {
		t.Fatalf("compat flags not set")
	}
	if cfg.LaunchAgentIdentifier != "com.example.agent" || cfg.LaunchDaemonIdentifier != "com.example.daemon" {
		t.Fatalf("identifiers not set")
	}
}
