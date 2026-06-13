package doclang

import "github.com/neokapi/neokapi/core/format"

// Config holds configuration for the DocLang format.
type Config struct {
	// EmitGeometry controls whether the writer emits <location> blocks from a
	// block's geometry annotation. Default true; set false to project to a
	// geometry-less DocLang (e.g. when re-emitting reflowable content).
	EmitGeometry bool `json:"emitGeometry"`
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "doclang" }

// Reset restores default values.
func (c *Config) Reset() {
	c.EmitGeometry = true
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	return format.ApplyMapViaJSON(c, values)
}
