package html

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
