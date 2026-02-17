package yaml

// Config holds configuration for the YAML format.
type Config struct{}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "yaml" }

// Reset restores default values.
func (c *Config) Reset() {}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }
