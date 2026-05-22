package retry

import (
	"os"
	"testing"
)

// retryStateScope swaps the on-disk retry counter location to a per-test temp
// path so concurrent or repeat runs don't observe each other's state.
type retryStateScope struct {
	previous string
}

func newRetryScope(t *testing.T) *retryStateScope {
	t.Helper()
	tmp := t.TempDir()
	scope := &retryStateScope{previous: retryCounterFile}
	retryCounterFile = tmp + "/state"
	t.Cleanup(func() {
		_ = os.RemoveAll(tmp)
		retryCounterFile = scope.previous
	})
	return scope
}

func TestRetryCounter_LifecycleAndShouldRetry(t *testing.T) {
	newRetryScope(t)

	if got := GetRetryCount(); got != 0 {
		t.Fatalf("initial count should be 0, got %d", got)
	}
	if ok, _ := ShouldRetry(); !ok {
		t.Fatalf("ShouldRetry must be true initially")
	}

	for i := 1; i <= MaxRetries; i++ {
		if err := IncrementRetryCount("test"); err != nil {
			t.Fatalf("increment %d: %v", i, err)
		}
		if got := GetRetryCount(); got != i {
			t.Fatalf("after increment %d: count = %d", i, got)
		}
	}

	if ok, err := ShouldRetry(); ok || err == nil {
		t.Fatalf("ShouldRetry must report false once MaxRetries reached: ok=%v err=%v", ok, err)
	}

	if err := ClearRetryCount(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got := GetRetryCount(); got != 0 {
		t.Fatalf("after clear: count = %d", got)
	}
	if ok, _ := ShouldRetry(); !ok {
		t.Fatalf("ShouldRetry must be true after clear")
	}
}

func TestRetryCounter_InfoStrings(t *testing.T) {
	newRetryScope(t)

	if got := GetRetryInfo(); got != "First attempt" {
		t.Fatalf("expected first-attempt string, got %q", got)
	}
	if err := IncrementRetryCount("setup failed"); err != nil {
		t.Fatalf("increment: %v", err)
	}
	info := GetRetryInfo()
	if info == "First attempt" {
		t.Fatalf("after increment, info should not be the first-attempt string: %q", info)
	}
}

func TestRetryCounter_PersistsAcrossReads(t *testing.T) {
	newRetryScope(t)

	if err := IncrementRetryCount("reason A"); err != nil {
		t.Fatalf("increment: %v", err)
	}
	// Simulate a fresh process by re-reading from disk.
	state, err := readRetryState()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if state.Count != 1 || state.Reason != "reason A" {
		t.Fatalf("state did not persist: %+v", state)
	}
}
