package download

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-installapplications/pkg/utils"
)

func tempFileWithContents(t *testing.T, body []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "blob")
	if err := os.WriteFile(p, body, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum[:])
}

func TestParseHashCheckPolicy(t *testing.T) {
	cases := map[string]HashCheckPolicy{
		"":         HashCheckWarning,
		"Strict":   HashCheckStrict,
		"strict":   HashCheckStrict,
		"STRICT":   HashCheckStrict,
		"Warning":  HashCheckWarning,
		"warning":  HashCheckWarning,
		"Ignore":   HashCheckIgnore,
		"ignore":   HashCheckIgnore,
		"  Strict": HashCheckStrict, // trimmed
		"garbage":  HashCheckWarning, // unknown defaults to safe
	}
	for in, want := range cases {
		got := ParseHashCheckPolicy(in)
		if got != want {
			t.Errorf("Parse(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestHashPolicyString(t *testing.T) {
	if HashCheckStrict.String() != "Strict" {
		t.Fatalf("Strict")
	}
	if HashCheckWarning.String() != "Warning" {
		t.Fatalf("Warning")
	}
	if HashCheckIgnore.String() != "Ignore" {
		t.Fatalf("Ignore")
	}
}

func TestVerifyFileHash_StrictRejectsMissingHash(t *testing.T) {
	c := NewClient(utils.NewLogger(false, false))
	c.SetHashCheckPolicy(HashCheckStrict)
	p := tempFileWithContents(t, []byte("hello"))
	err := c.VerifyFileHash(p, "")
	if err == nil || !strings.Contains(err.Error(), "Strict") {
		t.Fatalf("Strict should reject missing hash, got %v", err)
	}
}

func TestVerifyFileHash_WarningAcceptsMissingHash(t *testing.T) {
	c := NewClient(utils.NewLogger(false, false))
	// Warning is the default; explicit set for clarity.
	c.SetHashCheckPolicy(HashCheckWarning)
	p := tempFileWithContents(t, []byte("hello"))
	if err := c.VerifyFileHash(p, ""); err != nil {
		t.Fatalf("Warning should accept missing hash, got %v", err)
	}
}

func TestVerifyFileHash_IgnoreAcceptsMismatch(t *testing.T) {
	c := NewClient(utils.NewLogger(false, false))
	c.SetHashCheckPolicy(HashCheckIgnore)
	p := tempFileWithContents(t, []byte("hello"))
	if err := c.VerifyFileHash(p, "deadbeef"); err != nil {
		t.Fatalf("Ignore should accept hash mismatch, got %v", err)
	}
}

func TestVerifyFileHash_StrictAndWarningRejectMismatch(t *testing.T) {
	for _, p := range []HashCheckPolicy{HashCheckStrict, HashCheckWarning} {
		t.Run(p.String(), func(t *testing.T) {
			c := NewClient(utils.NewLogger(false, false))
			c.SetHashCheckPolicy(p)
			path := tempFileWithContents(t, []byte("hello"))
			if err := c.VerifyFileHash(path, "deadbeef"); err == nil {
				t.Fatalf("policy %s should reject mismatch", p)
			}
		})
	}
}

func TestVerifyFileHash_AllPoliciesAcceptCorrectHash(t *testing.T) {
	body := []byte("hash-me")
	want := sha256hex(body)
	for _, p := range []HashCheckPolicy{HashCheckStrict, HashCheckWarning, HashCheckIgnore} {
		t.Run(p.String(), func(t *testing.T) {
			c := NewClient(utils.NewLogger(false, false))
			c.SetHashCheckPolicy(p)
			path := tempFileWithContents(t, body)
			if err := c.VerifyFileHash(path, want); err != nil {
				t.Fatalf("policy %s should accept matching hash, got %v", p, err)
			}
		})
	}
}
