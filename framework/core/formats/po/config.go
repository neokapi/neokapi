package po

import "fmt"

// Config holds configuration for the PO format.
type Config struct {
	// PreserveUntranslated if true, emits blocks for entries with empty msgstr.
	PreserveUntranslated bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "po" }

// Reset restores default values.
func (c *Config) Reset() {
	c.PreserveUntranslated = true
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "preserveUntranslated":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("preserveUntranslated: expected bool, got %T", val)
			}
			c.PreserveUntranslated = b
		default:
			return fmt.Errorf("po: unknown parameter: %s", key)
		}
	}
	return nil
}
