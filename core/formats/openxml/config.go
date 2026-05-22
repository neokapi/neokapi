package openxml

import "fmt"

// Config holds configuration for the OpenXML format reader/writer.
type Config struct {
	// --- Common extraction toggles ---
	TranslateDocProperties  bool // Extract title, subject, keywords from docProps/core.xml
	TranslateHiddenText     bool // Extract text with vanish property
	TranslateHeadersFooters bool // Extract headers and footers
	TranslateFootnotes      bool // Extract footnotes and endnotes
	TranslateComments       bool // Extract comments
	TranslateHyperlinks     bool // Extract hyperlink text

	// --- Formatting control ---
	AggressiveCleanup bool // Strip rsid*, proofErr, lastRenderedPageBreak before merging
	TabAsCharacter    bool // Treat <w:tab/> as a tab character instead of a placeholder span

	// --- PPTX options ---
	TranslateSlideNotes   bool  // Extract slide notes
	TranslateSlideMasters bool  // Extract slide master text
	TranslateHiddenSlides bool  // Extract hidden slides
	TranslateCharts       bool  // Extract chart strings
	TranslateDiagrams     bool  // Extract diagram data text
	IncludedSlides        []int // If non-empty, only extract these slide numbers (1-based)

	// --- XLSX options ---
	TranslateSheetNames    bool     // Extract sheet names
	TranslateSharedStrings bool     // Extract shared strings
	ExcludedSheets         []string // Sheet names to exclude from extraction
	ExcludedColumns        []string // Column letters to exclude (e.g., "A", "C", "AA")

	// --- Style/color filtering ---
	ExcludeColors          []string // Font colors to exclude (hex RGB, e.g., "FF0000")
	ExcludeHighlightColors []string // Highlight colors to exclude (e.g., "yellow", "red")
	IncludeHighlightColors []string // If non-empty, only extract text with these highlight colors
	ExcludeStyles          []string // Word paragraph/character styles to exclude
	IncludeStyles          []string // If non-empty, only extract text with these styles

	// --- Code finder ---
	UseCodeFinder   bool     // Enable regex-based inline code detection
	CodeFinderRules []string // Regex patterns for inline codes (e.g., `<\/?[a-z]+>`, `\{[0-9]+\}`)

	// --- Complex fields ---
	ComplexFieldDefinitionsToExtract []string // Field instruction prefixes to extract (e.g., "HYPERLINK", "REF")

	// --- Style optimization ---
	OptimiseWordStyles bool // Resolve style inheritance and strip redundant run properties

	// --- Font mappings ---
	FontMappings map[string]string // Font name → script group (e.g., "MS Gothic": "ja")

	// --- Media extraction (Bowrain AD-007) ---
	ExtractMedia bool // Emit PartMedia parts for embedded images/objects from word/media/

	// --- Advanced ---
	ExtractRunFontsInfo      bool   // Emit font metadata as annotations on blocks
	ReplaceLineSeparator     bool   // Replace Unicode line separator (U+2028) in output
	LineSeparatorReplacement string // Replacement string for line separator (default: "\n")

	// --- General options (from bridge) ---
	IgnoreSoftHyphenTag          bool // Ignore soft hyphen tags
	ReplaceNoBreakHyphenTag      bool // Replace no-break hyphen tags with non-breaking hyphen character
	AutomaticallyAcceptRevisions bool // Automatically accept tracked changes before extraction
}

// FormatName returns the format identifier.
func (c *Config) FormatName() string { return "openxml" }

// Reset restores default configuration values.
func (c *Config) Reset() {
	c.TranslateDocProperties = true
	c.TranslateHiddenText = false
	c.TranslateHeadersFooters = true
	c.TranslateFootnotes = true
	// Okapi's ConditionalParameters.reset() (ConditionalParameters.java
	// line 781) defaults this to true:
	//   setTranslateComments(true); // Word, Excel Comments
	// Comments are part of the StyledTextPart family in upstream Okapi —
	// WordDocument.isStyledTextPart() (WordDocument.java lines 192-206)
	// returns true for Word.COMMENTS_TYPE alongside MAIN_DOCUMENT_TYPE,
	// FOOTNOTES_TYPE, and ENDNOTES_TYPE, so word/comments.xml flows
	// through the same translatable-block pipeline as document.xml.
	c.TranslateComments = true
	c.TranslateHyperlinks = true
	c.AggressiveCleanup = true
	c.TabAsCharacter = false
	c.TranslateSlideNotes = true
	c.TranslateSlideMasters = false
	c.TranslateHiddenSlides = false
	c.TranslateCharts = false
	c.TranslateDiagrams = false
	c.IncludedSlides = nil
	c.TranslateSheetNames = false
	c.TranslateSharedStrings = true
	c.ExcludedSheets = nil
	c.ExcludedColumns = nil
	c.ExcludeColors = nil
	c.ExcludeHighlightColors = nil
	c.IncludeHighlightColors = nil
	c.ExcludeStyles = nil
	c.IncludeStyles = nil
	c.UseCodeFinder = false
	c.CodeFinderRules = nil
	// Okapi defaults the extract list to {"HYPERLINK"} — see
	// ConditionalParameters.java line 826-827 of okapi/filters/openxml/
	// src/main/java/net/sf/okapi/filters/openxml/ConditionalParameters.
	// java:
	//   tsComplexFieldDefinitionsToExtract = new TreeSet<>();
	//   tsComplexFieldDefinitionsToExtract.add("HYPERLINK");
	// HYPERLINK fields carry user-visible cached display text that
	// represents the link's anchor in the source language; without it
	// in the extract set the display text is dropped from translation
	// and the HYPERLINK field round-trips with the source-language
	// anchor still present (a real semantic divergence on every
	// HYPERLINK fixture, e.g. 768.docx).
	c.ComplexFieldDefinitionsToExtract = []string{"HYPERLINK"}
	// Native is FAITHFUL by default: source rPr is preserved inline, no
	// synthesised paragraph styles, no attribute stripping. Word Style
	// Optimisation (WSO) is a clean, opt-in post-pass that mimics Okapi's
	// compact output (synthesising `NF…-Normal` pStyles from common run
	// properties, stripping "moot" attributes, renaming font subsets) —
	// nothing ECMA-376 / ISO/IEC 29500 requires. The faithful pre-WSO
	// output (`renderWMLBlock` + `postNonWSOForName`) is already a valid
	// OOXML producer that renders identically; Okapi's compact form and
	// the faithful inline form are equally spec-valid (§17.3.2.1 CT_R +
	// §17.7 style resolution — direct and style-based formatting resolve
	// to the same effective formatting).
	//
	// Defaulting WSO off (a) closes #597 (no rPr rewrite → source
	// `<w:spacing>` is preserved; no docDefaults `rFonts` overlay → no
	// injected `<w:rFonts w:cs="Helvetica"/>`) and #598b (no attr strip →
	// source `<w:color>` preserved), and (b) removes WSO's global,
	// order-coupled synth-style ID counter (the 847-3 "architectural
	// blocker"). Equivalence with Okapi's synth-pStyle form is proved in
	// the parity comparator by an effective-rPr normalizer that resolves
	// the style indirection on both sides (cli/parity/roundtrip), not in
	// the writer. This mirrors the established xliff "faithful default +
	// opt-in okapi-compat" pattern. See the OpenXML faithful-writer design
	// note (docs/internals/research/openxml-faithful-writer-design.md).
	//
	// Okapi's AllowWordStyleOptimisation parameter defaults to true
	// (upstream ConditionalParameters.java line 813); we deliberately
	// diverge from that default here because byte-matching Okapi is the
	// comparator's job, not the producer's.
	c.OptimiseWordStyles = false
	c.FontMappings = nil
	c.ExtractRunFontsInfo = false
	c.ReplaceLineSeparator = false
	c.LineSeparatorReplacement = "\n"
	c.IgnoreSoftHyphenTag = false
	c.ReplaceNoBreakHyphenTag = false
	c.AutomaticallyAcceptRevisions = true
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		// Boolean options
		case "translateDocProperties":
			c.TranslateDocProperties = toBool(val)
		case "translateHiddenText":
			c.TranslateHiddenText = toBool(val)
		case "translateHeadersFooters":
			c.TranslateHeadersFooters = toBool(val)
		case "translateFootnotes":
			c.TranslateFootnotes = toBool(val)
		case "translateComments":
			c.TranslateComments = toBool(val)
		case "translateHyperlinks":
			c.TranslateHyperlinks = toBool(val)
		case "aggressiveCleanup":
			c.AggressiveCleanup = toBool(val)
		case "tabAsCharacter":
			c.TabAsCharacter = toBool(val)
		case "translateSlideNotes":
			c.TranslateSlideNotes = toBool(val)
		case "translateSlideMasters":
			c.TranslateSlideMasters = toBool(val)
		case "translateHiddenSlides":
			c.TranslateHiddenSlides = toBool(val)
		case "translateCharts":
			c.TranslateCharts = toBool(val)
		case "translateDiagrams":
			c.TranslateDiagrams = toBool(val)
		case "translateSheetNames":
			c.TranslateSheetNames = toBool(val)
		case "translateSharedStrings":
			c.TranslateSharedStrings = toBool(val)
		case "useCodeFinder":
			c.UseCodeFinder = toBool(val)
		case "optimiseWordStyles":
			c.OptimiseWordStyles = toBool(val)
		case "extractMedia":
			c.ExtractMedia = toBool(val)
		case "extractRunFontsInfo":
			c.ExtractRunFontsInfo = toBool(val)
		case "replaceLineSeparator":
			c.ReplaceLineSeparator = toBool(val)
		case "ignoreSoftHyphenTag":
			c.IgnoreSoftHyphenTag = toBool(val)
		case "replaceNoBreakHyphenTag":
			c.ReplaceNoBreakHyphenTag = toBool(val)
		case "automaticallyAcceptRevisions":
			c.AutomaticallyAcceptRevisions = toBool(val)

		// String options
		case "lineSeparatorReplacement":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("openxml: config key %q expects string, got %T", key, val)
			}
			c.LineSeparatorReplacement = s

		// String list options
		case "excludedSheets":
			list, err := toStringSlice(key, val)
			if err != nil {
				return err
			}
			c.ExcludedSheets = list
		case "excludedColumns":
			list, err := toStringSlice(key, val)
			if err != nil {
				return err
			}
			c.ExcludedColumns = list
		case "excludeColors":
			list, err := toStringSlice(key, val)
			if err != nil {
				return err
			}
			c.ExcludeColors = list
		case "excludeHighlightColors":
			list, err := toStringSlice(key, val)
			if err != nil {
				return err
			}
			c.ExcludeHighlightColors = list
		case "includeHighlightColors":
			list, err := toStringSlice(key, val)
			if err != nil {
				return err
			}
			c.IncludeHighlightColors = list
		case "excludeStyles":
			list, err := toStringSlice(key, val)
			if err != nil {
				return err
			}
			c.ExcludeStyles = list
		case "includeStyles":
			list, err := toStringSlice(key, val)
			if err != nil {
				return err
			}
			c.IncludeStyles = list

		case "codeFinderRules":
			list, err := toStringSlice(key, val)
			if err != nil {
				return err
			}
			c.CodeFinderRules = list
		case "complexFieldDefinitionsToExtract":
			list, err := toStringSlice(key, val)
			if err != nil {
				return err
			}
			c.ComplexFieldDefinitionsToExtract = list
		case "fontMappings":
			m, err := toStringMap(key, val)
			if err != nil {
				return err
			}
			c.FontMappings = m

		// Int list options
		case "includedSlides":
			list, err := toIntSlice(key, val)
			if err != nil {
				return err
			}
			c.IncludedSlides = list

		default:
			return fmt.Errorf("openxml: unknown config key %q", key)
		}
	}
	return nil
}

// toBool converts a value to bool, accepting bool and string representations.
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

// toStringSlice converts a value to []string.
func toStringSlice(key string, val any) ([]string, error) {
	switch v := val.(type) {
	case []string:
		return v, nil
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("openxml: config key %q: list item expects string, got %T", key, item)
			}
			result = append(result, s)
		}
		return result, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("openxml: config key %q expects string list, got %T", key, val)
	}
}

// toStringMap converts a value to map[string]string.
func toStringMap(key string, val any) (map[string]string, error) {
	switch v := val.(type) {
	case map[string]string:
		return v, nil
	case map[string]any:
		result := make(map[string]string, len(v))
		for k, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("openxml: config key %q: map value for %q expects string, got %T", key, k, item)
			}
			result[k] = s
		}
		return result, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("openxml: config key %q expects string map, got %T", key, val)
	}
}

// toIntSlice converts a value to []int.
func toIntSlice(key string, val any) ([]int, error) {
	switch v := val.(type) {
	case []int:
		return v, nil
	case []any:
		result := make([]int, 0, len(v))
		for _, item := range v {
			switch n := item.(type) {
			case int:
				result = append(result, n)
			case float64:
				result = append(result, int(n))
			default:
				return nil, fmt.Errorf("openxml: config key %q: list item expects int, got %T", key, item)
			}
		}
		return result, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("openxml: config key %q expects int list, got %T", key, val)
	}
}
