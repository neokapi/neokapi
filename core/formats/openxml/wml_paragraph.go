// Paragraph parsing: the w:p element loop that collects a paragraph's runs,
// properties and structure, together with the revision wrappers (w:ins,
// w:moveTo, …) and smart tags that nest inside it.

package openxml

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// parseParagraph parses a <w:p> element and emits a Block if it contains text.
func (p *wmlParser) parseParagraph(d *xml.Decoder, partPath string, emitBlock func(*model.Block)) error {
	// Reset per-paragraph style-chain context. parseRunPropsFromRaw
	// consults p.currentStyleChainNames during minifyRPrChildren —
	// see the field declaration on wmlParser for the upstream-Okapi
	// citation. The reset is mandatory: an earlier paragraph in the
	// same part may have set this for its own pStyle, and leaking
	// that chain into a sibling paragraph would falsely preserve
	// explicit-off WPML toggles whose parent style chain does NOT
	// actually carry them. We restore the prior value on return so
	// nested paragraph parsers (e.g. textbox / table-cell recursion
	// reusing this method) see their parent's context again — though
	// the current wmlParser doesn't recurse paragraphs through
	// parseParagraph, the save/restore keeps the contract clean.
	savedStyleChainNames := p.currentStyleChainNames
	p.currentStyleChainNames = nil
	defer func() { p.currentStyleChainNames = savedStyleChainNames }()

	var runs []textRun
	var hyperlinkRuns []textRun
	var inHyperlink bool
	var hyperlinkID string
	// hyperlinkAttrs captures every attribute on the <w:hyperlink>
	// start element other than `r:id` so the writer can re-emit them
	// verbatim. ECMA-376-1 §17.16.22 (CT_Hyperlink) defines tooltip,
	// history, anchor, docLocation, tgtFrame; upstream Okapi preserves
	// the start element verbatim via RunContainer.startMarkup
	// (RunContainer.java:97-99, getEvents() lines 168-176) and does NOT
	// synthesise the `href` attribute the native writer was emitting.
	var hyperlinkAttrs []xml.Attr
	var paraProps string
	var paraStyleID string
	// Use the parser-wide complex-field state so begin/end pairs that
	// straddle paragraph boundaries carry the correct extractable flag
	// across `<w:p>` borders. Mirrors upstream Okapi
	// parseComplexField (RunParser.java:461-542) which reads through
	// the entire event stream — paragraph boundaries between begin and
	// end land in deferredEvents (lines 508-514) rather than splitting
	// the field into independent state machines. Fixture
	// 1083-date-and-hyperlink-instructions.docx hits this path: the
	// `A link` run lives in its own `<w:p>` inside a non-extractable
	// DATE field and must not be extracted.
	cfs := &p.partCfs
	// Snapshot the complex-field state at paragraph entry so the
	// cross-paragraph absorption guards can distinguish "field opened
	// DURING this paragraph" (e.g. 1102.docx P2 which contains the
	// fldChar-begin + separate, leaving cfs.active=true at paragraph
	// close) from "field already open BEFORE this paragraph" (e.g.
	// 847-3.docx P2 which sits between P1's fldChar-separate and P3's
	// fldChar-end). Upstream Okapi's `mergeable` flag on a Block is
	// driven solely by the block's own pPr (BlockParser.java:207-213)
	// — it knows nothing about complex-field state. But cross-block
	// absorption only fires in StyledTextPart.process when the block
	// is actually built as a separate Block; when fldChar-begin opens
	// mid-paragraph, RunParser.parseComplexField consumes subsequent
	// paragraph events as opaque markup inside the SAME RunBuilder
	// (RunParser.java:461-542) and no separate Block is built for the
	// inner paragraphs — so mergeable absorption never fires for them.
	// We mirror that by allowing delMark absorption only when the
	// field was already open at paragraph entry (passthrough case).
	cfsActiveAtEntry := cfs.active
	cfsExtractableAtEntry := cfs.extractable
	cfsAtResultAtEntry := cfs.atResult
	var bms bookmarkSkipState

	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "pPr":
				// Capture paragraph properties for skeleton, extracting pStyle if present
				raw, styleID, err := captureParaProps(d, t)
				if err != nil {
					return err
				}
				// When the paragraph sits inside a NON-extractable
				// complex field's display area (between separate and
				// end of an unsupported-code field, e.g. DATE), upstream
				// Okapi captures the entire paragraph as raw markup
				// inside the field's RunBuilder via parseContent →
				// runBuilder.addToMarkup (RunParser.java:501-506) so
				// the source's pPr/rPr structure survives verbatim
				// regardless of upstream's normal `BlockProperties.
				// Default.getEvents` empty-collapse rule (BlockProperties.
				// java:169-172). For extractable fields and ordinary
				// paragraphs, ParagraphBlockProperties (line 302-304)
				// emits the inner rPr wrapper unconditionally only when
				// the wrapping pPr already had non-empty content — an
				// originally-skippable-only `<w:rPr>` collapses to a
				// missing wrapper instead. To match the non-extractable
				// path on round-trip, mark the captured pPr's inner rPr
				// with the keep-empty marker so the writer's
				// stripWMLSkippableElements pass leaves it in place even
				// after lang/noProof stripping. Fixture
				// 1083-date-and-hyperlink-instructions.docx paragraph 3
				// is the canonical case: a `<w:pPr><w:rPr><w:lang/>
				// </w:rPr></w:pPr>` shell inside a DATE field's display
				// area must round-trip as `<w:pPr><w:rPr></w:rPr>
				// </w:pPr>`.
				if cfs.active && !cfs.extractable && cfs.atResult {
					raw = markPPrInnerRPrKeepEmpty(raw)
				}
				paraProps = raw
				paraStyleID = styleID
				// Resolve the style chain's rPr-child-name set so
				// parseRunPropsFromRaw → minifyRPrChildren can honour
				// upstream Okapi's
				// `preCombined.contains(p.getName())` clearing-toggle
				// guard (RunProperties.java:497-540). When the
				// paragraph has no pStyle, docDefaults alone still
				// contribute names.
				if p.styles != nil {
					p.currentStyleChainNames = p.styles.effectiveRPrChildNames(paraStyleID)
				}

			case "r":
				// Text run — may contain fldChar/instrText for complex
				// fields. parseRunWithFieldState collapses such runs to
				// a single SubTypeFieldChar sentinel carrying the raw
				// <w:r>...</w:r>; surface them through the field-aware
				// keep/drop logic below.
				rawStart := startElementToRaw(t)
				run, err := p.parseRunWithFieldState(d, cfs, rawStart)
				if err != nil {
					return err
				}
				run = filterFieldRuns(run, cfs)
				// If we're inside a non-extractable complex field, drop
				// any plain text runs (the field-markup sentinel runs
				// have already been retained by filterFieldRuns); only
				// the cached display text from non-extractable fields is
				// suppressed per upstream Okapi
				// (RunParser.parseComplexField, lines 501-506).
				if cfs.active && !cfs.extractable {
					run = dropTextRuns(run)
				}
				// If we're inside an extractable field but before the
				// separator, drop translatable text but keep field
				// markup (begin / instrText / separate sentinels).
				if cfs.active && cfs.extractable && !cfs.atResult {
					run = dropTextRuns(run)
				}
				if len(run) == 0 {
					continue
				}
				if inHyperlink {
					hyperlinkRuns = append(hyperlinkRuns, run...)
				} else {
					runs = append(runs, run...)
				}

			case "hyperlink":
				// Inside a NON-extractable complex field's display area
				// (e.g. TOC \h \z \u — code "TOC" is not in
				// tsComplexFieldDefinitionsToExtract by default), every
				// event flows through runBuilder.addToMarkup verbatim per
				// upstream Okapi RunParser.parseComplexField (lines 501-
				// 506 of okapi/filters/openxml/src/main/java/net/sf/okapi/
				// filters/openxml/RunParser.java). The `<w:hyperlink>`
				// subtree — including the inner `<w:r><w:t>...</w:t></w:r>`
				// chain and any nested PAGEREF field markup — is preserved
				// as opaque markup; nothing inside it is extracted as
				// translatable text. ECMA-376-1 §17.16.22 (CT_Hyperlink)
				// places `<w:hyperlink>` as a direct `<w:p>` child, so we
				// reuse the U+E108 field-markup sentinel which the writer
				// emits verbatim with NO `<w:r>` wrapper.
				//
				// Without this branch, the standard hyperlink path opens
				// inHyperlink=true and routes inner runs through
				// dropTextRuns (since the surrounding field is non-
				// extractable) — the inner `<w:t>Text of Heading 1</w:t>`
				// is dropped, and the U+E103/U+E104 paired-code sentinels
				// emitted by wrapHyperlinkRuns hit the all-sentinel
				// `isEmptyRuns` branch where `runToXML` lacks an open/
				// close hyperlink case and falls through to the default
				// `<w:t>` text emit — exactly the apissue.docx /
				// docxsegtest.docx / docxtest.docx / table of contents -
				// automatic.docx divergence in the parity report
				// (offset-833 native `<pStyle>TOC1` vs ref `<hyperlink>`).
				if cfs.active && !cfs.extractable {
					raw, err := captureRawElement(d, t)
					if err != nil {
						return err
					}
					runs = append(runs, textRun{text: ":hyperlinkOpaque", data: raw})
					continue
				}
				inHyperlink = true
				hyperlinkID = attrVal(t, "id")
				hyperlinkAttrs = hyperlinkAttrs[:0]
				for _, a := range t.Attr {
					// Skip r:id — wrapHyperlinkRuns re-emits it from
					// the hyperlinkID we just captured.
					if a.Name.Local == "id" {
						continue
					}
					hyperlinkAttrs = append(hyperlinkAttrs, a)
				}
				hyperlinkRuns = nil

			case "bookmarkStart", "bookmarkEnd":
				// Bookmarks are direct children of <w:p> per ECMA-376
				// Part 1 §17.13.6 (Bookmarks). They are cross-structure
				// markers that delimit a named range; the markers can
				// span runs, paragraphs, tables, and even sections, so
				// they must be preserved verbatim at the position they
				// appear in the source.
				//
				// Mirrors upstream Okapi
				// SkippableElements.BookmarkCrossStructure
				// (SkippableElements.java lines 300-331) and
				// BlockSkippableElements.skip (BlockSkippableElements.java
				// lines 116-121): the `_GoBack` bookmark — Word's auto-
				// generated "return-to-last-edit" bookmark — is
				// silently skipped (start AND its matching end by id),
				// every other bookmark falls through to be added as
				// inline markup on the block.
				bookmark, captured, err := p.captureBookmark(d, t, &bms)
				if err != nil {
					return err
				}
				if !captured {
					continue
				}
				if inHyperlink {
					hyperlinkRuns = append(hyperlinkRuns, bookmark)
				} else {
					runs = append(runs, bookmark)
				}

			case "commentRangeStart", "commentRangeEnd":
				// Comment range markers are direct children of <w:p>
				// per ECMA-376 Part 1 §17.13.4.4 (CT_MarkupRange) and
				// §17.13.4.3 (CT_MarkupRangeStart). They delimit the
				// run-range that a comment annotates and must round-
				// trip verbatim so the commentReference run still has
				// a valid range to associate with. Upstream Okapi's
				// wordConfiguration.ymlbal classifies them as INLINE
				// rules (lines 59-63) — preserved as inline markup
				// chunks on the block, not as translatable text.
				//
				// We reuse the bookmark sentinel machinery: capture
				// the element verbatim, tag with a comment-range
				// sentinel char ( / ), and let the writer
				// re-emit the raw XML at the original position so the
				// commentRangeStart/end pair survives a round-trip
				// without being absorbed into a neighbouring <w:r>.
				marker, err := p.captureCommentRangeMarker(d, t)
				if err != nil {
					return err
				}
				if inHyperlink {
					hyperlinkRuns = append(hyperlinkRuns, marker)
				} else {
					runs = append(runs, marker)
				}

			case "proofErr", "permStart", "permEnd":
				if err := skipElement(d); err != nil {
					return err
				}

			case "sdt":
				// Inline structured document tag — capture wrapper as
				// paired-code sentinels around inner runs so the
				// `<w:sdt>...</w:sdt>` envelope plus `<w:sdtPr>`,
				// `<w:sdtEndPr/>`, `<w:sdtContent>` round-trip on the
				// wire. ECMA-376-1 §17.5.2 (Structured Document Tags);
				// upstream Okapi RunContainer.java:97-176 preserves the
				// outer markup as paired startMarkup / endMarkup events
				// around the extracted inner content.
				rawStart := startElementToRaw(t)
				target := &runs
				if inHyperlink {
					target = &hyperlinkRuns
				}
				if err := p.parseInlineSDT(d, target, rawStart); err != nil {
					return err
				}

			case "smartTag":
				// <w:smartTag> is a transparent run-container per
				// ECMA-376 Part 1 §17.5.1.9 and upstream Okapi
				// RunContainer (RunContainer.java lines 29-43,
				// 187-191). Drain the wrapper, processing inner
				// runs as if they were direct children of <w:p>;
				// the start/end tags are preserved verbatim as
				// paired-code sentinels around the inner runs.
				rawStart := startElementToRaw(t)
				target := &runs
				if inHyperlink {
					target = &hyperlinkRuns
				}
				if err := p.parseSmartTag(d, target, cfs, rawStart); err != nil {
					return err
				}

			case "ins", "moveTo":
				// Revision-tracking content wrapper: insertion / move-to.
				// Mirrors okapi's SkippableElements.RevisionInline.skip
				// (lines 209-212 of okapi/filters/openxml/src/main/java/
				// net/sf/okapi/filters/openxml/SkippableElements.java)
				// which returns early without skipping for INSERTED_CONTENT
				// and MOVED_CONTENT_TO — i.e. the wrapper is unwrapped and
				// its child runs are kept (the auto-accept-revisions
				// default semantics: insertions are accepted into the
				// final document).
				//
				// Process child <w:r> runs as if they were direct
				// children of <w:p> by handing them off to the run
				// parser inline.
				if err := p.parseRevisionInsertion(d, t.Name.Local, &runs, cfs, t); err != nil {
					return err
				}

			case "del", "moveFrom":
				// Revision-tracking content wrapper: deletion / move-from.
				// Auto-accept-revisions drops the entire subtree (deleted
				// content is removed from the final document). Per
				// SkippableElements.RevisionInline at lines 213-214 of
				// SkippableElements.java this falls through to the default
				// skip path. The skipElement walker discards the subtree
				// entirely, including any nested <w:r><w:delText>...
				// </w:delText></w:r> runs.
				if err := skipElement(d); err != nil {
					return err
				}

			case "oMathPara", "oMath":
				// Math content (Office Math Markup Language, OMML —
				// ECMA-376 Part 1 §22.1). Word may emit <m:oMathPara>
				// or <m:oMath> as a direct child of <w:p>, not wrapped
				// in <w:r>. Okapi's MathSymbol / MathBlock parsers
				// preserve the entire OMML subtree opaquely — text
				// inside m:t is mathematical typography, not natural
				// language — so we capture the raw XML as a sentinel
				// run (TypeImage) so the writer round-trips the
				// equation byte-for-byte. equation.docx is the
				// canonical fixture.
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				runs = append(runs, textRun{text: "", data: raw})

			case "AlternateContent":
				// Paragraph-level mc:AlternateContent (rare but legal:
				// some authoring tools emit it as a <w:p> child rather
				// than a <w:r> child). Same MCE semantics as the
				// run-level handler — keep the wrapper + selected
				// Choice, drop Fallback. ECMA-376 Part 3 §10. See
				// captureAlternateContent for citations. Tagged with the
				// paragraph-level sentinel  so runToXML emits it
				// without wrapping in <w:r>.
				raw, err := captureAlternateContent(d, t)
				if err != nil {
					return err
				}
				runs = append(runs, textRun{text: "", data: raw})

			case "fldSimple":
				// Simple field — `<w:fldSimple w:instr="...">...</
				// w:fldSimple>` per ECMA-376 Part 1 §17.16.6. Per
				// upstream Okapi the entire fldSimple element is
				// gathered and flushed as a single opaque markup chunk
				// (BlockParser.parse lines 242-250 of okapi/filters/
				// openxml/src/main/java/net/sf/okapi/filters/openxml/
				// BlockParser.java); nothing inside is treated as
				// translatable. Mirror that here: capture the whole
				// element raw and hand it to the block as a
				// SubTypeFieldSimple sentinel so the writer emits it
				// verbatim with no modifications.
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				// Protect every nested <w:rPr> inside the captured
				// payload from the writer's stripWMLSkippableElements
				// pass: Okapi's BlockParser routes fldSimple through
				// the gather-events-into-markup path (lines 242-250 of
				// okapi/filters/openxml/src/main/java/net/sf/okapi/
				// filters/openxml/BlockParser.java) which preserves the
				// inner runs verbatim — no skippable-element stripping
				// applied. So inner rPrs that carry only `<w:noProof/>`
				// (e.g. AUTHOR cached-result run in Document-with-
				// formula-and-tabs.docx) need to round-trip with the
				// noProof intact, not stripped + empty-rPr-collapsed.
				raw = protectFieldPayloadFromStripping(raw)
				if inHyperlink {
					hyperlinkRuns = append(hyperlinkRuns, textRun{text: ":fldSimple", data: raw})
				} else {
					runs = append(runs, textRun{text: ":fldSimple", data: raw})
				}

			default:
				if err := skipElement(d); err != nil {
					return err
				}
			}

		case xml.EndElement:
			if t.Name.Local == "hyperlink" {
				if inHyperlink && len(hyperlinkRuns) > 0 {
					runs = append(runs, p.wrapHyperlinkRuns(hyperlinkRuns, hyperlinkID, hyperlinkAttrs)...)
				}
				inHyperlink = false
				hyperlinkID = ""
				hyperlinkAttrs = hyperlinkAttrs[:0]
				continue
			}

			if t.Name.Local == "p" {
				// Apply style optimization: subtract inherited properties.
				// The inherited chain combines:
				//   1. The paragraph's pStyle chain (resolveProps walks
				//      basedOn from paraStyleID up).
				//   2. Each run's rStyle chain (a character style applied
				//      directly to the run; mergeProps overlays the
				//      character style's resolved rPr on top of the
				//      paragraph's resolved rPr per ECMA-376-1 §17.7.1
				//      (Style Inheritance) — character style wins over
				//      paragraph style for run-level properties).
				//
				// Without the rStyle merge, a directly-authored property
				// on a run that matches what the rStyle chain provides
				// (e.g. 948-1.docx's `Character1`-styled run carries
				// `rFonts ascii=Calibri ...` AND the Character1 style
				// chain already supplies the same rFonts) is NOT seen
				// as redundant by subtractProps — the run keeps the
				// duplicate rPr child and the writer emits it on the
				// wire even though upstream Okapi (which DOES walk the
				// rStyle chain at minified() time) drops it. Mirrors
				// upstream Okapi's CombinedRunProperties.combine
				// (RunProperties.java:497-540) which builds preCombined
				// from BOTH the pStyle chain and the rStyle chain
				// before computing minified().
				if p.styles != nil {
					var paraStyleProps runProps
					if paraStyleID != "" {
						paraStyleProps = p.styles.resolveProps(paraStyleID)
					}
					paraChainNames := p.currentStyleChainNames
					// When the paragraph has NO `<w:pPr>` element,
					// `currentStyleChainNames` is still nil from the
					// per-paragraph reset above. Resolve it now so the
					// chain-aware run-prop strips below see the
					// docDefaults baseline. ECMA-376-1 §17.3.1.10
					// (CT_P): a paragraph without pPr inherits all
					// properties from the default paragraph style;
					// styleMap.effectiveRPrChildNames("") already
					// handles the empty-paraStyleID fallback. Without
					// this, fixtures whose paragraphs omit pPr (e.g.
					// docxsegtest.docx P11) would see chainNames=nil
					// and the chain-absent szCs strip below would
					// incorrectly fire on a chain that DOES carry szCs
					// via docDefaults.
					if paraChainNames == nil {
						paraChainNames = p.styles.effectiveRPrChildNames(paraStyleID)
					}
					for i := range runs {
						if isSentinel(runs[i].text) {
							continue
						}
						rStyleID := extractRStyleID(runs[i].props.rPrChildren)
						styleProps := paraStyleProps
						chainNames := paraChainNames
						if rStyleID != "" {
							rStyleProps := p.styles.resolveProps(rStyleID)
							mergeProps(&styleProps, rStyleProps)
							// Compute the merged chain-name set so
							// minifyRPrChildren's preCombined-by-name
							// guard can see properties contributed by
							// the rStyle chain (e.g. lang.docx's
							// `editform` character style supplies
							// <w:vanish/>; without folding it into
							// chainNames, an explicit-off
							// `<w:vanish w:val="0"/>` on a Character1-
							// styled run looks like a no-op default
							// and gets stripped, breaking lang.docx).
							chainNames = mergeChainNames(paraChainNames, p.styles.effectiveRPrChildNames(rStyleID))
						}
						subtractProps(&runs[i].props, styleProps)
						// Re-run minifyRPrChildren with the merged
						// per-run chain (paraStyle ∪ rStyle). The
						// initial pass in parseRunProps used only the
						// paragraph's chain; if the rStyle adds new
						// names (e.g. <w:vanish/> on `editform`), an
						// explicit-off entry that the parse-time
						// minify dropped should now be preserved, and
						// vice-versa. Mirrors upstream Okapi's late
						// minified() invocation that operates on the
						// FULL preCombined view (RunParser.java:280-294
						// + RunProperties.java:497-540).
						runs[i].props.rPrChildren = minifyRPrChildren(runs[i].props.rPrChildren, chainNames)
						// Strip an explicit-off `<w:vanish w:val="0"/>`
						// from the run's rPrChildren when the merged
						// style chain does not author <w:vanish/> by
						// name. ECMA-376-1 §17.3.2.42 (<w:vanish>):
						// the toggle defaults OFF, so an explicit-off
						// authoring is redundant unless the inherited
						// chain turns it ON. Mirrors upstream Okapi
						// RunProperties.minified() default-strip
						// (RunProperties.java:497-540) on the
						// PreCombined view that includes both pStyle
						// AND rStyle chains. Vanish is excluded from
						// `wpmlToggleNames` so the parse-time minify
						// (which only sees the paragraph chain) never
						// strips the clearing form prematurely; this
						// late strip runs only when both chains have
						// been merged. Fixtures: 948-1.docx ($
						// `Character1`-styled run carries vanish=0 but
						// Character1 chain has no vanish — drop it),
						// lang.docx (editform-styled run carries
						// vanish=0 AND editform supplies <w:vanish/> —
						// keep it).
						if !chainNames["vanish"] {
							runs[i].props.rPrChildren = stripExplicitOffVanish(runs[i].props.rPrChildren)
						}
						// Strip `<w:szCs/>` from a non-CS run when the
						// merged chain has no szCs by name. Mirrors the
						// `else { v = true }` branch of upstream Okapi's
						// RunParser.canBeSkipped (RunParser.java:236-250)
						// feeding the no-CS-text strip at
						// RunParser.java:226-228 (skips
						// RUN_PROPERTY_COMPLEX_SCRIPT_FONT_SIZE when
						// !runFonts.containsDetectedComplexScriptContentCategories
						// AND the chain doesn't carry a szCs to compare
						// against). Per ECMA-376-1 §17.3.2.39 (szCs)
						// the property is the complex-script side of
						// `<w:sz>` (§17.3.2.38) and is a no-op duplicate
						// when neither the chain nor the run text is CS-
						// bearing. The other half (chain HAS szCs +
						// values match) is handled by the chain-XML-
						// match strip below. MissingPara.docx is the
						// canonical case — every translatable paragraph
						// has runs with `<w:szCs val="…"/>` on ASCII
						// text, the chain carries no szCs, and upstream
						// strips them at parse time so WSO sees empty-
						// rPr runs and synthesises no spurious `Normal2`
						// style.
						if !chainNames["szCs"] && !containsComplexScriptText(runs[i].text) {
							runs[i].props.rPrChildren = stripChainAbsentSzCs(runs[i].props.rPrChildren)
						}
						// Strip per-run rPrChildren whose canonical XML
						// matches the resolved style chain. Mirrors
						// upstream Okapi RunProperties.minified()'s
						// `if (preCombined.contains(p))` branch
						// (RunProperties.java:497-540): a directly-
						// authored property is dropped from the run
						// when the resolved chain already supplies it
						// with the SAME value (Property.equals via
						// RunProperty.equalsProperty implementations).
						// Native captures the chain side via
						// styleEntry.rPrChildXMLs (parseStyles writes
						// the canonical w:-prefixed XML for every rPr
						// child the style authors) and the run side
						// via rPrChild.xml (parseRunProps writes the
						// matching wmlPrefixed form via
						// serializeRPrChildElement /
						// serializeWithCapture). When both sides match
						// byte-for-byte, the run-level entry is a
						// no-op duplicate of the inherited chain and
						// gets dropped — fixture HiddenTablesApachePoi
						// is the canonical case (per-run
						// `<w:outline w:val="0"/>` matches the `Body`
						// pStyle's chain `<w:outline w:val="0"/>` from
						// docDefaults; without this strip native lifts
						// outline=0 into the synth NF974E24F-Body1
						// style's rPr). Per ECMA-376-1 §17.3.2 every
						// rPr child element is identified by its name
						// + attribute set (no character data content),
						// so byte-equal canonicalised XML is a safe
						// equality check.
						//
						// Excluded names: rStyle (the run's character-
						// style reference is not a Property.equals-
						// minifiable entry — upstream's
						// RunProperties.minified() filter at line 497
						// of RunProperties.java explicitly excludes
						// StyleRunProperty via the
						// `combineDistinct(.. !(p instanceof
						// StyleRunProperty))` branch in
						// combinedRunProperties, and the chain
						// matching here would falsely match an rStyle
						// reference against itself).
						if rStyleID != "" || paraStyleID != "" {
							children := runs[i].props.rPrChildren
							out := children[:0]
							for _, c := range children {
								if c.name == "rStyle" {
									out = append(out, c)
									continue
								}
								chainXML := ""
								if rStyleID != "" {
									chainXML = p.styles.effectiveRPrChildXML(rStyleID, c.name)
								}
								if chainXML == "" && paraStyleID != "" {
									chainXML = p.styles.effectiveRPrChildXML(paraStyleID, c.name)
								}
								if chainXML != "" && chainXML == c.xml {
									continue
								}
								out = append(out, c)
							}
							runs[i].props.rPrChildren = out
						}
					}
				}

				// Apply font mapping: normalize font names to script groups for merging
				if len(p.cfg.FontMappings) > 0 {
					for i := range runs {
						if runs[i].props.fontName != "" {
							if group, ok := p.cfg.FontMappings[runs[i].props.fontName]; ok {
								runs[i].props.fontName = group
							}
						}
					}
				}

				// Cross-paragraph absorption (deleted paragraph mark).
				// When a previous paragraph in this part carried
				// `<w:pPr><w:rPr><w:del/></w:rPr></w:pPr>` (ECMA-376
				// Part 1 §17.13.5.13 CT_ParaRPr) AND had non-empty
				// translatable content, its runs were buffered on
				// `p.partMergeable` rather than written. Under
				// auto-accept-revisions the deleted paragraph mark
				// removes the paragraph break, so its content collapses
				// into the FOLLOWING paragraph. Prepend those buffered
				// runs to the receiver's `runs` slice now — this
				// mirrors upstream Okapi `Block.mergeWith` (Block.java
				// lines 144-154) which inserts the mergeable block's
				// middle chunks (chunks 1..N-1, i.e. the run chunks
				// without the paragraph open/close markup) into the
				// receiver block ahead of the receiver's own runs via
				// `chunks.listIterator(1)`.
				//
				// The buffered runs already went through THEIR
				// original paragraph's style subtraction loop above
				// (each set of runs is subtracted against its OWN
				// pStyle before being buffered); prepending here —
				// AFTER the receiver's subtraction loop — keeps each
				// run subtracted against its own paragraph's chain
				// rather than double-subtracting. This matches
				// upstream where mergeWith just splices already-built
				// Run chunks; `block.optimiseStyles()` runs once on
				// the merged whole at StyledTextPart line 320 but the
				// per-run minified() state was set by each run's own
				// BlockParser pass (RunParser.java:280-294 +
				// RunProperties.java:497-540).
				//
				// Re-running mergeRuns / commonRPrChildren on the
				// combined slice below is safe — both are idempotent
				// across already-merged groups and the boundary
				// between buffered and receiver runs is allowed to
				// fuse if their rPr is mergeable per
				// RunMerger.canRunPropertiesBeMerged
				// (RunMerger.java:156-229).
				//
				// Fixtures: 847-2.docx, 847-3.docx, 1102.docx.
				if p.partMergeable != nil {
					prepended := make([]textRun, 0, len(p.partMergeable.runs)+len(runs))
					prepended = append(prepended, p.partMergeable.runs...)
					prepended = append(prepended, runs...)
					runs = prepended
					p.partMergeable = nil
				}

				// Compute the per-paragraph common rPr children BEFORE
				// mergeRuns collapses adjacent runs. mergeRuns drops the
				// rPrChildren of merged-away neighbours (it only keeps
				// the first run's props), so the intersection must be
				// taken across the original source runs.
				//
				// commonRPrChildren mirrors upstream Okapi
				// StyleOptimisation.commonRunPropertiesOf
				// (StyleOptimisation.java lines 204-237) — the set of
				// rPr child elements present and equal across every
				// translatable text run in the paragraph. The writer
				// emits these on every <w:r> for the block (#592). Native
				// is faithful: the rPr stays inline (no synthesised
				// paragraph style).
				commonRPr := commonRPrChildren(runs)
				commonRPrXML := joinRPrChildren(commonRPr)

				// Merge adjacent runs with mergeable rPr (mirrors
				// upstream Okapi RunMerger.canRunPropertiesBeMerged
				// at RunMerger.java:156-229). mergeRuns updates the
				// surviving textRun's rPrChildren to the merged
				// per-attribute union so the sidecars below see the
				// post-merge consensus props.
				merged := mergeRuns(runs)

				// Cross-paragraph absorption (deleted paragraph mark) —
				// content-bearing case. When this paragraph carries
				// `<w:pPr><w:rPr><w:del/></w:rPr></w:pPr>` (or
				// `<w:moveFrom/>`) AND has non-empty translatable
				// content, the paragraph break is part of a tracked
				// deletion (ECMA-376 Part 1 §17.13.5.13 CT_ParaRPr).
				// Under auto-accept-revisions the break is removed,
				// collapsing the paragraph's content into the FOLLOWING
				// paragraph. Mirrors upstream Okapi:
				//   - BlockParser.parse lines 207-213: marks the block
				//     mergeable when ParagraphBlockProperties.
				//     containsRunPropertyDeletedParagraphMark() is true
				//     (ParagraphBlockProperties.java lines 576-586).
				//   - StyledTextPart.process lines 312-319: buffers
				//     the block as `mergeableBlock`; when the next
				//     block arrives, calls `block.mergeWith(mergeableBlock)`
				//     (Block.java lines 139-166) which inserts the
				//     mergeable's middle chunks (chunks 1..N-1) into
				//     the receiver ahead of the receiver's own runs
				//     and discards the mergeable's pPr.
				//
				// We buffer the post-mergeRuns slice on `p.partMergeable`
				// and return without writing skeleton bytes for this
				// paragraph. The next parseParagraph invocation
				// prepends the buffer to its runs (above, before
				// commonRPrChildren) so commonRPrChildren / mergeRuns /
				// buildBlock all run on the combined slice. If no
				// successor paragraph absorbs the buffer, the EOF
				// flush in parsePart emits it as a standalone
				// paragraph (matching upstream's
				// StyledTextPart.process tail at lines 642-644 which
				// still emits the dangling mergeableBlock).
				//
				// The empty-content case (`len(merged) == 0`) is
				// handled by the existing branch below at the
				// `isEmptyRuns(merged)` check — that path drops the
				// paragraph entirely (no buffer set), matching
				// Block.mergeWith's `chunks.size() <= 2` short-circuit
				// at Block.java line 140 (a mergeable block whose
				// only chunks are paragraph open + close drops away).
				//
				// Fixtures: 847-2.docx, 847-3.docx exercise the
				// content-bearing buffer path; 1370-same-nested-
				// revisions.docx remains the empty-content drop case.
				//
				// Exception: when this paragraph ITSELF opens the
				// extractable complex field (fldChar-begin appears
				// inside, so cfs.active flipped from false → true
				// during parsing), upstream Okapi's
				// `<w:pPr>` events for any FOLLOWING paragraphs flow
				// through `RunParser.parseContent` as opaque markup
				// inside the field's RunBuilder
				// (RunParser.java:516-535) — they never reach
				// `BlockParser.parse`'s `containsRunPropertyDeletedParagraphMark`
				// check at line 207-213. But THIS paragraph IS its own
				// Block — built by BlockParser before fldChar-begin
				// triggered parseComplexField — so its mergeable flag
				// IS honoured by StyledTextPart.process at lines
				// 312-319. However, the very next "block" StyledTextPart
				// sees is NOT a separate paragraph (those are engulfed
				// by parseComplexField) but the next non-field paragraph
				// AFTER fldChar-end. For 1102.docx P2 this means
				// merging P2 into P5 (the bare trailing paragraph
				// after the field) — that path is already wired via
				// partFieldStraddle + partAbsorbedTrailingEmpty below;
				// the partMergeable mechanism is NOT the right one for
				// field-opener paragraphs.
				//
				// In contrast, when the field is already open at
				// paragraph entry (847-3.docx P2 — fldChar-begin was in
				// P1, fldChar-end is in P3), THIS paragraph is itself
				// engulfed by parseComplexField in upstream and never
				// reaches StyledTextPart as a separate Block. The
				// rendered output naturally fuses P2's content with the
				// surrounding field-display runs, so on the writer side
				// we absorb P2's runs into the NEXT paragraph (P3) via
				// the standard partMergeable plumbing — that is what
				// the diff demands. Allow buffering when the field was
				// open at paragraph entry (passthrough case).
				//
				// Fixtures: 847-3.docx P2 exercises the passthrough
				// absorb path; 1102.docx P2 exercises the field-opener
				// path that must NOT use partMergeable.
				openFieldAtEntry := cfsActiveAtEntry && cfsExtractableAtEntry && cfsAtResultAtEntry
				openedFieldThisPara := !openFieldAtEntry && cfs.active && cfs.extractable
				if paragraphHasDeletedMark(paraProps) && !isEmptyRuns(merged) && !openedFieldThisPara {
					p.partMergeable = &pendingMergeable{
						runs:        merged,
						paraProps:   paraProps,
						paraStyleID: paraStyleID,
					}
					return nil
				}

				// Capture per-text-run rPr fragments AFTER mergeRuns
				// so the sidecar aligns 1:1 with the model.TextRun
				// stream the writer emits. mergeRuns updates the
				// kept run's rPrChildren to the merged consensus, so
				// the post-merge fragment is the correct rPr to emit
				// for that <w:r>. Phase 1 only stashes the sidecar
				// on the block; Phase 2 wires it into the writer.
				// See PARITY_NOTES.md "1083-*" per-run rPr.
				perRunRPrXML := perRunRPrFragments(merged)
				// Capture per-text-run "starts new source <w:r>"
				// flags AFTER mergeRuns so the slice aligns 1:1
				// with the model.TextRun stream the writer sees
				// (mergeRuns preserves the srcRunStart of the
				// first run it keeps in a merge group).
				perRunSrcRunStart := perRunSrcRunStartFlags(merged)

				// Pre-extract translatable bits from any drawing
				// sentinel runs in this paragraph so they reach
				// the translation pipeline regardless of which
				// writer path handles the run later (the empty-
				// paragraph skeleton flush in writeDrawingXMLToSkel
				// already extracted, but the build-block path
				// below dumps Ph.Data verbatim through the
				// renderBlock TypeImage handler — without this
				// pre-extraction step, drawings inside paragraphs
				// that ALSO contain translatable text never get
				// their textbox/textpath content translated, e.g.
				// TextBoxes.docx and OutOfTheTextBox.docx).
				for i := range merged {
					if isDrawingSentinel(merged[i].text) && merged[i].data != "" {
						merged[i].data = p.extractDrawingTranslations(merged[i].data, partPath, emitBlock)
					}
				}

				// Skip empty paragraphs. A "non-translatable but
				// non-empty" paragraph (one whose only runs are
				// drawing/pict/object sentinels) still needs its
				// runs flushed to the skeleton so the embedded
				// markup survives the round-trip — losing
				// <w:drawing> here is the bug fixed in #590.
				if isEmptyRuns(merged) {
					// A paragraph whose only content is an OMML equation
					// (an <m:oMath>/<m:oMathPara> direct <w:p> child with no
					// translatable text) is written to skeleton below — verbatim for
					// pure math, or as a sub-skeleton (writeOMathSubSkeleton, via the
					// writeRunToSkel intercept) that makes any embedded <m:nor/> prose
					// translatable when non-translatable-content surfacing is on.
					//
					// When that flag is on, ALSO emit a detached non-translatable
					// RoleFormula block carrying the equation's portable LaTeX/MathML
					// so cross-format export (markdown/DocLang) can render the whole
					// formula — the nor prose travels inside the LaTeX as \text{…}.
					// This block is NOT skeleton-referenced, so the docx round-trip
					// (which replays the paragraph bytes / nor refs from skeleton)
					// stays byte-exact; parity forces the flag off, so the part stream
					// is unchanged there.
					if p.cfg != nil && p.cfg.ExtractNonTranslatableContent() {
						for _, r := range merged {
							if r.data == "" || !strings.HasPrefix(r.data, "<m:oMath") {
								continue
							}
							equiv, disp := ommlToMathEquiv(r.data)
							if equiv == "" {
								continue
							}
							*p.blockCounter++
							blk := model.NewBlock(fmt.Sprintf("tu%d", *p.blockCounter), "")
							blk.Name = p.path.name(p.path.reserve("oMath"))
							blk.Translatable = false
							blk.Type = "math"
							blk.SetSemanticRole(model.RoleFormula, 0)
							blk.Source = []model.Run{{Ph: &model.PlaceholderRun{
								ID: "c1", Type: TypeOpaqueParaChild, SubType: SubTypeOMath,
								Data: r.data, Equiv: equiv, Disp: disp,
							}}}
							emitBlock(blk)
						}
						// The same treatment for a paragraph whose only content
						// is a drawing. An image-only paragraph carries no
						// translatable text, so it is not a text unit and no
						// block is emitted — correct for extraction, but it also
						// means the picture is absent from every cross-format
						// export, while its name and alt text leak out
						// separately as loose metadata paragraphs.
						//
						// Emit a detached non-translatable RolePicture block
						// carrying the drawing's placeholder run, whose Attrs
						// now hold the resolved part name and alt text. Not
						// skeleton-referenced, so the docx round-trip still
						// replays the paragraph bytes verbatim; parity forces
						// the flag off, so the part stream is unchanged there.
						for _, r := range merged {
							if !isDrawingSentinel(r.text) || r.data == "" {
								continue
							}
							attrs := p.drawingAttrs(r.data)
							if attrs[model.AttrSrc] == "" {
								continue
							}
							*p.blockCounter++
							blk := model.NewBlock(fmt.Sprintf("tu%d", *p.blockCounter), "")
							blk.Name = p.path.name(p.path.reserve("drawing"))
							blk.Translatable = false
							blk.Type = "picture"
							blk.SetSemanticRole(model.RolePicture, 0)
							blk.Source = []model.Run{{Ph: &model.PlaceholderRun{
								ID: "c1", Type: TypeImage, SubType: SubTypeImage,
								Data: r.data, Attrs: attrs,
							}}}
							emitBlock(blk)
						}
					}
					// Field-straddle absorption (fldChar-end-only
					// paragraph). When the previous paragraph buffered
					// itself as a `pendingFieldBlock` (display content
					// + cfs.active+extractable+atResult at close) AND
					// THIS paragraph carries only a lone fldChar-end
					// sentinel (no other fldChar / instrText / display
					// content — strictly the field's closing marker),
					// upstream Okapi absorbs that tail run back into
					// the prior block via `parseComplexField`'s
					// deferred-events `goesAfterAnotherRun=true`
					// branch (RunParser.java:594-598): the fldChar-end
					// markup lands BEFORE the deferred pEnd events, so
					// the rendered output places it in the previous
					// paragraph. The paragraph that originally held
					// the fldChar-end survives as a structural shell —
					// `<w:p>pPr</w:p>`.
					//
					// Fixtures: 1172.docx P3 (only fldChar-end +
					// _GoBack bookmark — bookmark filtered, fldChar-
					// end remains as the only run) and 1341-textbox-
					// with-a-hyperlink.docx textbox P2 (only
					// fldChar-end). ECMA-376-1 §17.16.5 (CT_FldChar)
					// defines fldChar children of <w:r>; §17.16.18
					// (HYPERLINK field instructions) defines the field
					// code extracted by `complexFieldCodeName`.
					//
					// The fldChar-end-only check `allFldCharEndOnly`
					// excludes placeholder-only sentinels (empty
					// `<w:r><w:rPr>...</w:rPr></w:r>` inside an open
					// field — 830-2.docx P2, 830-6.docx P2) because
					// upstream keeps those placeholders in their
					// source paragraph and the writer-side
					// `pullLeadingFldCharEndIntoPrevParagraph` post-
					// pass migrates the fld-end into them.
					if p.partFieldStraddle != nil && allFldCharEndOnly(merged) {
						if err := p.flushPendingFieldBlock(merged, partPath, emitBlock); err != nil {
							return err
						}
						// Suppress deletedMark-bearing pPr — see
						// stripPPrIfDeletedMark for the BlockParser.java:
						// 207-213 citation.
						emitParaProps := stripPPrIfDeletedMark(paraProps)
						p.skelWriteString("<w:p>")
						if emitParaProps != "" {
							p.skelText(emitParaProps)
						}
						p.skelWriteString("</w:p>")
						return nil
					}
					// If a field-straddle buffer is pending but THIS
					// paragraph is not the fldChar-end-only absorber
					// (e.g. an empty paragraph with deletedMark sitting
					// between the field-display paragraph and the
					// field-end paragraph — 1102.docx P3, or a
					// placeholder-only paragraph — 830-2.docx P2), flush
					// the buffer first so the buffered block emits
					// BEFORE this paragraph in document order. The
					// current paragraph then proceeds through the
					// existing empty-runs path (its placeholder
					// content reaches the skeleton via writeRunToSkel).
					if p.partFieldStraddle != nil {
						if err := p.flushPendingFieldBlock(nil, partPath, emitBlock); err != nil {
							return err
						}
					}
					// Tracked deletion of the paragraph mark
					// (ECMA-376 Part 1 §17.13.5.13 CT_ParaRPr):
					// when <w:pPr><w:rPr> carries <w:del> or
					// <w:moveFrom>, the paragraph break itself is
					// deleted and the (empty) paragraph collapses
					// into the next one under auto-accept-revisions.
					// Mirror upstream Okapi's mergeable-block path
					// (BlockParser.parse lines 207-213 +
					// StyledTextPart.process lines 312-319 +
					// Block.mergeWith short-circuit on chunks<=2 at
					// Block.java line 140): a mergeable block whose
					// only chunks are markup-start + markup-end is
					// dropped entirely. Fixture
					// 1370-same-nested-revisions.docx is the
					// canonical case.
					//
					// Exception (same rationale as the content-bearing
					// branch above): when an extractable complex field
					// is OPEN across this paragraph boundary, inner
					// pPr events are opaque markup to upstream's
					// BlockParser — the deletedMark drop does not fire.
					// Fixture 1102.docx: P3 is empty with deletedMark
					// in pPr and sits between the HYPERLINK field's
					// separate (P2) and end (P4); reference keeps P3
					// as an empty paragraph rather than dropping it.
					if paragraphHasDeletedMark(paraProps) && len(merged) == 0 && !(cfs.active && cfs.extractable) {
						return nil
					}
					// Structural absorption target for a delMark-bearing
					// partFieldStraddle that already flushed (see
					// partAbsorbedTrailingEmpty field doc and
					// flushPendingFieldBlock). Upstream Okapi marks the
					// absorbed cross-field block `mergeable=true`
					// (BlockParser.java:207-213) and consumes the NEXT
					// non-mergeable paragraph as the wrapper
					// (StyledTextPart.process lines 312-319 +
					// Block.mergeWith at Block.java:139-166). The
					// trailing paragraph itself is dropped — its content
					// (none) and its wrapper hold the absorbed runs.
					// Mirror that here by silently dropping a plain
					// empty `<w:p ...>` (no pPr, no body) sitting between
					// the flushed straddle and the section properties.
					// Fixture 1102.docx P5 is the canonical case (source
					// `<w:p w14:paraId="2E5C8AD6" .../>` immediately
					// before `<w:sectPr>`).
					//
					// Guarded on `len(paraProps) == 0` so paragraphs that
					// carry their OWN pPr (rare in this position, but
					// distinct from the structural shell) still emit.
					// Also gated on `!cfs.active` so we don't drop a
					// placeholder paragraph that the field machinery
					// still expects to flow through (e.g. fldChar-end-
					// only sole-run cases handled above).
					if p.partAbsorbedTrailingEmpty && paraProps == "" && !cfs.active {
						p.partAbsorbedTrailingEmpty = false
						return nil
					}
					// When the captured pPr/rPr carries a deletedMark
					// (`<w:del>` / `<w:moveFrom>`), upstream Okapi's
					// BlockParser (BlockParser.java:207-213) suppresses
					// the pPr entirely — only the paragraph's `<w:p>`
					// shell survives. Mirror that here so the inner
					// rPr (which can carry a leftover `<w:rStyle>` etc.)
					// does not leak through. Fixture 1102.docx P3.
					emitParaProps := stripPPrIfDeletedMark(paraProps)
					p.skelWriteString("<w:p>")
					if emitParaProps != "" {
						p.skelText(emitParaProps)
					}
					// Fuse adjacent same-rPr opaque-drawing runs (each
					// was a separate `<w:r>` envelope in the source) so
					// they share one `<w:r>` envelope on output.
					// Mirrors upstream Okapi RunMerger
					// (RunMerger.java:83-95 +
					// canRunPropertiesBeMerged at :156-229): adjacent
					// source `<w:r>` envelopes with identical
					// RunProperties merge into one RunBuilder; the
					// resulting `<w:r>` carries each drawing as a
					// separate Markup body chunk under the shared rPr.
					// Per ECMA-376-1 §17.3.2.1 (CT_R) a single `<w:r>`
					// may carry multiple `<w:drawing>` / `<w:pict>` /
					// `<w:object>` children alongside one shared
					// `<w:rPr>`.
					//
					// Fixture neverendingloop.docx is the canonical
					// case: three adjacent `<w:r><w:rPr><w:sz val=
					// "40"/></w:rPr><w:pict>...</w:pict></w:r>`
					// envelopes that bridge fuses into one `<w:r>`
					// with all three picts.
					// Drop trivially-empty `<w:r><w:t></w:t></w:r>`
					// placeholders sitting alongside drawing-bearing
					// runs in an otherwise-content-empty paragraph.
					// Mirrors upstream Okapi RunMerger
					// (RunMerger.java:83-95): a RunBuilder whose
					// chunks list materialises only an empty Text
					// chunk does not survive the merge — the run is
					// dropped before flushBuilders. Per ECMA-376-1
					// §17.3.2.1 (CT_R) an empty `<w:r>` carrying a
					// single `<w:t/>` and no rPr children contributes
					// no formatting and no content, so dropping it is
					// a no-op for rendering. Fixture
					// AlternateContent.docx is the canonical case:
					// each AC-bearing paragraph ends with an empty
					// `<w:r><w:t xml:space="preserve"></w:t></w:r>`
					// trailing the drawings, which upstream drops.
					emptyDropped := merged[:0]
					for _, r := range merged {
						if isEmptyTextPlaceholder(r) {
							continue
						}
						emptyDropped = append(emptyDropped, r)
					}
					merged = emptyDropped
					i := 0
					for i < len(merged) {
						r := merged[i]
						// Simple run-children (text / tab / break /
						// footnote-ref) are emitted natively as <w:r>
						// envelopes via emitRunEnvelopes (#602): it groups
						// same-source splits AND fuses the cross-source
						// break->text adjacency, replacing the
						// fuseBareBrAndTextRuns post-serialization regex.
						// Drawings/opaque/paragraph-level sentinels are
						// NOT simple (see simpleRunChild) and fall through
						// to the drawing-fusion + writeRunToSkel paths
						// below, which they require for docPr/@name
						// attribute extraction.
						if simpleRunChild(r) {
							j := i + 1
							for j < len(merged) && simpleRunChild(merged[j]) {
								j++
							}
							p.skelText(emitRunEnvelopes(merged[i:j]))
							i = j
							continue
						}
						// Same-source-`<w:r>` group: when the next
						// merged run was emitted from the SAME source
						// `<w:r>` as r (its `srcRunStart` is false),
						// the source XML had multiple body children
						// (e.g. `<mc:AlternateContent>` + `<w:drawing>`
						// siblings) under one shared `<w:rPr>`. Per
						// ECMA-376-1 §17.3.2.1 (CT_R) a single `<w:r>`
						// may carry any combination of run children,
						// and upstream Okapi RunBuilder
						// (RunBuilder.java:73-188) materialises the
						// source `<w:r>` body chunks in order under one
						// envelope. Without this branch the output
						// splits the source `<w:r>` into N envelopes
						// (one per child) and the AC/drawing pair
						// loses its shared run boundary
						// (992.docx header1.xml canonical case: source
						// `<w:r><w:rPr/><mc:AlternateContent/>
						// <w:drawing/></w:r>` was being emitted as
						// `<w:r><AC/></w:r><w:r><drawing/></w:r>`).
						//
						// Both runs must be opaque sentinels ()
						// so we can splice their payloads safely;
						// non-opaque follower runs fall through to the
						// per-run path below.
						if isRunLevelOpaque(r) {
							j := i + 1
							for j < len(merged) {
								if merged[j].srcRunStart {
									break
								}
								if !isRunLevelOpaque(merged[j]) {
									break
								}
								j++
							}
							if j > i+1 {
								open, close := splitRunWrapper(r)
								p.skelText(open)
								for k := i; k < j; k++ {
									p.writeDrawingXMLToSkel(merged[k].data, partPath, emitBlock)
								}
								p.skelText(close)
								i = j
								continue
							}
						}
						if !isFusableDrawingRun(r) {
							p.writeRunToSkel(r, partPath, emitBlock)
							i++
							continue
						}
						// Look ahead for adjacent fusable drawing runs
						// with identical rPr AND identical opaque
						// element kind. Per ECMA-376-1 §17.3.2.1
						// (CT_R) a single `<w:r>` may host multiple
						// `<w:drawing>` siblings or multiple `<w:pict>`
						// siblings, but mixing kinds (drawing + AC,
						// drawing + pict) inside one `<w:r>` is not
						// what upstream Okapi RunMerger emits — its
						// MarkupComponent merge logic groups by
						// component kind. AlternateContent.docx
						// canonical case: a `<w:r><w:drawing></w:r>`
						// followed by `<w:r><mc:AlternateContent></w:r>`
						// share rsidRPr-only rPr but stay TWO `<w:r>`
						// envelopes upstream because the inner kind
						// differs (`<w:drawing>` vs
						// `<mc:AlternateContent>`).
						rKind := opaqueRunKind(r.data)
						j := i + 1
						for j < len(merged) {
							if !isFusableDrawingRun(merged[j]) {
								break
							}
							if !merged[j].props.equalIncludingChildren(r.props) {
								break
							}
							if opaqueRunKind(merged[j].data) != rKind {
								break
							}
							j++
						}
						if j == i+1 {
							p.writeRunToSkel(r, partPath, emitBlock)
							i++
							continue
						}
						// Emit one `<w:r>` envelope with all the
						// drawing payloads concatenated under the
						// shared rPr.
						open, close := splitRunWrapper(r)
						p.skelText(open)
						for k := i; k < j; k++ {
							p.writeDrawingXMLToSkel(merged[k].data, partPath, emitBlock)
						}
						p.skelText(close)
						i = j
					}
					p.skelWriteString("</w:p>")
					return nil
				}

				// Skip hidden text unless configured. inheritedVanish lets
				// a paragraph whose <w:vanish/> travels via pStyle (e.g.
				// after WSO promoted vanish from per-run rPr into a
				// synthesised paragraph style — PageBreak.docx,
				// Hidden_Textbox.docx) still get filtered out.
				inheritedVanish := false
				if p.styles != nil && paraStyleID != "" {
					inheritedVanish = p.styles.effectiveProps(paraStyleID).vanish
				}
				if !p.cfg.TranslateHiddenText && allHidden(merged, inheritedVanish) {
					// Suppress deletedMark-bearing pPr — see
					// stripPPrIfDeletedMark for the BlockParser.java:
					// 207-213 citation.
					emitParaProps := stripPPrIfDeletedMark(paraProps)
					p.skelWriteString("<w:p>")
					if emitParaProps != "" {
						p.skelText(emitParaProps)
					}
					// Write runs as skeleton text
					p.skelText(emitRunEnvelopes(merged))
					p.skelWriteString("</w:p>")
					return nil
				}

				// If a previous paragraph buffered itself as a
				// partFieldStraddle and THIS paragraph carries display
				// content (didn't fall into the empty-runs branch
				// above), flush the buffered block FIRST so it emits
				// in document order. This covers 1102.docx P4 (" " +
				// "text" + fldChar-end — display content interleaved
				// with the field-end, NOT a sole fldChar-end) and the
				// existing 830-2/830-6 case where the prev paragraph's
				// buffer flushes before this paragraph's text runs
				// (and the writer-side `pullLeadingFldCharEndIntoPrevParagraph`
				// handles the cross-paragraph fld-end move).
				if p.partFieldStraddle != nil {
					if err := p.flushPendingFieldBlock(nil, partPath, emitBlock); err != nil {
						return err
					}
				}

				// Cross-paragraph extractable-field straddle (defer
				// trigger). When this paragraph closes while an
				// extractable complex field is still open at result
				// phase (cfs.active && cfs.extractable && cfs.atResult
				// — fldChar-begin and fldChar-separate seen,
				// fldChar-end NOT yet), upstream Okapi defers the
				// `</w:p>` event inside `RunParser.parseComplexField`'s
				// `deferredEvents` queue (RunParser.java:508-514) and
				// continues gathering events into the SAME RunBuilder
				// until the field closes. The display-area runs from
				// this paragraph plus the lone fldChar-end run from a
				// SOLE-fldChar-end successor paragraph all land in one
				// block; the fldChar-end's original `<w:p>` survives
				// only as a structural shell with its pPr.
				//
				// Native mirrors this by buffering the post-mergeRuns
				// slice + paraProps + sidecars. The next
				// parseParagraph invocation either appends the
				// successor's fldChar-end and flushes (1172.docx P3,
				// 1341-textbox-with-a-hyperlink.docx textbox P2) or —
				// when the successor carries display content too —
				// flushes the buffer first then proceeds normally (no
				// regression vs the unbuffered baseline). Skip the
				// buffer when partMergeable is also being set
				// (deletedMark + open-field-at-entry passthrough case
				// — already buffered above by the `!openedFieldThisPara`
				// guard which returns before this point) so the two
				// cross-paragraph mechanisms never overlap. The
				// field-opener case (1102.docx P2 — cfs.active flipped
				// to true DURING this paragraph) falls through to here
				// and uses partFieldStraddle, which is the right path
				// for that scenario.
				if cfs.active && cfs.extractable && cfs.atResult {
					p.partFieldStraddle = &pendingFieldBlock{
						runs:        merged,
						paraProps:   paraProps,
						paraStyleID: paraStyleID,
						partPath:    partPath,
					}
					return nil
				}

				// Build block
				*p.blockCounter++
				blockID := fmt.Sprintf("tu%d", *p.blockCounter)

				// Note: do NOT clear partAbsorbedTrailingEmpty here.
				// In the 1102.docx pattern the content-bearing
				// paragraph that follows the partFieldStraddle flush
				// (P4) carries the fldChar-end run that closes the
				// straddling field, so it is structurally part of the
				// same absorbed block in upstream Okapi's view
				// (RunParser.parseComplexField + StyledTextPart.process
				// — P4's `<w:p>` never opens a new BlockParser frame;
				// it is consumed as opaque markup inside P2's
				// RunBuilder). The structural merge target Okapi
				// consumes is the BARE empty paragraph that follows
				// (P5: no pPr, no body, sole sibling of `<w:sectPr>`).
				// Clearing the flag here would drop the consumption
				// before that bare empty arrives.
				//
				// The flag is consumed at the empty-runs branch above
				// (single bare `<w:p ... />` drop) or cleared at sectPr
				// time in parsePart so it never escapes one part.

				// Skeleton: write paragraph open, props, ref, close.
				// Suppress deletedMark-bearing pPr — see
				// stripPPrIfDeletedMark for the BlockParser.java:
				// 207-213 citation. (Most content-bearing paragraphs
				// with a deletedMark are absorbed via partMergeable
				// above; this strip protects emit-paths where the
				// absorption was gated off.)
				emitParaProps := stripPPrIfDeletedMark(paraProps)
				p.skelWriteString("<w:p>")
				if emitParaProps != "" {
					p.skelText(emitParaProps)
				}
				p.skelRef(blockID)
				p.skelWriteString("</w:p>")

				block := p.buildBlock(blockID, merged, partPath, commonRPrXML, perRunRPrXML, perRunSrcRunStart)
				p.applyParagraphRole(block, paraStyleID, paraProps, allHidden(merged, inheritedVanish))
				emitBlock(block)
				return nil
			}
		}
	}
}

// parseRevisionInsertion drains the children of a <w:ins> or <w:moveTo>
// content wrapper that appears at paragraph level, appending any <w:r>
// runs found inside to the caller's run list. The wrapper element is
// effectively unwrapped — children are kept, the wrapper itself is
// dropped — to mirror okapi's auto-accept-revisions semantics for
// inserted/moved-in content.
//
// The local name passed in (`ins` or `moveTo`) lets the function know
// when to stop draining (matching close tag).
//
// Nested <w:ins>/<w:moveTo> inside the wrapper are handled recursively.
// Nested <w:del>/<w:moveFrom> inside the wrapper are skipped (their
// content is "deletion-of-an-insertion", which auto-accept treats as
// removal — same end state as if the deletion was direct).
func (p *wmlParser) parseRevisionInsertion(d *xml.Decoder, wrapperName string, runs *[]textRun, cfs *complexFieldState, wrapperStart xml.StartElement) error {
	// Strict OOXML preservation: when the wrapper sits in the strict
	// WordprocessingML namespace, upstream Okapi's
	// SkippableElement.RevisionInline (RUN_INSERTED_CONTENT /
	// MOVED_CONTENT_TO at SkippableElement.java:209-212) does NOT
	// classify it as skippable — the QName binds to the transitional
	// URI via Namespaces.WordProcessingML.getQName (Namespaces.java:26)
	// — so the wrapper round-trips around its child runs verbatim.
	// Emit paired-code sentinels (\uE10E open, \uE10F close) carrying
	// the captured `<w:ins ...>` / `<w:moveTo ...>` start tag and the
	// synthesised matching close tag; buildBlock dispatches them into
	// PcOpen/PcClose with TypeRevisionIns so the writer re-emits the
	// element verbatim around the inner runs.
	strictWrapper := p.strict && wrapperStart.Name.Space == wmlStrictNamespace
	if strictWrapper {
		rawStart := startElementToRaw(wrapperStart)
		*runs = append(*runs, textRun{text: "\uE10E:" + wrapperName + ":" + rawStart, props: runProps{}})
	}
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "r":
				rawStart := startElementToRaw(t)
				run, err := p.parseRunWithFieldState(d, cfs, rawStart)
				if err != nil {
					return err
				}
				run = filterFieldRuns(run, cfs)
				if cfs.active && !cfs.extractable {
					run = dropTextRuns(run)
				}
				if cfs.active && cfs.extractable && !cfs.atResult {
					run = dropTextRuns(run)
				}
				if len(run) == 0 {
					continue
				}
				*runs = append(*runs, run...)
			case "ins", "moveTo":
				if err := p.parseRevisionInsertion(d, t.Name.Local, runs, cfs, t); err != nil {
					return err
				}
			case "del", "moveFrom":
				if err := skipElement(d); err != nil {
					return err
				}
			default:
				// Unknown content (bookmarks, sdt, hyperlinks, etc. —
				// rare inside revision wrappers in practice). Skip the
				// subtree to mirror parseParagraph's default fallback;
				// future fixtures can extend this case if needed.
				if err := skipElement(d); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == wrapperName {
				if strictWrapper {
					closeData := "</w:" + wrapperName + ">"
					*runs = append(*runs, textRun{text: "\uE10F:" + wrapperName + ":" + closeData, props: runProps{}})
				}
				return nil
			}
		}
	}
}

// parseSmartTag drains a <w:smartTag> wrapper, processing its <w:r>
// children as if they were direct paragraph children and emitting
// paired-code sentinels ( open,  close) around them so
// the writer can round-trip the smartTag start/end tags verbatim.
//
// Mirrors upstream Okapi's RunContainer model (RunContainer.java
// lines 29-43, 187-191) where <w:smartTag> — alongside <w:hyperlink>
// and <w:sdt> — is a transparent wrapper around runs: inner runs
// can be simplified and consolidated, but the wrapper boundary is
// preserved as a single set of paired codes on the block. ECMA-376
// Part 1 §17.5.1.9 (smartTag) defines smartTag as a markup container
// that nests around a CT_R (run) sequence; smartTag may itself
// contain nested <w:smartTag> elements (commonly seen for a
// place/country-region pair around the same text). The nesting is
// handled by recursing through this helper.
//
// <w:smartTagPr> is dropped per upstream Okapi
// RunContainer.isPropertiesStart (line 77-83): smartTagPr properties
// are skippable and are NOT part of the preserved paired-code
// payload — only the <w:smartTag ...> start element itself (with its
// w:uri and w:element attributes) and its matching end tag are
// round-tripped.
//
// rawStart is the raw XML form of the <w:smartTag ...> open tag
// (including any namespace declarations and attributes) produced by
// the caller via startElementToRaw. It is paired with the literal
// "</w:smartTag>" close tag in the close sentinel.
func (p *wmlParser) parseSmartTag(d *xml.Decoder, runs *[]textRun, cfs *complexFieldState, rawStart string) error {
	*runs = append(*runs, textRun{text: ":" + rawStart, props: runProps{}})
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "smartTagPr":
				// Drop smartTag properties — preserved only as a
				// skippable per upstream RunContainer.isPropertiesStart
				// (RunContainer.java lines 77-83).
				if err := skipElement(d); err != nil {
					return err
				}
			case "r":
				rawRStart := startElementToRaw(t)
				run, err := p.parseRunWithFieldState(d, cfs, rawRStart)
				if err != nil {
					return err
				}
				run = filterFieldRuns(run, cfs)
				if cfs.active && !cfs.extractable {
					run = dropTextRuns(run)
				}
				if cfs.active && cfs.extractable && !cfs.atResult {
					run = dropTextRuns(run)
				}
				if len(run) == 0 {
					continue
				}
				*runs = append(*runs, run...)
			case "smartTag":
				// Nested smartTag (e.g. <smartTag element="place">
				// wrapping <smartTag element="country-region"> in
				// 952-3.docx). Recurse so the nested wrapper emits
				// its own paired-code sentinels.
				nestedRaw := startElementToRaw(t)
				if err := p.parseSmartTag(d, runs, cfs, nestedRaw); err != nil {
					return err
				}
			case "ins", "moveTo":
				// Revision insertion inside a smartTag — unwrap
				// children. Mirrors parseParagraph's handling.
				if err := p.parseRevisionInsertion(d, t.Name.Local, runs, cfs, t); err != nil {
					return err
				}
			case "del", "moveFrom":
				if err := skipElement(d); err != nil {
					return err
				}
			default:
				// Unknown content — skip the subtree. Per upstream
				// Okapi smartTag is restricted to runs and nested
				// containers (RunContainer.RUN_CONTAINER_TYPES), so
				// other children are out of spec.
				if err := skipElement(d); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "smartTag" {
				*runs = append(*runs, textRun{text: ":</w:smartTag>", props: runProps{}})
				return nil
			}
		}
	}
}
