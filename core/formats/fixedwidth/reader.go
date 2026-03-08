package fixedwidth

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for fixed-width column files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new fixed-width reader.
func NewReader() *Reader {
	cfg := &Config{}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "fixedwidth",
			FormatDisplayName: "Fixed-Width",
			FormatMimeType:    "text/plain",
			FormatExtensions:  []string{".txt", ".dat", ".fixed"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		Extensions: []string{".dat", ".fixed"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("fixedwidth: nil document or reader")
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
		Format:   "fixedwidth",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/plain",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	scanner := bufio.NewScanner(r.Doc.Reader)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if len(lines) == 0 {
		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
		return
	}

	startRow := 0
	dataCounter := 0
	blockCounter := 0

	// Handle header row
	if r.cfg.HasHeader && len(lines) > 0 {
		dataCounter++
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", dataCounter),
			Name: "header-row",
			Properties: map[string]string{
				"content": lines[0],
			},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
			return
		}
		startRow = 1
	}

	for rowIdx := startRow; rowIdx < len(lines); rowIdx++ {
		line := lines[rowIdx]
		runes := []rune(line)
		rowNum := rowIdx - startRow + 1

		for _, col := range r.cfg.Columns {
			value := r.extractColumn(runes, col)
			if r.cfg.TrimValues {
				value = strings.TrimSpace(value)
			}

			if !col.Translatable {
				// Non-translatable -> Data
				dataCounter++
				data := &model.Data{
					ID:   fmt.Sprintf("d%d", dataCounter),
					Name: fmt.Sprintf("%s.row%d", col.Name, rowNum),
					Properties: map[string]string{
						"content": value,
						"column":  col.Name,
						"row":     fmt.Sprintf("%d", rowNum),
					},
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
					return
				}
				continue
			}

			if value == "" {
				continue
			}

			blockCounter++
			block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), value)
			block.Name = fmt.Sprintf("%s.row%d", col.Name, rowNum)
			block.Properties["column"] = col.Name
			block.Properties["row"] = fmt.Sprintf("%d", rowNum)
			block.Properties["start"] = fmt.Sprintf("%d", col.Start)
			block.Properties["width"] = fmt.Sprintf("%d", col.Width)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// extractColumn extracts a column value from a line of runes.
func (r *Reader) extractColumn(runes []rune, col ColumnDef) string {
	if col.Start >= len(runes) {
		return ""
	}
	end := col.Start + col.Width
	if end > len(runes) {
		end = len(runes)
	}
	return string(runes[col.Start:end])
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
