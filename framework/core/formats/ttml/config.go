package ttml

import (
	"fmt"
)

// toInt converts a value to int, handling JSON number types.
func toInt(val any) (int, bool) {
	switch v := val.(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	case int64:
		return int(v), true
	default:
		return 0, false
	}
}

// Config holds configuration for the TTML subtitle format.
type Config struct {
	// MergeAdjacentCaptions merges adjacent <p> elements whose text ends with
	// trailing punctuation (comma, semicolon) into a single block.
	MergeAdjacentCaptions bool

	// EscapeBR controls <br/> handling. When true (default), <br/> elements
	// are removed and surrounding text is joined with a space. When false,
	// <br/> is preserved as literal text in the extracted content.
	EscapeBR bool

	// MaxCharsPerLine sets the maximum characters per line in output.
	// 0 means no limit.
	MaxCharsPerLine int

	// MaxLinesPerCaption sets the maximum lines per caption in output.
	// 0 means no limit.
	MaxLinesPerCaption int

	// CJKCharsPerLine sets the maximum characters per line for CJK languages.
	// 0 means no limit (falls back to MaxCharsPerLine).
	CJKCharsPerLine int

	// SplitWords allows splitting words to enforce the character limit per line.
	SplitWords bool
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
	for key, val := range values {
		switch key {
		case "mergeAdjacentCaptions":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("mergeAdjacentCaptions: expected bool")
			}
			c.MergeAdjacentCaptions = b
		case "escapeBR":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("escapeBR: expected bool")
			}
			c.EscapeBR = b
		case "maxCharsPerLine":
			n, ok := toInt(val)
			if !ok {
				return fmt.Errorf("maxCharsPerLine: expected integer, got %T", val)
			}
			c.MaxCharsPerLine = n
		case "maxLinesPerCaption":
			n, ok := toInt(val)
			if !ok {
				return fmt.Errorf("maxLinesPerCaption: expected integer, got %T", val)
			}
			c.MaxLinesPerCaption = n
		case "cjkCharsPerLine":
			n, ok := toInt(val)
			if !ok {
				return fmt.Errorf("cjkCharsPerLine: expected integer, got %T", val)
			}
			c.CJKCharsPerLine = n
		case "splitWords":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("splitWords: expected bool")
			}
			c.SplitWords = b
		default:
			return fmt.Errorf("ttml: unknown parameter: %s", key)
		}
	}
	return nil
}
