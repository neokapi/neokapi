package transtable

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for translation table files.
// Translation tables are tab-separated key-value pairs, one per line.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new translation table reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "transtable",
			FormatDisplayName: "Translation Table",
			FormatMimeType:    "text/tab-separated-values",
			FormatExtensions:  []string{".tab", ".tsv"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/tab-separated-values"},
		Extensions: []string{".tab", ".tsv"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("transtable: nil document or reader")
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
		Format:   "transtable",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/tab-separated-values",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	scanner := bufio.NewScanner(r.Doc.Reader)
	blockID := 0
	dataID := 0
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		line = strings.TrimRight(line, "\r")

		// Empty lines are Data
		if strings.TrimSpace(line) == "" {
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: fmt.Sprintf("empty-line.%d", lineNum),
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
			continue
		}

		// Comment lines (starting with #) are Data
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			dataID++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", dataID),
				Name: fmt.Sprintf("comment.%d", lineNum),
				Properties: map[string]string{
					"comment": line,
				},
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
				return
			}
			continue
		}

		// Tab-separated key-value pair
		parts := strings.SplitN(line, "\t", 2)
		key := parts[0]
		value := ""
		if len(parts) == 2 {
			value = parts[1]
		}

		blockID++
		block := model.NewBlock(key, value)
		block.Name = key
		block.Properties["key"] = key
		block.Properties["line"] = fmt.Sprintf("%d", lineNum)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}

	if err := scanner.Err(); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("transtable: reading: %w", err)}
		return
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
