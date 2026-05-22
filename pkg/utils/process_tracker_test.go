package utils

import (
	"os/exec"
	"testing"
	"time"
)

// Two short-lived processes should both complete cleanly with no errors.
func TestProcessTracker_HappyPath(t *testing.T) {
	pt := NewProcessTracker(NewLogger(false, false))

	for i := 0; i < 2; i++ {
		cmd := exec.Command("/bin/sh", "-c", "exit 0")
		if err := pt.StartBackgroundProcess(cmd, "noop"); err != nil {
			t.Fatalf("start: %v", err)
		}
	}
	if got := pt.GetActiveCount(); got != 2 {
		t.Fatalf("expected 2 tracked, got %d", got)
	}

	if errs := pt.WaitForCompletion(5 * time.Second); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if got := pt.GetActiveCount(); got != 0 {
		t.Fatalf("tracker should be cleared after wait, got %d", got)
	}
}

// A non-zero exit propagates as an error from WaitForCompletion.
func TestProcessTracker_PropagatesFailure(t *testing.T) {
	pt := NewProcessTracker(NewLogger(false, false))
	cmd := exec.Command("/bin/sh", "-c", "exit 7")
	if err := pt.StartBackgroundProcess(cmd, "failing"); err != nil {
		t.Fatalf("start: %v", err)
	}
	errs := pt.WaitForCompletion(5 * time.Second)
	if len(errs) != 1 {
		t.Fatalf("expected one error, got %d: %v", len(errs), errs)
	}
}

// A process that outlives the timeout is killed and reported as a timeout.
func TestProcessTracker_KillsOnTimeout(t *testing.T) {
	pt := NewProcessTracker(NewLogger(false, false))
	cmd := exec.Command("/bin/sh", "-c", "sleep 30")
	if err := pt.StartBackgroundProcess(cmd, "sleep"); err != nil {
		t.Fatalf("start: %v", err)
	}
	start := time.Now()
	errs := pt.WaitForCompletion(200 * time.Millisecond)
	if len(errs) == 0 {
		t.Fatalf("expected timeout error")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("timeout enforcement took too long")
	}
	if got := pt.GetActiveCount(); got != 0 {
		t.Fatalf("tracker should be cleared after timeout, got %d", got)
	}
}

// GetActiveCount on an empty tracker returns 0 without locking issues.
func TestProcessTracker_EmptyTracker(t *testing.T) {
	pt := NewProcessTracker(NewLogger(false, false))
	if pt.GetActiveCount() != 0 {
		t.Fatalf("expected 0")
	}
	if errs := pt.WaitForCompletion(time.Second); errs != nil {
		t.Fatalf("expected nil errors, got %v", errs)
	}
}
