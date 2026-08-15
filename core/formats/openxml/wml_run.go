// Run parsing: the w:r element loop that turns a source run's children into
// textRuns, carrying the complex-field state machine across the run boundary.

package openxml

import (
	"encoding/xml"
	"strings"

	"github.com/neokapi/neokapi/core/internal/xmlesc"
)

// parseRunWithFieldState parses a <w:r> element while tracking complex field state.
// It delegates to parseRun for content extraction, but handles fldChar and instrText
// to maintain the field state machine across runs within a paragraph.
//
// When the run carries field markup (fldChar begin/separate/end or
// instrText), the *entire* <w:r> — rPr, all children, end tag — is also
// captured raw and returned as a SubTypeFieldChar sentinel run so the
// writer can round-trip the markup verbatim. This mirrors upstream
// Okapi's RunParser.parseComplexField behaviour (lines 461-542 of
// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/openxml/
// RunParser.java) which routes fldChar/instrText runs through
// runBuilder.addToMarkup so they survive on the block as opaque markup
// chunks regardless of whether the field code is in
// ConditionalParameters.tsComplexFieldDefinitionsToExtract.
//
// rawStart is the raw XML form of the <w:r> start tag (including the
// open angle bracket and attributes) produced by the caller via
// startElementToString. The function appends children verbatim to a
// raw buffer alongside parsing them for content; if any child triggers
// the field-markup path, the assembled raw block is returned as the
// sentinel run's data field. Otherwise the raw buffer is discarded.
func (p *wmlParser) parseRunWithFieldState(d *xml.Decoder, cfs *complexFieldState, rawStart string) ([]textRun, error) {
	var props runProps
	var runs []textRun
	hasProps := false

	// rawBuf accumulates the verbatim XML serialisation of the run as
	// we decode it, so we can hand back an opaque copy when fldChar /
	// instrText is detected. Initialised lazily on first need; backLog
	// holds any post-<w:r> content already consumed before raw capture
	// engaged (e.g. an rPr that precedes the fldChar in document order
	// — `<w:r><w:rPr><w:b/></w:rPr><w:fldChar .../></w:r>` is the
	// canonical shape in 768.docx). Without backLog the rPr would be
	// dropped from the captured payload and the field-marker run would
	// emit without its source rPr.
	var rawBuf strings.Builder
	var rawCaptured bool
	var hasFieldMarkup bool
	var backLog strings.Builder
	// splitRPrRaw holds the run's stripped `<w:rPr>…</w:rPr>` so that, when
	// the eager-capture run is split at a field-marker boundary (the #598a
	// `end → text → begin` window, see flushOpaqueSegment below), each
	// synthesised opaque segment `<w:r>` and the surfaced body-text run
	// carry the source run's run-properties. Per ECMA-376-1 §17.3.2.1
	// (CT_R) every `<w:rPr>` child applies to the whole run regardless of
	// which child (fldChar / `<w:t>`) it sits beside, so splitting the run
	// into parts must replicate the rPr onto each part. Empty when the run
	// had no rPr (or it stripped to nothing).
	var splitRPrRaw string
	startRawCapture := func() {
		if rawCaptured {
			return
		}
		rawBuf.WriteString(rawStart)
		if backLog.Len() > 0 {
			rawBuf.WriteString(backLog.String())
			backLog.Reset()
		}
		rawCaptured = true
	}
	// emitRaw appends s to rawBuf when raw capture is active, otherwise
	// holds it in backLog so a later startRawCapture() can replay any
	// pre-trigger content (rPr that precedes the field marker, etc.).
	emitRaw := func(s string) {
		if rawCaptured {
			rawBuf.WriteString(s)
		} else {
			backLog.WriteString(s)
		}
	}
	// flushOpaqueSegment closes the in-progress opaque field segment
	// (rawBuf, which begins with a `<w:r>` start tag) with a `</w:r>` and
	// emits it as a SubTypeFieldChar sentinel run, then resets the raw
	// buffers so a fresh `<w:r>` segment can begin. This is the #598a
	// run-splitting primitive: when a single source `<w:r>` CLOSES one
	// complex field (`<w:fldChar end/>`), authors translatable body text,
	// then OPENS another (`<w:fldChar begin/>`), the field markers must
	// stay opaque while the body text in between becomes a translatable
	// run. Eager raw-capture (engaged when cfs.active at run entry) would
	// otherwise swallow the whole run into one opaque sentinel, silently
	// losing the body text — the #598a data-loss bug (fixture 830-7.docx
	// run `<w:r><w:rPr>…</w:rPr><w:fldChar end/><w:t>, a race of</w:t>
	// <w:fldChar begin/></w:r>`).
	//
	// Per ECMA-376-1 §17.3.2.1 (CT_R) run children apply in document
	// order; text after an `end` and before the next `begin` is ordinary
	// body text, NOT field markup. Upstream Okapi's RunParser models this
	// by RETURNING from parseComplexField on the matching `end`
	// (RunParser.java:472-479) back to the parse() loop, which then routes
	// the following `<w:t>` through parseContent as a translatable RunText
	// body chunk (RunParser.java:537) until the next begin re-enters
	// parseComplexField (RunParser.java:259, 494-499). The split sentinels
	// the writer re-fuses via detectFldCharEndForMerge /
	// detectFldCharBeginForMerge so the original single-`<w:r>` shape is
	// reconstructed on write.
	//
	// The synthesised sentinel `<w:r>` carries the run's rPr (splitRPrRaw)
	// so the opaque field-marker run keeps the source formatting.
	didSplit := false
	flushOpaqueSegment := func() {
		if !rawCaptured {
			return
		}
		rawBuf.WriteString("</w:r>")
		runs = append(runs, textRun{
			text:        ":fldChar",
			props:       props,
			data:        rawBuf.String(),
			srcRunStart: true,
		})
		rawBuf.Reset()
		rawCaptured = false
		didSplit = true
	}
	// reengageRawCapture re-opens raw capture on a fresh `<w:r>` segment
	// after a #598a split. The original rPr lives in the already-flushed
	// leading segment, so a re-engaged segment must replay splitRPrRaw to
	// keep the trailing field-marker `<w:r>` carrying the source run's
	// run-properties (ECMA-376-1 §17.3.2.1, CT_R). On fusion the writer's
	// detectFldCharBeginForMerge folds this run into the preceding text
	// run and the duplicate rPr is dropped; the replay only matters when
	// the trailing segment stands alone.
	reengageRawCapture := func() {
		freshStart := !rawCaptured
		startRawCapture()
		if freshStart && didSplit && splitRPrRaw != "" {
			rawBuf.WriteString(splitRPrRaw)
		}
	}
	// When the caller is already inside an active complex field whose
	// content is being preserved verbatim — i.e. between begin and end
	// for any non-extractable field, or between begin and separate for
	// any field — every run in that span is opaque markup per upstream
	// Okapi (RunParser.parseComplexField, lines 501-506: events route
	// to runBuilder.addToMarkup unless extractable && atResult). Engage
	// raw capture eagerly so display-text runs lacking fldChar /
	// instrText (e.g. the cached `<w:r><w:rPr><w:noProof/></w:rPr>
	// <w:t>I am a textfield.</w:t></w:r>` between separate and end in
	// Textfield.docx) survive the round-trip with their rPr intact.
	if cfs.active && (!cfs.extractable || !cfs.atResult) {
		startRawCapture()
		hasFieldMarkup = true
	}

	for {
		tok, err := d.Token()
		if err != nil {
			return nil, err
		}

		// When raw capture is active, mirror the token verbatim into
		// rawBuf alongside whatever specialised handling the switch
		// performs below. The handlers themselves call into helpers
		// (readCharData, parseRunProps, skipElement, captureRawElement)
		// that consume tokens from d *without* re-emitting them, so the
		// raw mirror has to be set up before each consumer call.
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "rPr":
				hasProps = true
				// Capture rPr raw before consuming its tokens so we can
				// preserve the run's run-properties verbatim on opaque
				// emission. parseRunProps drains through the matching
				// </w:rPr> via skipElement, so without pre-capture the
				// raw buffer would lose the rPr subtree entirely.
				rPrRaw, err := captureRawElement(d, t)
				if err != nil {
					return nil, err
				}
				// Pre-strip noProof / lang / rPrChange / etc. from the
				// captured rPr to mirror upstream Okapi
				// RunSkippableElements (lines 50-62 of okapi/filters/
				// openxml/src/main/java/net/sf/okapi/filters/openxml/
				// RunSkippableElements.java).
				stripped := stripFieldRPrSkippables(rPrRaw)
				// Remember the stripped rPr for the #598a run-split path:
				// when a field-active run is split at an `end → text → begin`
				// boundary (flushOpaqueSegment + the body-text surfacing
				// below) the synthesised opaque segment `<w:r>` and the
				// surfaced body-text run must replicate this run's rPr per
				// ECMA-376-1 §17.3.2.1 (CT_R). Only retain a non-empty
				// stripped form — an empty wrapper contributes nothing.
				if !isStrippedRPrEmpty(stripped) {
					splitRPrRaw = stripped
				}
				// rPr policy on the field-markup capture path mirrors
				// the upstream RunParser flow:
				//   - When raw capture is already engaged (i.e. this
				//     run is an interior field-content run, e.g. a
				//     <w:rPr><w:noProof/></w:rPr> on a cached display
				//     text run inside an active complex field) the
				//     stripped rPr — even if empty — is included in
				//     the opaque payload. Okapi's RunParser drops the
				//     containing run into runBuilder.addToMarkup
				//     verbatim (RunParser.parseComplexField lines
				//     501-506) so the empty <w:rPr/> survives the
				//     round-trip (Textfield.docx is the canonical
				//     fixture).
				//   - When raw capture has not yet engaged (this run
				//     is the entry-point of the field, i.e. carries
				//     the begin / instrText / separate / end marker
				//     and the rPr appears in document order BEFORE
				//     the marker), only stash the rPr in backLog if
				//     stripping leaves a non-empty body. Okapi's
				//     RunParser routes the entry-point run's rPr
				//     through parseRunPropertiesAndRunStyle (line
				//     159) and ultimately through
				//     RunProperties.Default.getEvents (line 580 of
				//     RunProperties.java) which returns an empty
				//     event list for empty properties — so the rPr
				//     wrapper is dropped from the output entirely
				//     when nothing remains after stripping. The
				//     768.docx HYPERLINK fixtures rely on the
				//     non-empty branch (rPr carries <w:b/>); the
				//     ComplexTextfield.docx IF-begin run relies on
				//     the empty branch (rPr only had <w:lang/>).
				if rawCaptured {
					emitRaw(stripped)
				} else if !isStrippedRPrEmpty(stripped) {
					emitRaw(stripped)
				} else if cfs.active && cfs.extractable && cfs.atResult {
					// Past the separate marker of an extractable field
					// — this run is in the display-text region whose
					// envelope upstream Okapi preserves verbatim
					// (parseComplexField at RunParser.java:461-542
					// routes the wrapping <w:r>/<w:rPr>/</w:r> events
					// through runBuilder.addToMarkup via the
					// non-isTextStartEvent branch of parseContent at
					// lines 808-816). The captured payload feeds the
					// fldChar-end + text merge in the writer (the same
					// Ph carries the entire <w:r>…<w:fldChar end/></w:r>
					// shell), so the post-strip rPr — even when empty
					// — must reach the backLog or the merged output
					// loses the empty <w:rPr/> wrapper that upstream
					// emits. Fixtures: 1083-empty-and-hyperlink-
					// instructions.docx (and the two hyperlink-and-*
					// siblings) — the field-end run's source rPr is
					// <w:rPr><w:lang/></w:rPr>; after stripping the
					// strippable lang the wrapper is empty but the
					// reference output still carries `<w:rPr/>` inside
					// the fused run.
					emitRaw(stripped)
				}
				// Re-parse the captured rPr for typed properties.
				props, err = p.parseRunPropsFromRawCached(rPrRaw, p.currentStyleChainNames)
				if err != nil {
					return nil, err
				}

			case "fldChar":
				hasFieldMarkup = true
				// reengageRawCapture (not startRawCapture) so that a
				// `<w:fldChar begin/>` that RE-OPENS a field after a #598a
				// `end → text` split lands in a fresh `<w:r>` segment that
				// replays the source run's rPr. For the first/non-split
				// fldChar this behaves identically to startRawCapture.
				reengageRawCapture()
				// Mirror the fldChar element raw (including its ffData
				// subtree if present, e.g. Textfield.docx) into the
				// buffer.
				fldRaw, err := captureRawElement(d, t)
				if err != nil {
					return nil, err
				}
				rawBuf.WriteString(fldRaw)
				// Complex field state machine transition.
				//
				// Nested fields (level > 1) push the parent's state onto
				// outerFrames so the inner field operates with a fresh
				// (extractable=false, atResult=false) frame — mirroring
				// the per-frame locals of upstream Okapi's recursive
				// parseComplexField (RunParser.java:461-542). On the
				// matching end we pop the frame so the parent's
				// extraction policy resumes for any remaining content
				// inside the parent's result area.
				fldCharType := attrVal(t, "fldCharType")
				switch fldCharType {
				case "begin":
					if cfs.nestingLevel >= 1 {
						cfs.outerFrames = append(cfs.outerFrames, complexFieldFrame{
							fieldCode:   cfs.fieldCode,
							extractable: cfs.extractable,
							atResult:    cfs.atResult,
						})
					}
					cfs.nestingLevel++
					cfs.active = true
					cfs.fieldCode = ""
					cfs.extractable = false
					cfs.atResult = false
				case "separate":
					cfs.atResult = true
				case "end":
					cfs.nestingLevel--
					if cfs.nestingLevel <= 0 {
						cfs.active = false
						cfs.fieldCode = ""
						cfs.extractable = false
						cfs.atResult = false
						cfs.nestingLevel = 0
						cfs.outerFrames = nil
					} else if n := len(cfs.outerFrames); n > 0 {
						top := cfs.outerFrames[n-1]
						cfs.outerFrames = cfs.outerFrames[:n-1]
						cfs.fieldCode = top.fieldCode
						cfs.extractable = top.extractable
						cfs.atResult = top.atResult
					}
				}

			case "instrText":
				hasFieldMarkup = true
				startRawCapture()
				// Mirror the instrText element raw, preserving the
				// xml:space="preserve" attribute that field codes
				// commonly carry (e.g. ` PAGE \* MERGEFORMAT `).
				rawBuf.WriteString("<")
				writeElementName(&rawBuf, t.Name)
				for _, a := range t.Attr {
					rawBuf.WriteString(" ")
					writeAttrName(&rawBuf, a.Name)
					rawBuf.WriteString(`="`)
					rawBuf.WriteString(xmlesc.Attr(a.Value))
					rawBuf.WriteString(`"`)
				}
				rawBuf.WriteString(">")
				// Field instruction text — extract the field code name
				text, err := readCharData(d)
				if err != nil {
					return nil, err
				}
				rawBuf.WriteString(xmlesc.Text(text))
				rawBuf.WriteString("</")
				writeElementName(&rawBuf, t.Name)
				rawBuf.WriteString(">")
				// The fieldCode / extractable update applies to whichever
				// frame is currently innermost — nested fields run with
				// their own (fieldCode, extractable) per the upstream
				// recursive parseComplexField semantics.
				if cfs.active && cfs.fieldCode == "" {
					cfs.fieldCode = complexFieldCodeName(text)
					cfs.extractable = p.isExtractableField(cfs.fieldCode)
				}

			case "t":
				// #598a `end → text → begin` body-text split. When eager
				// raw-capture is engaged (the run was field-active at entry)
				// but the field is NO LONGER active (cfs.active==false — a
				// `<w:fldChar end/>` earlier in THIS run closed the
				// innermost field, dropping the nesting level to 0), the
				// `<w:t>` we are about to read is ordinary translatable body
				// text, NOT field markup. Per ECMA-376-1 §17.3.2.1 (CT_R)
				// run children apply in document order; text after an `end`
				// and before the next `begin` is body content. Upstream
				// Okapi RETURNS from parseComplexField on the matching `end`
				// (RunParser.java:472-479) so this text flows through the
				// parse() loop to parseContent as a RunText body chunk
				// (RunParser.java:537), exactly as it would for a run with no
				// field at all.
				//
				// We mirror that by flushing the accumulated opaque segment
				// (the run-start + rPr + the closing `<w:fldChar end/>`) as
				// its own SubTypeFieldChar sentinel run, then dropping out of
				// raw-capture so the text below surfaces as a translatable
				// run. A subsequent `<w:fldChar begin/>` in the same source
				// `<w:r>` re-engages startRawCapture() on a fresh segment,
				// producing the run sequence
				// `[end-sentinel, body-text, begin-sentinel]`; the writer
				// re-fuses these into the original single `<w:r>` via
				// detectFldCharEndForMerge / detectFldCharBeginForMerge.
				// Fixture 830-7.docx run
				// `<w:r><w:rPr>…</w:rPr><w:fldChar end/><w:t>, a race of</w:t>
				// <w:fldChar begin/></w:r>` (and the `end → text` tail
				// `<w:r><w:fldChar end/><w:t>, a </w:t>…` with no trailing
				// begin) previously lost this body text to eager capture.
				if rawCaptured && !cfs.active {
					flushOpaqueSegment()
				}
				// Capture <w:t ...> open tag verbatim into rawBuf
				// before draining its char data, so opaque emission
				// preserves the text exactly as authored (including
				// xml:space="preserve" when present).
				if rawCaptured {
					rawBuf.WriteString("<")
					writeElementName(&rawBuf, t.Name)
					for _, a := range t.Attr {
						rawBuf.WriteString(" ")
						writeAttrName(&rawBuf, a.Name)
						rawBuf.WriteString(`="`)
						rawBuf.WriteString(xmlesc.Attr(a.Value))
						rawBuf.WriteString(`"`)
					}
					rawBuf.WriteString(">")
				}
				text, err := readCharData(d)
				if err != nil {
					return nil, err
				}
				if rawCaptured {
					rawBuf.WriteString(xmlesc.Text(text))
					rawBuf.WriteString("</")
					writeElementName(&rawBuf, t.Name)
					rawBuf.WriteString(">")
				}
				// Tag display-text runs inside an extractable complex
				// field's result region so mergeRuns honours the
				// source's per-<w:r> boundary. See textRun.inFieldDisplay
				// for the upstream-Okapi rationale (parseComplexField
				// captures these as RunText body chunks inside the
				// field's RunBuilder, separated by Markup chunks
				// preserving the source `</w:r><w:r>` boundaries —
				// they do NOT pass through RunMerger.canMergeWith).
				inField := cfs.active && cfs.extractable && cfs.atResult
				// preFieldBody marks `<w:t>` text decoded as a REAL
				// translatable run (rawCaptured==false, so it is NOT also
				// mirrored into the opaque field sentinel's rawBuf) while NO
				// complex field is open (cfs.active==false). This is the
				// 830-7.docx shape `<w:r><w:rPr>…</w:rPr><w:t>, humans
				// exiled…; the </w:t><w:fldChar w:fldCharType="begin"/></w:r>`
				// — body text authored BEFORE a begin marker that opens a
				// field in the SAME source `<w:r>`. The run is returned as
				// `[text…, fldChar-sentinel]`; without the flag the caller's
				// field-aware dropTextRuns discards the text because cfs is
				// active on return. Upstream Okapi accumulates this as a
				// RunText body chunk of the field-opening run before
				// transitioning to parseComplexField (RunParser.java:259,
				// 537), so the text is translatable body content, not
				// suppressed field markup.
				//
				// The rawCaptured guard avoids double emission: when raw
				// capture is already engaged (the field was active at run
				// entry — e.g. a run that CLOSES one field with `<w:fldChar
				// end/>`, authors text, then opens another with `<w:fldChar
				// begin/>`), the text is already mirrored verbatim into the
				// opaque sentinel payload, so re-surfacing it as a translatable
				// run would duplicate it on the wire. See textRun.preFieldBody.
				preField := !cfs.active && !rawCaptured
				runs = append(runs, textRun{text: text, props: props, inFieldDisplay: inField, sourceHadRPr: hasProps, preFieldBody: preField})

			case "br":
				// Capture the break element verbatim (including any
				// w:type="page" / w:type="column" / w:clear attribute)
				// so the writer can re-emit the source's full element.
				// Per ECMA-376-1 §17.3.3.1 (CT_Br) the type attribute
				// distinguishes textWrap (default), page, and column
				// break semantics — losing it on round-trip changes
				// rendering. Fixture: PageBreak.docx (P2 carries
				// `<w:br w:type="page"/>` whose type attr was dropped
				// by the previous reader path's hardcoded `<w:br/>`).
				var brXML strings.Builder
				brXML.WriteString("<")
				writeElementName(&brXML, t.Name)
				for _, a := range t.Attr {
					brXML.WriteString(" ")
					writeAttrName(&brXML, a.Name)
					brXML.WriteString(`="`)
					brXML.WriteString(xmlesc.Attr(a.Value))
					brXML.WriteString(`"`)
				}
				brXML.WriteString("/>")
				if rawCaptured {
					rawBuf.WriteString(brXML.String())
				}
				// Carry the surrounding `<w:r>`'s rPr through on
				// the break run so toggle-bearing properties like
				// <w:vanish/> survive into the model. ECMA-376-1
				// §17.3.2.1 (CT_R) — every rPr child applies to the
				// run regardless of its payload (text vs <w:br/> vs
				// <w:tab/>). Without this, a vanish-bearing page-break
				// run loses its hidden marker on read; the writer's
				// runToXML uses serializeFullRPrXML(r.props) to emit
				// the rPr so the vanish round-trips faithfully
				// (PageBreak.docx — `<w:r><w:rPr><w:vanish/></w:rPr>
				// <w:br w:type="page"/></w:r>` must round-trip with the
				// vanish in place; upstream Okapi additionally promotes
				// it into a synthesised pStyle, which the parity
				// comparator resolves).
				runs = append(runs, textRun{
					text:  "\n",
					props: props,
					data:  brXML.String(),
				})
				if err := skipElement(d); err != nil {
					return nil, err
				}

			case "tab":
				if rawCaptured {
					rawBuf.WriteString("<")
					writeElementName(&rawBuf, t.Name)
					rawBuf.WriteString("/>")
				}
				if p.cfg.TabAsCharacter {
					runs = append(runs, textRun{text: "\t", props: props})
				} else {
					runs = append(runs, textRun{text: "\uE100", props: props}) // sentinel
				}
				if err := skipElement(d); err != nil {
					return nil, err
				}

			case "cr":
				// Per ECMA-376-1 \u00A717.3.3.4 (CT_Empty cr) \u2014 a soft
				// carriage return inside a run, equivalent to a
				// <w:br/> with default w:type="textWrap" but emitted
				// as its own element. Upstream Okapi RunParser
				// (RunParser.java:752-766) routes <w:cr/> to
				// runBuilder.addToMarkup so it survives the round-trip
				// inside the same <w:r> as its rPr context. RunMerger
				// does not collapse cr-bearing runs across <w:r>
				// boundaries (RunMerger.java:156-229 \u2014 same rPr fuses
				// only Markup chunks, the cr stays inside its own
				// envelope when neighbouring runs differ).
				//
				// Without this case the default branch at the bottom
				// of the dispatcher silently skipElement-s the
				// <w:cr/>, which has two side effects: the source
				// <w:r> wrapper that bracketed the cr disappears
				// (textRun boundary lost), and the subsequent text
				// run loses its source-run identity, dropping its rPr
				// (see MissingPara.docx fixture where
				// `<w:r><w:rPr><w:rStyle val="DONOTTRANSLATE"/></w:rPr>
				// <w:cr/></w:r>` was being dropped, taking the
				// DONOTTRANSLATE rStyle with it).
				//
				// We piggy-back on the U+E10D raw-run-markup sentinel
				// (already plumbed end-to-end via SubTypeCR in
				// vocabulary.go and TypeRawRunMarkup in writer.go) so
				// the writer re-emits `<w:cr/>` verbatim inside a
				// <w:r> carrying the source rPr. The element is
				// CT_Empty per the schema so there are no children
				// to capture.
				if rawCaptured {
					rawBuf.WriteString("<")
					writeElementName(&rawBuf, t.Name)
					rawBuf.WriteString("/>")
				}
				runs = append(runs, textRun{text: "\uE10D:<w:cr/>", props: props})
				if err := skipElement(d); err != nil {
					return nil, err
				}

			case "ptab":
				// Per ECMA-376-1 §17.3.1.32 (CT_PTab) — a positional
				// tab is a run-child element with attributes (alignment,
				// relativeTo, leader) controlling rendering position
				// relative to the page. Upstream Okapi RunParser routes
				// `<w:ptab .../>` to runBuilder.addToMarkup
				// (RunParser.java:752-766) so it survives the round-trip
				// inside the same <w:r> as its rPr context.
				//
				// Without this case the default branch at the bottom of
				// the dispatcher silently skipElement-s the <w:ptab/>,
				// so the writer drops it on round-trip and the source-run
				// envelope around it disappears (the surrounding text
				// runs collapse into one). Fixture:
				// OpenXML_text_reference_v1_2.docx — header1.xml authors
				// `<w:r><w:t>Header left align</w:t></w:r><w:r>
				// <w:ptab w:relativeTo="margin" w:alignment="center" .../>
				// </w:r><w:r><w:t>Header center</w:t></w:r>` and reference
				// output preserves both ptab elements between the text
				// runs.
				//
				// We piggy-back on the U+E10D raw-run-markup sentinel
				// (TypeRawRunMarkup in writer.go re-emits the captured
				// XML inside its <w:r>). Unlike <w:cr/> / <w:tab/>, ptab
				// carries attributes (relativeTo / alignment / leader), so
				// we capture the full start-element raw rather than
				// hard-coding a literal `<w:ptab/>`.
				ptabRaw := startElementToRaw(t)
				if strings.HasSuffix(ptabRaw, ">") {
					ptabRaw = ptabRaw[:len(ptabRaw)-1] + "/>"
				}
				if rawCaptured {
					rawBuf.WriteString(ptabRaw)
				}
				runs = append(runs, textRun{text: ":" + ptabRaw, props: props})
				if err := skipElement(d); err != nil {
					return nil, err
				}

			case "noBreakHyphen", "softHyphen":
				// Per ECMA-376-1 \u00A717.3.3.18 (CT_Empty noBreakHyphen)
				// and \u00A717.3.3.30 (CT_Empty softHyphen), these are
				// run-child elements with no content. Upstream Okapi
				// RunParser (RunParser.java lines 752-766) preserves
				// the element verbatim unless the conditional
				// parameter `replaceNoBreakHyphenTag` is true (in which
				// case it's substituted with a regular hyphen "-") or
				// `ignoreSoftHyphenTag` is true (in which case the
				// softHyphen is dropped). When preserved, upstream
				// adds the element to the run's Markup chunk stream so
				// it survives the round-trip \u2014 see fixture
				// special-chars-and-linebreaks.docx whose gold output
				// retains both <w:noBreakHyphen/> and <w:softHyphen/>.
				//
				// We mirror that with the \uE10D raw-run-markup
				// sentinel: the marker prefix carries the literal XML
				// to re-emit, so the writer can drop it back inside a
				// <w:r> without needing a dedicated Ph type. The
				// element's source <w:r> rPr travels in `props` so the
				// per-run rPr sidecar stays slot-aligned with the
				// model run population.
				localName := t.Name.Local
				if rawCaptured {
					rawBuf.WriteString("<")
					writeElementName(&rawBuf, t.Name)
					rawBuf.WriteString("/>")
				}
				if localName == "noBreakHyphen" && p.cfg.ReplaceNoBreakHyphenTag {
					runs = append(runs, textRun{text: "-", props: props})
				} else if localName == "softHyphen" && p.cfg.IgnoreSoftHyphenTag {
					// drop entirely per upstream's IGNORE_SOFT_HYPHEN_TAG
				} else {
					rawXML := "<w:" + localName + "/>"
					runs = append(runs, textRun{text: "\uE10D:" + rawXML, props: props})
				}
				if err := skipElement(d); err != nil {
					return nil, err
				}

			case "bidi":
				// Per ECMA-376-1 \u00A717.3.1.6 (CT_OnOff bidi) \u2014 schema places
				// `<w:bidi>` inside `<w:rPr>` (CT_RPr child), not as a
				// direct `<w:r>` child. However real-world authored
				// .docx files do place it as a DIRECT child of `<w:r>`,
				// between the `<w:r>` start tag and `<w:rPr>`. Fixture
				// 899.docx authors `<w:r><w:bidi w:val="0"/><w:rPr>
				// <w:rtl w:val="0"/><w:lang w:val="en-US"/></w:rPr>
				// <w:t>C11</w:t></w:r>`. Upstream Okapi RunParser
				// handles this via the generic markup fall-through
				// (RunParser.java:815 \u2014 `runBuilder.addToMarkup(e)`):
				// the bidi element survives as Markup inside the
				// containing RunBuilder's `<w:r>` envelope, emerging
				// alongside the run's `<w:t>` text under one shared
				// (post-strip) `<w:rPr>`.
				//
				// Without this case the default branch silently
				// skipElement-s the `<w:bidi>` and the writer loses
				// the marker entirely. We piggy-back on the U+E10D
				// raw-run-markup sentinel (TypeRawRunMarkup) and tag
				// the Ph with SubTypeBidi so the writer's
				// TypeRawRunMarkup branch can recognise it and leave
				// the `<w:r>` open (inRunNoText=true) \u2014 the following
				// same-source-run text then fuses inside the same
				// envelope via the writer's existing inRunNoText
				// branch. Per ECMA-376-1 \u00A717.3.2.1 (CT_R) a single
				// `<w:r>` may carry multiple body children alongside
				// one shared `<w:rPr>`; preserving the bidi as a
				// direct child rather than relocating it into the
				// rPr matches upstream Okapi's verbatim markup
				// preservation.
				//
				// CT_OnOff carries at most a single `w:val` attribute.
				// We capture the full start element raw (including
				// the attribute) so 1 vs 0 vs absent are all round-
				// tripped exactly. The element has no children per
				// the CT_OnOff schema.
				bidiRaw := startElementToRaw(t)
				if strings.HasSuffix(bidiRaw, ">") {
					bidiRaw = bidiRaw[:len(bidiRaw)-1] + "/>"
				}
				if rawCaptured {
					rawBuf.WriteString(bidiRaw)
				}
				runs = append(runs, textRun{text: "\uE10D:" + bidiRaw, props: props})
				if err := skipElement(d); err != nil {
					return nil, err
				}

			case "drawing", "pict", "object":
				// Capture the full element verbatim so the writer can
				// restore the original markup (drawings, OLE objects,
				// pictures with VML/DrawingML are opaque to the
				// translator but must round-trip byte-equivalently).
				raw, err := captureRawElement(d, t)
				if err != nil {
					return nil, err
				}
				if rawCaptured {
					rawBuf.WriteString(raw)
				}
				runs = append(runs, textRun{text: "\uE101", props: props, data: raw}) // image sentinel

			case "ruby":
				// <w:ruby> (ECMA-376-1 §17.3.3.25) wraps phonetic
				// guides above base text — used for East Asian ruby
				// annotations (furigana, pinyin, etc.). Capture the
				// full element verbatim so the writer can restore the
				// nested <w:rt> (ruby text) and <w:rubyBase> structure
				// byte-for-byte. Translatable strings inside ruby are
				// not yet extracted — bridge keeps them inline within
				// the ruby element in its reference output (the rt and
				// rubyBase <w:t> bodies survive translation but are
				// not pseudo-translated separately in the regression
				// suite), so verbatim capture matches the bridge
				// envelope for round-trip purposes. Per ECMA-376-1
				// §17.3.3.25 (CT_Ruby) ruby is a run child whose
				// CT_RubyContent + CT_RubyContent children are
				// themselves <w:r> wrappers — the captured payload
				// preserves the entire subtree.
				raw, err := captureRawElement(d, t)
				if err != nil {
					return nil, err
				}
				if rawCaptured {
					rawBuf.WriteString(raw)
				}
				runs = append(runs, textRun{text: "\uE101", props: props, data: raw}) // ruby reuses the opaque-image sentinel

			case "AlternateContent":
				// Markup Compatibility (ECMA-376 Part 3 / ISO/IEC
				// 29500-3 \u00A710): mc:AlternateContent wraps one or more
				// mc:Choice branches plus an optional mc:Fallback.
				// The processor selects the first Choice whose
				// Requires namespaces are all understood, otherwise
				// the Fallback. Okapi unconditionally selects Choice
				// and drops Fallback \u2014 see
				// SkippableElement.GeneralInline.ALTERNATE_CONTENT_FALLBACK
				// (line 56 of okapi/filters/openxml/src/main/java/
				// net/sf/okapi/filters/openxml/SkippableElement.java)
				// wired into RunSkippableElements (lines 45-49 and
				// 93-105 of okapi/filters/openxml/src/main/java/
				// net/sf/okapi/filters/openxml/RunSkippableElements.java).
				// The wrapper itself (mc:AlternateContent + mc:Choice)
				// stays in the output verbatim; the gold fixture
				// gold/parts/block/document-alternate-content.xml
				// shows mc:AlternateContent>mc:Choice surviving
				// round-trip with Fallback stripped. Mirror that here.
				raw, err := captureAlternateContent(d, t)
				if err != nil {
					return nil, err
				}
				if rawCaptured {
					rawBuf.WriteString(raw)
				}
				runs = append(runs, textRun{text: "\uE101", props: props, data: raw})

			case "footnoteReference", "endnoteReference":
				// Call-site marker (in document.xml). The containing
				// <w:r> may carry its own rPr (e.g.
				// <w:rStyle w:val="FootnoteReference"/>); upstream
				// Okapi keeps the marker inside the same <w:r> as that
				// rPr so the rStyle applies to the note number. ECMA-376
				// Part 1 \u00A717.11.13 (CT_FtnEdnRef) plus \u00A717.3.2.1
				// (CT_R: rPr precedes children). Capture the full
				// <w:r>...</w:r> verbatim via the field-markup machinery
				// so the writer emits the run with its rPr intact, just
				// like the back-reference case below. The previous Ph
				// path (TypeFootnoteRef) dropped the run-specific rPr
				// because it only consulted the paragraph-wide
				// sourceRPr fallback.
				noteID := attrVal(t, "id")
				if rawCaptured {
					rawBuf.WriteString("<")
					writeElementName(&rawBuf, t.Name)
					for _, a := range t.Attr {
						rawBuf.WriteString(" ")
						writeAttrName(&rawBuf, a.Name)
						rawBuf.WriteString(`="`)
						rawBuf.WriteString(xmlesc.Attr(a.Value))
						rawBuf.WriteString(`"`)
					}
					rawBuf.WriteString("/>")
				}
				// Encode the element kind into the sentinel so the writer
				// emits the correct marker (footnoteReference vs
				// endnoteReference). Default to "f" for back-compat with
				// any legacy callers that don't tag the sentinel.
				kind := "f"
				if t.Name.Local == "endnoteReference" {
					kind = "e"
				}
				runs = append(runs, textRun{text: "\uE102:" + kind + ":" + noteID, props: props}) // footnote/endnote sentinel
				if err := skipElement(d); err != nil {
					return nil, err
				}

			case "footnoteRef", "endnoteRef", "commentReference", "annotationRef":
				// Back-reference / annotation marker elements appearing
				// inside footnote/endnote/comment body paragraphs and
				// inside main-document runs that wrap a comment marker.
				//
				// Footnote/endnote back-references (e.g. <w:footnote
				// w:id="1"><w:p><w:r><w:rPr><w:rStyle
				// w:val="FootnoteReference"/></w:rPr><w:footnoteRef/>
				// </w:r>...</w:p></w:footnote>) \u2014 ECMA-376 Part 1
				// \u00A717.11.13 (CT_FtnEdnRef) / \u00A717.11.6: child of <w:r>,
				// no attributes, sibling to the run's <w:rPr>.
				//
				// Comment annotation marker (CT_Markup) \u2014 the comment
				// part's <w:r><w:rPr><w:rStyle w:val="CommentReference"/>
				// </w:rPr><w:annotationRef/></w:r> at the start of every
				// <w:comment> body, ECMA-376 Part 1 \u00A717.13.4.1.
				//
				// Comment reference call-site (CT_Markup) \u2014 the main
				// document's <w:r><w:rPr><w:rStyle
				// w:val="CommentReference"/></w:rPr><w:commentReference
				// w:id="N"/></w:r>, ECMA-376 Part 1 \u00A717.13.4.5.
				//
				// All four share the same shape: a <w:r> whose body is
				// the marker element plus an optional rPr, with no
				// translatable text. Upstream Okapi's wordConfiguration
				// .ymlbal classifies w_commentreference (line 65) as
				// INLINE alongside w_footnotereference / w_endnotereference,
				// and RunBuilder routes the run through addToMarkup so
				// the whole <w:r>...</w:r> is preserved verbatim. We
				// reuse the field-markup capture machinery so the run
				// keeps its rPr inside the same <w:r> per the schema.
				elemName := t.Name.Local
				startRawCapture()
				hasFieldMarkup = true
				rawBuf.WriteString("<w:")
				rawBuf.WriteString(elemName)
				// commentReference carries a w:id attribute (CT_Markup
				// derives from CT_Markup with required ID); the back-
				// reference forms (footnoteRef/endnoteRef/annotationRef)
				// are attribute-less per their schema, so we only emit
				// the attributes that were actually present.
				for _, a := range t.Attr {
					rawBuf.WriteString(" ")
					writeAttrName(&rawBuf, a.Name)
					rawBuf.WriteString(`="`)
					rawBuf.WriteString(xmlesc.Attr(a.Value))
					rawBuf.WriteString(`"`)
				}
				rawBuf.WriteString("/>")
				if err := skipElement(d); err != nil {
					return nil, err
				}

			case "sym":
				char := attrVal(t, "char")
				if rawCaptured {
					rawBuf.WriteString("<")
					writeElementName(&rawBuf, t.Name)
					for _, a := range t.Attr {
						rawBuf.WriteString(" ")
						writeAttrName(&rawBuf, a.Name)
						rawBuf.WriteString(`="`)
						rawBuf.WriteString(xmlesc.Attr(a.Value))
						rawBuf.WriteString(`"`)
					}
					rawBuf.WriteString("/>")
				}
				if char != "" {
					runs = append(runs, textRun{text: "[sym:" + char + "]", props: props})
				}
				if err := skipElement(d); err != nil {
					return nil, err
				}

			default:
				// Unknown / unsupported child element. Mirror raw if
				// we're already capturing \u2014 losing it on the opaque
				// path would corrupt the field markup.
				if rawCaptured {
					raw, err := captureRawElement(d, t)
					if err != nil {
						return nil, err
					}
					rawBuf.WriteString(raw)
				} else {
					if err := skipElement(d); err != nil {
						return nil, err
					}
				}
			}

		case xml.EndElement:
			if t.Name.Local == "r" {
				if hasFieldMarkup && !rawCaptured {
					// #598a `end → text` tail with NO trailing field
					// marker: a run that CLOSED a field (`<w:fldChar end/>`)
					// then authored body text and ended. The `end` markup
					// was already flushed as its own SubTypeFieldChar
					// sentinel by flushOpaqueSegment (in the `t` case), the
					// body text is a translatable run in `runs`, and no
					// further field segment is open (rawCaptured==false). So
					// there is nothing left to close — just mark the source
					// boundary and return the already-built run sequence
					// `[end-sentinel, body-text…]`. Without this guard the
					// hasFieldMarkup branch below would emit a spurious
					// `</w:r>`-only sentinel from the empty rawBuf. Fixture
					// 830-7.docx run
					// `<w:r><w:fldChar end/><w:t>, a </w:t>…</w:r>`.
					if len(runs) > 0 {
						runs[0].srcRunStart = true
					}
					return runs, nil
				}
				if hasFieldMarkup {
					rawBuf.WriteString("</")
					writeElementName(&rawBuf, t.Name)
					rawBuf.WriteString(">")
					// Pre-fldChar translatable content preservation.
					// Source shape `<w:r><w:rPr>...</w:rPr><w:t>text</w:t>
					// <w:fldChar w:fldCharType="begin"/></w:r>` (830-7.docx
					// line 65; also 956.docx, N_001_Auswertung_Part2.docx,
					// neverendingloop.docx) authors translatable text \u2014 or
					// `<w:tab/>` markup \u2014 BEFORE the field markup in the
					// same source `<w:r>`. Upstream Okapi's RunParser
					// processes the `<w:t>` as a RunText body chunk first
					// (parseContent at RunParser.java:537), then sees the
					// fldChar and transitions to parseComplexField (line
					// 259) \u2014 the text remains a body chunk of the run.
					//
					// Without this branch the runs slice is discarded
					// when hasFieldMarkup fires, losing translatable
					// content. The opaque sentinel's rawBuf only mirrors
					// content AFTER startRawCapture() engaged (i.e. on
					// the fldChar), so the pre-field `<w:t>` does NOT
					// appear in the sentinel's payload \u2014 emitting both
					// the text run AND the sentinel does NOT double the
					// text in output.
					//
					// Note: byte-shape divergence from upstream Okapi's
					// reference output is INTENTIONAL here. Okapi's
					// bridge runner drops the pre-fldChar text entirely
					// for some shapes (956.docx footer1.xml's `<w:t>1</w:t>
					// <w:fldChar end/>`, N_001_Auswertung_Part2.docx's
					// `<w:tab/><w:fldChar begin/>`, neverendingloop.docx
					// similar). Per ECMA-376-1 \u00A717.3.2.1 (CT_R), every
					// run child applies to the run; translatable text
					// must not be silently dropped on extraction. Native
					// is spec-correct; the parity tier "regression" on
					// 956/N_001/neverendingloop reflects Okapi being
					// equally-wrong-in-the-other-direction.
					if len(runs) > 0 {
						runs[0].srcRunStart = true
						runs = append(runs, textRun{
							text:        "\uE108:fldChar",
							props:       props,
							data:        rawBuf.String(),
							srcRunStart: true,
						})
						return runs, nil
					}
					return []textRun{{
						text:        "\uE108:fldChar",
						props:       props,
						data:        rawBuf.String(),
						srcRunStart: true,
					}}, nil
				}
				if len(runs) == 0 && hasProps && backLog.Len() > 0 && cfs.active {
					// Empty placeholder run preservation INSIDE an active
					// complex field. Source shape:
					// `<w:r><w:rPr>...</w:rPr></w:r>` with no body chunks
					// (no <w:t>, <w:fldChar>, <w:tab>, <w:br>, etc.). The
					// rPr lands in backLog via the rPr case above when its
					// stripped form is non-empty. Without this branch the
					// run is dropped entirely (caller's
					// `if len(run) == 0 { continue }` at parseParagraph),
					// taking its source <w:r> wrapper with it.
					//
					// The cfs.active gate matches upstream Okapi's
					// observed behaviour: empty placeholder runs sitting
					// between a complex field's separate and end markers
					// (often inside an intermediate paragraph that gets
					// pulled into the begin paragraph by the fld-end
					// migration logic) round-trip with their rPr intact \u2014
					// see 830-2.docx para 7 and 830-6.docx para 7, where
					// the placeholder run carries
					// `<w:rPr><w:rtl w:val="0"/></w:rPr>` and survives
					// alongside the migrated `<w:r><w:fldChar end/></w:r>`.
					//
					// Empty placeholders OUTSIDE field state (no active
					// field) are dropped by upstream \u2014 830-6.docx para 5
					// is the canonical case: a standalone
					// `<w:r><w:rPr><w:rtl w:val="0"/></w:rPr></w:r>` in a
					// paragraph with no field activity gets dropped, and
					// the paragraph collapses to `<w:p><w:pPr/></w:p>`.
					// The cfs.active guard mirrors that: only emit the
					// sentinel when the parser is between separate and
					// end (or otherwise inside a field span), so the
					// out-of-field placeholders return empty runs and
					// fall through to the caller's drop branch.
					//
					// Sentinel choice: piggy-back on SubTypeFieldChar
					// \u2014 its "captured opaque <w:r>...</w:r> payload"
					// semantics are exactly what we need, and the
					// writer's existing fldChar handler emits the data
					// verbatim. Avoiding a new sentinel type keeps the
					// cross-cutting writer logic untouched.
					var rb strings.Builder
					rb.WriteString(rawStart)
					rb.WriteString(backLog.String())
					rb.WriteString("</")
					writeElementName(&rb, t.Name)
					rb.WriteString(">")
					return []textRun{{
						text:        "\uE108:fldChar",
						props:       props,
						data:        rb.String(),
						srcRunStart: true,
					}}, nil
				}
				if len(runs) > 0 {
					// Mark the first emitted textRun with the source-run
					// boundary so downstream merging and the writer can keep
					// the original <w:r> envelope visible (e.g. a leading
					// <w:br/> in a fresh source <w:r> must NOT inline into
					// the preceding text's run \u2014 see textRun.srcRunStart).
					runs[0].srcRunStart = true
				}
				return runs, nil
			}
		}
	}
}
