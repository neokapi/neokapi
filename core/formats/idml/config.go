package idml

import "fmt"

// Config holds configuration for the IDML format reader/writer.
type Config struct {
	// ExtractMasterSpreads controls whether text in master spread stories is extracted.
	ExtractMasterSpreads bool

	// ExtractNotes controls whether footnote and endnote text is extracted.
	ExtractNotes bool

	// SkipDiscretionaryHyphens removes discretionary (soft) hyphens from extracted text.
	SkipDiscretionaryHyphens bool
}

// FormatName returns the format identifier.
func (c *Config) FormatName() string { return "idml" }

// Reset restores default configuration values.
//
// Defaults track okapi's IDML filter defaults so out-of-the-box
// extraction matches the reference engine. ExtractNotes=false in
// particular avoids surfacing reviewer notes (Note/Footnote/Endnote)
// as translatable units — okapi's filter excludes them unless the
// caller opts in.
func (c *Config) Reset() {
	c.ExtractMasterSpreads = false
	c.ExtractNotes = false
	c.SkipDiscretionaryHyphens = true
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "extractMasterSpreads":
			c.ExtractMasterSpreads = toBool(val)
		case "extractNotes":
			c.ExtractNotes = toBool(val)
		case "skipDiscretionaryHyphens":
			c.SkipDiscretionaryHyphens = toBool(val)
		default:
			return fmt.Errorf("idml: unknown config key %q", key)
		}
	}
	return nil
}

// toBool converts a value to bool.
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
