package json

import "fmt"

// Config holds configuration for the JSON format.
type Config struct {
	// ExtractArrayStrings controls whether string values inside arrays
	// are extracted as translatable Blocks. Defaults to true.
	ExtractArrayStrings bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "json" }

// Reset restores default values.
func (c *Config) Reset() {
	c.ExtractArrayStrings = true
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "extractArrayStrings":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractArrayStrings: expected bool, got %T", val)
			}
			c.ExtractArrayStrings = b
		default:
			return fmt.Errorf("json: unknown parameter: %s", key)
		}
	}
	return nil
}
