package plaintext

import "github.com/neokapi/neokapi/core/format"

// Config holds configuration for the plain text format.
type Config struct {
	// SegmentByLine if true, each line is a Block. If false, paragraphs
	// (separated by blank lines) are Blocks.
	SegmentByLine bool `json:"segmentByLine" schema:"title=Segment by Line,description=If true each line becomes a separate block; if false paragraphs separated by blank lines are blocks,default=true"`
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
	return format.ApplyMapViaJSON(c, values)
}
