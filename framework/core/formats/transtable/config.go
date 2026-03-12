package transtable

import "fmt"

// Config holds configuration for the translation table format.
type Config struct{}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "transtable" }

// Reset restores default values.
func (c *Config) Reset() {}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key := range values {
		return fmt.Errorf("transtable: unknown parameter: %s", key)
	}
	return nil
}
