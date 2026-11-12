package pdf

import "github.com/neokapi/neokapi/core/format"

// Config holds configuration for the PDF format.
type Config struct{}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "pdf" }

// Reset restores default values.
func (c *Config) Reset() {}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key := range values {
		return format.UnknownParameterError(c, "pdf", key)
	}
	return nil
}
