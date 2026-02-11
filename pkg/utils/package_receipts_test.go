package utils

import "testing"

func TestLooseVersionCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"10.6", "10.6.0", 0},
		{"10.6.0", "10.6", 0},
		{"1.0", "1.0.0", 0},
		{"2.0", "1.9", 1},
		{"1.9", "2.0", -1},
		{"1.10", "1.9", 1},
		{"1.0.0", "1.0", 0},
		{"", "1.0", -1},
		{"1.0", "", 1},
	}
	for _, tt := range tests {
		got := LooseVersionCompare(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("LooseVersionCompare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
