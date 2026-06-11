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

	// Write skeleton entries based on text ranges
	blockCounter := 0
	pos := 0
	for i, tr := range textRanges {
		// Write skeleton text for bytes before this range
		if tr.start > pos {
			_ = r.skeletonStore.WriteText(data[pos:tr.start])
		}

		// Write skeleton ref for this <p> text content
		if i < len(nonEmptyCaptions) {
			blockCounter++
			blockIDStr := fmt.Sprintf("tu%d", blockCounter)
			_ = r.skeletonStore.WriteRef(blockIDStr)
		}

		pos = tr.end
	}
	// Write remaining bytes after the last range
	if pos < len(data) {
		_ = r.skeletonStore.WriteText(data[pos:])
	}

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
