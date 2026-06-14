package docling

import "github.com/neokapi/neokapi/core/format"

// Config holds configuration for the DoclingDocument JSON format. The reader is
// read-only and currently has no tunables; the struct exists so the format
// participates uniformly in the config/schema registry.
type Config struct{}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "docling" }

// Reset restores default values.
func (c *Config) Reset() {}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	return format.ApplyMapViaJSON(c, values)
}
