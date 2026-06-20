package ttml

import (
	"fmt"

	"github.com/neokapi/neokapi/core/format"
)

// Config holds configuration for the TTML subtitle format.
type Config struct {
	// MergeAdjacentCaptions merges adjacent <p> elements whose text ends with
	// trailing punctuation (comma, semicolon) into a single block.
	MergeAdjacentCaptions bool `json:"mergeAdjacentCaptions"`

	// EscapeBR controls <br/> handling. When true (default), <br/> elements
	// are removed and surrounding text is joined with a space. When false,
	// <br/> is preserved as literal text in the extracted content.
	EscapeBR bool `json:"escapeBR"`

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
	// content from the document head (the ttm:copyright and ttm:agent metadata)
	// in opaque skeleton instead of surfacing it as RoleCode content blocks
	// (visible to ingestion/LLM consumers, skipped by MT). Zero value = surfacing
	// ON (the opt-out default). Inverted so the default is ON regardless of how
	// the Config is constructed. No json tag: it is driven solely via the
	// extractNonTranslatableContent ApplyMap key / SetExtractNonTranslatableContent.
	disableNonTranslatableContent bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "ttml" }

// Reset restores default values.
func (c *Config) Reset() {
	c.MergeAdjacentCaptions = false
	c.EscapeBR = true
	c.MaxCharsPerLine = 0
	c.MaxLinesPerCaption = 0
	c.CJKCharsPerLine = 0
	c.SplitWords = false
	c.disableNonTranslatableContent = false
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ExtractNonTranslatableContent reports whether non-translatable contextual
// head metadata (ttm:copyright, ttm:agent) is surfaced as RoleCode content
// blocks. Default true.
func (c *Config) ExtractNonTranslatableContent() bool {
	return !c.disableNonTranslatableContent
}

// SetExtractNonTranslatableContent toggles surfacing of non-translatable head
// metadata as content blocks (used by the parity runner to match the Okapi
// bridge, which keeps such content in skeleton).
func (c *Config) SetExtractNonTranslatableContent(v bool) {
	c.disableNonTranslatableContent = !v
}

// ApplyMap applies configuration values from a map.
//
// extractNonTranslatableContent drives the inverted, unexported
// disableNonTranslatableContent field and so is handled here before delegating
// the remaining (json-tagged) keys to ApplyMapViaJSON.
func (c *Config) ApplyMap(values map[string]any) error {
	if v, ok := values["extractNonTranslatableContent"]; ok {
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf("extractNonTranslatableContent: expected bool, got %T", v)
		}
		c.disableNonTranslatableContent = !b
		rest := make(map[string]any, len(values))
		for k, val := range values {
			if k == "extractNonTranslatableContent" {
				continue
			}
			rest[k] = val
		}
		values = rest
	}
	if len(values) == 0 {
		return nil
	}
	return format.ApplyMapViaJSON(c, values)
}
