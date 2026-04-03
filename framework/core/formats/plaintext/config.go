package plaintext

import "fmt"

// Config holds configuration for the plain text format.
type Config struct {
	// SegmentByLine if true, each line is a Block. If false, paragraphs
	// (separated by blank lines) are Blocks.
	SegmentByLine bool `schema:"title=Segment by Line,description=If true each line becomes a separate block; if false paragraphs separated by blank lines are blocks,default=true"`
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "plaintext" }

// Reset restores default values.
func (c *Config) Reset() {
	c.SegmentByLine = true
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "segmentByLine":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("segmentByLine: expected bool, got %T", val)
			}
			c.SegmentByLine = b
		default:
			return fmt.Errorf("plaintext: unknown parameter: %s", key)
		}
	}
	return nil
}
