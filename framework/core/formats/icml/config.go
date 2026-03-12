package icml

import "fmt"

// Config holds configuration for the Adobe InCopy ICML format.
type Config struct {
	// ExtractNotes controls whether note content is extracted as translatable.
	ExtractNotes bool

	// NewTUOnBr creates a new translation unit when a <Br/> element is encountered.
	NewTUOnBr bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "icml" }

// Reset restores default values.
func (c *Config) Reset() {
	c.ExtractNotes = false
	c.NewTUOnBr = false
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "extractNotes":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractNotes: expected bool")
			}
			c.ExtractNotes = b
		case "newTuOnBr":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("newTuOnBr: expected bool")
			}
			c.NewTUOnBr = b
		default:
			return fmt.Errorf("icml: unknown parameter: %s", key)
		}
	}
	return nil
}
