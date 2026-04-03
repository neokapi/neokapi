package vignette

import "fmt"

// Config holds configuration for the R Vignette format.
type Config struct{}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "vignette" }

// Reset restores default values.
func (c *Config) Reset() {}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key := range values {
		return fmt.Errorf("vignette: unknown parameter: %s", key)
	}
	return nil
}
