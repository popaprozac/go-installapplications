package config

import (
	"encoding/json"
	"testing"
)

// Original Python InstallApplications uses the JSON key "required" for packages.
// installapplications-swiftly uses "pkg_required". Our Item must accept both spellings
// and surface them through the same struct field.
func TestItemUnmarshal_RequiredAlias(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"pkg_required true", `{"name":"a","file":"/tmp/x.pkg","type":"package","pkg_required":true}`, true},
		{"required true (python alias)", `{"name":"a","file":"/tmp/x.pkg","type":"package","required":true}`, true},
		{"neither set", `{"name":"a","file":"/tmp/x.pkg","type":"package"}`, false},
		{"both set true", `{"name":"a","file":"/tmp/x.pkg","type":"package","pkg_required":true,"required":true}`, true},
		{"pkg_required wins when required is false but pkg_required true", `{"name":"a","file":"/tmp/x.pkg","type":"package","pkg_required":true,"required":false}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var it Item
			if err := json.Unmarshal([]byte(tc.body), &it); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if it.PkgRequired != tc.want {
				t.Fatalf("PkgRequired = %v, want %v", it.PkgRequired, tc.want)
			}
		})
	}
}

func TestItemUnmarshal_AllFields(t *testing.T) {
	body := `{
		"name":"Demo",
		"file":"/Library/installapplications/demo.pkg",
		"type":"package",
		"url":"https://example.com/demo.pkg",
		"hash":"abc123",
		"packageid":"com.example.demo",
		"version":"1.2.3",
		"donotwait":true,
		"pkg_required":false,
		"skip_if":"intel",
		"retries":7,
		"retrywait":11,
		"fail_policy":"failable"
	}`
	var it Item
	if err := json.Unmarshal([]byte(body), &it); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if it.Name != "Demo" || it.Type != "package" || it.URL == "" || it.Hash != "abc123" ||
		it.PackageID != "com.example.demo" || it.Version != "1.2.3" || !it.DoNotWait ||
		it.SkipIf != "intel" || it.Retries != 7 || it.RetryWait != 11 || it.FailPolicy != "failable" {
		t.Fatalf("unexpected struct: %+v", it)
	}
}

func TestGetEffectiveFailPolicy(t *testing.T) {
	if got := (&Item{}).GetEffectiveFailPolicy(); got != "failable_execution" {
		t.Fatalf("empty default should be failable_execution, got %q", got)
	}
	if got := (&Item{FailPolicy: "failable"}).GetEffectiveFailPolicy(); got != "failable" {
		t.Fatalf("explicit override should be preserved, got %q", got)
	}
}

func TestShouldStopOnError_Matrix(t *testing.T) {
	cases := []struct {
		policy    string
		operation string
		want      bool
	}{
		// failure_is_not_an_option always stops
		{"failure_is_not_an_option", "script execution", true},
		{"failure_is_not_an_option", "package installation", true},
		{"failure_is_not_an_option", "download", true},
		// failable never stops
		{"failable", "script execution", false},
		{"failable", "package installation", false},
		{"failable", "download", false},
		// failable_execution tolerates script errors only
		{"failable_execution", "script execution", false},
		{"failable_execution", "package installation", true},
		{"failable_execution", "download", true},
		{"failable_execution", "file placement", true},
		// empty defaults to failable_execution
		{"", "script execution", false},
		{"", "package installation", true},
		// unknown policy treated as strict
		{"made-up", "script execution", true},
	}
	for _, tc := range cases {
		it := &Item{FailPolicy: tc.policy}
		if got := it.ShouldStopOnError(tc.operation); got != tc.want {
			t.Errorf("policy=%q op=%q stop=%v want=%v", tc.policy, tc.operation, got, tc.want)
		}
	}
}

func TestValidateBootstrap_SetupAssistantRejectsUserItems(t *testing.T) {
	b := &Bootstrap{
		SetupAssistant: []Item{{Name: "bad", File: "/tmp/x", Type: "userscript"}},
	}
	if err := ValidateBootstrap(b); err == nil {
		t.Fatalf("expected error for userscript in setupassistant")
	}
	b = &Bootstrap{
		SetupAssistant: []Item{{Name: "bad", File: "/tmp/x", Type: "userfile"}},
	}
	if err := ValidateBootstrap(b); err == nil {
		t.Fatalf("expected error for userfile in setupassistant")
	}
}

func TestValidateBootstrap_FailPolicyAccepted(t *testing.T) {
	for _, p := range []string{"failable", "failable_execution", "failure_is_not_an_option", ""} {
		b := &Bootstrap{Userland: []Item{{Name: "x", File: "/tmp/x", Type: "rootscript", FailPolicy: p}}}
		if err := ValidateBootstrap(b); err != nil {
			t.Fatalf("policy %q should be valid: %v", p, err)
		}
	}
	b := &Bootstrap{Userland: []Item{{Name: "x", File: "/tmp/x", Type: "rootscript", FailPolicy: "bogus"}}}
	if err := ValidateBootstrap(b); err == nil {
		t.Fatalf("expected error for invalid fail_policy")
	}
}
