package csv

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for CSV files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new CSV reader.
func NewReader() *Reader {
	cfg := &Config{Separator: ',', HasHeader: true}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "csv",
			FormatDisplayName: "CSV",
			FormatMimeType:    "text/csv",
			FormatExtensions:  []string{".csv", ".tsv"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/csv", "text/tab-separated-values"},
		Extensions: []string{".csv", ".tsv"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("csv: nil document or reader")
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
		Format:   "csv",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "text/csv",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("csv: reading: %w", err)}
		return
	}

	csvReader := csv.NewReader(strings.NewReader(string(content)))
	csvReader.Comma = r.cfg.Separator
	csvReader.LazyQuotes = true

	records, err := csvReader.ReadAll()
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("csv: parsing: %w", err)}
		return
	}

	if len(records) == 0 {
		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
		return
	}

	var headers []string
	startRow := 0
	blockCounter := 0
	dataCounter := 0

	if r.cfg.HasHeader && len(records) > 0 {
		headers = records[0]
		startRow = 1

		// Emit header row as Data
		dataCounter++
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", dataCounter),
			Name: "header-row",
			Properties: map[string]string{
				"content": strings.Join(headers, string(r.cfg.Separator)),
			},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
			return
		}
	}

	for rowIdx := startRow; rowIdx < len(records); rowIdx++ {
		row := records[rowIdx]
		rowNum := rowIdx - startRow + 1

		for colIdx, cell := range row {
			if !r.isTranslatable(colIdx) {
				// Non-translatable column → Data
				dataCounter++
				colName := fmt.Sprintf("col%d", colIdx)
				if colIdx < len(headers) {
					colName = headers[colIdx]
				}
				data := &model.Data{
					ID:   fmt.Sprintf("d%d", dataCounter),
					Name: fmt.Sprintf("%s.row%d", colName, rowNum),
					Properties: map[string]string{
						"content": cell,
						"column":  fmt.Sprintf("%d", colIdx),
						"row":     fmt.Sprintf("%d", rowNum),
					},
				}
				if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
					return
				}
				continue
			}

			if strings.TrimSpace(cell) == "" {
				continue
			}

			blockCounter++
			colName := fmt.Sprintf("col%d", colIdx)
			if colIdx < len(headers) {
				colName = headers[colIdx]
			}

			block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), cell)
			block.Name = fmt.Sprintf("%s.row%d", colName, rowNum)
			block.Properties["column"] = fmt.Sprintf("%d", colIdx)
			block.Properties["row"] = fmt.Sprintf("%d", rowNum)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) isTranslatable(colIdx int) bool {
	if len(r.cfg.TranslatableColumns) == 0 {
		return true // all columns translatable by default
	}
	for _, c := range r.cfg.TranslatableColumns {
		if c == colIdx {
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
