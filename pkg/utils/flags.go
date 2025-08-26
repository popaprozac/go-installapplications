package utils

import (
	"fmt"
	"strings"
)

// NormalizeBooleanFlags rewrites args so that "--flag false" becomes "--flag=false" for known boolean flags.
// This improves UX with Go's flag package which interprets bare boolean flags as true when present.
//
// Pass os.Args and a set of boolean flag names. The returned slice should be assigned back to os.Args.
func NormalizeBooleanFlags(args []string, booleanFlags map[string]struct{}) []string {
	if len(args) <= 2 {
		return args
	}

	normalized := make([]string, 0, len(args))
	normalized = append(normalized, args[0])

	i := 1
	for i < len(args) {
		current := args[i]
		// Stop normalizing after end-of-flags terminator
		if current == "--" {
			normalized = append(normalized, args[i:]...)
			break
		}

		// Match -flag or --flag forms without an equals sign
		if strings.HasPrefix(current, "-") && !strings.Contains(current, "=") {
			// Capture original dash prefix length for a nicer rewrite
			dashPrefix := "-"
			if strings.HasPrefix(current, "--") {
				dashPrefix = "--"
			}
			name := strings.TrimLeft(current, "-")
			if _, ok := booleanFlags[name]; ok && i+1 < len(args) {
				next := strings.ToLower(args[i+1])
				if next == "true" || next == "false" {
					normalized = append(normalized, fmt.Sprintf("%s%s=%s", dashPrefix, name, next))
					i += 2
					continue
				}
			}
		}

		normalized = append(normalized, current)
		i++
	}

	return normalized
}

// MultiValueHeader implements flag.Value to collect repeated --log-header Name=Value entries.
type MultiValueHeader struct {
	Headers map[string]string
}

func (m *MultiValueHeader) String() string { return "" }

func (m *MultiValueHeader) Set(val string) error {
	if m.Headers == nil {
		m.Headers = map[string]string{}
	}
	// split on first '='
	for i := 0; i < len(val); i++ {
		if val[i] == '=' {
			name := val[:i]
			value := ""
			if i+1 < len(val) {
				value = val[i+1:]
			}
			if name != "" {
				m.Headers[name] = value
			}
			return nil
		}
	}
	// no '=' present; treat whole as name with empty value
	if val != "" {
		m.Headers[val] = ""
	}
	return nil
}
