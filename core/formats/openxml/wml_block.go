// Block assembly: turning a paragraph's merged run list into a model.Block —
// the inline codes, annotations and skeleton references that carry the source
// markup — plus the hidden-run policy that decides what reaches the block.

package openxml

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/internal/xmlesc"
	"github.com/neokapi/neokapi/core/model"
)

// TypeHiddenRun tags an isolated RunCode-style placeholder carrying the
// FULL `<w:r>...</w:r>` envelope of a hidden-text run (vanish on the
// run's own rPr or via the rStyle character-style chain). The Ph.Data
// field holds the raw `<w:r>...</w:r>` XML; the writer's renderWMLBlock
// `default` case in the Ph dispatch emits Ph.Data verbatim, which
// preserves the source text untranslated. Mirrors upstream Okapi
// StyledTextMapping.addRun (StyledTextMapping.java:203-211) which
// promotes runs with `!containsVisibleText()` to isolated RunCodes.
//
// SubTypeHiddenRunVanish is the only refinement currently emitted: it
// covers both direct `<w:vanish/>` on the run AND vanish inherited via
// rStyle. ECMA-376-1 §17.3.2.45 (<w:vanish>) defines the toggle; per
// §17.3.2.29 (<w:rStyle>) the resolved style chain contributes to the
// run's effective formatting, so a chain that authors `<w:vanish/>`
// (e.g. HiddenExcluded.docx's Haydn / FranzJosef styles) produces an
// effectively hidden run even when the run's own rPr lacks vanish.
//
// These two constants live here (in wml.go) rather than in
// vocabulary.go so the change stays scoped to the reader-side
// promotion path; the writer's existing `default` Ph branch emits the
// payload verbatim regardless of the type-string value, so no writer
// dispatch update is required.
const (
	TypeHiddenRun          = "struct:hidden-run"
	SubTypeHiddenRunVanish = "openxml:vanish"
)

// isHiddenRun reports whether a textRun should be promoted to an
// isolated RunCode-style Ph (TypeHiddenRun) so the pseudo-translator
// and downstream tooling skip its body.
//
// A run is hidden when:
//
//  1. Its own rPr carries `<w:vanish/>` (parsed into runProps.vanish).
//  2. Its rStyle chain resolves to a style whose effective rPr has
//     vanish (e.g. HiddenExcluded.docx's Haydn / FranzJosef styles
//     which carry `<w:vanish/>` directly in the style's rPr).
//
// Whole-paragraph hidden cases (paragraph-level `<w:vanish/>` via
// pStyle, all runs hidden) are filtered upstream by allHidden in
// parseParagraph — those paragraphs never reach buildBlock. This
// helper covers the per-paragraph mixed case where some runs are
// hidden and some are visible.
//
// Mirrors upstream Okapi RunParser.clarifyVisibility
// (RunParser.java:298-316): the vanish lookup walks
// `combinedRunProperties = styleDefinitions.combinedRunProperties(
// paragraphStyle, runStyle, runProperties)` so the rStyle's resolved
// chain participates in the visibility decision.
//
// We deliberately skip the highlight / color / excluded-style branches
// of upstream's clarifyVisibility — those paths only fire when
// `tsExcludeWordStyles`, `tsWordHighlightColors`, or
// `tsWordExcludedColors` are non-empty (defaults are empty, see
// ConditionalParameters.reset() at line 829-832). Native's Config has
// no equivalent toggles wired through yet; if one is added, extend
// this helper symmetrically.
//
// Vanish-clear semantics: when the run carries an explicit
// `<w:vanish w:val="0"/>` (or "false"/"off"), runProps.vanishExplicit
// is true AND runProps.vanish is false — the run's direct rPr CLEARS
// any vanish inherited via the rStyle chain. ECMA-376-1 §17.3.2.45
// (CT_OnOff) toggle semantics: a clearing override at the closer
// (more specific) level wins over the inherited setting. Mirrors
// upstream Okapi RunParser.clarifyVisibility (RunParser.java:310-316)
// which iterates `combinedRunProperties.properties()` and the FIRST
// vanish encountered (the run's direct one — combine merges the
// run's properties last, so they sit on top of the chain) is the
// deciding value. HiddenExcluded.docx's 17th paragraph is the
// canonical case: rStyle=Haydn (Haydn carries `<w:vanish/>`) with a
// direct `<w:vanish w:val="0"/>` override → the run is VISIBLE and
// must be translated.
func (p *wmlParser) isHiddenRun(run textRun) bool {
	if run.props.vanishExplicit {
		// Direct rPr authored a vanish toggle (on or off). The run's
		// own value overrides any rStyle-chain inheritance.
		return run.props.vanish
	}
	if run.props.vanish {
		return true
	}
	if p.styles == nil {
		return false
	}
	rStyleID := extractRStyleID(run.props.rPrChildren)
	if rStyleID == "" {
		return false
	}
	return p.styles.resolveProps(rStyleID).vanish
}

// buildBlock builds a model.Block from a list of merged text runs.
//
// commonRPrXML is the children-only serialisation of the rPr elements
// that are present and identical across every translatable source run
// in the paragraph (computed by commonRPrChildren BEFORE mergeRuns
// collapsed adjacent same-toggle runs). When non-empty it is stored as
// the openxmlSourceRPrAnnotation on the block so the writer can
// reapply it on every emitted <w:r>. This is the per-run rPr
// preservation path required by Bowrain Issue #592.
//
// perRunRPrXML is the per-text-run rPr fragments sidecar (Phase 1 of
// the per-run rPr work — see PARITY_NOTES.md "1083-*" cluster).
// When non-empty it is stashed as the openxmlPerRunRPrAnnotation on
// the block; the writer wire-up that consumes it lands in Phase 2.
// Until then this annotation is read-only sidecar data and does not
// change writer behaviour.
func (p *wmlParser) buildBlock(id string, runs []textRun, partPath, commonRPrXML string, perRunRPrXML []string, perRunSrcRunStart []bool) *model.Block {
	// The paragraph's address: the part it lives in, the table/row/cell
	// enclosing it, and its position among the paragraphs of that cell — or of
	// the part body when it is not in a table. See structural_name.go.
	p.path.ensurePart(partPath)
	blockName := p.path.name(p.path.reserve("p"))
	b := &runBuilder{}
	ids := &spanIDs{}

	var activeProps *runProps

	for _, run := range runs {
		// Handle sentinel markers for special content.
		//
		// The single-char sentinels (U+E100 tab, U+E101 image) are
		// dispatched only on EXACT match, not HasPrefix, so source text
		// that legitimately contains private-use characters in this
		// range (e.g. fixture OkapiMarkers.docx whose first <w:t> body
		// is U+E101 U+E102 U+E103) does not trip the sentinel branches
		// and get rewritten as a phantom <w:tab/> / <w:drawing/>. The
		// reader populates these sentinel runs with the codepoint as
		// the WHOLE text (textRun{text:"\uE100"...} at the tab read
		// site, {text:"\uE101"...} at the drawing/AlternateContent
		// read sites); mergeRuns refuses to fuse sentinel runs with
		// regular text (see isSentinel guard) so a true sentinel never
		// grows past one rune. Per Unicode U+E000..U+F8FF (Private Use
		// Area) these codepoints carry no inherent semantics — Okapi's
		// reservation of them as internal markers must not collide
		// with documents that author them as text. Mirrors upstream
		// Okapi which never substitutes a synthetic element for source
		// text containing PUA chars: RunParser.parseText
		// (RunParser.java lines 820-836) emits the source text
		// verbatim into the RunText body chunk regardless of code
		// point.
		if run.text == "\uE100" {
			// Tab placeholder. Upstream Okapi RunMerger fuses
			// adjacent same-rPr runs even when one begins with
			// <w:tab/> (Document-with-tabs.docx reference output:
			// `<r>Before</r><r><tab/>after</r>` merges to
			// `<r><t>Before</t><tab/><t>after</t></r>`); the writer's
			// inline-into-run path mirrors that behaviour.
			//
			// RunMerger.canRunPropertiesBeMerged (RunMerger.java:156-229)
			// gates merging on rPr equality, so when the tab's source
			// <w:r> rPr toggles diverge from the currently-active
			// toggles upstream's RunMerger does NOT merge \u2014 the bold or
			// italic run before the tab stays in its own envelope.
			//
			// When the tab started a fresh source <w:r> AND its source
			// rPr toggles (b/i/u/strike/vertAlign) differ from activeProps,
			// close the active toggles BEFORE emitting the Ph so the
			// writer's runProps no longer carries them. Otherwise the
			// writer's inline-into-run path (curRPr == adjSrc) would
			// silently match on the empty common-rPr while the OPEN
			// <w:r> carries a runProps toggle that the tab's source
			// <w:r> never had, trapping the <w:tab/> inside a bold or
			// italic envelope. Fixture: TabAtEndAfterNewRun.docx
			// (`<r>Usag</r><r><rPr><b/></rPr>es</r><r><tab/></r>` \u2014 the
			// trailing tab's <w:r> has no <w:rPr>, so the bold close
			// must land between "es" and the <w:tab/>, and the tab
			// opens a fresh empty-rPr <w:r>). Per ECMA-376-1
			// \u00A717.3.3.31 (<w:tab/>) the tab is a run child whose rPr
			// context is its containing <w:r>; preserving the source
			// envelope means the per-run rPr round-trips intact.
			if run.srcRunStart && activeProps != nil && !activeProps.isEmpty() && !activeProps.equal(run.props) {
				activeProps.appendClosingRuns(b, ids)
				activeProps = nil
			}
			// Embed the source <w:r>'s rPr into the Ph.Data so the writer
			// can inline this tab into a preceding text run when their
			// source-rPrs match. Without this, a `<w:r>{X}<w:t>foo</w:t></w:r>
			// <w:r>{X}<w:tab/></w:r>` source sequence (separate source <w:r>s
			// with identical rPr X) splits at the writer because the tab Ph
			// has no per-run sidecar slot, so the writer's inline gate
			// (curRPr == adjustRPrForRunText(sourceRPr,text)) falls back to
			// comparing against the paragraph-common rPr (empty when the
			// paragraph's runs vary) and refuses the inline. Mirrors
			// upstream Okapi RunMerger (RunMerger.java:156-229) which
			// fuses adjacent same-rPr source <w:r> envelopes \u2014 tab-bearing
			// or text-bearing \u2014 into one RunBuilder with both <w:t> and
			// <w:tab/> body chunks under the shared rPr. Per ECMA-376-1
			// \u00A717.3.2.1 (CT_R) a single <w:r> may carry multiple <w:tab/>
			// children alongside <w:t> children under one shared rPr.
			// AlternateContentTest.docx footer1 is the canonical fixture:
			// `<w:r>{FontStyle18,lang}<w:t>46 70 82 19</w:t></w:r>
			// <w:r>{FontStyle18,lang}<w:tab/></w:r>` round-trips as
			// `<w:r>{FontStyle18}<w:tab/><w:t>...</w:t><w:tab/></w:r>`
			// after WSO/lang strip.
			tabData := "<w:tab/>"
			if rPr := serializeFullRPrXML(run.props); rPr != "" {
				tabData = rPr + tabData
			}
			b.AddPh(ids.placeholder(),
				TypeTab, SubTypeTab,
				tabData, "\t", "",
				false, false, false)
			continue
		}
		if run.text == "\uE105" {
			// Paragraph-level opaque sentinel \u2014 captures
			// `<m:oMath>` / `<m:oMathPara>` (ECMA-376 Part 1 \u00A722.1)
			// or paragraph-level `<mc:AlternateContent>` (ECMA-376
			// Part 3 \u00A710) that the reader saw as a direct `<w:p>`
			// child rather than wrapped in a `<w:r>` (parseParagraph
			// dispatch at the `case "oMathPara", "oMath":` and
			// `case "AlternateContent":` arms emits the run with
			// `text: "\uE105", data: <captured raw XML>`).
			//
			// Without an explicit case here the run falls through to
			// the formatting/AddText branches below, where
			// `b.AddText("\uE105")` swallows the sentinel as plain
			// text and the captured paragraph-level payload would be
			// lost on round-trip. Mirrors upstream Okapi BlockParser
			// (BlockParser.java:240-260) which routes `<m:oMath>`
			// and paragraph-level `<mc:AlternateContent>` events
			// into the gather-into-markup path so the entire subtree
			// survives as opaque markup chunks on the resulting
			// Block.
			//
			// The writer's TypeOpaqueParaChild branch dumps Ph.Data
			// raw at paragraph level (no `<w:r>` wrapper) \u2014 matching
			// the source's direct-`<w:p>`-child position. Canonical
			// fixture: OpenXML_text_reference_v1_2.docx (an
			// `<m:oMath>` integral equation immediately follows the
			// translatable "Here is a math equation:  " text body
			// inside the same `<w:p>`).
			subType := SubTypeOMath
			// The captured payload begins with the source element's
			// start tag \u2014 `<m:oMath`, `<m:oMathPara`, or
			// `<mc:AlternateContent`. Most paragraph-level AC sits
			// inside `<w:r>` and reaches the `\uE101` image sentinel
			// path; the rare `<w:p>`-direct-child AC variant lands
			// here. Tag the subtype so downstream consumers can
			// distinguish the two without reparsing Ph.Data.
			if strings.HasPrefix(run.data, "<mc:AlternateContent") {
				subType = SubTypeAlternateContentParaChild
			}
			// Surface OMML equations as portable LaTeX in the placeholder's
			// Equiv, so cross-format writers (markdown/DocLang) render the math
			// while docx round-trip still replays the opaque OMML from Ph.Data.
			// Equiv is not part of the byte-exact/parity rendering (which uses
			// Ph.Data), so this is round-trip- and parity-safe.
			equiv, disp := "", ""
			if subType == SubTypeOMath && p.cfg != nil && p.cfg.ExtractNonTranslatableContent() {
				equiv, disp = ommlToMathEquiv(run.data)
			}
			b.AddPh(ids.placeholder(),
				TypeOpaqueParaChild, subType,
				run.data, equiv, disp,
				false, false, false)
			continue
		}
		if run.text == "\uE101" {
			// Image/drawing/pict/object/oMath/AlternateContent
			// placeholder. The original element's full XML is in
			// run.data so the writer can restore it byte-for-byte.
			// Fall back to a self-closing <w:drawing/> if data was
			// never populated (legacy callers).
			//
			// When the source <w:r> wrapping the drawing carried its
			// own <w:rPr>, prepend that rPr to Ph.Data so the writer's
			// TypeImage handler can re-emit it inside the <w:r>. Per
			// ECMA-376-1 \u00A717.3.2.1 (CT_R) <w:rPr> precedes the run's
			// other children, so the embedded fragment is in document
			// order. The writer detects the `<w:rPr>` prefix and emits
			// the rPr alongside the drawing payload (mirroring the
			// existing TypeFootnoteRef envelope, which also threads its
			// per-run rPr through the Ph.Data prefix). 859.docx is the
			// canonical fixture: the drawing-bearing run carries
			// `<w:rPr><w:noProof/><w:lang w:eastAsia="ru-RU"/></w:rPr>
			// <w:drawing>` and the rPr must round-trip with the
			// drawing on the wire.
			data := run.data
			if data == "" {
				data = "<w:drawing/>"
			}
			if rPr := serializeFullRPrXML(run.props); rPr != "" {
				data = rPr + data
			}
			subType := SubTypeImage
			if !run.srcRunStart {
				// Drawing/pict/object/AlternateContent/ruby NOT at the
				// start of its source <w:r> — the writer can fuse it
				// back into a still-open envelope from the preceding
				// text/Markup chunk. See SubTypeImageInline doc.
				subType = SubTypeImageInline
			}
			b.AddPhAttrs(ids.placeholder(),
				TypeImage, subType,
				data, "", "",
				false, false, false,
				p.drawingAttrs(data))
			continue
		}
		if after, ok := strings.CutPrefix(run.text, "\uE10D:"); ok {
			// Raw run-child markup (TypeRawRunMarkup) for empty
			// CT_Empty elements that round-trip verbatim:
			// <w:noBreakHyphen/> (ECMA-376-1 \u00A717.3.3.18) and
			// <w:softHyphen/> (\u00A717.3.3.30). Mirrors upstream Okapi
			// RunParser (RunParser.java lines 752-766) which routes
			// these to runBuilder.addToMarkup so they survive the
			// round-trip when ConditionalParameters has neither
			// `replaceNoBreakHyphenTag` nor `ignoreSoftHyphenTag`
			// set. The sentinel payload after the ":" is the literal
			// XML to re-emit; the writer wraps it in a <w:r> with
			// the source rPr context.
			rawXML := after
			subType := SubTypeNoBreakHyphen
			switch {
			case strings.Contains(rawXML, "softHyphen"):
				subType = SubTypeSoftHyphen
			case strings.Contains(rawXML, "cr"):
				subType = SubTypeCR
			case strings.Contains(rawXML, "bidi"):
				// `<w:bidi>` as direct `<w:r>` child (899.docx). See
				// SubTypeBidi doc + the reader's `case "bidi":` in
				// parseRunWithFieldState for the upstream-Okapi
				// citation. The writer's TypeRawRunMarkup branch
				// keys on this subtype to leave the `<w:r>` open for
				// the following same-source-run text to fuse into.
				subType = SubTypeBidi
			}
			// When the paragraph has no common rPr (heterogeneous
			// rPr across text runs \u2192 commonRPrXML is empty) AND this
			// raw-markup run carried its OWN rPr in the source, the
			// writer's TypeRawRunMarkup branch would emit
			// `<w:r><w:cr/></w:r>` with no rPr at all \u2014 the source's
			// per-run rPr (e.g. `<w:rStyle w:val="DONOTTRANSLATE"/>`)
			// is lost on the wire. Embed the rPr into the Ph.Data so
			// the writer's empty-sourceRPr branch (writer.go ~3394:
			// `<w:r>` + Ph.Data + `</w:r>`) emits the rPr in document
			// order. Mirrors the TypeImage / TypeBreak embedded-rPr
			// pattern in writer.go for the same heterogeneous-rPr
			// paragraph scenario. Per ECMA-376-1 \u00A717.3.2.1 (CT_R)
			// `<w:rPr>` precedes the run's other children; per
			// \u00A717.3.3.4 the `<w:cr/>` inherits its containing `<w:r>`'s
			// rPr context.
			//
			// Guarded on commonRPrXML == "" so the homogeneous-rPr
			// case (sourceRPr non-empty \u2192 writer prefixes its own
			// `<w:rPr>` block) doesn't get a duplicate `<w:rPr>`
			// element.
			//
			// Strip `<w:szCs/>` from the embedded rPr \u2014 sentinels were
			// skipped by the per-run szCs strip in parseParagraph
			// (isSentinel guard at line 2285) because they previously
			// did not surface their rPr. With the embedding the cr's
			// rPr does reach the wire, so the same chain-absent strip
			// must apply per upstream Okapi RunParser.canBeSkipped
			// (RunParser.java:226-228) \u2014 szCs is the complex-script
			// mirror of `<w:sz>` (ECMA-376-1 \u00A717.3.2.39) and the cr
			// element carries no character data, so the no-CS-text
			// gate trivially passes. Without this strip MissingPara's
			// cr-bearing runs would emit `<w:szCs val="48"/>` that
			// upstream Okapi strips at parse time. The chain-absent
			// gate `!chainNames["szCs"]` is checked because some
			// fixtures (947-non-cs.docx) intentionally inherit
			// `<w:szCs val="\u2026"/>` via docDefaults \u2014 there the strip
			// is correctly gated off.
			//
			// Fixture: MissingPara.docx \u2014 the `<w:r>` carrying
			// `<w:rPr><w:rStyle w:val="DONOTTRANSLATE"/></w:rPr>
			// <w:cr/></w:r>` was emitting as `<w:r><w:cr/></w:r>`
			// with the rStyle dropped.
			if commonRPrXML == "" {
				crProps := run.props
				// Mirror the body-text loop's szCs strip at
				// parseParagraph line ~2365 — sentinels were skipped
				// there by the isSentinel guard because their rPr did
				// not previously reach the wire. Now that we embed the
				// rPr the same chain-absent strip applies; subType ==
				// SubTypeCR guarantees the run carries no character
				// data so the containsComplexScriptText gate from
				// upstream RunParser.canBeSkipped trivially passes.
				// The chain-authored-szCs case is rare for cr-bearing
				// runs (cr appears inside a body-text paragraph whose
				// chain already passed the strip on its text runs);
				// when present the cr's szCs will match the chain via
				// the chain-XML-match strip applied by later optim
				// passes. Per ECMA-376-1 §17.3.2.39 (szCs) the strip
				// is semantically safe for the no-CS-content case.
				if subType == SubTypeCR {
					crProps.rPrChildren = stripChainAbsentSzCs(append([]rPrChild(nil), run.props.rPrChildren...))
				}
				if rPr := serializeFullRPrXML(crProps); rPr != "" {
					rawXML = rPr + rawXML
				}
			}
			b.AddPh(ids.placeholder(),
				TypeRawRunMarkup, subType,
				rawXML, "", "",
				false, false, false)
			continue
		}
		if after, ok := strings.CutPrefix(run.text, "\uE102:"); ok {
			// Footnote/endnote reference. The per-run rPr children
			// (e.g. <w:rStyle w:val="FootnoteReference"/>) travel
			// alongside the marker so the writer can emit the marker
			// inside a <w:r> that carries that rPr \u2014 matching upstream
			// Okapi RunBuilder which keeps the marker inside the same
			// <w:r> as its rPr (ECMA-376 Part 1 \u00A717.3.2.1: CT_R requires
			// rPr to precede children).
			// The sentinel may tag the element kind ("f" for
			// footnoteReference, "e" for endnoteReference). Older
			// callers emit the untagged form ("\uE102:<id>"); treat
			// those as footnote references for back-compat.
			rest := after
			markerElem := "footnoteReference"
			if after, ok := strings.CutPrefix(rest, "f:"); ok {
				rest = after
			} else if after, ok := strings.CutPrefix(rest, "e:"); ok {
				rest = after
				markerElem = "endnoteReference"
			}
			noteID := rest
			data := fmt.Sprintf(`<w:%s w:id="%s"/>`, markerElem, noteID)
			if rPr := serializeRPrChildrenXML(run.props); rPr != "" {
				data = rPr + data
			}
			b.AddPh(ids.placeholder(),
				TypeFootnoteRef, SubTypeFootnoteRef,
				data,
				"",
				fmt.Sprintf("[%s]", noteID),
				false, false, false)
			continue
		}
		if after, ok := strings.CutPrefix(run.text, "\uE103:"); ok {
			// Hyperlink open
			data := after
			b.AddPcOpenAttrs(ids.openSpan(),
				TypeHyperlink, SubTypeHyperlink,
				data, "", "",
				true, true, true,
				run.attrs)
			continue
		}
		if strings.HasPrefix(run.text, "\uE104:") {
			// Hyperlink close
			if activeProps != nil && !activeProps.isEmpty() {
				// Close formatting before hyperlink close
				activeProps.appendClosingRuns(b, ids)
				activeProps = nil
			}
			b.AddPcClose(ids.closeSpan(),
				TypeHyperlink, SubTypeHyperlink,
				"</w:hyperlink>", "")
			continue
		}
		if strings.HasPrefix(run.text, "\uE109:") {
			// SmartTag open \u2014 paired-code open emitted as opaque
			// markup. Per ECMA-376 Part 1 \u00A717.5.1.9 and upstream
			// Okapi RunContainer (RunContainer.java lines 29-43)
			// the start tag must round-trip verbatim around the
			// inner runs. Close any active rPr toggle so the
			// smartTag start element doesn't sit inside an open
			// <w:r>.
			if activeProps != nil && !activeProps.isEmpty() {
				activeProps.appendClosingRuns(b, ids)
				activeProps = nil
			}
			data := strings.TrimPrefix(run.text, "\uE109:")
			b.AddPcOpen(ids.openSpan(),
				TypeSmartTag, SubTypeSmartTag,
				data, "", "",
				true, true, true)
			continue
		}
		if strings.HasPrefix(run.text, "\uE10A:") {
			// SmartTag close \u2014 paired-code close emitted as opaque
			// markup. Same close-active-rPr discipline as the open
			// half so the end tag isn't trapped inside an open
			// <w:r>.
			if activeProps != nil && !activeProps.isEmpty() {
				activeProps.appendClosingRuns(b, ids)
				activeProps = nil
			}
			data := strings.TrimPrefix(run.text, "\uE10A:")
			b.AddPcClose(ids.closeSpan(),
				TypeSmartTag, SubTypeSmartTag,
				data, "")
			continue
		}
		if strings.HasPrefix(run.text, "\uE10E:") {
			// Generic opaque paired-code OPEN. Currently dispatches:
			//   - "ins" / "moveTo": Strict-OOXML revision-insertion
			//     wrapper. Per ECMA-376-1 \u00A717.13.5.16 the wrapper
			//     preserves around its inner runs in the strict namespace
			//     (upstream Okapi's RevisionInline skippable QName is
			//     bound to the transitional URI only \u2014
			//     SkippableElement.java:209-212).
			//   - "sdt" / "sdt-no-content": inline `<w:sdt>` Structured
			//     Document Tag wrapper. Per ECMA-376-1 \u00A717.5.2 the
			//     `<w:sdt>` envelope and its `<w:sdtPr>` /
			//     `<w:sdtEndPr>` / `<w:sdtContent>` children round-trip
			//     verbatim (upstream Okapi RunContainer.java:97-176).
			// Close any active rPr toggle so the wrapper start tag
			// doesn't sit inside an open <w:r>. Sentinel payload format:
			// "\uE10E:<localName>:<rawStartTagOrPrefix>".
			if activeProps != nil && !activeProps.isEmpty() {
				activeProps.appendClosingRuns(b, ids)
				activeProps = nil
			}
			rest := strings.TrimPrefix(run.text, "\uE10E:")
			localName, data, _ := strings.Cut(rest, ":")
			pcType := TypeRevisionIns
			subType := SubTypeRevisionIns
			switch localName {
			case "moveTo":
				subType = SubTypeRevisionMoveTo
			case "sdt":
				pcType = TypeSDT
				subType = SubTypeSDT
			case "sdt-no-content":
				pcType = TypeSDT
				subType = SubTypeSDTNoContent
			}
			b.AddPcOpen(ids.openSpan(),
				pcType, subType,
				data, "", "",
				true, true, true)
			continue
		}
		if strings.HasPrefix(run.text, "\uE10F:") {
			// Generic opaque paired-code CLOSE. See the OPEN dispatch
			// above for the full localName \u2192 Type/SubType mapping.
			// Sentinel payload format: "\uE10F:<localName>:<rawEndTag>".
			if activeProps != nil && !activeProps.isEmpty() {
				activeProps.appendClosingRuns(b, ids)
				activeProps = nil
			}
			rest := strings.TrimPrefix(run.text, "\uE10F:")
			localName, data, _ := strings.Cut(rest, ":")
			pcType := TypeRevisionIns
			subType := SubTypeRevisionIns
			switch localName {
			case "moveTo":
				subType = SubTypeRevisionMoveTo
			case "sdt":
				pcType = TypeSDT
				subType = SubTypeSDT
			case "sdt-no-content":
				pcType = TypeSDT
				subType = SubTypeSDTNoContent
			}
			b.AddPcClose(ids.closeSpan(),
				pcType, subType,
				data, "")
			continue
		}
		if strings.HasPrefix(run.text, "\uE106:") || strings.HasPrefix(run.text, "\uE107:") {
			// Bookmark start/end placeholder. Per ECMA-376 Part 1
			// \u00A717.13.6 these are direct children of <w:p> rather
			// than <w:r>. The writer's `default` Ph branch emits
			// Ph.Data verbatim with no <w:r> wrapper, mirroring
			// upstream Okapi which adds non-_GoBack bookmarks as
			// inline Markup chunks on the Block (see
			// BlockSkippableElements.skip / BlockParser line 294).
			//
			// Close any active formatting first so the bookmark
			// doesn't sit between the open <w:r>...rPr and the
			// next text run when re-rendered.
			if activeProps != nil && !activeProps.isEmpty() {
				activeProps.appendClosingRuns(b, ids)
				activeProps = nil
			}
			subType := SubTypeBookmarkStart
			if strings.HasPrefix(run.text, "\uE107:") {
				subType = SubTypeBookmarkEnd
			}
			b.AddPh(ids.placeholder(),
				TypeBookmark, subType,
				run.data, "", "",
				false, false, false)
			continue
		}
		if strings.HasPrefix(run.text, "\uE10B:") || strings.HasPrefix(run.text, "\uE10C:") {
			// Comment-range start/end placeholder. Per ECMA-376
			// Part 1 \u00A717.13.4.3 / \u00A717.13.4.4 (CT_MarkupRangeStart
			// / CT_MarkupRange) these are direct children of <w:p>
			// \u2014 same shape as <w:bookmarkStart>/<w:bookmarkEnd>.
			// The writer's `default` Ph branch emits Ph.Data
			// verbatim with no <w:r> wrapper, mirroring upstream
			// Okapi's wordConfiguration.ymlbal classification of
			// w_commentrangestart / w_commentrangeend as INLINE
			// markup (lines 59-63).
			//
			// Close any active formatting first so the marker
			// doesn't sit between the open <w:r>...rPr and the
			// next text run when re-rendered.
			if activeProps != nil && !activeProps.isEmpty() {
				activeProps.appendClosingRuns(b, ids)
				activeProps = nil
			}
			subType := SubTypeCommentRangeStart
			if strings.HasPrefix(run.text, "\uE10C:") {
				subType = SubTypeCommentRangeEnd
			}
			b.AddPh(ids.placeholder(),
				TypeCommentRange, subType,
				run.data, "", "",
				false, false, false)
			continue
		}
		if isFieldSentinel(run.text) {
			// Complex-field markup chunk. Per upstream Okapi
			// RunParser.parseComplexField (lines 461-542 of
			// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/
			// openxml/RunParser.java) every fldChar (begin/separate/
			// end) and instrText event flows through
			// runBuilder.addToMarkup so the original markup survives
			// the round-trip even when the field code is not in
			// tsComplexFieldDefinitionsToExtract. Same shape applies to
			// fldSimple per BlockParser.parse lines 242-250.
			//
			// Close any active formatting first so the field markup
			// doesn't get trapped inside an <w:r>...rPr wrapper meant
			// for the surrounding translatable text. The captured
			// payload already carries its own <w:r>...</w:r> (or
			// <w:fldSimple>...</w:fldSimple>) wrapper.
			if activeProps != nil && !activeProps.isEmpty() {
				activeProps.appendClosingRuns(b, ids)
				activeProps = nil
			}
			subType := SubTypeFieldChar
			if strings.HasPrefix(run.text, "\uE108:fldSimple") {
				subType = SubTypeFieldSimple
			}
			b.AddPh(ids.placeholder(),
				TypeField, subType,
				run.data, "", "",
				false, false, false)
			continue
		}

		// Handle line break. When the source <w:br/> began a new
		// <w:r> with no preceding text in it, tag the Ph with
		// SubTypeBreakStandalone so the writer keeps the source-run
		// envelope intact (cannot inline into the previous run).
		// 1421-line-break.docx is the canonical fixture: three
		// source runs <r>text</r><r>br</r><r>br+text</r> must
		// round-trip as three output runs, not collapse into one.
		if run.text == "\n" {
			// When the br started a fresh source <w:r> AND its source
			// rPr toggles (b/i/u/strike/vertAlign) differ from
			// activeProps, close the active toggles BEFORE emitting
			// the Ph so the writer's runProps no longer carries them.
			// Symmetric with the <w:tab/> guard above (line ~4227).
			// Without this, a `<r><rPr><i/></rPr><t>...</t></r>
			// <r><rPr><rFonts.../></rPr><br/></r>` source sequence
			// (br.docx, br2.docx, EndGroup.docx canonical case) leaks
			// the open <w:i/> toggle into the standalone <w:br/>'s
			// emitted <w:r> — upstream Okapi RunBuilder + RunMerger
			// (RunBuilder.java:73-188, RunMerger.java:156-229) treat
			// a heterogeneous-rPr boundary as a hard run break,
			// closing toggles first per ECMA-376-1 §17.3.2.1 (CT_R)
			// where each <w:r> has its own <w:rPr> context. The
			// `run.srcRunStart` predicate matches the tab branch: a
			// br that DIDN'T begin a fresh source <w:r> shares the
			// surrounding text's <w:r> envelope and should keep the
			// active toggle context.
			if run.srcRunStart && activeProps != nil && !activeProps.isEmpty() && !activeProps.equal(run.props) {
				activeProps.appendClosingRuns(b, ids)
				activeProps = nil
			}
			subType := SubTypeBreak
			if run.srcRunStart {
				subType = SubTypeBreakStandalone
			}
			// Use the captured br element verbatim if available so
			// page/column-break attrs survive the round-trip; fall
			// back to the literal `<w:br/>` for legacy callers that
			// did not populate run.data. Per ECMA-376-1 §17.3.3.1
			// (CT_Br), w:type ("page" / "column" / "textWrap") and
			// w:clear control rendering and must round-trip.
			brXML := run.data
			if brXML == "" {
				brXML = "<w:br/>"
			}
			// When the source <w:r> wrapping the br carries its own
			// <w:rPr>, prepend that rPr to the Ph data so the writer's
			// TypeBreak handler can re-emit it inside the <w:r>.
			// Mirrors the existing TypeImage / TypeFootnoteRef
			// embedded-rPr pattern (wml.go ~line 4309, writer.go
			// ~line 3060). Per ECMA-376-1 §17.3.2.1 (CT_R) <w:rPr>
			// precedes the run's other children, so the embedded
			// fragment is in document order. Without this, a
			// `<w:r><w:rPr>{szCs}</w:rPr><w:br/></w:r>` source run
			// (EndGroup.docx canonical case) loses its szCs sidecar
			// on the way out — the writer falls back to the empty
			// paragraph-wide sourceRPr when the surrounding text
			// runs have different rPr (so the common-rPr is empty)
			// and the br Ph has no per-text-run sidecar slot.
			if rPr := serializeFullRPrXML(run.props); rPr != "" {
				brXML = rPr + brXML
			}
			b.AddPh(ids.placeholder(),
				TypeBreak, subType,
				brXML, "\n", "",
				false, false, false)
			continue
		}

		// Promote a hidden-text run (vanish on the run's own rPr OR via
		// the rStyle character-style chain) to an opaque RunCode-style
		// Ph carrying the FULL `<w:r>...</w:r>` envelope verbatim. The
		// pseudo-translator and the writer never look inside the Ph's
		// raw payload — the source text round-trips untranslated, the
		// hidden run's source-rPr (vanish toggle, rStyle reference, …)
		// is preserved byte-for-byte, and the run boundaries against
		// the surrounding visible runs survive intact.
		//
		// Mirrors upstream Okapi StyledTextMapping.addRun
		// (StyledTextMapping.java:203-211): when
		// `!run.containsVisibleText()` the run is converted into an
		// isolated RunCode (PLACEHOLDER) so it does not contribute
		// translatable text to the TextFragment. Run.containsVisibleText
		// returns false when the RunBuilder's `isHidden` flag is set —
		// computed by RunParser.clarifyVisibility (RunParser.java:298-364)
		// from `combinedRunProperties` which folds the rStyle chain in
		// alongside the run's direct rPr (line 305-309).
		// `clarifyVisibility` reads the merged vanish toggle at line 311-314
		// and short-circuits on `getTranslateWordHidden`.
		//
		// Per ECMA-376-1 §17.3.2.45 (<w:vanish>) hidden text is
		// suppressed from display; treating it as translatable would
		// expose it to the translator and pseudo-pass would mutate
		// content that is never shown. Per ECMA-376-1 §17.3.2.29
		// (<w:rStyle>) the referenced character style's rPr is part of
		// the run's effective formatting, so a style chain that
		// authors `<w:vanish/>` (e.g. the Haydn / FranzJosef styles in
		// HiddenExcluded.docx) marks every run that uses it as hidden
		// even when the run's own rPr lacks vanish.
		//
		// HiddenExcluded.docx is the canonical fixture: a paragraph
		// mixes visible runs with a `<w:rPr><w:vanish/></w:rPr>` run
		// AND a `<w:rPr><w:rStyle w:val="Haydn"/></w:rPr>` run (Haydn
		// rStyle has `<w:vanish/>` in its rPr). The reference
		// pseudo-translates the visible runs only; the two hidden runs
		// keep their source text verbatim. Whole-paragraph hidden
		// cases (paras whose pStyle inherits vanish, runs whose own
		// vanish covers the whole para) are filtered earlier by
		// `allHidden` in parseParagraph (no Block emitted at all);
		// this branch handles the per-paragraph mixed case where some
		// runs are hidden and some are not.
		//
		// `cfg.TranslateHiddenText` mirrors upstream's
		// `getTranslateWordHidden`: when true the hidden runs flow as
		// regular translatable text (no Ph promotion).
		if !p.cfg.TranslateHiddenText && p.isHiddenRun(run) {
			if activeProps != nil && !activeProps.isEmpty() {
				activeProps.appendClosingRuns(b, ids)
				activeProps = nil
			}
			rPrXML := serializeFullRPrXML(run.props)
			// Always emit `xml:space="preserve"` — the source text may
			// carry leading/trailing whitespace (HiddenExcluded.docx's
			// `hidden [direct vanish] ` ends with a space) and the
			// reference output preserves it. Per ECMA-376-1 §17.3.3.20
			// (<w:t>) the xml:space attribute defaults to "default"
			// which collapses surrounding whitespace; "preserve" keeps
			// it intact, matching upstream Okapi RunBuilder which
			// emits xml:space="preserve" whenever the run text is not
			// pure non-whitespace.
			fullRunXML := "<w:r>" + rPrXML + `<w:t xml:space="preserve">` + xmlesc.Text(run.text) + "</w:t></w:r>"
			b.AddPh(ids.placeholder(),
				TypeHiddenRun, SubTypeHiddenRunVanish,
				fullRunXML, run.text, "",
				false, false, false)
			// Reset activeProps so the next visible run opens its own
			// formatting context — the hidden Ph has its own
			// self-contained <w:r> envelope and does not influence
			// open toggles.
			activeProps = nil
			continue
		}

		// Handle formatting changes
		if activeProps == nil || !activeProps.equal(run.props) {
			// Close previous formatting. We measure the runBuilder
			// before/after so the post-emit "boundary still invisible"
			// guard below sees the ACTUAL marker count, not just the
			// "tried to emit" intent. appendClosingRuns / appendOpeningRuns
			// only emit Pc markers for the toggles bold / italic /
			// underline / strike / vertAlign — toggles like vanish that
			// runProps tracks but never round-trip as inline codes (per
			// ECMA-376-1 §17.3.2.42 the hidden-text bit is a run-level
			// rPr property with no inline span representation) appear in
			// `equal()` but contribute no Pc markers. Without measuring
			// actual emission, a run boundary that differs ONLY in vanish
			// would silently coalesce into the previous TextRun via
			// AddText (HiddenExcluded.docx fixture: a paragraph mixing
			// `<w:r><w:t>visible</w:t></w:r>` and
			// `<w:r><w:rPr><w:vanish/></w:rPr><w:t>hidden</w:t></w:r>`
			// would emit a single fused TextRun, dropping the per-source
			// vanish sidecar). Mirrors upstream Okapi RunBuilder.java
			// lines 73-188 + RunMerger.canRunPropertiesBeMerged
			// (RunMerger.java:156-229): hidden runs are kept distinct
			// (RunMerger.canMergeWith line 127 short-circuits on
			// `runBuilder.isHidden() || otherRunBuilder.isHidden()`).
			beforeClose := len(b.runs)
			if activeProps != nil && !activeProps.isEmpty() {
				activeProps.appendClosingRuns(b, ids)
			}
			emittedClose := len(b.runs) > beforeClose
			beforeOpen := len(b.runs)
			if !run.props.isEmpty() {
				run.props.appendOpeningRuns(b, ids)
			}
			emittedOpen := len(b.runs) > beforeOpen
			// When neither close nor open emitted any toggle codes the
			// run boundary is invisible to runBuilder's text-coalescing
			// path — AddText would append into the previous TextRun and
			// lose the source-run boundary. This happens when adjacent
			// source runs share toggle props (both empty) but differ on
			// font name (rFonts ascii vs asciiTheme — fixture
			// 1312-fonts-info.docx), on vanish (HiddenExcluded.docx —
			// `<w:vanish/>` toggles hidden state without an inline code),
			// or on other non-toggle properties that runProps.equal()
			// inspects. The rule is "any !equal() that emits no markers".
			// Force a model.Run boundary so the per-source-run rPr sidecar
			// (#592 Phase 1) stays slot-aligned with the model.Run
			// population — otherwise the writer's alignment guard
			// (renderWMLBlock) nils the sidecar and per-run rPr emission
			// (Phase 2) silently regresses to common-rPr-only output.
			//
			// Mirrors upstream Okapi RunBuilder.java lines 73-188 +
			// RunMerger.canRunPropertiesBeMerged (RunMerger.java lines
			// 156-229): heterogeneous RunProperties keep runs distinct
			// on the way to the writer. Per ECMA-376-1 §17.3.2 and
			// §17.3.2.26 (the rFonts content-category model that makes
			// asciiTheme/ascii alternatives for the same Latin script).
			if activeProps != nil && !emittedClose && !emittedOpen {
				b.Break()
			}
			propsCopy := run.props
			activeProps = &propsCopy
		} else if !activeProps.equalIncludingChildren(run.props) {
			// Toggles match (so no PcOpen/PcClose break was emitted)
			// but the non-toggle rPrChildren differ between adjacent
			// source runs (e.g. different <w:color>, <w:sz>, or
			// <w:rStyle>). Force a model.Run boundary so the per-
			// source-run rPr sidecar (#592 Phase 1) stays slot-
			// aligned with the model.Run population — otherwise the
			// writer's alignment guard (renderWMLBlock) nils the
			// sidecar and per-run rPr emission (Phase 2) silently
			// regresses to common-rPr-only output.
			//
			// Mirrors upstream Okapi RunBuilder.java lines 73-188 +
			// RunMerger.canRunPropertiesBeMerged (RunMerger.java
			// lines 156-229): heterogeneous RunProperties (toggle OR
			// non-toggle) keep runs distinct on the way to the
			// writer. Per ECMA-376-1 §17.3.2.
			b.Break()
			propsCopy := run.props
			activeProps = &propsCopy
		} else if run.inFieldDisplay && run.srcRunStart {
			// Same toggle + non-toggle rPr as the previous run, but
			// this run started a fresh source <w:r> inside an
			// extractable complex field's display text region. Force
			// a model.Run boundary so the writer keeps the source's
			// per-<w:r> envelopes distinct, mirroring upstream Okapi
			// parseComplexField (RunParser.java:461-542) where each
			// display-text source run becomes its own RunText body
			// chunk inside the field's RunBuilder and the surrounding
			// </w:r><w:r> boundaries survive as Markup chunks
			// between them. Per ECMA-376-1 §17.16.5 (Complex Fields)
			// the field's display text retains the source's run
			// grouping. Without this break the writer would emit the
			// pair as a single <w:r> via runBuilder's text-coalescing
			// path. Fixtures: 1083-empty-and-hyperlink-instructions.
			// docx (and the two hyperlink-and-* siblings).
			b.Break()
			propsCopy := run.props
			activeProps = &propsCopy
		}

		b.AddText(run.text)
	}

	// Close any remaining open formatting
	if activeProps != nil && !activeProps.isEmpty() {
		activeProps.appendClosingRuns(b, ids)
	}

	// Apply code finder before block construction so the placeholder
	// runs it inserts land in the builder's run sequence alongside the
	// formatting runs.
	blockRuns := b.Runs()
	if p.codeFinder != nil {
		blockRuns = p.codeFinder.applyToRuns(blockRuns, ids)
	}

	block := &model.Block{
		ID:           id,
		Type:         "paragraph",
		Translatable: true,
		Source:       blockRuns,
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   map[string]string{"partPath": partPath},
	}

	// Collect font info if configured
	if p.cfg.ExtractRunFontsInfo {
		fonts := collectFonts(runs)
		if fonts != "" {
			block.SetAnno("fonts", &model.GenericAnnotation{
				Kind:   "fonts",
				Fields: map[string]any{"names": fonts},
			})
		}
	}

	// Stash the common per-source-run rPr children for the writer (#592).
	// The writer prepends this XML to every emitted <w:r>'s <w:rPr>.
	// Native is faithful — the rPr stays inline. Upstream Okapi
	// additionally lifts it into a synthesised paragraph style
	// (StyleOptimisation.Default.applyTo, StyleOptimisation.java lines
	// 96-129); native does not — the equivalence is folded by the parity
	// comparator's effective-rPr normalizer instead.
	if commonRPrXML != "" {
		block.SetAnno(openxmlSourceRPrAnnotationKey, &model.GenericAnnotation{
			Kind:   openxmlSourceRPrAnnotationKey,
			Fields: map[string]any{"xml": commonRPrXML},
		})
	}

	// Stash the per-text-run rPr fragments sidecar (Phase 1 of the
	// per-run rPr work — see PARITY_NOTES.md "1083-*" cluster). The
	// writer wire-up (Phase 2) consumes this annotation; until then it
	// is read-only sidecar data and does not change writer behaviour.
	if len(perRunRPrXML) > 0 {
		block.SetAnno(openxmlPerRunRPrAnnotationKey, &model.GenericAnnotation{
			Kind:   openxmlPerRunRPrAnnotationKey,
			Fields: map[string]any{"fragments": perRunRPrXML},
		})
	}

	// Stash the per-text-run "starts new source <w:r>" boolean sidecar
	// so the writer can decide whether a text run reuses the still-open
	// <w:r> from a preceding standalone <w:br/> / <w:tab/> Ph or opens
	// a fresh <w:r>. See openxmlPerRunSrcRunStartAnnotationKey.
	if len(perRunSrcRunStart) > 0 {
		block.SetAnno(openxmlPerRunSrcRunStartAnnotationKey, &model.GenericAnnotation{
			Kind:   openxmlPerRunSrcRunStartAnnotationKey,
			Fields: map[string]any{"flags": perRunSrcRunStart},
		})
	}

	// Stash the per-text-run "inside a complex-field display region"
	// boolean sidecar so the writer keeps separate <w:r> envelopes for
	// each source run inside an extractable field's display text. See
	// openxmlPerRunInFieldDisplayAnnotationKey for the upstream-Okapi
	// rationale (parseComplexField at RunParser.java:461-542).
	perRunInFieldDisplay := perRunInFieldDisplayFlags(runs)
	if len(perRunInFieldDisplay) > 0 {
		anyTrue := false
		for _, f := range perRunInFieldDisplay {
			if f {
				anyTrue = true
				break
			}
		}
		if anyTrue {
			block.SetAnno(openxmlPerRunInFieldDisplayAnnotationKey, &model.GenericAnnotation{
				Kind:   openxmlPerRunInFieldDisplayAnnotationKey,
				Fields: map[string]any{"flags": perRunInFieldDisplay},
			})
		}
	}

	// Stash the per-text-run "source had rPr" boolean sidecar so the
	// writer (emitRPr) can emit an empty `<w:rPr></w:rPr>` placeholder
	// for in-field-display runs whose source declared an rPr even when
	// nothing survived the strip pass. See
	// openxmlPerRunSourceHadRPrAnnotationKey.
	perRunSourceHadRPr := perRunSourceHadRPrFlags(runs)
	if len(perRunSourceHadRPr) > 0 {
		anyTrue := false
		for _, f := range perRunSourceHadRPr {
			if f {
				anyTrue = true
				break
			}
		}
		if anyTrue {
			block.SetAnno(openxmlPerRunSourceHadRPrAnnotationKey, &model.GenericAnnotation{
				Kind:   openxmlPerRunSourceHadRPrAnnotationKey,
				Fields: map[string]any{"flags": perRunSourceHadRPr},
			})
		}
	}

	block.Name = blockName
	return block
}

// collectFonts returns a comma-separated list of unique font names from runs.
func collectFonts(runs []textRun) string {
	seen := make(map[string]bool)
	var fonts []string
	for _, r := range runs {
		for _, f := range []string{r.props.fontName, r.props.fontNameCS, r.props.fontNameEA} {
			if f != "" && !seen[f] {
				seen[f] = true
				fonts = append(fonts, f)
			}
		}
	}
	return strings.Join(fonts, ", ")
}
