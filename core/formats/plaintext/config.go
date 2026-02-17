package plaintext

// Config holds configuration for the plain text format.
type Config struct {
	// SegmentByLine if true, each line is a Block. If false, paragraphs
	// (separated by blank lines) are Blocks.
	SegmentByLine bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "plaintext" }

// Reset restores default values.
func (c *Config) Reset() {
	c.SegmentByLine = true
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }
