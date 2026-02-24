package properties

import "fmt"

// Config holds configuration for the Java Properties format.
type Config struct {
	// Separator is the key-value separator character. Default is '='.
	Separator string
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "properties" }

// Reset restores default values.
func (c *Config) Reset() {
	c.Separator = "="
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "separator":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("separator: expected string, got %T", val)
			}
			c.Separator = s
		default:
			return fmt.Errorf("properties: unknown parameter: %s", key)
		}
	}
	return nil
}
