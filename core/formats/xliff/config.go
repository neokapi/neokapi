package xliff

// Config holds configuration for the XLIFF 1.2 format.
type Config struct{}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "xliff" }

// Reset restores default values.
func (c *Config) Reset() {}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }
