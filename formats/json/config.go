package json

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
