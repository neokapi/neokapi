package openxml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/format"
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
}

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

// wmlParser parses WordprocessingML XML parts (document.xml, headers, footers, etc.).
type wmlParser struct {
	cfg           *Config
	blockCounter  *int
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer
	rels          map[string]relationship // hyperlink rels for this part
	codeFinder    *codeFinder             // regex-based inline code detection
	styles        *styleMap               // resolved style inheritance (nil if not enabled)
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
}

// parsePart streams through a WordprocessingML XML part, emitting Blocks.
func (p *wmlParser) parsePart(data []byte, partPath string, emitBlock func(*model.Block), emitData func()) error {
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
				// Table — recurse to find paragraphs inside cells
				p.skelWriteStartElement(t)
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
				// Non-translatable properties — skeleton only
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				p.skelText(raw)
			default:
				p.skelWriteStartElement(t)
			}

		case xml.EndElement:
			p.skelWriteEndElement(t)

		case xml.CharData:
			p.skelText(xmlEscape(string(t)))

		case xml.ProcInst:
			p.skelText("<?" + t.Target + " " + string(t.Inst) + "?>")

		case xml.Directive:
			p.skelText("<!" + string(t) + ">")

		case xml.Comment:
			p.skelText("<!--" + string(t) + "-->")
		}
	}
	return nil
}

// dropDeletedRows removes every <w:tr ...>...</w:tr> region whose
// <w:trPr> carries a top-level <w:del> child — the row-deletion
// revision marker per ECMA-376 Part 1 §17.13.5.13 (CT_TrPrBase /
// `del`). The streaming parser's handleTableRow already strips
// these rows, but pre-stripping at the byte level lets dropEmptyTables
// collapse a table whose every row was deleted; otherwise the
// structurally-empty <w:tbl> would survive the round-trip (fixture
// 1080-1.docx table 2 with <w:tblpPr> positioning).
//
// Mirrors upstream Okapi's row-removal path:
// StyledTextPart.process() lines 530-551 (the
// RevisionPropertyTableRowDeletedSkippableElements.skip dispatch)
// removes the queued row markup; the downstream TableEnd branch
// (lines 410-424) then drops the whole table when no translatable
// block reached it. The context-aware `del` → `trPr` mapping is at
// SkippableElements.java lines 528-531
// (CONTEXT_AWARE_REVISION_SKIPPABLE_ELEMENTS).
//
// Nested rows (legal per the schema — a <w:tc> may contain another
// <w:tbl>) are handled correctly by tracking depth on <w:tr balanced
// open/close pairs.
func dropDeletedRows(data []byte) []byte {
	const trOpen = "<w:tr"
	const trClose = "</w:tr>"
	const trPrOpen = "<w:trPr"
	if !bytes.Contains(data, []byte(trPrOpen)) {
		// Fast path: no trPr means no row-deletion markers either.
		return data
	}
	out := make([]byte, 0, len(data))
	for {
		idx := bytes.Index(data, []byte(trOpen))
		if idx < 0 {
			out = append(out, data...)
			break
		}
		j := idx + len(trOpen)
		if j >= len(data) {
			out = append(out, data...)
			break
		}
		b := data[j]
		if b != '>' && b != '/' && b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			// Not <w:tr; advance past this position.
			out = append(out, data[:j+1]...)
			data = data[j+1:]
			continue
		}
		k := bytes.IndexByte(data[j:], '>')
		if k < 0 {
			out = append(out, data...)
			break
		}
		startEnd := j + k
		if startEnd > 0 && data[startEnd-1] == '/' {
			// Self-closing <w:tr/>: no <w:trPr>, never deleted.
			out = append(out, data[:startEnd+1]...)
			data = data[startEnd+1:]
			continue
		}
		bodyStart := startEnd + 1
		depth := 1
		cursor := bodyStart
		for depth > 0 {
			nextOpen := bytes.Index(data[cursor:], []byte(trOpen))
			nextClose := bytes.Index(data[cursor:], []byte(trClose))
			if nextClose < 0 {
				out = append(out, data...)
				return out
			}
			if nextOpen >= 0 && nextOpen < nextClose {
				absOpen := cursor + nextOpen
				jj := absOpen + len(trOpen)
				if jj < len(data) {
					bb := data[jj]
					if bb == '>' || bb == '/' || bb == ' ' || bb == '\t' || bb == '\n' || bb == '\r' {
						kk := bytes.IndexByte(data[jj:], '>')
						if kk < 0 {
							out = append(out, data...)
							return out
						}
						nestedOpenEnd := jj + kk
						if nestedOpenEnd > 0 && data[nestedOpenEnd-1] != '/' {
							depth++
						}
						cursor = nestedOpenEnd + 1
						continue
					}
				}
				// Misleading prefix (e.g. <w:trPr inside the body).
				cursor = cursor + nextOpen + len(trOpen)
				continue
			}
			cursor = cursor + nextClose + len(trClose)
			depth--
		}
		rowEnd := cursor // one past the last byte of </w:tr>
		body := data[bodyStart : rowEnd-len(trClose)]
		if rowBodyHasDeletedTrPr(body) {
			out = append(out, data[:idx]...)
			data = data[rowEnd:]
			continue
		}
		out = append(out, data[:rowEnd]...)
		data = data[rowEnd:]
	}
	return out
}

// rowBodyHasDeletedTrPr reports whether the captured row body's own
// direct-child <w:trPr> contains a top-level <w:del> element — the
// row-deletion revision marker per ECMA-376 Part 1 §17.13.5.13
// (CT_TrPrBase / `del`). Mirrors upstream Okapi's
// RevisionProperty.TABLE_ROW_DELETED context-aware skip
// (SkippableElements.java lines 528-531 — `del` keyed under parent
// `trPr`).
//
// Per the schema's `tblPrEx? trPr? content*` sequence the row's
// own trPr precedes any cell content. We locate it by finding the
// first <w:trPr> open tag and verifying no <w:tc>, <w:tbl>, or
// nested <w:tr> appears before it — otherwise the matched trPr
// belongs to a deeper nested row, not the outer row we're examining,
// and must be ignored so a deleted nested row doesn't drag its
// outer ancestor with it.
func rowBodyHasDeletedTrPr(body []byte) bool {
	const trPrOpen = "<w:trPr"
	idx := bytes.Index(body, []byte(trPrOpen))
	if idx < 0 {
		return false
	}
	// Validate element-name boundary so <w:trPrChange> doesn't match.
	j := idx + len(trPrOpen)
	if j >= len(body) {
		return false
	}
	b := body[j]
	if b != '>' && b != '/' && b != ' ' && b != '\t' && b != '\n' && b != '\r' {
		return false
	}
	// Reject if any nested container precedes this trPr — the trPr
	// then belongs to a deeper-nested row, not this one.
	prefix := body[:idx]
	for _, name := range [...]string{"<w:tc", "<w:tbl", "<w:tr"} {
		if pIdx := indexValidElement(prefix, name); pIdx >= 0 {
			return false
		}
	}
	// Find the closing `>` of the open tag and read through </w:trPr>.
	k := bytes.IndexByte(body[j:], '>')
	if k < 0 {
		return false
	}
	startEnd := j + k
	if startEnd > 0 && body[startEnd-1] == '/' {
		// Self-closing <w:trPr/> — no children, no row deletion.
		return false
	}
	closeIdx := bytes.Index(body[startEnd+1:], []byte("</w:trPr>"))
	if closeIdx < 0 {
		return false
	}
	raw := body[idx : startEnd+1+closeIdx+len("</w:trPr>")]
	return trPrHasRowDeletion(string(raw))
}

// dropMoveFromRanges removes the cross-structure spans bracketed by
// <w:moveFromRangeStart w:id="N"/> ... <w:moveFromRangeEnd w:id="N"/>
// markers (ECMA-376 Part 1 §17.13.5.18 / §17.13.5.19) when accepting
// revisions. Mirrors upstream Okapi's
// SkippableElements.MoveFromRevisionCrossStructure (lines 371-450 of
// SkippableElements.java) + BlockParser.parse skipped-block handling
// (lines 267-274 of BlockParser.java) + StyledTextPart.process
// dispatch (lines 580-593 + 299-305 of StyledTextPart.java).
//
// Upstream semantics: when moveFromRangeStart is encountered, an
// event-by-event skip walks through the reader until moveFromRangeEnd
// is consumed (inclusive). EVERY event in between — including the
// </w:p>/<w:p> boundaries of any straddled paragraphs and any
// untracked text in those paragraphs — is dropped wholesale. The
// enclosing block (the <w:p> containing moveFromRangeStart) is marked
// skipped(true) by the BlockParser because parentStructureCrossed
// became true during the skip, and StyledTextPart drops it.
//
// At the byte level we mirror this by, for each (moveFromRangeStart,
// moveFromRangeEnd) pair matched by w:id, removing from the start
// tag of the <w:p> that contains moveFromRangeStart through and
// INCLUDING the </w:p> end tag of the <w:p> that contains
// moveFromRangeEnd. Rationale:
//
//   - The paragraph holding moveFromRangeStart is dropped because the
//     BlockParser returns skipped=true (parentStructureCrossed).
//   - All paragraphs strictly between the two markers are consumed by
//     the cross-structure skip (their start/end tags + content all
//     pass through the skip's event loop).
//   - The paragraph holding moveFromRangeEnd is consumed too: by the
//     time the skip exits, the eventReader is positioned past
//     moveFromRangeEnd inside that paragraph; the trailing events
//     (any content between moveFromRangeEnd and </w:p>, plus the
//     </w:p>) are emitted by the outer loop without a paragraph
//     start. In practice for the 843-3* fixtures upstream produces an
//     empty <w:p></w:p> here (the trailing content is itself
//     revision-tracked <w:del>/<w:ins> that auto-accept-revisions
//     erases). Dropping the wrapper paragraph entirely loses that
//     synthetic empty <w:p> shell — but the difference does not
//     affect translatable content, only document-structural skeleton
//     bytes that the XMLCanonical normalizer compares against the
//     reference. The observed delta on 843-3* is small enough that
//     wrapping the byte-level pass with paragraph-end heuristics
//     (rather than full XML parsing) keeps complexity low.
//
// Pairs are matched by w:id attribute value. Unmatched start markers
// (no corresponding end with matching id, or vice versa) are left
// alone — the writer's stripWMLSkippableElements pass strips the
// stray markers. Self-closing markers (always the schema form for
// these elements per ECMA-376 §CT_MarkupRange) and the explicit
// open+empty-close form are both recognised.
func dropMoveFromRanges(data []byte) []byte {
	const startMarker = "<w:moveFromRangeStart"
	const endMarker = "<w:moveFromRangeEnd"
	if !bytes.Contains(data, []byte(startMarker)) {
		return data
	}
	out := make([]byte, 0, len(data))
	cursor := 0
	for cursor < len(data) {
		startIdx := bytes.Index(data[cursor:], []byte(startMarker))
		if startIdx < 0 {
			out = append(out, data[cursor:]...)
			break
		}
		startIdx += cursor
		// Validate element-name boundary: next byte must be `/`,
		// `>`, or whitespace (rules out e.g. <w:moveFromRangeStartX).
		if !isElementNameBoundary(data, startIdx+len(startMarker)) {
			out = append(out, data[cursor:startIdx+len(startMarker)]...)
			cursor = startIdx + len(startMarker)
			continue
		}
		// Find the closing `>` of the moveFromRangeStart element.
		startTagEnd := bytes.IndexByte(data[startIdx:], '>')
		if startTagEnd < 0 {
			out = append(out, data[cursor:]...)
			break
		}
		startTagEnd += startIdx // absolute position of `>`
		// Extract the w:id="N" value from the start marker.
		id := extractWIDAttr(data[startIdx : startTagEnd+1])
		if id == "" {
			// Malformed start marker — pass through unchanged.
			out = append(out, data[cursor:startTagEnd+1]...)
			cursor = startTagEnd + 1
			continue
		}
		// Find the matching <w:moveFromRangeEnd w:id="N"/> after
		// startTagEnd. Iterate end markers and match by w:id value.
		endStart, endTagEnd := findMoveFromRangeEnd(data, startTagEnd+1, id, endMarker)
		if endStart < 0 {
			// No matching end — leave the start marker in place;
			// the writer strips it. Continue from after the start
			// marker so we don't hunt the same location forever.
			out = append(out, data[cursor:startTagEnd+1]...)
			cursor = startTagEnd + 1
			continue
		}
		// Determine which structural boundaries the span between the
		// two markers crosses. Mirrors upstream's table/row/parent
		// crossed flags (SkippableElements.java lines 415-426):
		//
		//   * crossesTable: a </w:tbl> end tag was traversed without
		//     a matching <w:tbl> start inside the span. Drop the whole
		//     enclosing table — upstream's
		//     removeComponentsFromLastWith(LOCAL_TABLE) + the
		//     TableEnd-branch table drop both fire.
		//
		//   * crossesRow: a </w:tr> end tag was traversed without a
		//     matching <w:tr> start. Drop from <w:tr> of the start
		//     marker through end of moveFromRangeEnd (or </w:tr> of
		//     the row containing it, whichever is later). Mirrors
		//     removeComponentsFromLastWith(LOCAL_TABLE_ROW) plus the
		//     consumed events between rows.
		//
		// Cell-only crossings (</w:tc>) without a row crossing collapse
		// to the row-drop case as well: even a same-row cross-cell
		// moveFromRange leaves the row's translatable content in
		// disarray (cells dropped from delayedTableMarkup), and
		// upstream's outer loop drops the row's downstream cells via
		// the skip's event consumption. The simpler byte-level model
		// drops the whole row.
		crossesTable, crossesRow, crossesCell := spanCrossesTableStructure(data[startTagEnd+1 : endStart])
		if crossesTable || crossesRow || crossesCell {
			scope := "tr"
			if crossesTable {
				scope = "tbl"
			}
			dropFrom := findEnclosingElementOpenStart(data, startIdx, scope)
			if dropFrom < 0 {
				// Defensive: marker is supposed to be inside a row or
				// table but we couldn't find the enclosing element.
				// Bail: leave the start marker, skip past it.
				out = append(out, data[cursor:startTagEnd+1]...)
				cursor = startTagEnd + 1
				continue
			}
			// Drop-to endpoint: extend through </w:tr> (or </w:tbl>)
			// of the element containing moveFromRangeEnd when the end
			// marker sits inside one. Otherwise stop after the end
			// marker itself (sibling-position case).
			dropTo := endTagEnd + 1
			if enclosingClose := findEnclosingElementCloseEnd(data, endTagEnd+1, scope); enclosingClose >= 0 {
				dropTo = enclosingClose
			}
			out = append(out, data[cursor:dropFrom]...)
			cursor = dropTo
			continue
		}
		// Locate the enclosing <w:p> open tag for the start marker
		// (search backwards from startIdx). If startIdx is at body
		// level (not inside any <w:p>), keep startIdx as-is so we
		// only drop from the start marker forward.
		var dropFrom int
		startInsideP := isInsideParagraph(data, startIdx)
		pOpenStartForStart := -1
		if startInsideP {
			pOpenStartForStart = findEnclosingParagraphOpenStart(data, startIdx)
			if pOpenStartForStart < 0 {
				// Defensive: should not happen if isInsideParagraph
				// said yes, but bail safely.
				out = append(out, data[cursor:endTagEnd+1]...)
				cursor = endTagEnd + 1
				continue
			}
			dropFrom = pOpenStartForStart
		} else {
			dropFrom = startIdx
		}
		// Drop endpoint depends on where the end marker sits.
		//
		//   * SAME paragraph as the start marker (no parentStructure
		//     crossed): drop only the byte span between (and
		//     including) the two markers. Mirrors upstream Okapi
		//     SkippableElements.MoveFromRevisionCrossStructure.skip
		//     (SkippableElements.java lines 402-434): the event walk
		//     consumes events from moveFromRangeStart through
		//     moveFromRangeEnd; when no parentStructure (<w:p>) end
		//     tag was traversed, parentStructureCrossed stays false
		//     and BlockParser does NOT mark the block as
		//     skipped(true) (BlockParser.java lines 267-274 only
		//     drops the block when the cross-structure skip marked
		//     it). The surrounding paragraph content (text, <w:ins>
		//     wrappers, <w:moveTo> already-accepted runs, sibling
		//     <w:r>s) survives verbatim. 843-1.docx is the canonical
		//     fixture: <w:moveFromRangeStart> and
		//     <w:moveFromRangeEnd> sit in the same paragraph,
		//     wrapping a single <w:moveFrom><w:r>...</w:r></w:moveFrom>
		//     that gets stripped, leaving "Moved text. Text 1. " (the
		//     accepted <w:moveTo> + plain text + accepted <w:ins>
		//     spaces).
		//
		//   * DIFFERENT paragraphs (parentStructure crossed): extend
		//     the drop through the enclosing </w:p> end tag of the
		//     paragraph containing the end marker, then re-emit a
		//     single synthetic empty <w:p/> in its place. Upstream
		//     BlockParser collapses the cross-structure span into a
		//     single skipped block whose closing tag is the </w:p>
		//     of the last straddled paragraph (lines 267-274 of
		//     BlockParser.java); the empty <w:p/> shell that
		//     remains at the boundary mirrors what upstream emits
		//     verbatim (observed on 843-31/-32 fixtures: a single
		//     `<w:p/>` precedes the trailing <w:sectPr>).
		//
		//   * AT BODY LEVEL (between sibling <w:p> elements, e.g.
		//     843-33/-34 fixtures): drop through the end marker
		//     only so any subsequent sibling paragraph survives
		//     unchanged.
		var dropTo int
		var insertEmptyP bool
		if isInsideParagraph(data, endStart) {
			pOpenStartForEnd := findEnclosingParagraphOpenStart(data, endStart)
			if pOpenStartForEnd < 0 {
				out = append(out, data[cursor:endTagEnd+1]...)
				cursor = endTagEnd + 1
				continue
			}
			if startInsideP && pOpenStartForEnd == pOpenStartForStart {
				// Same paragraph: drop only the marker-to-marker
				// span; the rest of the paragraph survives.
				dropFrom = startIdx
				dropTo = endTagEnd + 1
			} else {
				pCloseEnd := findEnclosingParagraphCloseEnd(data, endTagEnd+1)
				if pCloseEnd < 0 {
					out = append(out, data[cursor:endTagEnd+1]...)
					cursor = endTagEnd + 1
					continue
				}
				dropTo = pCloseEnd
				insertEmptyP = true
			}
		} else {
			dropTo = endTagEnd + 1
		}
		// Drop everything in [dropFrom, dropTo); inject a synthetic
		// empty paragraph if the boundary needs one.
		out = append(out, data[cursor:dropFrom]...)
		if insertEmptyP {
			out = append(out, []byte("<w:p/>")...)
		}
		cursor = dropTo
	}
	return out
}

// isInsideParagraph reports whether the position pos in data falls
// inside an open <w:p>...</w:p> region (i.e. between an unmatched
// <w:p> open tag and its eventual </w:p> close). Linear scan from
// the start of data; suitable for the once-per-call check we need.
func isInsideParagraph(data []byte, pos int) bool {
	const pOpen = "<w:p"
	const pClose = "</w:p>"
	depth := 0
	cursor := 0
	for cursor < pos {
		nextOpen := indexValidElement(data[cursor:pos], pOpen)
		nextClose := bytes.Index(data[cursor:pos], []byte(pClose))
		if nextOpen < 0 && nextClose < 0 {
			return depth > 0
		}
		if nextOpen >= 0 && (nextClose < 0 || nextOpen < nextClose) {
			absOpen := cursor + nextOpen
			tagEnd := bytes.IndexByte(data[absOpen:], '>')
			if tagEnd < 0 {
				return depth > 0
			}
			absOpenEnd := absOpen + tagEnd
			if absOpenEnd > 0 && data[absOpenEnd-1] != '/' {
				depth++
			}
			cursor = absOpenEnd + 1
		} else {
			depth--
			cursor = cursor + nextClose + len(pClose)
		}
	}
	return depth > 0
}

// spanCrossesTableStructure inspects the byte slice between a
// moveFromRangeStart and the matching moveFromRangeEnd and reports
// which table-structural boundaries it crosses. Mirrors upstream
// Okapi's tableRowStructureCrossed / tableStructureCrossed flag
// bookkeeping in SkippableElements.MoveFromRevisionCrossStructure
// (SkippableElements.java lines 415-426): an end-element of the
// given local name with no matching start-element earlier in the
// span flips the corresponding "crossed" flag on.
//
// Returns (crossesTable, crossesRow, crossesCell). The caller picks
// the outermost crossed scope as the drop scope.
func spanCrossesTableStructure(span []byte) (crossesTable, crossesRow, crossesCell bool) {
	crossesCell = spanCrossesElement(span, "tc")
	crossesRow = spanCrossesElement(span, "tr")
	crossesTable = spanCrossesElement(span, "tbl")
	return
}

// spanCrossesElement reports whether the byte slice between a
// moveFromRangeStart and the matching moveFromRangeEnd crosses a
// </w:NAME> end tag without first opening a matching <w:NAME> inside
// the span. A crossing would mean dropping the span verbatim would
// unbalance the structure.
func spanCrossesElement(span []byte, name string) bool {
	open := "<w:" + name
	close := "</w:" + name + ">"
	depth := 0
	cursor := 0
	for cursor < len(span) {
		nextOpen := indexValidElement(span[cursor:], open)
		nextClose := bytes.Index(span[cursor:], []byte(close))
		if nextOpen < 0 && nextClose < 0 {
			return false
		}
		if nextClose < 0 || (nextOpen >= 0 && nextOpen < nextClose) {
			absOpen := cursor + nextOpen
			tagEnd := bytes.IndexByte(span[absOpen:], '>')
			if tagEnd < 0 {
				return false
			}
			absOpenEnd := absOpen + tagEnd
			if absOpenEnd > 0 && span[absOpenEnd-1] != '/' {
				depth++
			}
			cursor = absOpenEnd + 1
			continue
		}
		if depth == 0 {
			return true
		}
		depth--
		cursor = cursor + nextClose + len(close)
	}
	return false
}

// findEnclosingElementOpenStart searches backwards from pos for the
// nearest `<w:NAME>` (or `<w:NAME ...>`) start tag whose matching
// `</w:NAME>` lies AFTER pos. Returns the absolute index of the `<`
// byte, or -1 if pos is not inside any such element. The element-
// name boundary check disambiguates from longer-name siblings (e.g.
// `<w:tr` from `<w:trPr`, `<w:tbl` from `<w:tblGrid`).
func findEnclosingElementOpenStart(data []byte, pos int, name string) int {
	open := "<w:" + name
	close := "</w:" + name + ">"
	depth := 0
	cursor := pos
	for cursor > 0 {
		closeIdx := bytes.LastIndex(data[:cursor], []byte(close))
		openIdx := lastIndexValidElement(data[:cursor], open)
		if openIdx < 0 && closeIdx < 0 {
			return -1
		}
		if openIdx > closeIdx {
			if depth == 0 {
				return openIdx
			}
			depth--
			cursor = openIdx
		} else {
			depth++
			cursor = closeIdx
		}
	}
	return -1
}

// findEnclosingElementCloseEnd searches forward from pos for the
// matching `</w:NAME>` end tag of the enclosing element (depth=0 at
// pos, so we want the first `</w:NAME>` not preceded by an unmatched
// `<w:NAME>`). Returns the absolute index ONE PAST the `>` of the
// end tag, or -1 if no match (i.e. pos is NOT inside an element of
// that name).
func findEnclosingElementCloseEnd(data []byte, pos int, name string) int {
	open := "<w:" + name
	close := "</w:" + name + ">"
	depth := 0
	cursor := pos
	for cursor < len(data) {
		nextOpen := indexValidElement(data[cursor:], open)
		nextClose := bytes.Index(data[cursor:], []byte(close))
		if nextClose < 0 {
			return -1
		}
		if nextOpen >= 0 && nextOpen < nextClose {
			absOpen := cursor + nextOpen
			tagEnd := bytes.IndexByte(data[absOpen:], '>')
			if tagEnd < 0 {
				return -1
			}
			absOpenEnd := absOpen + tagEnd
			if data[absOpenEnd-1] != '/' {
				depth++
			}
			cursor = absOpenEnd + 1
			continue
		}
		if depth == 0 {
			return cursor + nextClose + len(close)
		}
		depth--
		cursor = cursor + nextClose + len(close)
	}
	return -1
}

// isElementNameBoundary reports whether the byte at position pos in
// data is a valid character that can follow an XML element name (so we
// know we matched the full element name and not a prefix).
func isElementNameBoundary(data []byte, pos int) bool {
	if pos >= len(data) {
		return false
	}
	b := data[pos]
	return b == '>' || b == '/' || b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// extractWIDAttr extracts the value of the w:id="..." attribute from
// the given element open-tag bytes (including the leading `<` and
// closing `>`). Returns "" if the attribute is absent or malformed.
func extractWIDAttr(tag []byte) string {
	const attr = "w:id="
	idx := bytes.Index(tag, []byte(attr))
	if idx < 0 {
		return ""
	}
	q := idx + len(attr)
	if q >= len(tag) {
		return ""
	}
	quote := tag[q]
	if quote != '"' && quote != '\'' {
		return ""
	}
	end := bytes.IndexByte(tag[q+1:], quote)
	if end < 0 {
		return ""
	}
	return string(tag[q+1 : q+1+end])
}

// findMoveFromRangeEnd searches data from start onward for the next
// <w:moveFromRangeEnd w:id="id" .../> marker. Returns (startIdx,
// endIdx) where startIdx is the position of the `<` and endIdx is the
// position of the closing `>`. Returns (-1, -1) if no matching marker
// is found.
func findMoveFromRangeEnd(data []byte, from int, id, endMarker string) (int, int) {
	cursor := from
	for cursor < len(data) {
		idx := bytes.Index(data[cursor:], []byte(endMarker))
		if idx < 0 {
			return -1, -1
		}
		idx += cursor
		if !isElementNameBoundary(data, idx+len(endMarker)) {
			cursor = idx + len(endMarker)
			continue
		}
		tagEnd := bytes.IndexByte(data[idx:], '>')
		if tagEnd < 0 {
			return -1, -1
		}
		tagEnd += idx
		if extractWIDAttr(data[idx:tagEnd+1]) == id {
			return idx, tagEnd
		}
		cursor = tagEnd + 1
	}
	return -1, -1
}

// findEnclosingParagraphOpenStart searches backwards from pos for the
// nearest `<w:p>` or `<w:p ...>` start tag whose content has not yet
// been closed by a `</w:p>` between the tag and pos. Returns the
// absolute index of the `<` byte, or -1 if pos is not inside any
// paragraph.
func findEnclosingParagraphOpenStart(data []byte, pos int) int {
	const pOpen = "<w:p"
	const pClose = "</w:p>"
	depth := 0
	cursor := pos
	for cursor > 0 {
		// Find the previous occurrence of either <w:p or </w:p>.
		// Search the substring data[:cursor] from the right.
		closeIdx := bytes.LastIndex(data[:cursor], []byte(pClose))
		// For openIdx we need the LAST occurrence of "<w:p" whose
		// boundary char is `>`, `/`, ` `, `\t`, `\n`, `\r` so we
		// don't match <w:pPr or <w:pict, etc.
		openIdx := lastIndexValidElement(data[:cursor], pOpen)
		if openIdx < 0 && closeIdx < 0 {
			return -1
		}
		// Pick the later of the two; that's the next event going
		// backwards.
		if openIdx > closeIdx {
			if depth == 0 {
				return openIdx
			}
			depth--
			cursor = openIdx
		} else {
			depth++
			cursor = closeIdx
		}
	}
	return -1
}

// lastIndexValidElement returns the last index in data where elemName
// appears followed by a valid element-name boundary character. -1 if
// none found.
func lastIndexValidElement(data []byte, elemName string) int {
	cursor := len(data)
	for cursor > 0 {
		idx := bytes.LastIndex(data[:cursor], []byte(elemName))
		if idx < 0 {
			return -1
		}
		if isElementNameBoundary(data, idx+len(elemName)) {
			return idx
		}
		cursor = idx
	}
	return -1
}

// findEnclosingParagraphCloseEnd searches forward from pos for the
// matching `</w:p>` end tag of the enclosing paragraph (depth=0 at
// pos, so we want the first `</w:p>` not preceded by an unmatched
// `<w:p>`). Returns the absolute index ONE PAST the `>` of the end
// tag (so it can be used as a slice upper bound), or -1 if no match.
func findEnclosingParagraphCloseEnd(data []byte, pos int) int {
	const pOpen = "<w:p"
	const pClose = "</w:p>"
	depth := 0
	cursor := pos
	for cursor < len(data) {
		nextOpen := indexValidElement(data[cursor:], pOpen)
		nextClose := bytes.Index(data[cursor:], []byte(pClose))
		if nextClose < 0 {
			return -1
		}
		if nextOpen >= 0 && nextOpen < nextClose {
			// Stepped into a nested paragraph (rare — paragraphs
			// don't nest in document.xml normally, but they can
			// inside textbox/sdt content). Track depth.
			absOpen := cursor + nextOpen
			tagEnd := bytes.IndexByte(data[absOpen:], '>')
			if tagEnd < 0 {
				return -1
			}
			absOpenEnd := absOpen + tagEnd
			if data[absOpenEnd-1] != '/' {
				depth++
			}
			cursor = absOpenEnd + 1
			continue
		}
		if depth == 0 {
			return cursor + nextClose + len(pClose)
		}
		depth--
		cursor = cursor + nextClose + len(pClose)
	}
	return -1
}

// indexValidElement returns the first index in data where elemName
// appears followed by a valid element-name boundary character. -1 if
// none found.
func indexValidElement(data []byte, elemName string) int {
	cursor := 0
	for cursor < len(data) {
		idx := bytes.Index(data[cursor:], []byte(elemName))
		if idx < 0 {
			return -1
		}
		idx += cursor
		if isElementNameBoundary(data, idx+len(elemName)) {
			return idx
		}
		cursor = idx + len(elemName)
	}
	return -1
}

// dropEmptyTables removes every <w:tbl ...>...</w:tbl> region from data
// whose body contains no <w:tr> child element. This complements
// dropDeletedRows and dropMoveFromRanges: when those passes strip
// every row of a table, the structurally-empty <w:tbl> shell would
// otherwise reach the writer. Upstream Okapi removes these via
// StyledTextPart.process lines 410-424 (the TableEnd branch): if
// delayedTableMarkup has accumulated no translatable block since the
// last <w:tbl>, the entire table-markup component chain is dropped
// via removeComponentsFromLastWith(LOCAL_TABLE).
//
// The pass iterates until fixed-point so that nested tables collapsed
// by an outer-level removal also disappear (a <w:tc> may contain
// another <w:tbl>; if that inner table becomes empty after row drops,
// the outer cell may itself become empty — but cell/row dropping is
// not addressed here, only the strictly-empty table case Okapi
// directly handles).
func dropEmptyTables(data []byte) []byte {
	const tblOpen = "<w:tbl"
	const tblClose = "</w:tbl>"
	if !bytes.Contains(data, []byte(tblOpen)) {
		return data
	}
	out := make([]byte, 0, len(data))
	for {
		idx := bytes.Index(data, []byte(tblOpen))
		if idx < 0 {
			out = append(out, data...)
			break
		}
		// Validate element-name boundary so we don't match <w:tblPr,
		// <w:tblGrid, <w:tblBorders, etc.
		j := idx + len(tblOpen)
		if j >= len(data) {
			out = append(out, data...)
			break
		}
		b := data[j]
		if b != '>' && b != '/' && b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			out = append(out, data[:j+1]...)
			data = data[j+1:]
			continue
		}
		k := bytes.IndexByte(data[j:], '>')
		if k < 0 {
			out = append(out, data...)
			break
		}
		startEnd := j + k
		// Self-closing <w:tbl/> is already empty — drop.
		if startEnd > 0 && data[startEnd-1] == '/' {
			out = append(out, data[:idx]...)
			data = data[startEnd+1:]
			continue
		}
		// Find matching </w:tbl> respecting nested tables.
		bodyStart := startEnd + 1
		depth := 1
		cursor := bodyStart
		for depth > 0 {
			nextOpen := bytes.Index(data[cursor:], []byte(tblOpen))
			nextClose := bytes.Index(data[cursor:], []byte(tblClose))
			if nextClose < 0 {
				out = append(out, data...)
				return out
			}
			if nextOpen >= 0 && nextOpen < nextClose {
				absOpen := cursor + nextOpen
				jj := absOpen + len(tblOpen)
				if jj < len(data) {
					bb := data[jj]
					if bb == '>' || bb == '/' || bb == ' ' || bb == '\t' || bb == '\n' || bb == '\r' {
						kk := bytes.IndexByte(data[jj:], '>')
						if kk < 0 {
							out = append(out, data...)
							return out
						}
						nestedOpenEnd := jj + kk
						if nestedOpenEnd > 0 && data[nestedOpenEnd-1] != '/' {
							depth++
						}
						cursor = nestedOpenEnd + 1
						continue
					}
				}
				cursor = cursor + nextOpen + len(tblOpen)
				continue
			}
			cursor = cursor + nextClose + len(tblClose)
			depth--
		}
		tableEnd := cursor
		body := data[bodyStart : tableEnd-len(tblClose)]
		if !tableBodyHasRow(body) {
			// Empty table — drop the whole region.
			out = append(out, data[:idx]...)
			data = data[tableEnd:]
			continue
		}
		out = append(out, data[:tableEnd]...)
		data = data[tableEnd:]
	}
	return out
}

// tableBodyHasRow reports whether the captured table body contains at
// least one <w:tr> element. The boundary check disambiguates <w:tr from
// <w:trPr/<w:trHeight/<w:trCantSplit etc.
func tableBodyHasRow(body []byte) bool {
	const marker = "<w:tr"
	cursor := 0
	for {
		idx := bytes.Index(body[cursor:], []byte(marker))
		if idx < 0 {
			return false
		}
		j := cursor + idx + len(marker)
		if j >= len(body) {
			return false
		}
		b := body[j]
		if b == '>' || b == '/' || b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			return true
		}
		cursor = j
	}
}

// rowBodyHasMoveFromContent reports whether the captured row body
// contains a <w:moveFrom> revision-tracking content wrapper (ECMA-376
// Part 1 §17.13.5.17 Move From Run Content). The detector explicitly
// disambiguates from <w:moveFromRangeStart and <w:moveFromRangeEnd
// (different element local names) by requiring the next byte after
// `<w:moveFrom` to be a space (attributes follow) or `>`; the wrapper
// form always carries id/author/date attributes per the schema.
func rowBodyHasMoveFromContent(body []byte) bool {
	const marker = "<w:moveFrom"
	cursor := 0
	for {
		idx := bytes.Index(body[cursor:], []byte(marker))
		if idx < 0 {
			return false
		}
		j := cursor + idx + len(marker)
		if j >= len(body) {
			return false
		}
		b := body[j]
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '>' {
			return true
		}
		cursor = j
	}
}

// handleTableRow processes a <w:tr> start element, deciding whether the
// entire row should be dropped because <w:trPr> carries a <w:del> child
// (revision tracking, ECMA-376 Part 1 §17.13.5.13). When a row-deletion
// marker is found AND AutomaticallyAcceptRevisions is true, the helper
// drains tokens through the matching </w:tr> end and emits no skeleton.
//
// If the row is NOT a deletion candidate, the helper emits the <w:tr>
// start element, any whitespace/comments seen before the first child,
// and then either the <w:trPr> raw bytes (if present) or the first
// non-trPr child (re-dispatched). The caller's outer loop continues
// reading the rest of the row's cell content.
//
// Mirrors upstream Okapi StyledTextPart.process() lines 530-551
// (revisionPropertyTableRowDeletedSkippableElements + delayedTableMarkup
// removal) and lines 515-528
// (revisionPropertyTableRowInsertedSkippableElements drain-only).
func (p *wmlParser) handleTableRow(d *xml.Decoder, start xml.StartElement) error {
	// Peek at the first child token. Per ECMA-376 §17.4.79 (CT_Row),
	// the row's child sequence is tblPrEx? trPr? content* — so trPr
	// is at most the second child. We tolerate an optional tblPrEx
	// preceding it. Whitespace between elements is preserved in the
	// skeleton so we capture it as we go.
	var pending []string // serialised whitespace / comments seen before first child

	emitPending := func() {
		for _, s := range pending {
			p.skelText(s)
		}
	}

	// Drain to matching </w:tr> end without emitting anything.
	skipRowToEnd := func() error {
		depth := 1
		for depth > 0 {
			tok, err := d.Token()
			if err != nil {
				return err
			}
			switch tt := tok.(type) {
			case xml.StartElement:
				if tt.Name.Local == "tr" {
					depth++
				}
			case xml.EndElement:
				if tt.Name.Local == "tr" {
					depth--
				}
			}
		}
		return nil
	}

	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch tt := tok.(type) {
		case xml.CharData:
			// xml.CharData backing slice is reused by the decoder; copy via string().
			pending = append(pending, xmlEscape(string(tt)))
		case xml.Comment:
			// xml.Comment backing slice is reused by the decoder; copy via string().
			pending = append(pending, "<!--"+string(tt)+"-->")
		case xml.StartElement:
			// Found the first child element.
			if tt.Name.Local == "trPr" {
				// Capture raw and inspect for a top-level <w:del> child.
				raw, err := captureRawElement(d, tt)
				if err != nil {
					return err
				}
				if trPrHasRowDeletion(raw) {
					// Drain the rest of the row and emit nothing.
					return skipRowToEnd()
				}
				// Not a deleted row — emit row start, any pending
				// whitespace/comments, then the trPr raw. Caller
				// continues normal processing for the rest of the row.
				p.skelWriteStartElement(start)
				emitPending()
				p.skelText(raw)
				return nil
			}
			// First child wasn't trPr — could be tblPrEx or a content
			// cell (no row-property block at all). Either way, the
			// row carries no row-revision marker; emit row start, any
			// pending whitespace, the child start element, then
			// hand back to the outer loop.
			p.skelWriteStartElement(start)
			emitPending()
			return p.dispatchInRow(d, tt)
		case xml.EndElement:
			// Empty row (no children at all). Emit row start and
			// row end, return — caller continues.
			p.skelWriteStartElement(start)
			emitPending()
			p.skelWriteEndElement(tt)
			return nil
		}
	}
}

// dispatchInRow forwards a start element seen as the first non-trPr
// child of <w:tr> to the appropriate parsePart handler. Mirrors the
// switch in parsePart for the elements that legitimately appear inside
// a row (typically <w:tc> via the default branch, or another
// <w:trPr>-less child).
func (p *wmlParser) dispatchInRow(d *xml.Decoder, t xml.StartElement) error {
	switch t.Name.Local {
	case "tcPr":
		raw, err := captureRawElement(d, t)
		if err != nil {
			return err
		}
		p.skelText(raw)
	default:
		p.skelWriteStartElement(t)
	}
	return nil
}

// trPrHasRowDeletion reports whether raw (the captured XML of a
// <w:trPr> element) contains a top-level <w:del> child — the row
// deletion revision marker per ECMA-376 Part 1 §17.13.5.13. Top-level
// is determined by a single-element-deep scan: the marker appears as
// a direct child of <w:trPr>, not inside any nested element. The
// scan tolerates whitespace, attribute variations, and self-closing
// or open/close empty forms.
//
// Mirrors upstream Okapi's
// SkippableElement.RevisionProperty.TABLE_ROW_DELETED entry
// (SkippableElement.java line 245) keyed on QName "del" with
// parent QName "trPr" via
// SkippableElements.RevisionProperty.CONTEXT_AWARE_REVISION_SKIPPABLE_ELEMENTS
// (SkippableElements.java line 528-531).
func trPrHasRowDeletion(raw string) bool {
	// Strip the outer <w:trPr ...> and </w:trPr> wrapper, then scan
	// only the immediate-child layer for <w:del. We use a simple
	// depth tracker since the trPr content is small (revision
	// markers, height, cantSplit, etc.) and rarely deeply nested.
	dec := xml.NewDecoder(strings.NewReader(raw))
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		switch tt := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 2 && tt.Name.Local == "del" {
				return true
			}
		case xml.EndElement:
			depth--
			if depth == 0 {
				return false
			}
		}
	}
}

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
						// Strip per-run rPrChildren whose canonical XML
						// matches the resolved style chain. Mirrors
						// upstream Okapi RunProperties.minified()'s
						// `if (preCombined.contains(p))` branch
						// (RunProperties.java:497-540): a directly-
						// authored property is dropped from the run
						// when the resolved chain already supplies it
						// with the SAME value. Today this strip covers
						// `<w:rFonts>` (the canonical case is
						// 948-1.docx, where every `Character1`-styled
						// run authors the same `<w:rFonts ...>` the
						// Character1 style supplies via its basedOn
						// chain). Other rPr children (`<w:color>`,
						// `<w:sz>`, …) are not yet covered because
						// their canonical XML is not captured into
						// styleEntry.rPrChildXMLs — extend the parser
						// + this strip to cover them as parity gaps
						// surface. Per ECMA-376-1 §17.3.2.26 (CT_Fonts)
						// the rFonts element identity is its attribute
						// set, so byte-equal canonicalised XML is a
						// safe equality check.
						if rStyleID != "" || paraStyleID != "" {
							children := runs[i].props.rPrChildren
							out := children[:0]
							for _, c := range children {
								if c.name == "rFonts" {
									chainXML := ""
									if rStyleID != "" {
										chainXML = p.styles.effectiveRPrChildXML(rStyleID, "rFonts")
									}
									if chainXML == "" && paraStyleID != "" {
										chainXML = p.styles.effectiveRPrChildXML(paraStyleID, "rFonts")
									}
									if chainXML != "" && chainXML == c.xml {
										continue
									}
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
				// emits these on every <w:r> for the block (#592), and
				// the WSO post-pass then lifts them into a synthesised
				// paragraph style when the threshold conditions are
				// met (#589 / style_optimization.go).
				commonRPr := commonRPrChildren(runs)
				commonRPrXML := joinRPrChildren(commonRPr)

				// Merge adjacent runs with mergeable rPr (mirrors
				// upstream Okapi RunMerger.canRunPropertiesBeMerged
				// at RunMerger.java:156-229). mergeRuns updates the
				// surviving textRun's rPrChildren to the merged
				// per-attribute union so the sidecars below see the
				// post-merge consensus props.
				merged := mergeRuns(runs)
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
					if paragraphHasDeletedMark(paraProps) && len(merged) == 0 {
						return nil
					}
					p.skelWriteString("<w:p>")
					if paraProps != "" {
						p.skelText(paraProps)
					}
					for _, r := range merged {
						p.writeRunToSkel(r, partPath, emitBlock)
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
					p.skelWriteString("<w:p>")
					if paraProps != "" {
						p.skelText(paraProps)
					}
					// Write runs as skeleton text
					for _, r := range merged {
						p.skelText(runToXML(r))
					}
					p.skelWriteString("</w:p>")
					return nil
				}

				// Build block
				*p.blockCounter++
				blockID := fmt.Sprintf("tu%d", *p.blockCounter)

				// Skeleton: write paragraph open, props, ref, close
				p.skelWriteString("<w:p>")
				if paraProps != "" {
					p.skelText(paraProps)
				}
				p.skelRef(blockID)
				p.skelWriteString("</w:p>")

				block := p.buildBlock(blockID, merged, partPath, commonRPrXML, perRunRPrXML, perRunSrcRunStart)
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
				props, err = parseRunPropsFromRaw(rPrRaw, p.cfg.AggressiveCleanup, p.strict, p.currentStyleChainNames)
				if err != nil {
					return nil, err
				}

			case "fldChar":
				hasFieldMarkup = true
				startRawCapture()
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
					rawBuf.WriteString(xmlEscapeAttr(a.Value))
					rawBuf.WriteString(`"`)
				}
				rawBuf.WriteString(">")
				// Field instruction text — extract the field code name
				text, err := readCharData(d)
				if err != nil {
					return nil, err
				}
				rawBuf.WriteString(xmlEscape(text))
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
						rawBuf.WriteString(xmlEscapeAttr(a.Value))
						rawBuf.WriteString(`"`)
					}
					rawBuf.WriteString(">")
				}
				text, err := readCharData(d)
				if err != nil {
					return nil, err
				}
				if rawCaptured {
					rawBuf.WriteString(xmlEscape(text))
					rawBuf.WriteString("</")
					writeElementName(&rawBuf, t.Name)
					rawBuf.WriteString(">")
				}
				_ = hasProps
				// Tag display-text runs inside an extractable complex
				// field's result region so mergeRuns honours the
				// source's per-<w:r> boundary. See textRun.inFieldDisplay
				// for the upstream-Okapi rationale (parseComplexField
				// captures these as RunText body chunks inside the
				// field's RunBuilder, separated by Markup chunks
				// preserving the source `</w:r><w:r>` boundaries —
				// they do NOT pass through RunMerger.canMergeWith).
				inField := cfs.active && cfs.extractable && cfs.atResult
				runs = append(runs, textRun{text: text, props: props, inFieldDisplay: inField})

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
					brXML.WriteString(xmlEscapeAttr(a.Value))
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
				// the rPr so the upstream WSO post-pass sees vanish
				// and lifts it into the synthesised paragraph style
				// (PageBreak.docx — `<w:r><w:rPr><w:vanish/></w:rPr>
				// <w:br w:type="page"/></w:r>` must round-trip with
				// the vanish in place; both reference and native then
				// promote it into NF974E24F-Standard1).
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
						rawBuf.WriteString(xmlEscapeAttr(a.Value))
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
					rawBuf.WriteString(xmlEscapeAttr(a.Value))
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
						rawBuf.WriteString(xmlEscapeAttr(a.Value))
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
				if hasFieldMarkup {
					rawBuf.WriteString("</")
					writeElementName(&rawBuf, t.Name)
					rawBuf.WriteString(">")
					// Replace any decoded child-runs with a single
					// SubTypeFieldChar sentinel carrying the verbatim
					// <w:r>...</w:r> payload so the writer can emit it
					// untouched. parseRunPropsFromRaw still populated
					// `props` so the run participates correctly in
					// downstream merging / common-rPr computation, but
					// the payload itself is opaque.
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

// parseSDT parses a structured document tag, extracting its content.
func (p *wmlParser) parseSDT(d *xml.Decoder, partPath string, emitBlock func(*model.Block), emitData func()) error {
	// The caller's case branch consumed the `<w:sdt>` start element. Write
	// it to the skeleton so the writer re-emits the SDT envelope on
	// round-trip; bridge preserves <w:sdt><w:sdtContent>...</w:sdtContent>
	// </w:sdt> wrappers around block-level paragraphs (e.g. watermark
	// header2.xml contains a single paragraph wrapped in sdt). Per
	// ECMA-376-1 §17.5.2.31 (CT_SdtBlock) the sdt is a structural envelope
	// for content controls; dropping it on round-trip changes the document
	// structure and breaks the byte-equivalence guarantee.
	//
	// <w:sdtPr> (the SDT properties block — id, alias, dataBinding, …) and
	// <w:sdtEndPr> (post-content rPr) are captured raw because their
	// children carry attributes the streaming skeleton emit would not
	// preserve byte-for-byte. <w:sdtContent> is the content envelope; the
	// inner <w:p> paragraphs route through parseParagraph normally and
	// emit their block refs into the skeleton between the wrapper markers.
	// Nested <w:sdt> inside <w:sdtContent> recurses (Practice2.docx
	// footer2.xml).
	depth := 1
	inContent := false

	p.skelText("<w:sdt>")

	// Buffer for sdtPr / sdtEndPr captured payloads — they appear before
	// the content and must be emitted between `<w:sdt>` and
	// `<w:sdtContent>`.
	var preContent strings.Builder

	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "sdtContent":
				p.skelText(preContent.String())
				preContent.Reset()
				inContent = true
				p.skelText("<w:sdtContent>")
			case "sdtPr", "sdtEndPr":
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				preContent.WriteString(raw)
				depth--
			case "sdt":
				if err := p.parseSDT(d, partPath, emitBlock, emitData); err != nil {
					return err
				}
				depth--
			case "p":
				if inContent {
					if err := p.parseParagraph(d, partPath, emitBlock); err != nil {
						return err
					}
					depth--
				}
			}
		case xml.EndElement:
			depth--
			if t.Name.Local == "sdtContent" {
				inContent = false
				p.skelText("</w:sdtContent>")
			}
		}
	}
	if preContent.Len() > 0 {
		p.skelText(preContent.String())
	}
	p.skelText("</w:sdt>")
	return nil
}

// sdtEndPrIsEmpty reports whether a captured `<w:sdtEndPr ...>` element
// has no child elements (either self-closing `<w:sdtEndPr/>` or empty
// body `<w:sdtEndPr></w:sdtEndPr>`). Per ECMA-376-1 \u00A717.5.2.38
// (CT_SdtEndPr) the element carries `<w:rPr>` children defining the
// post-content run properties; in practice most authoring tools emit
// an empty sdtEndPr that upstream Okapi drops on round-trip (the
// RunContainer SDT path filters the empty form out via the
// SDT_END_PROPERTIES skippable set).
//
// Used by parseInlineSDT to suppress the empty form when wrapping the
// SDT envelope into the paired-code OPEN sentinel \u2014 keeping the
// non-empty form for fidelity in the rare cases it carries children.
// 1085.docx is the canonical empty-form fixture.
func sdtEndPrIsEmpty(raw string) bool {
	// Self-closing form: `<w:sdtEndPr/>` or `<w:sdtEndPr ... />`.
	if strings.HasSuffix(strings.TrimRight(raw, " \t\r\n"), "/>") {
		return true
	}
	// Empty-body form: `<w:sdtEndPr ...></w:sdtEndPr>` with only whitespace
	// (or nothing) between the open and close tags.
	openEnd := strings.IndexByte(raw, '>')
	if openEnd < 0 {
		return false
	}
	body := raw[openEnd+1:]
	closeIdx := strings.LastIndex(body, "</")
	if closeIdx < 0 {
		return false
	}
	return strings.TrimSpace(body[:closeIdx]) == ""
}

// parseInlineSDT drains an inline `<w:sdt>` wrapper, processing its
// child runs as if they were direct paragraph children and emitting
// paired-code sentinels (\uE10E open / \uE10F close, shared with the
// strict-OOXML revision-insertion path) around them so the writer can
// re-emit the SDT envelope verbatim.
//
// The OPEN sentinel carries the captured raw `<w:sdt ...>` start tag
// followed by every child verbatim up to the matching `</w:sdtContent>`
// open boundary — i.e. the captured `<w:sdtPr>...</w:sdtPr>`, the
// optional `<w:sdtEndPr/>`, and the `<w:sdtContent>` start tag itself.
// The CLOSE sentinel emits the literal `</w:sdtContent></w:sdt>` close
// pair. Inner runs of `<w:sdtContent>` are parsed inline and live in
// the textRun slice between the OPEN and CLOSE sentinels.
//
// When `<w:sdtContent>` is self-closing (no inner runs at all — the
// 1085.docx fixture: `<w:sdt><w:sdtPr><w:tag/><w:id/></w:sdtPr>
// <w:sdtEndPr/><w:sdtContent/></w:sdt>`), the OPEN sentinel emits
// `<w:sdt><w:sdtPr>...</w:sdtPr><w:sdtEndPr/><w:sdtContent>` and the
// CLOSE emits `</w:sdtContent></w:sdt>`; the empty
// `<w:sdtContent></w:sdtContent>` is canonical-equivalent to the
// self-closing form (XML canonicalisation collapses an empty element
// to its self-closing variant).
//
// Mirrors upstream Okapi RunContainer (RunContainer.java:97-176),
// which preserves <w:sdt>, <w:sdtPr>, <w:sdtEndPr>, and <w:sdtContent>
// as outer/inner markup around the extracted inner content
// (RunContainer.RUN_CONTAINER_TYPES + the sdt-specific properties handler).
// Per ECMA-376 Part 1 / ISO/IEC 29500-1 §17.5.2 (Structured Document
// Tags), `<w:sdtPr>` and `<w:sdtEndPr>` carry SDT metadata (id, tag,
// alias, …) that must round-trip; `<w:sdtContent>` wraps the placeholder
// content.
//
// rawStart is the raw XML form of the `<w:sdt ...>` open tag (including
// any attributes) produced by the caller via startElementToRaw.
func (p *wmlParser) parseInlineSDT(d *xml.Decoder, runs *[]textRun, rawStart string) error {
	// Capture sdtPr (always present per CT_SdtRun) and the optional
	// sdtEndPr verbatim, then accumulate them onto rawStart so the
	// OPEN sentinel emits the full `<w:sdt><w:sdtPr>...</w:sdtPr>
	// <w:sdtEndPr/><w:sdtContent>` prefix.
	wrapperOpen := rawStart
	inSdtContent := false
	for !inSdtContent {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "sdtPr":
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				wrapperOpen += raw
			case "sdtEndPr":
				// Empty <w:sdtEndPr/> is dropped by upstream Okapi
				// (RunContainer.SDT_END_PROPERTIES filter — only the
				// non-trivial members survive). When sdtEndPr carries
				// child elements (rare), preserve verbatim.
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				// Self-closing or empty body: drop. Otherwise keep.
				if !sdtEndPrIsEmpty(raw) {
					wrapperOpen += raw
				}
			case "sdtContent":
				wrapperOpen += startElementToRaw(t)
				inSdtContent = true
			default:
				// Unknown SDT child outside sdtContent — skip the
				// subtree to keep round-trip safe; future fixtures
				// can add handling here.
				if err := skipElement(d); err != nil {
					return err
				}
			}
		case xml.EndElement:
			// Premature `</w:sdt>` — the SDT had no sdtContent at all.
			// Emit a single-shot pair: the OPEN sentinel carries the
			// rawStart + captured sdtPr/sdtEndPr; the CLOSE sentinel
			// emits a synthesised empty `</w:sdt>` pair (no
			// sdtContent boundary because the source had none).
			if t.Name.Local == "sdt" {
				*runs = append(*runs, textRun{text: "\uE10E:sdt-no-content:" + wrapperOpen, props: runProps{}})
				*runs = append(*runs, textRun{text: "\uE10F:sdt-no-content:</w:sdt>", props: runProps{}})
				return nil
			}
		}
	}
	// Emit the OPEN sentinel covering everything through the
	// `<w:sdtContent>` start tag.
	*runs = append(*runs, textRun{text: "\uE10E:sdt:" + wrapperOpen, props: runProps{}})
	// Drain `<w:sdtContent>` children, processing inner runs inline.
	var cfs complexFieldState
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "r":
				rawRStart := startElementToRaw(t)
				r, err := p.parseRunWithFieldState(d, &cfs, rawRStart)
				if err != nil {
					return err
				}
				*runs = append(*runs, r...)
			default:
				// Unknown child inside sdtContent — skip subtree.
				if err := skipElement(d); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "sdtContent" {
				// Now drain to the matching `</w:sdt>` (no children
				// expected after sdtContent per CT_SdtRun, but be
				// defensive).
				for {
					tok2, err := d.Token()
					if err != nil {
						return err
					}
					if et, ok := tok2.(xml.EndElement); ok && et.Name.Local == "sdt" {
						*runs = append(*runs, textRun{text: "\uE10F:sdt:</w:sdtContent></w:sdt>", props: runProps{}})
						return nil
					}
				}
			}
		}
	}
}

// wrapHyperlinkRuns wraps runs in hyperlink opening/closing markers.
//
// The emitted <w:hyperlink> start tag mirrors upstream Okapi's preserved
// startMarkup (RunContainer.java:97-99, getEvents() lines 168-176): every
// non-`r:id` attribute on the source <w:hyperlink> survives the round-
// trip, including w:tooltip, w:history, w:anchor, w:docLocation, and
// w:tgtFrame (ECMA-376-1 \u00A717.16.22 CT_Hyperlink). The native pipeline
// previously reconstructed the tag from `relID` alone and synthesised a
// non-OOXML `href=...` attribute, which dropped tooltip/history and
// added a spurious href that the reference output never carries
// (830-7.docx, 952-1.docx, 952-2.docx, hyperlink.docx,
// external_hyperlink.docx, 1341-textbox-with-a-hyperlink.docx).
func (p *wmlParser) wrapHyperlinkRuns(runs []textRun, relID string, extraAttrs []xml.Attr) []textRun {
	// Build <w:hyperlink> opening tag preserving every captured
	// attribute. The relID feeds the r:id attribute; the remaining
	// attributes come from extraAttrs in source order.
	var b strings.Builder
	b.WriteString("<w:hyperlink")
	if relID != "" {
		b.WriteString(` r:id="`)
		b.WriteString(xmlEscapeAttr(relID))
		b.WriteString(`"`)
	}
	for _, a := range extraAttrs {
		b.WriteString(" ")
		writeAttrName(&b, a.Name)
		b.WriteString(`="`)
		b.WriteString(xmlEscapeAttr(a.Value))
		b.WriteString(`"`)
	}
	b.WriteString(">")
	data := b.String()

	// Create wrapper with sentinel markers
	var result []textRun
	result = append(result, textRun{text: "\uE103:" + data, props: runProps{}}) // hyperlink open sentinel
	result = append(result, runs...)
	result = append(result, textRun{text: "\uE104:" + data, props: runProps{}}) // hyperlink close sentinel
	return result
}

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
		b.WriteString("<w:vanish/>")
	}
	for _, c := range p.rPrChildren {
		b.WriteString(c.xml)
	}
	b.WriteString("</w:rPr>")
	return b.String()
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
	b := &runBuilder{}
	spanCounter := 0

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
				activeProps.appendClosingRuns(b, &spanCounter)
				activeProps = nil
			}
			spanCounter++
			b.AddPh(fmt.Sprintf("c%d", spanCounter),
				TypeTab, SubTypeTab,
				"<w:tab/>", "\t", "",
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
			spanCounter++
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
			b.AddPh(fmt.Sprintf("c%d", spanCounter),
				TypeImage, subType,
				data, "", "",
				false, false, false)
			continue
		}
		if strings.HasPrefix(run.text, "\uE10D:") {
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
			rawXML := strings.TrimPrefix(run.text, "\uE10D:")
			subType := SubTypeNoBreakHyphen
			switch {
			case strings.Contains(rawXML, "softHyphen"):
				subType = SubTypeSoftHyphen
			case strings.Contains(rawXML, "cr"):
				subType = SubTypeCR
			}
			spanCounter++
			b.AddPh(fmt.Sprintf("c%d", spanCounter),
				TypeRawRunMarkup, subType,
				rawXML, "", "",
				false, false, false)
			continue
		}
		if strings.HasPrefix(run.text, "\uE102:") {
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
			rest := strings.TrimPrefix(run.text, "\uE102:")
			markerElem := "footnoteReference"
			if strings.HasPrefix(rest, "f:") {
				rest = strings.TrimPrefix(rest, "f:")
			} else if strings.HasPrefix(rest, "e:") {
				rest = strings.TrimPrefix(rest, "e:")
				markerElem = "endnoteReference"
			}
			noteID := rest
			spanCounter++
			data := fmt.Sprintf(`<w:%s w:id="%s"/>`, markerElem, noteID)
			if rPr := serializeRPrChildrenXML(run.props); rPr != "" {
				data = rPr + data
			}
			b.AddPh(fmt.Sprintf("c%d", spanCounter),
				TypeFootnoteRef, SubTypeFootnoteRef,
				data,
				"",
				fmt.Sprintf("[%s]", noteID),
				false, false, false)
			continue
		}
		if strings.HasPrefix(run.text, "\uE103:") {
			// Hyperlink open
			data := strings.TrimPrefix(run.text, "\uE103:")
			spanCounter++
			b.AddPcOpen(fmt.Sprintf("c%d", spanCounter),
				TypeHyperlink, SubTypeHyperlink,
				data, "", "",
				true, true, true)
			continue
		}
		if strings.HasPrefix(run.text, "\uE104:") {
			// Hyperlink close
			if activeProps != nil && !activeProps.isEmpty() {
				// Close formatting before hyperlink close
				activeProps.appendClosingRuns(b, &spanCounter)
				activeProps = nil
			}
			spanCounter++
			b.AddPcClose(fmt.Sprintf("c%d", spanCounter),
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
				activeProps.appendClosingRuns(b, &spanCounter)
				activeProps = nil
			}
			data := strings.TrimPrefix(run.text, "\uE109:")
			spanCounter++
			b.AddPcOpen(fmt.Sprintf("c%d", spanCounter),
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
				activeProps.appendClosingRuns(b, &spanCounter)
				activeProps = nil
			}
			data := strings.TrimPrefix(run.text, "\uE10A:")
			spanCounter++
			b.AddPcClose(fmt.Sprintf("c%d", spanCounter),
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
				activeProps.appendClosingRuns(b, &spanCounter)
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
			spanCounter++
			b.AddPcOpen(fmt.Sprintf("c%d", spanCounter),
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
				activeProps.appendClosingRuns(b, &spanCounter)
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
			spanCounter++
			b.AddPcClose(fmt.Sprintf("c%d", spanCounter),
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
				activeProps.appendClosingRuns(b, &spanCounter)
				activeProps = nil
			}
			subType := SubTypeBookmarkStart
			if strings.HasPrefix(run.text, "\uE107:") {
				subType = SubTypeBookmarkEnd
			}
			spanCounter++
			b.AddPh(fmt.Sprintf("c%d", spanCounter),
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
				activeProps.appendClosingRuns(b, &spanCounter)
				activeProps = nil
			}
			subType := SubTypeCommentRangeStart
			if strings.HasPrefix(run.text, "\uE10C:") {
				subType = SubTypeCommentRangeEnd
			}
			spanCounter++
			b.AddPh(fmt.Sprintf("c%d", spanCounter),
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
				activeProps.appendClosingRuns(b, &spanCounter)
				activeProps = nil
			}
			subType := SubTypeFieldChar
			if strings.HasPrefix(run.text, "\uE108:fldSimple") {
				subType = SubTypeFieldSimple
			}
			spanCounter++
			b.AddPh(fmt.Sprintf("c%d", spanCounter),
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
			spanCounter++
			b.AddPh(fmt.Sprintf("c%d", spanCounter),
				TypeBreak, subType,
				brXML, "\n", "",
				false, false, false)
			continue
		}

		// Handle formatting changes
		if activeProps == nil || !activeProps.equal(run.props) {
			// Close previous formatting
			emittedClose := false
			if activeProps != nil && !activeProps.isEmpty() {
				activeProps.appendClosingRuns(b, &spanCounter)
				emittedClose = true
			}
			// Open new formatting
			emittedOpen := false
			if !run.props.isEmpty() {
				run.props.appendOpeningRuns(b, &spanCounter)
				emittedOpen = true
			}
			// When neither close nor open emitted any toggle codes the
			// run boundary is invisible to runBuilder's text-coalescing
			// path — AddText would append into the previous TextRun and
			// lose the source-run boundary. This happens when adjacent
			// source runs share toggle props (both empty) but differ on
			// font name (rFonts ascii vs asciiTheme — fixture
			// 1312-fonts-info.docx) or other non-toggle properties that
			// runProps.equal() inspects (just fontName today, but the
			// rule is "any !equal() that emits no markers"). Force a
			// model.Run boundary so the per-source-run rPr sidecar
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
		activeProps.appendClosingRuns(b, &spanCounter)
	}

	// Apply code finder before block construction so the placeholder
	// runs it inserts land in the builder's run sequence alongside the
	// formatting runs.
	blockRuns := b.Runs()
	if p.codeFinder != nil {
		blockRuns = p.codeFinder.applyToRuns(blockRuns, &spanCounter)
	}

	block := &model.Block{
		ID:           id,
		Type:         "paragraph",
		Translatable: true,
		Source:       []*model.Segment{model.NewRunsSegment("s1", blockRuns)},
		Targets:      make(map[model.LocaleID][]*model.Segment),
		Properties:   map[string]string{"partPath": partPath},
		Annotations:  make(map[string]model.Annotation),
	}

	// Collect font info if configured
	if p.cfg.ExtractRunFontsInfo {
		fonts := collectFonts(runs)
		if fonts != "" {
			block.Annotations["fonts"] = &model.GenericAnnotation{
				Kind:   "fonts",
				Fields: map[string]any{"names": fonts},
			}
		}
	}

	// Stash the common per-source-run rPr children for the writer (#592).
	// The writer prepends this XML to every emitted <w:r>'s <w:rPr>; the
	// WSO post-pass then lifts it into a synthesised paragraph style when
	// the optimisation conditions are met (mirroring upstream Okapi
	// StyleOptimisation.Default.applyTo, see StyleOptimisation.java
	// lines 96-129 of okapi-filter-openxml).
	if commonRPrXML != "" {
		block.Annotations[openxmlSourceRPrAnnotationKey] = &model.GenericAnnotation{
			Kind:   openxmlSourceRPrAnnotationKey,
			Fields: map[string]any{"xml": commonRPrXML},
		}
	}

	// Stash the per-text-run rPr fragments sidecar (Phase 1 of the
	// per-run rPr work — see PARITY_NOTES.md "1083-*" cluster). The
	// writer wire-up (Phase 2) consumes this annotation; until then it
	// is read-only sidecar data and does not change writer behaviour.
	if len(perRunRPrXML) > 0 {
		block.Annotations[openxmlPerRunRPrAnnotationKey] = &model.GenericAnnotation{
			Kind:   openxmlPerRunRPrAnnotationKey,
			Fields: map[string]any{"fragments": perRunRPrXML},
		}
	}

	// Stash the per-text-run "starts new source <w:r>" boolean sidecar
	// so the writer can decide whether a text run reuses the still-open
	// <w:r> from a preceding standalone <w:br/> / <w:tab/> Ph or opens
	// a fresh <w:r>. See openxmlPerRunSrcRunStartAnnotationKey.
	if len(perRunSrcRunStart) > 0 {
		block.Annotations[openxmlPerRunSrcRunStartAnnotationKey] = &model.GenericAnnotation{
			Kind:   openxmlPerRunSrcRunStartAnnotationKey,
			Fields: map[string]any{"flags": perRunSrcRunStart},
		}
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
			block.Annotations[openxmlPerRunInFieldDisplayAnnotationKey] = &model.GenericAnnotation{
				Kind:   openxmlPerRunInFieldDisplayAnnotationKey,
				Fields: map[string]any{"flags": perRunInFieldDisplay},
			}
		}
	}

	return block
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
		if current.props.canBeMergedWith(r.props) {
			current.text += r.text
			// Replace the kept run's rPrChildren with the merged
			// per-attribute union of rFonts so downstream sidecars
			// (perRunRPrFragments) see the consensus rFonts.
			if !current.props.equalIncludingChildren(r.props) {
				current.props.rPrChildren = mergeRPrChildren(
					current.props.rPrChildren, r.props.rPrChildren)
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
	r := []rune(s)
	if len(r) == 0 {
		return false
	}
	// Sentinel range covers all reserved Private Use Area code points
	// used by the WML reader: \uE100 (tab) through \uE10F (revision-
	// insertion close). Extending the range past \uE10D requires the
	// matching dispatch in buildBlock \u2014 see the \uE10E / \uE10F
	// (revision-insertion paired-code OPEN/CLOSE) cases there.
	if r[0] < '\uE100' || r[0] > '\uE10F' {
		return false
	}
	// Single-char sentinels (tab \uE100, image \uE101, paragraph
	// opaque \uE105). Note: \uE105 wraps math (m:oMathPara/m:oMath)
	// or paragraph-level mc:AlternateContent \u2014 content that is a
	// direct <w:p> child rather than a <w:r> child, so the writer
	// must not wrap it in <w:r> when re-emitting.
	if len(r) == 1 {
		return true
	}
	// Multi-char sentinels must have ':' separator
	// (\uE102:id, \uE103:data, \uE104:data, \uE106:id, \uE107:id,
	// \uE108:fldChar / \uE108:fldSimple, \uE109:data, \uE10A:data,
	// \uE10B:id, \uE10C:id, \uE10D:rawXML, \uE10E:wrapper:rawStart,
	// \uE10F:wrapper:rawEnd)
	return len(r) >= 2 && r[1] == ':'
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
	r := []rune(text)
	if len(r) == 0 {
		return false
	}
	return r[0] == '\uE108'
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

// dropTextRuns removes plain translatable runs from a slice while
// keeping every sentinel run (field markup, drawings, bookmarks, \u2026).
// Mirrors upstream Okapi's parseComplexField branching at lines 501-
// 506 of RunParser.java where, when the field is non-extractable or
// the reader is still before the separator, content events are routed
// to runBuilder.addToMarkup (preserved as opaque markup) rather than
// to the run text. Translatable text alongside the field markup never
// reaches the block, but the field markup itself does.
func dropTextRuns(runs []textRun) []textRun {
	out := runs[:0]
	for _, r := range runs {
		if isSentinel(r.text) {
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
	r := []rune(text)
	if len(r) == 0 {
		return false
	}
	return r[0] == '\uE10B' || r[0] == '\uE10C'
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
	r := []rune(text)
	if len(r) == 0 {
		return false
	}
	return r[0] == '' || r[0] == ''
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
	r := []rune(text)
	if len(r) == 0 {
		return false
	}
	return r[0] == '' || r[0] == ''
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
func allHidden(runs []textRun, inheritedVanish bool) bool {
	for _, r := range runs {
		if isSentinel(r.text) {
			continue
		}
		if !r.props.vanish && !inheritedVanish && strings.TrimSpace(r.text) != "" {
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
	// the skeleton emit path (parseParagraph isEmptyRuns branch). The
	// downstream WSO post-pass then sees a stripped-down `<w:rPr>` and
	// silently synthesises a paragraph style from the surviving toggle
	// (e.g. lang.docx's editform-styled space run: source rPr was
	// `<w:rStyle w:val="editform"/><w:b/><w:vanish w:val="0"/>...`,
	// stripped to just `<w:b/>` here, then WSO lifts the `<w:b/>` into
	// a synthesised pStyle). Per ECMA-376-1 §17.3.2.1 (CT_R) every
	// rPr child applies to the run regardless of the run's payload
	// (text vs whitespace vs drawing); upstream Okapi RunBuilder
	// materialises the full source RunProperties on every emitted run.
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
		if strings.HasPrefix(rest, "f:") {
			rest = strings.TrimPrefix(rest, "f:")
		} else if strings.HasPrefix(rest, "e:") {
			rest = strings.TrimPrefix(rest, "e:")
			markerElem = "endnoteReference"
		}
		buf.WriteString(fmt.Sprintf(`<w:%s w:id="%s"/>`, markerElem, rest))
	default:
		buf.WriteString(`<w:t xml:space="preserve">`)
		buf.WriteString(xmlEscape(r.text))
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
				p.writeStartElementWithTranslatableAttrTo(out, t, "name", "drawing-name", partPath, emitBlock)
			case t.Name.Local == "textpath":
				p.writeStartElementWithTranslatableAttrTo(out, t, "string", "vml-textpath-string", partPath, emitBlock)
			case t.Name.Local == "txbxContent":
				writeRawStartElementTo(out, t)
				if err := p.extractTxbxContent(dec, out, t, partPath, emitBlock); err != nil {
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
			default:
				writeRawStartElementTo(out, t)
			}
		case xml.EndElement:
			if t.Name.Local == drawingDecodeWrapperLocal {
				return nil
			}
			writeRawEndElementTo(out, t)
		case xml.CharData:
			out.WriteString(xmlEscape(string(t)))
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
// When a <w:p> contains a complex field (`<w:fldChar>`), the
// paragraph is preserved verbatim — parseParagraph's existing
// non-extractable-field path drops the field markup along with
// its display runs. Falling back to verbatim keeps round-trip
// safe (TextboxNumber.docx with PAGE \* MERGEFORMAT is the
// canonical fixture for this corner).
func (p *wmlParser) extractTxbxContent(
	dec *xml.Decoder,
	out *strings.Builder,
	start xml.StartElement,
	partPath string,
	emitBlock func(*model.Block),
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
				if containsComplexField(rawP) {
					// Preserve the field-bearing paragraph verbatim
					// (rawP from captureRawElement is the full
					// paragraph including its open and close tags).
					// parseParagraph's existing non-extractable-field
					// path drops both the field display runs AND the
					// fldChar markers themselves, which would lose
					// markup like PAGE \* MERGEFORMAT. Verbatim
					// preservation is round-trip safe for textboxes.
					// TextboxNumber.docx is the canonical fixture.
					out.WriteString(rawP)
					continue
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
				if err := p.extractTxbxParagraph(idec, out, partPath, emitBlock); err != nil {
					return err
				}
			} else if t.Name.Local == "tbl" || t.Name.Local == "tr" || t.Name.Local == "tc" {
				writeRawStartElementTo(out, t)
				if err := p.extractTxbxContent(dec, out, t, partPath, emitBlock); err != nil {
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
			out.WriteString(xmlEscape(string(t)))
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
func (p *wmlParser) extractTxbxParagraph(dec *xml.Decoder, out *strings.Builder, partPath string, emitBlock func(*model.Block)) error {
	// Reset per-paragraph style-chain context — see parseParagraph
	// for the rationale.
	savedStyleChainNames := p.currentStyleChainNames
	p.currentStyleChainNames = nil
	defer func() { p.currentStyleChainNames = savedStyleChainNames }()

	var paraProps string
	var paraStyleID string
	var runs []textRun
	var cfs complexFieldState
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
				rs, err := p.parseRunWithFieldState(dec, &cfs, rawStart)
				if err != nil {
					return err
				}
				rs = filterFieldRuns(rs, &cfs)
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
				if err := p.parseSmartTag(dec, &runs, &cfs, rawStart); err != nil {
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
			// Apply style optimisation as parseParagraph does.
			if p.styles != nil && paraStyleID != "" {
				styleProps := p.styles.resolveProps(paraStyleID)
				for i := range runs {
					if !isSentinel(runs[i].text) {
						subtractProps(&runs[i].props, styleProps)
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

// writeRawStartElementTo emits an XML start element to a strings.Builder,
// preserving namespace prefixes via the package nsPrefixMap and
// registering any new xmlns declarations on the element.
func writeRawStartElementTo(out *strings.Builder, t xml.StartElement) {
	registerNamespaces(t.Attr)
	out.WriteString("<")
	writeElementName(out, t.Name)
	for _, a := range t.Attr {
		out.WriteString(" ")
		writeAttrName(out, a.Name)
		out.WriteString(`="`)
		out.WriteString(xmlEscapeAttr(a.Value))
		out.WriteString(`"`)
	}
	out.WriteString(">")
}

// writeRawEndElementTo emits an XML end element to a strings.Builder.
func writeRawEndElementTo(out *strings.Builder, t xml.EndElement) {
	out.WriteString("</")
	writeElementName(out, t.Name)
	out.WriteString(">")
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
				Source: []*model.Segment{model.NewRunsSegment(
					"s1",
					[]model.Run{{Text: &model.TextRun{Text: a.Value}}},
				)},
				Targets: make(map[model.LocaleID][]*model.Segment),
				Properties: map[string]string{
					"partPath": partPath,
					"element":  blockElementTag,
				},
				Annotations: make(map[string]model.Annotation),
			})
		} else {
			out.WriteString(xmlEscapeAttr(a.Value))
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
					emitBlock(&model.Block{
						ID:           refID,
						Type:         "property",
						Translatable: true,
						Source: []*model.Segment{model.NewRunsSegment(
							"s1",
							[]model.Run{{Text: &model.TextRun{Text: text.String()}}},
						)},
						Targets: make(map[model.LocaleID][]*model.Segment),
						Properties: map[string]string{
							"partPath": partPath,
							"element":  "alt-content-text",
						},
						Annotations: make(map[string]model.Annotation),
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
		b.WriteString(xmlEscapeAttr(uri))
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

func startElementToRaw(start xml.StartElement) string {
	var b strings.Builder
	b.WriteString("<")
	writeElementName(&b, start.Name)
	for _, a := range start.Attr {
		b.WriteString(" ")
		writeAttrName(&b, a.Name)
		b.WriteString(`="`)
		b.WriteString(xmlEscapeAttr(a.Value))
		b.WriteString(`"`)
	}
	b.WriteString(">")
	return b.String()
}

// containsComplexField reports whether a captured <w:p> XML
// fragment contains an Office complex-field marker (`<w:fldChar`).
// Used by walkTxbxContent to decide between extracting the
// paragraph's text (clean case) and preserving the paragraph
// verbatim (the field-bearing case). String-level scan is
// sufficient — captureRawElement always emits prefixed names via
// the package nsPrefixMap, so the literal `<w:fldChar` substring
// is a stable test for any namespace binding the source used.
func containsComplexField(rawP string) bool {
	return strings.Contains(rawP, "<w:fldChar")
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

// Skeleton helpers

func (p *wmlParser) skelText(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}

func (p *wmlParser) skelRef(id string) {
	if p.skeletonStore != nil {
		if p.skelBuf.Len() > 0 {
			_ = p.skeletonStore.WriteText(p.skelBuf.Bytes())
			p.skelBuf.Reset()
		}
		_ = p.skeletonStore.WriteRef(id)
	}
}

func (p *wmlParser) skelFlush() {
	if p.skeletonStore != nil && p.skelBuf.Len() > 0 {
		_ = p.skeletonStore.WriteText(p.skelBuf.Bytes())
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
		buf.WriteString(xmlEscapeAttr(a.Value))
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
			p.skelText(xmlEscape(string(t)))
		}
	}
	return nil
}

// XML helpers

// nsRegistry tracks namespace URI → prefix mappings discovered during parsing.
// It supplements the static nsPrefixMap with dynamic mappings from xmlns: attributes.
var nsRegistry = struct {
	sync.RWMutex
	m map[string]string
}{m: make(map[string]string)}

// registerNamespaces scans an element's attributes for xmlns declarations
// and records the URI → prefix mapping.
func registerNamespaces(attrs []xml.Attr) {
	nsRegistry.Lock()
	for _, a := range attrs {
		if a.Name.Space == "xmlns" {
			// xmlns:prefix="URI" → map URI to prefix
			nsRegistry.m[a.Value] = a.Name.Local
		} else if a.Name.Space == "" && a.Name.Local == "xmlns" {
			// xmlns="URI" (default namespace) → map URI to "" (no prefix)
			nsRegistry.m[a.Value] = ""
		}
	}
	nsRegistry.Unlock()
}

// resolvePrefix returns the namespace prefix for a URI, checking the dynamic
// registry first (which reflects the document's actual declarations), then
// falling back to the static map.
func resolvePrefix(ns string) string {
	nsRegistry.RLock()
	p, ok := nsRegistry.m[ns]
	nsRegistry.RUnlock()
	if ok {
		return p
	}
	if p, ok := nsPrefixMap[ns]; ok {
		return p
	}
	return ""
}

// writeElementName writes an element name with its namespace prefix.
func writeElementName(buf *strings.Builder, name xml.Name) {
	if name.Space != "" {
		prefix := resolvePrefix(name.Space)
		if prefix != "" {
			buf.WriteString(prefix)
			buf.WriteString(":")
		}
		// If no known prefix, write local name only — the namespace is
		// already declared on a parent element via xmlns.
	}
	buf.WriteString(name.Local)
}

// writeAttrName writes an attribute name, handling xmlns declarations.
func writeAttrName(buf *strings.Builder, name xml.Name) {
	if name.Space == "xmlns" {
		// Namespace declaration: xmlns:prefix
		buf.WriteString("xmlns:")
		buf.WriteString(name.Local)
		return
	}
	if name.Space == "" && name.Local == "xmlns" {
		// Default namespace declaration
		buf.WriteString("xmlns")
		return
	}
	if name.Space != "" {
		prefix := resolvePrefix(name.Space)
		if prefix != "" {
			buf.WriteString(prefix)
			buf.WriteString(":")
		}
		// Unknown namespace — omit the prefix. The namespace is
		// already declared on a parent element and the attribute
		// name alone is sufficient for well-formed output.
	}
	buf.WriteString(name.Local)
}

// xmlEscapeAttr escapes a string for use as an XML attribute value.
func xmlEscapeAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// nsPrefix maps namespace URI → prefix for known OpenXML namespaces.
var nsPrefixMap = map[string]string{
	wmlNamespace:       "w",
	wmlStrictNamespace: "w",
	dmlNamespace:       "a",
	"http://schemas.openxmlformats.org/officeDocument/2006/relationships":       "r",
	"http://schemas.openxmlformats.org/markup-compatibility/2006":               "mc",
	"http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing":    "wp",
	"http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing":       "xdr",
	"http://schemas.openxmlformats.org/drawingml/2006/chart":                    "c",
	"http://schemas.openxmlformats.org/drawingml/2006/diagram":                  "dgm",
	"http://schemas.openxmlformats.org/drawingml/2006/picture":                  "pic",
	"http://schemas.openxmlformats.org/officeDocument/2006/math":                "m",
	"http://schemas.openxmlformats.org/officeDocument/2006/extended-properties": "ep",
	"http://schemas.openxmlformats.org/officeDocument/2006/custom-properties":   "cp",
	"http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes":      "vt",
	"http://schemas.openxmlformats.org/spreadsheetml/2006/main":                 "x",
	"http://schemas.openxmlformats.org/presentationml/2006/main":                "p",
	"http://schemas.openxmlformats.org/package/2006/relationships":              "pr",
	"http://schemas.openxmlformats.org/package/2006/content-types":              "ct",
	"http://schemas.openxmlformats.org/package/2006/metadata/core-properties":   "coreProperties",
	"http://schemas.microsoft.com/office/word/2010/wordml":                      "w14",
	"http://schemas.microsoft.com/office/word/2012/wordml":                      "w15",
	"http://schemas.microsoft.com/office/word/2015/wordml/symex":                "w16se",
	"http://schemas.microsoft.com/office/spreadsheetml/2009/9/main":             "x14",
	"http://schemas.microsoft.com/office/spreadsheetml/2010/11/main":            "x15",
	"http://schemas.microsoft.com/office/powerpoint/2010/main":                  "p14",
	"http://schemas.microsoft.com/office/powerpoint/2012/main":                  "p15",
	"http://schemas.microsoft.com/office/drawing/2010/main":                     "a14",
	"http://schemas.microsoft.com/office/drawing/2014/main":                     "a16",
	"http://purl.org/dc/elements/1.1/":                                          "dc",
	"http://purl.org/dc/terms/":                                                 "dcterms",
	"http://schemas.openxmlformats.org/officeDocument/2006/customXml":           "ds",
	"urn:schemas-microsoft-com:vml":                                             "v",
	"urn:schemas-microsoft-com:office:office":                                   "o",
	"urn:schemas-microsoft-com:office:word":                                     "w10",
	"http://www.w3.org/2001/XMLSchema-instance":                                 "xsi",
	"http://www.w3.org/2001/XMLSchema":                                          "xsd",
	"http://www.w3.org/XML/1998/namespace":                                      "xml",
	// Microsoft Office extension namespaces
	"http://schemas.microsoft.com/office/word/2010/wordprocessingCanvas":  "wpc",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingDrawing": "wp14",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingGroup":   "wpg",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingInk":     "wpi",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingShape":   "wps",
	"http://schemas.microsoft.com/office/word/2006/wordml":                "wne",
	"http://schemas.microsoft.com/office/mac/office/2008/main":            "mo",
	"urn:schemas-microsoft-com:mac:vml":                                   "mv",
	"http://schemas.microsoft.com/office/drawing/2012/chart":              "c15",
	"http://schemas.microsoft.com/office/drawing/2014/chartex":            "cx",
	"http://schemas.openxmlformats.org/drawingml/2006/lockedCanvas":       "lc",
	"http://schemas.microsoft.com/office/drawing/2008/diagram":            "dsp",
	"http://schemas.microsoft.com/office/drawing/2010/diagram":            "dgm14",
	"http://schemas.microsoft.com/office/thememl/2012/main":               "thm15",
	"http://schemas.microsoft.com/office/drawing/2017/decorative":         "adec",
	"http://schemas.microsoft.com/office/drawing/2018/hyperlinkcolor":     "ahlc",
	"http://schemas.microsoft.com/office/word/2016/wordml/cid":            "w16cid",
	"http://schemas.microsoft.com/office/word/2018/wordml":                "w16",
	"http://schemas.microsoft.com/office/word/2018/wordml/cex":            "w16cex",
	"http://schemas.microsoft.com/office/word/2020/wordml/sdtdatahash":    "w16sdtdh",
}

func isWML(el xml.StartElement) bool {
	return el.Name.Space == wmlNamespace || el.Name.Space == wmlStrictNamespace
}

func isWMLNoNS(el xml.StartElement) bool {
	return el.Name.Space == ""
}

// readCharData reads character data content of a simple element and consumes its end tag.
func readCharData(d *xml.Decoder) (string, error) {
	var text strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			return text.String(), nil
		case xml.StartElement:
			// Unexpected nested element — skip it
			if err := skipElement(d); err != nil {
				return "", err
			}
		}
	}
}

// captureParaProps captures paragraph properties as raw XML and extracts the pStyle value.
func captureParaProps(d *xml.Decoder, start xml.StartElement) (string, string, error) {
	raw, err := captureRawElement(d, start)
	if err != nil {
		return "", "", err
	}
	// Extract pStyle value from the raw XML
	styleID := extractPStyle(raw)
	return raw, styleID, nil
}

// pprInnerRPrRE matches a `<w:rPr>...</w:rPr>` (or self-closing
// `<w:rPr/>`) that is a direct child of `<w:pPr>` and captures the
// children fragment in submatch 1. Used by markPPrInnerRPrKeepEmpty
// to inspect/mark the wrapper.
var pprInnerRPrRE = regexp.MustCompile(`<w:rPr\b[^>]*>([\s\S]*?)</w:rPr>|<w:rPr\b[^>]*/>`)

// pprInnerRPrSkippableRE matches the rPr children that upstream Okapi's
// RunSkippableElements drops on round-trip (lang/noProof/rPrChange). A
// `<w:rPr>` inside pPr whose every child is one of these is the
// candidate for the keep-empty marker — after the writer's strip pass
// the wrapper would otherwise collapse to a missing pPr/rPr.
var pprInnerRPrSkippableRE = regexp.MustCompile(
	`<w:(?:lang|noProof|rPrChange)\b[^>]*/>` +
		`|<w:(?:lang|noProof|rPrChange)\b[^>]*>[\s\S]*?</w:(?:lang|noProof|rPrChange)>`,
)

// markPPrInnerRPrKeepEmpty injects fieldRPrKeepEmptyMarker into the
// FIRST `<w:rPr>` direct child of `<w:pPr>` when that rPr's children
// are entirely skippable per pprInnerRPrSkippableRE. The marker
// (an XML comment) prevents the writer's stripWMLSkippableElements
// fixpoint from collapsing the wrapper, mirroring upstream Okapi's
// raw-markup capture path for paragraphs inside non-extractable
// complex fields (parseContent → addToMarkup at RunParser.java:501-506
// preserves the source structure verbatim, including the
// post-skippable-strip empty `<w:rPr></w:rPr>`). The marker itself is
// stripped from the wire by postNonWSOForName, so the final emission
// carries `<w:rPr></w:rPr>` rather than the comment-bearing
// intermediate. Only the pPr → rPr direct-child relationship is
// targeted.
func markPPrInnerRPrKeepEmpty(raw string) string {
	if !strings.HasPrefix(strings.TrimLeft(raw, " \t\r\n"), "<w:pPr") {
		return raw
	}
	if !strings.Contains(raw, "<w:rPr") {
		return raw
	}
	loc := pprInnerRPrRE.FindStringIndex(raw)
	if loc == nil {
		return raw
	}
	// Verify the match sits at depth-2 directly under pPr (no other
	// element opens between the `<w:pPr ...>` open tag and the
	// matched `<w:rPr ...>` start).
	pprStartEnd := strings.Index(raw, ">")
	if pprStartEnd < 0 || pprStartEnd >= loc[0] {
		return raw
	}
	between := raw[pprStartEnd+1 : loc[0]]
	if strings.TrimSpace(between) != "" {
		return raw
	}
	sub := pprInnerRPrRE.FindStringSubmatch(raw[loc[0]:loc[1]])
	if sub == nil {
		return raw
	}
	children := sub[1]
	residue := pprInnerRPrSkippableRE.ReplaceAllString(children, "")
	if strings.TrimSpace(residue) != "" {
		return raw
	}
	matched := raw[loc[0]:loc[1]]
	var replacement string
	if strings.HasSuffix(matched, "/>") {
		replacement = "<w:rPr>" + fieldRPrKeepEmptyMarker + "</w:rPr>"
	} else {
		closeTagIdx := strings.LastIndex(matched, "</w:rPr>")
		if closeTagIdx < 0 {
			return raw
		}
		replacement = matched[:closeTagIdx] + fieldRPrKeepEmptyMarker + matched[closeTagIdx:]
	}
	return raw[:loc[0]] + replacement + raw[loc[1]:]
}

// paragraphHasDeletedMark reports whether the raw `<w:pPr>` payload
// contains a `<w:rPr>` direct child that itself carries a `<w:del>` or
// `<w:moveFrom>` start element — the "deleted paragraph mark" /
// "moved-from paragraph mark" tracked-change markers introduced by
// ECMA-376 Part 1 §17.13.5.13 (CT_ParaRPr) and §17.13.5.14
// (CT_ParaRPrChange).
//
// In ECMA-376 these markers indicate that the paragraph mark (¶) itself
// is part of a tracked deletion / move-from. Under auto-accept-revisions
// the paragraph break is removed, which collapses the paragraph into the
// following one. Upstream Okapi mirrors this via
// `ParagraphBlockProperties.containsRunPropertyDeletedParagraphMark()`
// (ParagraphBlockProperties.java lines 576-586) — keyed on
// `SkippableElement.RevisionProperty.RUN_PROPERTY_DELETED_PARAGRAPH_MARK`
// (`w:del`) and `RUN_PROPERTY_MOVED_PARAGRAPH_FROM` (`w:moveFrom`) per
// SkippableElement.java lines 232 and 234. `BlockParser.parse` lines
// 207-213 then sets `builder.mergeable(true)` when this marker is
// present so `StyledTextPart.process` (lines 312-319) can absorb the
// paragraph into the next block.
//
// We use the xml.Decoder for safety rather than substring search so
// nested `<w:pPrChange>` history (which can itself contain a
// `<w:rPr><w:del/></w:rPr>` re-stating the pre-change state) does not
// produce a false positive — we only consider the immediate
// `<w:pPr><w:rPr>` direct-child path.
func paragraphHasDeletedMark(raw string) bool {
	if raw == "" {
		return false
	}
	if !strings.Contains(raw, "<w:del") && !strings.Contains(raw, "<w:moveFrom") {
		return false
	}
	dec := xml.NewDecoder(strings.NewReader(raw))
	var depth int
	// Path stack of element local names from the root <w:pPr>.
	var path []string
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			path = append(path, t.Name.Local)
			// We want the chain <pPr> (depth 1) -> <rPr> (depth 2) ->
			// <del>|<moveFrom> (depth 3). pPrChange / rPrChange history
			// blocks live one level deeper, so this check excludes them.
			if depth == 3 && len(path) >= 3 &&
				path[0] == "pPr" && path[1] == "rPr" &&
				(t.Name.Local == "del" || t.Name.Local == "moveFrom") {
				return true
			}
		case xml.EndElement:
			depth--
			if len(path) > 0 {
				path = path[:len(path)-1]
			}
		}
	}
}

// extractPStyle extracts the w:val attribute from <w:pStyle> in raw paragraph properties XML.
func extractPStyle(raw string) string {
	idx := strings.Index(raw, "<w:pStyle")
	if idx < 0 {
		// Try without namespace prefix
		idx = strings.Index(raw, "<pStyle")
		if idx < 0 {
			return ""
		}
	}
	// Find w:val="..." or val="..."
	valIdx := strings.Index(raw[idx:], `val="`)
	if valIdx < 0 {
		return ""
	}
	start := idx + valIdx + 5
	end := strings.Index(raw[start:], `"`)
	if end < 0 {
		return ""
	}
	return raw[start : start+end]
}

// skippableBookmarkName is the well-known Word internal bookmark
// generated to track the user's last edit position. ECMA-376 doesn't
// reserve the name, but every modern Word build emits it on save (and
// expects it to round-trip as a no-op). Upstream Okapi's
// SkippableElements.BookmarkCrossStructure.SKIPPABLE_BOOKMARK_NAME
// (SkippableElements.java line 304) hard-codes it to `_GoBack` and
// drops both the start and the matching end (by id) silently — we
// mirror that policy exactly. The matching is by id, not by name,
// because the end element only carries an id attribute (ECMA-376
// Part 1 §17.13.6.2 — `<w:bookmarkEnd>` has only the `w:id` attribute).
const skippableBookmarkName = "_GoBack"

// bookmarkSkipState tracks the id of the most recent skipped
// bookmarkStart so the matching bookmarkEnd can also be dropped.
// Mirrors the `identifier` field on
// SkippableElements.CrossStructure (SkippableElements.java line 231)
// and the conditional id check on canBeSkipped (lines 277-281).
type bookmarkSkipState struct {
	skippedID string // id of the last skipped bookmarkStart, "" when no pending skip
}

// captureBookmark serializes a `<w:bookmarkStart>` or `<w:bookmarkEnd>`
// element verbatim (preserving every attribute and namespace prefix)
// and returns it as a sentinel textRun. The boolean second result is
// false when the bookmark should be silently dropped (matching upstream
// Okapi's `_GoBack` skip policy — see skippableBookmarkName for the
// citation). The decoder is advanced past the matching end token in
// every case so the caller can continue draining sibling tokens.
//
// ECMA-376 Part 1 §17.13.6.1 — `<w:bookmarkStart>` has `w:id`,
// `w:name`, plus optional revision-tracking attributes (`w:colFirst`,
// `w:colLast`, `w:displacedByCustomXml`). We preserve ALL of them.
//
// ECMA-376 Part 1 §17.13.6.2 — `<w:bookmarkEnd>` has only `w:id` plus
// the optional `w:displacedByCustomXml`.
func (p *wmlParser) captureBookmark(d *xml.Decoder, start xml.StartElement, bms *bookmarkSkipState) (textRun, bool, error) {
	id := attrVal(start, "id")
	if start.Name.Local == "bookmarkStart" {
		name := attrVal(start, "name")
		if name == skippableBookmarkName {
			bms.skippedID = id
			if err := skipElement(d); err != nil {
				return textRun{}, false, err
			}
			return textRun{}, false, nil
		}
	} else if start.Name.Local == "bookmarkEnd" {
		// A bookmarkEnd whose id matches the last skipped start is
		// the closing half of a skipped `_GoBack` and is dropped
		// silently; once consumed the tracking id is cleared so a
		// later bookmarkEnd with the same id (uncommon but legal
		// when ids are recycled) is preserved.
		if bms.skippedID != "" && bms.skippedID == id {
			bms.skippedID = ""
			if err := skipElement(d); err != nil {
				return textRun{}, false, err
			}
			return textRun{}, false, nil
		}
	}

	raw, err := captureRawElement(d, start)
	if err != nil {
		return textRun{}, false, err
	}

	var sentinel string
	if start.Name.Local == "bookmarkStart" {
		sentinel = ":" + id
	} else {
		sentinel = ":" + id
	}
	return textRun{text: sentinel, data: raw}, true, nil
}

// captureCommentRangeMarker serializes a <w:commentRangeStart/> or
// <w:commentRangeEnd/> element verbatim and returns it as a sentinel
// textRun. ECMA-376 Part 1 §17.13.4.3 (CT_MarkupRangeStart) /
// §17.13.4.4 (CT_MarkupRange) define both as direct children of <w:p>
// carrying a required w:id attribute that ties the range to the
// matching <w:commentReference w:id="N"/> in a sibling run.
//
// Mirrors the bookmark capture path (captureBookmark): the marker
// has no inner content (empty element), so a single self-closing tag
// captures its complete representation. The sentinel uses a distinct
// PUA char ( for start, for end) so the writer can tell
// comment-range markers apart from bookmarks and dispatch the
// appropriate SubType on the resulting Run.Ph.
func (p *wmlParser) captureCommentRangeMarker(d *xml.Decoder, start xml.StartElement) (textRun, error) {
	raw, err := captureRawElement(d, start)
	if err != nil {
		return textRun{}, err
	}
	id := attrVal(start, "id")
	var sentinel string
	if start.Name.Local == "commentRangeStart" {
		sentinel = ":" + id
	} else {
		sentinel = ":" + id
	}
	return textRun{text: sentinel, data: raw}, nil
}

// captureRawElement captures an entire element (start to end) as raw XML.
func captureRawElement(d *xml.Decoder, start xml.StartElement) (string, error) {
	var buf strings.Builder
	buf.WriteString("<")
	writeElementName(&buf, start.Name)
	for _, a := range start.Attr {
		buf.WriteString(" ")
		writeAttrName(&buf, a.Name)
		buf.WriteString(`="`)
		buf.WriteString(xmlEscapeAttr(a.Value))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")

	depth := 1
	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			buf.WriteString("<")
			writeElementName(&buf, t.Name)
			for _, a := range t.Attr {
				buf.WriteString(" ")
				writeAttrName(&buf, a.Name)
				buf.WriteString(`="`)
				buf.WriteString(xmlEscapeAttr(a.Value))
				buf.WriteString(`"`)
			}
			buf.WriteString(">")
		case xml.EndElement:
			depth--
			buf.WriteString("</")
			writeElementName(&buf, t.Name)
			buf.WriteString(">")
		case xml.CharData:
			buf.WriteString(xmlEscape(string(t)))
		case xml.Comment:
			buf.WriteString("<!--")
			buf.Write(t)
			buf.WriteString("-->")
		}
	}
	return buf.String(), nil
}

// captureAlternateContent serializes an <mc:AlternateContent> element,
// preserving the wrapper plus the selected branch but dropping
// <mc:Fallback>. Per ECMA-376 Part 3 / ISO/IEC 29500-3 §10 (Markup
// Compatibility and Extensibility) the consumer must select the first
// <mc:Choice Requires="..."> whose required namespaces are all
// supported, otherwise the <mc:Fallback>. Okapi's reference filter
// always selects the first Choice and unconditionally strips Fallback
// (SkippableElement.GeneralInline.ALTERNATE_CONTENT_FALLBACK at line
// 56 of okapi/filters/openxml/src/main/java/net/sf/okapi/filters/
// openxml/SkippableElement.java; gold fixture
// gold/parts/block/document-alternate-content.xml shows
// <mc:AlternateContent><mc:Choice Requires="wps">...</mc:Choice></
// mc:AlternateContent> surviving the round-trip with Fallback gone).
// We mirror that policy: keep the wrapper, keep every Choice, drop
// every Fallback. The wrapper element name (mc:AlternateContent) and
// child Choice/Fallback names are matched by local-name regardless of
// prefix so documents that bind the markup-compatibility namespace to
// a non-default prefix still work.
func captureAlternateContent(d *xml.Decoder, start xml.StartElement) (string, error) {
	var buf strings.Builder
	buf.WriteString("<")
	writeElementName(&buf, start.Name)
	for _, a := range start.Attr {
		buf.WriteString(" ")
		writeAttrName(&buf, a.Name)
		buf.WriteString(`="`)
		buf.WriteString(xmlEscapeAttr(a.Value))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")

	for {
		tok, err := d.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "Fallback":
				// Drop the Fallback subtree entirely. Skip without
				// emitting anything — matches Okapi's
				// SkippableElement.GeneralInline.ALTERNATE_CONTENT_FALLBACK
				// behaviour described above.
				if err := skipElement(d); err != nil {
					return "", err
				}
			case "Choice":
				// Keep the Choice element verbatim, including its
				// Requires attribute and full subtree. Per the MCE
				// spec a Choice consumer MAY select the first
				// supported Choice — Okapi simply preserves every
				// Choice and lets the rendering pipeline decide,
				// which is byte-faithful to the source for any
				// document that already had its wrapper survive a
				// Word save/load round-trip.
				raw, err := captureRawElement(d, t)
				if err != nil {
					return "", err
				}
				buf.WriteString(raw)
			default:
				// Defensive: unknown child of mc:AlternateContent
				// (the schema only allows Choice and Fallback).
				// Preserve it verbatim so unusual documents don't
				// regress silently.
				raw, err := captureRawElement(d, t)
				if err != nil {
					return "", err
				}
				buf.WriteString(raw)
			}
		case xml.EndElement:
			if t.Name.Local == start.Name.Local {
				buf.WriteString("</")
				writeElementName(&buf, t.Name)
				buf.WriteString(">")
				return buf.String(), nil
			}
			// Should not happen for a well-formed document, but
			// emit the close tag defensively.
			buf.WriteString("</")
			writeElementName(&buf, t.Name)
			buf.WriteString(">")
		case xml.CharData:
			buf.WriteString(xmlEscape(string(t)))
		case xml.Comment:
			buf.WriteString("<!--")
			buf.Write(t)
			buf.WriteString("-->")
		}
	}
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// xmlEscapeRune writes a single rune to a string builder, XML-escaping if needed.
func xmlEscapeRune(buf *strings.Builder, r rune) {
	switch r {
	case '&':
		buf.WriteString("&amp;")
	case '<':
		buf.WriteString("&lt;")
	case '>':
		buf.WriteString("&gt;")
	case '"':
		buf.WriteString("&quot;")
	default:
		buf.WriteRune(r)
	}
}
