package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Bootstrap represents the JSON structure for InstallApplications
type Bootstrap struct {
	Preflight      []Item `json:"preflight,omitempty"`
	SetupAssistant []Item `json:"setupassistant,omitempty"`
	Userland       []Item `json:"userland,omitempty"`
}

// Item represents a single installation item (package, script, or file)
type Item struct {
	// Required fields
	File string `json:"file"`
	Name string `json:"name"`
	Type string `json:"type"` // "package", "rootscript", "userscript", "rootfile", "userfile"

	// Download fields
	URL  string `json:"url,omitempty"`
	Hash string `json:"hash,omitempty"`

	// Package specific fields
	PackageID string `json:"packageid,omitempty"`
	Version   string `json:"version,omitempty"`

	// Execution control
	DoNotWait   bool   `json:"donotwait,omitempty"`
	PkgRequired bool   `json:"pkg_required,omitempty"`
	SkipIf      string `json:"skip_if,omitempty"` // "x86_64", "intel", "arm64", "apple_silicon"

	// Retry settings (NEW)
	Retries   int `json:"retries,omitempty"`
	RetryWait int `json:"retrywait,omitempty"`

	// Failure handling policy from Swift version
	FailPolicy string `json:"fail_policy,omitempty"` // "failable", "failable_execution", "failure_is_not_an_option"
}

// LoadBootstrap loads bootstrap JSON from a file (validates structure)
func LoadBootstrap(filename string) (*Bootstrap, error) {
	return LoadBootstrapWithOptions(filename, true)
}

// LoadBootstrapWithOptions loads bootstrap JSON and optionally validates
func LoadBootstrapWithOptions(filename string, validate bool) (*Bootstrap, error) {
	// Read the file
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	// Parse the JSON
	var bootstrap Bootstrap
	if err := json.Unmarshal(data, &bootstrap); err != nil {
		return nil, err
	}

	// Validate phase restrictions
	if validate {
		if err := ValidateBootstrap(&bootstrap); err != nil {
			return nil, err
		}
	}

	return &bootstrap, nil
}

// ValidateBootstrap validates that items are appropriate for their phases
func ValidateBootstrap(bootstrap *Bootstrap) error {
	// Validate preflight phase - original InstallApplications only allows single rootscript
	if len(bootstrap.Preflight) > 1 {
		return fmt.Errorf("preflight phase only supports a single rootscript (original InstallApplications compatibility)")
	}
	for _, item := range bootstrap.Preflight {
		if err := validateItemForPhase(item, "preflight"); err != nil {
			return err
		}
		// Ensure preflight only contains rootscripts
		if item.Type != "rootscript" {
			return fmt.Errorf("preflight phase only supports rootscript type, got: %s", item.Type)
		}
	}

	// Validate setupassistant phase
	for _, item := range bootstrap.SetupAssistant {
		if err := validateItemForPhase(item, "setupassistant"); err != nil {
			return err
		}
	}

	// Userland phase: allow all types, but still validate that type is recognized
	for _, item := range bootstrap.Userland {
		if err := validateItemForPhase(item, "userland"); err != nil {
			return err
		}
	}

	return nil
}

// validateItemForPhase ensures items are valid for their execution phase
func validateItemForPhase(item Item, phase string) error {
	// Validate allowed item types early
	switch item.Type {
	case "package", "rootscript", "userscript", "rootfile", "userfile":
		// ok
	default:
		return fmt.Errorf("invalid item type '%s' for '%s' (allowed: package, rootscript, userscript, rootfile, userfile)", item.Type, item.Name)
	}

	switch phase {
	case "preflight", "setupassistant":
		// These phases run as root daemon - only root operations allowed
		if item.Type == "userscript" || item.Type == "userfile" {
			return fmt.Errorf("phase '%s' only supports root operations (package, rootscript, rootfile), not '%s'", phase, item.Type)
		}
	case "userland":
		// Userland phase supports all types - no restrictions
	default:
		return fmt.Errorf("unknown phase: %s", phase)
	}

	// Validate fail policy if specified
	if item.FailPolicy != "" {
		if err := validateFailPolicy(item.FailPolicy); err != nil {
			return fmt.Errorf("invalid fail_policy for item '%s': %w", item.Name, err)
		}
	}

	return nil
}

// validateFailPolicy ensures fail policy values are valid
func validateFailPolicy(policy string) error {
	switch policy {
	case "failure_is_not_an_option", "failable", "failable_execution":
		return nil
	case "":
		return nil // Empty is valid (uses default)
	default:
		return fmt.Errorf("invalid fail_policy: '%s' (must be: failure_is_not_an_option, failable, or failable_execution)", policy)
	}
}

// GetEffectiveFailPolicy returns the effective fail policy with default fallback
func (item *Item) GetEffectiveFailPolicy() string {
	if item.FailPolicy == "" {
		return "failable_execution" // Default behavior - scripts can fail, packages must succeed
	}
	return item.FailPolicy
}
