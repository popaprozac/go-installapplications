package installer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-installapplications/pkg/utils"
)

func writeScript(t *testing.T, contents string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "script.sh")
	if err := os.WriteFile(p, []byte(contents), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

// Cover the shebang-detection branches that drive interpreter labelling.
func TestDetectScriptInterpreter(t *testing.T) {
	se := NewScriptExecutor(false, utils.NewLogger(false, false), false)
	cases := []struct {
		name   string
		script string
		want   string
	}{
		{"bash", "#!/bin/bash\necho hi", "bash"},
		{"sh", "#!/bin/sh\necho hi", "shell"},
		{"python", "#!/usr/bin/env python3\nprint('hi')", "python"},
		{"node", "#!/usr/bin/env node\nconsole.log('hi')", "node.js"},
		{"ruby", "#!/usr/bin/env ruby\nputs 'hi'", "ruby"},
		{"perl", "#!/usr/bin/perl\nprint 'hi'", "perl"},
		{"no shebang defaults to shell", "echo hi", "shell"},
		{"unknown interpreter passes through", "#!/usr/local/bin/lua5.3\nprint()", "lua5.3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeScript(t, tc.script)
			got, err := se.detectScriptInterpreter(path)
			if err != nil {
				t.Fatalf("detect: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ExecuteScript should refuse to run a missing path.
func TestExecuteScript_MissingFile(t *testing.T) {
	se := NewScriptExecutor(false, utils.NewLogger(false, false), false)
	err := se.ExecuteScript("/nonexistent/script.sh", "rootscript", false, false)
	if err == nil {
		t.Fatalf("expected error for missing script")
	}
}

// In dry-run mode no command is executed but no error is returned either.
func TestExecuteScript_DryRunSucceeds(t *testing.T) {
	se := NewScriptExecutor(true, utils.NewLogger(false, false), false)
	if err := se.ExecuteScript("/nonexistent/script.sh", "rootscript", false, false); err != nil {
		t.Fatalf("dry-run should swallow missing-file: %v", err)
	}
}

// PreflightSuccessError is the sentinel daemon/standalone use to detect exit-0.
func TestPreflightSuccessError(t *testing.T) {
	var err error = &PreflightSuccessError{}
	if err.Error() == "" {
		t.Fatalf("Error() should be non-empty")
	}
}
