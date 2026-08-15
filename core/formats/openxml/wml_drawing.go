// Drawing capture: the translatable text inside a w:drawing or w:pict — text
// box content, non-visual property names and alt text — surfaced as blocks
// through markers substituted into the captured markup, plus the drawing
// attributes (image relationship, description) a drawing sentinel carries.

package openxml

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/internal/xmlesc"
	"github.com/neokapi/neokapi/core/model"
)

// drawingNameAttrRE matches a name="..." attribute on either a
// non-visual drawing object property element (<wp:docPr>) or a
// non-visual canvas property element (<pic:cNvPr>, <wps:cNvPr>, …).
// Both elements are translatable per Okapi's
// XMLEventHelpers.isDrawingProperty (line 292 of okapi/filters/openxml
// /src/main/java/net/sf/okapi/filters/openxml/XMLEventHelpers.java)
// when ConditionalParameters.getTranslateWordGraphicName() is true
// (default true; ConditionalParameters.java line ~setTranslate-
// WordGraphicName(true) in the constructor). The submatch ordering is:
//
//	[1] open tag prefix up to the name= attribute (incl. the leading
//	    "<docPr " or "<cNvPr " plus any preceding attributes)
//	[2] quote character (' or ")
//	[3] attribute value
//	[4] tail of the open tag (closing '>' or '/>')
//
// Conservative: only matches docPr and cNvPr when they appear in a
// drawing context. We don't try to disambiguate against unrelated
// elements named docPr/cNvPr because none exist in the OOXML schema.
// Multiline/indented forms tolerated via [^>]* segments.
var drawingNameAttrRE = regexp.MustCompile(
	`(<(?:[A-Za-z_][\w-]*:)?(?:docPr|cNvPr)\b[^>]*?\s+name=)(["'])([^"']*)(["'][^>]*?/?>)`,
)

// drawingAttrs resolves a captured drawing's markup to the canonical run
// attributes core/projection consumes: model.AttrSrc for the image part and
// model.AttrAlt for its accessibility text.
//
// Like a hyperlink's destination, neither is in the element that names the
// image. `<a:blip r:embed="rIdN"/>` points at a relationship, and only that
// lookup yields a part name; the alt text lives in a `descr=` attribute on the
// drawing's non-visual properties. The drawing's raw markup is captured whole
// for the skeleton path, so both are read from it here rather than threaded
// through the walk — the property elements are visited before the blip, so
// there is no point during parsing where both are in hand.
//
// Returns nil when the drawing embeds nothing (a chart, a shape, a linked
// image), so a placeholder that is not a picture gains no image attributes.
func (p *wmlParser) drawingAttrs(raw string) map[string]string {
	attrs := make(map[string]string, 2)
	if m := blipEmbedRE.FindStringSubmatch(raw); m != nil {
		if rel, ok := p.rels[m[1]]; ok && rel.Target != "" {
			attrs[model.AttrSrc] = rel.Target
		}
	}
	if len(attrs) == 0 {
		return nil
	}
	if m := drawingDescrRE.FindStringSubmatch(raw); m != nil {
		alt := xmlesc.UnescapeAttr(m[1])
		if ref, ok := strings.CutPrefix(alt, drawingMarkerPropPrefix); ok {
			alt = p.drawingPropText[strings.TrimSuffix(ref, drawingMarkerSuffix)]
		}
		if alt != "" {
			attrs[model.AttrAlt] = alt
		}
	}
	return attrs
}

// blipEmbedRE matches the relationship id on `<a:blip r:embed="rIdN"/>`
// (ECMA-376-1 §20.1.8.13, CT_Blip) — the only link between a drawing and the
// image part it renders.
var blipEmbedRE = regexp.MustCompile(`<a:blip\b[^>]*\br:embed="([^"]*)"`)

// drawingDescrRE matches the first `descr=` in a drawing — the accessibility
// text on its non-visual properties (§20.4.2.5, CT_NonVisualDrawingProps).
// `<wp:docPr>` precedes the nested `<pic:cNvPr>` and both normally carry the
// same value, so the first match is the drawing's own alt text.
//
// The attribute is matched on its own rather than anchored to the element name:
// by this point a preceding surfaced attribute has been replaced by a comment
// marker, and a marker contains `>`, so any element-anchored pattern that walks
// `[^>]*` to reach `descr=` stops short of it.
var drawingDescrRE = regexp.MustCompile(`\bdescr="([^"]*)"`)

// drawingMarkerProp is the comment marker syntax embedded inside
// captured drawing XML at READ time to flag a translatable
// attribute value (drawing-name, vml-textpath-string). The writer
// expands these markers either into skeleton refs (skeleton path,
// writeDrawingXMLToSkel) or into rendered "property" Block content
// (in-block path, writer.go renderWMLBlock TypeImage handler).
const drawingMarkerPropPrefix = "<!--KAPI-PROP:"

// drawingMarkerPara is the marker syntax for a translatable
// paragraph block — used when a captured drawing contains
// <w:txbxContent><w:p>...</w:p></w:txbxContent> (textbox body
// paragraphs).
const drawingMarkerParaPrefix = "<!--KAPI-PARA:"

// drawingMarkerText is the marker syntax for a translatable
// text node — used when a captured drawing contains a bare
// <w:t> element (no enclosing <w:r>/<w:p>) such as inside
// <mc:AlternateContent><mc:Choice><w:t>...</w:t></mc:Choice></
// mc:AlternateContent>. AltContentEscaping.docx is the
// canonical fixture: a <w:t xml:space="preserve"> appearing
// directly under <mc:Choice Requires="wpg">. Per ECMA-376
// Part 3 / ISO/IEC 29500-3 §10 (Markup Compatibility) the
// consumer walks INTO mc:Choice transparently and continues
// processing children with their own semantics — upstream
// Okapi's RunParser.parseContent (RunParser.java line 708-818)
// hits isTextStartEvent on the inner <w:t> and emits its
// character data as translatable text (line 710-713), with
// the surrounding mc:AlternateContent/mc:Choice wrapper
// preserved as opaque markup. Mirror that: descend through
// mc:Choice, replace <w:t>...</w:t>'s character data with
// this marker, and emit a property block carrying the text.
const drawingMarkerTextPrefix = "<!--KAPI-TEXT:"

const drawingMarkerSuffix = "-->"

// drawingMarkerRE matches a property marker
// (<!--KAPI-PROP:tu123-->), a paragraph marker
// (<!--KAPI-PARA:tu123-->), or a text marker
// (<!--KAPI-TEXT:tu123-->) and captures the kind plus block ID.
var drawingMarkerRE = regexp.MustCompile(`<!--KAPI-(PROP|PARA|TEXT):([a-zA-Z0-9_-]+)-->`)

// extractDrawingTranslations scans a captured drawing XML payload,
// emits "property" / "paragraph" Blocks for every translatable
// site (drawing-name attributes, vml-textpath strings, txbx-
// content paragraph bodies), and returns the XML with each site
// replaced by a comment marker referencing the emitted block.
//
// Both writer paths (skeleton flush + in-block TypeImage handler)
// then expand the markers — the skeleton flush turns them into
// real skel refs (inside writeDrawingXMLToSkel), the TypeImage
// handler resolves them against the blocks map and substitutes
// rendered content. Splitting extraction from emission lets
// drawings inside paragraphs that ALSO contain translatable text
// runs (e.g. TextBoxes.docx where the body paragraph has three
// pict-only runs followed by a "Doggy " text run) participate in
// translation — the buildBlock path stuffs the captured XML into
// a TypeImage placeholder, bypassing the skeleton entirely, so the
// extraction must happen up-front.
//
// Mirrors Okapi's RunParser.processTranslatableAttributes
// (RunParser.java lines 838-858) for attribute extraction and
// wordConfiguration.yml's `'wps:txbx': ruleTypes: [GROUP]` (line
// 141) for textbox descent.
func (p *wmlParser) extractDrawingTranslations(xmlData, partPath string, emitBlock func(*model.Block)) string {
	// Resolve the drawing fragment's prefixes from the canonical wrapper
	// namespaces, isolated from (and not leaking into) the surrounding
	// document's registry.
	defer isolateNamespaces()()
	var out strings.Builder
	out.Grow(len(xmlData))
	wrapped := wrapDrawingXMLForDecode(xmlData)
	dec := xml.NewDecoder(strings.NewReader(wrapped))
	if _, err := dec.Token(); err != nil {
		return xmlData
	}
	if err := p.copyAndExtractDrawing(dec, &out, partPath, emitBlock); err != nil {
		// Decoding failure: fall back to verbatim. Do not corrupt
		// the round-trip.
		return xmlData
	}
	return out.String()
}

// copyAndExtractDrawing serialises tokens from dec into out until
// it consumes the matching end of the synthetic wrapper element
// emitted by wrapDrawingXMLForDecode. Translatable sites are
// replaced with marker comments; everything else round-trips
// verbatim.
func (p *wmlParser) copyAndExtractDrawing(dec *xml.Decoder, out *strings.Builder, partPath string, emitBlock func(*model.Block)) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case isDrawingPropertyElement(t):
				p.writeDrawingPropertyElementTo(out, t, partPath, emitBlock)
			case t.Name.Local == "textpath":
				p.writeStartElementWithTranslatableAttrTo(out, t, "string", "vml-textpath-string", partPath, emitBlock)
			case t.Name.Local == "txbxContent":
				writeRawStartElementTo(out, t)
				// Each <w:txbxContent> is its own logical scope for
				// complex fields: an open `<w:fldChar>` started inside
				// the textbox body cannot straddle the surrounding
				// drawing's outer paragraph (the txbx is XML-nested
				// inside a non-WML run-container). Allocate a fresh
				// state machine that the textbox paragraphs share so a
				// HYPERLINK begin in one `<w:p>` keeps its extractable
				// flag through the matching end in a later sibling
				// `<w:p>`. Fixture: 1341-textbox-with-a-hyperlink.docx.
				var txbxCfs complexFieldState
				if err := p.extractTxbxContent(dec, out, t, partPath, emitBlock, &txbxCfs); err != nil {
					return err
				}
			case t.Name.Local == "t" && isWML(t):
				// Bare <w:t> inside opaque markup (typically
				// <mc:Choice>): replace its character data with
				// a TEXT marker pointing at an emitted property
				// block. Per ECMA-376 Part 3 §10 the consumer
				// walks INTO mc:Choice transparently and treats
				// inner WML elements with their normal semantics
				// — including <w:t> (Part 1 §17.3.3.31) which is
				// always translatable text. Mirrors upstream
				// Okapi RunParser.parseContent line 710-713
				// (isTextStartEvent → parseText) for any <w:t>
				// reached during the AlternateContent walk.
				// Fixture: AltContentEscaping.docx.
				if err := p.extractBareTextElement(dec, out, t, partPath, emitBlock); err != nil {
					return err
				}
			case t.Name.Local == "t" && t.Name.Space == dmlNamespace:
				// DrawingML <a:t> (ECMA-376-1 §21.1.2.2.7) inside a
				// captured drawing payload — text content of an <a:r>
				// run inside an <a:p> paragraph inside a DrawingML
				// container (<lc:lockedCanvas>, <wps:txbx>, <a:txBody>,
				// chart text, …). Upstream Okapi's wordConfiguration.yml
				// declares 'a:t' as a TEXTMARKER (line ~138) so its
				// character data is the translatable text payload of
				// the surrounding <a:r>; the run/text envelope rounds
				// trips verbatim. Mirror that here by replacing the
				// CDATA with a TEXT marker — the wrapping <a:r><a:rPr/>
				// <a:t> ... </a:t></a:r> stays intact in the captured
				// XML stream.
				//
				// Fixture: DrawingML_Test.docx (a <lc:lockedCanvas>
				// hosting an <a:p><a:r><a:t>Important</a:t></a:r></a:p>
				// inside a <wp:inline> drawing in document.xml).
				if err := p.extractBareTextElement(dec, out, t, partPath, emitBlock); err != nil {
					return err
				}
			default:
				writeRawStartElementTo(out, t)
			}
		case xml.EndElement:
			if t.Name.Local == drawingDecodeWrapperLocal {
				return nil
			}
			writeRawEndElementTo(out, t)
		case xml.CharData:
			out.WriteString(xmlesc.Text(string(t)))
		case xml.Comment:
			out.WriteString("<!--")
			out.Write(t)
			out.WriteString("-->")
		case xml.ProcInst:
			out.WriteString("<?")
			out.WriteString(t.Target)
			if len(t.Inst) > 0 {
				out.WriteString(" ")
				out.Write(t.Inst)
			}
			out.WriteString("?>")
		}
	}
}

// extractTxbxContent processes children of <w:txbxContent>: emits a
// paragraph Block (and a marker comment in place) per <w:p> with
// translatable runs; copies non-paragraph children verbatim.
//
// txbxCfs carries complex-field state across the textbox's sibling
// paragraphs. Upstream Okapi reads the WML event stream as one
// continuous flow (RunParser.parseComplexField at lines 461-542 of
// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/openxml/
// RunParser.java) so a `<w:fldChar fldCharType="begin"/>` opened in
// one textbox paragraph can be closed by a matching end in the next.
// The caller scopes the state to one `<w:txbxContent>` (allocating a
// fresh instance at the txbxContent boundary) — the textbox body is
// XML-nested inside a non-WML run-container, so its field state never
// leaks into the surrounding paragraph's `partCfs`. Fixture:
// 1341-textbox-with-a-hyperlink.docx (a HYPERLINK whose begin /
// instrText / separate sit in `<w:p>` #1 and whose matching end sits
// in `<w:p>` #2; the display text "Okapiframework" inside the field's
// result region must reach the translation pipeline). Non-extractable
// fields (TextboxNumber.docx's PAGE \* MERGEFORMAT) still drop their
// display text runs the same way parseParagraph does via dropTextRuns
// — see extractTxbxParagraph's `cfs.active && !cfs.extractable` guard.
func (p *wmlParser) extractTxbxContent(
	dec *xml.Decoder,
	out *strings.Builder,
	start xml.StartElement,
	partPath string,
	emitBlock func(*model.Block),
	txbxCfs *complexFieldState,
) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "p" {
				rawP, err := captureRawElement(dec, t)
				if err != nil {
					return err
				}
				// Re-decode the captured paragraph through a fresh
				// namespace-aware decoder so extractTxbxParagraph
				// sees the canonical token stream with the same
				// prefix bindings as the outer document.
				inner := wrapDrawingXMLForDecode(rawP)
				idec := xml.NewDecoder(strings.NewReader(inner))
				if _, err := idec.Token(); err != nil {
					return err
				}
				// Advance past the <w:p> start tag so
				// extractTxbxParagraph sees the inside of the
				// paragraph (its pPr / runs / end tag).
				for {
					itok, err := idec.Token()
					if err != nil {
						return err
					}
					if se, ok := itok.(xml.StartElement); ok && se.Name.Local == "p" {
						break
					}
				}
				if err := p.extractTxbxParagraph(idec, out, partPath, emitBlock, txbxCfs); err != nil {
					return err
				}
			} else if t.Name.Local == "tbl" || t.Name.Local == "tr" || t.Name.Local == "tc" {
				writeRawStartElementTo(out, t)
				if err := p.extractTxbxContent(dec, out, t, partPath, emitBlock, txbxCfs); err != nil {
					return err
				}
			} else {
				raw, err := captureRawElement(dec, t)
				if err != nil {
					return err
				}
				out.WriteString(raw)
			}
		case xml.EndElement:
			writeRawEndElementTo(out, t)
			if t.Name.Local == start.Name.Local {
				return nil
			}
		case xml.CharData:
			out.WriteString(xmlesc.Text(string(t)))
		case xml.Comment:
			out.WriteString("<!--")
			out.Write(t)
			out.WriteString("-->")
		}
	}
}

// extractTxbxParagraph parses a <w:p> from a textbox body: the
// caller has already positioned the decoder right after the <w:p>
// start tag. We re-implement a minimal subset of parseParagraph's
// behaviour here, capturing pPr verbatim and collecting <w:r>
// runs for blocking, then emit the paragraph block and write a
// `<w:p><pPr/><!--KAPI-PARA:id--></w:p>` to out.
//
// Hyperlinks, sdt, ins/del/moveTo/moveFrom, and AlternateContent
// inside textboxes are rare; we skip them via skipElement to keep
// this scoped. Future fixtures can extend.
//
// cfs is the textbox-scoped complex-field state shared across sibling
// paragraphs inside one `<w:txbxContent>` so a HYPERLINK that opens in
// paragraph N keeps its extractable flag through the matching end in
// paragraph N+1. See extractTxbxContent's contract for the upstream
// citation. Mirrors parseParagraph's use of `p.partCfs` for body-text
// paragraphs.
func (p *wmlParser) extractTxbxParagraph(dec *xml.Decoder, out *strings.Builder, partPath string, emitBlock func(*model.Block), cfs *complexFieldState) error {
	// Reset per-paragraph style-chain context — see parseParagraph
	// for the rationale.
	savedStyleChainNames := p.currentStyleChainNames
	p.currentStyleChainNames = nil
	defer func() { p.currentStyleChainNames = savedStyleChainNames }()

	var paraProps string
	var paraStyleID string
	var runs []textRun
	var bms bookmarkSkipState

	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "pPr":
				raw, styleID, err := captureParaProps(dec, t)
				if err != nil {
					return err
				}
				paraProps = raw
				paraStyleID = styleID
				// See parseParagraph for the upstream-Okapi citation;
				// textbox paragraphs share the same run-property
				// minification path and need the same style-chain
				// awareness.
				if p.styles != nil {
					p.currentStyleChainNames = p.styles.effectiveRPrChildNames(paraStyleID)
				}
			case "r":
				rawStart := startElementToRaw(t)
				rs, err := p.parseRunWithFieldState(dec, cfs, rawStart)
				if err != nil {
					return err
				}
				rs = filterFieldRuns(rs, cfs)
				if cfs.active && !cfs.extractable {
					rs = dropTextRuns(rs)
				}
				if cfs.active && cfs.extractable && !cfs.atResult {
					rs = dropTextRuns(rs)
				}
				runs = append(runs, rs...)
			case "bookmarkStart", "bookmarkEnd":
				// See parseParagraph for the bookmark capture rationale.
				bookmark, captured, err := p.captureBookmark(dec, t, &bms)
				if err != nil {
					return err
				}
				if captured {
					runs = append(runs, bookmark)
				}
			case "fldSimple":
				// See parseParagraph for the fldSimple rationale.
				raw, err := captureRawElement(dec, t)
				if err != nil {
					return err
				}
				raw = protectFieldPayloadFromStripping(raw)
				runs = append(runs, textRun{text: ":fldSimple", data: raw})
			case "smartTag":
				// See parseParagraph for the smartTag rationale —
				// transparent run-container unwrap per ECMA-376
				// Part 1 §17.5.1.9 and upstream Okapi RunContainer.
				rawStart := startElementToRaw(t)
				if err := p.parseSmartTag(dec, &runs, cfs, rawStart); err != nil {
					return err
				}
			case "commentRangeStart", "commentRangeEnd":
				// See parseParagraph for the comment-range rationale.
				marker, err := p.captureCommentRangeMarker(dec, t)
				if err != nil {
					return err
				}
				runs = append(runs, marker)
			case "proofErr", "permStart", "permEnd":
				if err := skipElement(dec); err != nil {
					return err
				}
			default:
				if err := skipElement(dec); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local != "p" {
				continue
			}
			// Apply style optimisation as parseParagraph does. The
			// parse-time minify in parseRunProps runs in deferred mode
			// (any default-valued rPr child whose name is absent from
			// the paragraph chain is KEPT, expecting a later minify
			// to fold in the rStyle chain before deciding). Run the
			// late minify here for textbox paragraphs too — without
			// it, an explicit-off WPML toggle (e.g. `<w:rtl w:val=
			// "0"/>` on a textbox run inside a header — fixture
			// HiddenTablesApachePoi.docx, header1.xml MERGEFORMAT
			// run) lingers in rPrChildren and round-trips to the
			// output, while upstream Okapi
			// `RunProperties.minified()` strips it because the
			// resolved style chain has no rtl by name
			// (RunProperties.java:497-540, the
			// `WpmlToggleRunProperty && !getToggleValue()` branch
			// gated by `!preCombined.contains(p.getName())`).
			//
			// Mirrors the parseParagraph late-minify block (see
			// the long doc comment at the rStyle chain merge site
			// around line 1985) — same upstream-Okapi citation.
			if p.styles != nil {
				paraStyleProps := p.styles.resolveProps(paraStyleID)
				paraChainNames := p.styles.effectiveRPrChildNames(paraStyleID)
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
						chainNames = mergeChainNames(paraChainNames, p.styles.effectiveRPrChildNames(rStyleID))
					}
					subtractProps(&runs[i].props, styleProps)
					runs[i].props.rPrChildren = minifyRPrChildren(runs[i].props.rPrChildren, chainNames)
					if !chainNames["vanish"] {
						runs[i].props.rPrChildren = stripExplicitOffVanish(runs[i].props.rPrChildren)
					}
					// Mirror the szCs strip from the body-text run loop
					// (see the canonical comment above the corresponding
					// stripChainAbsentSzCs call) — same chain + non-CS
					// gate, applied here for nested-block / textbox-body
					// run loops so MissingPara-style fixtures (and any
					// nested paragraph that authors `<w:szCs/>` on non-
					// CS text without chain support) don't slip through.
					if !chainNames["szCs"] && !containsComplexScriptText(runs[i].text) {
						runs[i].props.rPrChildren = stripChainAbsentSzCs(runs[i].props.rPrChildren)
					}
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
			commonRPr := commonRPrChildren(runs)
			commonRPrXML := joinRPrChildren(commonRPr)
			merged := mergeRuns(runs)
			// Per-run rPr sidecar (Phase 1) computed AFTER mergeRuns
			// so the slice aligns 1:1 with the model.TextRun stream
			// the writer emits. mergeRuns updates merged-away runs'
			// rPr to the per-attribute consensus (RunMerger
			// at RunMerger.java:156-229 + RunFonts.merge at
			// RunFonts.java:267-288). See PARITY_NOTES.md.
			perRunRPrXML := perRunRPrFragments(merged)
			// Per-text-run srcRunStart flags align with merged runs.
			perRunSrcRunStart := perRunSrcRunStartFlags(merged)
			// Recurse extraction into nested drawing/pict
			// payloads so e.g. a docPr name inside an image
			// embedded within a textbox paragraph still reaches
			// the translation pipeline (GraphicInTextBox.docx).
			for i := range merged {
				if isDrawingSentinel(merged[i].text) && merged[i].data != "" {
					merged[i].data = p.extractDrawingTranslations(merged[i].data, partPath, emitBlock)
				}
			}
			// Empty paragraph: emit verbatim wrapper without a
			// translatable block. The pPr (if any) is preserved
			// inside <w:p>...</w:p>.
			if isEmptyRuns(merged) {
				out.WriteString("<w:p>")
				if paraProps != "" {
					out.WriteString(paraProps)
				}
				for _, r := range merged {
					out.WriteString(runToXML(r))
				}
				out.WriteString("</w:p>")
				return nil
			}
			// Hidden text inside a textbox paragraph: emit verbatim
			// (mirrors the parseParagraph allHidden guard at line ~2026).
			// Without this, vanish-bearing textbox runs (Hidden_Textbox.docx
			// — `<w:r><w:rPr><w:vanish/></w:rPr><w:t>Hidden Text</w:t></w:r>`
			// inside a wps:txbx body) get extracted as translatable, then
			// the writer reconstructs the paragraph without the original
			// rPr structure and WSO no longer sees the vanish to promote.
			// inheritedVanish is computed the same way as the outer
			// parseParagraph path — see allHidden() and styleMap.effectiveProps().
			inheritedVanish := false
			if p.styles != nil && paraStyleID != "" {
				inheritedVanish = p.styles.effectiveProps(paraStyleID).vanish
			}
			if !p.cfg.TranslateHiddenText && allHidden(merged, inheritedVanish) {
				out.WriteString("<w:p>")
				if paraProps != "" {
					out.WriteString(paraProps)
				}
				for _, r := range merged {
					out.WriteString(runToXML(r))
				}
				out.WriteString("</w:p>")
				return nil
			}
			*p.blockCounter++
			blockID := fmt.Sprintf("tu%d", *p.blockCounter)
			out.WriteString("<w:p>")
			if paraProps != "" {
				out.WriteString(paraProps)
			}
			out.WriteString(drawingMarkerParaPrefix)
			out.WriteString(blockID)
			out.WriteString(drawingMarkerSuffix)
			out.WriteString("</w:p>")
			block := p.buildBlock(blockID, merged, partPath, commonRPrXML, perRunRPrXML, perRunSrcRunStart)
			emitBlock(block)
			return nil
		}
	}
}

// writeDrawingPropertyElementTo emits a <wp:docPr> / <pic:cNvPr> (and friends)
// drawing-property start element, substituting drawingMarkerProp comment markers
// for the attribute values it surfaces.
//
//   - name= — the graphic object name — is ALWAYS extracted as a TRANSLATABLE
//     "property" Block (mirroring Okapi's getTranslateWordGraphicName, default
//     true; see drawingNameAttrRE for the citation).
//   - descr= (accessibility alt text) and title= (object title) carry
//     human-readable prose describing the image/shape (ECMA-376-1 §20.4.2.5
//     CT_NonVisualDrawingProps). When ExtractNonTranslatableContent is on (the
//     default) they are surfaced as Translatable:false RoleCaption "property"
//     Blocks — visible to an ingestion/LLM consumer but skipped by machine
//     translation (#928). When the flag is off they pass through verbatim, so
//     the emitted XML is byte-identical to the prior name-only behaviour and
//     the canonical parity stream is unchanged.
//
// Both surfaced values ride the same drawingMarkerProp mechanism as name=, so
// the writer (skeleton flush and in-block TypeImage paths) expands them back
// to xml-attr-escaped text; an untranslated Translatable:false block expands to
// its source value, keeping the round-trip byte-exact.
func (p *wmlParser) writeDrawingPropertyElementTo(
	out *strings.Builder,
	t xml.StartElement,
	partPath string,
	emitBlock func(*model.Block),
) {
	surface := p.cfg != nil && p.cfg.ExtractNonTranslatableContent()
	nameDone := false

	out.WriteString("<")
	writeElementName(out, t.Name)
	for _, a := range t.Attr {
		out.WriteString(" ")
		writeAttrName(out, a.Name)
		out.WriteString(`="`)
		switch {
		case !nameDone && a.Name.Local == "name" && a.Name.Space == "" && strings.TrimSpace(a.Value) != "":
			nameDone = true
			p.emitDrawingPropMarker(out, a.Value, partPath, "drawing-name", true, emitBlock)
		case surface && a.Name.Space == "" && strings.TrimSpace(a.Value) != "" && a.Name.Local == "descr":
			p.emitDrawingPropMarker(out, a.Value, partPath, "drawing-descr", false, emitBlock)
		case surface && a.Name.Space == "" && strings.TrimSpace(a.Value) != "" && a.Name.Local == "title":
			p.emitDrawingPropMarker(out, a.Value, partPath, "drawing-title", false, emitBlock)
		default:
			out.WriteString(xmlesc.Attr(a.Value))
		}
		out.WriteString(`"`)
	}
	out.WriteString(">")
}

// emitDrawingPropMarker writes a drawingMarkerProp comment marker for the next
// block id, then emits a "property" Block carrying value as a single verbatim
// run. translatable selects the MT-visible (name=) vs MT-skipped, RoleCaption
// (descr=/title= alt text) treatment; a non-translatable block carries the
// caption role so ingestion/exporters can identify the alt text.
func (p *wmlParser) emitDrawingPropMarker(
	out *strings.Builder,
	value, partPath, element string,
	translatable bool,
	emitBlock func(*model.Block),
) {
	*p.blockCounter++
	refID := fmt.Sprintf("tu%d", *p.blockCounter)
	out.WriteString(drawingMarkerPropPrefix)
	out.WriteString(refID)
	out.WriteString(drawingMarkerSuffix)

	p.path.ensurePart(partPath)
	// Remember what this marker stands for. By the time the surrounding
	// <w:drawing> is captured as raw markup, every surfaced attribute has been
	// replaced by its marker, so the alt text is no longer readable from the
	// captured bytes — drawingAttrs resolves it back through this map.
	if p.drawingPropText == nil {
		p.drawingPropText = map[string]string{}
	}
	p.drawingPropText[refID] = value
	block := &model.Block{
		ID: refID,
		// The drawing's own address plus the attribute the text came from, so
		// a shape's name, its alt text and its title are three addresses.
		Name:         p.path.name(p.path.reserve("drawing"), "@"+element),
		Type:         "property",
		Translatable: translatable,
		Source:       []model.Run{{Text: &model.TextRun{Text: value}}},
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties: map[string]string{
			"partPath": partPath,
			"element":  element,
		},
	}
	if !translatable {
		// Alt text / object title: descriptive prose for an image or shape.
		// RoleCaption is the closest canonical role; it lets semantic export
		// and the editor identify the alt text without treating it as MT input.
		block.SetSemanticRole(model.RoleCaption, 0)
	}
	emitBlock(block)
}

// writeStartElementWithTranslatableAttrTo emits a start element to
// the given builder, replacing the named attribute's value with a
// drawingMarkerProp comment marker referencing an emitted block.
func (p *wmlParser) writeStartElementWithTranslatableAttrTo(
	out *strings.Builder,
	t xml.StartElement,
	attrLocal, blockElementTag, partPath string,
	emitBlock func(*model.Block),
) {
	out.WriteString("<")
	writeElementName(out, t.Name)
	emittedRef := false
	for _, a := range t.Attr {
		out.WriteString(" ")
		writeAttrName(out, a.Name)
		out.WriteString(`="`)
		if !emittedRef && a.Name.Local == attrLocal && a.Name.Space == "" && strings.TrimSpace(a.Value) != "" {
			*p.blockCounter++
			refID := fmt.Sprintf("tu%d", *p.blockCounter)
			out.WriteString(drawingMarkerPropPrefix)
			out.WriteString(refID)
			out.WriteString(drawingMarkerSuffix)
			emittedRef = true
			emitBlock(&model.Block{
				ID:           refID,
				Type:         "property",
				Translatable: true,
				Source:       []model.Run{{Text: &model.TextRun{Text: a.Value}}},
				Targets:      make(map[model.VariantKey]*model.Target),
				Properties: map[string]string{
					"partPath": partPath,
					"element":  blockElementTag,
				},
			})
		} else {
			out.WriteString(xmlesc.Attr(a.Value))
		}
		out.WriteString(`"`)
	}
	out.WriteString(">")
}

// extractBareTextElement handles a bare <w:t> element encountered
// during a copyAndExtractDrawing walk. It emits the start tag
// verbatim (preserving xml:space="preserve" and any other
// attributes), accumulates the character data into a property
// Block (text-run only), inserts a <!--KAPI-TEXT:tuN--> marker
// in place of the text content, then emits the end tag.
//
// Used for <w:t> children of <mc:Choice> in
// AltContentEscaping.docx — see the case in copyAndExtractDrawing
// for the namespace check and ECMA-376 / upstream-Okapi
// citations. The marker is later expanded by the writer's
// expandDrawingMarkers (kind=TEXT) to xml-escaped translation
// text (no element wrapping). If the <w:t> has no character
// data the function still emits the surrounding tags but skips
// the block emission so the writer doesn't materialise an
// empty target later.
func (p *wmlParser) extractBareTextElement(
	dec *xml.Decoder,
	out *strings.Builder,
	start xml.StartElement,
	partPath string,
	emitBlock func(*model.Block),
) error {
	writeRawStartElementTo(out, start)
	var text strings.Builder
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tt := tok.(type) {
		case xml.StartElement:
			// <w:t> per ECMA-376 Part 1 §17.3.3.31 has only
			// CT_Text (string content); nested elements are not
			// schema-valid. Defensive: copy the unexpected
			// child verbatim so malformed inputs round-trip
			// rather than corrupt.
			depth++
			writeRawStartElementTo(out, tt)
		case xml.EndElement:
			depth--
			if depth == 0 {
				if text.Len() > 0 {
					*p.blockCounter++
					refID := fmt.Sprintf("tu%d", *p.blockCounter)
					out.WriteString(drawingMarkerTextPrefix)
					out.WriteString(refID)
					out.WriteString(drawingMarkerSuffix)
					p.path.ensurePart(partPath)
					emitBlock(&model.Block{
						ID:           refID,
						Name:         p.path.name(p.path.reserve("alt-content-text")),
						Type:         "property",
						Translatable: true,
						Source:       []model.Run{{Text: &model.TextRun{Text: text.String()}}},
						Targets:      make(map[model.VariantKey]*model.Target),
						Properties: map[string]string{
							"partPath": partPath,
							"element":  "alt-content-text",
						},
					})
				}
				writeRawEndElementTo(out, tt)
				return nil
			}
			writeRawEndElementTo(out, tt)
		case xml.CharData:
			text.WriteString(string(tt))
		case xml.Comment:
			out.WriteString("<!--")
			out.Write(tt)
			out.WriteString("-->")
		}
	}
	return nil
}

// writeDrawingXMLToSkel emits a drawing's captured raw XML to the
// skeleton, walking the XML token stream to extract translatable
// content at three structural sites:
//
//  1. name= attribute on <wp:docPr> / <pic:cNvPr> / <wps:cNvPr>
//     (drawing object names) — extracted as a "property" Block.
//     Mirrors Okapi's RunParser.processTranslatableAttributes
//     (RunParser.java lines 838-858) gated by
//     ConditionalParameters.getTranslateWordGraphicName() (default
//     true).
//
//  2. string= attribute on <v:textpath> (legacy WordArt text
//     painted along a curve) — extracted as a "property" Block.
//     Mirrors RunParser.processTranslatableAttributes (RunParser.java
//     lines 854-855) which calls processTranslatableAttribute(startEl,
//     "string") whenever XMLEventHelpers.isTextPath(startEl) holds
//     (XMLEventHelpers.java lines 287-289, LOCAL_TEXTPATH = "textpath"
//     at line 77). Per ECMA-376 Part 4 (VML) §6.2.2, the textpath
//     element's string attribute carries the displayed text.
//
//  3. <w:p> paragraphs nested inside <w:txbxContent> (drawing
//     textbox bodies — both the WordprocessingML <wps:txbx> shape
//     wrapper and the legacy VML <v:textbox> wrapper produce a
//     <w:txbxContent> child holding regular WML paragraphs). These
//     are parsed via parseParagraph so the inner text emits as
//     normal "paragraph" Blocks (with inline runs, hyperlinks,
//     fldChars, …). The skeleton stream interleaves the captured
//     drawing/textbox markup with paragraph block refs so the
//     writer reconstructs <w:txbxContent> with translated runs in
//     place. Mirrors Okapi's word-configuration.yml at line 141
//     ('wps:txbx': ruleTypes: [GROUP]) which directs the filter to
//     descend into the textbox content as a structural group rather
//     than treating it as opaque inline content.
//
// Anything else passes through verbatim.
//
// The xmlData has already been processed by
// extractDrawingTranslations (called from parseParagraph before
// the empty-runs path branches into writeRunToSkel) — meaning
// translatable sites are already represented as
// <!--KAPI-PROP:tu123--> / <!--KAPI-PARA:tu123--> markers and the
// corresponding Blocks have been emitted to the part stream. All
// this function does is split the modified XML on markers,
// emitting skeleton refs in their place so the writer's skeleton
// stitching expands them into rendered block content.
func (p *wmlParser) writeDrawingXMLToSkel(xmlData, _partPath string, _emitBlock func(*model.Block)) {
	matches := drawingMarkerRE.FindAllStringSubmatchIndex(xmlData, -1)
	if len(matches) == 0 {
		p.skelText(xmlData)
		return
	}
	pos := 0
	for _, m := range matches {
		// m = [whole_lo, whole_hi, kind_lo, kind_hi, id_lo, id_hi]
		p.skelText(xmlData[pos:m[0]])
		blockID := xmlData[m[4]:m[5]]
		p.skelRef(blockID)
		pos = m[1]
	}
	p.skelText(xmlData[pos:])
}

// drawingDecodeWrapperLocal is the local-name of the synthetic root
// element used to wrap captured drawing XML so encoding/xml can
// resolve prefixes. It only ever exists in the temporary input to
// the decoder and never reaches the skeleton stream.
const drawingDecodeWrapperLocal = "neokapi_drawing_wrapper"

// drawingDecodeWrapperPrefix is the namespace declarations injected
// onto the synthetic wrapper so every known OpenXML prefix resolves
// to its full URI when the decoder reads child elements. Built once
// at package init from nsPrefixMap (skipping the empty prefix and
// the synthetic xmlns/xml prefixes which encoding/xml handles).
var drawingDecodeWrapperPrefix string

func init() {
	var b strings.Builder
	b.WriteString("<")
	b.WriteString(drawingDecodeWrapperLocal)
	for uri, prefix := range nsPrefixMap {
		// xml prefix is implicit; xmlns prefix is reserved.
		if prefix == "" || prefix == "xml" || prefix == "xmlns" {
			continue
		}
		b.WriteString(` xmlns:`)
		b.WriteString(prefix)
		b.WriteString(`="`)
		b.WriteString(xmlesc.Attr(uri))
		b.WriteString(`"`)
	}
	b.WriteString(">")
	drawingDecodeWrapperPrefix = b.String()
}

// wrapDrawingXMLForDecode wraps captured drawing XML in a synthetic
// root that declares every known OpenXML namespace prefix, so
// encoding/xml's namespace-aware decoder can fully qualify the
// Names of nested elements (`w:drawing`, `v:textpath`, `wps:txbx`,
// …). The wrapper is stripped during re-emission — see
// writeDrawingXMLToSkel.
func wrapDrawingXMLForDecode(xmlData string) string {
	var b strings.Builder
	b.Grow(len(drawingDecodeWrapperPrefix) + len(xmlData) + len(drawingDecodeWrapperLocal) + 4)
	b.WriteString(drawingDecodeWrapperPrefix)
	b.WriteString(xmlData)
	b.WriteString("</")
	b.WriteString(drawingDecodeWrapperLocal)
	b.WriteString(">")
	return b.String()
}

// isDrawingPropertyElement reports whether t is a non-visual drawing
// property carrier (<docPr> on a wp wrapper, or <cNvPr> on any
// pic/wps/dgm wrapper) whose name attribute Okapi treats as
// translatable. Mirrors XMLEventHelpers.isDrawingProperty (lines
// 291-294 of okapi/filters/openxml/src/main/java/net/sf/okapi/
// filters/openxml/XMLEventHelpers.java) which checks two local
// names: LOCAL_NON_VISUAL_OBJECT_PROPERTY ("docPr") and
// LOCAL_NON_VISUAL_CANVAS_PROPERTY ("cNvPr").
func isDrawingPropertyElement(t xml.StartElement) bool {
	return t.Name.Local == "docPr" || t.Name.Local == "cNvPr"
}
