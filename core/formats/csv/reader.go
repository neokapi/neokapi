package csv

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"slices"
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
			FormatExtensions:  []string{".csv"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// NewTSVReader creates a new TSV reader (tab-separated values).
func NewTSVReader() *Reader {
	cfg := &Config{Separator: '\t', HasHeader: true}
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "tsv",
			FormatDisplayName: "TSV",
			FormatMimeType:    "text/tab-separated-values",
			FormatExtensions:  []string{".tsv"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	if r.cfg.Separator == '\t' {
		return format.FormatSignature{
			MIMETypes:  []string{"text/tab-separated-values"},
			Extensions: []string{".tsv"},
		}
	}
	return format.FormatSignature{
		MIMETypes:  []string{"text/csv"},
		Extensions: []string{".csv"},
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

	mimeType := "text/csv"
	if r.cfg.Separator == '\t' {
		mimeType = "text/tab-separated-values"
	}

	formatName := "csv"
	if r.cfg.Separator == '\t' {
		formatName = "tsv"
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   formatName,
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: mimeType,
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
	csvReader.FieldsPerRecord = -1 // allow variable number of fields per row

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
	headerRow := -1
	startRow := 0
	blockCounter := 0
	dataCounter := 0

	// Determine header row
	if r.cfg.HasHeader {
		if r.cfg.ColumnNamesRow > 0 {
			headerRow = r.cfg.ColumnNamesRow - 1 // convert 1-based to 0-based
		} else {
			headerRow = 0
		}
		if headerRow < len(records) {
			headers = records[headerRow]
		}
	}

	// Determine start row for data values
	if r.cfg.ValuesStartRow > 0 {
		startRow = r.cfg.ValuesStartRow - 1 // convert 1-based to 0-based
	} else if r.cfg.HasHeader {
		if headerRow >= 0 {
			startRow = headerRow + 1
		} else {
			startRow = 1
		}
	}

	// Emit rows before the data start as Data parts (headers, preamble, etc.)
	for rowIdx := 0; rowIdx < startRow && rowIdx < len(records); rowIdx++ {
		dataCounter++
		row := records[rowIdx]
		name := "header-row"
		if rowIdx != headerRow {
			name = fmt.Sprintf("preamble-row%d", rowIdx+1)
		}
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", dataCounter),
			Name: name,
			Properties: map[string]string{
				"content": strings.Join(row, string(r.cfg.Separator)),
			},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data}) {
			return
		}
	}

	for rowIdx := startRow; rowIdx < len(records); rowIdx++ {
		row := records[rowIdx]
		rowNum := rowIdx - startRow + 1

		// Build key from key columns if configured
		var rowKey string
		if len(r.cfg.KeyColumns) > 0 {
			var keyParts []string
			for _, kc := range r.cfg.KeyColumns {
				if kc < len(row) {
					keyParts = append(keyParts, row[kc])
				}
			}
			rowKey = strings.Join(keyParts, ".")
		}

		// Build comment from comment columns if configured
		var rowComment string
		if len(r.cfg.CommentColumns) > 0 {
			var commentParts []string
			for _, cc := range r.cfg.CommentColumns {
				if cc < len(row) && strings.TrimSpace(row[cc]) != "" {
					commentParts = append(commentParts, row[cc])
				}
			}
			rowComment = strings.Join(commentParts, "; ")
		}

		for colIdx, cell := range row {
			// Skip key and comment columns (they are metadata, not content)
			if r.isKeyColumn(colIdx) || r.isCommentColumn(colIdx) {
				continue
			}

			if !r.isTranslatable(colIdx) {
				// Non-translatable column -> Data
				dataCounter++
				colName := r.columnName(headers, colIdx)
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

			cellValue := cell
			if r.cfg.TrimValues {
				cellValue = strings.TrimSpace(cellValue)
			}

			if cellValue == "" {
				continue
			}

			blockCounter++
			colName := r.columnName(headers, colIdx)

			blockID := fmt.Sprintf("tu%d", blockCounter)
			if rowKey != "" {
				blockID = rowKey
				if len(r.cfg.TranslatableColumns) > 1 {
					// Multiple translatable columns with key: add column suffix
					blockID = fmt.Sprintf("%s.%s", rowKey, colName)
				}
			}

			block := model.NewBlock(blockID, cellValue)
			block.Name = fmt.Sprintf("%s.row%d", colName, rowNum)
			block.Properties["column"] = fmt.Sprintf("%d", colIdx)
			block.Properties["row"] = fmt.Sprintf("%d", rowNum)
			if rowComment != "" {
				block.Properties["comment"] = rowComment
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

func (r *Reader) columnName(headers []string, colIdx int) string {
	if colIdx < len(headers) {
		return headers[colIdx]
	}
	return fmt.Sprintf("col%d", colIdx)
}

func (r *Reader) isTranslatable(colIdx int) bool {
	if len(r.cfg.TranslatableColumns) == 0 {
		return true // all columns translatable by default
	}
	return slices.Contains(r.cfg.TranslatableColumns, colIdx)
}

func (r *Reader) isKeyColumn(colIdx int) bool {
	return slices.Contains(r.cfg.KeyColumns, colIdx)
}

func (r *Reader) isCommentColumn(colIdx int) bool {
	return slices.Contains(r.cfg.CommentColumns, colIdx)
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
