package ttml

import "github.com/neokapi/neokapi/core/format"

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
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	return format.ApplyMapViaJSON(c, values)
}
