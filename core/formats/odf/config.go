package odf

import "fmt"

// Config holds configuration for the ODF format reader/writer.
type Config struct {
	// TranslateNotes controls extraction of presentation notes.
	TranslateNotes bool

	// TranslateHiddenContent controls extraction of hidden content.
	TranslateHiddenContent bool

	// disableNonTranslatableContent, when set, keeps non-translatable
	// contextual content — image accessibility text (<svg:title>/<svg:desc>
	// alt-text and long descriptions on drawing frames) and form-control
	// display strings (form:label / form:title / form:help-text) — in opaque
	// skeleton instead of surfacing it as Translatable:false content blocks
	// (visible to ingestion/LLM consumers, skipped by machine translation).
	// Zero value = surfacing ON (the opt-out default). This does not change
	// the translatable/MT payload; surfaced content is always non-translatable.
	disableNonTranslatableContent bool
}

// FormatName returns the format identifier.
func (c *Config) FormatName() string { return "odf" }

// Reset restores default configuration values.
func (c *Config) Reset() {
	c.TranslateNotes = true
	c.TranslateHiddenContent = false
	c.disableNonTranslatableContent = false
}

// ExtractNonTranslatableContent reports whether non-translatable contextual
// content (image accessibility text and form-control display strings) is
// surfaced as content blocks. Default true.
func (c *Config) ExtractNonTranslatableContent() bool {
	return !c.disableNonTranslatableContent
}

// SetExtractNonTranslatableContent toggles surfacing of non-translatable
// contextual content as content blocks. The parity runner sets this false to
// match the Okapi bridge, which keeps such content in skeleton.
func (c *Config) SetExtractNonTranslatableContent(v bool) {
	c.disableNonTranslatableContent = !v
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "translateNotes":
			c.TranslateNotes = toBool(val)
		case "translateHiddenContent":
			c.TranslateHiddenContent = toBool(val)
		case "extractNonTranslatableContent":
			c.disableNonTranslatableContent = !toBool(val)
		default:
			return fmt.Errorf("odf: unknown config key %q", key)
		}
	}
	return nil
}

// toBool converts a value to bool, accepting bool and string representations.
func toBool(val any) bool {
	switch v := val.(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1" || v == "yes"
	default:
		return false
	}
}
