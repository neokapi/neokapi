package properties

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
