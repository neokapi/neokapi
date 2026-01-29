package tmx

// Config holds configuration for the TMX format.
type Config struct{}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "tmx" }

// Reset restores default values.
func (c *Config) Reset() {}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }
