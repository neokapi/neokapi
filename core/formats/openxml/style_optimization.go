// Package openxml — Word style optimisation
//
// Implements Okapi's AllowWordStyleOptimisation transform on
// WordprocessingML output, mirroring upstream Java behaviour:
//
//   - Per paragraph, compute the run-property elements (rPr children) that
//     are present and identical across every <w:r> in the paragraph.
//   - If the common set is non-empty AND no run carries an "exclusion"
//     property (toggle/highlight/etc.), synthesise a paragraph style with
//     basedOn=<paragraph's pStyle, or "Normal" by default> and
//     rPr=<common props>. The styleId follows Okapi's IdGenerator pattern:
//     "NF974E24F-{parentBase}{N}" where {N} is the per-parent sequence
//     starting at 1.
//   - Add <w:pStyle w:val="<id>"/> to the paragraph's pPr.
//   - Strip the common props from each run's rPr.
//
// Upstream references:
//   - StyleOptimisation.java (lines 96-129) — the Default.applyTo loop.
//   - WordStyleDefinitions.java (lines 148-185) — place() of a
//     synthesised style with basedOn/rPr.
//   - WordStyleDefinitions.Ids (lines 445-516) — parentBased/defaultBased
//     ID lookup-or-generate.
//   - IdGenerator.java + Util.makeId — the "NF974E24F" prefix is the
//     hex-formatted Java string-hash of the literal "style" via
//     Util.makeId("style"), used as the IdGenerator root for the
//     openxml-filter style id stream.
//
// This implementation runs as a POST-pass on the writer-emitted
// document.xml, headerN.xml, footerN.xml etc. — it does not change the
// reader/skeleton paths. The set of synthesised styles is collected and
// injected into word/styles.xml via injectSynthesisedStyles.

package openxml

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// styleHashRoot is the hex form of Util.makeId("style") — the literal
// IdGenerator root used by Okapi's WordStyleDefinitions for synthesised
// paragraph styles. This is hard-coded because every Okapi reference
// run uses "style" as the root for the openxml id generator (see
// WordStyleDefinitions.readWith line 114 — IdGenerator(STYLE, STYLE)).
const styleHashRoot = "NF974E24F"

// runPropExclusions are local-names of <w:rPr> children that BLOCK a
// paragraph from being optimised at all when ANY run in it carries one.
//
// Upstream Okapi's WSO exclusion list for WordprocessingML is JUST
// rStyle (see okapi/filters/openxml/WordDocument.java:335-337 — the
// styleOptimisationsFor() factory passes
// Collections.singletonList(rStyle) as the exclusion set when building
// StyleOptimisation.Default for a WML part).
//
// rStyle (character style reference) is run-scoped semantics: it points
// to a character style by id and must remain on each <w:r>. ECMA-376-1
// §17.7.4 (Character Style Definitions). Lifting it into a synthesised
// PARAGRAPH style would silently change the rendered result.
//
// Other rPr children that might look like exclusion candidates:
//   - <w:lang>, <w:noProof>, <w:rPrChange> are stripped from rPr at
//     parse time (parseRunProps) and at writer post-pass time
//     (stripWMLSkippableElements) — mirroring upstream RunSkippableElements
//     (RunSkippableElements.java:50-62). They never reach this map.
//   - Tracked-revision run-property elements (rPrChange, ins, del,
//     moveTo, moveFrom inside rPr) have already been stripped by
//     stripWMLSkippableElements when this function runs.
//
// rtl was previously listed here as a compensating guard for the missing
// RunProperty.minified() pass. minifyRPrChildren in runprops.go now
// implements that pass (mirrors RunProperties.java:497-540), so explicit
// `<w:rtl w:val="0"/>` / `<w:rtl w:val="false"/>` toggles are stripped
// from the run rPr at parse time — before WSO sees them. The exclusion
// is no longer needed for rtl (or for any other WPML toggle), and keeping
// it would actually swallow legitimate `<w:rtl/>` (true) markers that
// must travel through to the writer.
//
// <w:vanish> (hidden text marker, ECMA-376-1 §17.3.2.42) was previously
// excluded pending paragraph-style→run inheritance in the native reader.
// allHidden in wml.go now consults `styleMap.effectiveProps(paraStyleID).
// vanish` so a paragraph whose vanish was promoted to its pStyle stays
// hidden on re-read even though its runs no longer carry direct vanish.
// Upstream Okapi DOES lift vanish into the synthesised style — see
// PageBreak.docx and Hidden_Textbox.docx reference output (NF974E24F-
// Standard1 / NF974E24F-Normal1 with `<w:rPr><w:vanish/></w:rPr>`).
var runPropExclusions = map[string]bool{
	"rStyle": true,
}

// runProp is a single <w:rPr> child element captured by name and raw
// XML serialization. Two runProps are "equal" when their canonical
// serialization matches (attribute order is preserved from the source —
// canonicalisation happens at the parity layer).
type runProp struct {
	name string // local element name without "w:" prefix (e.g. "rFonts", "lang")
	xml  string // raw element XML, e.g. `<w:rFonts w:ascii="Arial"/>`
}

// synthesisedStyle is a paragraph-style placeholder that was created by
// the optimisation pass. parentID is the basedOn target ("Normal" by
// default).
type synthesisedStyle struct {
	id       string
	parentID string
	rPrXML   string // children only
}

// rawParagraph is a slice into the document buffer covering one
// <w:p>...</w:p> element (start-tag offset to end-tag offset+len).
type rawParagraph struct {
	start int // index of opening "<w:p" in src
	end   int // index just past "</w:p>"
}

// optimizeWMLPart applies AllowWordStyleOptimisation to a
// WordprocessingML XML part. It is the entry point used by the writer
// post-pass for word/document.xml, word/header*.xml, word/footer*.xml,
// word/footnotes.xml, word/endnotes.xml.
//
// idCounter is a single shared sequence number — a *int updated in
// place across calls so that styleId sequence numbers continue across
// multi-part documents (matching Okapi's single IdGenerator scope for
// the entire openxml filter invocation, see IdGenerator.createId at
// okapi/core/.../IdGenerator.java:124-138 — the seq field is
// IdGenerator-scoped, NOT prefix-scoped, so e.g. "Normal1", "Normal2",
// "Footer3" can interleave by call order).
//
// existingStyleIDs is the SOURCE styles.xml id set. It is consulted
// for two purposes: (1) parent-style lookup — a paragraph pStyle that
// isn't defined in this set falls back to defaultParagraphStyleID (or
// "Normal" if no default exists), mirroring
// WordStyleDefinitions.Ids.basedOn at lines 453-460; (2) generated-id
// collision avoidance — generation tickets that hit an existing id
// re-roll, mirroring parentBasedGenerated's do/while loop. The map is
// updated in place when a new synthesised id is added so subsequent
// generations see the new id too (matching the upstream contract).
//
// defaultParagraphStyleID is the styleId of the default paragraph
// style declared in word/styles.xml (i.e. the
// `<w:style w:type="paragraph" w:default="1" w:styleId="X">` element).
// Mirrors upstream WordStyleDefinitions.Ids.defaultBased
// (WordStyleDefinitions.java:485-491): when a paragraph has no pStyle,
// the synthesised style's basedOn (and the parentBased id) derive from
// the default paragraph style id rather than the literal "Normal". If
// styles.xml has no default paragraph style, fall back to "Normal" —
// matches Okapi's documentDefaultBased path collapsing onto the
// document defaults (see WordStyleDefinitions.java:493-505).
//
// hasStylesPart reports whether the source ZIP includes a
// word/styles.xml part. When it does NOT, WSO still runs but cannot
// synthesise a styleId — upstream Okapi instantiates
// StyleDefinitions.Empty for the missing part
// (WordDocument.java:115-119, StyleDefinitions.java:39-93), whose
// place(parentId, …) is a no-op and whose placedId() returns null.
// The optimiser still inserts a <w:pStyle> element into the
// paragraph's pPr (and strips common rPr props from each run), but
// the w:val attribute is empty — there is no parent style to base
// on and no styles.xml to append a synthesised <w:style> to. Per
// ECMA-376-1 §17.7.4, when no styles part is present no style
// hierarchy exists; the empty-val pStyle is upstream's surfaced
// form of "synthesis ran but produced no id."
func optimizeWMLPart(
	src []byte,
	existingStyleIDs map[string]bool,
	defaultParagraphStyleID string,
	hasStylesPart bool,
	partStrict bool,
	idCounter *int,
	synthesised map[string]synthesisedStyle,
	orderedIDs *[]string,
) []byte {
	return optimizeWMLPartWithSource(
		src, nil,
		existingStyleIDs, defaultParagraphStyleID, hasStylesPart, partStrict,
		idCounter, synthesised, orderedIDs,
	)
}

// optimizeWMLPartWithSource is optimizeWMLPart with an optional original
// SOURCE XML argument. When srcXML is non-nil and its top-level paragraph
// count matches the post-strip src paragraph count, paragraphs whose
// SOURCE counterpart was entirely composed of revision-tracked content
// wrappers (<w:ins>/<w:del>/<w:moveTo>/<w:moveFrom>) at content level
// bypass WSO synthesis.
//
// Rationale: upstream Okapi's BlockParser-driven WSO operates on the
// Block.chunks tree where revision wrappers participate as RunContainer
// chunks; for paragraphs whose ENTIRE run content was inside revision
// wrappers, the upstream commonRunPropertiesOf walk computes properties
// from chunks that the post-write pass cannot reconstruct (the
// auto-accept-revisions unwrap at READ time discards the wrapper
// structure that the upstream filter still has visibility into via
// preserved revision events). The existing post-strip
// containsContentRevisionWrapper check at optimizeParagraph cannot
// detect this case because the read-time unwrap removed the wrappers
// before they reached the writer.
//
// Fixture 847-3.docx P2: every run is wrapped in <w:ins> (with nested
// <w:del>) in the source; the upstream reference preserves each run's
// `<w:rPr><w:b/></w:rPr>` verbatim without synthesising a paragraph
// style, while native (pre-fix) synthesised an NF974E24F-Normal1 with
// the docDefaults rFonts overlay because the post-strip paragraph
// showed no <w:ins>/<w:del> wrappers.
//
// References:
//   - ECMA-376 Part 1 / ISO/IEC 29500-1 §17.13.5.16 (CT_RunTrackChange,
//     `<w:ins>`) — revision insertion content wrappers.
//   - okapi BlockParser.parse — auto-accept-revisions unwrap of
//     transitional <w:ins> content children (SkippableElements.java:
//     209-212).
//   - okapi StyleOptimisation.Default.applyTo — chunks-level common
//     property computation that the wrappers participate in upstream.
func optimizeWMLPartWithSource(
	src []byte,
	srcXML []byte,
	existingStyleIDs map[string]bool,
	defaultParagraphStyleID string,
	hasStylesPart bool,
	partStrict bool,
	idCounter *int,
	synthesised map[string]synthesisedStyle,
	orderedIDs *[]string,
) []byte {
	if len(src) == 0 {
		return src
	}
	if !bytes.Contains(src, []byte("<w:p")) {
		return src
	}

	paragraphs := findParagraphs(src)
	if len(paragraphs) == 0 {
		return src
	}

	// Build the source-side revision-wrapped paragraph flag list. Only
	// usable when the source and post-strip paragraph counts align (i.e.
	// no merge/drop happened between read and post-strip). When counts
	// diverge we fall back to nil and the per-paragraph bypass is
	// disabled — this is conservative and never triggers false-positive
	// bypasses on fixtures where the writer's paragraph-mark-merge logic
	// changes the count.
	//
	// A paragraph qualifies for bypass when BOTH of:
	//   - its content body (direct children of <w:p> excluding <w:pPr>)
	//     is entirely composed of revision content wrappers (<w:ins>,
	//     <w:del>, <w:moveTo>, <w:moveFrom>); AND
	//   - its <w:pPr>/<w:rPr> carries a paragraph-mark deletion marker
	//     (<w:del> or <w:moveFrom>).
	//
	// The AND-pair is required to avoid bypassing paragraphs that
	// merely use <w:ins> as the only content wrapper for a single
	// translatable insertion (e.g. document-revision-information-
	// stripping.docx P4-P6, where each paragraph has one <w:ins>
	// wrapper around a single <w:r> and the upstream reference DOES
	// synthesise a paragraph style from the runs' common rPr after
	// the auto-accept-revisions unwrap). The del-paragraph-mark gate
	// targets the cross-paragraph merge scenario specifically:
	// upstream Okapi sets builder.mergeable(true) for these paragraphs
	// (BlockParser.java:207-213) and StyledTextPart.process buffers
	// them for absorption into the next block; native's writer
	// reconstruction does not always reproduce the absorption (cfs.active
	// && cfs.extractable can guard the absorption — see wml.go:2583),
	// leaving a standalone paragraph that WSO synthesises a style on
	// when upstream would have folded the runs into the merged block
	// where they participate in the next paragraph's commonRunProperties
	// computation (with different chunk structure that doesn't
	// synthesise). Fixture: 847-3.docx P2.
	var srcRevWrapped []bool
	if len(srcXML) > 0 {
		srcParas := findParagraphs(srcXML)
		if len(srcParas) == len(paragraphs) {
			srcRevWrapped = make([]bool, len(srcParas))
			for i, sp := range srcParas {
				sb := srcXML[sp.start:sp.end]
				if !paragraphAllContentRevisionWrapped(sb) {
					continue
				}
				if !paragraphSourceHasDeletedMark(sb) {
					continue
				}
				srcRevWrapped[i] = true
			}
		}
	}

	var out bytes.Buffer
	out.Grow(len(src) + 1024)
	cursor := 0
	for i, para := range paragraphs {
		out.Write(src[cursor:para.start])
		paraBytes := src[para.start:para.end]
		// Pre-recurse into any paragraphs nested within this outer
		// paragraph (typically inside a <w:drawing><wps:txbx><w:txbxContent>
		// body). Upstream Okapi treats every textbox-paragraph body as
		// its own StyledTextPart (RunBuilder + StyleOptimisation runs
		// per nested paragraph — see WordDocument.java's per-block
		// StyleOptimisation construction at line 261-271). The outer
		// drawing-bearing paragraph itself is processed below; runs
		// nested in the drawing are filtered from its findRuns scope
		// by the opaque-subtree skip (style_optimization.go:650-733
		// per the 1a3627db doc) so synthesising on the outer is a
		// no-op while the inner paragraphs need their own pass to pick
		// up textbox-body rPr lifting (AlternateContentTest.docx,
		// AlternateContent.docx, graphicdata.docx footers).
		if hasNestedParagraphs(paraBytes) {
			paraBytes = optimizeNestedParagraphs(
				paraBytes,
				existingStyleIDs, defaultParagraphStyleID, hasStylesPart, partStrict, idCounter, synthesised, orderedIDs,
			)
		}
		// Source-driven bypass: when the source paragraph at this
		// index was entirely composed of revision-wrapped content,
		// skip WSO synthesis. See the optimizeWMLPartWithSource
		// docstring for the rationale (the post-strip XML cannot
		// detect revision-wrapper paragraphs because the wrappers were
		// unwrapped at READ time — auto-accept-revisions semantics
		// per okapi SkippableElement.RevisionInline).
		if srcRevWrapped != nil && srcRevWrapped[i] {
			out.Write(paraBytes)
			cursor = para.end
			continue
		}
		rewritten := optimizeParagraph(
			paraBytes,
			existingStyleIDs, defaultParagraphStyleID, hasStylesPart, partStrict, idCounter, synthesised, orderedIDs,
		)
		// Post-pass: split any <w:r> envelope that fused multiple
		// drawing-bearing source runs whose <wp:docPr> carries a
		// translatable @name or @descr (or whose <v:textpath> carries a
		// translatable @string). Mirrors upstream Okapi RunMerger's
		// nested-items refusal — RunMerger.canMergeWith
		// (RunMerger.java:143-149) returns false when either
		// RunBuilder.containsNestedItems() is true. The flag is set by
		// RunParser.processTranslatableAttribute (RunParser.java:867)
		// whenever a translatable graphic name/desc or v:textpath string
		// is encountered. Native's parseParagraph drawing-fusion path
		// (wml.go ~2865) coalesces adjacent same-rPr drawing runs
		// without checking for translatable attributes, so we split
		// post-WSO to restore the per-source-`<w:r>` envelope
		// boundary. Fixture: delTextAmp.docx header1.xml — two adjacent
		// `<w:r><w:rPr><w:noProof/></w:rPr><w:drawing>...
		// <wp:docPr name="Picture 54" descr="HBF Logo_HorizColor copy"/>
		// ...</w:drawing></w:r>` envelopes get fused by native into one
		// `<w:r>` with two `<w:drawing>` children; bridge keeps them as
		// two `<w:r>` envelopes.
		rewritten = splitFusedTranslatableDrawingRuns(rewritten)
		out.Write(rewritten)
		cursor = para.end
	}
	out.Write(src[cursor:])
	return out.Bytes()
}

// splitFusedTranslatableDrawingRuns scans a paragraph and splits any
// `<w:r>` envelope that contains 2+ opaque drawing children (`<w:drawing>`
// or `<w:pict>`) where ANY child carries a translatable attribute marker
// — `<wp:docPr w:name="…"/>` or `<wp:docPr w:descr="…"/>` (graphic name
// or description) or `<v:textpath v:string="…"/>` — into separate
// `<w:r>` envelopes, one per opaque child.
//
// Rationale: upstream Okapi's RunMerger refuses to fuse adjacent
// `RunBuilder`s when either's `containsNestedItems` is true
// (RunMerger.canMergeWith, RunMerger.java:143-149). The flag is set by
// `RunParser.processTranslatableAttribute` (RunParser.java:860-885)
// whenever it encounters a translatable graphic name/descr or
// v:textpath string. Native's drawing-fusion path in
// `parseParagraph` (wml.go ~2865) coalesces adjacent same-rPr
// drawing-only runs without consulting the nested-items flag, so when
// the source has two adjacent `<w:r><w:rPr><w:noProof/></w:rPr><w:drawing>
// …<wp:docPr name="…"/>…</w:drawing></w:r>` envelopes they get fused
// into one `<w:r>` carrying both `<w:drawing>` children. Bridge keeps
// them separate. This post-pass restores the per-source-`<w:r>`
// envelope boundary by detecting the fused shape and splitting.
//
// The function operates on the WSO-rewritten paragraph bytes — after
// `optimizeParagraph` may have stripped common-rPr children — so the
// `<w:rPr>` we replicate is the one each split envelope actually carries
// on the wire. Drawing children without a translatable name/descr/string
// (e.g. `<w:pict>` envelopes containing only `<v:rect>` shapes, which
// `RunParser.processTranslatableAttributes` ignores) skip the split —
// upstream Okapi DOES fuse those into one `<w:r>` (neverendingloop.docx
// is the canonical preserved-fusion fixture).
//
// References:
//   - ECMA-376-1 §17.3.2.1 (CT_R) — a `<w:r>` may carry zero or more
//     `<w:drawing>` / `<w:pict>` / `<w:object>` children.
//   - ECMA-376-1 §20.4.2.5 (`<wp:docPr>`) — graphic name/descr.
//   - okapi RunMerger.canMergeWith (RunMerger.java:143-149) — nested-
//     items refusal.
//   - okapi RunParser.processTranslatableAttribute (RunParser.java:
//     860-885) — sets containsNestedItems when name/descr/string is
//     present and the translate option is enabled (defaults true for
//     all three — ConditionalParameters.java:789-792).
func splitFusedTranslatableDrawingRuns(paraSrc []byte) []byte {
	if len(paraSrc) == 0 {
		return paraSrc
	}
	// Fast bail: no drawing/pict at all, or no translatable marker.
	if !bytes.Contains(paraSrc, []byte("<w:drawing")) &&
		!bytes.Contains(paraSrc, []byte("<w:pict")) {
		return paraSrc
	}
	if !drawingHasTranslatableAttr(paraSrc) {
		return paraSrc
	}
	runs := findRuns(paraSrc)
	if len(runs) == 0 {
		return paraSrc
	}
	var out bytes.Buffer
	out.Grow(len(paraSrc) + 256)
	cursor := 0
	for _, r := range runs {
		runBytes := paraSrc[r.start:r.end]
		// Locate top-level opaque children inside this <w:r>.
		// (findOpaqueDrawingChildren skips children nested inside
		// other opaque subtrees — e.g. a nested <w:drawing> inside
		// a <w:pict>'s VML body never qualifies as a top-level
		// sibling for splitting purposes.)
		children := findOpaqueDrawingChildren(runBytes)
		if len(children) < 2 {
			continue
		}
		// At least one child must carry a translatable attribute
		// marker. If none do, leave the run intact: upstream Okapi
		// fuses non-translatable adjacent drawing/pict runs.
		anyTranslatable := false
		for _, c := range children {
			if drawingHasTranslatableAttr(runBytes[c.start:c.end]) {
				anyTranslatable = true
				break
			}
		}
		if !anyTranslatable {
			continue
		}
		// Locate the run's <w:rPr> (if any) and the run's
		// `<w:r ...>` open + `</w:r>` close so each split envelope
		// can replicate the original wrapper.
		openTagEnd := bytes.IndexByte(runBytes, '>')
		if openTagEnd < 0 {
			continue
		}
		openTag := runBytes[:openTagEnd+1]
		closeTag := []byte("</w:r>")
		// rPr child (if present) sits immediately after the open
		// tag. findFirstChild handles namespaced names without the
		// "w:" prefix.
		rPrStart, rPrEnd, hasRPr := findFirstChild(runBytes, "rPr")
		var rPrXML []byte
		if hasRPr {
			rPrXML = runBytes[rPrStart:rPrEnd]
		}
		// Emit pre-run bytes.
		out.Write(paraSrc[cursor:r.start])
		// Emit one <w:r> envelope per opaque child.
		for k, c := range children {
			out.Write(openTag)
			if hasRPr {
				out.Write(rPrXML)
			}
			// Between consecutive opaque children, the source
			// could carry whitespace or comments — keep ONLY the
			// child bytes themselves; whitespace between siblings
			// inside a fused <w:r> is non-semantic per ECMA-376
			// (CT_R children are element-only). Subsequent
			// extraction of inline text fields ensures intra-
			// child markers stay intact.
			out.Write(runBytes[c.start:c.end])
			out.Write(closeTag)
			_ = k
		}
		cursor = r.end
	}
	out.Write(paraSrc[cursor:])
	return out.Bytes()
}

// drawingHasTranslatableAttr reports whether the byte slice contains a
// translatable attribute marker on a `<wp:docPr>` or `<v:textpath>`
// element — `name="…"`, `descr="…"` (drawing properties), or
// `string="…"` (VML textpath). Used by splitFusedTranslatableDrawingRuns
// to gate the split — only runs whose payload would have triggered
// RunBuilder.setContainsNestedItems upstream are split.
//
// We accept any non-empty attribute value (mirroring upstream Okapi
// which always sets the flag when the attribute is present, regardless
// of value; see RunParser.processTranslatableAttribute,
// RunParser.java:860-885).
func drawingHasTranslatableAttr(data []byte) bool {
	// Look for <wp:docPr ... name=" or <wp:docPr ... descr=".
	for _, tag := range [][]byte{[]byte("<wp:docPr"), []byte("<v:textpath")} {
		i := 0
		for {
			idx := bytes.Index(data[i:], tag)
			if idx < 0 {
				break
			}
			abs := i + idx
			// Confirm element-name boundary.
			j := abs + len(tag)
			if j >= len(data) {
				break
			}
			b := data[j]
			if b != '>' && b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '/' {
				i = j
				continue
			}
			// Find end of this open tag.
			endIdx := bytes.IndexByte(data[j:], '>')
			if endIdx < 0 {
				break
			}
			tagBody := data[j : j+endIdx]
			var attrs []string
			if bytes.Equal(tag, []byte("<wp:docPr")) {
				attrs = []string{"name=\"", "descr=\""}
			} else {
				attrs = []string{"string=\""}
			}
			for _, a := range attrs {
				if bytes.Contains(tagBody, []byte(a)) {
					return true
				}
			}
			i = j + endIdx
		}
	}
	return false
}

// skipBalancedSubtree returns the absolute offset just past the end of
// the balanced element opened at `start` with the given open-tag form
// (`<mc:AlternateContent`, `<w:object`, …). If the element is
// self-closing or unbalanced/truncated, returns the offset just past
// the open tag (best-effort — the runs in our pipeline are always
// well-formed XML, so the balanced walk is the live path).
func skipBalancedSubtree(src []byte, start int, openTag string) int {
	if start >= len(src) {
		return len(src)
	}
	endIdx := bytes.IndexByte(src[start:], '>')
	if endIdx < 0 {
		return len(src)
	}
	openTagAbsEnd := start + endIdx
	if openTagAbsEnd > 0 && src[openTagAbsEnd-1] == '/' {
		return openTagAbsEnd + 1
	}
	closeName := []byte("</" + openTag[1:] + ">")
	depth := 1
	inner := openTagAbsEnd + 1
	for depth > 0 && inner < len(src) {
		nextOpen := bytes.Index(src[inner:], []byte(openTag))
		nextClose := bytes.Index(src[inner:], closeName)
		if nextClose < 0 {
			return len(src)
		}
		if nextOpen >= 0 && nextOpen < nextClose {
			bj := inner + nextOpen + len(openTag)
			if bj < len(src) {
				bb := src[bj]
				if bb == '>' || bb == ' ' || bb == '\t' || bb == '\n' || bb == '\r' || bb == '/' {
					kk := bytes.IndexByte(src[bj:], '>')
					if kk >= 0 {
						se := bj + kk
						if !(se > 0 && src[se-1] == '/') {
							depth++
						}
						inner = se + 1
						continue
					}
				}
			}
			inner = bj
			continue
		}
		depth--
		inner = inner + nextClose + len(closeName)
	}
	return inner
}

// opaqueDrawingChild is a top-level `<w:drawing>` or `<w:pict>` child
// inside a `<w:r>` envelope, captured by its byte offsets within the
// run.
type opaqueDrawingChild struct {
	start int
	end   int
}

// findOpaqueDrawingChildren returns the top-level `<w:drawing>` and
// `<w:pict>` children of a `<w:r>` envelope, with offsets relative to
// the run bytes. Nested drawings/picts inside a textbox or VML payload
// are excluded — only direct children of the `<w:r>` qualify.
//
// `<w:object>` and `<mc:AlternateContent>` are NOT scanned: the wml.go
// drawing-fusion path (wml.go ~2865) only fuses runs whose opaqueRunKind
// matches AND whose kind is in the fusable-drawing set — and
// `mc:AlternateContent` is explicitly excluded from fusion
// (isFusableDrawingRun, wml.go ~6000). `<w:object>` is treated like a
// drawing by the fusion path so we include `<w:object>` too if it
// surfaces; in practice we have no fixture exercising the fused-object
// translatable-attribute shape, so we keep the include conservative.
func findOpaqueDrawingChildren(runBytes []byte) []opaqueDrawingChild {
	// Skip past the <w:r ...> open tag.
	openEnd := bytes.IndexByte(runBytes, '>')
	if openEnd < 0 {
		return nil
	}
	if openEnd > 0 && runBytes[openEnd-1] == '/' {
		// Self-closing <w:r/> — no children.
		return nil
	}
	scan := openEnd + 1
	var out []opaqueDrawingChild
	for scan < len(runBytes) {
		// Find next opening tag.
		lt := bytes.IndexByte(runBytes[scan:], '<')
		if lt < 0 {
			break
		}
		abs := scan + lt
		if abs+1 >= len(runBytes) {
			break
		}
		// Bail out if we've reached </w:r>.
		if bytes.HasPrefix(runBytes[abs:], []byte("</w:r>")) {
			break
		}
		// Identify the element name.
		var matched string
		for _, tag := range []string{"<w:drawing", "<w:pict"} {
			if bytes.HasPrefix(runBytes[abs:], []byte(tag)) {
				bj := abs + len(tag)
				if bj >= len(runBytes) {
					return out
				}
				b := runBytes[bj]
				if b == '>' || b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '/' {
					matched = tag
					break
				}
			}
		}
		// Sibling opaque envelopes that aren't drawing/pict (mc:Alternate
		// Content, w:object, w:ruby, w:fldSimple, …) must be skipped as
		// balanced subtrees so we don't descend into them and pick up
		// nested <w:drawing>/<w:pict> children as if they were direct
		// siblings of this <w:r>. Fixture: 992.docx header1.xml — a
		// source `<w:r>` carries `<mc:AlternateContent>...
		// <w:drawing>...</w:drawing></mc:AlternateContent><w:drawing>...
		// </w:drawing></w:r>`; the outer AlternateContent must be
		// skipped as a single sibling so the drawing inside its
		// `<mc:Choice>` doesn't surface as a top-level child alongside
		// the trailing `<w:drawing>`.
		if matched == "" {
			skipped := false
			for _, skipTag := range []string{"<mc:AlternateContent", "<w:object", "<w:ruby", "<w:fldSimple"} {
				if !bytes.HasPrefix(runBytes[abs:], []byte(skipTag)) {
					continue
				}
				bj := abs + len(skipTag)
				if bj >= len(runBytes) {
					return out
				}
				b := runBytes[bj]
				if b != '>' && b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '/' {
					continue
				}
				scan = skipBalancedSubtree(runBytes, abs, skipTag)
				skipped = true
				break
			}
			if skipped {
				continue
			}
			// Plain non-opaque element — skip past this open tag.
			endIdx := bytes.IndexByte(runBytes[abs:], '>')
			if endIdx < 0 {
				break
			}
			scan = abs + endIdx + 1
			continue
		}
		// Find balanced close for this opaque element.
		openTagEnd := bytes.IndexByte(runBytes[abs:], '>')
		if openTagEnd < 0 {
			break
		}
		openTagAbsEnd := abs + openTagEnd
		// Self-closing form: e.g. <w:drawing/>.
		if openTagAbsEnd > 0 && runBytes[openTagAbsEnd-1] == '/' {
			out = append(out, opaqueDrawingChild{start: abs, end: openTagAbsEnd + 1})
			scan = openTagAbsEnd + 1
			continue
		}
		closeName := []byte("</" + matched[1:] + ">")
		// Depth-counted walk to handle nested same-name elements.
		depth := 1
		inner := openTagAbsEnd + 1
		for depth > 0 && inner < len(runBytes) {
			nextOpen := bytes.Index(runBytes[inner:], []byte(matched))
			nextClose := bytes.Index(runBytes[inner:], closeName)
			if nextClose < 0 {
				return out
			}
			if nextOpen >= 0 && nextOpen < nextClose {
				bj := inner + nextOpen + len(matched)
				if bj < len(runBytes) {
					bb := runBytes[bj]
					if bb == '>' || bb == ' ' || bb == '\t' || bb == '\n' || bb == '\r' || bb == '/' {
						// Determine self-closing.
						kk := bytes.IndexByte(runBytes[bj:], '>')
						if kk >= 0 {
							se := bj + kk
							if !(se > 0 && runBytes[se-1] == '/') {
								depth++
							}
							inner = se + 1
							continue
						}
					}
				}
				inner = bj
				continue
			}
			depth--
			inner = inner + nextClose + len(closeName)
		}
		out = append(out, opaqueDrawingChild{start: abs, end: inner})
		scan = inner
	}
	return out
}

// paragraphSourceHasDeletedMark reports whether a SOURCE paragraph's
// <w:pPr> contains a paragraph-mark deletion marker (<w:del> or
// <w:moveFrom> inside <w:pPr>/<w:rPr>). Used together with
// paragraphAllContentRevisionWrapped to decide whether to bypass WSO
// on a paragraph that upstream Okapi would have folded into the next
// block via the auto-accept-revisions paragraph-mark merge path. The
// underlying paragraphHasDeletedMark walker is in wml.go and accepts
// just the <w:pPr> element raw XML; this helper extracts that element
// from the paragraph and forwards.
func paragraphSourceHasDeletedMark(paraSrc []byte) bool {
	pPrStart, pPrEnd, hasPPr := findFirstChild(paraSrc, "pPr")
	if !hasPPr {
		return false
	}
	return paragraphHasDeletedMark(string(paraSrc[pPrStart:pPrEnd]))
}

// paragraphAllContentRevisionWrapped reports whether every direct child
// content element of a SOURCE paragraph (excluding <w:pPr>) is wrapped
// in a tracked-revision content element (<w:ins>, <w:del>, <w:moveTo>,
// <w:moveFrom>). Returns false when the paragraph mixes plain <w:r>
// children with revision-wrapped runs, or contains no revision
// wrappers at all.
//
// The check operates on the raw SOURCE paragraph (before
// stripWMLSkippableElements) — only this view has the content wrappers
// because the writer's auto-accept-revisions unwrap at READ time
// (BlockParser.parse case "ins"/"moveTo" handling) discards the wrapper
// element before its inner runs reach the post-strip XML.
//
// Self-closing/empty-body revision elements (the paragraph-mark
// variants `<w:ins/>` / `<w:del/>` etc. that live inside `<w:pPr>/<w:rPr>`)
// are NOT counted as content wrappers — they are accounted for
// separately by stripWMLSkippableElements and the BlockParser
// mergeable-paragraph machinery.
//
// Mirrors the auto-accept-revisions semantics in okapi
// SkippableElement.RevisionInline (SkippableElement.java:209-214) which
// unwraps `<w:ins>` / `<w:moveTo>` and discards `<w:del>` / `<w:moveFrom>`
// at read time.
func paragraphAllContentRevisionWrapped(src []byte) bool {
	// Find pPr range (must be the first child if present) so we can
	// skip past it. The pPr's own <w:rPr> may contain
	// `<w:ins>` / `<w:del>` paragraph-mark variants that don't count.
	pPrStart, pPrEnd, hasPPr := findFirstChild(src, "pPr")
	// Locate the start of the paragraph's content (after the opening
	// <w:p ...> tag).
	openEnd := bytes.IndexByte(src, '>')
	if openEnd < 0 {
		return false
	}
	bodyStart := openEnd + 1
	// Strip the trailing </w:p> closer.
	bodyEnd := len(src)
	if idx := bytes.LastIndex(src, []byte("</w:p>")); idx > bodyStart {
		bodyEnd = idx
	}
	// Skip past pPr inside the body if it's present.
	contentStart := bodyStart
	if hasPPr && pPrStart >= bodyStart && pPrEnd <= bodyEnd {
		contentStart = pPrEnd
	}
	body := src[contentStart:bodyEnd]
	if len(body) == 0 {
		return false
	}
	// Walk direct-child elements. We require:
	//   1. At least one revision content wrapper child is present, AND
	//   2. Every non-whitespace, non-bookmarkStart/End, non-empty-revision
	//      direct child is itself a revision content wrapper.
	revWrapperNames := map[string]bool{
		"ins":      true,
		"del":      true,
		"moveTo":   true,
		"moveFrom": true,
	}
	// Direct-children that don't carry rendered run content and don't
	// disqualify the "all wrapped" determination: bookmarkStart,
	// bookmarkEnd, commentRangeStart, commentRangeEnd, proofErr.
	// These are revision/annotation markers that surround content
	// without participating in WSO's commonRunPropertiesOf.
	transparentChildNames := map[string]bool{
		"bookmarkStart":      true,
		"bookmarkEnd":        true,
		"commentRangeStart":  true,
		"commentRangeEnd":    true,
		"commentReference":   true,
		"proofErr":           true,
		"permStart":          true,
		"permEnd":            true,
	}
	sawRevWrapper := false
	depth := 0
	i := 0
	for i < len(body) {
		ch := body[i]
		// Skip whitespace at depth 0.
		if depth == 0 && (ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r') {
			i++
			continue
		}
		if ch != '<' {
			i++
			continue
		}
		// Start tag, end tag, or self-closing?
		if i+1 < len(body) && body[i+1] == '/' {
			// End tag at depth 0 means premature close — shouldn't happen
			// in well-formed XML, but bail safely.
			if depth == 0 {
				return false
			}
			// Find tag end
			end := bytes.IndexByte(body[i:], '>')
			if end < 0 {
				return false
			}
			depth--
			i += end + 1
			continue
		}
		// Start tag (or self-closing). Only inspect direct children at
		// depth 0.
		if depth > 0 {
			// Scan past this nested start tag to track depth.
			end := bytes.IndexByte(body[i:], '>')
			if end < 0 {
				return false
			}
			selfClosing := end > 0 && body[i+end-1] == '/'
			if !selfClosing {
				depth++
			}
			i += end + 1
			continue
		}
		// At depth 0: inspect element name.
		end := bytes.IndexByte(body[i:], '>')
		if end < 0 {
			return false
		}
		tag := body[i : i+end+1]
		selfClosing := end > 0 && body[i+end-1] == '/'
		// Extract local name. Tag form: "<w:name ..." or "<name ...".
		nameStart := i + 1
		nameEnd := nameStart
		for nameEnd < i+end && body[nameEnd] != ' ' && body[nameEnd] != '\t' &&
			body[nameEnd] != '\n' && body[nameEnd] != '\r' && body[nameEnd] != '>' &&
			body[nameEnd] != '/' {
			nameEnd++
		}
		name := string(body[nameStart:nameEnd])
		// Strip namespace prefix.
		if colon := strings.IndexByte(name, ':'); colon >= 0 {
			name = name[colon+1:]
		}
		_ = tag
		if revWrapperNames[name] {
			// A revision content wrapper. Self-closing forms are the
			// paragraph-mark variants (empty body) — those don't count
			// as content wrappers.
			if !selfClosing {
				sawRevWrapper = true
				depth++
				i += end + 1
				continue
			}
			// Self-closing revision element at depth 0 is unusual
			// (paragraph-mark variants live INSIDE pPr/rPr, not at
			// paragraph content level). Treat as transparent — don't
			// disqualify and don't count as wrapper.
			i += end + 1
			continue
		}
		// Transparent / non-disqualifying direct children.
		if transparentChildNames[name] {
			if !selfClosing {
				depth++
			}
			i += end + 1
			continue
		}
		// Any other direct child (notably <w:r>, <w:fldSimple>, <w:hyperlink>,
		// <w:sdt>, etc.) disqualifies the "all wrapped" determination.
		return false
	}
	return sawRevWrapper
}

// hasNestedParagraphs reports whether src (one outer paragraph
// extent) contains additional <w:p> elements nested inside it
// (typically textbox-body paragraphs within a <w:drawing>).
// Quick byte scan: any <w:p... after the first opening tag,
// before the matching </w:p> closer, indicates nesting.
func hasNestedParagraphs(src []byte) bool {
	// Skip past the outer <w:p ...> start tag.
	i := bytes.IndexByte(src, '>')
	if i < 0 {
		return false
	}
	rest := src[i+1:]
	// Strip the trailing </w:p> closer.
	closer := []byte("</w:p>")
	if cidx := bytes.LastIndex(rest, closer); cidx >= 0 {
		rest = rest[:cidx]
	}
	openTag := []byte("<w:p")
	for j := 0; j < len(rest); {
		k := bytes.Index(rest[j:], openTag)
		if k < 0 {
			return false
		}
		bj := j + k + len(openTag)
		if bj >= len(rest) {
			return false
		}
		b := rest[bj]
		if b == '>' || b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '/' {
			return true
		}
		j = bj
	}
	return false
}

// optimizeNestedParagraphs recursively applies optimizeWMLPart to
// any nested paragraphs inside src. Used for textbox-body
// paragraphs wrapped in <w:drawing><wps:txbx><w:txbxContent>...
// (AlternateContentTest.docx, AlternateContent.docx) and for any
// other site where Okapi's per-block StyleOptimisation reaches a
// nested paragraph that the outer-level findParagraphs walk
// skipped.
//
// Strategy: locate the inner <w:txbxContent>...</w:txbxContent>
// (or nested-paragraph window in general) and recurse with
// optimizeWMLPart. This keeps the SHARED idCounter, synthesised
// map, and orderedIDs in lockstep so styleId numbering matches
// upstream's per-document IdGenerator stream.
//
// Both `<w:txbxContent>` (Transitional WPML) and `<wne:txbxContent>`
// (Strict OOXML, bound to the wordml extension namespace
// "http://schemas.openxmlformats.org/wordml/...") are recognised.
// Per ECMA-376-1 / ISO/IEC 29500-1, the textbox-content element is
// part of the WordprocessingML schema in both conformance classes;
// Word's "Save As → Strict Open XML Document" emits it with the
// `wne:` prefix on the WORDML extension namespace. Fixture 859.docx
// is the canonical case — its drawing-bearing paragraph carries
// `<wp:txbx><wne:txbxContent><w:p>...` and the inner paragraph's
// `<w:lang w:val="en-US"/>` rPr must be lifted into a synthesised
// pStyle ("NF974E24F-Normal1") to match upstream Okapi's reference
// output (which does the same lift via WordDocument.java's per-block
// StyleOptimisation construction at line 261-271).
func optimizeNestedParagraphs(
	src []byte,
	existingStyleIDs map[string]bool,
	defaultParagraphStyleID string,
	hasStylesPart bool,
	partStrict bool,
	idCounter *int,
	synthesised map[string]synthesisedStyle,
	orderedIDs *[]string,
) []byte {
	// Find every <*:txbxContent>...</*:txbxContent> inside src and
	// recurse into its body. Both `<w:txbxContent>` (Transitional WPML
	// — AlternateContentTest.docx, AlternateContent.docx, graphicdata.
	// docx footers) and `<wne:txbxContent>` (Strict OOXML — 859.docx)
	// are scanned. Other nested-paragraph carriers (e.g. footnote
	// references, custom XML) don't typically appear inside a textbox/
	// drawing scope and need their own part-level treatment —
	// txbxContent is the canonical case.
	variants := []struct {
		open  []byte
		close []byte
	}{
		{[]byte("<w:txbxContent"), []byte("</w:txbxContent>")},
		{[]byte("<wne:txbxContent"), []byte("</wne:txbxContent>")},
	}
	var out bytes.Buffer
	out.Grow(len(src))
	cursor := 0
	for cursor < len(src) {
		// Find the earliest opener of any variant past cursor.
		bestOI := -1
		bestVar := -1
		for vi, v := range variants {
			oi := bytes.Index(src[cursor:], v.open)
			if oi < 0 {
				continue
			}
			if bestOI < 0 || oi < bestOI {
				bestOI = oi
				bestVar = vi
			}
		}
		if bestOI < 0 {
			break
		}
		v := variants[bestVar]
		// Skip past the start tag's terminator.
		k := bytes.IndexByte(src[cursor+bestOI:], '>')
		if k < 0 {
			break
		}
		bodyStart := cursor + bestOI + k + 1
		ci := bytes.Index(src[bodyStart:], v.close)
		if ci < 0 {
			break
		}
		bodyEnd := bodyStart + ci
		out.Write(src[cursor:bodyStart])
		body := src[bodyStart:bodyEnd]
		// Recursively optimize the txbxContent body — its inner
		// paragraphs surface to optimizeWMLPart's outer-level
		// findParagraphs walk on the recursive call.
		out.Write(optimizeWMLPart(body, existingStyleIDs, defaultParagraphStyleID, hasStylesPart, partStrict, idCounter, synthesised, orderedIDs))
		cursor = bodyEnd
	}
	out.Write(src[cursor:])
	return out.Bytes()
}

// findParagraphs walks src and returns the byte ranges of every
// top-level <w:p>...</w:p> element. Paragraphs nested inside <w:tbl>,
// <w:txbxContent> etc. are also found because the matcher is purely
// structural — every "<w:p" with a balanced "</w:p>" qualifies. Self-
// closing <w:p/> paragraphs are skipped (they have no runs).
func findParagraphs(src []byte) []rawParagraph {
	var out []rawParagraph
	openTag := []byte("<w:p")
	closeTag := []byte("</w:p>")
	i := 0
	for i < len(src) {
		idx := bytes.Index(src[i:], openTag)
		if idx < 0 {
			break
		}
		start := i + idx
		// Confirm element-name boundary — next char must be `>`, ` ` or `/`.
		// Reject "<w:pPr", "<w:pgSz", "<w:pgMar", "<w:pStyle", etc.
		j := start + len(openTag)
		if j >= len(src) {
			break
		}
		b := src[j]
		if b != '>' && b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '/' {
			i = j
			continue
		}
		// Find the start tag's terminator
		k := bytes.IndexByte(src[j:], '>')
		if k < 0 {
			break
		}
		startTagEnd := j + k
		// Self-closing <w:p/>?
		if startTagEnd > 0 && src[startTagEnd-1] == '/' {
			i = startTagEnd + 1
			continue
		}
		// Find matching </w:p> — must skip nested <w:p> (which can occur
		// in textbox content). Simple depth counter on top-level w:p
		// open/close tags.
		depth := 1
		scan := startTagEnd + 1
		for depth > 0 && scan < len(src) {
			ni := bytes.Index(src[scan:], openTag)
			ci := bytes.Index(src[scan:], closeTag)
			if ci < 0 {
				return out // unbalanced — bail
			}
			// If a nested <w:p starts before the next </w:p>, increase depth.
			if ni >= 0 && ni < ci {
				abs := scan + ni
				bj := abs + len(openTag)
				if bj < len(src) {
					bb := src[bj]
					if bb == '>' || bb == ' ' || bb == '\t' || bb == '\n' || bb == '\r' || bb == '/' {
						// Confirm not self-closing
						k := bytes.IndexByte(src[bj:], '>')
						if k >= 0 {
							se := bj + k
							if !(se > 0 && src[se-1] == '/') {
								depth++
							}
							scan = se + 1
							continue
						}
					}
				}
				scan = bj
				continue
			}
			// Match a </w:p>
			depth--
			scan = scan + ci + len(closeTag)
		}
		out = append(out, rawParagraph{start: start, end: scan})
		i = scan
	}
	return out
}

// runEntry captures a parsed <w:r>...</w:r> with its rPr properties.
type runEntry struct {
	runStart, runEnd int // <w:r ...> ... </w:r> range
	rPrStart, rPrEnd int // <w:rPr> ... </w:rPr> range (or zero if no rPr)
	hasRPr           bool
	props            []runProp
	excluded         bool // run carries an exclusion property
	// csOnlyText is true when the run's text is purely complex-script
	// (no detected ASCII / HighAnsi / EastAsian content categories).
	// Mirrors upstream Okapi RunParser.java:208-217 which strips
	// b/i/sz from such runs at parse time. Native applies the strip
	// downstream — see optimizeParagraph for the rationale.
	csOnlyText bool
	// fieldContentRun is true when this <w:r> is part of a complex
	// field's body chunks per the upstream model — any run that sits
	// strictly between a `<w:fldChar fldCharType="begin"/>` run (the
	// "outer" run that owns the complex-field span) and the matching
	// `<w:fldChar fldCharType="end"/>` run, INCLUDING the
	// instrText-bearing runs, the fldChar=separate run, the
	// fldChar=end run itself, AND any plain text-bearing runs that
	// appear between fldChar=separate and fldChar=end (the field's
	// rendered display content, e.g. `<w:r><w:t>2020</w:t></w:r>`
	// inside a `DATE` field's result region).
	//
	// Upstream Okapi's RunParser.parseComplexField (RunParser.java:
	// 461-542) routes every event between fldChar=begin and
	// fldChar=end into the SAME RunBuilder via runBuilder.addToMarkup
	// (line 505). The result is ONE Run object whose body chunks
	// carry the instrText / fldChar separate/end events AND the
	// display-content runs as Markup. Only the outer Run object's
	// RunProperties (the begin run's rPr) are passed to
	// RunBuilder.setRunProperties (RunParser.java:280-294); the
	// body-chunk runs' rPr survives verbatim through the Markup
	// events.
	//
	// At StyleOptimisation time (StyleOptimisation.java:204-237,
	// commonRunPropertiesOf, and 240-249, refineRuns) only the
	// OUTER Run is visited as a Chunk — display-text runs INSIDE
	// the field span are body chunks of that Run and are invisible
	// to both common-prop computation AND the per-run rPr refine
	// pass. Per ECMA-376-1 §17.3.2.1 (CT_R) and §17.16.5 (Complex
	// Fields) every `<w:r>` is structurally a run regardless of
	// whether it carries fldChar/instrText/text markup, but the
	// upstream pipeline collapses them for WSO scope.
	//
	// Native's reader/writer emit each `<w:r>` inside the field as
	// a separate model.Run entry (paragraph-level findRuns sees
	// them as siblings), so without this flag WSO would BOTH (a)
	// fold the field-content rPr into the common-prop computation
	// (skewing the lift) AND (b) strip the lifted props from each
	// field-content run on the way out. Marking field-content runs
	// — including display-text runs between separate/end — and
	// excluding them from BOTH the seed/intersection (commonProps)
	// AND the per-run strip (optimizeParagraph's run-rewrite loop)
	// aligns the visible XML output with upstream's "only the outer
	// run is refined" model.
	//
	// Fixtures:
	//   - Mauris.docx — paragraph contains one EQ field with
	//     fldChar=begin / instrText / fldChar=end runs all carrying
	//     rPr `<w:noProof/><w:sz w:val="144"/><w:szCs w:val="144"/>`.
	//     Upstream emits the BEGIN run with empty rPr and every
	//     instrText / END run with `<w:sz w:val="144"/><w:szCs
	//     w:val="144"/>` preserved (Normal1 also synth'd with sz/szCs).
	//   - 956.docx — footer paragraph contains a DATE field whose
	//     display-text run `<w:r><w:rPr><w:noProof/><w:sz
	//     w:val="14"/></w:rPr><w:t>2020</w:t></w:r>` sits between
	//     fldChar=separate and fldChar=end. Upstream preserves that
	//     run's rPr verbatim (sz=14 is not stripped) because the
	//     display-text run is a Markup body chunk of the outer Run.
	fieldContentRun bool

	// breakOnlyRun is true when this <w:r> carries a `<w:br/>` element
	// and no text. Mirrors upstream Okapi WordStyleOptimisation's
	// treatment of standalone-break Markup body chunks: the br run's
	// rPr is invisible to commonRunPropertiesOf (StyleOptimisation.java:
	// 204-237) because the break itself is a Markup component whose
	// formatting toggles render no text. Excluding break-only runs
	// from BOTH the seed/intersection (commonProps) AND the per-run
	// strip (optimizeParagraph's run-rewrite loop) aligns the visible
	// XML output with upstream's "only text-bearing runs are refined"
	// model for break-bearing paragraphs. See runIsBreakOnly for the
	// citation and EndGroup.docx fixture rationale.
	breakOnlyRun bool
}

// optimizeParagraph rewrites a single <w:p>...</w:p> block applying
// AllowWordStyleOptimisation. Returns the original bytes if no
// optimisation is applicable (or if structure is too unusual to
// safely transform).
func optimizeParagraph(
	src []byte,
	existingStyleIDs map[string]bool,
	defaultParagraphStyleID string,
	hasStylesPart bool,
	partStrict bool,
	idCounter *int,
	synthesised map[string]synthesisedStyle,
	orderedIDs *[]string,
) []byte {
	// Find pPr (or its absence). pPr must be the first child if present.
	pPrStart, pPrEnd, hasPPr := findFirstChild(src, "pPr")
	pStyleID := ""
	if hasPPr {
		// Extract any <w:pStyle w:val="..."/> already present.
		pStyleID = extractAttrVal(src[pPrStart:pPrEnd], "pStyle", "w:val")
	}

	// Bail if paragraph contains tracked-revision content wrappers
	// (<w:ins> or <w:del> at content level, NOT inside <w:rPr>) — these
	// confuse the inner-chunks/exclusion checks in Okapi's
	// StyleOptimisation and the safe path is bypass. Native's
	// stripWMLSkippableElements has already removed the empty-form
	// (paragraph-mark) <w:ins>/<w:del> from inside <w:rPr>, so any
	// surviving instance is a content wrapper.
	//
	// EXCEPTION: For Strict-OOXML parts (xmlns="http://purl.oclc.org/
	// ooxml/wordprocessingml/main") the reader's parseRevisionInsertion
	// preserves <w:ins>/<w:moveTo> as paired-code wrappers around the
	// inner runs (see vocabulary.go TypeRevisionIns) — there is no
	// auto-accept-revisions unwrap to mismatch with. Upstream Okapi's
	// StyleOptimisation still walks into the preserved wrappers and
	// computes common rPr across the inner runs (the bypass exists in
	// upstream only for the transitional-namespace empty-form
	// paragraph-mark variant inside pPr's rPr, which our pipeline has
	// already stripped). 859.docx is the canonical fixture — its first
	// paragraph carries `<w:r>...<w:t>Saving as OOXML Strict...</w:t>
	// </w:r><w:ins>...<w:r>...<w:t> New text...</w:t></w:r></w:ins>`,
	// and the reference output synthesises a Normal1 pStyle from the
	// shared `<w:lang w:val="en-US"/>` across both runs.
	if !partStrict && containsContentRevisionWrapper(src, pPrStart, pPrEnd, hasPPr) {
		return src
	}

	// Collect run rPr blocks (and the runs themselves so we can rewrite).
	runs := findRuns(src)
	if len(runs) < 1 {
		// Empty paragraph (no runs at all) — nothing to optimise.
		return src
	}
	// Threshold note: upstream Okapi optimises 1-run paragraphs too
	// (StyleOptimisation.Default.applyTo line 98 bypasses only when
	// chunks.size() <= 2 — i.e. 0 runs in addition to outer markup).
	// With #592 the native writer now preserves per-source-run rPr on
	// every emitted <w:r>, so 1-run paragraphs carry the same rPr
	// payload Okapi sees and the optimisation premise — common props
	// across rendered runs — holds for them too. Pre-#592 the native
	// reader/writer aggressively collapsed source runs into a single
	// rPr-less <w:r>, so a 2+ threshold was used as a safety net to
	// avoid synthesising styles upstream did not.
	entries := make([]runEntry, 0, len(runs))
	for _, r := range runs {
		e := runEntry{runStart: r.start, runEnd: r.end}
		rps, rpe, has := findFirstChild(src[r.start:r.end], "rPr")
		if has {
			e.hasRPr = true
			e.rPrStart = r.start + rps
			e.rPrEnd = r.start + rpe
			e.props = parseRunPropElements(src[e.rPrStart:e.rPrEnd])
			for _, p := range e.props {
				if runPropExclusions[p.name] {
					e.excluded = true
					break
				}
			}
			// Symmetric counterpart of writer.go's stripToggleMirrorChildren
			// — drop <w:b/> and <w:i/> from runs whose text is purely
			// complex-script. Upstream Okapi's RunParser.endRunParsing
			// (RunParser.java:208-217) classifies each run's text via
			// ContentCategoriesDetection and adds RUN_PROPERTY_BOLD /
			// RUN_PROPERTY_ITALICS to the run's skippableProperties when
			// runFonts.containsDetectedNonComplexScriptContentCategories
			// returns false (i.e. no ASCII / HighAnsi / EastAsian chars),
			// then filters those properties out of the run's RunProperties
			// before WSO sees them (RunParser.java:230-232). The neokapi
			// reader keeps b/i alive through the run-codes / runProps
			// reconstruction in writer.go (PcOpen TypeBold / TypeItalic →
			// addWMLProp emits `<w:b/>` / `<w:i/>` after the per-run
			// sidecar), so the b/i appears in the EMITTED rPr that WSO's
			// post-pass sees. Without this strip the toggles get lifted
			// into the synthesised paragraph style for paragraphs that mix
			// CS-only and non-CS runs (947-non-cs-and-cs.docx — reference
			// synth carries only `<w:sz/><w:szCs/>` while native promoted
			// `<w:b/><w:i/>` from the LTR run because the CS-only run also
			// reconstructed b/i in its emitted rPr).
			//
			// We drop ONLY b and i — sz is conditionally preserved in
			// upstream Okapi via `canBeSkipped` (RunParser.java:236-250),
			// which checks the value against the inherited style chain
			// and keeps the property when the inherited value differs
			// (947-cs.docx: pre.sz=24 from docDefaults, direct.sz=28 → not
			// stripped). Native cannot reconstruct the inherited chain at
			// WSO time, so we mirror only the toggles whose default chain
			// is empty (b/i are absent in Normal + docDefaults across the
			// fixture corpus) and skip the value-dependent sz strip.
			//
			// References:
			//   - okapi RunParser.java:208-217 — symmetric strip trigger.
			//   - okapi ContentCategoriesDetection.java:37-49,56,64,84,
			//     147-154 — CS vs non-CS detection patterns.
			//   - okapi RunFonts.java:156-163 —
			//     containsDetectedNonComplexScriptContentCategories.
			//   - ECMA-376-1 §17.3.2.1 (`<w:b>`), §17.3.2.13 (`<w:i>`).
			if e.hasRPr && textIsAllComplexScript(extractRunText(src[r.start:r.end])) {
				e.csOnlyText = true
				e.props = stripWMLNamesFromProps(e.props, "b", "i")
			}
			// Drawing-only runs (run carries `<w:drawing>` / `<w:pict>`
			// / `<w:object>` / `<mc:AlternateContent>` and no `<w:t>`)
			// don't render text — toggle properties like `<w:b>` /
			// `<w:i>` / `<w:u>` on their rPr have no rendering effect.
			// Mirrors upstream Okapi's RunBuilder which materialises
			// MarkupComponent runs without surfacing their direct
			// RunProperties' rendering toggles into the
			// commonRunProperties pass: WordStyleOptimisation lifts
			// only properties that contribute to TEXT formatting, and
			// drawing-only runs aren't text-formatting carriers.
			//
			// AlternateContentTest.docx canonical case: the textbox-
			// bearing paragraph holds 1 run with rPr `<w:i/><w:iCs/>
			// <w:noProof/><w:sz w:val="18"/><w:szCs w:val="18"/><w:lang
			// .../>` and an `<mc:AlternateContent>` body. Upstream
			// synthesised Style12 lifts only `<w:sz/><w:szCs/>` (the
			// font-size shape that DOES affect drawing layout via the
			// inherited paragraph-mark rPr), dropping `<w:i/>` /
			// `<w:iCs/>` / `<w:noProof/>` / `<w:lang/>` because they
			// have no effect on a drawing-only run.
			//
			// We strip b/i (the toggle pair native promotes from
			// boolean fields) here so the WSO common-prop computation
			// matches upstream's emitted style. Other rPr children
			// like noProof/lang remain in the props slice — they
			// don't typically appear in commonForStyle either, but
			// when they do, the synth match is fixture-specific and
			// the targeted strip below avoids over-eager removal.
			if e.hasRPr && runIsDrawingOnly(src[r.start:r.end]) {
				e.props = stripWMLNamesFromProps(e.props, "b", "i")
			}
		}
		// Mark break-only runs so they're excluded from common-rPr
		// computation downstream (matches upstream Okapi WSO's
		// "Markup chunks don't contribute to commonRunPropertiesOf"
		// model). See runIsBreakOnly + breakOnlyRun docstring.
		if runIsBreakOnly(src[r.start:r.end]) {
			e.breakOnlyRun = true
		}
		entries = append(entries, e)
	}

	// Detect field-content runs by tracking complex-field nesting
	// depth across the paragraph's run siblings. A run is field-
	// content iff it is a body chunk of an enclosing outer Run per
	// upstream Okapi's parseComplexField model — i.e. it sits
	// strictly between a `<w:fldChar fldCharType="begin"/>` run (the
	// "outer" run that owns the field span) and the matching
	// `<w:fldChar fldCharType="end"/>` run, INCLUSIVE of the end run
	// itself (which is a Markup body chunk of the outer Run).
	//
	// The classification is depth-sensitive to handle three corner
	// cases:
	//
	//   1. Nested complex fields — `<w:r><w:fldChar="begin"/></w:r>`
	//      INSIDE an already-open field becomes a body chunk of the
	//      outer Run. Upstream's parseComplexField recurses
	//      (RunParser.java:494-499 isComplexFieldBegin branch); the
	//      nested begin/instrText/separate/end runs are all Markup
	//      chunks of the outermost Run. Our flag follows the same
	//      contract: a begin run is field-content only if fieldDepth
	//      was already > 0 when we encountered it.
	//
	//   2. Orphan fldChar=end at paragraph start — common in
	//      paragraphs that continue a field opened in the prior
	//      paragraph (e.g. 830-5.docx p[5] starts with
	//      `<w:r><w:fldChar fldCharType="end"/></w:r>` followed by a
	//      regular text run). Upstream's RunParser is per-paragraph
	//      (paragraph boundaries reset the RunBuilder); the orphan
	//      end run becomes a plain Run with the fldChar as run body
	//      events, and IS subject to commonRunPropertiesOf. Our flag
	//      mirrors this: an end run is field-content only if there's
	//      an open field at our depth (fieldDepth > 0).
	//
	//   3. instrText / fldChar=separate outside any field span —
	//      these are not legally producible by Word but the parser
	//      should not panic. We require fieldDepth > 0 for them too
	//      so a malformed orphan stays a regular run.
	//
	// References:
	//   - RunParser.java:461-542 (parseComplexField) — recursive
	//     descent, addToMarkup for body events.
	//   - StyleOptimisation.java:204-237 (commonRunPropertiesOf) —
	//     only iterates Chunk-level Runs.
	//   - ECMA-376-1 §17.16.5 (Complex Fields) — fldChar
	//     begin/separate/end sequencing.
	fieldDepth := 0
	for i := range entries {
		runBody := src[entries[i].runStart:entries[i].runEnd]
		hasBegin := bytes.Contains(runBody, []byte(`<w:fldChar w:fldCharType="begin"`))
		hasEnd := bytes.Contains(runBody, []byte(`<w:fldChar w:fldCharType="end"`))
		hasSep := bytes.Contains(runBody, []byte(`<w:fldChar w:fldCharType="separate"`))
		hasInstr := bytes.Contains(runBody, []byte("<w:instrText"))

		switch {
		case hasBegin:
			// A nested-begin sitting inside an already-open field is
			// a body chunk of the outer Run (case 1 above).
			if fieldDepth > 0 {
				entries[i].fieldContentRun = true
			}
			fieldDepth++
		case hasEnd:
			// An end run closes the current field span; mark it as
			// field-content BEFORE decrementing so it counts as part
			// of the span it's closing. Orphan end runs (fieldDepth
			// == 0 — case 2) stay as regular runs upstream-equivalently.
			if fieldDepth > 0 {
				entries[i].fieldContentRun = true
				fieldDepth--
			}
		case hasInstr, hasSep:
			// instrText / separate runs are field-content only inside
			// an open field (case 3). Orphans (malformed XML) stay
			// regular runs.
			if fieldDepth > 0 {
				entries[i].fieldContentRun = true
			}
		default:
			// Plain run (text / drawing / hyperlink markup / etc).
			// It's field-content iff it sits inside an open field.
			// This covers the display-text region between
			// fldChar=separate and fldChar=end (e.g.
			// `<w:r><w:t>2020</w:t></w:r>` for a DATE field's
			// rendered result in 956.docx footer1.xml).
			if fieldDepth > 0 {
				entries[i].fieldContentRun = true
			}
		}
	}

	// Compute common props across all runs. If any non-field-content
	// run has empty rPr, commons is empty (per Okapi: "if direct
	// properties empty, commonRunProperties.clear()"). Field-content
	// runs (instrText / fldChar / display-text runs between
	// separate/end) are body chunks of the outer Run upstream and
	// are invisible to commonRunPropertiesOf — they cannot trigger
	// the empty-rPr bypass. See runEntry.fieldContentRun for the
	// upstream contract.
	// Pre-compute whether to skip break-only runs in the early-bail
	// scan. We skip them only when there's at least one non-field-
	// content, non-break-only entry — see commonProps for the same
	// gate's rationale.
	skipBreakOnlyEarly := false
	if hasBreakOnly(entries) {
		for _, e := range entries {
			if !e.fieldContentRun && !e.breakOnlyRun {
				skipBreakOnlyEarly = true
				break
			}
		}
	}
	for _, e := range entries {
		if e.fieldContentRun {
			// Field-content runs (between fldChar=begin and matching
			// fldChar=end) live as Markup/RunText body chunks of the
			// OUTER field Run upstream — they are invisible to
			// StyleOptimisation.innerChunksContainExclusions
			// (StyleOptimisation.java:131-150), which iterates only
			// Chunk-level Runs and RunContainers. The outer field Run's
			// direct properties are what the exclusion check sees; an
			// rStyle on a field-display run (e.g. <w:rStyle val=
			// "Hyperlink"/> on the rendered link text of a HYPERLINK
			// field) does NOT reach the exclusion gate upstream and so
			// does NOT block the synthesis. Mirror that by skipping the
			// excluded check for field-content runs — they still don't
			// participate in commonRunPropertiesOf (per the
			// fieldContentRun branch in the seed/intersection loop
			// below) but they no longer veto the whole paragraph just
			// because they happen to carry an excluded property.
			//
			// Fixture 1341-textbox-with-a-hyperlink.docx: textbox P1
			// is a HYPERLINK field whose display-text run carries
			// rStyle="Hyperlink"; upstream synthesises a Normal1 style
			// from the outer field run's {b,bCs,sz,szCs} common rPr.
			// Without this guard the rStyle on the inner display-text
			// run triggers the bypass and native loses the pStyle.
			continue
		}
		if skipBreakOnlyEarly && e.breakOnlyRun {
			// Break-only runs (`<w:r>...<w:br/></w:r>` with no text)
			// are Markup body chunks of an outer Run upstream — they
			// don't participate in commonRunPropertiesOf when the
			// paragraph contains other text-bearing runs. See
			// breakOnlyRun docstring + commonProps rationale.
			continue
		}
		if e.excluded {
			return src // bypass per StyleOptimisation.innerChunksContainExclusions
		}
		if !e.hasRPr || len(e.props) == 0 {
			// Relax the early bail when docDefaults carries an rFonts:
			// commonRFonts (style_optimization.go:2087) virtually
			// overlays currentDocDefaultsRFonts onto runs that lack a
			// direct rFonts, mirroring upstream Okapi's RunMerger
			// combined-rFonts view (RunMerger.java:389-396
			// clarifyFontsRunProperties + WordStyleDefinitions.
			// combinedRunProperties lines 302-315). Per ECMA-376-1
			// §17.7.5.5 (CT_DocDefaults) the docDefaults rPr applies
			// implicitly to every run that doesn't override the
			// property — so an empty-rPr run still has an effective
			// rFonts via the docDefaults overlay, and commonProps can
			// still surface an agreed rFonts value across the runs.
			//
			// StartsWithLineSeparator.docx paragraph 1: R1 carries a
			// direct rFonts (MS Gothic) and R2-R4 carry no rPr at all.
			// With docDefaults `<w:rFonts asciiTheme=minorHAnsi
			// hAnsiTheme=minorHAnsi …/>`, the overlay gives R2-R4 a
			// virtual rFonts matching docDefaults so the
			// per-content-category intersection can surface
			// asciiTheme=minorHAnsi (R1's `MS Gothic` direct ascii is
			// detected via the docDefaults asciiTheme on R2-R4 — and
			// the merged rFonts gets lifted into a synth pStyle).
			//
			// Non-rFonts props of empty-rPr runs still drop the
			// corresponding common-prop entries via the loop in
			// commonProps (entry's prop set is empty so the lookup
			// fails) — only rFonts benefits from the overlay. The
			// relaxation is therefore safe for paragraphs where every
			// run has empty rPr (commonProps returns nil → outer
			// `len(common) == 0` bail still fires).
			if len(currentDocDefaultsRFonts) > 0 {
				continue
			}
			return src
		}
	}
	// Seed the intersection from the first non-field-content run.
	// When the paragraph starts with a complex field (e.g. the first
	// sibling is a `<w:r><w:fldChar w:fldCharType="begin"/></w:r>`
	// which IS the outer begin run and therefore NOT field-content),
	// entries[0] is already the upstream-visible outer run. But for
	// safety pick the first entry whose fieldContentRun==false — it
	// matches the seed upstream's commonRunPropertiesOf would use.
	// Pre-compute the same break-only skip gate used by commonProps.
	skipBreakOnlySeed := false
	if hasBreakOnly(entries) {
		for _, e := range entries {
			if !e.fieldContentRun && !e.breakOnlyRun {
				skipBreakOnlySeed = true
				break
			}
		}
	}
	seedIdx := -1
	for i := range entries {
		if entries[i].fieldContentRun {
			continue
		}
		if skipBreakOnlySeed && entries[i].breakOnlyRun {
			continue
		}
		seedIdx = i
		break
	}
	if seedIdx < 0 {
		// Every run is field-content — paragraph is degenerate
		// (e.g. malformed XML with no begin run). Bail.
		return src
	}
	common := commonProps(entries[seedIdx].props, entries)
	if len(common) == 0 {
		return src
	}
	// Normalise the b/i toggle entries in the SYNTHESISED style's rPr
	// (commonForStyle) to the bidi form that matches the common run's
	// script direction:
	//
	//   - LTR runs (no <w:rtl/> in common): keep b/i, drop bCs/iCs.
	//     bCs/iCs are the complex-script bidi-mirrors of b/i
	//     (ECMA-376-1 §17.3.2.4); upstream Okapi reconstructs the
	//     mirror at run-emit time from the b/i toggle and never
	//     surfaces bCs/iCs alone into a synthesised paragraph style.
	//   - RTL runs (<w:rtl/> in common): rename b→bCs, i→iCs in the
	//     synthesised rPr. Per ECMA-376-1 §17.3.2.4, bCs/iCs are the
	//     bold/italic toggles that APPLY to complex-script (RTL) text
	//     in a run; upstream Okapi's WSO promotes the bidi-script
	//     toggle that matches the run's directionality. The native
	//     writer's blockPerRunRPrFragments path always strips bCs/iCs
	//     from per-run sidecars BEFORE WSO sees the XML (writer.go
	//     :1028-1041), so the surviving b/i entries are the stand-in
	//     for the original bCs/iCs on RTL runs. Observed in
	//     947-cs.docx / 947-non-cs-and-cs.docx reference output:
	//     synthesised style rPr is <bCs/><iCs/><rtl/><sz/>, not
	//     <b/><i/><rtl/><sz/>.
	//
	// b/i remain in `common`, so the run-strip pass below (which
	// builds its commonNames map from `common`, not commonForStyle)
	// continues to lift them off each run — matching upstream's "no
	// rPr at all on the run" emit shape for 952-3.docx /
	// TestDako2.docx / 947-cs.docx.
	//
	// This is the WSO-layer counterpart of writer.go's
	// stripToggleMirrorChildren (lines 1044-1058) which performs the
	// equivalent strip on the per-source-run rPr sidecar before write.
	rtlCommon := commonContainsRTL(common)

	// Build the synthesised style id. Mirrors WordStyleDefinitions.Ids
	// .basedOn (lines 453-460): the paragraph's pStyle is used as
	// parent ONLY if it is defined in styles.xml; otherwise the call
	// falls through to defaultBased() which uses the document default
	// paragraph style (universally "Normal" in practice — see
	// StyleDefinitions.defaultStylesByStyleTypes population at
	// WordStyleDefinitions.readWith). This guard matters for fixtures
	// like 992.docx whose footer paragraphs carry a pStyle ("Corpodeltesto",
	// "Pidipagina") that isn't actually defined — Okapi resolves them
	// to Normal-based, native must do the same to keep styleId
	// sequences aligned.
	//
	// existingStyleIDs accumulates synthesised ids too (so that the
	// collision-avoidance loop below sees them). To distinguish source
	// from synthesised, we exclude entries whose styleId starts with
	// the synthesised-id prefix — no source-defined pStyle ever takes
	// that shape in practice.
	parentID := pStyleID
	if parentID == "" || !existingStyleIDs[parentID] || strings.HasPrefix(parentID, styleHashRoot+"-") {
		if defaultParagraphStyleID != "" {
			parentID = defaultParagraphStyleID
		} else {
			parentID = "Normal"
		}
	}
	// Does the synth style's parent chain inherit `<w:rtl/>`? Used by
	// stripToggleMirrorsFromCommon's `case "rtl":` to PRESERVE an
	// explicit-off `<w:rtl w:val="0"/>` lift in the synth style's rPr
	// (899.docx Normal-with-rtl) vs DROP it as redundant (830-2.docx
	// Normal-without-rtl). currentRTLChainStyles is set by writer.go
	// from extractRTLChainStyleIDs(stylesXML).
	parentInheritsRTL := currentRTLChainStyles != nil && currentRTLChainStyles[parentID]
	commonForStyle := stripToggleMirrorsFromCommon(common, rtlCommon, parentInheritsRTL)
	if len(commonForStyle) == 0 {
		// Only the dropped toggle members were common — there is
		// nothing meaningful to lift into a parent style. Upstream
		// Okapi would have skipped these from the common set in the
		// first place (the toggle mirrors don't surface as standalone
		// synthesisable props), so bail to match upstream's "no style
		// synthesised" outcome.
		return src
	}
	commonRPrXML := buildRPrXML(commonForStyle)
	var matchedID string
	if !hasStylesPart {
		// No word/styles.xml in the source → upstream uses
		// StyleDefinitions.Empty whose place() is a no-op and whose
		// placedId() returns null (StyleDefinitions.java:53-59). The
		// run-strip + pPr-pStyle insertion still happens, but the
		// pStyle's w:val is empty and no <w:style> is accumulated
		// (there is no styles.xml to inject it into). Per ECMA-376-1
		// §17.7.4: no style hierarchy exists when the styles part is
		// absent.
		matchedID = ""
	} else {
		// Writer-merge recovery (rFonts hint variant): when the paragraph
		// is the merged-collapse shape of two source runs (break-only +
		// text, fused into one run by the writer), the surviving merged
		// run's rPr lacks attributes that the original text run carried.
		// In particular, when the text run authored an rFonts.hint that
		// the break-only run lacked, the merge drops the hint. Upstream
		// Okapi's RunMerger sees the source runs as separate Run events
		// (canRunPropertiesBeMerged refuses to merge runs whose rFonts
		// disagree on a non-category attribute — RunMerger.java:156-229),
		// so its WSO computes the common rPr from the text run alone
		// (the break-only run is a Markup body chunk and contributes no
		// rPr) and matches/reuses an existing synth style that carries
		// the hint.
		//
		// Native's WSO sees the merged-shape: a single run with the
		// hint-less rPr. The merged paragraph also retains the paragraph
		// mark's pPr.rPr declarations, which in this case carry the same
		// rFonts including the hint=eastAsia attribute. We use that
		// paragraph-mark rFonts to surface the effective hint and prefer
		// an existing synth style that matches the hint-augmented rFonts
		// over an exact match against the hint-less common rPr.
		//
		// Per ECMA-376-1 §17.3.1.29 (CT_PPr.rPr) the paragraph-mark rPr
		// participates in the run-level effective-rFonts merge as the
		// fallback context for runs that don't override (RunFonts.merge
		// at RunFonts.java:232-248). The augmented match runs BEFORE the
		// plain findMatchingStyle so the hint-bearing synth style (a14
		// in Hangs.docx) wins over the hint-less twin (a19) that may
		// have been synthesised earlier for a paragraph whose pPr.rPr
		// genuinely lacked the hint.
		if paragraphHasMergedBreakTextRun(runs, entries, src) && hasPPr {
			pPrHintAttrs := extractPPrRPrRFontsAttrs(src[pPrStart:pPrEnd])
			if pPrHintAttrs != nil {
				if id := findSynthMatchingWithPPrHint(parentID, commonForStyle, pPrHintAttrs, synthesised, *orderedIDs); id != "" {
					matchedID = id
					common = parseRunPropElements([]byte("<w:rPr>" + synthesised[id].rPrXML + "</w:rPr>"))
				}
			}
		}
		if matchedID == "" {
			matchedID = findMatchingStyle(parentID, commonRPrXML, commonForStyle, synthesised, *orderedIDs)
		}
		// Writer-merge recovery: a single-run paragraph whose lone run
		// carries BOTH <w:br> AND <w:t> is the merged-collapse shape of
		// what upstream Okapi sees as two distinct <w:r> source events
		// (a break-only run + an adjacent text run). The merge can drop
		// the text run's richer rPr (e.g. `{rFonts, szCs}`) in favour
		// of the break-only run's narrower rPr (e.g. `{szCs}`), so WSO
		// computes an under-specified common set and would otherwise
		// synthesise a spurious fresh style instead of reusing the
		// richer style that earlier same-document paragraphs already
		// placed. When an existing synth style with the matching
		// parentID has rPr props that are a strict SUPERSET of the
		// computed common, reuse it and broaden the per-run strip to
		// cover the style's full prop set. See findSupersetSynthStyle
		// doc + EndGroup.docx fixture rationale.
		if matchedID == "" && paragraphHasMergedBreakTextRun(runs, entries, src) {
			if id := findSupersetSynthStyle(parentID, commonForStyle, synthesised, *orderedIDs); id != "" {
				matchedID = id
				// Broaden `common` to the matched style's full rPr so
				// the run-strip below removes every property the style
				// supplies, leaving the run rPr-less to match upstream's
				// emit shape. (commonForStyle / commonRPrXML are not read
				// after this point — they only drove the style synthesis
				// fork which we just bypassed.)
				common = parseRunPropElements([]byte("<w:rPr>" + synthesised[id].rPrXML + "</w:rPr>"))
			}
		}
		if matchedID == "" {
			// Generate a fresh id "NF974E24F-<parentID><N>" using the
			// SHARED IdGenerator counter — see the optimizeWMLPart doc
			// comment for the upstream contract. The do/while in
			// WordStyleDefinitions.Ids.parentBasedGenerated keeps ticking
			// the shared counter until an id not already in
			// stylesByStyleIds is produced.
			for {
				*idCounter++
				candidate := fmt.Sprintf("%s-%s%d", styleHashRoot, parentID, *idCounter)
				if !existingStyleIDs[candidate] {
					matchedID = candidate
					break
				}
			}
			synthesised[matchedID] = synthesisedStyle{
				id:       matchedID,
				parentID: parentID,
				rPrXML:   commonRPrXML,
			}
			*orderedIDs = append(*orderedIDs, matchedID)
			existingStyleIDs[matchedID] = true
		}
	}

	// Rewrite paragraph: insert pStyle into pPr (or create pPr) and
	// strip common props from each run's rPr.
	var out bytes.Buffer
	out.Grow(len(src) + 256)
	cursor := 0

	// Insert/update pPr.
	if hasPPr {
		out.Write(src[:pPrStart])
		newPPr := insertPStyle(src[pPrStart:pPrEnd], matchedID)
		out.Write(newPPr)
		cursor = pPrEnd
	} else {
		// Find the start-tag end of <w:p ...> — pPr goes immediately after.
		startTagEnd := bytes.IndexByte(src, '>')
		if startTagEnd < 0 {
			return src
		}
		out.Write(src[:startTagEnd+1])
		out.WriteString(`<w:pPr><w:pStyle w:val="`)
		out.WriteString(matchedID)
		out.WriteString(`"/></w:pPr>`)
		cursor = startTagEnd + 1
	}

	// Now iterate runs, stripping common props from each rPr.
	commonNames := make(map[string]bool, len(common))
	for _, p := range common {
		// When matchedID is empty (no word/styles.xml present →
		// upstream's StyleDefinitions.Empty.placedId() returns null),
		// the synthesised pStyle is unresolvable, so nothing can be
		// inherited back at re-read time. Stripping <w:vanish/> from
		// the run's rPr would silently expose hidden text on the next
		// pass (formatted.docx — a docx with no styles.xml whose only
		// translatable content is a `<w:vanish/>` run that must
		// remain hidden after round-trip). Keep vanish on the run in
		// that case; the synth pStyle's val is empty so the inherited-
		// vanish path in allHidden cannot recover it.
		if matchedID == "" && p.name == "vanish" {
			continue
		}
		commonNames[p.name] = true
	}
	for _, e := range entries {
		if e.runStart < cursor {
			// Should not happen, but guard.
			continue
		}
		out.Write(src[cursor:e.runStart])
		runBuf := src[e.runStart:e.runEnd]
		// Skip the per-run rPr strip on field-content runs (instrText,
		// fldChar=separate, fldChar=end) so their rPr survives
		// verbatim — mirrors upstream Okapi's "only the outer Run
		// is refined" model. See runEntry.fieldContentRun docstring
		// for the upstream contract reference. Break-only runs DO
		// participate in the strip pass — bridge's WSO strips the
		// lifted-common property names from break-only `<w:r>`
		// envelopes too (br.docx fixture: source `<w:r><w:rPr>
		// {rFonts,szCs}</w:rPr><w:br/></w:r>` runs whose common rPr
		// was lifted to a synth pStyle emit as `<w:r><w:br/></w:r>`).
		if e.hasRPr && !e.fieldContentRun {
			stripNames := commonNames
			if e.csOnlyText {
				// Symmetric upstream strip: drop b/i from the emitted
				// rPr in addition to any commonNames the WSO lift
				// computes. Without this the writer's runProps
				// reconstruction (writer.go addWMLProp emits `<w:b/>` /
				// `<w:i/>` from PcOpen TypeBold/TypeItalic toggles) would
				// echo b/i back onto a CS-only run that upstream Okapi
				// never emits — see the "symmetric counterpart" comment
				// at the props-parse site above for the full rationale.
				stripNames = make(map[string]bool, len(commonNames)+2)
				for k, v := range commonNames {
					stripNames[k] = v
				}
				stripNames["b"] = true
				stripNames["i"] = true
			}
			// Symmetric drawing-only strip: when the run is drawing-
			// only, b/i (and the bCs/iCs mirror pair) were removed
			// from e.props above so they don't get LIFTED into the
			// synth pStyle. They must ALSO be stripped from the
			// EMITTED per-run rPr, otherwise the surviving toggle
			// re-renders an effective bold/italic toggle that the
			// drawing-only run shouldn't carry on the wire. Mirrors
			// upstream Okapi's RunBuilder MarkupComponent path:
			// BlockTextUnitWriter walks the MarkupComponent body
			// chunks directly without the addToggleToRPr re-emission
			// pass that text runs go through, so b/i never appear on
			// the materialised drawing-only `<w:r>`. Per ECMA-376-1
			// §17.3.2.1 (CT_R) toggle properties apply to text
			// children; for a run with no text those toggles are
			// no-ops at render time.
			//
			// Fixture AlternateContentTest.docx: a textbox-bearing
			// paragraph holds a single `<w:r><w:rPr><w:i/><w:iCs/>
			// <w:noProof/><w:sz val="18"/><w:szCs val="18"/><w:lang
			// .../></w:rPr><mc:AlternateContent>...</w:r>`. Pre-fix
			// native re-emitted `<w:i/>` on the per-run rPr; post-fix
			// the strip drops it, matching upstream's emitted
			// `<w:r><w:rPr><w:noProof/><w:sz/><w:szCs/><w:lang/>
			// </w:rPr><mc:AlternateContent>...</w:r>` (the lang/
			// noProof survive on the per-run rPr because they are
			// non-toggle properties — the strip is targeted at the
			// b/i pair only).
			if runIsDrawingOnly(src[e.runStart:e.runEnd]) {
				if len(stripNames) == 0 {
					stripNames = make(map[string]bool, 4)
				} else if !e.csOnlyText {
					orig := stripNames
					stripNames = make(map[string]bool, len(orig)+4)
					for k, v := range orig {
						stripNames[k] = v
					}
				}
				stripNames["b"] = true
				stripNames["bCs"] = true
				stripNames["i"] = true
				stripNames["iCs"] = true
			}
			if len(stripNames) > 0 {
				runBuf = stripPropsFromRun(runBuf, stripNames)
			}
		}
		out.Write(runBuf)
		cursor = e.runEnd
	}
	out.Write(src[cursor:])
	return out.Bytes()
}

// findFirstChild returns the byte range of the first <w:NAME>...</w:NAME>
// (or self-closing <w:NAME/>) element appearing as a DIRECT child of the
// parent element represented by src. start/end are relative to src.
//
// Per ECMA-376-1 §17.3.1.10 (CT_P) <w:pPr> MUST be the first child of
// <w:p> if present, and per §17.3.2.1 (CT_R) <w:rPr> MUST be the first
// child of <w:r> if present. So a "first direct child" search is
// equivalent to "is the next non-whitespace content right after the
// parent's start tag a <w:NAME ...>?".
//
// The previous implementation did `bytes.Index(src, open)` which would
// happily match a <w:NAME> nested arbitrarily deep — e.g. a
// <w:r><mc:AlternateContent>…<w:p><w:rPr>…</w:rPr>…</w:p>…</mc:Alternate
// Content></w:r> drawing-only run would falsely report having an rPr
// because the inner txbxContent paragraph carries one. That mis-detected
// rPr would then feed WSO's optimizeParagraph as if it belonged to the
// outer drawing-only run, polluting the common-rPr lift. Fixture
// highlights_block.docx is the canonical case: a paragraph whose first
// run carries `<w:rPr><w:color/></w:rPr><w:t>Run 1.</w:t>` and whose
// second run is a drawing-only `<w:r><mc:AlternateContent>…</w:r>` with
// NO direct rPr was being mis-classified as a 2-run paragraph with
// matching rPrs (the inner txbxContent rPr leaking to the outer drawing
// run), causing native to synthesise a spurious BodyText3 pStyle that
// the bridge correctly leaves un-synthesised because the drawing run
// has no direct rPr to lift.
func findFirstChild(src []byte, name string) (int, int, bool) {
	open := []byte("<w:" + name)
	close := []byte("</w:" + name + ">")
	// Skip past the parent's start tag. The parent is the outermost
	// element in src (e.g. <w:p ...> or <w:r ...>); its start tag ends
	// at the first '>'. After that we expect either whitespace or the
	// first child element to start.
	parentTagEnd := bytes.IndexByte(src, '>')
	if parentTagEnd < 0 {
		return 0, 0, false
	}
	// Parent could be self-closing (<w:r/>) — no children possible.
	if parentTagEnd > 0 && src[parentTagEnd-1] == '/' {
		return 0, 0, false
	}
	// Walk past whitespace immediately after the parent's start tag to
	// the first non-whitespace byte. That byte must be the start of an
	// element — `<` — or there is no first child to compare against.
	cursor := parentTagEnd + 1
	for cursor < len(src) {
		b := src[cursor]
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			cursor++
			continue
		}
		break
	}
	if cursor >= len(src) || src[cursor] != '<' {
		return 0, 0, false
	}
	// The first child must be `<w:NAME ...>` for the lookup to succeed.
	if !bytes.HasPrefix(src[cursor:], open) {
		return 0, 0, false
	}
	i := cursor
	// Confirm name boundary.
	j := i + len(open)
	if j >= len(src) {
		return 0, 0, false
	}
	b := src[j]
	if b != '>' && b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '/' {
		return 0, 0, false
	}
	k := bytes.IndexByte(src[j:], '>')
	if k < 0 {
		return 0, 0, false
	}
	startTagEnd := j + k
	// Self-closing form.
	if startTagEnd > 0 && src[startTagEnd-1] == '/' {
		return i, startTagEnd + 1, true
	}
	// Open form — find matching close (no nesting in pPr/rPr in WML).
	ci := bytes.Index(src[startTagEnd+1:], close)
	if ci < 0 {
		return 0, 0, false
	}
	return i, startTagEnd + 1 + ci + len(close), true
}

// extractAttrVal scans src for an element <w:elem ... attr="VAL"...>
// and returns VAL. Returns "" if not found.
func extractAttrVal(src []byte, elemName, attr string) string {
	open := []byte("<w:" + elemName)
	i := bytes.Index(src, open)
	if i < 0 {
		return ""
	}
	j := i + len(open)
	if j >= len(src) {
		return ""
	}
	b := src[j]
	if b != '>' && b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '/' {
		return ""
	}
	k := bytes.IndexByte(src[j:], '>')
	if k < 0 {
		return ""
	}
	startTag := string(src[j : j+k])
	// Locate attr=
	ai := strings.Index(startTag, attr+"=")
	if ai < 0 {
		return ""
	}
	rest := startTag[ai+len(attr)+1:]
	if len(rest) == 0 {
		return ""
	}
	q := rest[0]
	if q != '"' && q != '\'' {
		return ""
	}
	end := strings.IndexByte(rest[1:], q)
	if end < 0 {
		return ""
	}
	return rest[1 : 1+end]
}

// rawRun is a byte range covering one <w:r>...</w:r> element.
type rawRun struct {
	start, end int
}

// findRuns returns every paragraph-level <w:r>...</w:r> element inside
// src. Per ECMA-376-1 §17.3.2.1 (CT_R) a <w:r> cannot directly
// contain another <w:r>, but it CAN contain a <w:drawing> /
// <w:pict> / <w:object> / <mc:AlternateContent> whose subtree
// carries a <wne:txbxContent> (or VML/AlternateContent equivalent)
// holding nested <w:p>...<w:r>...</w:p> blocks. Those nested runs
// belong to a SUB-document (a separate styled-text part in upstream
// Okapi) and must NOT be surfaced as siblings of the outer
// drawing-bearing run for WSO purposes — promoting them would
// mis-attribute their per-run rPr to the OUTER paragraph and
// cause WSO to synthesise a spurious paragraph style on the
// drawing-only paragraph (859.docx — drawing paragraph picked up
// `<w:lang w:val="en-US"/>` from the inner textbox run rPr and got
// `<w:pStyle w:val="NF974E24F-Normal1"/>` injected; the okapi
// reference promotes the inner textbox-paragraph rPr to its own
// synthesised style and leaves the outer drawing-only paragraph
// alone).
//
// To match Okapi's per-block scope (upstream walks one paragraph at
// a time and treats the drawing subtree as opaque markup —
// MarkupComponent payload, see RunBuilder.addToMarkup
// (RunBuilder.java:73-188) and RunContainer's opaque-chunk model),
// the byte scanner skips past the entire <w:drawing>...</w:drawing>
// (and <w:pict>/<w:object>/<mc:AlternateContent>) extent before
// resuming the <w:r> hunt. The outer drawing run is still emitted
// (it's a paragraph-level <w:r>); only the inner runs are filtered.
//
// Sequential top-level scan: paragraphs returned by findParagraphs
// are the outermost <w:p>, but within ONE such paragraph runs may
// appear at any depth (inside hyperlink, sdt content, ins/del
// wrappers, smartTag). All surfaced as separate rawRun entries;
// the opaque-subtree skip targets only drawing/pict/object/AC,
// which carry their own paragraphs.
func findRuns(src []byte) []rawRun {
	var out []rawRun
	open := []byte("<w:r")
	close := []byte("</w:r>")
	// opaqueRunChildren names elements that may appear inside <w:r>
	// and contain nested <w:r> in a sub-document scope. Mirrors the
	// upstream RunContainer/MarkupComponent opaque-payload set:
	// drawings + objects + pictures + AlternateContent (the latter
	// because mc:Choice often wraps a drawing).
	opaqueRunChildren := []string{"<w:drawing", "<w:pict", "<w:object", "<mc:AlternateContent"}
	i := 0
	for i < len(src) {
		idx := bytes.Index(src[i:], open)
		if idx < 0 {
			return out
		}
		start := i + idx
		j := start + len(open)
		if j >= len(src) {
			return out
		}
		b := src[j]
		if b != '>' && b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '/' {
			// Not a w:r — could be w:rPr, w:rFonts, w:rStyle, etc.
			i = j
			continue
		}
		k := bytes.IndexByte(src[j:], '>')
		if k < 0 {
			return out
		}
		startTagEnd := j + k
		// Self-closing <w:r/> — empty run, skip.
		if startTagEnd > 0 && src[startTagEnd-1] == '/' {
			i = startTagEnd + 1
			continue
		}
		// Walk forward looking for either an opaque-subtree start
		// (<w:drawing>/<w:pict>/<w:object>/<mc:AlternateContent>)
		// or this run's matching </w:r>. When we hit an opaque
		// subtree, jump past its balanced end tag so any nested
		// <w:r> inside textbox/object content is filtered out.
		scan := startTagEnd + 1
		end := -1
		for scan < len(src) {
			ci := bytes.Index(src[scan:], close)
			if ci < 0 {
				return out
			}
			closeAbs := scan + ci
			// Find the earliest opaque-subtree open that precedes
			// the candidate </w:r> close. If found, skip over it.
			earliestOpaqueAbs := -1
			var matchedTag string
			for _, tag := range opaqueRunChildren {
				ti := bytes.Index(src[scan:closeAbs], []byte(tag))
				if ti < 0 {
					continue
				}
				// Element-name boundary: next char must be ` `, `>`, `/`.
				bj := scan + ti + len(tag)
				if bj >= len(src) {
					continue
				}
				bb := src[bj]
				if bb != '>' && bb != ' ' && bb != '\t' && bb != '\n' && bb != '\r' && bb != '/' {
					continue
				}
				abs := scan + ti
				if earliestOpaqueAbs < 0 || abs < earliestOpaqueAbs {
					earliestOpaqueAbs = abs
					matchedTag = tag
				}
			}
			if earliestOpaqueAbs >= 0 {
				// Find balanced close for this opaque element.
				closeName := []byte("</" + matchedTag[1:] + ">")
				// Self-closing form: <w:drawing/> — no inner content.
				openTagEnd := bytes.IndexByte(src[earliestOpaqueAbs:], '>')
				if openTagEnd < 0 {
					return out
				}
				openTagAbsEnd := earliestOpaqueAbs + openTagEnd
				if openTagAbsEnd > 0 && src[openTagAbsEnd-1] == '/' {
					scan = openTagAbsEnd + 1
					continue
				}
				// Find matching close tag with depth counter for
				// nested same-name elements (rare but possible for
				// AlternateContent).
				depth := 1
				inner := openTagAbsEnd + 1
				for depth > 0 && inner < len(src) {
					nextOpen := bytes.Index(src[inner:], []byte(matchedTag))
					nextClose := bytes.Index(src[inner:], closeName)
					if nextClose < 0 {
						return out
					}
					if nextOpen >= 0 && nextOpen < nextClose {
						// Confirm element-name boundary on the open.
						bj := inner + nextOpen + len(matchedTag)
						if bj < len(src) {
							bb := src[bj]
							if bb == '>' || bb == ' ' || bb == '\t' || bb == '\n' || bb == '\r' || bb == '/' {
								// Self-closing variant doesn't
								// increment depth.
								kk := bytes.IndexByte(src[bj:], '>')
								if kk >= 0 {
									se := bj + kk
									if !(se > 0 && src[se-1] == '/') {
										depth++
									}
									inner = se + 1
									continue
								}
							}
						}
						inner = bj
						continue
					}
					depth--
					inner = inner + nextClose + len(closeName)
				}
				scan = inner
				continue
			}
			// No opaque subtree before </w:r> — this is our close.
			end = closeAbs + len(close)
			break
		}
		if end < 0 {
			return out
		}
		out = append(out, rawRun{start: start, end: end})
		i = end
	}
	return out
}

// parseRunPropElements parses the children of a <w:rPr>...</w:rPr>
// block (src includes the enclosing tags). It returns a slice of
// runProp records preserving source order. Each runProp captures both
// the local element name and the literal serialization (so order-
// sensitive attribute equality works).
func parseRunPropElements(src []byte) []runProp {
	// Strip the wrapping <w:rPr>...</w:rPr> or <w:rPr/>.
	open := []byte("<w:rPr")
	close := []byte("</w:rPr>")
	i := bytes.Index(src, open)
	if i < 0 {
		return nil
	}
	startTagEnd := bytes.IndexByte(src[i:], '>')
	if startTagEnd < 0 {
		return nil
	}
	// Self-closing rPr — no children.
	if startTagEnd > 0 && src[i+startTagEnd-1] == '/' {
		return nil
	}
	body := src[i+startTagEnd+1:]
	// Trim trailing </w:rPr>.
	if ci := bytes.Index(body, close); ci >= 0 {
		body = body[:ci]
	}
	// Now scan body for child elements (no text content expected;
	// every child is a property element). Each element is either
	// self-closing or open/close-balanced (no nesting in rPr).
	var out []runProp
	for j := 0; j < len(body); {
		if body[j] != '<' {
			j++
			continue
		}
		// Must be of form <w:NAME ...
		if !bytes.HasPrefix(body[j:], []byte("<w:")) {
			// Non-w: child — skip the tag (could be e.g. <w14:foo/>
			// extension).
			tagEnd := bytes.IndexByte(body[j:], '>')
			if tagEnd < 0 {
				break
			}
			// Self-closing? Skip it. Otherwise skip up to matching close —
			// rare; conservative implementation just records the tag verbatim.
			selfClose := tagEnd > 0 && body[j+tagEnd-1] == '/'
			if selfClose {
				out = append(out, runProp{name: extractLocal(body[j : j+tagEnd+1]), xml: string(body[j : j+tagEnd+1])})
				j = j + tagEnd + 1
				continue
			}
			// Find balanced close.
			localName := extractLocal(body[j : j+tagEnd+1])
			if localName == "" {
				j = j + tagEnd + 1
				continue
			}
			// Look for the closest matching </prefix:localName>
			closeNeedle := []byte("</" + extractPrefixedName(body[j:j+tagEnd+1]) + ">")
			endIdx := bytes.Index(body[j+tagEnd+1:], closeNeedle)
			if endIdx < 0 {
				j = j + tagEnd + 1
				continue
			}
			elemEnd := j + tagEnd + 1 + endIdx + len(closeNeedle)
			out = append(out, runProp{name: localName, xml: normalizeEmptyElement(string(body[j:elemEnd]))})
			j = elemEnd
			continue
		}
		// <w:NAME ...
		tagEnd := bytes.IndexByte(body[j:], '>')
		if tagEnd < 0 {
			break
		}
		nameEnd := bytes.IndexAny(body[j+3:], " \t\n\r/>")
		if nameEnd < 0 {
			break
		}
		name := string(body[j+3 : j+3+nameEnd])
		// Self-closing?
		if tagEnd > 0 && body[j+tagEnd-1] == '/' {
			out = append(out, runProp{name: name, xml: string(body[j : j+tagEnd+1])})
			j = j + tagEnd + 1
			continue
		}
		// Open form — find matching </w:NAME>.
		closeNeedle := []byte("</w:" + name + ">")
		endIdx := bytes.Index(body[j+tagEnd+1:], closeNeedle)
		if endIdx < 0 {
			j = j + tagEnd + 1
			continue
		}
		elemEnd := j + tagEnd + 1 + endIdx + len(closeNeedle)
		out = append(out, runProp{name: name, xml: normalizeEmptyElement(string(body[j:elemEnd]))})
		j = elemEnd
	}
	return out
}

// normalizeEmptyElement collapses an empty open/close element form
// (`<w:X></w:X>` or `<w:X attr="…"></w:X>`) to its self-closing form
// (`<w:X/>` / `<w:X attr="…"/>`). encoding/xml's Decoder/Encoder cycle
// re-emits captureRawElement payloads in open/close form even when the
// source was self-closing — see the same #592 note in insertPStyle.
// Without normalisation, two semantically identical run-property
// elements compare unequal here (one self-closing, one open/close) and
// commonProps spuriously returns empty, which is what causes WSO to
// silently bypass headers/footers in fixtures like 956.docx and 992.docx
// (the runs come back from encoding/xml in mixed forms).
//
// The normalisation is conservative: only elements with EMPTY bodies
// (no child elements, no character data, only optional whitespace)
// collapse. Anything with content is left untouched.
func normalizeEmptyElement(xml string) string {
	if len(xml) < 4 || xml[0] != '<' || xml[len(xml)-1] != '>' {
		return xml
	}
	if xml[len(xml)-2] == '/' {
		return xml // already self-closing
	}
	startTagEnd := strings.IndexByte(xml, '>')
	if startTagEnd < 0 {
		return xml
	}
	body := xml[startTagEnd+1:]
	closeIdx := strings.LastIndex(body, "</")
	if closeIdx < 0 {
		return xml
	}
	inner := body[:closeIdx]
	for i := range len(inner) {
		c := inner[i]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return xml // non-empty body
		}
	}
	// Re-emit start tag as self-closing.
	return xml[:startTagEnd] + "/>"
}

// extractLocal returns the local element name from a tag like
// "<w:rFonts ..." or "<w14:foo ..." → "rFonts", "foo".
func extractLocal(tag []byte) string {
	if len(tag) < 2 || tag[0] != '<' {
		return ""
	}
	s := tag[1:]
	if i := bytes.IndexByte(s, ':'); i >= 0 {
		s = s[i+1:]
	}
	end := bytes.IndexAny(s, " \t\n\r/>")
	if end < 0 {
		return string(s)
	}
	return string(s[:end])
}

// extractPrefixedName returns "prefix:local" from a tag like
// "<w14:foo ..." → "w14:foo".
func extractPrefixedName(tag []byte) string {
	if len(tag) < 2 || tag[0] != '<' {
		return ""
	}
	s := tag[1:]
	end := bytes.IndexAny(s, " \t\n\r/>")
	if end < 0 {
		return string(s)
	}
	return string(s[:end])
}

// commonProps returns the run-property elements present and equal
// (by exact xml serialization) in EVERY run-entry. Order is preserved
// from the first run.
//
// <w:rFonts> is special-cased: the common rFonts is the per-attribute
// intersection of every run's rFonts (an attribute is kept iff every
// run that has rFonts agrees on the value AND every run has rFonts).
// This mirrors upstream Okapi's behaviour: RunMerger fuses adjacent
// runs whose rFonts are mergeable (RunFonts.canBeMerged + RunFonts.merge)
// BEFORE StyleOptimisation runs, so by the time WSO sees the runs, all
// rFonts are already the merged consensus. We don't have RunMerger in
// the post-write pass, so we compute the consensus here. The intersection
// rule is the safe approximation of Okapi's merge logic for plain-text
// runs (where the COMPLEX_SCRIPT/EAST_ASIAN content categories aren't
// "detected" and thus don't carry extra attributes through the merge).
//
// Per ECMA-376-1 §17.3.2.26 (rFonts), the ascii, hAnsi, cs, eastAsia
// (and corresponding theme variants) attributes are independent: an
// rFonts element may carry any subset. The intersection of attribute/
// value pairs is therefore a valid rFonts and a faithful per-run common
// font specification.
//
// References:
//   - okapi/filters/openxml/RunFonts.java lines 190-247 (canBeMerged,
//     mergeContentCategories) — upstream merge contract.
//   - okapi/filters/openxml/StyleOptimisation.java lines 204-238
//     (commonRunPropertiesOf) — exact-equality List.retainAll on
//     post-merge runs.
//   - ECMA-376-1 4th ed §17.3.2.26 (rFonts).
func commonProps(seed []runProp, entries []runEntry) []runProp {
	if len(entries) == 0 {
		return nil
	}
	// Field-content runs (instrText / fldChar=separate / fldChar=end
	// AND any display-text runs sitting between fldChar=separate and
	// fldChar=end) are body chunks of the outer Run upstream and are
	// invisible to commonRunPropertiesOf (StyleOptimisation.java:
	// 204-237). Break-only runs (standalone `<w:r><w:br/></w:r>`
	// envelopes with no text) are likewise Markup body chunks
	// upstream and contribute no rPr to the common-prop computation
	// — UNLESS they're the only kind of run in the paragraph. When
	// the paragraph contains other (text-bearing) runs, including
	// break-only chunks would skew the lift:
	//   - When the chunk's rPr DIVERGES from the outer runs, the
	//     intersection collapses and the synth lift gets cancelled
	//     for an outer-run property that upstream would have lifted
	//     (EndGroup.docx: a `<w:r><w:rPr><w:szCs/></w:rPr><w:br/>
	//     </w:r>` whose rPr lacks rFonts narrows the common away
	//     from rFonts hint=eastAsia, costing the rFonts lift).
	// When the paragraph holds ONLY break-only runs (e.g.
	// PageBreak.docx P2: `<w:p><w:r><w:rPr><w:vanish/></w:rPr>
	// <w:br type="page"/></w:r></w:p>`), upstream still computes a
	// common-rPr from those runs and lifts a synthesised pStyle —
	// the BlockTextUnitWriter sees a single Markup chunk Run whose
	// RunProperties are the br-run's rPr. Mirror that by NOT
	// excluding break-only runs when they are the sole non-field-
	// content runs in the paragraph.
	//
	// For field-content runs the same kind of override would apply,
	// but in practice a paragraph with ONLY field-content runs is
	// degenerate (a complex field's outer begin run is always a
	// regular Run from the WSO viewpoint), so we keep field-content
	// exclusion unconditional.
	//
	// Filter once so all subsequent operations (rFonts intersection
	// AND per-prop intersection) operate on the upstream-visible set.
	visible := entries
	excludeBreakOnly := false
	if hasBreakOnly(entries) {
		// Only exclude when at least one non-field-content,
		// non-break-only entry exists.
		for _, e := range entries {
			if !e.fieldContentRun && !e.breakOnlyRun {
				excludeBreakOnly = true
				break
			}
		}
	}
	if hasFieldContent(entries) || excludeBreakOnly {
		visible = make([]runEntry, 0, len(entries))
		for _, e := range entries {
			if e.fieldContentRun {
				continue
			}
			if excludeBreakOnly && e.breakOnlyRun {
				continue
			}
			visible = append(visible, e)
		}
		if len(visible) == 0 {
			// All runs were field-content (degenerate paragraph
			// containing only a single complex field's body chunks).
			return nil
		}
	}
	out := make([]runProp, 0, len(seed))
	rFontsHandled := false
	for _, p := range seed {
		if p.name == "rFonts" {
			if rFontsHandled {
				continue
			}
			rFontsHandled = true
			if merged, ok := commonRFonts(visible); ok {
				out = append(out, runProp{name: "rFonts", xml: merged})
			}
			continue
		}
		all := true
		for _, e := range visible {
			found := false
			for _, q := range e.props {
				if q.name == p.name && q.xml == p.xml {
					found = true
					break
				}
			}
			if !found {
				all = false
				break
			}
		}
		if all {
			out = append(out, p)
		}
	}
	return out
}

// hasFieldContent reports whether any entry is a field-content run.
// Used by commonProps to short-circuit the per-entry filter when
// the paragraph carries no complex-field markup at all (the common
// case across the corpus).
func hasFieldContent(entries []runEntry) bool {
	for _, e := range entries {
		if e.fieldContentRun {
			return true
		}
	}
	return false
}

// hasBreakOnly reports whether any entry is a break-only run. Used by
// commonProps to short-circuit the per-entry filter when the paragraph
// carries no standalone-break envelopes (the common case).
func hasBreakOnly(entries []runEntry) bool {
	for _, e := range entries {
		if e.breakOnlyRun {
			return true
		}
	}
	return false
}

// stripToggleMirrorsFromCommon rewrites the b/i toggle entries in
// the WSO common-rPr set based on the common run's script direction:
//
//   - rtl == false (LTR): drop <w:bCs/>/<w:iCs/> if present, keep
//     <w:b/>/<w:i/>. bCs/iCs are the complex-script bidi-mirrors of
//     b/i (ECMA-376-1 §17.3.2.4): per-run rPr in WordprocessingML
//     pairs `<w:b/>` with `<w:bCs/>` and `<w:i/>` with `<w:iCs/>` to
//     describe the same toggle for LTR vs complex-script text.
//     Upstream Okapi's RunBuilder/RunMerger and StyleOptimisation
//     lift only the b/i toggle into synthesised paragraph styles for
//     LTR runs; the bCs/iCs mirror is reconstructed at run-emit time
//     and is never the surfaced form in the parent <w:rPr>. Observed
//     in 952-3.docx / TestDako2.docx reference.
//   - rtl == true (RTL): rename <w:b/>→<w:bCs/> and <w:i/>→<w:iCs/>
//     so the synthesised style carries the bidi-script toggle that
//     actually applies to the run's complex-script text. Per
//     ECMA-376-1 §17.3.2.4, bCs/iCs are the bold/italic toggles that
//     APPLY to complex-script (RTL) text in a run. The native
//     writer's blockPerRunRPrFragments path always strips bCs/iCs
//     from per-run sidecars BEFORE WSO sees the XML (writer.go
//     :1028-1041), so for RTL runs the b/i entries in `common` are
//     the surviving stand-in — we rename them at WSO time to recover
//     the upstream-emit shape. Observed in 947-cs.docx /
//     947-non-cs-and-cs.docx reference: the synthesised style's rPr
//     is <bCs/><iCs/><rtl/><sz/>, not <b/><i/><rtl/><sz/>.
//
// The rename/drop applies only to the SYNTHESISED style's rPr — the
// raw `common` slice the caller passes in is left untouched, so the
// run-strip pass below (which builds its commonNames map from
// `common`, not `commonForStyle`) continues to lift b/i off every
// run. Upstream's observed behaviour matches: 952-3.docx /
// TestDako2.docx ref has runs with no rPr at all and a synthesised
// paragraph style carrying only <w:b/>; 947-cs.docx ref has the same
// shape with <bCs/><iCs/><rtl/> in the synthesised style.
//
// This function is the WSO-layer counterpart of writer.go's
// stripToggleMirrorChildren (lines 1044-1058) which performs an
// equivalent strip on the per-source-run rPr sidecar before write.
func stripToggleMirrorsFromCommon(props []runProp, rtl bool, parentInheritsRTL bool) []runProp {
	if len(props) == 0 {
		return props
	}
	// Pre-scan: bCs/iCs may already be present in the common props
	// when the writer's per-run sidecar preserved them for complex-
	// script-bearing runs (writer.go adjustRPrForRunText keeps the
	// mirror toggle when run text matches the Okapi complex-script
	// pattern — see ContentCategoriesDetection.java:134-138, ECMA-
	// 376-1 §17.3.2.16 / .17). When that's the case AND the
	// paragraph is RTL, the b→bCs / i→iCs rename below must be
	// SUPPRESSED — otherwise the synthesised style emits a duplicate
	// `<w:bCs/>` (one preserved, one renamed) and a duplicate
	// `<w:iCs/>`. The reference emits a single bCs and iCs per
	// instance (mirrors upstream Okapi RunPropertyFactory.java
	// :201-222 which treats the WpmlToggleRunProperty set as a
	// singleton per property name; ECMA-376-1 §17.3.2 toggle
	// elements appear at most once per <w:rPr>).
	hasBCs := false
	hasICs := false
	// Paired-toggle detection (b ↔ bCs / i ↔ iCs). Mirrors
	// stripToggleMirrorChildren in writer.go which preserves bCs/iCs
	// when the SAME fragment also carries the b/i mirror partner.
	// Upstream Okapi RunParser.canBeSkipped (RunParser.java:240-250):
	// bCs is skippable only when preCombined and runProperties have
	// EQUAL bCs values. Two well-known cases where preCombined and
	// run disagree:
	//
	//   (a) Explicit-off pair: `<w:b w:val="0"/><w:bCs w:val="0"/>`
	//       — paragraph clears an inherited bold AND its complex-
	//       script mirror (highlights_block.docx Caption).
	//   (b) Bare-on pair: `<w:b/><w:bCs/>` — paragraph sets bold on
	//       both halves, often paired against a docDefaults that
	//       clears them (992.docx footer: docDefaults <w:bCs val=0/>
	//       differs from common's bare-on <w:bCs/>, so upstream's
	//       canBeSkipped returns false and bCs survives into the
	//       synthesised pStyle).
	//
	// Native lacks the preCombined view at the WSO lift site but
	// DOES see the b↔bCs / i↔iCs pairing in the common props —
	// authoring tools emit both halves only when they intend the
	// pair to land in the resolved chain, so the pairing is a
	// faithful proxy for "the strip cannot fire".
	hasExplicitOffB := false
	hasExplicitOffI := false
	hasExplicitOffBCs := false
	hasExplicitOffICs := false
	hasAnyB := false
	hasAnyI := false
	hasBareOnBCs := false
	hasBareOnICs := false
	for _, p := range props {
		switch p.name {
		case "bCs":
			hasBCs = true
			if v, ok := parseRPrChildVal(p.xml); ok && (v == "0" || v == "false" || v == "off") {
				hasExplicitOffBCs = true
			} else if !ok {
				// No val attribute — bare-on form `<w:bCs/>`.
				hasBareOnBCs = true
			}
		case "iCs":
			hasICs = true
			if v, ok := parseRPrChildVal(p.xml); ok && (v == "0" || v == "false" || v == "off") {
				hasExplicitOffICs = true
			} else if !ok {
				hasBareOnICs = true
			}
		case "b":
			hasAnyB = true
			if v, ok := parseRPrChildVal(p.xml); ok && (v == "0" || v == "false" || v == "off") {
				hasExplicitOffB = true
			}
		case "i":
			hasAnyI = true
			if v, ok := parseRPrChildVal(p.xml); ok && (v == "0" || v == "false" || v == "off") {
				hasExplicitOffI = true
			}
		}
	}
	out := make([]runProp, 0, len(props))
	for _, p := range props {
		switch p.name {
		case "bCs":
			// Keep bCs on RTL paragraphs (the bidi-script bold toggle
			// applies to complex-script text). On LTR, also keep when:
			//   (a) explicit-off pair (clearing inherited bold from
			//       both Latin and complex-script halves), OR
			//   (b) bare-on pair AND docDefaults declares
			//       <w:bCs w:val="0"/> — the docDefaults' explicit-
			//       off bCs disagrees with the run's bare-on bCs,
			//       so upstream RunParser.canBeSkipped (RunParser.java:
			//       240-250) returns false (pcrp.equals(rp)=false) and
			//       refuses the strip.
			// 992.docx: docDefaults `<w:bCs val="0"/>` + run `<w:bCs/>`
			// → preserve.
			// large-attribute.docx counter: no docDefaults bCs → strip.
			if rtl || (hasExplicitOffBCs && hasExplicitOffB) || (hasBareOnBCs && hasAnyB && currentDocDefaultsBCsExplicitOff) {
				out = append(out, p)
			}
		case "iCs":
			if rtl || (hasExplicitOffICs && hasExplicitOffI) || (hasBareOnICs && hasAnyI && currentDocDefaultsICsExplicitOff) {
				out = append(out, p)
			}
		case "b":
			if rtl {
				// Rename b→bCs only when bCs is not already present
				// in the common — otherwise the synthesised style
				// would emit a duplicate `<w:bCs/>`. Dropping `b`
				// when bCs is present is correct: the paragraph's
				// runs are complex-script (rtl=true) and the
				// preserved bCs covers the bold toggle for that
				// text per ECMA-376-1 §17.3.2.16. 947-cs.docx is
				// the canonical fixture.
				if !hasBCs {
					out = append(out, runProp{name: "bCs", xml: "<w:bCs/>"})
				}
			} else if !defaultCharStyleSuppliesToggle(p, "b") {
				out = append(out, p)
			}
		case "i":
			if rtl {
				if !hasICs {
					out = append(out, runProp{name: "iCs", xml: "<w:iCs/>"})
				}
			} else if !defaultCharStyleSuppliesToggle(p, "i") {
				out = append(out, p)
			}
		case "rtl":
			// Drop explicit-off `<w:rtl w:val="0"/>` from the
			// synthesised style's rPr. minifyRPrChildren preserves
			// the clearing form on per-run rPr (mirroring Okapi's
			// observed behavior — see runprops.go for the empirical
			// rationale), but lifting the SAME clearing form into
			// the synthesised paragraph style is a different
			// outcome: upstream Okapi's WSO never promotes a
			// directly-specified explicit-off rtl into the synth
			// style's rPr (830-2.docx / 830-6.docx reference output:
			// runs keep `<w:rtl w:val="0"/>` while the synthesised
			// `NF974E24F-a1` style has NO rtl child). Per ECMA-376-1
			// §17.3.2.4 a paragraph style without `<w:rtl/>` already
			// implies LTR for its runs, so a pStyle-level
			// `<w:rtl w:val="0"/>` is structurally redundant — it
			// would only matter if some basedOn ancestor turned rtl
			// on. parentInheritsRTL flags exactly that case (899.docx
			// Normal style authors `<w:rtl/>` so synth children based
			// on Normal need the clearing form to actually clear).
			// For 830-2-shaped fixtures the chain is rtl-free, so the
			// explicit-off form is dropped from the lift.
			val, hasVal := parseRPrChildVal(p.xml)
			if hasVal && (val == "0" || val == "false" || val == "off") {
				if !parentInheritsRTL {
					continue
				}
			}
			out = append(out, p)
		case "highlight":
			// Drop `<w:highlight w:val="white"/>` from the
			// synthesised style's rPr — `white` and `none` resolve
			// to the same RGB FFFFFF (the system default
			// background), and upstream Okapi treats them as
			// equivalent via HighlightRunProperty.equalsProperty
			// (RunProperty.java:259-264) which compares values
			// through HighlightColorValues.valuesFor (Color.java:
			// 172-176, matching by RGB / external name / internal
			// name). When the document defaults' rPr lacks
			// `<w:highlight>`, addExplicitDefaults
			// (WordStyleDefinition.java:164-191) injects a phantom
			// `<w:highlight w:val="none"/>` for the lifetime of the
			// minified()/contains() comparison; the run-side
			// `highlight=white` then matches and is excluded from
			// the synthesised style's lifted set. Per ECMA-376-1
			// §17.3.2.15 (CT_Highlight) the rendered colour for
			// "none" and "white" is identical; lifting "white"
			// into the synthesised style is a no-op vs the implicit
			// default, and Okapi's reference output omits it.
			//
			// Other highlight values (yellow, green, red, …) are
			// preserved verbatim — they encode a real visible
			// highlight that differs from the default background.
			// 830-3.docx, 830-5.docx, 830-6.docx are the canonical
			// fixtures where every run carries
			// `<w:highlight w:val="white"/>` and the synthesised
			// style's reference rPr does NOT include it.
			val, hasVal := parseRPrChildVal(p.xml)
			if hasVal && val == "white" {
				continue
			}
			out = append(out, p)
		default:
			// Default character style toggle suppression — apply to
			// any toggle in the recognised set (strike, outline,
			// shadow, vanish, caps, smallCaps, emboss, imprint,
			// dstrike, rtl). Per ECMA-376-1 §17.7.4 the default
			// character style applies implicitly; toggles it asserts
			// already flow into preCombined and would be dropped by
			// upstream's RunProperties.minified() before the WSO
			// common-rPr lift.
			if defaultCharStyleSuppliesToggle(p, p.name) {
				continue
			}
			out = append(out, p)
		}
	}
	return out
}

// defaultCharStyleSuppliesToggle reports whether the default
// character style (currentDefaultCharacterStyleToggles, populated by
// the writer's WSO pre-pass) declares a bare-on form of the named
// toggle, AND the candidate prop p is itself the bare-on form (no
// explicit-off val attribute). When true, lifting p into a
// synthesised paragraph style would be redundant — upstream Okapi
// drops the duplicate via RunProperties.minified()'s
// `preCombined.contains(p)` Property.equals branch
// (RunProperties.java:497-540).
//
// document-style-definitions.docx canonical: Emphasis is the default
// character style with `<w:i/>`; runs that author `<w:i/>` directly
// match the chain value and are dropped from the synthesised pStyle
// by upstream. Native consumes this flag in stripToggleMirrorsFromCommon
// to mirror that behaviour.
func defaultCharStyleSuppliesToggle(p runProp, name string) bool {
	if currentDefaultCharacterStyleToggles == nil {
		return false
	}
	if !currentDefaultCharacterStyleToggles[name] {
		return false
	}
	// Only suppress when p is the bare-on form (no explicit-off val).
	if val, ok := parseRPrChildVal(p.xml); ok && (val == "0" || val == "false" || val == "off") {
		return false
	}
	return true
}

// commonContainsRTL reports whether the common-rPr set contains a
// TRUTHY <w:rtl/> marker — i.e. the run is complex-script (RTL).
// Per ECMA-376-1 §17.3.2.4, <w:rtl/> marks the run as containing
// complex-script (right-to-left) content — the cue used by
// RunBuilder/RunMerger to pick the bCs/iCs toggles over b/i.
//
// minifyRPrChildren NOW preserves explicit-off `<w:rtl w:val="0"/>`
// when the resolved style chain carries an rtl toggle by name
// (mirrors RunProperties.java:497-540's preCombined.contains-by-name
// branch — used by 899.docx where the Normal style has <w:rtl/>).
// That clearing form must NOT be treated as a truthy RTL marker
// here: it is the run authoring "I am LTR despite my paragraph
// style being RTL." Pre-#xxx the clearing form was unconditionally
// stripped at parse time, so any surviving rtl runProp was
// guaranteed truthy. The check below now also inspects the value
// attribute and excludes the "0" / "false" / "off" forms (ECMA-376
// §17.3.2 toggle semantics).
func commonContainsRTL(props []runProp) bool {
	for _, p := range props {
		if p.name == "rtl" {
			val, hasVal := parseRPrChildVal(p.xml)
			if hasVal && (val == "0" || val == "false" || val == "off") {
				continue
			}
			return true
		}
	}
	return false
}

// commonRFonts computes the per-attribute intersection of every run's
// <w:rFonts>. Returns the synthesised rFonts XML (with attribute order
// matching the first run that has rFonts) and true iff the intersection
// is non-empty AND every run has an rFonts.
//
// Attribute equality is by exact (name, value) pair. Attribute name
// uses the namespace-prefixed form as it appears in the source (e.g.
// "w:ascii"); the value is compared after stripping its quote
// character. Both forms are preserved in the emitted rFonts.
func commonRFonts(entries []runEntry) (string, bool) {
	if len(entries) == 0 {
		return "", false
	}
	// Every entry must have exactly one rFonts (the typical case;
	// duplicate rFonts within a single rPr is invalid per ECMA-376
	// schema and would indicate malformed input — skip optimisation).
	//
	// When the source docDefaults declares an rFonts (ECMA-376-1
	// §17.7.5.5 CT_DocDefaults) we virtually overlay it onto entries
	// that lack a direct rFonts — mirroring upstream Okapi
	// RunMerger's clarifyFontsRunProperties (RunMerger.java:389-396)
	// which refines the merged run's direct rFonts to the combined-
	// run-properties view (docDefaults + pStyle + rStyle + direct,
	// per WordStyleDefinitions.combinedRunProperties lines 302-315).
	// The overlay lets the per-content-category intersection see
	// the docDefaults-supplied attributes instead of bailing out the
	// moment a single rFonts-less run shows up.
	//
	// StartsWithLineSeparator.docx paragraph 1 canonical fixture:
	// R1 carries `<w:rFonts ascii=MS Gothic eastAsia=MS Gothic
	// hAnsi=MS Gothic cs=MS Gothic/>` and R2-R4 carry no direct rPr
	// at all. With docDefaults `<w:rFonts asciiTheme=minorHAnsi
	// eastAsiaTheme=minorHAnsi hAnsiTheme=minorHAnsi
	// cstheme=minorBidi/>`, the overlay gives R2-R4 a virtual rFonts
	// matching docDefaults so the per-category intersection can
	// surface the agreed effective values (asciiTheme=minorHAnsi via
	// R1's docDefaults overlay + R2-R4's direct-from-overlay).
	var firstAttrs []rfontsAttr
	allAttrs := make([][]rfontsAttr, len(entries))
	allAttrSets := make([]map[string]string, len(entries))
	// hadDirectRFonts[i] tracks whether entry i ORIGINALLY authored
	// rFonts (i.e. before the docDefaults overlay). Used below to
	// keep first-attr-source-ordering anchored on a real run rather
	// than a virtual one.
	hadDirectRFonts := make([]bool, len(entries))
	for i, e := range entries {
		var rfonts *runProp
		for k := range e.props {
			if e.props[k].name == "rFonts" {
				if rfonts != nil {
					return "", false // duplicate rFonts in one rPr
				}
				rfonts = &e.props[k]
			}
		}
		var attrs []rfontsAttr
		if rfonts != nil {
			parsed, ok := parseRFontsAttrs(rfonts.xml)
			if !ok {
				return "", false
			}
			attrs = parsed
			hadDirectRFonts[i] = true
		} else if len(currentDocDefaultsRFonts) > 0 {
			// Virtual overlay from docDefaults. Copy the slice so any
			// downstream mutation (none today, but defensive) does
			// not leak into the module-level state.
			attrs = make([]rfontsAttr, len(currentDocDefaultsRFonts))
			copy(attrs, currentDocDefaultsRFonts)
		} else {
			return "", false // a run lacks rFonts AND no docDefaults overlay → not common
		}
		if firstAttrs == nil {
			firstAttrs = attrs
		}
		allAttrs[i] = attrs
		m := make(map[string]string, len(attrs))
		for _, a := range attrs {
			m[a.name] = a.value
		}
		allAttrSets[i] = m
	}
	// Prefer the first entry that ORIGINALLY authored rFonts as the
	// source-order anchor for firstAttrs. When the first entry was
	// virtual (docDefaults overlay) and a later entry carries a real
	// direct rFonts, the real direct's attribute ORDER is what
	// upstream's emitted rFonts surfaces — keeping the synth-pStyle
	// output stable across map iteration order and aligned with
	// upstream Okapi's serialised attribute sequence.
	for i, attrs := range allAttrs {
		if hadDirectRFonts[i] {
			firstAttrs = attrs
			break
		}
	}
	// Per-content-category emit with theme/direct cross-equivalence.
	// Mirrors upstream Okapi RunFonts.mergeContentCategories
	// (RunFonts.java:299-315): for each (direct, theme) pair, the
	// merged effective value is the agreed value across runs (theme
	// dominates direct per run); when every run asserting the
	// category agrees on the effective value, emit every attribute
	// any run authored for that pair.
	//
	// FontThemeOverFont.docx WSO lift: runs disagree on the direct
	// `ascii` attribute (minorHAnsi vs "Times New Roman") but agree
	// on the effective ASCII content-category value (minorHAnsi via
	// asciiTheme on the Times-direct run). Upstream lifts BOTH
	// ascii=minorHAnsi (R1) AND asciiTheme=minorHAnsi (R3) into the
	// synthesised pStyle — preserving each run's content-category-
	// detection signal per ECMA-376-1 §17.3.2.26.
	categoryEmit := make(map[string]string, 8)
	categoryAttrs := rFontsCategoryAttrSet()
	for _, pair := range rFontsThemePairs {
		type sample struct {
			eff    string
			direct string
			theme  string
			hasD   bool
			hasT   bool
		}
		samples := make([]sample, 0, len(allAttrSets))
		for _, m := range allAttrSets {
			direct, hasDirect := m[pair.direct]
			theme, hasTheme := m[pair.theme]
			if !hasDirect && !hasTheme {
				continue
			}
			s := sample{direct: direct, theme: theme, hasD: hasDirect, hasT: hasTheme}
			switch {
			case hasTheme:
				s.eff = theme
			case hasDirect:
				s.eff = direct
			}
			samples = append(samples, s)
		}
		if len(samples) < len(allAttrSets) {
			// Some run lacks both direct/theme for this category —
			// intersection drops the pair.
			continue
		}
		eff := samples[0].eff
		agree := true
		for _, s := range samples[1:] {
			if s.eff != eff {
				agree = false
				break
			}
		}
		if !agree {
			continue
		}
		// Prefer "theme-less direct" for the direct slot — those are
		// the runs whose direct attribute is "the effective value" per
		// upstream's containsDetected semantics (when the run has no
		// theme set, the direct attribute IS the detected category
		// value). When every asserting run also has theme, fall back
		// to any direct value (they all agree on the effective value
		// per the gate above, but their direct values may differ —
		// e.g. one carries `ascii="Times New Roman"` overridden by
		// `asciiTheme=minorHAnsi`).
		var preferredDirect, preferredTheme string
		var preferredDirectFromThemeless bool
		for _, s := range samples {
			if s.hasD {
				if !s.hasT {
					if !preferredDirectFromThemeless {
						preferredDirect = s.direct
						preferredDirectFromThemeless = true
					}
				} else if !preferredDirectFromThemeless && preferredDirect == "" {
					preferredDirect = s.direct
				}
			}
			if s.hasT && preferredTheme == "" {
				preferredTheme = s.theme
			}
		}
		anyDirect := false
		anyTheme := false
		for _, s := range samples {
			if s.hasD {
				anyDirect = true
			}
			if s.hasT {
				anyTheme = true
			}
		}
		if anyDirect {
			categoryEmit[pair.direct] = preferredDirect
		}
		if anyTheme {
			categoryEmit[pair.theme] = preferredTheme
		}
	}
	// Non-category attributes (including hint): byte-equal
	// intersection. The hint slot's per-run merge applies at parse
	// time via RunMerger; the WSO common-rPr lift here operates on
	// the post-merge population and treats every property as "common
	// iff every entry has the same byte-equal value". Mirrors
	// upstream StyleOptimisation.commonRunPropertiesOf
	// (StyleOptimisation.java:204-237) Property.equals semantics.
	var keptNonCategory []rfontsAttr
	for _, a := range firstAttrs {
		if categoryAttrs[a.name] {
			continue
		}
		ok := true
		for j := 1; j < len(allAttrSets); j++ {
			v, present := allAttrSets[j][a.name]
			if !present || v != a.value {
				ok = false
				break
			}
		}
		if ok {
			keptNonCategory = append(keptNonCategory, a)
		}
	}
	// Assemble category attributes in source-order: first run's
	// authored order takes precedence, then later runs' category
	// attributes that the first run lacked (e.g. asciiTheme from R3).
	var emitted []rfontsAttr
	seen := make(map[string]bool, len(categoryEmit))
	for _, a := range firstAttrs {
		if !categoryAttrs[a.name] {
			continue
		}
		if v, ok := categoryEmit[a.name]; ok && !seen[a.name] {
			emitted = append(emitted, rfontsAttr{name: a.name, value: v, quote: a.quote})
			seen[a.name] = true
		}
	}
	for i := 1; i < len(allAttrs); i++ {
		for _, a := range allAttrs[i] {
			if !categoryAttrs[a.name] {
				continue
			}
			if v, ok := categoryEmit[a.name]; ok && !seen[a.name] {
				emitted = append(emitted, rfontsAttr{name: a.name, value: v, quote: a.quote})
				seen[a.name] = true
			}
		}
	}
	emitted = append(emitted, keptNonCategory...)
	if len(emitted) == 0 {
		return "", false
	}
	// Whole-RunFonts equality gate — mirrors upstream Okapi's
	// StyleOptimisation.commonRunPropertiesOf which intersects via
	// `List<Property>.retainAll` using RunFonts.equals (RunFonts.java:
	// 380-387, strict map equality of the EnumMap<ContentCategory,String>).
	// A Property is "common" iff every run's RunProperties list contains
	// a Property that compares equal — for RunFonts that means every run
	// must have an effectively-equal RunFonts.
	//
	// The per-attribute intersection above is the natural Go shape, but
	// without this gate it can lift a partial-overlap rFonts that upstream
	// would reject. Hangs.docx P6 is the canonical case: R1 has only
	// `<w:rFonts w:hint="eastAsia"/>` while R2 has
	// `<w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman"
	// w:hint="eastAsia"/>`. The hint matches byte-equal in both, so
	// keptNonCategory keeps it, but upstream's RunFonts.equals sees
	// `{HINT=eastAsia}` ≠ `{ASCII=TNR, HIGH_ANSI=TNR, HINT=eastAsia}`
	// and drops rFonts from the common — common is empty, WSO bypasses,
	// pStyle=a4 stays. Without this gate native lifts `{rFonts hint}`,
	// synthesises NF974E24F-a425, and the entire header1.xml diverges
	// (+26K bytes from the bigger style table + every paragraph swap).
	//
	// Theme/direct equivalence (FontThemeOverFont.docx) IS preserved: the
	// effective-value comparison considers `ascii`+`asciiTheme` jointly
	// per content-category (theme dominates per RunFonts.mergeContent
	// Categories at RunFonts.java:299-315) — when a run authors only
	// `ascii=minorHAnsi` and another authors `ascii=TNR asciiTheme=
	// minorHAnsi`, both effective ASCII values are minorHAnsi so the
	// equivalence holds and the rFonts stays in the common.
	if !rFontsEmittedEquivalentToEveryRun(emitted, allAttrSets) {
		return "", false
	}
	// Re-emit. Preserve the source rFonts element name prefix (likely
	// "w:rFonts" but could differ).
	prefix := extractRFontsElemNameFromProps(entries[0].props)
	if prefix == "" {
		prefix = "w:rFonts"
	}
	var b strings.Builder
	b.WriteByte('<')
	b.WriteString(prefix)
	for _, a := range emitted {
		b.WriteByte(' ')
		b.WriteString(a.name)
		b.WriteByte('=')
		q := a.quote
		if q == 0 {
			q = '"'
		}
		b.WriteByte(q)
		b.WriteString(a.value)
		b.WriteByte(q)
	}
	b.WriteString("/>")
	return b.String(), true
}

// rFontsEmittedEquivalentToEveryRun reports whether the synthesised
// `emitted` rFonts attribute set is RunFonts-equivalent to every run's
// rFonts attribute set in `allAttrSets`. Mirrors upstream Okapi's
// RunFonts.equals (RunFonts.java:380-387, strict EnumMap equality)
// modulo the theme/direct effective-value collapse from
// RunFonts.mergeContentCategories (RunFonts.java:299-315):
//
//  1. For each content category (ASCII/HIGH_ANSI/COMPLEX_SCRIPT/
//     EAST_ASIAN), the effective value is the theme attribute's value
//     when set, otherwise the direct attribute's value, otherwise unset.
//     A run is equivalent to emitted at this category iff their effective
//     values are equal (or both unset).
//  2. Non-category attributes (e.g. `w:hint`) are compared by direct
//     value equality — equivalent iff both sides have the same value (or
//     both lack the attribute).
//
// The check is needed because commonRFonts's per-attribute intersection
// can land on an emit shape that's a strict subset of one of the runs'
// attribute sets — upstream's whole-Property semantics would have
// rejected that case in commonRunPropertiesOf. See the gate-call site
// in commonRFonts for the Hangs.docx motivating fixture.
func rFontsEmittedEquivalentToEveryRun(emitted []rfontsAttr, allAttrSets []map[string]string) bool {
	emittedMap := make(map[string]string, len(emitted))
	for _, a := range emitted {
		emittedMap[a.name] = a.value
	}
	// Compute emitted's effective ContentCategory map. Keyed by direct
	// attribute name (the "category id"). Theme overrides direct.
	type catEff struct {
		value string
		set   bool
	}
	emittedCat := make(map[string]catEff, len(rFontsThemePairs))
	for _, pair := range rFontsThemePairs {
		var eff catEff
		if v, ok := emittedMap[pair.theme]; ok {
			eff = catEff{value: v, set: true}
		} else if v, ok := emittedMap[pair.direct]; ok {
			eff = catEff{value: v, set: true}
		}
		emittedCat[pair.direct] = eff
	}
	// Per-run check.
	for _, runMap := range allAttrSets {
		// Category-effective equivalence.
		for _, pair := range rFontsThemePairs {
			var runEff catEff
			if v, ok := runMap[pair.theme]; ok {
				runEff = catEff{value: v, set: true}
			} else if v, ok := runMap[pair.direct]; ok {
				runEff = catEff{value: v, set: true}
			}
			ee := emittedCat[pair.direct]
			if runEff.set != ee.set || runEff.value != ee.value {
				return false
			}
		}
		// Non-category attribute equivalence — covers `w:hint` and any
		// other rFonts attribute outside the theme-pair table (the
		// category attrs are explicitly handled above; the
		// rFontsCategoryAttrSet exclusion in commonRFonts's per-attribute
		// loop means non-category attrs in `emitted` are the only ones
		// here).
		categoryAttrs := rFontsCategoryAttrSet()
		// Every non-category attr in emitted must equal run's.
		for name, val := range emittedMap {
			if categoryAttrs[name] {
				continue
			}
			rv, ok := runMap[name]
			if !ok || rv != val {
				return false
			}
		}
		// Every non-category attr in run must equal emitted's (catches
		// the case where the run has an extra non-category attribute the
		// emitted set lacks — upstream's whole-RunFonts equality would
		// reject that).
		for name := range runMap {
			if categoryAttrs[name] {
				continue
			}
			if _, ok := emittedMap[name]; !ok {
				return false
			}
		}
	}
	return true
}

// extractRFontsElemNameFromProps returns the prefixed element name of the first
// rFonts found in props, e.g. "w:rFonts". Returns "" if not found.
func extractRFontsElemNameFromProps(props []runProp) string {
	for _, p := range props {
		if p.name != "rFonts" {
			continue
		}
		// Tag is like "<w:rFonts ...>" — extract up to first space/slash/>.
		if len(p.xml) < 2 || p.xml[0] != '<' {
			return ""
		}
		end := strings.IndexAny(p.xml[1:], " \t\n\r/>")
		if end < 0 {
			return ""
		}
		return p.xml[1 : 1+end]
	}
	return ""
}

// rfontsAttr captures one parsed rFonts attribute.
type rfontsAttr struct {
	name  string // prefixed name as in source, e.g. "w:ascii"
	value string // unescaped value (quotes stripped)
	quote byte
}

// parseRFontsAttrs parses attributes of a self-closing or open-form
// <w:rFonts ...> element. Returns the attribute list in source order.
// Returns false if the element is malformed.
func parseRFontsAttrs(xmlStr string) ([]rfontsAttr, bool) {
	if len(xmlStr) < 2 || xmlStr[0] != '<' {
		return nil, false
	}
	// Skip element name.
	nameEnd := strings.IndexAny(xmlStr[1:], " \t\n\r/>")
	if nameEnd < 0 {
		return nil, false
	}
	rest := xmlStr[1+nameEnd:]
	// Find end of start-tag.
	tagEnd := strings.IndexByte(rest, '>')
	if tagEnd < 0 {
		return nil, false
	}
	body := rest[:tagEnd]
	if len(body) > 0 && body[len(body)-1] == '/' {
		body = body[:len(body)-1]
	}
	var attrs []rfontsAttr
	i := 0
	for i < len(body) {
		// Skip whitespace.
		for i < len(body) && (body[i] == ' ' || body[i] == '\t' || body[i] == '\n' || body[i] == '\r') {
			i++
		}
		if i >= len(body) {
			break
		}
		// Read name up to '='.
		eq := strings.IndexByte(body[i:], '=')
		if eq < 0 {
			return nil, false
		}
		name := strings.TrimRight(body[i:i+eq], " \t\n\r")
		i += eq + 1
		// Skip whitespace.
		for i < len(body) && (body[i] == ' ' || body[i] == '\t' || body[i] == '\n' || body[i] == '\r') {
			i++
		}
		if i >= len(body) {
			return nil, false
		}
		q := body[i]
		if q != '"' && q != '\'' {
			return nil, false
		}
		i++
		end := strings.IndexByte(body[i:], q)
		if end < 0 {
			return nil, false
		}
		val := body[i : i+end]
		i += end + 1
		attrs = append(attrs, rfontsAttr{name: name, value: val, quote: q})
	}
	return attrs, true
}

// buildRPrXML emits the children-only serialization of the common
// props (no enclosing <w:rPr>...</w:rPr>).
func buildRPrXML(props []runProp) string {
	if len(props) == 0 {
		return ""
	}
	var b strings.Builder
	for _, p := range props {
		b.WriteString(p.xml)
	}
	return b.String()
}

// findMatchingStyle searches both the existing-source and the
// in-progress synthesised set for a paragraph style with the same
// parent and identical rPr body. Returns the styleId or "" if none.
//
// Mirrors WordStyleDefinitions.Ids.parentBased() (WordStyleDefinitions
// .java:462-475) — Okapi's optimiser re-uses an existing matching style
// (source OR synthesised) instead of creating a new one. The upstream
// `stylesByStyleIds` map is populated with source styles during
// WordStyleDefinitions.readWith BEFORE any synth ids are placed, so the
// `.entrySet().stream()...findFirst()` walk naturally considers BOTH
// kinds.
//
// Match criteria (mirroring the upstream filter chain):
//
//  1. `type == StyleType.PARAGRAPH` — source side is filtered by
//     extractSourceParagraphStyles which only collects
//     `w:type="paragraph"` entries; synthesised side is paragraph by
//     construction (`<w:style w:type="paragraph" .../>` in
//     injectSynthesisedStyles).
//  2. `parentId.equals(...)` — string-equal on basedOn.
//  3. `paragraphBlockProperties.mergeableWith(other.paragraphProperties())`
//     — the candidate's pPr properties must be a subset of the
//     paragraph's pPr at the WSO call site (ParagraphBlockProperties
//     .java:693-701, Word.mergeableWith). Native's WSO scope is rPr
//     only; we don't track paragraph-level pPr equality, so we apply
//     the conservative guard described on sourceParagraphStyleInfo
//     .hasParagraphProps — candidates with a non-empty pPr are
//     rejected.
//  4. `runProperties.equals(other.runProperties())` — order-sensitive
//     element-by-element equality of the rPr children list
//     (RunProperties.java:653-663). On the synth side we already use a
//     byte-equal rPrXML compare; on the source side we use the
//     canonical runProp slice via runPropsEqual.
//
// Source-style matches return the source styleId verbatim (e.g.
// "FranzJosef"), so the inserted `<w:pStyle w:val="FranzJosef"/>` re-
// uses an existing definition and NO synth id is generated — clearing
// counter-drift in fixtures where upstream's parentBased finds a
// source-side match before the IdGenerator ticks.
func findMatchingStyle(
	parentID string,
	rPrXML string,
	common []runProp,
	synthesised map[string]synthesisedStyle,
	orderedIDs []string,
) string {
	// Source paragraph styles win when present — upstream's
	// `entrySet().stream().findFirst()` traversal sees source styles
	// first because they were placed during readWith before any synth
	// ids exist. Match deterministically by sorted styleId so output is
	// stable across map iteration order (Go map ranges are intentionally
	// unordered).
	if len(currentSourceParagraphStyles) > 0 {
		ids := make([]string, 0, len(currentSourceParagraphStyles))
		for id := range currentSourceParagraphStyles {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			info := currentSourceParagraphStyles[id]
			if info.basedOn != parentID {
				continue
			}
			if info.hasParagraphProps {
				// See sourceParagraphStyleInfo.hasParagraphProps doc —
				// conservative skip; native does not track per-pPr
				// property equality so we can't safely assert
				// mergeableWith for candidates with pPr content.
				continue
			}
			if !runPropsEqual(info.rPrProps, common) {
				continue
			}
			return id
		}
	}
	for _, id := range orderedIDs {
		s := synthesised[id]
		if s.parentID == parentID && s.rPrXML == rPrXML {
			return id
		}
	}
	return ""
}

// findSupersetSynthStyle finds an existing synthesised paragraph style
// whose rPr props are a strict SUPERSET of `common` (every prop in
// `common` is present, with identical XML, in the style's rPr) and
// whose parentID matches.
//
// Used as a narrow recovery for paragraphs where the writer has merged
// a break-only `<w:r>` source chunk into the adjacent text-bearing
// `<w:r>` (collapsing two distinct upstream Runs into one merged Run).
// The merge loses the text-bearing run's richer rPr in favour of the
// break-only run's rPr, so WSO sees an under-specified common-prop set
// (e.g. `{szCs=21}` instead of the upstream-visible `{rFonts hint=eastAsia,
// szCs=21}`). When an earlier paragraph in the same document already
// synthesised the richer style, that style is what upstream Okapi
// would have placed on this paragraph too — see EndGroup.docx:
// upstream synth Normal1 carries `{rFonts hint=eastAsia, szCs=21}` and
// is reused on the merged-break+text paragraph; native, computing
// common=`{szCs=21}` from the merged run, would otherwise synthesise a
// fresh "Normal2" with just `<w:szCs/>`.
//
// Narrow trigger gate (single-run paragraph where the run contains BOTH
// `<w:br>` AND `<w:t>`) avoids false-positive matches on independent
// paragraphs whose smaller rPr happens to be a subset of a richer
// earlier synth — upstream creates one style per unique rPr signature
// (WordStyleDefinitions.Ids.parentBased line 469 uses RunProperties
// .equals on the COMPLETE set, not a subset match) and so do we, except
// in this writer-merge recovery path.
//
// Returns "" when no superset match exists. Matches deterministically
// in orderedIDs order so output stays reproducible.
func findSupersetSynthStyle(
	parentID string,
	common []runProp,
	synthesised map[string]synthesisedStyle,
	orderedIDs []string,
) string {
	if len(common) == 0 {
		return ""
	}
	for _, id := range orderedIDs {
		s := synthesised[id]
		if s.parentID != parentID {
			continue
		}
		// synthesisedStyle.rPrXML stores children-only XML (no wrapping
		// <w:rPr> element) — wrap it so parseRunPropElements can locate
		// its parent. See buildRPrXML for the children-only contract.
		styleProps := parseRunPropElements([]byte("<w:rPr>" + s.rPrXML + "</w:rPr>"))
		if len(styleProps) <= len(common) {
			// Need a strict superset — same size means equality (which
			// would have already matched in findMatchingStyle).
			continue
		}
		allFound := true
		for _, c := range common {
			found := false
			for _, p := range styleProps {
				if p.name == c.name && p.xml == c.xml {
					found = true
					break
				}
			}
			if !found {
				allFound = false
				break
			}
		}
		if allFound {
			return id
		}
	}
	return ""
}

// extractPPrRPrRFontsAttrs returns the rFonts attribute set declared on
// the paragraph mark's rPr (i.e. <w:pPr><w:rPr><w:rFonts .../></w:rPr>
// </w:pPr>) inside the given pPr extent, or nil when no rFonts is
// present. Used by findSynthMatchingWithPPrHint to surface the paragraph-
// mark's effective rFonts.hint when the merged run's rPr lacks it.
//
// Per ECMA-376-1 §17.3.1.29 (CT_PPr.rPr) the paragraph mark's rPr
// describes the formatting applied to the paragraph mark glyph AND
// participates in the run-level rFonts merge as a fallback context for
// runs whose own rFonts doesn't override (RunFonts.merge in upstream
// Okapi — RunFonts.java:232-248 — composes the effective rFonts per
// content category from the docDefaults / pStyle / paragraph-mark / run
// chain).
func extractPPrRPrRFontsAttrs(pPrSrc []byte) []rfontsAttr {
	// Locate <w:rPr> child of pPr.
	rPrStart, rPrEnd, hasRPr := findFirstChild(pPrSrc, "rPr")
	if !hasRPr {
		return nil
	}
	rPrBody := pPrSrc[rPrStart:rPrEnd]
	// Find <w:rFonts ...> within rPr.
	rfStart, rfEnd, hasRF := findFirstChild(rPrBody, "rFonts")
	if !hasRF {
		return nil
	}
	attrs, ok := parseRFontsAttrs(string(rPrBody[rfStart:rfEnd]))
	if !ok {
		return nil
	}
	return attrs
}

// findSynthMatchingWithPPrHint finds an existing synthesised paragraph
// style whose rPr matches `common` EXCEPT that the style's rFonts may
// carry additional attributes (notably `hint`) that the paragraph mark's
// rPr.rFonts authors. Used for the writer-merge recovery case where the
// merged run's rPr lacks an explicit hint that the paragraph-mark rFonts
// provides; per RunFonts.merge (RunFonts.java:232-248) the effective
// rFonts seen by upstream Okapi WSO would carry the paragraph-mark hint.
//
// Match criteria:
//   - Style's parentID matches.
//   - Style's non-rFonts props equal `common`'s non-rFonts props
//     element-by-element (same order, same xml).
//   - Style's rFonts attributes are equal to `common`'s rFonts plus
//     OPTIONALLY a `hint` attribute that pPrRFontsAttrs also declares
//     with the same value. Style may also carry attributes that
//     pPrRFontsAttrs declares — supporting paragraphs whose run-level
//     rFonts dropped a font category attribute (e.g. eastAsia direct)
//     that the paragraph mark still declares.
//
// Returns "" when no match exists. Matches deterministically in
// orderedIDs order so output stays reproducible.
//
// Fixture: Hangs.docx — a single-run paragraph whose source authored two
// runs (a break-only run + a text run with rFonts.hint=eastAsia) gets
// fused by the writer into one run whose rPr lacks the hint. The
// paragraph mark's rPr.rFonts still carries hint=eastAsia. Existing
// synth a14 with rFonts ascii=hAnsi=TNR hint=eastAsia + sz=22 is what
// upstream Okapi reuses (text run's rPr survives upstream's RunMerger
// path because the two source runs disagree on rFonts.hint and canMerge
// blocks the merge); native must match a14 here to avoid synthesising a
// duplicate a19 with the hint-less rFonts.
func findSynthMatchingWithPPrHint(
	parentID string,
	common []runProp,
	pPrRFontsAttrs []rfontsAttr,
	synthesised map[string]synthesisedStyle,
	orderedIDs []string,
) string {
	if len(pPrRFontsAttrs) == 0 {
		return ""
	}
	// Extract common's rFonts attrs (if any) for delta comparison.
	var commonRFontsAttrs []rfontsAttr
	commonHasRFonts := false
	for _, p := range common {
		if p.name == "rFonts" {
			attrs, ok := parseRFontsAttrs(p.xml)
			if !ok {
				return ""
			}
			commonRFontsAttrs = attrs
			commonHasRFonts = true
			break
		}
	}
	if !commonHasRFonts {
		return ""
	}
	// Build a hint-augmented attribute set from common + any attribute
	// pPrRFontsAttrs provides that common lacks. Only consider non-
	// category attributes (notably `hint`) for the augmentation — the
	// category attribute set (ascii/hAnsi/cs/eastAsia + their theme
	// siblings) must already agree byte-equal between common's rFonts
	// and the candidate style's rFonts to avoid widening matches into
	// fonts the run never authored.
	categoryAttrs := rFontsCategoryAttrSet()
	augmented := make(map[string]string, len(commonRFontsAttrs)+1)
	for _, a := range commonRFontsAttrs {
		augmented[a.name] = a.value
	}
	pPrAttrMap := make(map[string]string, len(pPrRFontsAttrs))
	for _, a := range pPrRFontsAttrs {
		pPrAttrMap[a.name] = a.value
	}
	// Augment common's rFonts with non-category attrs from pPr.rPr.rFonts.
	for n, v := range pPrAttrMap {
		if categoryAttrs[n] {
			continue
		}
		if _, present := augmented[n]; !present {
			augmented[n] = v
		}
	}
	if len(augmented) == len(commonRFontsAttrs) {
		// No augmentation occurred — the match would already be found by
		// findMatchingStyle's exact compare. Skip to avoid double-match.
		return ""
	}
	for _, id := range orderedIDs {
		s := synthesised[id]
		if s.parentID != parentID {
			continue
		}
		styleProps := parseRunPropElements([]byte("<w:rPr>" + s.rPrXML + "</w:rPr>"))
		if len(styleProps) != len(common) {
			continue
		}
		// Walk style props in lockstep with common; rFonts compared via
		// augmented attribute equality, others via byte-equal xml.
		mismatch := false
		var styleRFontsAttrs []rfontsAttr
		for i, sp := range styleProps {
			cp := common[i]
			if sp.name != cp.name {
				mismatch = true
				break
			}
			if sp.name == "rFonts" {
				attrs, ok := parseRFontsAttrs(sp.xml)
				if !ok {
					mismatch = true
					break
				}
				styleRFontsAttrs = attrs
				continue // verified below after the loop
			}
			if sp.xml != cp.xml {
				mismatch = true
				break
			}
		}
		if mismatch || styleRFontsAttrs == nil {
			continue
		}
		// Style's rFonts attrs must equal `augmented` (the common rFonts
		// augmented with the pPr.rPr non-category attrs).
		if len(styleRFontsAttrs) != len(augmented) {
			continue
		}
		allMatch := true
		for _, a := range styleRFontsAttrs {
			v, ok := augmented[a.name]
			if !ok || v != a.value {
				allMatch = false
				break
			}
		}
		if !allMatch {
			continue
		}
		return id
	}
	return ""
}

// paragraphHasMergedBreakTextRun reports whether `src` (a single <w:p>
// element extent) holds exactly ONE non-field-content, non-break-only
// run that itself carries BOTH a `<w:br>` child AND a `<w:t>` child.
// This is the writer-merged shape that motivates findSupersetSynthStyle:
// upstream's RunBuilder treats `<w:br>` and `<w:t>` chunks as separate
// MarkupComponent / RunText body events with their own owning Run rPrs,
// so the upstream byte trace shows two distinct <w:r>...</w:r>
// envelopes; the native writer collapses them when emitting because the
// run-encoded fragment chain happens to land both chunks on a single
// emitted run.
//
// Both source-run flavours of `<w:br>` (line break, page break, column
// break per ECMA-376-1 §17.3.3.1 CT_Br) qualify — the merge is purely
// structural, independent of break type.
func paragraphHasMergedBreakTextRun(runs []rawRun, entries []runEntry, src []byte) bool {
	if len(runs) != 1 || len(entries) != 1 {
		return false
	}
	runBody := src[runs[0].start:runs[0].end]
	if !bytes.Contains(runBody, []byte("<w:br")) {
		return false
	}
	// Skip opaque-subtree text scan so a textbox-content <w:t> inside
	// a drawing-bearing run doesn't qualify (per the upstream Markup
	// body-chunk model — see extractRunTextSkippingOpaque docstring).
	if extractRunTextSkippingOpaque(runBody) == "" {
		return false
	}
	return true
}

// insertPStyle returns a new <w:pPr>...</w:pPr> block with
// <w:pStyle w:val="<id>"/> inserted as the FIRST child. Okapi places
// pStyle as the first child of pPr (per ParagraphBlockProperties.refine).
//
// If the existing pPr already has a pStyle ANYWHERE in its body it is
// REPLACED with the new one (Okapi's refine() overrides the
// paragraphStyle slot regardless of position). Per ECMA-376-1
// §17.3.1.26 (CT_PPr) <w:pStyle> is normally the first child, but
// real-world authoring tools occasionally emit it later (fixture
// 847-2.docx is the canonical case: its P3 source has
// `<w:pPr><w:rPr><w:b/></w:rPr><w:pStyle w:val="i1"/></w:pPr>` with
// pStyle as the SECOND child after rPr). Without this strip, the
// WSO post-pass leaves the original pStyle in place AND prepends the
// synthesised one, producing an invalid two-pStyle pPr.
func insertPStyle(src []byte, id string) []byte {
	// Self-closing <w:pPr/> — convert to open/close with pStyle child.
	if bytes.HasSuffix(bytes.TrimSpace(src), []byte("/>")) {
		// Find "/>" and replace.
		idx := bytes.LastIndex(src, []byte("/>"))
		if idx < 0 {
			return src
		}
		var b bytes.Buffer
		b.Write(src[:idx])
		b.WriteString(`><w:pStyle w:val="`)
		b.WriteString(id)
		b.WriteString(`"/></w:pPr>`)
		return b.Bytes()
	}
	// Open form — find start-tag end.
	startTagEnd := bytes.IndexByte(src, '>')
	if startTagEnd < 0 {
		return src
	}
	// Strip an existing <w:pStyle ...> child wherever it appears in
	// the body. The captured pPr may carry pStyle in either self-
	// closing form ("<w:pStyle w:val=\"...\"/>") OR open/close form
	// ("<w:pStyle w:val=\"...\"></w:pStyle>" — encoding/xml's
	// Decoder/Encoder cycle re-emits captureRawElement payloads in
	// the latter form even when the source was self-closing, which
	// exposes the strip-only-self-closing path as a #592 regression
	// for fixtures whose pPr was lifted into a synthesised pStyle by
	// the WSO post-pass).
	//
	// stripChildElement does a hard name-boundary check so we don't
	// match a longer element name that starts with "pStyle" (no such
	// element exists in WPML, but the guard costs nothing).
	body := stripChildElement(src[startTagEnd+1:], "w:pStyle")
	var b bytes.Buffer
	b.Write(src[:startTagEnd+1])
	b.WriteString(`<w:pStyle w:val="`)
	b.WriteString(id)
	b.WriteString(`"/>`)
	b.Write(body)
	return b.Bytes()
}

// stripChildElement removes the FIRST occurrence of a `<name ...>...
// </name>` (open/close) or `<name .../>` (self-closing) child element
// from a fragment of WPML XML. Surrounding whitespace runs (immediately
// preceding AND immediately following the element) are collapsed so
// the fragment does not accumulate empty gaps; when whitespace existed
// on both sides the leading whitespace run is retained as a single
// separator between the surviving siblings.
//
// Used by insertPStyle to drop an existing <w:pStyle> regardless of
// its position in the pPr's child sequence — see insertPStyle for the
// ECMA-376 / fixture rationale.
func stripChildElement(body []byte, name string) []byte {
	prefix := append([]byte("<"), name...)
	idx := bytes.Index(body, prefix)
	for idx >= 0 {
		end := idx + len(prefix)
		if end >= len(body) {
			return body
		}
		b := body[end]
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '/' && b != '>' {
			next := bytes.Index(body[end:], prefix)
			if next < 0 {
				return body
			}
			idx = end + next
			continue
		}
		tagEnd := bytes.IndexByte(body[end:], '>')
		if tagEnd < 0 {
			return body
		}
		absTagEnd := end + tagEnd
		var endOfElem int
		if absTagEnd > 0 && body[absTagEnd-1] == '/' {
			endOfElem = absTagEnd + 1
		} else {
			closeNeedle := append([]byte("</"), name...)
			closeNeedle = append(closeNeedle, '>')
			closeIdx := bytes.Index(body[absTagEnd+1:], closeNeedle)
			if closeIdx < 0 {
				return body
			}
			endOfElem = absTagEnd + 1 + closeIdx + len(closeNeedle)
		}
		wsBefore := idx
		for wsBefore > 0 {
			c := body[wsBefore-1]
			if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
				break
			}
			wsBefore--
		}
		wsAfter := endOfElem
		for wsAfter < len(body) {
			c := body[wsAfter]
			if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
				break
			}
			wsAfter++
		}
		out := make([]byte, 0, len(body)-(wsAfter-wsBefore))
		out = append(out, body[:wsBefore]...)
		if wsBefore != idx && wsAfter != endOfElem {
			out = append(out, body[wsBefore:idx]...)
		}
		out = append(out, body[wsAfter:]...)
		return out
	}
	return body
}

// stripPropsFromRun removes named property elements from the <w:rPr>
// inside a <w:r>...</w:r> block. If the resulting rPr is empty, the
// rPr container itself is removed (matching the
// wmlEmptyPropertiesContainerRE post-pass).
func stripPropsFromRun(runSrc []byte, names map[string]bool) []byte {
	rps, rpe, has := findFirstChild(runSrc, "rPr")
	if !has {
		return runSrc
	}
	rPrSrc := runSrc[rps:rpe]
	props := parseRunPropElements(rPrSrc)
	// Strip only the FIRST occurrence per matching name. Mirrors
	// upstream Okapi RunProperties.refine
	// (RunProperties.java :240-260) which removes properties from the
	// run's rPr by Property.equals against commonRunProperties — each
	// property is removed once, not all instances. Source documents
	// occasionally author the same property element twice in a single
	// run rPr (e.g. content_category_test.docx authors `<w:sz w:val=
	// "32"/>` twice in the Arabic run); stripping ALL instances when
	// commonNames lifts ONE into the synth pStyle drops the surviving
	// duplicate that upstream keeps, costing a per-run rPr child on
	// the wire. Per ECMA-376-1 §17.3.2 the toggle/value-bearing
	// elements are well-defined as single-instance per <w:rPr>; the
	// duplicate in source is malformed but Okapi preserves it
	// faithfully, so native must too.
	stripBudget := make(map[string]int, len(names))
	for n := range names {
		stripBudget[n] = 1
	}
	var kept []runProp
	for _, p := range props {
		if stripBudget[p.name] > 0 {
			stripBudget[p.name]--
			continue
		}
		kept = append(kept, p)
	}
	var newRPr bytes.Buffer
	if len(kept) == 0 {
		// Remove rPr entirely.
		var out bytes.Buffer
		out.Write(runSrc[:rps])
		out.Write(runSrc[rpe:])
		return out.Bytes()
	}
	// Re-emit rPr with kept props, preserving the original opening tag
	// (which may carry namespace declarations).
	openEnd := bytes.IndexByte(rPrSrc, '>')
	if openEnd < 0 {
		return runSrc
	}
	newRPr.Write(rPrSrc[:openEnd+1])
	for _, p := range kept {
		newRPr.WriteString(p.xml)
	}
	newRPr.WriteString(`</w:rPr>`)
	var out bytes.Buffer
	out.Write(runSrc[:rps])
	out.Write(newRPr.Bytes())
	out.Write(runSrc[rpe:])
	return out.Bytes()
}

// stripWMLNamesFromProps returns a new []runProp with every entry
// whose `name` matches one of the supplied names removed. Used by
// optimizeParagraph to drop b/i from CS-only runs before WSO computes
// the common props (so they don't get lifted into the synthesised
// paragraph style and don't survive on the run after the strip rewrite).
func stripWMLNamesFromProps(props []runProp, names ...string) []runProp {
	if len(props) == 0 || len(names) == 0 {
		return props
	}
	drop := make(map[string]bool, len(names))
	for _, n := range names {
		drop[n] = true
	}
	out := props[:0:0]
	for _, p := range props {
		if drop[p.name] {
			continue
		}
		out = append(out, p)
	}
	return out
}

// extractRunText concatenates the character data inside every <w:t>...
// </w:t> child of the given <w:r>...</w:r> source bytes. Returns the
// empty string when the run has no <w:t> child or its text is empty.
//
// Mirrors upstream Okapi Run.text (Run.java:99-107) which feeds the
// run's effective text to ContentCategoriesDetection.performFor. The
// detection runs against the run's TEXT only — non-text run children
// (<w:tab/>, <w:br/>, <w:drawing>, ...) don't classify as content
// categories. Native applies the strip on the WSO-rewrite pass, so
// we extract from the EMITTED run bytes (post-render) which is the
// post-pseudo text the upstream filter would also see.
//
// XML entity references inside <w:t> (`&amp;`, `&#x...;`) are passed
// through verbatim — the strip's CS-detection only inspects characters
// in dedicated Unicode ranges and treats unknowns as non-CS, so a stray
// "&amp;" in the run text will correctly mark it as containing non-CS
// content (the literal "&" is ASCII).
// runIsDrawingOnly reports whether runSrc contains a `<w:drawing>`,
// `<w:pict>`, `<w:object>`, `<mc:AlternateContent>` body and NO
// `<w:t>` text element. Used by optimizeParagraph to recognise
// drawing-only runs whose rPr toggle properties (b/i) don't affect
// rendering and should be stripped before the common-prop pass.
//
// Mirrors upstream Okapi's RunBuilder MarkupComponent path: a run
// holding only opaque markup components contributes those components
// to the Block's chunk list but does NOT promote rendering toggles
// into the WSO commonRunProperties view. Per ECMA-376-1 §17.3.2.1
// (CT_R) the rPr applies to the run's TEXT children; for a run with
// no text, properties like `<w:b>`/`<w:i>`/`<w:u>` are no-ops at
// render time.
func runIsDrawingOnly(runSrc []byte) bool {
	hasDrawing := bytes.Contains(runSrc, []byte("<w:drawing")) ||
		bytes.Contains(runSrc, []byte("<w:pict")) ||
		bytes.Contains(runSrc, []byte("<w:object")) ||
		bytes.Contains(runSrc, []byte("<mc:AlternateContent"))
	if !hasDrawing {
		return false
	}
	if extractRunTextSkippingOpaque(runSrc) != "" {
		return false
	}
	return true
}

// runIsBreakOnly reports whether runSrc carries a `<w:br/>` element and
// has no `<w:t>` text (and no opaque drawing/pict/object content). Used
// by optimizeParagraph to recognise standalone-break runs whose rPr is
// invisible to upstream Okapi's WordStyleOptimisation common-rPr lift.
//
// Mirrors upstream Okapi's RunBuilder + WordStyleOptimisation behaviour:
// a `<w:br>` arrives as a Markup body chunk (RunParser.java:752-766
// addToMarkup path) of an outer Run whose RunProperties come from the
// surrounding text-bearing rPr context. When the source authors a
// dedicated `<w:r><w:rPr>...</w:rPr><w:br/></w:r>` envelope (no `<w:t>`
// in the same run), the upstream pipeline does NOT promote the br run's
// rPr into commonRunPropertiesOf — the break is rendering-neutral with
// respect to character formatting per ECMA-376-1 §17.3.3.1 (CT_Br) and
// §17.3.2.1 (CT_R), so the br-run's rPr does not contribute to the
// paragraph-wide common rPr that WSO lifts into a synthesised pStyle.
//
// EndGroup.docx is the canonical fixture: a paragraph with runs
//
//	<w:r><w:rPr><w:rFonts hint="eastAsia"/><w:szCs val="21"/></w:rPr><w:t>...</w:t></w:r>
//	<w:r><w:rPr><w:szCs val="21"/></w:rPr><w:br/></w:r>
//	<w:r><w:rPr><w:rFonts hint="eastAsia"/><w:szCs val="21"/></w:rPr><w:t>...</w:t></w:r>
//
// Including the br run in the common-rPr computation narrows the
// intersection to `<w:szCs val="21"/>` (the br-run lacks rFonts), so
// native synthesises a fresh `Normal2` style with just szCs while
// bridge re-uses `Normal1` (rFonts+szCs). Excluding the br run lets the
// remaining text-runs' rFonts hint = eastAsia survive in the common,
// matching upstream's lift.
func runIsBreakOnly(runSrc []byte) bool {
	if !bytes.Contains(runSrc, []byte("<w:br")) {
		return false
	}
	// Reject runs that ALSO carry text or opaque content — the br is
	// then a sibling chunk of text in the SAME source <w:r>, and its
	// rPr is the rPr of the text-bearing run too.
	if extractRunTextSkippingOpaque(runSrc) != "" {
		return false
	}
	if bytes.Contains(runSrc, []byte("<w:drawing")) ||
		bytes.Contains(runSrc, []byte("<w:pict")) ||
		bytes.Contains(runSrc, []byte("<w:object")) ||
		bytes.Contains(runSrc, []byte("<mc:AlternateContent")) {
		return false
	}
	return true
}

// extractRunTextSkippingOpaque returns the concatenated text of every
// `<w:t>` that is a DIRECT child of runSrc — i.e. not nested inside a
// `<w:drawing>` / `<w:pict>` / `<w:object>` / `<mc:AlternateContent>`
// opaque subtree. Mirrors upstream Okapi's RunBuilder which treats
// opaque MarkupComponent payloads as black boxes whose internal
// `<w:t>` (e.g. inside a textbox `<w:txbxContent>`) belongs to a
// nested StyledTextPart, not to the outer run. Per ECMA-376-1
// §17.3.2.1 (CT_R), the run's direct text children are `<w:t>`,
// `<w:delText>`, `<w:instrText>` etc.; an opaque subtree's text
// belongs to its own scope.
//
// Without this skip, AlternateContentTest.docx's textbox-bearing run
// is mis-classified as text-bearing (the inner `<w:txbxContent>`
// paragraph carries `<w:t>Text2</w:t>` that the simple
// `extractRunText` scan finds), defeating the drawing-only b/i
// strip and letting `<w:i/>` leak into the synth Style12's lifted
// rPr.
func extractRunTextSkippingOpaque(runSrc []byte) string {
	opaqueOpens := []string{"<w:drawing", "<w:pict", "<w:object", "<mc:AlternateContent"}
	var out strings.Builder
	i := 0
	for i < len(runSrc) {
		// Find the next opaque-subtree start OR the next <w:t>.
		// Whichever comes first wins; if opaque, skip past it.
		nextOpaqueAbs := -1
		var matchedTag string
		for _, tag := range opaqueOpens {
			ti := bytes.Index(runSrc[i:], []byte(tag))
			if ti < 0 {
				continue
			}
			// Element-name boundary.
			bj := i + ti + len(tag)
			if bj >= len(runSrc) {
				continue
			}
			bb := runSrc[bj]
			if bb != '>' && bb != ' ' && bb != '\t' && bb != '\n' && bb != '\r' && bb != '/' {
				continue
			}
			abs := i + ti
			if nextOpaqueAbs < 0 || abs < nextOpaqueAbs {
				nextOpaqueAbs = abs
				matchedTag = tag
			}
		}
		nextTAbs := -1
		for k := i; k < len(runSrc); {
			ti := bytes.Index(runSrc[k:], []byte("<w:t"))
			if ti < 0 {
				break
			}
			abs := k + ti
			j := abs + len("<w:t")
			if j >= len(runSrc) {
				break
			}
			b := runSrc[j]
			// Element-name boundary — accept "<w:t " / "<w:t>" /
			// "<w:t/>" but reject "<w:tab" or "<w:tbl".
			if b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '>' && b != '/' {
				k = j
				continue
			}
			nextTAbs = abs
			break
		}
		if nextTAbs < 0 && nextOpaqueAbs < 0 {
			break
		}
		if nextOpaqueAbs >= 0 && (nextTAbs < 0 || nextOpaqueAbs < nextTAbs) {
			// Skip past the opaque subtree (balanced close).
			openTagEnd := bytes.IndexByte(runSrc[nextOpaqueAbs:], '>')
			if openTagEnd < 0 {
				break
			}
			openTagAbsEnd := nextOpaqueAbs + openTagEnd
			// Self-closing.
			if openTagAbsEnd > 0 && runSrc[openTagAbsEnd-1] == '/' {
				i = openTagAbsEnd + 1
				continue
			}
			closeName := []byte("</" + matchedTag[1:] + ">")
			depth := 1
			inner := openTagAbsEnd + 1
			for depth > 0 && inner < len(runSrc) {
				no := bytes.Index(runSrc[inner:], []byte(matchedTag))
				nc := bytes.Index(runSrc[inner:], closeName)
				if nc < 0 {
					return out.String()
				}
				if no >= 0 && no < nc {
					// Confirm boundary.
					bjj := inner + no + len(matchedTag)
					if bjj < len(runSrc) {
						bbb := runSrc[bjj]
						if bbb == '>' || bbb == ' ' || bbb == '\t' || bbb == '\n' || bbb == '\r' || bbb == '/' {
							// Self-closing inner same-name open?
							ote := bytes.IndexByte(runSrc[inner+no:], '>')
							if ote > 0 && runSrc[inner+no+ote-1] == '/' {
								inner = inner + no + ote + 1
								continue
							}
							depth++
							inner = inner + no + len(matchedTag)
							continue
						}
					}
					inner = inner + no + len(matchedTag)
					continue
				}
				depth--
				inner = inner + nc + len(closeName)
			}
			i = inner
			continue
		}
		// Process the <w:t> at nextTAbs.
		j := nextTAbs + len("<w:t")
		tagEnd := bytes.IndexByte(runSrc[j:], '>')
		if tagEnd < 0 {
			break
		}
		absTagEnd := j + tagEnd
		if absTagEnd > 0 && runSrc[absTagEnd-1] == '/' {
			// Self-closing <w:t/> — no content.
			i = absTagEnd + 1
			continue
		}
		closeIdx := bytes.Index(runSrc[absTagEnd+1:], []byte("</w:t>"))
		if closeIdx < 0 {
			break
		}
		out.Write(runSrc[absTagEnd+1 : absTagEnd+1+closeIdx])
		i = absTagEnd + 1 + closeIdx + len("</w:t>")
	}
	return out.String()
}

func extractRunText(runSrc []byte) string {
	var out strings.Builder
	cursor := 0
	for {
		idx := bytes.Index(runSrc[cursor:], []byte("<w:t"))
		if idx < 0 {
			break
		}
		start := cursor + idx
		j := start + len("<w:t")
		if j >= len(runSrc) {
			break
		}
		// Element-name boundary check — accept "<w:t " / "<w:t>" /
		// "<w:t/>" but reject "<w:tab" or "<w:tbl".
		b := runSrc[j]
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '>' && b != '/' {
			cursor = j
			continue
		}
		// Find element-tag end.
		tagEnd := bytes.IndexByte(runSrc[j:], '>')
		if tagEnd < 0 {
			break
		}
		absTagEnd := j + tagEnd
		// Self-closing <w:t/> — no content.
		if absTagEnd > 0 && runSrc[absTagEnd-1] == '/' {
			cursor = absTagEnd + 1
			continue
		}
		closeIdx := bytes.Index(runSrc[absTagEnd+1:], []byte("</w:t>"))
		if closeIdx < 0 {
			break
		}
		out.Write(runSrc[absTagEnd+1 : absTagEnd+1+closeIdx])
		cursor = absTagEnd + 1 + closeIdx + len("</w:t>")
	}
	return out.String()
}

// textIsAllComplexScript reports whether s is non-empty AND every
// non-whitespace character classifies as complex-script per upstream
// Okapi's ContentCategoriesDetection (no detected ASCII / HighAnsi /
// EastAsian / Symbols / Shared categories). Mirrors
// `!runFonts.containsDetectedNonComplexScriptContentCategories()` from
// RunParser.endRunParsing (RunParser.java:208) — the gate for the
// symmetric strip of b/i (and sz, value-permitting) from purely
// complex-script runs.
//
// The CS character ranges follow the same inventory as
// containsComplexScriptText (writer.go), derived from
// ContentCategoriesDetection.java:71-74 + Microsoft's "Office Open XML
// Themes, Schemes and Fonts" guidance referenced by ECMA-376-1
// §17.3.2.16 / .17.
//
// Whitespace is treated as compatible with either side (the strip is
// safe on `" "` runs because Okapi's detection considers ASCII / Latin /
// CS independently, and a whitespace-only run has neither — which
// upstream treats as "no non-CS detected" too, so the strip fires).
func textIsAllComplexScript(s string) bool {
	if s == "" {
		return false
	}
	hasCS := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		if isComplexScriptRune(r) {
			hasCS = true
			continue
		}
		// Any non-whitespace, non-CS character disqualifies the run.
		return false
	}
	return hasCS
}

// isComplexScriptRune reports whether r belongs to one of the Unicode
// ranges upstream Okapi's ContentCategoriesDetection classifies as
// complex-script. Mirrors containsComplexScriptText in writer.go.
func isComplexScriptRune(r rune) bool {
	switch {
	case r >= 0x0590 && r <= 0x074F: // Hebrew, Arabic, Syriac, …
		return true
	case r >= 0x0780 && r <= 0x07BF: // Thaana
		return true
	case r >= 0x0900 && r <= 0x109F: // Devanagari … Myanmar
		return true
	case r >= 0x1780 && r <= 0x18AF: // Khmer … Mongolian
		return true
	case r >= 0x200C && r <= 0x200F: // ZWJ / ZWNJ / LRM / RLM
		return true
	case r >= 0x202A && r <= 0x202F: // bidi formatting + NNBSP
		return true
	case r >= 0x2670 && r <= 0x2671: // misc symbols
		return true
	case r >= 0xFB1D && r <= 0xFB4F: // Hebrew presentation forms
		return true
	}
	return false
}

// injectSynthesisedStyles inserts synthesised <w:style> elements into
// word/styles.xml just before the closing </w:styles> tag. Order is
// the orderedIDs slice (insertion order).
//
// Mirrors WordStyleDefinitions.asMarkup (lines 429-443) — synthesised
// styles append to the end of the styles list as their place() calls
// occur.
func injectSynthesisedStyles(stylesXML []byte, synthesised map[string]synthesisedStyle, orderedIDs []string) []byte {
	if len(orderedIDs) == 0 {
		return stylesXML
	}
	closeTag := []byte("</w:styles>")
	idx := bytes.LastIndex(stylesXML, closeTag)
	if idx < 0 {
		return stylesXML
	}
	var inj bytes.Buffer
	for _, id := range orderedIDs {
		s := synthesised[id]
		inj.WriteString(`<w:style w:type="paragraph" w:styleId="`)
		inj.WriteString(s.id)
		inj.WriteString(`"><w:name w:val="`)
		inj.WriteString(s.id)
		inj.WriteString(`"/><w:basedOn w:val="`)
		inj.WriteString(s.parentID)
		inj.WriteString(`"/>`)
		if s.rPrXML != "" {
			inj.WriteString(`<w:rPr>`)
			inj.WriteString(s.rPrXML)
			inj.WriteString(`</w:rPr>`)
		}
		inj.WriteString(`</w:style>`)
	}
	var out bytes.Buffer
	out.Grow(len(stylesXML) + inj.Len())
	out.Write(stylesXML[:idx])
	out.Write(inj.Bytes())
	out.Write(stylesXML[idx:])
	return out.Bytes()
}

// extractDefaultParagraphStyleID scans word/styles.xml for the default
// paragraph style — the
// `<w:style w:type="paragraph" w:default="1" w:styleId="X">` element —
// and returns "X". Returns "" if no such element is present.
//
// Mirrors WordStyleDefinitions.place (line 138-145) which builds the
// defaultStylesByStyleTypes map keyed on (StyleType.PARAGRAPH ->
// styleId). That map is consulted by Ids.defaultBased
// (WordStyleDefinitions.java:485-491) when a paragraph lacks pStyle —
// the styleId becomes the parent of the synthesised style and feeds
// IdGenerator.createId(parentId).
func extractDefaultParagraphStyleID(stylesXML []byte) string {
	cursor := 0
	openNeedle := []byte("<w:style")
	for {
		idx := bytes.Index(stylesXML[cursor:], openNeedle)
		if idx < 0 {
			return ""
		}
		start := cursor + idx
		j := start + len(openNeedle)
		if j >= len(stylesXML) {
			return ""
		}
		// Element-name boundary check.
		if b := stylesXML[j]; b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '>' && b != '/' {
			cursor = j
			continue
		}
		end := bytes.IndexByte(stylesXML[j:], '>')
		if end < 0 {
			return ""
		}
		tag := stylesXML[j : j+end]
		hasType := bytes.Contains(tag, []byte(`w:type="paragraph"`))
		hasDefault := bytes.Contains(tag, []byte(`w:default="1"`)) || bytes.Contains(tag, []byte(`w:default="true"`))
		if hasType && hasDefault {
			styleIDIdx := bytes.Index(tag, []byte(`w:styleId="`))
			if styleIDIdx >= 0 {
				vstart := styleIDIdx + len(`w:styleId="`)
				vend := bytes.IndexByte(tag[vstart:], '"')
				if vend > 0 {
					return string(tag[vstart : vstart+vend])
				}
			}
		}
		cursor = j + end + 1
	}
}

// extractExistingStyleIDs scans word/styles.xml for every
// w:styleId="..." attribute and returns the set. This is consulted
// during id generation to avoid colliding with a pre-existing styleId.
func extractExistingStyleIDs(stylesXML []byte) map[string]bool {
	out := make(map[string]bool)
	cursor := 0
	needle := []byte(`w:styleId="`)
	for {
		idx := bytes.Index(stylesXML[cursor:], needle)
		if idx < 0 {
			break
		}
		start := cursor + idx + len(needle)
		end := bytes.IndexByte(stylesXML[start:], '"')
		if end < 0 {
			break
		}
		out[string(stylesXML[start:start+end])] = true
		cursor = start + end + 1
	}
	return out
}

// _ keeps sort imported for future use (sorted-id traversal).
var _ = sort.Strings

// currentRTLChainStyles is set by the writer (writer.go) to the result
// of extractRTLChainStyleIDs(stylesXML) before invoking optimizeWMLPart
// on each WML part. It is consulted by stripToggleMirrorsFromCommon's
// rtl-clearing-form preservation branch to decide whether an explicit-
// off `<w:rtl w:val="0"/>` lifted into a synthesised paragraph style
// must be PRESERVED (parent chain inherits `<w:rtl/>`) or DROPPED as
// structurally redundant (parent chain has no rtl).
//
// Module-level state instead of a threaded parameter so the existing
// optimizeWMLPart / optimizeNestedParagraphs / optimizeParagraph call
// sites in style_optimization_test.go keep their current 8-argument
// signatures. The Writer always resets this to nil after each
// per-Writer WSO pass via a deferred cleanup; tests that run
// optimizeWMLPart directly leave it nil (treated as "no parent
// inherits rtl", preserving the pre-fix drop behaviour for in-test
// fixtures whose styles.xml has no rtl-bearing chain).
var currentRTLChainStyles map[string]bool

// currentDocDefaultsBCsExplicitOff reports whether the source
// styles.xml docDefaults rPr authors `<w:bCs w:val="0"/>` (or
// "false" / "off"). Consulted by stripToggleMirrorsFromCommon and
// writer.go's stripToggleMirrorChildren to decide whether a run-level
// bare-on `<w:bCs/>` must be preserved (because preCombined's bCs
// value differs from the run's, so upstream RunParser.canBeSkipped at
// RunParser.java:240-250 refuses the strip).
//
// Module-level state instead of threaded parameter for the same
// reason as currentRTLChainStyles — see its doc above for the
// rationale on signature stability.
var currentDocDefaultsBCsExplicitOff bool

// currentDocDefaultsICsExplicitOff is the iCs counterpart of
// currentDocDefaultsBCsExplicitOff. See its doc for the rationale.
var currentDocDefaultsICsExplicitOff bool

// currentDefaultCharacterStyleToggles is the set of toggle property
// names (from {b, i, strike, outline, shadow, vanish, caps, smallCaps,
// emboss, imprint, …}) authored as bare-on form by the default
// character style — the
// `<w:style w:type="character" w:default="true">` element in
// styles.xml. Per ECMA-376-1 §17.7.4 the default character style
// applies implicitly to runs that don't author rStyle, so any toggle
// it asserts is part of every run's preCombined view. The WSO common-
// rPr lift must skip those toggles to avoid emitting redundant
// `<w:b/>` / `<w:i/>` / etc on the synthesised paragraph style — the
// inherited default character style already supplies them.
//
// Mirrors upstream Okapi
// WordStyleDefinitions.combinedRunProperties (lines 302-315) which
// folds the default character style into preCombined when the run has
// no rStyle, and RunProperties.minified() (RunProperties.java:497-540)
// which drops a duplicate property whose Property.equals matches the
// preCombined value.
//
// document-style-definitions.docx canonical: source has
// `<w:style w:type="character" w:default="true" w:styleId="Emphasis">`
// with `<w:i/><w:iCs/><w:color val="0000FF"/>`. Runs that author
// `<w:i/>` directly are emitting a value that the implicit Emphasis
// chain already supplies; upstream Okapi's commonRunProperties pass
// drops the redundant `<w:i/>` from the WSO common-rPr lift. The set
// is populated by the writer's WSO pre-pass via
// extractDefaultCharacterStyleToggles.
var currentDefaultCharacterStyleToggles map[string]bool

// currentDocDefaultsRFonts holds the attribute list of the rFonts
// element authored by `<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts
// .../></w:rPr></w:rPrDefault></w:docDefaults>` in word/styles.xml.
//
// Per ECMA-376-1 §17.7.5.5 (CT_DocDefaults) the docDefaults rPr
// applies implicitly to every run that doesn't override the property
// through pStyle / rStyle / direct formatting. When upstream Okapi's
// RunMerger fuses adjacent runs whose direct rFonts disagree but whose
// combined (docDefaults-overlaid) rFonts agree per
// `RunFonts.canContentCategoriesBeMerged` (RunFonts.java:211-230) and
// `mergeContentCategories` (RunFonts.java:299-315), the merged run's
// direct rFonts captures the combined view. WSO then sees ONE run
// whose direct rFonts is the merged value and trivially lifts it into
// a synthesised pStyle.
//
// Native does NOT run RunMerger at parse time; runs reach WSO with
// their source-direct rFonts intact (or absent). When SOME runs in a
// paragraph carry rFonts directly and OTHERS author no rFonts,
// `commonRFonts`'s per-attribute intersection collapses (the run
// without rFonts has no attributes to intersect with), and the
// synthesis path bails out.
//
// To approximate upstream's RunMerger-equivalent lift in that case,
// `commonRFonts` virtually overlays this docDefaults rFonts onto runs
// that lack a direct rFonts. The intersection then runs against the
// docDefaults-supplied attributes — matching the values RunMerger
// would have surfaced after `clarifyFontsRunProperties`
// (RunMerger.java:389-396) refines the merged run's direct rFonts to
// the merged combined view.
//
// Populated as a side effect of extractDefaultCharacterStyleToggles
// (which writer.go already invokes with stylesXML at the WSO
// pre-pass entry — see writer.go:1494). The piggyback keeps the
// docDefaults rFonts lifecycle aligned with the other docDefaults-
// derived state (currentDocDefaultsBCsExplicitOff, etc.) without
// requiring an additional plumbing call. Nil/empty when the source
// has no docDefaults rFonts.
//
// Fixture StartsWithLineSeparator.docx: docDefaults declares
//
//	<w:rFonts w:asciiTheme="minorHAnsi" w:eastAsiaTheme="minorHAnsi"
//	          w:hAnsiTheme="minorHAnsi" w:cstheme="minorBidi"/>
//
// Paragraph 1 has R1=`<w:rPr><w:rFonts ascii=MS Gothic eastAsia=MS
// Gothic hAnsi=MS Gothic cs=MS Gothic/></w:rPr>` followed by R2-R4
// with empty rPr. Upstream RunMerger fuses R1+R2-R4, surfaces a
// merged rFonts that includes `ascii=MS Gothic, asciiTheme=minorHAnsi,
// hAnsi=MS Gothic, hAnsiTheme=minorHAnsi` (via
// `mergeContentCategories` + `preserveAsciiAndHighAnsi`), then
// `commonRunPropertiesOf` lifts the merged direct rFonts into a
// synthesised `NF974E24F-Standard1` pStyle. Without the docDefaults
// overlay, native's `commonRFonts` would short-circuit because R2-R4
// lack rFonts, and no synth pStyle would be emitted.
//
// Module-level state instead of threaded parameter for the same
// reason as currentRTLChainStyles — see its doc above for the
// rationale on signature stability.
var currentDocDefaultsRFonts []rfontsAttr

// extractDocDefaultsRFontsAttrs returns the parsed attribute list of
// the `<w:rFonts ...>` element that lives directly under
// `<w:docDefaults><w:rPrDefault><w:rPr>` in stylesXML. Returns nil if
// no such element exists.
//
// Used by extractDefaultCharacterStyleToggles to refresh
// currentDocDefaultsRFonts as a piggyback side effect at the WSO
// pre-pass entry. See currentDocDefaultsRFonts doc for the upstream
// contract reference and the StartsWithLineSeparator.docx fixture
// rationale.
//
// Per ECMA-376-1 §17.7.5.5 (CT_DocDefaults) only one
// `<w:rPrDefault>` may appear and at most one `<w:rFonts>` inside it.
// The parse mirrors extractDocDefaultsToggleExplicitOff's byte-scan
// approach — keeping the cost cheap and aligned with the rest of the
// stylesXML parsing in this file.
func extractDocDefaultsRFontsAttrs(stylesXML []byte) []rfontsAttr {
	if len(stylesXML) == 0 {
		return nil
	}
	docDefaultsStart := bytes.Index(stylesXML, []byte("<w:docDefaults"))
	if docDefaultsStart < 0 {
		return nil
	}
	docDefaultsEnd := bytes.Index(stylesXML[docDefaultsStart:], []byte("</w:docDefaults>"))
	if docDefaultsEnd < 0 {
		return nil
	}
	body := stylesXML[docDefaultsStart : docDefaultsStart+docDefaultsEnd]
	rPrDefaultStart := bytes.Index(body, []byte("<w:rPrDefault"))
	if rPrDefaultStart < 0 {
		return nil
	}
	rPrDefaultEnd := bytes.Index(body[rPrDefaultStart:], []byte("</w:rPrDefault>"))
	if rPrDefaultEnd < 0 {
		return nil
	}
	rPrBody := body[rPrDefaultStart : rPrDefaultStart+rPrDefaultEnd]
	// Locate the rFonts element (self-closing or open form).
	rfontsStart := bytes.Index(rPrBody, []byte("<w:rFonts"))
	if rfontsStart < 0 {
		return nil
	}
	// Find the end of the rFonts start-tag (`/>` for self-closing or
	// `>` for open form; both are accepted by parseRFontsAttrs).
	rfontsTagEnd := bytes.IndexByte(rPrBody[rfontsStart:], '>')
	if rfontsTagEnd < 0 {
		return nil
	}
	rfontsXML := string(rPrBody[rfontsStart : rfontsStart+rfontsTagEnd+1])
	attrs, ok := parseRFontsAttrs(rfontsXML)
	if !ok {
		return nil
	}
	return attrs
}

// styleHasRTLDirect reports whether the named styleID's rPr (in
// stylesXML) has a bare-on `<w:rtl/>` element (i.e. NOT explicit-off
// `<w:rtl w:val="0"/>`). Used by extractRTLChainStyleIDs to seed the
// chain walk.
//
// The match is local to ONE `<w:style w:styleId="X">` entry — it does
// NOT walk basedOn (the caller does the chain walk). Returns false for
// styles whose rPr authors `<w:rtl w:val="0"/>`/"false"/"off" (those
// are clearing forms, not bare-on per ECMA-376-1 §17.3.2.4 CT_OnOff).
func styleHasRTLDirect(stylesXML []byte, styleID string) bool {
	needle := []byte(`w:styleId="` + styleID + `"`)
	idx := bytes.Index(stylesXML, needle)
	if idx < 0 {
		return false
	}
	end := bytes.Index(stylesXML[idx:], []byte(`</w:style>`))
	if end < 0 {
		return false
	}
	body := stylesXML[idx : idx+end]
	for cursor := 0; cursor < len(body); {
		i := bytes.Index(body[cursor:], []byte("<w:rtl"))
		if i < 0 {
			break
		}
		start := cursor + i + len("<w:rtl")
		if start >= len(body) {
			break
		}
		b := body[start]
		if b != ' ' && b != '/' && b != '>' && b != '\t' && b != '\n' && b != '\r' {
			cursor = start
			continue
		}
		te := bytes.IndexByte(body[start:], '>')
		if te < 0 {
			break
		}
		tag := body[start : start+te]
		if bytes.Contains(tag, []byte(`w:val="0"`)) ||
			bytes.Contains(tag, []byte(`w:val="false"`)) ||
			bytes.Contains(tag, []byte(`w:val="off"`)) {
			cursor = start + te + 1
			continue
		}
		return true
	}
	return false
}

// styleBasedOn returns the basedOn value for the named styleID in
// stylesXML, or "" if not found / no basedOn declared.
func styleBasedOn(stylesXML []byte, styleID string) string {
	needle := []byte(`w:styleId="` + styleID + `"`)
	idx := bytes.Index(stylesXML, needle)
	if idx < 0 {
		return ""
	}
	end := bytes.Index(stylesXML[idx:], []byte(`</w:style>`))
	if end < 0 {
		return ""
	}
	body := stylesXML[idx : idx+end]
	bi := bytes.Index(body, []byte(`<w:basedOn w:val="`))
	if bi < 0 {
		return ""
	}
	bi += len(`<w:basedOn w:val="`)
	be := bytes.IndexByte(body[bi:], '"')
	if be < 0 {
		return ""
	}
	return string(body[bi : bi+be])
}

// extractRTLChainStyleIDs returns the set of styleIDs in stylesXML
// whose chain (own rPr + basedOn-walked ancestors) carries a bare-on
// `<w:rtl/>` toggle. Consumed by stripToggleMirrorsFromCommon to
// decide when an explicit-off `<w:rtl w:val="0"/>` lifted into a
// synthesised paragraph style must be PRESERVED — without it, the
// synth style fails to clear the inherited rtl from the parent style
// chain, producing right-aligned text where the source authored a
// left-aligned LTR override.
//
// 899.docx canonical case: the Normal paragraph style declares
// `<w:rPr><w:rtl/></w:rPr>` (the document defaults to RTL). Every LTR
// paragraph carries a per-run `<w:rtl w:val="0"/>` clearing override;
// WSO lifts the common clearing form into a synthesised `Normal1`
// style based on `Normal`. If the synthesised style drops `<w:rtl
// w:val="0"/>`, the inherited `<w:rtl/>` from `Normal` flows through
// and the LTR text renders RTL.
//
// 830-2.docx counterexample: Normal does NOT carry `<w:rtl/>`, so the
// per-run `<w:rtl w:val="0"/>` lift is structurally redundant (no
// inherited rtl to clear). The drop matches upstream output.
// extractDocDefaultsToggleExplicitOff reports whether
// <w:docDefaults><w:rPrDefault><w:rPr> in stylesXML authors an
// explicit-off form of `<w:NAME w:val="0"/>` (or "false" / "off").
// Used by the writer's WSO pre-pass to populate
// currentDocDefaultsBCsExplicitOff / currentDocDefaultsICsExplicitOff
// — consulted by stripToggleMirrorsFromCommon and the writer's per-
// run sidecar strip to decide whether a bare-on `<w:bCs/>` /
// `<w:iCs/>` must be preserved on the run/synth-pStyle (because the
// run-side value differs from the preCombined value coming from
// docDefaults, blocking upstream's RunParser.canBeSkipped strip).
//
// Per ECMA-376-1 §17.7.5.5 (CT_DocDefaults) the docDefaults rPr
// applies to every run unless overridden by a paragraph style /
// rStyle / direct formatting. A `<w:bCs w:val="0"/>` in docDefaults
// means the inherited bCs is "off"; a run's bare `<w:bCs/>` (val
// implicit true) is then a clearing-vs-bare-on disagreement that
// RunParser.canBeSkipped (RunParser.java:240-250) detects and
// refuses to strip.
//
// Fixture 992.docx: docDefaults declares
//   <w:bCs w:val="0"/><w:iCs w:val="0"/>
// against which every run that authors a bare `<w:bCs/>` / `<w:iCs/>`
// must preserve the toggle mirror on the per-run rPr AND on any
// synthesised paragraph style's lift.
//
// large-attribute.docx counter-example: docDefaults has NO bCs/iCs,
// so upstream's canBeSkipped takes the `else { v = true; }` branch
// and bCs/iCs ARE stripped — the docDefaults-flag-false path
// preserves that behaviour.
func extractDocDefaultsToggleExplicitOff(stylesXML []byte, name string) bool {
	if len(stylesXML) == 0 {
		return false
	}
	// Locate <w:docDefaults>...<w:rPrDefault>...<w:rPr>...</w:rPr>
	docDefaultsStart := bytes.Index(stylesXML, []byte("<w:docDefaults"))
	if docDefaultsStart < 0 {
		return false
	}
	docDefaultsEnd := bytes.Index(stylesXML[docDefaultsStart:], []byte("</w:docDefaults>"))
	if docDefaultsEnd < 0 {
		return false
	}
	body := stylesXML[docDefaultsStart : docDefaultsStart+docDefaultsEnd]
	rPrDefaultStart := bytes.Index(body, []byte("<w:rPrDefault"))
	if rPrDefaultStart < 0 {
		return false
	}
	rPrDefaultEnd := bytes.Index(body[rPrDefaultStart:], []byte("</w:rPrDefault>"))
	if rPrDefaultEnd < 0 {
		return false
	}
	rPrBody := body[rPrDefaultStart : rPrDefaultStart+rPrDefaultEnd]
	// Find <w:NAME ... w:val="..."/> with val ∈ {0, false, off}.
	prefix := []byte("<w:" + name + " ")
	for cursor := 0; cursor < len(rPrBody); {
		i := bytes.Index(rPrBody[cursor:], prefix)
		if i < 0 {
			return false
		}
		start := cursor + i
		end := bytes.IndexAny(rPrBody[start:], "/>")
		if end < 0 {
			return false
		}
		elem := string(rPrBody[start : start+end])
		// Check val attribute.
		if strings.Contains(elem, `w:val="0"`) || strings.Contains(elem, `w:val="false"`) || strings.Contains(elem, `w:val="off"`) {
			return true
		}
		cursor = start + end + 1
	}
	return false
}

// extractDefaultCharacterStyleToggles returns the set of bare-on
// toggle property names authored by the default character style in
// stylesXML — i.e. the
// `<w:style w:type="character" w:default="true">` element's rPr's
// children that match a known toggle name AND lack an explicit-off
// `w:val="0"/"false"/"off"` attribute.
//
// Per ECMA-376-1 §17.7.4 the default character style applies
// implicitly to every run that doesn't author rStyle. Its toggles
// flow into preCombined, so any direct on the run that matches one
// of them (Property.equals) is dropped by RunProperties.minified()
// (RunProperties.java:497-540) and never reaches the WSO common-rPr
// lift in the upstream output.
//
// Native consumes this set in the WSO common-rPr lift to drop
// duplicate toggles that the default character style already supplies
// — preserving byte-equality with upstream's synthesised pStyle's
// rPr.
//
// Toggle names recognised: b, i, strike, dstrike, outline, shadow,
// vanish, caps, smallCaps, emboss, imprint, rtl. The list mirrors
// ECMA-376-1 §17.3.2's CT_OnOff toggle properties; values that
// aren't in this set (rFonts, color, sz, lang, …) are not toggles
// and are not handled here.
func extractDefaultCharacterStyleToggles(stylesXML []byte) map[string]bool {
	// Piggyback-refresh the docDefaults rFonts state from the same
	// stylesXML. Writer.go owns this function's call lifecycle (see
	// writer.go:1494 — invoked once per Writer at the WSO pre-pass,
	// reset to nil in the deferred cleanup at writer.go:1520). By
	// piggybacking the rFonts extraction here, the currentDocDefaultsRFonts
	// state shares that lifecycle automatically — the explicit reset
	// at the start guarantees no leak across Writer invocations when
	// stylesXML is empty or lacks a docDefaults rFonts.
	//
	// See currentDocDefaultsRFonts doc for the upstream contract
	// (WordStyleDefinitions.combinedRunProperties / RunMerger.
	// clarifyFontsRunProperties) and the StartsWithLineSeparator.docx
	// fixture rationale that motivated the overlay.
	currentDocDefaultsRFonts = extractDocDefaultsRFontsAttrs(stylesXML)
	out := make(map[string]bool)
	if len(stylesXML) == 0 {
		return out
	}
	// Find the default character style entry.
	cursor := 0
	openNeedle := []byte("<w:style")
	for {
		idx := bytes.Index(stylesXML[cursor:], openNeedle)
		if idx < 0 {
			return out
		}
		start := cursor + idx
		j := start + len(openNeedle)
		if j >= len(stylesXML) {
			return out
		}
		if b := stylesXML[j]; b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '>' && b != '/' {
			cursor = j
			continue
		}
		end := bytes.IndexByte(stylesXML[j:], '>')
		if end < 0 {
			return out
		}
		tag := stylesXML[j : j+end]
		hasType := bytes.Contains(tag, []byte(`w:type="character"`))
		hasDefault := bytes.Contains(tag, []byte(`w:default="1"`)) || bytes.Contains(tag, []byte(`w:default="true"`))
		if !hasType || !hasDefault {
			cursor = j + end + 1
			continue
		}
		// Locate the style's <w:rPr>...</w:rPr> within the entry body.
		bodyEnd := bytes.Index(stylesXML[j+end+1:], []byte("</w:style>"))
		if bodyEnd < 0 {
			return out
		}
		body := stylesXML[j+end+1 : j+end+1+bodyEnd]
		rPrStart := bytes.Index(body, []byte("<w:rPr"))
		if rPrStart < 0 {
			return out
		}
		rPrEnd := bytes.Index(body[rPrStart:], []byte("</w:rPr>"))
		if rPrEnd < 0 {
			return out
		}
		rPrBody := body[rPrStart : rPrStart+rPrEnd]
		// Scan for known toggle children.
		toggleNames := []string{"b", "i", "strike", "dstrike", "outline", "shadow", "vanish", "caps", "smallCaps", "emboss", "imprint", "rtl"}
		for _, name := range toggleNames {
			// Match `<w:NAME ` (with attrs), `<w:NAME/`, or `<w:NAME>`.
			prefixes := [][]byte{
				[]byte("<w:" + name + " "),
				[]byte("<w:" + name + "/"),
				[]byte("<w:" + name + ">"),
			}
			for _, prefix := range prefixes {
				idx := bytes.Index(rPrBody, prefix)
				if idx < 0 {
					continue
				}
				// Found. Check if explicit-off (w:val="0"/"false"/"off").
				elemEnd := bytes.IndexByte(rPrBody[idx:], '>')
				if elemEnd < 0 {
					continue
				}
				elem := string(rPrBody[idx : idx+elemEnd])
				if strings.Contains(elem, `w:val="0"`) || strings.Contains(elem, `w:val="false"`) || strings.Contains(elem, `w:val="off"`) {
					// Explicit-off — does NOT contribute to preCombined as on.
					continue
				}
				out[name] = true
				break
			}
		}
		return out
	}
}

func extractRTLChainStyleIDs(stylesXML []byte) map[string]bool {
	out := make(map[string]bool)
	if len(stylesXML) == 0 {
		return out
	}
	allIDs := extractExistingStyleIDs(stylesXML)
	directRTL := make(map[string]bool)
	for id := range allIDs {
		if styleHasRTLDirect(stylesXML, id) {
			directRTL[id] = true
		}
	}
	for id := range allIDs {
		visited := make(map[string]bool)
		cursor := id
		for cursor != "" && !visited[cursor] {
			visited[cursor] = true
			if directRTL[cursor] {
				out[id] = true
				break
			}
			cursor = styleBasedOn(stylesXML, cursor)
		}
	}
	return out
}

// sourceParagraphStyleInfo captures the metadata WSO needs to test a
// source paragraph style as a match candidate per upstream Okapi's
// WordStyleDefinitions.Ids.parentBased contract — see
// WordStyleDefinitions.java:462-475:
//
//	final Optional<String> existing = this.styleDefinitions.stylesByStyleIds.entrySet()
//	    .stream()
//	    .filter(e -> type == e.getValue().type())
//	    .filter(e -> parentId.equals(e.getValue().parentId()))
//	    .filter(e -> paragraphBlockProperties.mergeableWith(e.getValue().paragraphProperties()))
//	    .filter(e -> runProperties.equals(e.getValue().runProperties()))
//	    .map(e -> e.getKey())
//	    .findFirst();
//
// The match fields:
//
//   - basedOn — the parent style id (filtered against the current
//     paragraph's resolved parentID). Native compares string-equal.
//   - rPrProps — the rPr children of THIS style entry, parsed in source
//     order into the same canonical-XML runProp shape native uses for
//     commonForStyle / commonRPrXML. Compared element-by-element for the
//     `runProperties.equals` check (List<Property>.equals in Java is
//     order-sensitive — see RunProperties.Default.equalsProperties at
//     RunProperties.java:653-663: `return properties.equals(rp.properties)`
//     with the per-RunProperty.equalsProperty implementations).
//   - hasParagraphProps — true when the style's pPr has ANY child
//     property element (a pPr element with content other than the bare
//     `<w:pPr/>` / `<w:pPr></w:pPr>` envelope). When the source style
//     authors paragraph-level pPr props, native conservatively SKIPS
//     this candidate to honour upstream's mergeableWith semantics. The
//     paragraph at the WSO call site has its pPr's pStyle already
//     stripped at the equivalence point (Word.mergeableWith /
//     Default.mergeableWith at ParagraphBlockProperties.java:128-131,
//     :693-701) but native does not track pPr property-by-property
//     equality, so the safe approximation is "candidate must have an
//     empty pPr too". This covers the recipe's targeted fixtures
//     (delTextAmp / Hangs / StartsWithLineSeparator: their source
//     paragraph styles either have no pPr or carry only structurally-
//     ignorable members like rsid+spacing, which our hasParagraphProps
//     check correctly rejects until per-prop equality is implemented).
//
// Per ECMA-376-1 §17.7.4 (Style Definitions) every `<w:style>` carries
// w:type ∈ {paragraph, character, table, numbering}; only paragraph
// styles participate in WSO's parentBased candidate set — character
// styles parent rStyle inheritance, table/numbering styles serve their
// own block types.
type sourceParagraphStyleInfo struct {
	basedOn           string
	rPrProps          []runProp
	hasParagraphProps bool
}

// currentSourceParagraphStyles is set by the writer (writer.go) before
// invoking optimizeWMLPart on each WML part. It maps every source
// paragraph styleId to its WSO-relevant metadata so findMatchingStyle
// can match against existing-source styles in addition to the in-pass
// synthesised set — mirroring upstream WordStyleDefinitions.Ids
// .parentBased (WordStyleDefinitions.java:462-475) which walks the
// FULL stylesByStyleIds map (which already contains source styles
// placed by WordStyleDefinitions.readWith before any synth occurs).
//
// Module-level state matching the pattern of currentRTLChainStyles
// (defined above) — keeps the optimizeWMLPart / optimizeParagraph
// signatures stable for the test suite. The Writer resets it to nil
// after each WSO pass via the same deferred cleanup that resets
// currentRTLChainStyles.
var currentSourceParagraphStyles map[string]sourceParagraphStyleInfo

// extractSourceParagraphStyles walks stylesXML and returns the map of
// every `<w:style w:type="paragraph" ... w:styleId="X">` entry to its
// WSO match metadata (basedOn, rPrProps, hasParagraphProps).
//
// Per ECMA-376-1 §17.7.4 the `w:type="paragraph"` attribute filters out
// character/table/numbering styles which never appear as WSO match
// candidates (Ids.parentBased filters `type == e.getValue().type()` and
// StyleOptimisation.Default constructs with `StyleType.PARAGRAPH`).
// Self-closing `<w:style/>` entries (no body) are skipped — they cannot
// carry rPr/pPr/basedOn.
//
// The rPr children are parsed via parseRunPropElements so the result is
// byte-equal in canonical form to the runProp slice that
// commonProps / buildRPrXML produce for the common-rPr lift. Property
// order is preserved from styles.xml — mirrors upstream's
// `properties.equals(rp.properties)` order-sensitive List comparison
// (RunProperties.java:653-663, with the TODO comment "handle out of
// order properties" acknowledging the order-sensitivity).
func extractSourceParagraphStyles(stylesXML []byte) map[string]sourceParagraphStyleInfo {
	out := make(map[string]sourceParagraphStyleInfo)
	if len(stylesXML) == 0 {
		return out
	}
	cursor := 0
	openNeedle := []byte("<w:style")
	closeNeedle := []byte("</w:style>")
	for {
		idx := bytes.Index(stylesXML[cursor:], openNeedle)
		if idx < 0 {
			return out
		}
		start := cursor + idx
		j := start + len(openNeedle)
		if j >= len(stylesXML) {
			return out
		}
		// Element-name boundary check (reject "<w:styles", "<w:styleLink").
		b := stylesXML[j]
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' && b != '>' && b != '/' {
			cursor = j
			continue
		}
		// Locate end of start tag.
		tagEnd := bytes.IndexByte(stylesXML[j:], '>')
		if tagEnd < 0 {
			return out
		}
		tag := stylesXML[start : j+tagEnd+1]
		// Self-closing `<w:style ... />` — no body.
		selfClose := tagEnd > 0 && stylesXML[j+tagEnd-1] == '/'
		if selfClose {
			cursor = j + tagEnd + 1
			continue
		}
		// Must be type="paragraph".
		if !bytes.Contains(tag, []byte(`w:type="paragraph"`)) {
			// Advance past start tag and continue.
			cursor = j + tagEnd + 1
			continue
		}
		// Extract styleId from start tag.
		idAttrStart := bytes.Index(tag, []byte(`w:styleId="`))
		if idAttrStart < 0 {
			cursor = j + tagEnd + 1
			continue
		}
		vstart := idAttrStart + len(`w:styleId="`)
		vend := bytes.IndexByte(tag[vstart:], '"')
		if vend < 0 {
			cursor = j + tagEnd + 1
			continue
		}
		styleID := string(tag[vstart : vstart+vend])
		// Locate close tag to bound body.
		ci := bytes.Index(stylesXML[j+tagEnd+1:], closeNeedle)
		if ci < 0 {
			return out
		}
		bodyStart := j + tagEnd + 1
		bodyEnd := bodyStart + ci
		body := stylesXML[bodyStart:bodyEnd]
		info := sourceParagraphStyleInfo{}
		// basedOn — flat scan (basedOn appears at most once per ECMA-376-1
		// §17.7.4.3 ST_BasedOn).
		if bi := bytes.Index(body, []byte(`<w:basedOn w:val="`)); bi >= 0 {
			vs := bi + len(`<w:basedOn w:val="`)
			ve := bytes.IndexByte(body[vs:], '"')
			if ve > 0 {
				info.basedOn = string(body[vs : vs+ve])
			}
		}
		// pPr presence + non-empty check. We look for `<w:pPr`; if
		// present and not the empty `<w:pPr/>` / `<w:pPr></w:pPr>`
		// shape, treat the style as carrying paragraph-level props.
		// Mirrors the conservative `mergeableWith` guard described on
		// the sourceParagraphStyleInfo type.
		if pi := bytes.Index(body, []byte("<w:pPr")); pi >= 0 {
			pj := pi + len("<w:pPr")
			if pj < len(body) {
				pc := body[pj]
				if pc == '/' || pc == '>' || pc == ' ' || pc == '\t' || pc == '\n' || pc == '\r' {
					// Find end of pPr start tag.
					pte := bytes.IndexByte(body[pj:], '>')
					if pte >= 0 {
						pTagEnd := pj + pte
						pSelf := pTagEnd > 0 && body[pTagEnd-1] == '/'
						if !pSelf {
							// Open form — find balanced close.
							pClose := bytes.Index(body[pTagEnd+1:], []byte("</w:pPr>"))
							if pClose >= 0 {
								inner := body[pTagEnd+1 : pTagEnd+1+pClose]
								// Treat as having properties when inner
								// has non-whitespace content.
								for _, ic := range inner {
									if ic != ' ' && ic != '\t' && ic != '\n' && ic != '\r' {
										info.hasParagraphProps = true
										break
									}
								}
							}
						}
					}
				}
			}
		}
		// rPr — parse children into runProp slice in source order.
		if ri := bytes.Index(body, []byte("<w:rPr")); ri >= 0 {
			rj := ri + len("<w:rPr")
			if rj < len(body) {
				rc := body[rj]
				if rc == '/' || rc == '>' || rc == ' ' || rc == '\t' || rc == '\n' || rc == '\r' {
					rte := bytes.IndexByte(body[rj:], '>')
					if rte >= 0 {
						rTagEnd := rj + rte
						rSelf := rTagEnd > 0 && body[rTagEnd-1] == '/'
						if !rSelf {
							rClose := bytes.Index(body[rTagEnd+1:], []byte("</w:rPr>"))
							if rClose >= 0 {
								// parseRunPropElements expects the rPr
								// envelope as input (matches its findFirst
								// of `<w:rPr` + trim to `</w:rPr>`); slice
								// the full envelope here.
								envelope := body[ri : rTagEnd+1+rClose+len("</w:rPr>")]
								info.rPrProps = parseRunPropElements(envelope)
							}
						}
					}
				}
			}
		}
		out[styleID] = info
		cursor = bodyEnd + len(closeNeedle)
	}
}

// runPropsEqual reports whether two runProp slices have identical
// element-by-element canonical XML. Used by findMatchingStyle's
// existing-source-style branch to mirror upstream's
// `runProperties.equals` check (RunProperties.java:653-663:
// `properties.equals(rp.properties)` — a List<Property>.equals that is
// order-sensitive per Java's AbstractList.equals contract).
//
// Native's normalizeEmptyElement (called by parseRunPropElements)
// canonicalises `<w:X></w:X>` to `<w:X/>` so the equality is robust to
// the open/close vs self-closing distinction encoding/xml's
// Decoder/Encoder cycle can introduce. Both inputs flow through
// parseRunPropElements (run-side at WSO time via runEntry.props;
// styles-side via extractSourceParagraphStyles → parseRunPropElements)
// so the canonical form is consistent on both sides.
func runPropsEqual(a, b []runProp) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].name != b[i].name || a[i].xml != b[i].xml {
			return false
		}
	}
	return true
}

// containsContentRevisionWrapper reports whether the paragraph src has
// any <w:ins>...</w:ins> or <w:del>...</w:del> CONTENT wrapper (i.e.
// outside of pPr's rPr, where the empty-form paragraph-mark variants
// would be — those have already been stripped by
// stripWMLSkippableElements in our pipeline). When this is true,
// StyleOptimisation must bypass the paragraph: the content wrappers
// imply tracked-revision inserted/deleted runs whose rPr should not
// participate in common-property extraction (Okapi's
// auto-accept-revisions semantics handle these specially via Block /
// RunBuilder routing that the post-write pass cannot replicate).
func containsContentRevisionWrapper(src []byte, pPrStart, pPrEnd int, hasPPr bool) bool {
	// Scan for "<w:ins" or "<w:del" outside the pPr range.
	scan := func(needle []byte) bool {
		i := 0
		for i < len(src) {
			idx := bytes.Index(src[i:], needle)
			if idx < 0 {
				return false
			}
			at := i + idx
			i = at + len(needle)
			// Skip if inside the pPr.
			if hasPPr && at >= pPrStart && at < pPrEnd {
				continue
			}
			// Confirm element-name boundary.
			if at+len(needle) >= len(src) {
				return false
			}
			b := src[at+len(needle)]
			if b == '>' || b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '/' {
				return true
			}
		}
		return false
	}
	return scan([]byte("<w:ins")) || scan([]byte("<w:del")) ||
		scan([]byte("<w:moveTo")) || scan([]byte("<w:moveFrom"))
}
