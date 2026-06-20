package doclang

import "github.com/neokapi/neokapi/core/format"

// Config holds configuration for the DocLang format.
type Config struct {
	// EmitGeometry controls whether the writer emits <location> blocks from a
	// block's geometry annotation. Default true; set false to project to a
	// geometry-less DocLang (e.g. when re-emitting reflowable content).
	EmitGeometry bool `json:"emitGeometry"`

	// ExtractNonTranslatableContent controls whether non-translatable contextual
	// content (table/figure/picture <caption> text) is surfaced as RoleCaption
	// content blocks (visible to ingestion, skipped by MT). Default true; disable
	// to keep captions in skeleton.
	ExtractNonTranslatableContent bool `json:"extractNonTranslatableContent"`
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "doclang" }

// SetExtractNonTranslatableContent toggles surfacing of non-translatable
// contextual content as content blocks (used by the parity runner to match the
// Okapi bridge).
func (c *Config) SetExtractNonTranslatableContent(v bool) { c.ExtractNonTranslatableContent = v }

// Reset restores default values.
func (c *Config) Reset() {
	c.EmitGeometry = true
	c.ExtractNonTranslatableContent = true
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	return format.ApplyMapViaJSON(c, values)
}
