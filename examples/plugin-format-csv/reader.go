package main

import (
	"context"
	"encoding/csv"
	"fmt"

	"github.com/asgeirf/gokapi/core/format"
	"github.com/asgeirf/gokapi/core/model"
)

// CSVReader reads CSV files and produces Parts.
// The first row is treated as headers. Each subsequent row becomes a Group
// containing one Block per cell, with the column header used as the Block's Name.
type CSVReader struct {
	format.BaseFormatReader
	records [][]string
	headers []string
}

// NewCSVReader creates a new CSVReader instance.
func NewCSVReader() *CSVReader {
	return &CSVReader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "csv",
			FormatDisplayName: "CSV Format Reader",
			FormatMimeType:    "text/csv",
			FormatExtensions:  []string{".csv"},
		},
	}
}

// Signature returns detection metadata for CSV files.
func (r *CSVReader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"text/csv"},
		Extensions: []string{".csv"},
	}
}

// Open reads the entire CSV document and parses it.
func (r *CSVReader) Open(_ context.Context, doc *model.RawDocument) error {
	if doc.Reader == nil {
		return fmt.Errorf("csv reader: no reader provided")
	}
	defer doc.Reader.Close()

	csvReader := csv.NewReader(doc.Reader)
	csvReader.LazyQuotes = true

	records, err := csvReader.ReadAll()
	if err != nil {
		return fmt.Errorf("csv reader: parsing CSV: %w", err)
	}
	if len(records) == 0 {
		return fmt.Errorf("csv reader: empty CSV file")
	}

	r.headers = records[0]
	r.records = records[1:]
	r.Doc = doc
	return nil
}

// Read produces a channel of PartResults: a LayerStart, then one Block per
// cell (row x column), then a LayerEnd.
func (r *CSVReader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult)
	go func() {
		defer close(ch)

		// Emit a root layer.
		uri := ""
		if r.Doc != nil {
			uri = r.Doc.URI
		}
		layer := &model.Layer{
			ID:       "csv-root",
			Name:     uri,
			Format:   "csv",
			Locale:   r.Doc.SourceLocale,
			Encoding: r.Doc.Encoding,
			MimeType: "text/csv",
		}
		if !r.emit(ctx, ch, model.PartResult{Part: &model.Part{Type: model.PartLayerStart, Resource: layer}}) {
			return
		}

		// Emit each row as a group, each cell as a block.
		for rowIdx, row := range r.records {
			groupID := fmt.Sprintf("row-%d", rowIdx+1)
			gs := &model.GroupStart{
				ID:   groupID,
				Name: fmt.Sprintf("Row %d", rowIdx+1),
				Type: "row",
			}
			if !r.emit(ctx, ch, model.PartResult{Part: &model.Part{Type: model.PartGroupStart, Resource: gs}}) {
				return
			}

			for colIdx, cell := range row {
				blockID := fmt.Sprintf("r%d-c%d", rowIdx+1, colIdx+1)
				name := ""
				if colIdx < len(r.headers) {
					name = r.headers[colIdx]
				}
				block := model.NewBlock(blockID, cell)
				block.Name = name
				block.Properties["row"] = fmt.Sprintf("%d", rowIdx+1)
				block.Properties["col"] = fmt.Sprintf("%d", colIdx+1)
				if name != "" {
					block.Properties["header"] = name
				}

				if !r.emit(ctx, ch, model.PartResult{Part: &model.Part{Type: model.PartBlock, Resource: block}}) {
					return
				}
			}

			ge := &model.GroupEnd{ID: groupID}
			if !r.emit(ctx, ch, model.PartResult{Part: &model.Part{Type: model.PartGroupEnd, Resource: ge}}) {
				return
			}
		}

		// End the root layer.
		r.emit(ctx, ch, model.PartResult{Part: &model.Part{Type: model.PartLayerEnd, Resource: layer}})
	}()
	return ch
}

// Close releases resources held by the reader.
func (r *CSVReader) Close() error {
	r.records = nil
	r.headers = nil
	return nil
}

// emit sends a PartResult to the channel, returning false if the context was cancelled.
func (r *CSVReader) emit(ctx context.Context, ch chan<- model.PartResult, pr model.PartResult) bool {
	select {
	case ch <- pr:
		return true
	case <-ctx.Done():
		return false
	}
}

// Ensure CSVReader satisfies the interface at compile time.
var _ format.DataFormatReader = (*CSVReader)(nil)
