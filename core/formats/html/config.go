package html

import "fmt"

// Config holds configuration for the HTML format.
type Config struct {
	// PreserveWhitespace preserves significant whitespace in text nodes.
	PreserveWhitespace bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "html" }

// Reset restores default values.
func (c *Config) Reset() {
	c.PreserveWhitespace = false
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "preserveWhitespace":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("preserveWhitespace: expected bool, got %T", val)
			}
			c.PreserveWhitespace = b
		default:
			return fmt.Errorf("html: unknown parameter: %s", key)
		}
	}
	return nil
}
