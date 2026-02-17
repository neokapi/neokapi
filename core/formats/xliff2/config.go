package xliff2

// Config holds configuration for the XLIFF 2.0 format.
type Config struct{}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "xliff2" }

// Reset restores default values.
func (c *Config) Reset() {}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }
