package odf

import "fmt"

// Config holds configuration for the ODF format reader/writer.
type Config struct {
	// TranslateNotes controls extraction of presentation notes.
	TranslateNotes bool

	// TranslateHiddenContent controls extraction of hidden content.
	TranslateHiddenContent bool
}

// FormatName returns the format identifier.
func (c *Config) FormatName() string { return "odf" }

// Reset restores default configuration values.
func (c *Config) Reset() {
	c.TranslateNotes = true
	c.TranslateHiddenContent = false
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "translateNotes":
			c.TranslateNotes = toBool(val)
		case "translateHiddenContent":
			c.TranslateHiddenContent = toBool(val)
		default:
			return fmt.Errorf("odf: unknown config key %q", key)
		}
	}
	return nil
}

// toBool converts a value to bool, accepting bool and string representations.
func toBool(val any) bool {
	switch v := val.(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1" || v == "yes"
	default:
		return false
	}
}
