package messageformat

import "fmt"

// Config holds configuration for the ICU MessageFormat format.
type Config struct {
	// disableNonTranslatableContent, when set, keeps literal prose that frames a
	// plural/select branch (the sentence frame around a picker, e.g. "You have "
	// and " in your cart." around {count, plural, …}) buried in the raw-line
	// skeleton instead of surfacing it as a non-translatable content block
	// (visible to ingestion/LLM consumers, skipped by MT). Zero value =
	// surfacing ON (the opt-out default).
	disableNonTranslatableContent bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "messageformat" }

// Reset restores default values.
func (c *Config) Reset() { *c = Config{} }

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ExtractNonTranslatableContent reports whether literal prose that frames a
// plural/select branch is surfaced as a non-translatable content block. Default
// true.
func (c *Config) ExtractNonTranslatableContent() bool {
	return !c.disableNonTranslatableContent
}

// SetExtractNonTranslatableContent toggles surfacing of literal framing prose as
// content blocks (used by the parity runner to match the Okapi bridge, which
// keeps such content in skeleton).
func (c *Config) SetExtractNonTranslatableContent(v bool) {
	c.disableNonTranslatableContent = !v
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "extractNonTranslatableContent":
			v, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractNonTranslatableContent: expected bool, got %T", val)
			}
			c.disableNonTranslatableContent = !v
		default:
			return fmt.Errorf("messageformat: unknown parameter: %s", key)
		}
	}
	return nil
}
