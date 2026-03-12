package ttml

import "fmt"

// Config holds configuration for the TTML subtitle format.
type Config struct {
	// MergeAdjacentCaptions merges adjacent <p> elements whose text ends with
	// trailing punctuation (comma, semicolon) into a single block.
	MergeAdjacentCaptions bool

	// EscapeBR controls <br/> handling. When true (default), <br/> elements
	// are removed and surrounding text is joined with a space. When false,
	// <br/> is preserved as literal text in the extracted content.
	EscapeBR bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "ttml" }

// Reset restores default values.
func (c *Config) Reset() {
	c.MergeAdjacentCaptions = false
	c.EscapeBR = true
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "mergeAdjacentCaptions":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("mergeAdjacentCaptions: expected bool")
			}
			c.MergeAdjacentCaptions = b
		case "escapeBR":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("escapeBR: expected bool")
			}
			c.EscapeBR = b
		default:
			return fmt.Errorf("ttml: unknown parameter: %s", key)
		}
	}
	return nil
}
