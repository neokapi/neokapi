package paraplaintext

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for paragraph-oriented plain text files.
// Text is split into paragraphs by blank lines (double newline).
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new paragraph plain text reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "paraplaintext",
			FormatDisplayName: "Paragraph Plain Text",
			FormatMimeType:    "text/plain",
			FormatExtensions:  []string{".txt"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("paraplaintext: nil document or reader")
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
		Format:   "paraplaintext",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/plain",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("paraplaintext: reading: %w", err)}
		return
	}

	text := string(content)
	if text == "" {
		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
		return
	}

	// Normalize line endings
	text = strings.ReplaceAll(text, "\r\n", "\n")

	// Split on double newlines to get paragraphs
	segments := strings.Split(text, "\n\n")

	blockID := 0
	dataID := 0

	for i, segment := range segments {
		if segment == "" {
			// Additional blank line separators
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "paragraph-separator",
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
			continue
		}

		blockID++
		block := model.NewBlock(fmt.Sprintf("tu%d", blockID), segment)
		block.Name = fmt.Sprintf("para%d", blockID)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}

		// Emit separator between paragraphs (not after last)
		if i < len(segments)-1 {
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: "paragraph-separator",
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
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
