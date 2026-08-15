// Operations over a parsed run list: run-property serialisation, merging
// adjacent runs, classifying sentinel and opaque runs, and writing runs back
// out as WordprocessingML.

package openxml

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/internal/xmlesc"
	"github.com/neokapi/neokapi/core/model"
)

// serializeRPrChildrenXML returns a `<w:rPr>...</w:rPr>` fragment for
// the run's non-toggle rPr children (rStyle, color, sz, etc.). Used by
// the footnote/endnote reference Ph emission so the marker travels with
// its per-run rPr inside the same <w:r>. Returns "" when the run has no
// rPrChildren — callers fall back to wrapping the marker in a bare <w:r>.
func serializeRPrChildrenXML(p runProps) string {
	if len(p.rPrChildren) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<w:rPr>")
	for _, c := range p.rPrChildren {
		b.WriteString(c.xml)
	}
	b.WriteString("</w:rPr>")
	return b.String()
}

// serializeFullRPrXML returns a `<w:rPr>...</w:rPr>` fragment combining
// every preserved property of the run: toggle elements (b/i/u/strike/
// vertAlign/vanish — bare-on form, mirroring source authoring) AND the
// non-toggle rPrChildren (rStyle, color, sz, lang, noProof, …). Returns
// "" when the run has neither. Used by the image-sentinel emission path
// (TypeImage Ph) so a drawing-bearing run carries its source <w:r>'s
// own rPr through the writer instead of relying on the paragraph-wide
// sourceRPr fallback (which the writer's TypeImage handler does not
// consult). 859.docx is the canonical fixture: the drawing's source
// run carries `<w:rPr><w:noProof/><w:lang w:eastAsia="ru-RU"/></w:rPr>`
// (both children preserved by the Strict-OOXML namespace gates on
// `lang`/`noProof` in runprops.go), and that rPr must round-trip on
// the wire alongside `<w:drawing>`.
//
// Per ECMA-376-1 §17.3.2.1 (CT_R) `<w:rPr>` is the first child of `<w:r>`,
// preceding `<w:drawing>` / `<w:pict>` / `<w:object>` and any other run
// children. Mirrors upstream Okapi's RunBuilder, which materialises the
// source RunProperties on every emitted run regardless of whether the
// run carries text or only an opaque drawing chunk.
func serializeFullRPrXML(p runProps) string {
	if p.isEmpty() && len(p.rPrChildren) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<w:rPr>")
	if p.bold {
		b.WriteString(boldOnXML(p))
	}
	if p.italic {
		b.WriteString(italicOnXML(p))
	}
	if p.underline != "" {
		b.WriteString(`<w:u w:val="` + p.underline + `"/>`)
	}
	if p.strike {
		b.WriteString("<w:strike/>")
	}
	if p.vertAlign != "" {
		b.WriteString(`<w:vertAlign w:val="` + p.vertAlign + `"/>`)
	}
	if p.vanish {
		b.WriteString(vanishOnXML(p))
	}
	for _, c := range p.rPrChildren {
		b.WriteString(c.xml)
	}
	b.WriteString("</w:rPr>")
	return b.String()
}

// mergeRuns merges adjacent runs whose rPr can be merged per upstream
// Okapi RunMerger.canRunPropertiesBeMerged (RunMerger.java:156-229).
//
// Two runs are mergeable when (a) toggles + fontName match (runProps.equal)
// AND (b) every non-rFonts rPr child is byte-equal AND (c) rFonts is
// per-attribute compatible (no contradictory values for shared
// attribute names — RunFonts.canBeMerged at RunFonts.java:190-247).
// When the rFonts differ but are compatible (e.g. one run carries
// rFonts ascii/hAnsi/cs all "Arial" and the next carries rFonts
// ascii/cs both "Arial" but no hAnsi), the merged run carries the
// per-attribute union via mergeRPrChildren — mirroring RunFonts.merge
// (RunFonts.java:267-288).
//
// Per ECMA-376-1 §17.3.2.1 (CT_R) and §17.3.2.26 (CT_Fonts), adjacent
// runs with equivalent rPr are semantically a single run; upstream
// RunMerger fuses them on the way to the writer so the corpus
// reference for 1411-mergable-runs.docx emits one <w:r> rather than
// three.
//
// The kept run's rPr (toggles + rPrChildren) is updated to the merged
// rPr so the per-source-run rPr sidecar — computed AFTER mergeRuns
// over the merged slice — sees the merged props and stays aligned 1:1
// with the model.Run population the writer emits.
func mergeRuns(runs []textRun) []textRun {
	if len(runs) <= 1 {
		return runs
	}

	var merged []textRun
	current := runs[0]

	for i := 1; i < len(runs); i++ {
		r := runs[i]
		// Don't merge sentinel markers or line breaks
		if isSentinel(current.text) || isSentinel(r.text) ||
			current.text == "\n" || r.text == "\n" {
			merged = append(merged, current)
			current = r
			continue
		}
		// Refuse to merge across the boundary of an extractable
		// complex field's display text. Upstream Okapi captures each
		// source <w:r> of that region as its own RunText body chunk
		// (parseContent at RunParser.java:537 + parseText at lines
		// 820-836) inside the field's single RunBuilder, with Markup
		// body chunks preserving the source </w:r><w:r> boundaries
		// between them — those runs do NOT pass through
		// RunMerger.canMergeWith so they emerge as separate <w:r>
		// envelopes in the output. Fixtures
		// 1083-empty-and-hyperlink-instructions.docx (and siblings)
		// rely on this for the " " + "with" sequence inside their
		// HYPERLINK field's display area. Per ECMA-376-1 §17.16.5
		// (Complex Fields) the field's display text retains the
		// source's run grouping.
		if r.inFieldDisplay && r.srcRunStart {
			merged = append(merged, current)
			current = r
			continue
		}
		if current.props.canBeMergedWithTexts(r.props, current.text, r.text) {
			oldText := current.text
			current.text += r.text
			// Replace the kept run's rPrChildren with the merged
			// per-attribute union of rFonts so downstream sidecars
			// (perRunRPrFragments) see the consensus rFonts. Use the
			// text-aware variant so a whitespace-only side defers to
			// the detected side's rFonts — mirrors upstream Okapi
			// RunFonts.merge (RunFonts.java:267-315) where the
			// detected content category's value wins.
			if !current.props.equalIncludingChildren(r.props) {
				current.props.rPrChildren = mergeRPrChildrenTexts(
					current.props.rPrChildren, r.props.rPrChildren,
					oldText, r.text)
			}
		} else {
			merged = append(merged, current)
			current = r
		}
	}
	merged = append(merged, current)
	return merged
}

// isSentinel returns true if the text is a special marker.
func isSentinel(s string) bool {
	r0, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return false
	}
	// Sentinel range covers all reserved Private Use Area code points
	// used by the WML reader: \uE100 (tab) through \uE10F (revision-
	// insertion close). Extending the range past \uE10D requires the
	// matching dispatch in buildBlock \u2014 see the \uE10E / \uE10F
	// (revision-insertion paired-code OPEN/CLOSE) cases there.
	if r0 < '\uE100' || r0 > '\uE10F' {
		return false
	}
	// Single-char sentinels (tab \uE100, image \uE101, paragraph
	// opaque \uE105). Note: \uE105 wraps math (m:oMathPara/m:oMath)
	// or paragraph-level mc:AlternateContent \u2014 content that is a
	// direct <w:p> child rather than a <w:r> child, so the writer
	// must not wrap it in <w:r> when re-emitting.
	rest := s[size:]
	if rest == "" {
		return true
	}
	// Multi-char sentinels must have ':' separator
	// (\uE102:id, \uE103:data, \uE104:data, \uE106:id, \uE107:id,
	// \uE108:fldChar / \uE108:fldSimple, \uE109:data, \uE10A:data,
	// \uE10B:id, \uE10C:id, \uE10D:rawXML, \uE10E:wrapper:rawStart,
	// \uE10F:wrapper:rawEnd)
	r1, _ := utf8.DecodeRuneInString(rest)
	return r1 == ':'
}

// dropTextRuns removes plain translatable runs from a slice while
// keeping every sentinel run (field markup, drawings, bookmarks, \u2026).
// Mirrors upstream Okapi's parseComplexField branching at lines 501-
// 506 of RunParser.java where, when the field is non-extractable or
// the reader is still before the separator, content events are routed
// to runBuilder.addToMarkup (preserved as opaque markup) rather than
// to the run text. Translatable text alongside the field markup never
// reaches the block, but the field markup itself does.
//
// Exception: a textRun tagged preFieldBody is translatable body content
// authored in the SAME source `<w:r>` BEFORE the begin marker that opened
// the field (the field was inactive when the run started). Upstream Okapi
// keeps this as a RunText body chunk of the field-opening run
// (RunParser.java:259, 537) \u2014 it is NOT inside the suppressed
// begin\u2192separate window \u2014 so dropTextRuns retains it. See
// textRun.preFieldBody and the 830-7.docx fixture rationale.
func dropTextRuns(runs []textRun) []textRun {
	out := runs[:0]
	for _, r := range runs {
		if isSentinel(r.text) || r.preFieldBody {
			out = append(out, r)
		}
	}
	return out
}

// isCommentRangeSentinel reports whether a textRun's text marker
// indicates a captured `<w:commentRangeStart>` (\uE10B) or
// `<w:commentRangeEnd>` (\uE10C). Like bookmarks, comment-range
// markers are direct children of `<w:p>` per ECMA-376 Part 1
// \u00A717.13.4.3 / \u00A717.13.4.4 (CT_MarkupRangeStart /
// CT_MarkupRange), so the writer must NOT wrap the captured XML
// in `<w:r>...</w:r>`.
func isCommentRangeSentinel(text string) bool {
	if text == "" {
		return false
	}
	r0, size := utf8.DecodeRuneInString(text)
	if size == 0 {
		return false
	}
	return r0 == '\uE10B' || r0 == '\uE10C'
}

// isBookmarkSentinel reports whether a textRun's text marker
// indicates a captured `<w:bookmarkStart>` (\uE106) or
// `<w:bookmarkEnd>` (\uE107). Bookmarks are direct children of
// `<w:p>` per ECMA-376 \u00A717.13.6, NOT children of `<w:r>`, so the
// writer must NOT wrap the captured XML in `<w:r>...</w:r>`.
func isBookmarkSentinel(text string) bool {
	if text == "" {
		return false
	}
	r0, size := utf8.DecodeRuneInString(text)
	if size == 0 {
		return false
	}
	return r0 == '' || r0 == ''
}

// isDrawingSentinel reports whether a textRun's text marker
// indicates an opaque drawing/pict/object/AlternateContent payload
// (run-level "" or paragraph-level ""). Used by
// parseParagraph to scope drawing-XML pre-extraction to the runs
// that actually carry captured payloads.
func isDrawingSentinel(text string) bool {
	if text == "" {
		return false
	}
	r0, size := utf8.DecodeRuneInString(text)
	if size == 0 {
		return false
	}
	return r0 == '' || r0 == ''
}

// isEmptyRuns returns true if all runs have no visible text content.
func isEmptyRuns(runs []textRun) bool {
	for _, r := range runs {
		if isSentinel(r.text) {
			continue
		}
		if strings.TrimSpace(r.text) != "" {
			return false
		}
	}
	return true
}

// allHidden returns true if all runs have the vanish property — either
// directly on the run's rPr OR inherited via inheritedVanish from the
// paragraph style chain. Mirrors upstream Okapi's
// `RunPropertyHidden.containsRunPropertyHidden(combinedRunProperties)`
// pattern (Block.java / RunBuilder), where an inherited <w:vanish/> from
// the paragraph's pStyle marks every run in the paragraph as hidden
// regardless of the run's own rPr.
//
// inheritedVanish lets the caller signal that the paragraph-style
// chain (resolved via styleMap.resolveProps) has <w:vanish/> set —
// required so a paragraph whose vanish travels via pStyle (e.g.
// PageBreak.docx after WSO promotes <w:vanish/> into a synthesised
// Standard1 pStyle) still gets skipped by the hidden-text filter on
// re-read. Callers without style context pass false.
//
// Vanish-clear semantics: a run carrying an explicit
// `<w:vanish w:val="0"/>` (runProps.vanishExplicit && !runProps.vanish)
// CLEARS any inherited vanish — that run is visible. Per ECMA-376-1
// §17.3.2.45 (CT_OnOff) the closer (more specific) authoring level
// wins. Mirrors upstream Okapi RunParser.clarifyVisibility
// (RunParser.java:310-316) where the run's direct vanish overrides
// inheritance. Without this, paragraph 18 of HiddenExcluded.docx
// (`<w:pPr><w:pStyle w:val="FranzJosef"/></w:pPr>` — FranzJosef
// has vanish — `<w:r><w:rPr><w:vanish w:val="0"/></w:rPr><w:t>…</w:t>`)
// would be incorrectly filtered as wholly hidden, when in fact the
// run's clear-override makes it visible and the paragraph must emit a
// translatable Block.
func allHidden(runs []textRun, inheritedVanish bool) bool {
	for _, r := range runs {
		if isSentinel(r.text) {
			continue
		}
		if strings.TrimSpace(r.text) == "" {
			continue
		}
		// Run with an explicit vanish-clear overrides paragraph-style
		// inheritance — visible.
		if r.props.vanishExplicit && !r.props.vanish {
			return false
		}
		if !r.props.vanish && !inheritedVanish {
			return false
		}
	}
	return true
}

// runToXML converts a text run back to XML for skeleton output. The
// run is wrapped in <w:r>...</w:r>; the body is either an opaque
// payload (drawing, pict, AlternateContent — preserved verbatim from
// run.data) or a <w:t> text element. Empty drawings (no captured data)
// fall back to a self-closing <w:drawing/>.
func runToXML(r textRun) string {
	// Paragraph-level opaque sentinel (\uE105): emit captured raw
	// XML directly with no <w:r> wrapper. Used for math (m:oMathPara,
	// m:oMath) and paragraph-level mc:AlternateContent that appear
	// as direct children of <w:p>.
	if strings.HasPrefix(r.text, "\uE105") {
		if r.data != "" {
			return r.data
		}
		return ""
	}
	// Bookmark sentinels (\uE106 / \uE107) \u2014 emit the captured raw
	// XML verbatim with no <w:r> wrapper. ECMA-376 Part 1
	// \u00A717.13.6.1 / \u00A717.13.6.2 specify <w:bookmarkStart> /
	// <w:bookmarkEnd> as direct children of <w:p>, not <w:r>.
	if isBookmarkSentinel(r.text) {
		return r.data
	}
	// Comment-range sentinels ( / ) — same shape as
	// bookmarks (paragraph-level direct child, no <w:r> wrapper).
	// Per ECMA-376 Part 1 §17.13.4.3 / §17.13.4.4.
	if isCommentRangeSentinel(r.text) {
		return r.data
	}
	// Field-markup sentinel (\uE108) \u2014 captured payload already
	// carries the full <w:r>...</w:r> (for fldChar / instrText) or
	// <w:fldSimple>...</w:fldSimple> wrapper, so emit verbatim with
	// no additional wrapping. Mirrors the bookmark path above.
	if isFieldSentinel(r.text) {
		return r.data
	}
	// Generic paired-code wrapper sentinels (\uE10E open / \uE10F close)
	// — used for strict-OOXML <w:ins>/<w:moveTo> revision wrappers
	// (TypeRevisionIns, ECMA-376-1 §17.13.5.16) and inline <w:sdt>
	// envelopes (TypeSDT, ECMA-376-1 §17.5.2). The captured payload
	// after the "<sentinel>:<localName>:" prefix is a complete XML
	// chunk (start tag for OPEN, end tag(s) for CLOSE) that's emitted
	// verbatim with no <w:r> wrapper — the wrapper is the SDT/ins
	// envelope itself, not a run. Used by writeRunToSkel for empty-
	// runs paragraphs (e.g. the 1085.docx
	// <w:p><w:sdt>...</w:sdt></w:p>).
	if strings.HasPrefix(r.text, "\uE10E:") || strings.HasPrefix(r.text, "\uE10F:") {
		rest := r.text[len("\uE10E:"):] // drop sentinel + ':' (both are 1+1 chars)
		_, data, _ := strings.Cut(rest, ":")
		return data
	}
	var buf strings.Builder
	buf.WriteString("<w:r>")
	// Emit BOTH the toggle properties AND the non-toggle rPrChildren
	// (rStyle, color, sz, szCs, lang, noProof, …). Previously this
	// path only emitted toggles, dropping rStyle and other non-toggle
	// children on whitespace-only / empty-text runs that route through
	// the skeleton emit path (parseParagraph isEmptyRuns branch),
	// losing distinctive formatting (e.g. lang.docx's editform-styled
	// space run: source rPr `<w:rStyle w:val="editform"/><w:b/>
	// <w:vanish w:val="0"/>...` was being stripped to just `<w:b/>`
	// here). Per ECMA-376-1 §17.3.2.1 (CT_R) every rPr child applies to
	// the run regardless of the run's payload (text vs whitespace vs
	// drawing); upstream Okapi RunBuilder materialises the full source
	// RunProperties on every emitted run.
	buf.WriteString(serializeFullRPrXML(r.props))
	switch {
	case strings.HasPrefix(r.text, ""):
		// drawing/pict/object/AlternateContent — emit captured raw XML
		if r.data != "" {
			buf.WriteString(r.data)
		} else {
			buf.WriteString("<w:drawing/>")
		}
	case r.text == "":
		buf.WriteString("<w:tab/>")
	case r.text == "\n":
		// Prefer the captured br element (r.data) so any
		// w:type="page" / w:type="column" / w:clear attribute
		// survives the round-trip. Per ECMA-376-1 §17.3.3.1
		// (CT_Br) the type attribute distinguishes textWrap,
		// page, and column break semantics.
		if r.data != "" {
			buf.WriteString(r.data)
		} else {
			buf.WriteString("<w:br/>")
		}
	case strings.HasPrefix(r.text, ":"):
		rest := strings.TrimPrefix(r.text, ":")
		markerElem := "footnoteReference"
		if after, ok := strings.CutPrefix(rest, "f:"); ok {
			rest = after
		} else if after, ok := strings.CutPrefix(rest, "e:"); ok {
			rest = after
			markerElem = "endnoteReference"
		}
		buf.WriteString(fmt.Sprintf(`<w:%s w:id="%s"/>`, markerElem, rest))
	default:
		buf.WriteString(`<w:t xml:space="preserve">`)
		buf.WriteString(xmlesc.Text(r.text))
		buf.WriteString("</w:t>")
	}
	buf.WriteString("</w:r>")
	return buf.String()
}

// writeRunToSkel emits a textRun directly into the skeleton stream.
// Mostly delegates to runToXML, but for opaque drawing/pict/object/
// AlternateContent payloads (sentinel "" or paragraph-level
// ""), it scans the captured XML for translatable name=
// attributes on <wp:docPr> / <pic:cNvPr> / <wps:cNvPr> elements and
// emits a separate "property" Block per match — interleaving the raw
// XML between attribute-value substitution points and skeleton refs
// to those blocks. This mirrors Okapi's
// RunParser.processTranslatableAttributes (line ~838 of
// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/openxml/
// RunParser.java) which extracts wp:docPr/@name when
// ConditionalParameters.getTranslateWordGraphicName() is true (the
// default). Without this extraction, drawings round-trip with the
// source-language object name still present (e.g. "Bild 1") while
// Okapi would have translated it ("ßĩĺď 1" under pseudo-translation),
// producing structural-but-semantic divergence.
func (p *wmlParser) writeRunToSkel(r textRun, partPath string, emitBlock func(*model.Block)) {
	// A paragraph-direct OMML equation (captured as a paragraph-opaque sentinel
	// run, sentinelParaOpaque + the raw <m:oMath…> payload): when
	// non-translatable-content surfacing is on, write it as a sub-skeleton so any
	// embedded <m:nor/> prose is translatable and splices back on write (no-op /
	// byte-exact when nothing is translated). Falls through to the verbatim path
	// when there is no prose.
	if p.cfg != nil && p.cfg.ExtractNonTranslatableContent() &&
		strings.HasPrefix(r.text, sentinelParaOpaque) && strings.HasPrefix(r.data, "<m:oMath") {
		if p.writeOMathSubSkeleton(r.data, emitBlock) {
			return
		}
	}
	// For opaque sentinel runs with captured data, do attribute
	// extraction. Otherwise, fall back to the simple runToXML path.
	isOpaque := strings.HasPrefix(r.text, "") || strings.HasPrefix(r.text, "")
	if !isOpaque || r.data == "" {
		p.skelText(runToXML(r))
		return
	}

	// Wrap opaque payload in <w:r>...</w:r> for run-level sentinels;
	// paragraph-level sentinels () carry no <w:r> wrapper.
	wrap := strings.HasPrefix(r.text, "")
	if wrap {
		// Emit the run open tag (with rPr if needed) via runToXML on
		// a stripped variant — simpler to construct a synthetic
		// run with empty data and slice the inner.
		open, close := splitRunWrapper(r)
		p.skelText(open)
		p.writeDrawingXMLToSkel(r.data, partPath, emitBlock)
		p.skelText(close)
		return
	}
	p.writeDrawingXMLToSkel(r.data, partPath, emitBlock)
}

// opaqueRunKind returns the local element name of the first opening
// tag in an opaque-drawing payload — "w:drawing", "w:pict",
// "w:object", "mc:AlternateContent", etc. Used by the drawing-fusion
// path to refuse merging adjacent same-rPr opaque runs whose inner
// element kinds differ. Per ECMA-376-1 §17.3.2.1 (CT_R), a single
// `<w:r>` may host repeats of one opaque-element kind but mixing
// kinds is not what upstream Okapi RunMerger emits — its
// MarkupComponent merge groups by component kind.
//
// Returns "" for payloads that do not begin with a recognised
// opaque-element open tag (e.g. payload starts with rPr or text).
// In practice the data passed in here is captured raw XML produced
// by captureRawElement / captureAlternateContent, which always
// starts with the wrapping element's open tag.
func opaqueRunKind(data string) string {
	if data == "" {
		return ""
	}
	if data[0] != '<' {
		return ""
	}
	end := strings.IndexAny(data[1:], " >/\t\n\r")
	if end < 0 {
		return ""
	}
	return data[1 : 1+end]
}

// isEmptyTextPlaceholder reports whether r is a content-bearing
// run carrying an empty `<w:t></w:t>` body and no surviving rPr
// children — the trivially-empty placeholder `<w:r><w:t/></w:r>`
// shape that upstream Okapi RunMerger discards before flushBuilders
// (RunMerger.java:83-95: a RunBuilder whose only chunk is an empty
// Text contributes nothing to the merged paragraph). Used by
// parseParagraph's isEmptyRuns skeleton-emit path to filter out
// trailing empty placeholders that sit alongside drawing-bearing
// runs in an otherwise-content-empty paragraph (AlternateContent.docx
// canonical case: each AC-bearing paragraph ends with `<w:r><w:t
// xml:space="preserve"></w:t></w:r>` after the drawings).
//
// Sentinel runs (drawings, fields, breaks, tabs) keep their full
// shape — only the text-payload empty form is dropped.
func isEmptyTextPlaceholder(r textRun) bool {
	if r.text != "" {
		return false
	}
	if isSentinel(r.text) {
		return false
	}
	if r.data != "" {
		return false
	}
	if len(r.props.rPrChildren) > 0 {
		return false
	}
	if r.props.bold || r.props.italic || r.props.strike ||
		r.props.vanish || r.props.underline != "" ||
		r.props.vertAlign != "" || r.props.fontName != "" ||
		r.props.boldClear || r.props.italicClear || r.props.strikeClear {
		return false
	}
	return true
}

// isRunLevelOpaque reports whether r is a run-level opaque sentinel
// (`` carrier) with a captured XML payload — i.e. a drawing,
// pict, object, mc:AlternateContent, or ruby element extracted by
// parseRun. Used by the same-source-`<w:r>` grouping path in
// parseParagraph to splice multiple opaque body children of one source
// `<w:r>` back under a single envelope (992.docx canonical case:
// `<w:r><mc:AlternateContent/><w:drawing/></w:r>`). Paragraph-level
// opaque sentinels (`` for math / paragraph-level
// mc:AlternateContent) are excluded — those are direct `<w:p>`
// children and never share a `<w:r>` envelope.
func isRunLevelOpaque(r textRun) bool {
	if !strings.HasPrefix(r.text, "") {
		return false
	}
	return r.data != ""
}

// isFusableDrawingRun reports whether r is an opaque drawing-bearing
// sentinel run (`<w:pict>`, `<w:drawing>`, `<w:object>`,
// `<w:AlternateContent>`, `<w:ruby>`, …) that the parser captured as
// raw XML and can be fused with adjacent same-rPr drawing runs into
// one `<w:r>` envelope on emit.
//
// Opaque-drawing sentinels carry text "" ( — the run-level
// drawing carrier set by parseRunWithFieldState's drawing branch).
// Paragraph-level drawings ("" / ) and other sentinels
// don't fuse — only run-level drawings share the same `<w:r>`
// container semantics under ECMA-376-1 §17.3.2.1 (CT_R).
//
// Used by parseParagraph's isEmptyRuns skeleton-emit path to coalesce
// neverendingloop.docx-style adjacent `<w:r><w:pict>...</w:pict></w:r>`
// envelopes — see commit message at the call site.
func isFusableDrawingRun(r textRun) bool {
	if !strings.HasPrefix(r.text, "") {
		return false
	}
	if r.data == "" {
		return false
	}
	// Drawing payloads that carry translatable content
	// (`<w:txbxContent>` — textbox body paragraphs that the reader
	// extracted as separate Blocks via extractTxbxContent) do NOT fuse:
	// upstream Okapi keeps the wrapping `<w:r>` per source pict so the
	// textbox's per-run markup boundary survives the round-trip.
	// Practice2.docx header3.xml is the canonical fixture: a
	// textbox-bearing `<w:r><w:pict>...<w:txbxContent>...
	// </w:txbxContent>...</w:pict></w:r>` followed by a plain
	// `<w:r><w:pict><v:rect/></w:pict></w:r>` — the picts share rPr
	// (`<w:noProof/>`) but bridge keeps them as two `<w:r>` envelopes.
	if strings.Contains(r.data, "<w:txbxContent") {
		return false
	}
	// mc:AlternateContent is a markup-compatibility selector
	// (ECMA-376 Part 3 / ISO/IEC 29500-3 §10): each AC is its own
	// alternative-resolution context with `<mc:Choice Requires="…">`
	// / `<mc:Fallback>` semantics. Fusing two adjacent same-rPr ACs
	// into one `<w:r>` would imply the consumer treats them as a
	// single resolution unit, contradicting the per-AC selection
	// rule. Upstream Okapi keeps each AC in its own `<w:r>` envelope.
	// AlternateContent.docx canonical case: a paragraph carrying two
	// adjacent `<w:r><mc:AlternateContent>...</mc:AlternateContent>
	// </w:r>` envelopes (each rsidRPr-only rPr) whose inner Choice
	// payloads have no txbxContent so the txbx guard doesn't catch
	// them.
	if strings.HasPrefix(r.data, "<mc:AlternateContent") {
		return false
	}
	return true
}

// splitRunWrapper returns the opening and closing portions of the
// <w:r>...</w:r> wrapper for a sentinel run, with the run's run-
// properties (rPr) included in the opening. Used by writeRunToSkel to
// frame an opaque drawing payload with the original run wrapper while
// emitting the inner XML piecewise to the skeleton.
func splitRunWrapper(r textRun) (open, close string) {
	// Delegate to serializeFullRPrXML so the wrapper carries BOTH
	// the toggle props (b/i/u/strike/vertAlign/vanish) AND the
	// non-toggle rPrChildren (rStyle, color, sz, lang, noProof, …).
	// Previously this function only emitted toggles, dropping
	// children like <w:noProof/> and <w:lang w:eastAsia="ru-RU"/> on
	// drawing-only paragraphs (859.docx — Strict OOXML — was the
	// canonical fixture: the drawing paragraph hits the
	// isEmptyRuns branch in parseParagraph, which routes through
	// writeRunToSkel → splitRunWrapper, bypassing buildBlock).
	// Per ECMA-376-1 §17.3.2.1 (CT_R) every rPr child applies to the
	// run regardless of what the run carries (text vs drawing).
	return "<w:r>" + serializeFullRPrXML(r.props), "</w:r>"
}
