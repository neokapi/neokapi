package tex

import "fmt"

// Config holds configuration for the TeX/LaTeX format.
type Config struct {
	// disableNonTranslatableContent, when set, keeps non-translatable
	// contextual content (verbatim/lstlisting code and math) in opaque
	// skeleton/Data instead of surfacing it as RoleCode / RoleFormula
	// content blocks (visible to ingestion/LLM consumers, skipped by MT).
	// Zero value = surfacing ON (the opt-out default).
	disableNonTranslatableContent bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "tex" }

// Reset restores default values.
func (c *Config) Reset() { *c = Config{} }

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ExtractNonTranslatableContent reports whether non-translatable contextual
// content (verbatim/lstlisting code, math) is surfaced as content blocks
// (RoleCode / RoleFormula) instead of opaque skeleton/Data. Default true.
func (c *Config) ExtractNonTranslatableContent() bool {
	return !c.disableNonTranslatableContent
}

// SetExtractNonTranslatableContent toggles surfacing of non-translatable
// contextual content as content blocks (used by the parity runner to match the
// Okapi bridge, which keeps such content opaque).
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
			return fmt.Errorf("tex: unknown parameter: %s", key)
		}
	}
	return nil
}
