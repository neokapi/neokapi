// The WordprocessingML part parser: the parser's own state, the textRun it
// collects paragraph content into, the streaming loop over one part's XML, the
// deferred-emit buffers that carry a paragraph across a part-level boundary,
// and the skeleton buffer the loop writes non-translatable markup to.

package openxml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/xmlesc"
	"github.com/neokapi/neokapi/core/model"
)

// wmlNamespace is the Transitional WordprocessingML namespace defined
// by ECMA-376 Part 1 §A.1 (the original 2006 schemas.openxmlformats.org
// URI). It identifies <w:p>/<w:r>/<w:t> etc. in the vast majority of
// .docx files produced by Word.
const wmlNamespace = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

// wmlStrictNamespace is the Strict OOXML WordprocessingML namespace
// defined by ISO/IEC 29500-1 §A.1 (the purl.oclc.org URI used when
// `<w:document w:conformance="strict">` is set). Word saves to this
// namespace via "Save as → Strict Open XML Document" (the OOXML Strict
// conformance class). The fixture 859.docx is the canonical example —
// see ECMA-376 Part 1 §17.13.5.16 (<w:ins>) inside a strict body.
//
// Upstream Okapi accepts both URIs as WordprocessingML via the
// Namespaces enum (WordProcessingML + StrictWordProcessingML, see
// Namespaces.class in okapi-filter-openxml-1.48.0). Without this
// alias the streaming parser falls through every `<w:p>` to skeleton-
// only output, which means no translatable block is ever emitted for
// strict documents and pseudo-translation (or any Block tool) never
// touches the body text — including any text wrapped in `<w:ins>`
// inserted-content wrappers.
const wmlStrictNamespace = "http://purl.oclc.org/ooxml/wordprocessingml/main"

// textRun holds a single run's text and formatting within a paragraph.
type textRun struct {
	text  string
	props runProps
	// data carries raw XML payload for sentinel runs (drawing, pict,
	// object, oMath, oMathPara, mc:AlternateContent). Empty for plain
	// text and zero-data sentinels (tab, break).
	data string
	// attrs carries the canonical, format-neutral run attributes
	// (model.AttrHref, model.AttrTitle, …) for a sentinel whose source
	// element holds them indirectly — today the hyperlink open sentinel,
	// whose destination lives in the part's relationships rather than in
	// the element. Kept alongside `data` (the verbatim markup), not
	// instead of it: `data` is what the skeleton write path replays,
	// `attrs` is what core/projection reads.
	attrs map[string]string
	// srcRunStart is true when this textRun is the FIRST content
	// emitted from a fresh source <w:r>. The flag survives mergeRuns
	// (mergeRuns never crosses sentinels or "\n" line breaks, so the
	// first textRun of each source run is preserved). buildBlock
	// consults this flag for <w:br/> textRuns so the writer can keep
	// the source-run boundary visible: upstream Okapi RunBuilder
	// (okapi/filters/openxml/RunBuilder.java:73-188) keeps tab/break
	// chunks INSIDE their source <w:r> rather than fusing across
	// run boundaries, so a <w:br/> that began a new <w:r> must NOT
	// be inlined into the preceding text's run on the way out. Per
	// ECMA-376-1 §17.3.3.1, <w:br/> is a run child whose containing
	// <w:r> defines its rPr context; reusing the previous <w:r> for
	// a break that originated in a different source <w:r> changes
	// the wire-level structure (1421-line-break.docx).
	srcRunStart bool
	// inFieldDisplay is true when this textRun was emitted while the
	// reader was inside the display-text region of an extractable
	// complex field (between fldChar-separate and fldChar-end with
	// cfs.atResult=true). Upstream Okapi captures every source <w:r>
	// of that region as its own RunText body chunk inside the field's
	// single RunBuilder (parseContent at RunParser.java:537 +
	// parseText at lines 820-836; addToMarkup at line 815 captures
	// the surrounding <w:r>...</w:r> envelope events as Markup body
	// chunks between the RunText chunks). The serialised output
	// therefore preserves the source's per-`<w:r>` boundaries —
	// adjacent same-rPr display-text runs do NOT collapse into one
	// `<w:r>` the way RunMerger fuses adjacent paragraph-level
	// RunBuilders (RunMerger.add at RunMerger.java:83-95). Honour
	// this in mergeRuns by refusing to merge across an inFieldDisplay
	// boundary. Per ECMA-376-1 §17.16.5 (Complex Fields) the
	// extracted display text retains the source's run grouping;
	// fixtures 1083-empty-and-hyperlink-instructions.docx (and the
	// two hyperlink-and-* siblings) expose the " " + "with" boundary
	// that must round-trip as two `<w:r>` shells, not one.
	inFieldDisplay bool
	// sourceHadRPr is true when the source `<w:r>` carried a `<w:rPr>`
	// child element at parse time — regardless of whether any of its
	// children survived the RunSkippableElements strip. Upstream Okapi's
	// flush(Run.Markup) path (BlockTextUnitWriter.java:238-251) emits the
	// raw `<w:rPr>` open/close events verbatim from the field's outer
	// Run body chunks, so an originally-`<w:rPr><w:lang/></w:rPr>` shell
	// surfaces as `<w:rPr></w:rPr>` after stripping. The flag lets the
	// writer (writer.go emitRPr) emit a placeholder-empty wrapper for
	// in-field-display runs whose source declared an rPr; runs that had
	// NO source rPr (e.g. 1172.docx P2's bare `<w:r><w:t>...</w:t></w:r>`
	// runs) stay without an rPr wrapper.
	sourceHadRPr bool
	// preFieldBody is true when this textRun is translatable body content
	// (a `<w:t>` RunText chunk, or `<w:tab/>` markup) authored in the SAME
	// source `<w:r>` BEFORE a `<w:fldChar w:fldCharType="begin"/>` that
	// OPENS a complex field — i.e. the field was NOT yet active when this
	// run started parsing. Upstream Okapi processes such text as a RunText
	// body chunk of the field-opening run (RunParser.parse loop +
	// parseContent at RunParser.java:537) BEFORE transitioning to
	// parseComplexField on the begin (RunParser.java:259), so the text
	// stays a translatable body chunk and is NOT suppressed by the field's
	// begin→separate markup-only window. Per ECMA-376-1 §17.3.2.1 (CT_R)
	// every run child applies to the run; pre-begin body text must survive
	// extraction. The caller's field-aware dropTextRuns keeps runs with
	// this flag set (see parseParagraph). Fixture: 830-7.docx
	// (`<w:r><w:rPr>…</w:rPr><w:t>, humans exiled…; the </w:t>
	// <w:fldChar w:fldCharType="begin"/></w:r>`).
	preFieldBody bool
}

// wmlParser parses WordprocessingML XML parts (document.xml, headers, footers, etc.).
type wmlParser struct {
	cfg           *Config
	blockCounter  *int
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer
	rels          map[string]relationship // hyperlink rels for this part
	// drawingPropText maps a surfaced drawing-property block id to the text it
	// replaced, so a drawing's alt text stays recoverable after the attribute
	// value has been substituted with its marker.
	drawingPropText map[string]string
	codeFinder      *codeFinder // regex-based inline code detection
	styles          *styleMap   // resolved style inheritance (nil if not enabled)
	// roleStyles maps a paragraph styleId to the semantic role it implies
	// (heading/title), resolved from word/styles.xml (WS2). It is additive
	// stand-off metadata recorded on each Block via SetSemanticRole — never
	// serialized back into the .docx, so byte-faithful round-trip is
	// unaffected. nil when styles.xml is absent; the built-in styleId
	// heuristic in roleForParaStyle still applies.
	roleStyles styleRoleMap
	// partPlane is the layout-layer plane every block in this part inherits
	// from the part's role (header/footer → furniture); "" for the main
	// document body. partNoteRole is the fallback semantic role for paragraphs
	// in a note part (footnotes/endnotes → footnote). Both are §8 structure
	// facets — additive stand-off metadata, never serialized back.
	partPlane    string
	partNoteRole string
	// currentStyleChainNames is the resolved set of rPr-child element
	// local names contributed by docDefaults + the current paragraph's
	// basedOn chain. It is recomputed on each <w:pPr> we encounter
	// (when styles is non-nil and the paragraph carries a pStyle that
	// matches a known styleEntry) and consumed by parseRunPropsFromRaw
	// → minifyRPrChildren so explicit-off WPML toggles can be kept as
	// style-chain clearing overrides. Reset to nil at paragraph entry
	// so it never leaks across paragraphs.
	currentStyleChainNames map[string]bool
	// strict reports whether the document binds the "w" prefix to the
	// Strict OOXML namespace (wmlStrictNamespace,
	// "http://purl.oclc.org/ooxml/wordprocessingml/main"). Used by
	// raw-rPr re-parse paths (parseRunPropsFromRaw) so that lang
	// skipping in parseRunProps mirrors upstream Okapi's namespace-
	// keyed RUN_PROPERTY_LANGUAGE QName check — strict documents
	// preserve <w:lang> through the round-trip per the QName mismatch
	// against Namespaces.WordProcessingML (Namespaces.java:26-27).
	strict bool
	// rawRPrCache memoizes parseRunPropsFromRaw results within this part
	// (#608, O1). parseRunPropsFromRaw builds an xmlns wrapper string and
	// spins a fresh xml.NewDecoder on every captured complex-field run;
	// most runs share a handful of distinct rPr shapes, so caching by
	// (rPr blob + resolved style-chain fingerprint) collapses the
	// per-run decode to one decode per distinct shape. cfg.AggressiveCleanup
	// and strict are fixed for the parser, so they are not part of the
	// key. The cached runProps is returned with a freshly cloned
	// rPrChildren slice so downstream in-place minification
	// (runs[i].props.rPrChildren = children[:0]...) cannot corrupt the
	// shared entry — keeping the result byte-identical to the uncached path.
	rawRPrCache map[rawRPrCacheKey]runProps
	// partCfs carries complex-field state ACROSS paragraph boundaries
	// within one XML part. A `<w:fldChar fldCharType="begin"/>` opens
	// the field at the run granularity, but the matching end may live
	// in a later paragraph — upstream Okapi reads the event stream as
	// one continuous flow (parseComplexField at RunParser.java:461-542
	// consumes events past `<w:p>` and `</w:p>` until isComplexFieldEnd
	// fires). To match that semantics our reader keeps the state
	// machine on the parser rather than re-initialising it on each
	// `<w:p>`. Per ECMA-376-1 §17.16.5 (Complex Fields) the field's
	// scope is defined by its begin/end pair regardless of the
	// enclosing block structure. Fixture
	// 1083-date-and-hyperlink-instructions.docx is the canonical
	// cross-paragraph non-extractable case ("A link" sits in its own
	// `<w:p>` between separate and end of a DATE field — must NOT be
	// extracted as translatable text).
	partCfs complexFieldState
	// partMergeable carries cross-paragraph "deleted paragraph mark"
	// merge state. When a paragraph carries the
	// `<w:pPr><w:rPr><w:del/></w:rPr></w:pPr>` (or `<w:moveFrom/>`)
	// marker (ECMA-376 Part 1 §17.13.5.13 CT_ParaRPr) AND has
	// non-empty translatable content, the paragraph mark itself is
	// part of a tracked deletion: under auto-accept-revisions the
	// paragraph break is removed, so the paragraph's content
	// collapses into the FOLLOWING paragraph's block.
	//
	// Mirrors upstream Okapi's mergeable-block flow:
	//   - BlockParser.parse line 207-213 sets builder.mergeable(true)
	//     on a block whose ParagraphBlockProperties.containsRunPropertyDeletedParagraphMark()
	//     returns true (ParagraphBlockProperties.java lines 576-586).
	//   - StyledTextPart.process lines 312-319 buffers that block as
	//     `mergeableBlock` and, when the next block arrives, calls
	//     `block.mergeWith(mergeableBlock)` (Block.java lines 139-166)
	//     to splice the mergeable's middle chunks into the receiver
	//     ahead of the receiver's own runs.
	//   - The mergeable's pPr is discarded — only the mergeable's
	//     content runs survive (Block.mergeWith copies chunks 1..N-1
	//     and keeps the receiver's chunk 0 paragraph markup).
	//
	// We mirror this by buffering the post-mergeRuns slice on the
	// parser (no skeleton bytes written for the deferred paragraph)
	// and prepending it to the next paragraph's runs before
	// commonRPrChildren / mergeRuns / buildBlock.
	//
	// `partMergeable` is scoped to one XML part (one `wmlParser`
	// instance) — each part gets a fresh parser via the reader, so
	// the buffer never leaks between parts. If a part ends with a
	// pending mergeable (no successor paragraph absorbs it), the
	// EOF flush in parsePart emits it as a standalone paragraph
	// using its saved pPr — matching upstream's
	// StyledTextPart.process tail at lines 642-644 which still
	// emits the dangling mergeableBlock.
	//
	// Fixtures: 847-2.docx, 847-3.docx, 1102.docx (the canonical
	// content-bearing cases). 1370 remains the empty-content
	// "drop entirely" case handled by the existing
	// paragraphHasDeletedMark check at the empty-runs branch.
	partMergeable *pendingMergeable
	// partFieldStraddle defers emit of a paragraph that closed while
	// an extractable complex field was still open at result phase
	// (cfs.active && cfs.extractable && cfs.atResult). The next
	// paragraph(s) may carry a lone fldChar-end run that upstream
	// Okapi absorbs back into the prior block via
	// `parseComplexField`'s deferred-events path
	// (RunParser.java:508-514 + endComplexFieldParsing at 594-609).
	// Deferring lets us append the tail fldChar-end to this
	// paragraph's buffered run slice and emit one combined block;
	// the trailing fldChar-end-only paragraph then re-emits as an
	// empty `<w:p>` with its own pPr.
	partFieldStraddle *pendingFieldBlock
	// partAbsorbedTrailingEmpty signals that we just flushed a
	// partFieldStraddle whose original pPr carried a
	// `<w:pPr><w:rPr><w:del/></w:rPr></w:pPr>` (ECMA-376 Part 1
	// §17.13.5.13 CT_ParaRPr) deleted-paragraph-mark. Upstream Okapi
	// makes the absorbed block `mergeable=true` (BlockParser.java:207-
	// 213). When such a block reaches StyledTextPart.process line
	// 312-319 it is buffered as `mergeableBlock` and only emitted when
	// the next non-mergeable block (often a trailing empty
	// `<w:p ...>/`) calls `block.mergeWith(mergeableBlock)` —
	// absorbing the buffered block's chunks INTO the trailing
	// paragraph's wrapper. The trailing wrapper carries through to
	// the rendered output; the original mergeable paragraph's `<w:p>`
	// wrapper is discarded.
	//
	// Native already emits the absorbed merged block inline at the
	// flush point (using the field-straddle paragraph's own pPr). To
	// mirror upstream's wrapper-consumption we mark the next plain
	// trailing `<w:p ...>` element (no pPr, no body) as the structural
	// merge target and drop it without emit. The flag clears after one
	// consumption (the next non-empty paragraph also clears it).
	//
	// Fixture 1102.docx is the canonical case: source P2 (mergeable
	// via delMark, content "Label 1:" plus HYPERLINK field begin/sep)
	// gets buffered as partFieldStraddle; P3 (empty with delMark)
	// triggers the flush; P5 (`<w:p ... />` self-closing, no pPr)
	// is the trailing wrapper Okapi consumes for the merged block.
	partAbsorbedTrailingEmpty bool
	// emitPart, when set, emits a Part directly to the reader's output channel
	// (in document order, interleaved with emitBlock). It is used to surface
	// table topology — w:tbl / w:tr become table / table-row Groups, w:tc cells'
	// paragraphs are tagged RoleTableCell — so cross-format writers and
	// core/projection rebuild the grid instead of seeing flat paragraphs. Groups
	// are additive: they carry no skeleton bytes, so docx→docx round-trip stays
	// byte-exact. nil disables table-structure emission.
	emitPart func(*model.Part)
	// structStack tracks the open table/table-row groups so a closing element
	// pops the matching one; cellDepth tracks w:tc nesting so a paragraph inside
	// a cell is tagged RoleTableCell. groupCounter names the emitted groups.
	structStack  []structFrame
	cellDepth    int
	groupCounter int
	// path addresses each block by part plus table/row/cell position — see
	// structural_name.go. It is maintained regardless of emitPart, because a
	// block's Name is not optional the way group emission is.
	path oxmlPath
	// pendingColSpan is the horizontal cell merge (w:tcPr/w:gridSpan) parsed for
	// the current cell, applied to its first paragraph (tagged RoleTableCell) so
	// spanned grids reconstruct aligned. Reset at each cell boundary.
	pendingColSpan int
	// pendingVMerge is the vertical-merge state of the current cell (w:vMerge):
	// "restart" begins a merge, "continue" extends the one above in this grid
	// column, "" is a normal cell. Reset at each cell boundary.
	pendingVMerge string
	// cellCol is the 0-based grid column of the current cell within its row,
	// advanced by each cell's gridSpan; cellSpan is the current cell's gridSpan
	// held so the column cursor advances correctly at the cell's close.
	cellCol  int
	cellSpan int
	// vmergeOpen maps a grid column to the StructureAnnotation of the cell that
	// began a vertical merge there, so a "continue" cell below increments that
	// cell's RowSpan. The cell block is already emitted; the table consumers
	// (cross-format writers, projection/anatomy) collect the whole stream before
	// reading spans, so the post-emit increment is observed (happens-before the
	// channel close). Cleared when a non-merge cell reuses the column or the
	// table ends.
	vmergeOpen map[int]*model.StructureAnnotation
}

// pendingMergeable carries the post-mergeRuns slice for a paragraph
// whose `<w:pPr><w:rPr>` declares a deleted/moveFrom paragraph mark
// (ECMA-376 Part 1 §17.13.5.13). The runs are saved AFTER style
// subtraction and mergeRuns, so they can be prepended into the
// next paragraph's run list without re-subtraction. The captured
// pPr is retained for the EOF dangling-mergeable fallback path so
// we can synthesise a standalone `<w:p>` if no successor arrives.
//
// Mirrors upstream Okapi's `mergeableBlock` local in
// StyledTextPart.process (lines 270, 312-319, 642-644) — a single
// pending block per parser, replaced when consumed by the next
// non-mergeable block.
type pendingMergeable struct {
	runs        []textRun
	paraProps   string
	paraStyleID string
}

// pendingFieldBlock carries a deferred paragraph emit for a paragraph
// that closed while an extractable complex field was still open at
// result phase. We retain everything needed to (a) append additional
// fldChar-end runs from a successor paragraph (b) re-run buildBlock /
// commonRPrChildren / per-run sidecars on the augmented slice and
// (c) emit the skeleton bytes (`<w:p>` + paraProps + ref + `</w:p>`)
// at the right moment.
//
// Mirrors upstream Okapi's `parseComplexField` deferred-events
// machinery (RunParser.java:461-542): when a paragraph end event
// arrives inside an extractable field at result phase, it goes into
// `deferredEvents`. When the field finally ends via the
// `goesAfterAnotherRun=true` branch of endComplexFieldParsing
// (RunParser.java:594-598), the deferred events flush through
// parseContent and the fldChar-end markup lands BEFORE the deferred
// `<w:p>` end events — so the field-end appears in the previous
// paragraph in the rendered output.
type pendingFieldBlock struct {
	runs        []textRun // post-mergeRuns
	paraProps   string
	paraStyleID string
	partPath    string
}

// parsePart streams through a WordprocessingML XML part, emitting Blocks.
func (p *wmlParser) parsePart(data []byte, partPath string, emitBlock func(*model.Block), emitData func()) error {
	// Root block names at this part before any structure opens — a later
	// re-rooting would drop the table/row/cell scopes already pushed.
	p.path.ensurePart(partPath)
	// When AutomaticallyAcceptRevisions is true, pre-process the bytes
	// to mirror upstream Okapi's revision-acceptance passes that
	// happen before the streaming parser sees the document:
	//
	//   1. dropMoveFromRanges: collapses <w:moveFromRangeStart ...>...
	//      <w:moveFromRangeEnd .../> cross-structure spans, dropping
	//      enclosing paragraphs/rows/tables depending on what the
	//      span crosses (ECMA-376 Part 1 §17.13.5.18 / §17.13.5.19).
	//      Mirrors SkippableElements.MoveFromRevisionCrossStructure +
	//      StyledTextPart row/table cleanup branches.
	//
	//   2. dropDeletedRows: drops <w:tr> rows whose <w:trPr> carries
	//      a top-level <w:del> child (ECMA-376 §17.13.5.13 Deleted
	//      Table Row). Mirrors StyledTextPart.process lines 530-551
	//      + RevisionProperty.TABLE_ROW_DELETED.
	//
	//   3. dropEmptyTables: collapses any <w:tbl> whose body lost all
	//      its rows to the previous passes. Mirrors the TableEnd
	//      branch in StyledTextPart (lines 410-424) which drops the
	//      queued delayedTableMarkup when no translatable block
	//      reached the writer between <w:tbl> and </w:tbl>.
	//
	// Byte-level pre-passes keep the streaming xml.Decoder loop
	// unchanged; the alternative — re-decoding captured subtrees
	// mid-parse — is invasive, changes namespace-resolution semantics
	// for the captured children (encoding/xml binds prefixes per-
	// decoder, our namespace registry is global), and breaks raw-
	// payload capture for VML shapes inside the row/table. Doing the
	// strips up front sidesteps both.
	if p.cfg != nil && p.cfg.AutomaticallyAcceptRevisions {
		data = dropMoveFromRanges(data)
		data = dropDeletedRows(data)
		data = dropEmptyTables(data)
	}
	d := xml.NewDecoder(bytes.NewReader(data))

	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("wml: parsing %s: %w", partPath, err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				if isWML(t) || isWMLNoNS(t) {
					if err := p.parseParagraph(d, partPath, emitBlock); err != nil {
						return err
					}
				} else {
					p.skelWriteStartElement(t)
				}
			case "sdt":
				// Structured document tag — recurse into content
				if err := p.parseSDT(d, partPath, emitBlock, emitData); err != nil {
					return err
				}
			case "tbl":
				// Table — recurse to find paragraphs inside cells. Bracket it in
				// a canonical "table" Group so cross-format writers + projection
				// rebuild the grid (additive; no skeleton bytes).
				p.skelWriteStartElement(t)
				p.openTableStruct("tbl")
			case "tc":
				// Table cell — recurse into its paragraphs, tagging them
				// RoleTableCell via cellDepth.
				p.skelWriteStartElement(t)
				p.openTableStruct("tc")
			case "tr":
				// Table row — inspect <w:trPr> for the row-deletion
				// marker <w:trPr><w:del .../></w:trPr> (revision tracking,
				// ECMA-376 Part 1 §17.13.5.13 Deleted Table Row). When
				// AutomaticallyAcceptRevisions is true (Okapi default —
				// ConditionalParameters.java line 813), the entire row
				// (start tag, content, end tag) is dropped from the
				// output. Mirrors upstream Okapi
				// StyledTextPart.process() lines 530-551, which calls
				// revisionPropertyTableRowDeletedSkippableElements.skip
				// and then removes the queued row markup via
				// delayedTableMarkup.componentsIteratorAtLastWith(
				// LOCAL_TABLE_ROW); iterator.remove();
				// removeComponentsWith(iterator).
				//
				// The row-INSERTION marker
				// <w:trPr><w:ins .../></w:trPr> (ECMA-376 §17.13.5.16)
				// is ALSO accepted: the inserted row stays, the <w:ins>
				// marker inside trPr is dropped at write time by
				// wmlRevisionParagraphMarkRE. Mirrors upstream
				// revisionPropertyTableRowInsertedSkippableElements.skip
				// at StyledTextPart.java lines 515-528, which drains the
				// <w:ins> element without removing the row.
				if (isWML(t) || isWMLNoNS(t)) && p.cfg != nil && p.cfg.AutomaticallyAcceptRevisions {
					if err := p.handleTableRow(d, t); err != nil {
						return err
					}
					continue
				}
				p.skelWriteStartElement(t)
				p.openTableStruct("tr")
			case "footnote", "endnote":
				// Skip the auto-generated separator/continuation
				// footnotes whose body is non-translatable boilerplate
				// (a <w:separator/>, <w:continuationSeparator/>, or
				// continuation-notice marker run). Per ECMA-376 Part 1
				// §17.11.10 (CT_Footnote) and §17.11.16 (CT_Endnote),
				// the w:type attribute (ST_FtnEdn) discriminates these
				// from the default ("normal") footnotes/endnotes that
				// carry translatable text. The previous heuristic of
				// matching by w:id ("0", "1", "-1") was unreliable —
				// the non-translatable IDs are author-assigned and
				// vary per document (e.g. {-1, 0} in docxtest.docx,
				// {0, 1} in OpenXML_text_reference_v1_2.docx), so any
				// id-based filter risked dropping the actual footnote
				// content from the translatable-block pipeline. Mirrors
				// upstream Okapi's behaviour: BlockParser emits no
				// translatable block for runs whose only content is a
				// <w:separator/> / <w:continuationSeparator/> element,
				// so those <w:footnote> wrappers reach the writer as
				// pure skeleton; the same outcome is achieved here by
				// switching on w:type.
				wType := attrVal(t, "type")
				if wType == "separator" || wType == "continuationSeparator" || wType == "continuationNotice" {
					p.skelWriteStartElement(t)
					if err := p.skipAndSkel(d); err != nil {
						return err
					}
					continue
				}
				p.skelWriteStartElement(t)
			case "pPr", "sectPr", "tblPr", "tblGrid", "trPr", "tcPr":
				// Non-translatable properties — skeleton only.
				// `<w:sectPr>` at the body level is the closing
				// structural marker; if a pending mergeable paragraph
				// (deleted paragraph mark, ECMA-376 Part 1
				// §17.13.5.13) is still buffered here, no successor
				// paragraph arrived to absorb it. Flush it first so
				// the standalone paragraph appears BEFORE the
				// `<w:sectPr>` in document order. Mirrors upstream
				// Okapi StyledTextPart.process tail at lines 642-644
				// emitting the dangling `mergeableBlock`. Fixture
				// 847-1.docx is the canonical case (one paragraph
				// with deleted-mark + content, immediately followed
				// by `<w:sectPr>`).
				if t.Name.Local == "sectPr" && p.partMergeable != nil {
					if err := p.flushPendingMergeable(partPath, emitBlock); err != nil {
						return err
					}
				}
				// Clear the trailing-empty absorption flag at sectPr —
				// if no bare empty `<w:p ... />` consumed it, the
				// absorbed merged block already emitted standalone via
				// flushPendingFieldBlock and there is no structural
				// merge target. Mirrors upstream Okapi's behaviour at
				// StyledTextPart.process lines 642-644: the dangling
				// `mergeableBlock` still emits as a standalone
				// paragraph at EOF (no successor non-mergeable arrives).
				if t.Name.Local == "sectPr" {
					p.partAbsorbedTrailingEmpty = false
				}
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				if t.Name.Local == "tcPr" {
					p.pendingColSpan = gridSpanFromTcPr(raw)
					p.cellSpan = max(1, p.pendingColSpan)
					p.pendingVMerge = vMergeFromTcPr(raw)
					p.resolveCellVMerge()
				}
				p.skelText(raw)
			default:
				p.skelWriteStartElement(t)
			}

		case xml.EndElement:
			p.skelWriteEndElement(t)
			// Close any table/table-row Group or cell depth this end leaves.
			// Every w:tbl/w:tr/w:tc close funnels through here (handleTableRow
			// hands control back before the row ends), so this is the single
			// close site for both row-handling paths.
			p.closeTableStruct(t.Name.Local)

		case xml.CharData:
			p.skelText(xmlesc.Text(string(t)))

		case xml.ProcInst:
			p.skelText("<?" + t.Target + " " + string(t.Inst) + "?>")

		case xml.Directive:
			p.skelText("<!" + string(t) + ">")

		case xml.Comment:
			p.skelText("<!--" + string(t) + "-->")
		}
	}
	// Flush any dangling mergeable paragraph buffer. If the last
	// paragraph in this part carried `<w:pPr><w:rPr><w:del/></w:rPr>`
	// (ECMA-376 Part 1 §17.13.5.13) but no successor paragraph
	// arrived to absorb it, emit the buffered runs as a standalone
	// paragraph using their saved pPr — matching upstream Okapi
	// StyledTextPart.process at lines 642-644 which still emits
	// `mergeableBlock` if it remains non-null at end of part. The
	// writer's stripWMLSkippableElements pass will subsequently
	// remove the `<w:del/>` paragraph mark from the emitted pPr.
	if p.partMergeable != nil {
		if err := p.flushPendingMergeable(partPath, emitBlock); err != nil {
			return err
		}
	}
	// Flush a dangling field-straddle buffer too. When an extractable
	// complex field remains open at end-of-part (no fldChar-end ever
	// arrives), emit the buffered paragraph as-is (no extra tail runs)
	// so its display content survives.
	if p.partFieldStraddle != nil {
		if err := p.flushPendingFieldBlock(nil, partPath, emitBlock); err != nil {
			return err
		}
	}
	// Reset the trailing-empty absorption flag at end-of-part — if any
	// flag survived past sectPr (no sectPr in this part — headers,
	// footers, comments, footnotes typically have none), clear it now
	// so the flag never leaks across parts.
	p.partAbsorbedTrailingEmpty = false
	return nil
}

// flushPendingMergeable emits a buffered mergeable paragraph as a
// standalone `<w:p>` block. Used by the EOF tail in parsePart when
// no successor paragraph arrives to absorb the buffer. Mirrors
// upstream Okapi StyledTextPart.process lines 642-644 (the
// `if (null != mergeableBlock) { mergeableBlock.optimiseStyles();
// mapToEvents(mergeableBlock); }` tail).
//
// Re-runs commonRPrChildren / mergeRuns / drawing extraction on
// the buffered runs (they were saved post-mergeRuns but pre-
// drawing-extraction; mergeRuns is idempotent on already-merged
// groups).
func (p *wmlParser) flushPendingMergeable(partPath string, emitBlock func(*model.Block)) error {
	pm := p.partMergeable
	p.partMergeable = nil
	runs := pm.runs
	commonRPr := commonRPrChildren(runs)
	commonRPrXML := joinRPrChildren(commonRPr)
	merged := mergeRuns(runs)
	perRunRPrXML := perRunRPrFragments(merged)
	perRunSrcRunStart := perRunSrcRunStartFlags(merged)
	for i := range merged {
		if isDrawingSentinel(merged[i].text) && merged[i].data != "" {
			merged[i].data = p.extractDrawingTranslations(merged[i].data, partPath, emitBlock)
		}
	}
	if isEmptyRuns(merged) {
		// Defensive: shouldn't happen because we only buffer when
		// !isEmptyRuns at the buffer site. Drop silently.
		return nil
	}
	inheritedVanish := false
	if p.styles != nil && pm.paraStyleID != "" {
		inheritedVanish = p.styles.effectiveProps(pm.paraStyleID).vanish
	}
	if !p.cfg.TranslateHiddenText && allHidden(merged, inheritedVanish) {
		p.skelWriteString("<w:p>")
		if pm.paraProps != "" {
			p.skelText(pm.paraProps)
		}
		p.skelText(emitRunEnvelopes(merged))
		p.skelWriteString("</w:p>")
		return nil
	}
	*p.blockCounter++
	blockID := fmt.Sprintf("tu%d", *p.blockCounter)
	p.skelWriteString("<w:p>")
	if pm.paraProps != "" {
		p.skelText(pm.paraProps)
	}
	p.skelRef(blockID)
	p.skelWriteString("</w:p>")
	block := p.buildBlock(blockID, merged, partPath, commonRPrXML, perRunRPrXML, perRunSrcRunStart)
	p.applyParagraphRole(block, pm.paraStyleID, pm.paraProps, allHidden(merged, inheritedVanish))
	emitBlock(block)
	return nil
}

// flushPendingFieldBlock emits a buffered field-straddle paragraph
// as a standalone `<w:p>` block. `extraTailRuns` carries any
// successor paragraph's field-tail runs (the lone fldChar-end run
// closing the straddling field) that should be appended to this
// block's run slice; pass nil when no successor paragraph absorbed
// them.
//
// Re-runs commonRPrChildren / mergeRuns / drawing-extraction on the
// (possibly augmented) slice — mergeRuns is idempotent for already-
// merged groups, and field-markup sentinels are skipped by
// commonRPrChildren (StyleOptimisation.java:204-237 only iterates
// text-bearing chunks), so appending them does not alter the
// per-paragraph common-rPr intersection.
//
// Mirrors upstream Okapi's tail of `endComplexFieldParsing`
// (RunParser.java:594-609) plus the `BlockParser.parse` block close
// (BlockParser.java:284-292).
func (p *wmlParser) flushPendingFieldBlock(extraTailRuns []textRun, partPath string, emitBlock func(*model.Block)) error {
	pf := p.partFieldStraddle
	p.partFieldStraddle = nil
	// When the captured paragraph's pPr/rPr carries a `<w:del>` or
	// `<w:moveFrom>` paragraph-mark revision marker, upstream Okapi
	// suppresses the pPr (BlockParser.java:207-213 — see
	// `stripPPrIfDeletedMark` for the full citation). Apply that
	// suppression here so the deferred paragraph mirrors Okapi's emit.
	// Fixture 1102.docx P2 is the canonical case.
	//
	// Capture the delMark presence BEFORE stripping so we can flag the
	// next trailing empty `<w:p ...>` element as the structural merge
	// target Okapi consumes (see partAbsorbedTrailingEmpty field doc
	// and the empty-runs emit branch in parseParagraph).
	if paragraphHasDeletedMark(pf.paraProps) {
		p.partAbsorbedTrailingEmpty = true
	}
	pf.paraProps = stripPPrIfDeletedMark(pf.paraProps)
	runs := pf.runs
	if len(extraTailRuns) > 0 {
		combined := make([]textRun, 0, len(runs)+len(extraTailRuns))
		combined = append(combined, runs...)
		combined = append(combined, extraTailRuns...)
		runs = combined
	}
	commonRPr := commonRPrChildren(runs)
	commonRPrXML := joinRPrChildren(commonRPr)
	merged := mergeRuns(runs)
	perRunRPrXML := perRunRPrFragments(merged)
	perRunSrcRunStart := perRunSrcRunStartFlags(merged)
	for i := range merged {
		if isDrawingSentinel(merged[i].text) && merged[i].data != "" {
			merged[i].data = p.extractDrawingTranslations(merged[i].data, partPath, emitBlock)
		}
	}
	if isEmptyRuns(merged) {
		// Defensive: we only buffer paragraphs with display
		// content, so the merged slice should remain non-empty
		// even after the tail-runs append. If somehow empty,
		// emit a degenerate empty paragraph as a safety net.
		p.skelWriteString("<w:p>")
		if pf.paraProps != "" {
			p.skelText(pf.paraProps)
		}
		p.skelWriteString("</w:p>")
		return nil
	}
	inheritedVanish := false
	if p.styles != nil && pf.paraStyleID != "" {
		inheritedVanish = p.styles.effectiveProps(pf.paraStyleID).vanish
	}
	if !p.cfg.TranslateHiddenText && allHidden(merged, inheritedVanish) {
		p.skelWriteString("<w:p>")
		if pf.paraProps != "" {
			p.skelText(pf.paraProps)
		}
		p.skelText(emitRunEnvelopes(merged))
		p.skelWriteString("</w:p>")
		return nil
	}
	*p.blockCounter++
	blockID := fmt.Sprintf("tu%d", *p.blockCounter)
	p.skelWriteString("<w:p>")
	if pf.paraProps != "" {
		p.skelText(pf.paraProps)
	}
	p.skelRef(blockID)
	p.skelWriteString("</w:p>")
	block := p.buildBlock(blockID, merged, partPath, commonRPrXML, perRunRPrXML, perRunSrcRunStart)
	// Mark the block as a cross-paragraph field straddle so the
	// writer can mirror upstream Okapi's flush(Run.Markup) artifact —
	// an empty `<w:r/>` placeholder emitted before every `<w:br>`
	// Component.Start inside the field's outer Run body chunks.
	// See writer.go (the openxmlBlockFieldStraddleProperty consumer)
	// for the citation chain — BlockTextUnitWriter.flush(Run.Markup)
	// at lines 238-251 closes any open `<w:r>` immediately before
	// re-opening a fresh `<w:r>` for the `<w:br>` events; the
	// initial flushRunStart at line 240 produces the empty
	// envelope when the first component happens to be a `<w:br>`
	// Start. Fixture 1172.docx is the canonical case: source P2
	// runs (text, br, br+text) become Markup body chunks of the
	// outer field Run, and Okapi inserts two empty `<w:r/>` before
	// the br-only and br+text runs respectively.
	if block.Properties == nil {
		block.Properties = map[string]string{}
	}
	block.Properties["openxml:field-straddle"] = "true"
	p.applyParagraphRole(block, pf.paraStyleID, pf.paraProps, allHidden(merged, inheritedVanish))
	emitBlock(block)
	return nil
}

// Skeleton helpers

func (p *wmlParser) skelText(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}

func (p *wmlParser) skelRef(id string) {
	if p.skeletonStore != nil {
		if p.skelBuf.Len() > 0 {
			p.skeletonStore.WriteText(p.skelBuf.Bytes())
			p.skelBuf.Reset()
		}
		p.skeletonStore.WriteRef(id)
	}
}

func (p *wmlParser) skelFlush() {
	if p.skeletonStore != nil && p.skelBuf.Len() > 0 {
		p.skeletonStore.WriteText(p.skelBuf.Bytes())
		p.skelBuf.Reset()
	}
}

func (p *wmlParser) skelWriteStartElement(t xml.StartElement) {
	if p.skeletonStore == nil {
		return
	}
	registerNamespaces(t.Attr)
	var buf strings.Builder
	buf.WriteString("<")
	writeElementName(&buf, t.Name)
	for _, a := range t.Attr {
		buf.WriteString(" ")
		writeAttrName(&buf, a.Name)
		buf.WriteString(`="`)
		buf.WriteString(xmlesc.Attr(a.Value))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")
	p.skelBuf.WriteString(buf.String())
}

func (p *wmlParser) skelWriteEndElement(t xml.EndElement) {
	if p.skeletonStore == nil {
		return
	}
	var buf strings.Builder
	buf.WriteString("</")
	writeElementName(&buf, t.Name)
	buf.WriteString(">")
	p.skelBuf.WriteString(buf.String())
}

func (p *wmlParser) skelWriteString(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}

func (p *wmlParser) skipAndSkel(d *xml.Decoder) error {
	depth := 1
	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			p.skelWriteStartElement(t)
		case xml.EndElement:
			depth--
			p.skelWriteEndElement(t)
		case xml.CharData:
			p.skelText(xmlesc.Text(string(t)))
		}
	}
	return nil
}
