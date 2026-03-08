package ttx

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for Trados TagEditor TTX files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new TTX reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "ttx",
			FormatDisplayName: "Trados TagEditor TTX",
			FormatMimeType:    "application/x-ttx+xml",
			FormatExtensions:  []string{".ttx"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/x-ttx+xml"},
		Extensions: []string{".ttx"},
		Sniff: func(data []byte) bool {
			s := string(data)
			return strings.Contains(s, "<TRADOStag")
		},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("ttx: nil document or reader")
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

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("ttx: reading: %w", err)}
		return
	}

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "ttx",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/x-ttx+xml",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	decoder := xml.NewDecoder(strings.NewReader(string(content)))
	decoder.Strict = false

	blockCounter := 0

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("ttx: parsing: %w", err)}
			return
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "Tu" {
				blockCounter++
				matchPercent := attrVal(t.Attr, "MatchPercent")
				block := r.parseTransUnit(decoder, locale, blockCounter, matchPercent)
				if block != nil {
					if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
						return
					}
				}
			}
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// parseTransUnit parses a <Tu> element with its <Tuv> children.
func (r *Reader) parseTransUnit(decoder *xml.Decoder, sourceLocale model.LocaleID, counter int, matchPercent string) *model.Block {
	var sourceText string
	var targetText string
	var targetLang model.LocaleID
	var sourceLang model.LocaleID

	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			return nil
		}

		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if t.Name.Local == "Tuv" {
				lang := model.LocaleID(attrVal(t.Attr, "Lang"))
				segText := r.parseTuv(decoder)
				depth-- // parseTuv consumed end element

				if sourceLang.IsEmpty() {
					sourceLang = lang
					sourceText = segText
				} else {
					targetLang = lang
					targetText = segText
				}
			}
		case xml.EndElement:
			depth--
		}
	}

	if sourceText == "" {
		return nil
	}

	block := model.NewBlock(fmt.Sprintf("tu%d", counter), sourceText)
	block.Name = fmt.Sprintf("tu%d", counter)
	if matchPercent != "" {
		block.Properties["match-percent"] = matchPercent
	}
	if !sourceLang.IsEmpty() {
		block.Properties["source-lang"] = string(sourceLang)
	}

	if targetText != "" && !targetLang.IsEmpty() {
		block.SetTargetText(targetLang, targetText)
	}

	return block
}

// parseTuv parses a <Tuv> element, extracting text from its <Seg> child.
func (r *Reader) parseTuv(decoder *xml.Decoder) string {
	depth := 1
	var segText string

	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			return ""
		}

		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if t.Name.Local == "Seg" {
				segText = readSegContent(decoder)
				depth-- // readSegContent consumed end element
			}
		case xml.EndElement:
			depth--
		}
	}

	return segText
}

// readSegContent reads the text content of a <Seg> element, handling inline tags.
func readSegContent(decoder *xml.Decoder) string {
	var buf strings.Builder
	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			// Skip inline elements like <ut>, <df>, <it> — just read their text content
		case xml.EndElement:
			depth--
		case xml.CharData:
			buf.Write(t)
		}
	}
	return buf.String()
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
