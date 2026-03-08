package ttml

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for TTML subtitle files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

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
		return fmt.Errorf("ttml: nil document or reader")
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
		for k, v := range cap.attrs {
			block.Properties[k] = v
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// parseCaptions extracts <p> elements from the TTML document using encoding/xml.
func (r *Reader) parseCaptions(data []byte) []*ttmlCaption {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
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
