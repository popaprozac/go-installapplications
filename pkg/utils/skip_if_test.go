package utils

import "testing"

// ShouldSkipItem is the gate the manager and daemon use to honor skip_if. Verify
// each accepted alias plus unknown strings and empty input.
func TestShouldSkipItem_AllVariants(t *testing.T) {
	logger := NewLogger(false, false)

	if ShouldSkipItem("", logger) {
		t.Fatalf("empty skip_if must never skip")
	}
	if ShouldSkipItem("unknown-arch", logger) {
		t.Fatalf("unknown criteria must not skip (we don't fail closed)")
	}

	// Architecture-specific results depend on runtime; we can only assert
	// consistency: arm64 ≡ apple_silicon, x86_64 ≡ intel.
	if got := ShouldSkipItem("arm64", logger); got != ShouldSkipItem("apple_silicon", logger) {
		t.Errorf("arm64 and apple_silicon should produce the same skip decision")
	}
	if got := ShouldSkipItem("x86_64", logger); got != ShouldSkipItem("intel", logger) {
		t.Errorf("x86_64 and intel should produce the same skip decision")
	}

	// Case insensitivity is part of the contract
	if ShouldSkipItem("INTEL", logger) != ShouldSkipItem("intel", logger) {
		t.Errorf("skip_if should be case-insensitive")
	}

	// At least one of (apple_silicon, intel) must be true on a real Mac;
	// confirm we never claim both at once.
	if IsAppleSilicon() && IsIntel() {
		t.Errorf("system cannot be both apple_silicon and intel")
	}
}
