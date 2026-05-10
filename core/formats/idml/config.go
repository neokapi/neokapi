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

	// ExtractHiddenLayers controls whether stories whose parent
	// TextFrame sits on a hidden InDesign layer
	// (designmap.xml `<Layer Visible="false">`) are extracted.
	// Mirrors okapi's `extractHiddenLayers` parameter
	// (Parameters.java:63, default false).
	ExtractHiddenLayers bool

	// ExtractHiddenPasteboardItems controls whether stories whose
	// parent TextFrame on a Spread/MasterSpread carries
	// `Visible="false"` are extracted. Mirrors okapi's
	// `extractHiddenPasteboardItems` parameter
	// (Parameters.java:64, default false).
	ExtractHiddenPasteboardItems bool

	// ExtractHyperlinkTextSourcesInline controls how
	// HyperlinkTextSource elements are stored in the resource model.
	// Mirrors okapi's `extractHyperlinkTextSourcesInline` parameter
	// (Parameters.java:67, default false).
	//
	// Independent of this flag, the writer always reconstructs
	// HyperlinkTextSource as an inline element in the Story_*.xml
	// output: bare Content/Br children of the HTS are wrapped in
	// synthetic CharacterStyleRange elements (using HTS's
	// AppliedCharacterStyle, falling back to "[No character style]"
	// when the HTS carries `n`), and any nested ParagraphStyleRange
	// is unwrapped so only its CSR children remain. This mirrors
	// upstream's HyperlinkTextSourceStyledTextReferenceElementParser
	// (StoryChildElementsParser.java:464-569) which ALWAYS routes
	// HTS children through parseFromCharacterStyleRange — the
	// `extractHyperlinkTextSourcesInline` flag only changes the
	// resource-model storage form (referent vs inline), not the
	// serialized XML structure.
	ExtractHyperlinkTextSourcesInline bool
}

// FormatName returns the format identifier.
func (c *Config) FormatName() string { return "idml" }

// Reset restores default configuration values.
//
// Defaults track okapi's IDML filter defaults verbatim
// (Parameters.java::reset, lines 197-203):
// ExtractMasterSpreads=true, ExtractNotes=false,
// SkipDiscretionaryHyphens=false, ExtractHiddenLayers=false,
// ExtractHiddenPasteboardItems=false,
// ExtractHyperlinkTextSourcesInline=false. Matching the reference
// engine out of the box keeps round-trip parity stable.
func (c *Config) Reset() {
	c.ExtractMasterSpreads = true
	c.ExtractNotes = false
	c.SkipDiscretionaryHyphens = false
	c.ExtractHiddenLayers = false
	c.ExtractHiddenPasteboardItems = false
	c.ExtractHyperlinkTextSourcesInline = false
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
		case "extractHiddenLayers":
			c.ExtractHiddenLayers = toBool(val)
		case "extractHiddenPasteboardItems":
			c.ExtractHiddenPasteboardItems = toBool(val)
		case "extractHyperlinkTextSourcesInline":
			c.ExtractHyperlinkTextSourcesInline = toBool(val)
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
