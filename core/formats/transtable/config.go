package transtable

import "fmt"

// Config holds configuration for the TransTable v1 format.
type Config struct {
	// AllowSegments toggles whether `:s=<seg-id>` suffixes on the crumb
	// id are honored. When true (the default), rows sharing the same
	// `tu=<id>` and carrying a `:s=<seg-id>` suffix merge into one
	// segmented text unit. When false, every row is its own text unit
	// regardless of any segment suffix.
	//
	// Mirrors the upstream Java parameter `allowSegments`.
	AllowSegments bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "transtable" }

// Reset restores default values.
func (c *Config) Reset() {
	c.AllowSegments = true
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, raw := range values {
		switch key {
		case "allowSegments":
			b, ok := raw.(bool)
			if !ok {
				return fmt.Errorf("transtable: allowSegments: expected bool, got %T", raw)
			}
			c.AllowSegments = b
		default:
			return fmt.Errorf("transtable: unknown parameter: %s", key)
		}
	}
	return nil
}
