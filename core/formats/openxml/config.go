package openxml

import "fmt"

// Config holds configuration for the OpenXML format reader/writer.
type Config struct {
	TranslateDocProperties   bool // Extract title, subject, keywords from docProps/core.xml
	TranslateHiddenText      bool // Extract text with vanish property
	TranslateHeadersFooters  bool // Extract headers and footers
	TranslateFootnotes       bool // Extract footnotes and endnotes
	TranslateComments        bool // Extract comments
	AggressiveCleanup        bool // Strip rsid*, proofErr, lastRenderedPageBreak before merging
	TabAsCharacter           bool // Treat <w:tab/> as a tab character instead of a placeholder span
	TranslateHyperlinks      bool // Extract hyperlink text
	TranslateSlideNotes      bool // PPTX: extract slide notes
	TranslateSlideMasters    bool // PPTX: extract slide master text
	TranslateSheetNames      bool // XLSX: extract sheet names
	TranslateSharedStrings   bool // XLSX: extract shared strings
}

// FormatName returns the format identifier.
func (c *Config) FormatName() string { return "openxml" }

// Reset restores default configuration values.
func (c *Config) Reset() {
	c.TranslateDocProperties = true
	c.TranslateHiddenText = false
	c.TranslateHeadersFooters = true
	c.TranslateFootnotes = true
	c.TranslateComments = false
	c.AggressiveCleanup = true
	c.TabAsCharacter = false
	c.TranslateHyperlinks = true
	c.TranslateSlideNotes = true
	c.TranslateSlideMasters = false
	c.TranslateSheetNames = false
	c.TranslateSharedStrings = true
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		b, ok := val.(bool)
		if !ok {
			return fmt.Errorf("openxml: config key %q expects bool, got %T", key, val)
		}
		switch key {
		case "translateDocProperties":
			c.TranslateDocProperties = b
		case "translateHiddenText":
			c.TranslateHiddenText = b
		case "translateHeadersFooters":
			c.TranslateHeadersFooters = b
		case "translateFootnotes":
			c.TranslateFootnotes = b
		case "translateComments":
			c.TranslateComments = b
		case "aggressiveCleanup":
			c.AggressiveCleanup = b
		case "tabAsCharacter":
			c.TabAsCharacter = b
		case "translateHyperlinks":
			c.TranslateHyperlinks = b
		default:
			return fmt.Errorf("openxml: unknown config key %q", key)
		}
	}
	return nil
}
