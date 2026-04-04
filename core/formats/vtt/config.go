package vtt

import "github.com/neokapi/neokapi/core/format"

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
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "vtt" }

// Reset restores default values.
func (c *Config) Reset() {
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
