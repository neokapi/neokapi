package xml

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for XML files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new XML reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "xml",
			FormatDisplayName: "XML",
			FormatMimeType:    "text/xml",
			FormatExtensions:  []string{".xml"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// SetConfig applies a new configuration.
func (r *Reader) SetConfig(cfg format.DataFormatConfig) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	r.Cfg = cfg
	if c, ok := cfg.(*Config); ok {
		r.cfg = c
	}
	return nil
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/xml", "application/xml"},
		Extensions: []string{".xml"},
		MagicBytes: [][]byte{[]byte("<?xml")},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("xml: nil document or reader")
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
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "xml",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/xml",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("xml: reading: %w", err)}
		return
	}

	decoder := xml.NewDecoder(strings.NewReader(string(content)))
	blockCounter := 0
	dataCounter := 0
	var elementStack []string

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("xml: parsing: %w", err)}
			return
		}

		switch t := tok.(type) {
		case xml.StartElement:
			elementStack = append(elementStack, t.Name.Local)
			// Emit translatable attributes as blocks
			for _, attr := range t.Attr {
				if r.isTranslatableAttr(attr.Name.Local) {
					blockCounter++
					block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), attr.Value)
					block.Name = strings.Join(elementStack, ".") + "@" + attr.Name.Local
					block.Type = "attribute"
					if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
						return
					}
				}
			}

		case xml.EndElement:
			if len(elementStack) > 0 {
				elementStack = elementStack[:len(elementStack)-1]
			}

		case xml.CharData:
			text := strings.TrimSpace(string(t))
			if text == "" {
				continue
			}
			if r.isTranslatableElement(elementStack) {
				blockCounter++
				block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), text)
				block.Name = strings.Join(elementStack, ".")
				if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
					return
				}
			} else {
				dataCounter++
				data := &model.Data{
					ID:   fmt.Sprintf("d%d", dataCounter),
					Name: strings.Join(elementStack, "."),
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
					return
				}
			}

		case xml.ProcInst:
			dataCounter++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: "processing-instruction",
			}
			r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})

		case xml.Comment:
			dataCounter++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataCounter),
				Name: "comment",
			}
			r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) isTranslatableElement(stack []string) bool {
	if len(r.cfg.TranslatableElements) == 0 {
		return true // all elements translatable by default
	}
	if len(stack) == 0 {
		return false
	}
	current := stack[len(stack)-1]
	for _, elem := range r.cfg.TranslatableElements {
		if elem == current {
			return true
		}
	}
	return false
}

func (r *Reader) isTranslatableAttr(name string) bool {
	for _, attr := range r.cfg.TranslatableAttributes {
		if attr == name {
			return true
		}
	}
	return false
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
