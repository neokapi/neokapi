package epub

import "fmt"

// Config holds configuration for the EPUB format.
type Config struct {
	// disableNonTranslatableContent, when set, keeps non-translatable contextual
	// content (verbatim <pre>/<code>/<kbd>/<samp> listings in the direct-XHTML
	// fallback extractor) buried in opaque skeleton instead of surfacing it as
	// RoleCode content blocks (visible to ingestion/LLM consumers, skipped by
	// machine translation). Zero value = surfacing ON (the opt-out default), so
	// the default holds however the Config is constructed.
	disableNonTranslatableContent bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "epub" }

// Reset restores default values.
func (c *Config) Reset() { *c = Config{} }

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ExtractNonTranslatableContent reports whether non-translatable contextual
// content (verbatim code listings) is surfaced as RoleCode content blocks.
// Default true.
func (c *Config) ExtractNonTranslatableContent() bool {
	return !c.disableNonTranslatableContent
}

// SetExtractNonTranslatableContent toggles surfacing of non-translatable
// contextual content as content blocks (used by the parity runner to match the
// Okapi bridge, which keeps such content in skeleton).
func (c *Config) SetExtractNonTranslatableContent(v bool) {
	c.disableNonTranslatableContent = !v
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "extractNonTranslatableContent":
			if v, ok := val.(bool); ok {
				c.disableNonTranslatableContent = !v
			}
		default:
			return fmt.Errorf("epub: unknown parameter: %s", key)
		}
	}
	return nil
}
