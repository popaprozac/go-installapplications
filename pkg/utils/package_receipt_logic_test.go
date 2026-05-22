package utils

import "testing"

// The receipt check requires a packageID. Without one, CheckPackageReceipt must
// report "not satisfied" so the caller proceeds with the install.
func TestCheckPackageReceipt_NoPackageID(t *testing.T) {
	logger := NewLogger(false, false)
	got, err := CheckPackageReceipt("", "1.0", logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Fatalf("expected false when packageID is empty (caller should install)")
	}
}

// A nonexistent packageID must report "not installed".
func TestCheckPackageReceipt_NonexistentPackage(t *testing.T) {
	logger := NewLogger(false, false)
	got, err := CheckPackageReceipt("com.example.this.package.does.not.exist", "1.0", logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Fatalf("nonexistent packageID should report not satisfied")
	}
}

// extractVersionFromPkgInfo parses pkgutil --pkg-info output. Cover the success
// case plus malformed inputs.
func TestExtractVersionFromPkgInfo(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "well-formed",
			input: "package-id: com.example.demo\nversion: 4.32.0\nvolume: /\n",
			want:  "4.32.0",
		},
		{
			name:  "version with leading spaces",
			input: "version:    1.0\n",
			want:  "1.0",
		},
		{
			name:    "missing version",
			input:   "package-id: com.example.demo\n",
			wantErr: true,
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractVersionFromPkgInfo(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// LooseVersionCompare is the version-comparison primitive shared with the
// receipt check. Add edge cases beyond the basic table in package_receipts_test.go.
//
// Behaviour vs. Python's distutils.LooseVersion: the Go implementation treats
// non-numeric segments as 0 (so "1.0b1" decomposes to [1,0,0,1]), whereas
// Python compares non-numeric segments as strings. For typical macOS package
// versions (`1.2.3`, `10.6.0`) the result is identical; pre-release suffixes
// diverge. The cases below pin our actual semantics.
func TestLooseVersionCompare_EdgeCases(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		// Trailing zero segments are equal — matches Python's LooseVersion.
		{"3.0", "3.0.0.0", 0},
		// Larger numeric segment beats more segments overall.
		{"3.1", "3.0.99", 1},
		// Pre-release suffix decomposes to [1,0,0,1] > [1,0,0] (Python: < because
		// Python compares "b" as a string). Documented divergence; pin the Go
		// behavior so a future implementation change is intentional.
		{"1.0b1", "1.0.0", 1},
		// Multi-digit comparison is numeric, not lexicographic.
		{"1.20", "1.3", 1},
	}
	for _, tc := range cases {
		got := LooseVersionCompare(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("LooseVersionCompare(%q,%q)=%d want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
