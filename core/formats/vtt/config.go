package vtt

import (
	"fmt"

	"github.com/neokapi/neokapi/core/format"
)

// Config holds configuration for the WebVTT subtitle format.
type Config struct {
	// MaxCharsPerLine sets the maximum characters per line in output.
	// 0 means no limit.
	MaxCharsPerLine int `json:"maxCharsPerLine"`

	// MaxLinesPerCaption sets the maximum lines per caption in output.
	// 0 means no limit.
	MaxLinesPerCaption int `json:"maxLinesPerCaption"`

	// CJKCharsPerLine sets the maximum characters per line for CJK languages.
	// 0 means no limit (falls back to MaxCharsPerLine).
	CJKCharsPerLine int `json:"cjkCharsPerLine"`

	// SplitWords allows splitting words to enforce the character limit per line.
	SplitWords bool `json:"splitWords"`

	// disableNonTranslatableContent, when set, keeps non-translatable contextual
	// content (the embedded CSS of a WebVTT STYLE block) in the legacy
	// cue-mis-parse path instead of surfacing it as a RoleCode content block
	// (visible to ingestion/LLM consumers, skipped by MT). Zero value =
	// surfacing ON (the opt-out default).
	disableNonTranslatableContent bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "vtt" }

// Reset restores default values.
func (c *Config) Reset() {
	c.MaxCharsPerLine = 0
	c.MaxLinesPerCaption = 0
	c.CJKCharsPerLine = 0
	c.SplitWords = false
	c.disableNonTranslatableContent = false
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ExtractNonTranslatableContent reports whether non-translatable contextual
// content (the embedded CSS of a STYLE block) is surfaced as a RoleCode content
// block. Default true.
func (c *Config) ExtractNonTranslatableContent() bool {
	return !c.disableNonTranslatableContent
}

// SetExtractNonTranslatableContent toggles surfacing of non-translatable
// contextual content as content blocks (used by the parity runner to match the
// Okapi bridge, which keeps such content in skeleton / opaque cues).
func (c *Config) SetExtractNonTranslatableContent(v bool) {
	c.disableNonTranslatableContent = !v
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	// extractNonTranslatableContent maps to the inverted private field, which
	// JSON (un)marshalling cannot reach. Pull it out and apply the remaining
	// keys via the JSON path.
	if raw, ok := values["extractNonTranslatableContent"]; ok {
		v, ok := raw.(bool)
		if !ok {
			return fmt.Errorf("extractNonTranslatableContent: expected bool, got %T", raw)
		}
		c.disableNonTranslatableContent = !v
		rest := make(map[string]any, len(values))
		for k, val := range values {
			if k == "extractNonTranslatableContent" {
				continue
			}
			rest[k] = val
		}
		values = rest
	}
	return format.ApplyMapViaJSON(c, values)
}
