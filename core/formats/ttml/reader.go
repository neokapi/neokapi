package ttml

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for TTML subtitle files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	skeletonStore *format.SkeletonStore
}

// Ensure Reader implements SkeletonStoreEmitter.
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new TTML reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "ttml",
			FormatDisplayName: "TTML Subtitles",
			FormatMimeType:    "application/ttml+xml",
			FormatExtensions:  []string{".ttml", ".dfxp"},
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
		MIMETypes:  []string{"application/ttml+xml"},
		Extensions: []string{".ttml", ".dfxp"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("ttml: nil document or reader")
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

// ttmlCaption represents a single <p> element from the TTML body.
type ttmlCaption struct {
	id         string
	begin      string
	end        string
	dur        string
	region     string
	textAlign  string
	rawContent string // text content with <br/> handling applied
	attrs      map[string]string
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	// Read the full document
	data, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		r.emitError(ch, fmt.Errorf("ttml: reading document: %w", err))
		return
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "ttml",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/ttml+xml",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	if r.skeletonStore != nil {
		r.readContentSkeleton(ctx, ch, data)
	} else {
		r.readContentSimple(ctx, ch, data)
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) readContentSimple(ctx context.Context, ch chan<- model.PartResult, data []byte) {
	// Emit the raw document as Data for roundtrip reconstruction
	docData := &model.Data{
		ID:   "d1",
		Name: "ttml-document",
		Properties: map[string]string{
			"content": string(data),
		},
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: docData}) {
		return
	}

	// Parse captions from the XML
	captions := r.parseCaptions(data)

	// Apply caption merging if enabled
	if r.cfg.MergeAdjacentCaptions {
		captions = r.mergeCaptions(captions)
	}

	// Surface non-translatable head metadata (ttm:copyright, ttm:agent) for
	// ingestion/LLM consumers. The original bytes are preserved verbatim in the
	// "ttml-document" Data part above (the non-skeleton writer only swaps <p>
	// caption text), so these blocks are purely informational here.
	if r.cfg.ExtractNonTranslatableContent() {
		r.emitHeadMetadataBlocks(ctx, ch, r.findHeadMetadataRanges(data), data)
	}

	r.emitCaptionBlocks(ctx, ch, captions)
}

// readContentSkeleton reads the TTML with skeleton tracking, preserving exact bytes.
func (r *Reader) readContentSkeleton(ctx context.Context, ch chan<- model.PartResult, data []byte) {
	// Also emit the raw document as Data (needed for non-skeleton writer fallback)
	docData := &model.Data{
		ID:   "d1",
		Name: "ttml-document",
		Properties: map[string]string{
			"content": string(data),
		},
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: docData}) {
		return
	}

	// Non-translatable head metadata (ttm:copyright, ttm:agent) is carved out of
	// the opaque head skeleton and surfaced as content blocks riding their own
	// skeleton refs. Gated so that, when off, the skeleton stream is byte-for-byte
	// what it was before (the head stays one verbatim text chunk).
	var headRanges []headMetaRange
	if r.cfg.ExtractNonTranslatableContent() {
		headRanges = r.findHeadMetadataRanges(data)
	}

	// Find the byte ranges of <p> element text content in the original document.
	// Everything outside those ranges is skeleton text; content inside is a ref.
	textRanges := r.findPTextRanges(data)

	// Parse captions for block metadata
	captions := r.parseCaptions(data)
	if r.cfg.MergeAdjacentCaptions {
		captions = r.mergeCaptions(captions)
	}

	// Filter to non-empty captions to match block emission
	var nonEmptyCaptions []*ttmlCaption
	for _, cap := range captions {
		if cap.rawContent != "" {
			nonEmptyCaptions = append(nonEmptyCaptions, cap)
		}
	}

	// Build one ordered list of referent byte ranges: head metadata (meta IDs)
	// followed by body captions (tu IDs). The head always precedes the body, so
	// the list is already sorted by start offset. With headRanges empty (flag
	// off) this reduces to exactly the previous <p>-only stream.
	type refRange struct {
		start, end int
		id         string
	}
	var refs []refRange
	for i, hr := range headRanges {
		refs = append(refs, refRange{start: hr.start, end: hr.end, id: fmt.Sprintf("meta%d", i+1)})
	}
	blockCounter := 0
	for i, tr := range textRanges {
		if i < len(nonEmptyCaptions) {
			blockCounter++
			refs = append(refs, refRange{start: tr.start, end: tr.end, id: fmt.Sprintf("tu%d", blockCounter)})
		}
	}

	// Write skeleton entries: verbatim text between refs, a ref for each range.
	pos := 0
	for _, rf := range refs {
		if rf.start > pos {
			_ = r.skeletonStore.WriteText(data[pos:rf.start])
		}
		_ = r.skeletonStore.WriteRef(rf.id)
		pos = rf.end
	}
	// Write remaining bytes after the last range
	if pos < len(data) {
		_ = r.skeletonStore.WriteText(data[pos:])
	}

	r.emitHeadMetadataBlocks(ctx, ch, headRanges, data)
	r.emitCaptionBlocks(ctx, ch, nonEmptyCaptions)
}

// textRange represents a byte range in the original document.
type textRange struct {
	start int // inclusive
	end   int // exclusive
}

// findPTextRanges finds the byte ranges of text content within <p> elements.
// It returns ranges for non-empty <p> elements only.
//
// Like parseCaptions, parsing is restricted to the <body> slice to
// survive non-conformant XML in the head (see parseCaptions for the
// full motivation).
func (r *Reader) findPTextRanges(data []byte) []textRange {
	body, bodyOffset := bodySlice(data)
	if body == nil {
		body = data
		bodyOffset = 0
	}
	// We need to find the exact byte offsets of the text content between
	// <p ...> and </p> tags. The XML decoder gives us offsets at token boundaries.
	decoder := xml.NewDecoder(strings.NewReader(string(body)))
	var ranges []textRange

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		if start, ok := tok.(xml.StartElement); ok && start.Name.Local == "p" {
			// The text content starts right after the ">" of the opening tag.
			contentStart := int(decoder.InputOffset())

			// Read through until we find the matching </p>
			depth := 1
			hasContent := false
			var textParts []string
			for {
				tok2, err2 := decoder.Token()
				if err2 != nil {
					break
				}
				switch t := tok2.(type) {
				case xml.StartElement:
					depth++
				case xml.EndElement:
					depth--
					if depth == 0 {
						// contentEnd is the offset just before "</p>"
						endOffset := int(decoder.InputOffset())
						// The decoder's InputOffset after reading </p> points to after it.
						// We need the offset of the start of "</p>".
						// Find "</p>" backwards from endOffset in the data.
						closeTag := findCloseTag(body, contentStart, endOffset, "p")
						if closeTag >= 0 {
							text := strings.Join(textParts, "")
							if strings.TrimSpace(text) != "" {
								hasContent = true
							}
							if hasContent {
								ranges = append(ranges, textRange{
									start: contentStart + bodyOffset,
									end:   closeTag + bodyOffset,
								})
							}
						}
						goto nextP
					}
				case xml.CharData:
					textParts = append(textParts, string(t))
				}
			}
		nextP:
		}
	}

	return ranges
}

// findCloseTag finds the byte offset of the closing tag "</tagName>" in data
// between start and end offsets.
func findCloseTag(data []byte, start, end int, tagName string) int {
	if end > len(data) {
		end = len(data)
	}
	closeTag := []byte("</" + tagName + ">")
	segment := data[start:end]
	idx := bytes.LastIndex(segment, closeTag)
	if idx < 0 {
		return -1
	}
	return start + idx
}

// headMetaRange is the inner-content byte range of a non-translatable head
// metadata element (ttm:copyright or ttm:agent), with absolute offsets.
type headMetaRange struct {
	start int    // inclusive, byte offset of the first inner-content byte
	end   int    // exclusive, byte offset just before the closing tag
	elem  string // element local name: "copyright" or "agent"
}

// headSlice returns the byte slice covering <head...>...</head> in data, plus
// the absolute byte offset of the opening "<head". Returns (nil, 0) when no
// closed <head> element is found. Mirrors bodySlice so head parsing is scoped
// and offsets can be reported absolutely.
func headSlice(data []byte) ([]byte, int) {
	src := string(data)
	open := strings.Index(src, "<head")
	if open < 0 {
		return nil, 0
	}
	closeIdx := strings.Index(src[open:], "</head>")
	if closeIdx < 0 {
		return nil, 0
	}
	end := open + closeIdx + len("</head>")
	return data[open:end], open
}

// findHeadMetadataRanges locates the inner-content byte ranges of the
// ttm:copyright and ttm:agent metadata elements inside <head>. ttm:title and
// ttm:desc are intentionally excluded (arguably translatable; out of scope).
//
// Parsing is scoped to the <head> slice and tolerant of malformed markup: the
// okapi reference fixtures ship non-conformant head XML (e.g. <okp:foo> closed
// by </lilt:foo>) that fails Go's xml.Decoder. On any decode error we stop and
// return whatever was found so far (typically nothing), so extraction degrades
// gracefully rather than aborting.
func (r *Reader) findHeadMetadataRanges(data []byte) []headMetaRange {
	head, off := headSlice(data)
	if head == nil {
		return nil
	}
	decoder := xml.NewDecoder(strings.NewReader(string(head)))
	var ranges []headMetaRange

	for {
		tok, err := decoder.Token()
		if err != nil {
			break // graceful stop on malformed head
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "copyright" && start.Name.Local != "agent" {
			continue
		}
		local := start.Name.Local
		contentStart := int(decoder.InputOffset())

		// Consume to the matching close tag, tracking nesting depth so a
		// structured <ttm:agent> (with child <ttm:name> etc.) is closed at its
		// own end tag rather than a child's.
		depth := 1
		malformed := false
		for {
			tok2, err2 := decoder.Token()
			if err2 != nil {
				malformed = true
				break
			}
			switch tok2.(type) {
			case xml.StartElement:
				depth++
			case xml.EndElement:
				depth--
				if depth == 0 {
					endOffset := int(decoder.InputOffset())
					closeStart := lastCloseTagStart(head, contentStart, endOffset)
					if closeStart > contentStart {
						ranges = append(ranges, headMetaRange{
							start: contentStart + off,
							end:   closeStart + off,
							elem:  local,
						})
					}
				}
			}
			if depth == 0 {
				break
			}
		}
		if malformed {
			break
		}
	}

	return ranges
}

// lastCloseTagStart returns the byte offset of the start of the last closing
// tag ("</") within data[start:end). The inner content of an element ends just
// before its (namespace-prefixed) closing tag, which is the last "</" in the
// range; children's closing tags appear earlier. Returns -1 if none is found.
func lastCloseTagStart(data []byte, start, end int) int {
	if end > len(data) {
		end = len(data)
	}
	if start >= end {
		return -1
	}
	idx := bytes.LastIndex(data[start:end], []byte("</"))
	if idx < 0 {
		return -1
	}
	return start + idx
}

// emitHeadMetadataBlocks emits one NON-translatable content block per head
// metadata range (ttm:copyright, ttm:agent). Each block carries the element's
// verbatim inner content as a single run (no inline parse), is anchored to the
// document layer, and is tagged RoleCode / LayerMetadata so ingestion/LLM
// consumers see the contextual content while MT skips it (Translatable=false).
// In the skeleton path these blocks ride their own refs (meta1, meta2, …) so the
// round-trip stays byte-exact; in the non-skeleton path their bytes live in the
// preserved document Data and the blocks are purely informational.
func (r *Reader) emitHeadMetadataBlocks(ctx context.Context, ch chan<- model.PartResult, ranges []headMetaRange, data []byte) {
	for i, hr := range ranges {
		if hr.start < 0 || hr.end > len(data) || hr.start >= hr.end {
			continue
		}
		text := string(data[hr.start:hr.end])
		if strings.TrimSpace(text) == "" {
			continue
		}
		block := model.NewBlock(fmt.Sprintf("meta%d", i+1), text)
		block.Name = "ttm:" + hr.elem
		block.Type = hr.elem
		block.Translatable = false
		block.PreserveWhitespace = true
		block.SetSemanticRole(model.RoleCode, 0)
		block.SetLayoutLayer(model.LayerMetadata)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}
}

func (r *Reader) emitCaptionBlocks(ctx context.Context, ch chan<- model.PartResult, captions []*ttmlCaption) {
	blockCounter := 0
	for _, cap := range captions {
		if cap.rawContent == "" {
			continue
		}
		blockCounter++
		block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), cap.rawContent)
		if cap.id != "" {
			block.Name = cap.id
		} else {
			block.Name = fmt.Sprintf("subtitle.%d", blockCounter)
		}
		if cap.begin != "" {
			block.Properties["begin"] = cap.begin
		}
		if cap.end != "" {
			block.Properties["end"] = cap.end
		}
		if cap.dur != "" {
			block.Properties["dur"] = cap.dur
		}
		if cap.region != "" {
			block.Properties["region"] = cap.region
		}
		if cap.textAlign != "" {
			block.Properties["textAlign"] = cap.textAlign
		}
		maps.Copy(block.Properties, cap.attrs)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}
}

// parseCaptions extracts <p> elements from the TTML document using encoding/xml.
//
// Parsing is restricted to the <body> slice so that head metadata with
// non-conformant XML (e.g. okapi's example1.ttml ships an opening
// <okp:foo> closed by </lilt:foo>, a real namespace-prefix mismatch
// that fails Go's xml.Decoder before reaching the captions) doesn't
// shut down extraction. TTML's translatable content lives only inside
// <body>, so head garbage is never load-bearing.
func (r *Reader) parseCaptions(data []byte) []*ttmlCaption {
	body, _ := bodySlice(data)
	if body == nil {
		body = data
	}
	decoder := xml.NewDecoder(strings.NewReader(string(body)))
	var captions []*ttmlCaption

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch el := tok.(type) {
		case xml.StartElement:
			if el.Name.Local == "p" {
				cap := r.parseCaption(decoder, el)
				captions = append(captions, cap)
			}
		}
	}

	return captions
}

// bodySlice returns the byte slice covering <body...>...</body> in data,
// along with the absolute byte offset of the opening "<body" in the
// original document. Returns (nil, 0) when no <body> element is found.
//
// Callers that report absolute offsets (e.g. findPTextRanges) must add
// the returned offset to ranges produced from the slice.
func bodySlice(data []byte) ([]byte, int) {
	src := string(data)
	bodyOpen := strings.Index(src, "<body")
	if bodyOpen < 0 {
		return nil, 0
	}
	bodyClose := strings.Index(src[bodyOpen:], "</body>")
	if bodyClose < 0 {
		// Self-closing <body/> — single token, no children.
		return nil, 0
	}
	end := bodyOpen + bodyClose + len("</body>")
	return data[bodyOpen:end], bodyOpen
}

// parseCaption parses a single <p> element and its content.
func (r *Reader) parseCaption(decoder *xml.Decoder, start xml.StartElement) *ttmlCaption {
	cap := &ttmlCaption{
		attrs: make(map[string]string),
	}

	// Extract attributes
	for _, attr := range start.Attr {
		switch {
		case attr.Name.Local == "id":
			cap.id = attr.Value
		case attr.Name.Local == "begin":
			cap.begin = attr.Value
		case attr.Name.Local == "end":
			cap.end = attr.Value
		case attr.Name.Local == "dur":
			cap.dur = attr.Value
		case attr.Name.Local == "region":
			cap.region = attr.Value
		case attr.Name.Local == "textAlign":
			cap.textAlign = attr.Value
		}
	}

	// Read content until </p>
	var textParts []string
	depth := 1
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := tok.(type) {
		case xml.CharData:
			textParts = append(textParts, string(t))
		case xml.StartElement:
			depth++
			if t.Name.Local == "br" {
				if r.cfg.EscapeBR {
					// Replace <br/> with space joining
					textParts = append(textParts, " ")
				} else {
					// Preserve <br/> as literal text
					textParts = append(textParts, "<br/>")
				}
			}
			// For other inline elements (e.g., <span>), we pass through
			// their text content but ignore the tags themselves.
		case xml.EndElement:
			depth--
			if depth == 0 {
				goto done
			}
		}
	}
done:

	// Join text parts and normalize whitespace from br-replacement
	text := strings.Join(textParts, "")
	if r.cfg.EscapeBR {
		// Clean up double spaces that may result from br-space insertion
		for strings.Contains(text, "  ") {
			text = strings.ReplaceAll(text, "  ", " ")
		}
	}
	cap.rawContent = text

	return cap
}

// mergeCaptions merges adjacent captions when the previous one ends with
// trailing punctuation (comma, semicolon).
func (r *Reader) mergeCaptions(captions []*ttmlCaption) []*ttmlCaption {
	if len(captions) == 0 {
		return captions
	}

	var merged []*ttmlCaption
	current := captions[0]

	for i := 1; i < len(captions); i++ {
		text := strings.TrimSpace(current.rawContent)
		if shouldMerge(text) {
			// Merge: append next caption's text
			current.rawContent = text + " " + strings.TrimSpace(captions[i].rawContent)
			// Keep the end time from the later caption
			if captions[i].end != "" {
				current.end = captions[i].end
			}
		} else {
			merged = append(merged, current)
			current = captions[i]
		}
	}
	merged = append(merged, current)

	return merged
}

// shouldMerge returns true if the text ends with punctuation that suggests
// continuation (comma, semicolon).
func shouldMerge(text string) bool {
	if text == "" {
		return false
	}
	last := text[len(text)-1]
	return last == ',' || last == ';'
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (r *Reader) emitError(ch chan<- model.PartResult, err error) {
	ch <- model.PartResult{Error: err}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
