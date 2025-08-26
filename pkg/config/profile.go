// pkg/config/profile.go
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"howett.net/plist"
)

const DefaultProfileDomain = "com.github.go-installapplications"

// ProfileResult contains what we read from the mobile config
type ProfileResult struct {
	ConfigFound     bool
	BootstrapSource string // "json_url", "embedded", or "none"
}

// ReadFromProfile reads configuration from nested mobile config structure
func (c *Config) ReadFromProfile(domain string) (*ProfileResult, error) {
	if domain == "" {
		domain = DefaultProfileDomain
	}

	// Try multiple locations where preferences might be stored
	prefs := c.readManagedPrefs(domain)
	if prefs == nil {
		prefs = c.readUserPrefs(domain)
	}

	if prefs == nil {
		return &ProfileResult{ConfigFound: false, BootstrapSource: "none"}, nil
	}

	result := &ProfileResult{ConfigFound: true}

	// Step 1: Apply shared settings first
	if err := c.applySharedSettings(prefs); err != nil {
		return nil, fmt.Errorf("failed to apply shared settings: %w", err)
	}

	// Step 2: Apply mode-specific overrides
	if err := c.applyModeSettings(prefs); err != nil {
		return nil, fmt.Errorf("failed to apply mode settings: %w", err)
	}

	// Step 3: Determine bootstrap source and validate
	bootstrapSource, err := c.determineBootstrapSource(prefs)
	if err != nil {
		return nil, err
	}
	result.BootstrapSource = bootstrapSource

	return result, nil
}

// readManagedPrefs reads from managed preferences (mobile config)
func (c *Config) readManagedPrefs(domain string) map[string]interface{} {
	managedPath := fmt.Sprintf("/Library/Managed Preferences/%s.plist", domain)
	return c.readPlistFile(managedPath)
}

// readUserPrefs reads from user preferences (manual defaults write)
func (c *Config) readUserPrefs(domain string) map[string]interface{} {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	userPath := filepath.Join(homeDir, "Library", "Preferences", domain+".plist")
	return c.readPlistFile(userPath)
}

// readPlistFile reads a plist file and returns its contents
func (c *Config) readPlistFile(path string) map[string]interface{} {
	file, err := os.Open(path)
	if err != nil {
		return nil // File doesn't exist or can't be read
	}
	defer file.Close()

	var prefs map[string]interface{}
	decoder := plist.NewDecoder(file)
	if err := decoder.Decode(&prefs); err != nil {
		return nil // Can't parse plist
	}

	return prefs
}

// applySharedSettings applies shared configuration settings
func (c *Config) applySharedSettings(prefs map[string]interface{}) error {
	shared, ok := prefs["shared"]
	if !ok {
		return nil // No shared settings
	}

	sharedMap, ok := shared.(map[string]interface{})
	if !ok {
		return fmt.Errorf("shared settings is not a dictionary")
	}

	return c.applySettingsMap(sharedMap)
}

// applyModeSettings applies mode-specific overrides
func (c *Config) applyModeSettings(prefs map[string]interface{}) error {
	// Agent does not require separate mode-specific options in the new model.
	// The agent acts as an IPC server and uses shared settings (e.g., Debug/Verbose).
	if c.Mode == "agent" {
		return nil
	}
	modeSettings, ok := prefs[c.Mode]
	if !ok {
		return nil // No mode-specific settings
	}

	modeMap, ok := modeSettings.(map[string]interface{})
	if !ok {
		return fmt.Errorf("%s settings is not a dictionary", c.Mode)
	}

	return c.applySettingsMap(modeMap)
}

// determineBootstrapSource checks bootstrap source and validates no conflicts
func (c *Config) determineBootstrapSource(prefs map[string]interface{}) (string, error) {
	hasJSONURL := c.JSONURL != ""
	hasBootstrapSection := false

	// Check if bootstrap section exists at top level (shared)
	if bootstrap, exists := prefs["bootstrap"]; exists {
		if bootstrapMap, ok := bootstrap.(map[string]interface{}); ok {
			// Check for preflight, setupassistant, or userland arrays
			if preflight, ok := bootstrapMap["preflight"].([]interface{}); ok && len(preflight) > 0 {
				hasBootstrapSection = true
			}
			if setupassistant, ok := bootstrapMap["setupassistant"].([]interface{}); ok && len(setupassistant) > 0 {
				hasBootstrapSection = true
			}
			if userland, ok := bootstrapMap["userland"].([]interface{}); ok && len(userland) > 0 {
				hasBootstrapSection = true
			}
		}
	}

	// Check if bootstrap section exists in current mode's settings (mode-specific)
	if c.bootstrapConfig != nil {
		hasBootstrapSection = true
	}

	// Validate no conflicts
	if hasJSONURL && hasBootstrapSection {
		return "", fmt.Errorf("mobile config error: cannot have both JSONURL and bootstrap section - choose one bootstrap source")
	}

	if hasJSONURL {
		return "json_url", nil
	} else if hasBootstrapSection {
		return "embedded", nil
	} else {
		return "none", nil
	}
}

// applySettingsMap applies a settings map to the config
func (c *Config) applySettingsMap(settings map[string]interface{}) error {
	if val, exists := settings["JSONURL"]; exists {
		if str, ok := val.(string); ok {
			if str == "" {
				return fmt.Errorf("JSONURL cannot be empty string - omit the key instead")
			}
			c.JSONURL = str
		}
	}

	if val, exists := settings["InstallPath"]; exists {
		if str, ok := val.(string); ok && str != "" {
			c.InstallPath = str
		}
	}

	if val, exists := settings["Debug"]; exists {
		if b, ok := val.(bool); ok {
			c.Debug = b
		}
	}

	if val, exists := settings["Verbose"]; exists {
		if b, ok := val.(bool); ok {
			c.Verbose = b
		}
	}

	if val, exists := settings["Reboot"]; exists {
		switch v := val.(type) {
		case bool:
			c.Reboot = v
		case string:
			if parsed, err := strconv.ParseBool(v); err == nil {
				c.Reboot = parsed
			}
		}
	}

	if val, exists := settings["MaxRetries"]; exists {
		if i, ok := val.(int64); ok {
			c.MaxRetries = int(i)
		} else if i, ok := val.(int); ok {
			c.MaxRetries = i
		}
	}

	if val, exists := settings["RetryDelay"]; exists {
		if i, ok := val.(int64); ok {
			c.RetryDelay = int(i)
		} else if i, ok := val.(int); ok {
			c.RetryDelay = i
		}
	}

	if val, exists := settings["CleanupOnFailure"]; exists {
		if b, ok := val.(bool); ok {
			c.CleanupOnFailure = b
		}
	}

	if val, exists := settings["KeepFailedFiles"]; exists {
		if b, ok := val.(bool); ok {
			c.KeepFailedFiles = b
		}
	}

	if val, exists := settings["DryRun"]; exists {
		if b, ok := val.(bool); ok {
			c.DryRun = b
		}
	}

	if val, exists := settings["TrackBackgroundProcesses"]; exists {
		if b, ok := val.(bool); ok {
			c.TrackBackgroundProcesses = b
		}
	}

	if val, exists := settings["BackgroundTimeout"]; exists {
		if i, ok := val.(int64); ok {
			c.BackgroundTimeout = time.Duration(i) * time.Second
		} else if i, ok := val.(int); ok {
			c.BackgroundTimeout = time.Duration(i) * time.Second
		} else if str, ok := val.(string); ok {
			if duration, err := time.ParseDuration(str); err == nil {
				c.BackgroundTimeout = duration
			} else if seconds, err := strconv.Atoi(str); err == nil {
				c.BackgroundTimeout = time.Duration(seconds) * time.Second
			}
			if val, exists := settings["DownloadMaxConcurrency"]; exists {
				if i, ok := val.(int64); ok {
					c.DownloadMaxConcurrency = int(i)
				} else if i, ok := val.(int); ok {
					c.DownloadMaxConcurrency = i
				} else if str, ok := val.(string); ok {
					if iv, err := strconv.Atoi(str); err == nil {
						c.DownloadMaxConcurrency = iv
					}
				}
			}

			// IPC/coordination timeouts (accept seconds as int or duration string)
			if val, exists := settings["WaitForAgentTimeout"]; exists {
				if i, ok := val.(int64); ok {
					c.WaitForAgentTimeout = time.Duration(i) * time.Second
				} else if i, ok := val.(int); ok {
					c.WaitForAgentTimeout = time.Duration(i) * time.Second
				} else if str, ok := val.(string); ok {
					if d, err := time.ParseDuration(str); err == nil {
						c.WaitForAgentTimeout = d
					} else if seconds, err := strconv.Atoi(str); err == nil {
						c.WaitForAgentTimeout = time.Duration(seconds) * time.Second
					}
				}
			}
			if val, exists := settings["AgentRequestTimeout"]; exists {
				if i, ok := val.(int64); ok {
					c.AgentRequestTimeout = time.Duration(i) * time.Second
				} else if i, ok := val.(int); ok {
					c.AgentRequestTimeout = time.Duration(i) * time.Second
				} else if str, ok := val.(string); ok {
					if d, err := time.ParseDuration(str); err == nil {
						c.AgentRequestTimeout = d
					} else if seconds, err := strconv.Atoi(str); err == nil {
						c.AgentRequestTimeout = time.Duration(seconds) * time.Second
					}
				}
			}
		}
	}

	// HTTP Authentication settings
	if val, exists := settings["HTTPAuthUser"]; exists {
		if str, ok := val.(string); ok && str != "" {
			c.HTTPAuthUser = str
		}
	}

	// Backwards-compatibility options
	if val, exists := settings["FollowRedirects"]; exists {
		if b, ok := val.(bool); ok {
			c.FollowRedirects = b
		}
	}
	if val, exists := settings["SkipValidation"]; exists {
		if b, ok := val.(bool); ok {
			c.SkipValidation = b
		}
	}

	if val, exists := settings["LaunchAgentIdentifier"]; exists {
		if str, ok := val.(string); ok && str != "" {
			c.LaunchAgentIdentifier = str
		}
	}
	if val, exists := settings["LaunchDaemonIdentifier"]; exists {
		if str, ok := val.(string); ok && str != "" {
			c.LaunchDaemonIdentifier = str
		}
	}

	if val, exists := settings["HTTPAuthPassword"]; exists {
		if str, ok := val.(string); ok && str != "" {
			c.HTTPAuthPassword = str
		}
	}

	// HTTP Headers (for advanced authentication or custom headers)
	if val, exists := settings["HTTPHeaders"]; exists {
		if c.HTTPHeaders == nil {
			c.HTTPHeaders = make(map[string]string)
		}

		// Handle both dictionary format and array format
		if headersMap, ok := val.(map[string]interface{}); ok {
			// Dictionary format: {"Authorization": "Basic xyz", "X-API-Key": "abc"}
			for key, value := range headersMap {
				if strValue, ok := value.(string); ok {
					c.HTTPHeaders[key] = strValue
				}
			}
		} else if headersArray, ok := val.([]interface{}); ok {
			// Array format: [{"name": "Authorization", "value": "Basic xyz"}]
			for _, item := range headersArray {
				if headerDict, ok := item.(map[string]interface{}); ok {
					if name, nameOk := headerDict["name"].(string); nameOk {
						if value, valueOk := headerDict["value"].(string); valueOk {
							c.HTTPHeaders[name] = value
						}
					}
				}
			}
		}
	}

	// Convenience: single Authorization header value (original --headers)
	if val, exists := settings["HeaderAuthorization"]; exists {
		if str, ok := val.(string); ok && str != "" {
			c.HeaderAuthorization = str
			if c.HTTPHeaders == nil {
				c.HTTPHeaders = map[string]string{}
			}
			c.HTTPHeaders["Authorization"] = str
		}
	}

	// Remote log shipping: LogDestination, LogProvider, LogHeaders NOT YET IMPLEMENTED
	// if val, exists := settings["LogDestination"]; exists {
	// 	if str, ok := val.(string); ok && str != "" {
	// 		c.LogDestination = str
	// 	}
	// }
	// if val, exists := settings["LogProvider"]; exists {
	// 	if str, ok := val.(string); ok && str != "" {
	// 		c.LogProvider = str
	// 	}
	// }
	// if val, exists := settings["LogHeaders"]; exists {
	// 	if c.LogHeaders == nil {
	// 		c.LogHeaders = make(map[string]string)
	// 	}
	// 	if headersMap, ok := val.(map[string]interface{}); ok {
	// 		for key, value := range headersMap {
	// 			if strValue, ok := value.(string); ok {
	// 				c.LogHeaders[key] = strValue
	// 			}
	// 		}
	// 	} else if headersArray, ok := val.([]interface{}); ok {
	// 		for _, item := range headersArray {
	// 			if headerDict, ok := item.(map[string]interface{}); ok {
	// 				if name, nameOk := headerDict["name"].(string); nameOk {
	// 					if value, valueOk := headerDict["value"].(string); valueOk {
	// 						c.LogHeaders[name] = value
	// 					}
	// 				}
	// 			}
	// 		}
	// 	}
	// }

	// Handle bootstrap section in settings
	if val, exists := settings["bootstrap"]; exists {
		c.bootstrapConfig = val
	}

	// Don't override Mode from profile - that should come from command line or defaults
	return nil
}

// LoadBootstrapFromProfile extracts bootstrap configuration from mobile config
func (c *Config) LoadBootstrapFromProfile(domain string) (*Bootstrap, error) {
	if domain == "" {
		domain = DefaultProfileDomain
	}

	// Try multiple locations where preferences might be stored
	prefs := c.readManagedPrefs(domain)
	if prefs == nil {
		prefs = c.readUserPrefs(domain)
	}

	if prefs == nil {
		return nil, fmt.Errorf("no mobile config found for domain: %s", domain)
	}

	// Priority: mode-specific bootstrap > top-level bootstrap
	var bootstrap interface{}

	// Check for mode-specific bootstrap first
	if c.bootstrapConfig != nil {
		bootstrap = c.bootstrapConfig
	} else {
		// Fall back to top-level bootstrap
		var exists bool
		bootstrap, exists = prefs["bootstrap"]
		if !exists {
			return nil, fmt.Errorf("no bootstrap section found in mobile config")
		}
	}

	bootstrapMap, ok := bootstrap.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("bootstrap section is not a dictionary")
	}

	// Convert the interface{} map to JSON and back to Bootstrap struct
	// This handles the type conversion from plist to our struct
	jsonData, err := json.Marshal(bootstrapMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal bootstrap section: %w", err)
	}

	var bootstrapConfig Bootstrap
	if err := json.Unmarshal(jsonData, &bootstrapConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal bootstrap section: %w", err)
	}

	return &bootstrapConfig, nil
}
