package txml

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for Wordfast Pro TXML files.
//
// The TXML schema (as parsed by Okapi's TXMLFilter) is:
//
//	<txml locale="..." targetlocale="..." version="1.0" ...>
//	  <skeleton>...source app markup...</skeleton>
//	  <translatable blockId="b1" datatype="html">
//	    <segment segmentId="s1">
//	      <ws>...</ws>?
//	      <source>...</source>
//	      <target>...</target>?
//	      <revisions>...</revisions>?
//	      <ws>...</ws>?
//	    </segment>
//	    <!-- a commented-out segment is skipped -->
//	    ...
//	  </translatable>
//	  <skeleton>...trailing markup...</skeleton>
//	</txml>
//
// Each <translatable> becomes one Block whose Source carries one
// Segment per <segment> child. <ut x='N' type='...'>data</ut>
// elements inside <source>/<target> become PlaceholderRun inline
// codes. <ws>, <skeleton>, and <revisions> elements are skeleton /
// metadata and never contribute to extracted text.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new TXML reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "txml",
			FormatDisplayName: "Wordfast Pro TXML",
			FormatMimeType:    "application/x-txml+xml",
			FormatExtensions:  []string{".txml"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/x-txml+xml"},
		Extensions: []string{".txml"},
		Sniff: func(data []byte) bool {
			s := string(data)
			return strings.Contains(s, "<txml")
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("txml: nil document or reader")
	}
	r.Doc = doc
	return nil
}

// Read returns a channel of PartResults.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		r.readContent(ctx, ch)
	}()
	return ch
}

// elemPosition records the byte position of a <source> or <target> content region.
type elemPosition struct {
	startOffset int    // byte offset after start tag
	endOffset   int    // byte offset before end tag
	blockIdx    int    // which translatable block (0-based)
	segIdx      int    // which segment within the block (0-based)
	elemType    string // "source", "target", or "target-insert"
	// insert marks a zero-width splice point (startOffset == endOffset)
	// rather than a content region to replace. It is emitted for segments
	// that have NO original <target> child: the writer injects a fresh
	// <target>…</target> element here when the block carries a translated
	// target segment, mirroring Okapi's TXMLSkeletonWriter which always
	// regenerates the <target> from the TextUnit when one exists for the
	// output locale (TXMLSkeletonWriter.java:167-176). Without it, a
	// pseudo-translated/translated target would be silently dropped on
	// write-back for any segment that arrived target-less.
	insert bool
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("txml: reading: %w", err)}
		return
	}
	rawText := string(content)

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	// First pass: parse the <txml> root element to extract locale info.
	var sourceLocale, targetLocale string
	decoder := xml.NewDecoder(strings.NewReader(rawText))
	decoder.Strict = false

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if start, ok := tok.(xml.StartElement); ok && start.Name.Local == "txml" {
			sourceLocale = attrVal(start.Attr, "locale")
			targetLocale = attrVal(start.Attr, "targetlocale")
			break
		}
	}

	if sourceLocale != "" {
		locale = model.LocaleID(sourceLocale)
	}
	targetLocaleID := model.LocaleID(targetLocale)

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "txml",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/x-txml+xml",
		Properties: map[string]string{
			"target-locale": targetLocale,
		},
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	// Second pass: walk the document for <translatable> blocks.
	decoder = xml.NewDecoder(strings.NewReader(rawText))
	decoder.Strict = false

	blockIdx := 0
	var elemPositions []elemPosition

	for {
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("txml: parsing: %w", err)}
			return
		}

		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "translatable" {
			continue
		}

		blockID := attrVal(start.Attr, "blockId")
		if blockID == "" {
			blockID = fmt.Sprintf("tu%d", blockIdx+1)
		}
		datatype := attrVal(start.Attr, "datatype")

		block, err := r.parseTranslatable(decoder, locale, targetLocaleID, blockIdx, blockID, datatype, &elemPositions)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("txml: translatable: %w", err)}
			return
		}
		if block != nil {
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
			blockIdx++
		}
	}

	// Build skeleton from collected positions.
	if r.skeletonStore != nil && len(elemPositions) > 0 {
		skelPos := 0
		for _, ep := range elemPositions {
			if ep.startOffset > skelPos {
				r.skelText(rawText[skelPos:ep.startOffset])
			}
			elemType := ep.elemType
			if ep.insert {
				// Mark target-insertion refs so the writer emits the full
				// <target>…</target> wrapper (the splice point is zero-width
				// and carries no original tags) only when a target exists.
				elemType = "target-insert"
			}
			refID := fmt.Sprintf("%d:%d:%s", ep.blockIdx, ep.segIdx, elemType)
			r.skelRef(refID)
			skelPos = ep.endOffset
		}
		if skelPos < len(rawText) {
			r.skelText(rawText[skelPos:])
		}
		r.skelFlush()
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// parseTranslatable parses a <translatable> element. Returns the
// resulting Block, or nil if the translatable contained no live
// (non-commented-out) segments.
//
// In the Run model a Block holds one flat Run sequence per side; the
// per-<segment> structure rides as a stand-off segmentation overlay
// over those runs (AD-017). The reader concatenates each segment's
// source (and target) runs into the single side and records a Span per
// former segment, carrying the segment id and the run-index boundaries
// so the writer can reconstruct one <segment> per span.
func (r *Reader) parseTranslatable(
	decoder *xml.Decoder,
	sourceLocale, targetLocale model.LocaleID,
	blockIdx int,
	blockID, datatype string,
	positions *[]elemPosition,
) (*model.Block, error) {
	block := &model.Block{
		ID:           blockID,
		Translatable: true,
		SourceLocale: sourceLocale,
		Source:       nil,
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   make(map[string]string),
	}
	if datatype != "" {
		block.Properties["datatype"] = datatype
	}

	segOrdinal := 0
	hasTarget := false
	sawAnySegment := false

	var (
		srcRuns  []model.Run
		trgRuns  []model.Run
		srcSpans []model.Span
		trgSpans []model.Span
		srcPos   int
		trgPos   int
	)

	for {
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("unexpected EOF inside <translatable>")
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local != "segment" {
				// Anything else at the translatable level (shouldn't
				// normally happen) — consume to its end so the cursor
				// stays balanced.
				if err := skipElement(decoder); err != nil {
					return nil, err
				}
				continue
			}
			segID := attrVal(t.Attr, "segmentId")
			if segID == "" {
				segID = fmt.Sprintf("s%d", segOrdinal+1)
			}
			segSrcRuns, segTrgRuns, sawTarget, err := r.parseSegment(decoder, blockIdx, segOrdinal, positions)
			if err != nil {
				return nil, err
			}
			sawAnySegment = true

			srcRuns = append(srcRuns, segSrcRuns...)
			srcEnd := srcPos + len(segSrcRuns)
			srcSpans = append(srcSpans, model.Span{
				ID:    segID,
				Range: model.RunRange{StartRun: srcPos, EndRun: srcEnd},
			})
			srcPos = srcEnd

			if sawTarget && !targetLocale.IsEmpty() {
				trgRuns = append(trgRuns, segTrgRuns...)
				trgEnd := trgPos + len(segTrgRuns)
				trgSpans = append(trgSpans, model.Span{
					ID:    segID,
					Range: model.RunRange{StartRun: trgPos, EndRun: trgEnd},
				})
				trgPos = trgEnd
				hasTarget = true
			}
			segOrdinal++

		case xml.EndElement:
			if t.Name.Local == "translatable" {
				if !sawAnySegment {
					// All segments were commented out (or there were
					// none) — emit no block, matching Okapi's
					// testEntryWithAllSegmentsCommentedOut behavior.
					return nil, nil
				}
				block.Source = srcRuns
				block.SetSegmentation(nil, srcSpans)
				if hasTarget {
					block.SetTargetRuns(targetLocale, trgRuns)
					key := model.Variant(targetLocale)
					block.SetSegmentation(&key, trgSpans)
				}
				return block, nil
			}
			// Otherwise ignore (e.g. closing of a stray sibling).
		}
	}
}

// parseSegment parses one <segment> element. Returns the source runs
// (always non-nil — possibly empty), the target runs (nil when no
// <target> child was present), and whether a <target> child was seen.
func (r *Reader) parseSegment(
	decoder *xml.Decoder,
	blockIdx, segIdx int,
	positions *[]elemPosition,
) (srcRuns, trgRuns []model.Run, sawTarget bool, err error) {
	srcRuns = []model.Run{}

	for {
		tok, terr := decoder.Token()
		if errors.Is(terr, io.EOF) {
			return nil, nil, false, errors.New("unexpected EOF inside <segment>")
		}
		if terr != nil {
			return nil, nil, false, terr
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "source":
				startOff := decoder.InputOffset()
				runs, perr := r.parseInlineContent(decoder, "source")
				if perr != nil {
					return nil, nil, false, perr
				}
				srcRuns = runs
				if r.skeletonStore != nil {
					endOff := decoder.InputOffset()
					endPos := max(int(endOff)-len("</source>"), 0)
					*positions = append(*positions, elemPosition{
						startOffset: int(startOff),
						endOffset:   endPos,
						blockIdx:    blockIdx,
						segIdx:      segIdx,
						elemType:    "source",
					})
				}
			case "target":
				startOff := decoder.InputOffset()
				runs, perr := r.parseInlineContent(decoder, "target")
				if perr != nil {
					return nil, nil, false, perr
				}
				trgRuns = runs
				sawTarget = true
				if r.skeletonStore != nil {
					endOff := decoder.InputOffset()
					endPos := max(int(endOff)-len("</target>"), 0)
					*positions = append(*positions, elemPosition{
						startOffset: int(startOff),
						endOffset:   endPos,
						blockIdx:    blockIdx,
						segIdx:      segIdx,
						elemType:    "target",
					})
				}
			case "ws", "revisions":
				// Skeleton / metadata — never extracted source text.
				if serr := skipElement(decoder); serr != nil {
					return nil, nil, false, serr
				}
			default:
				// Unknown element inside a segment — skip it.
				if serr := skipElement(decoder); serr != nil {
					return nil, nil, false, serr
				}
			}

		case xml.EndElement:
			if t.Name.Local == "segment" {
				// If this segment had no original <target>, record a
				// zero-width splice point just before </segment> so the
				// writer can inject a fresh <target> when the block
				// carries a translated target for this segment. The XSD
				// content model is (ws?, source, ws? target), so the
				// target always sits last, immediately before </segment>
				// (TXMLSkeletonWriter.java:167-176, and the trailing-<ws>
				// case at :135-165). decoder.InputOffset() here is the
				// offset just after the closing ">" of </segment>.
				if r.skeletonStore != nil && !sawTarget {
					insertPos := max(int(decoder.InputOffset())-len("</segment>"), 0)
					*positions = append(*positions, elemPosition{
						startOffset: insertPos,
						endOffset:   insertPos,
						blockIdx:    blockIdx,
						segIdx:      segIdx,
						elemType:    "target",
						insert:      true,
					})
				}
				return srcRuns, trgRuns, sawTarget, nil
			}
		}
	}
}

// parseInlineContent reads the inner content of a <source> or
// <target> element, building a Run sequence. <ut> children become
// PlaceholderRun codes carrying the original markup as Data. The
// caller's exitName names the wrapping element so we know when to
// stop.
func (r *Reader) parseInlineContent(decoder *xml.Decoder, exitName string) ([]model.Run, error) {
	var runs []model.Run
	var textBuf strings.Builder

	flushText := func() {
		if textBuf.Len() == 0 {
			return
		}
		runs = append(runs, model.Run{Text: &model.TextRun{Text: textBuf.String()}})
		textBuf.Reset()
	}

	for {
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("unexpected EOF inside <%s>", exitName)
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.CharData:
			textBuf.Write(t)

		case xml.StartElement:
			if t.Name.Local == "ut" {
				flushText()
				id := attrVal(t.Attr, "x")
				typ := attrVal(t.Attr, "type")
				data, err := readUTContent(decoder)
				if err != nil {
					return nil, err
				}
				runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
					ID:    id,
					Type:  typ,
					Data:  data,
					Equiv: id, // <N/> placeholder rendering matches Okapi's printSegmentedContent
				}})
				continue
			}
			// Unknown nested element — skip its contents.
			if err := skipElement(decoder); err != nil {
				return nil, err
			}

		case xml.EndElement:
			if t.Name.Local == exitName {
				flushText()
				return runs, nil
			}
		}
	}
}

// readUTContent reads the textual content of a <ut> element until
// </ut>. Inner XML elements (rare) are flattened to their textual
// body. Returns the captured text (already entity-decoded by the
// XML parser).
func readUTContent(decoder *xml.Decoder) (string, error) {
	var buf strings.Builder
	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return "", errors.New("unexpected EOF inside <ut>")
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.CharData:
			buf.Write(t)
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
			if depth == 0 && t.Name.Local == "ut" {
				return buf.String(), nil
			}
		}
	}
	return buf.String(), nil
}

// skipElement consumes tokens up to (and including) the matching end
// element for the just-read start element.
func skipElement(decoder *xml.Decoder) error {
	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return nil
}

// attrVal returns the value of named attribute, or "".
func attrVal(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// skelText appends text to the skeleton buffer if active.
func (r *Reader) skelText(s string) {
	if r.skeletonStore != nil && s != "" {
		r.skelBuf.WriteString(s)
	}
}

// skelRef flushes buffered text and writes a block reference to the skeleton store.
func (r *Reader) skelRef(id string) {
	if r.skeletonStore != nil {
		if r.skelBuf.Len() > 0 {
			_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
			r.skelBuf.Reset()
		}
		_ = r.skeletonStore.WriteRef(id)
	}
}

// skelFlush writes any remaining buffered text to the skeleton store.
func (r *Reader) skelFlush() {
	if r.skeletonStore != nil && r.skelBuf.Len() > 0 {
		_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
		r.skelBuf.Reset()
	}
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
