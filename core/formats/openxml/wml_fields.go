// Complex fields in WordprocessingML: the fldChar begin/separate/end
// state machine that spans runs and paragraphs, the field-code policy
// that decides whether a field's display text is translatable, the
// sentinels that carry field markup through a run list, and the run-property
// stripping applied to a field's captured markup.

package openxml

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// complexFieldState tracks the state machine for complex field (fldChar) parsing.
//
// The effective fields (active, fieldCode, extractable, atResult) describe
// the INNERMOST currently-open field — they mirror what upstream Okapi's
// recursive parseComplexField sees at the deepest stack frame. When a
// nested begin is encountered we push the current frame's
// (fieldCode, extractable, atResult) snapshot onto outerFrames and reset the
// effective state for the inner field; on its matching end we pop back to
// the outer frame so the parent field's extraction policy resumes.
//
// Upstream reference: okapi/filters/openxml/.../RunParser.parseComplexField
// (RunParser.java:461-542) — each recursive invocation owns its own
// `extractable` / `atComplexFieldResult` locals, so a nested non-extractable
// field (e.g. TITLE or COMMENTS) cannot leak its result text into the parent
// HYPERLINK's translatable area.
type complexFieldState struct {
	active       bool   // inside a complex field (between begin and end)
	fieldCode    string // field instruction name (e.g., "HYPERLINK", "TOC")
	extractable  bool   // whether the field's display text should be extracted
	atResult     bool   // past the "separate" marker (in display text area)
	nestingLevel int    // nesting depth for nested complex fields

	// outerFrames preserves enclosing-field state (one frame per open
	// outer level) so that on inner-field end we can pop back. Mirrors
	// the per-frame locals of upstream Okapi's recursive
	// parseComplexField.
	outerFrames []complexFieldFrame
}

// complexFieldFrame is the per-level snapshot saved on outerFrames when
// nesting into an inner complex field.
type complexFieldFrame struct {
	fieldCode   string
	extractable bool
	atResult    bool
}

// allFldCharEndOnly reports whether every entry in `runs` carries
// only a fldChar-end marker (U+E108 field sentinel whose captured
// payload contains `w:fldCharType="end"` and no other fldChar /
// instrText). Returns false for empty slices and for sentinels
// carrying non-end content (e.g. a `<w:r><w:rPr><w:rtl/></w:rPr>
// </w:r>` empty placeholder run which is also a U+E108 sentinel but
// represents the field's display area, not its closing marker —
// 830-2.docx P2 / 830-6.docx P2). Used by the cross-paragraph
// field-straddle reabsorption path to distinguish a true
// "fldChar-end-only" paragraph (whose lone fldChar-end can be moved
// back to the prior block — 1172.docx P3, 1341 textbox P2) from a
// placeholder paragraph that should keep its content. Per ECMA-376-1
// §17.16.5.6 (CT_FldChar) the fldCharType attribute discriminates
// begin / separate / end forms; only `end` closes the field.
func allFldCharEndOnly(runs []textRun) bool {
	if len(runs) == 0 {
		return false
	}
	for _, r := range runs {
		if !isFieldSentinel(r.text) {
			return false
		}
		if !strings.Contains(r.data, `w:fldCharType="end"`) {
			return false
		}
		if strings.Contains(r.data, `w:fldCharType="begin"`) ||
			strings.Contains(r.data, `w:fldCharType="separate"`) {
			return false
		}
	}
	return true
}

// complexFieldCodeName extracts the field code name (first word) from instrText content.
// e.g., ` HYPERLINK "http://example.com" \t "_blank" ` → "HYPERLINK"
func complexFieldCodeName(instrText string) string {
	s := strings.TrimSpace(instrText)
	if idx := strings.IndexByte(s, ' '); idx > 0 {
		return s[:idx]
	}
	return s
}

// isExtractableField returns true if the field code is in the configured extract list.
func (p *wmlParser) isExtractableField(fieldCode string) bool {
	for _, prefix := range p.cfg.ComplexFieldDefinitionsToExtract {
		if strings.EqualFold(fieldCode, prefix) {
			return true
		}
	}
	return false
}

// isFieldSentinel reports whether a textRun's text marker indicates
// captured complex-field markup: a <w:r> wrapping fldChar / instrText
// (subtype suffix `fldChar`) or a <w:fldSimple>...</w:fldSimple>
// (subtype suffix `fldSimple`). Carrier sentinel is U+E108. Per
// upstream Okapi (RunParser.parseComplexField, lines 461-542 of
// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/openxml/
// RunParser.java; BlockParser.parse for fldSimple, lines 242-250 of
// BlockParser.java) such markup is preserved as opaque chunks on the
// block irrespective of whether the field code is in
// tsComplexFieldDefinitionsToExtract \u2014 the writer dumps Ph.Data
// verbatim with no <w:r> wrapper because the <w:r> open/close (or
// <w:fldSimple> open/close) is part of the captured payload.
func isFieldSentinel(text string) bool {
	if text == "" {
		return false
	}
	r0, size := utf8.DecodeRuneInString(text)
	if size == 0 {
		return false
	}
	return r0 == '\uE108'
}

// filterFieldRuns is currently a pass-through that documents the run
// shape coming out of parseRunWithFieldState: when a field-marker
// child was seen the returned slice is exactly one SubTypeFieldChar
// sentinel run carrying the raw <w:r>...</w:r> payload; otherwise
// it's a regular slice of translatable text runs. The function exists
// as a future extension point if per-run policy needs to evolve (e.g.
// dropping field markup inside hidden text). At present we always
// keep the captured field markup so it survives the round-trip.
func filterFieldRuns(runs []textRun, _ *complexFieldState) []textRun {
	return runs
}

// startElementToRaw serialises the open form of an xml.StartElement to
// the same raw XML shape captureRawElement uses — prefixed local name,
// attribute pairs in source order, attributes xml-attr-escaped, no
// closing slash. Used by callers of parseRunWithFieldState that need
// to hand the function the raw <w:r ...> open tag so it can rebuild
// the verbatim run payload when field markup is detected inside.
// fieldRPrKeepEmptyMarker is the comment marker emitted inside an
// otherwise-empty `<w:rPr></w:rPr>` captured from a complex-field run
// so the writer's stripWMLSkippableElements pass leaves the wrapper
// in place. Removed by postWML before the document is written to the
// output zip. Per upstream Okapi (RunParser.parseComplexField, lines
// 461-542 of okapi/filters/openxml/src/main/java/net/sf/okapi/filters/
// openxml/RunParser.java) field-bearing runs flow through
// runBuilder.addToMarkup verbatim, bypassing
// RunProperties.Default.getEvents (RunProperties.java line 580) which
// would otherwise collapse the empty rPr — so the emitted shape is
// `<w:r><w:rPr/><w:t>...</w:t></w:r>` rather than the bare
// `<w:r><w:t>...</w:t></w:r>` Okapi emits for non-field runs.
const fieldRPrKeepEmptyMarker = "<!--KAPI-FIELD-RPR-->"

// fieldRPrStripREs are the per-element regexes used by
// stripFieldRPrSkippables to remove run-property children that Okapi
// strips via RunSkippableElements (RunSkippableElements.java lines
// 50-62 of okapi/filters/openxml/src/main/java/net/sf/okapi/filters/
// openxml/RunSkippableElements.java). The complete list per upstream:
//   - <w:lang>            (RUN_PROPERTY_LANGUAGE)
//   - <w:noProof>         (RUN_PROPERTY_NO_SPELLING_OR_GRAMMAR)
//   - <w:rPrChange>       (RUN_PROPERTIES_CHANGE — revision tracking)
//
// Each regex matches both self-closing and open/close forms and
// allows attributes / xmlns declarations on the start tag.
//
// fieldRPrColorBlackRE additionally drops `<w:color w:val="000000"/>` —
// the black foreground color is implicitly injected by upstream Okapi
// into docDefaults' rPr (WordStyleDefinition.DocumentDefaults
// .addExplicitDefaults() at WordStyleDefinition.java:192-227 with
// DEFAULT_FOREGROUND_NAME="windowText" → RGB 000000 per
// Color.java:953). RunProperties.minified() then drops any directly-
// specified `<w:color w:val="000000"/>` via the
// `preCombined.contains(p)` branch (RunProperties.java:504). The
// minified result is what upstream's RunParser.parseRunPropertiesAndRunStyle
// (RunParser.java:280-294) feeds into RunBuilder.setRunProperties for
// EVERY run, including the fldChar / instrText / display-text runs that
// flow through parseComplexField. Native's parseRunWithFieldState
// captures these runs verbatim, bypassing parseRunProps's minification
// path; the equivalent strip has to be applied at the raw-rPr layer
// here so field-bearing runs do not retain redundant black foreground.
//
// Fixture: 830-7.docx — runs surrounding the COMMENTS / HYPERLINK
// extractable field markers carry `<w:color w:val="000000"/>` that
// upstream strips; native otherwise emits the redundant element on
// the field markers. Per ECMA-376-1 §17.3.2.6 (`<w:color>`).
var fieldRPrStripREs = []*regexp.Regexp{
	regexp.MustCompile(`<w:lang\b[^>]*/>|<w:lang\b[^>]*>.*?</w:lang>`),
	regexp.MustCompile(`<w:noProof\b[^>]*/>|<w:noProof\b[^>]*>.*?</w:noProof>`),
	regexp.MustCompile(`<w:rPrChange\b[^>]*/>|<w:rPrChange\b[^>]*>.*?</w:rPrChange>`),
	regexp.MustCompile(`<w:color\b[^>]*\bw:val="000000"[^>]*/>|<w:color\b[^>]*\bw:val="000000"[^>]*>.*?</w:color>`),
}

// fieldRPrEmptyRE matches an `<w:rPr>` that is empty after
// stripFieldRPrSkippables removed every child. Captures the open and
// close tags so the helper can replace the run with the
// fieldRPrKeepEmptyMarker variant.
var fieldRPrEmptyRE = regexp.MustCompile(`<w:rPr>\s*</w:rPr>|<w:rPr\s*/>`)

// isStrippedRPrEmpty reports whether stripFieldRPrSkippables's output
// represents an empty rPr — either the bare `<w:rPr></w:rPr>` /
// `<w:rPr/>` shape OR the keep-empty marker variant
// `<w:rPr><!--KAPI-FIELD-RPR--></w:rPr>` the helper emits when the
// original rPr collapsed to empty after skippable-element stripping.
// Used by the entry-point-run path of parseRunWithFieldState to drop
// the rPr entirely when nothing of substance survives — mirroring
// upstream Okapi's RunProperties.Default.getEvents (line 580 of
// RunProperties.java) which returns no events for empty properties.
func isStrippedRPrEmpty(stripped string) bool {
	if fieldRPrEmptyRE.MatchString(stripped) {
		return true
	}
	return stripped == "<w:rPr>"+fieldRPrKeepEmptyMarker+"</w:rPr>"
}

// protectFieldPayloadFromStripping wraps an opaque field payload (a
// captured <w:fldSimple>...</w:fldSimple> blob, or any future opaque
// field chunk) in element renames so the writer's
// stripWMLSkippableElements pass leaves the payload alone. Per
// upstream Okapi BlockParser.parse
// (lines 242-250 of okapi/filters/openxml/src/main/java/net/sf/okapi/
// filters/openxml/BlockParser.java) the entire <w:fldSimple> element
// is gathered into markup verbatim — so any <w:noProof/> / <w:lang/>
// / <w:rPrChange/> inside it must survive the round-trip with no
// stripping (Document-with-formula-and-tabs.docx is the canonical
// AUTHOR-fldSimple fixture: source has `<w:rPr><w:noProof/></w:rPr>`,
// reference round-trip preserves it). Rename each strippable element's
// open tag (e.g. `w:noProof` → `w:noProofKAPIKEEP`) so the writer's
// stripWMLSkippableElements regex does not match. postWML reverses
// the rename after stripping.
//
// This protect/unprotect dance is the cleanest way to scope a
// document-wide regex strip to "everything except these regions",
// short of refactoring stripWMLSkippableElements to be position-aware
// (which would require an XML parse pass over the full document.xml
// per write, and is overkill for a handful of opaque field payloads).
func protectFieldPayloadFromStripping(payload string) string {
	for _, name := range fieldKeepElementNames {
		// Match `<w:NAME` (open tag, attrs follow) — replace with
		// `<w:NAMEKAPIKEEP`. Match `</w:NAME` (close tag) — same. The
		// body of the element is left untouched. Self-closing forms
		// (`<w:NAME/>`) are also covered by the open-tag rename
		// because the trailing `/>` is part of attribute-territory.
		open := "<w:" + name
		openKeep := "<w:" + name + fieldKeepElementSuffix
		payload = strings.ReplaceAll(payload, open, openKeep)
		closeTag := "</w:" + name + ">"
		closeKeep := "</w:" + name + fieldKeepElementSuffix + ">"
		payload = strings.ReplaceAll(payload, closeTag, closeKeep)
	}
	return payload
}

// fieldKeepElementNames lists the WordprocessingML element local
// names that the writer's stripWMLSkippableElements pass would strip
// from the entire document.xml — protectFieldPayloadFromStripping
// renames each occurrence inside an opaque field payload so the strip
// passes them by. Mirrors stripWMLSkippableElements / wmlNoProofRE /
// wmlStrippableElementRE in writer.go: any element name added there
// also needs to appear here so fldSimple round-trip stays clean.
var fieldKeepElementNames = []string{
	"noProof",
	"lang",
	"bidiVisual",
	"rPrChange",
	"moveToRange",
	"moveFromRange",
	"moveToRangeStart",
	"moveToRangeEnd",
	"moveFromRangeStart",
	"moveFromRangeEnd",
}

// fieldKeepElementSuffix is the rename suffix appended by
// protectFieldPayloadFromStripping. Chosen so the resulting element
// name is well-formed XML, has no chance of colliding with a real
// WordprocessingML element name, and is cheap to scan-and-replace in
// postWML.
const fieldKeepElementSuffix = "KAPIKEEP"

// stripFieldRPrSkippables takes the raw `<w:rPr>...</w:rPr>` blob
// captured from a complex-field run, strips the always-stripped
// children (noProof, lang, rPrChange — the same set
// RunSkippableElements drops upstream), and re-emits the wrapper. If
// the wrapper would collapse to empty, emits
// `<w:rPr>fieldRPrKeepEmptyMarker</w:rPr>` so the writer's empty-
// container regex skips it. Pure string transform — keeps the prefix
// shape (e.g. `w:`) the captureRawElement output uses.
func stripFieldRPrSkippables(rPrXML string) string {
	for _, re := range fieldRPrStripREs {
		rPrXML = re.ReplaceAllString(rPrXML, "")
	}
	if fieldRPrEmptyRE.MatchString(rPrXML) {
		return "<w:rPr>" + fieldRPrKeepEmptyMarker + "</w:rPr>"
	}
	return rPrXML
}
