package csv

import (
	"context"
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for CSV files.
type Writer struct {
	format.BaseFormatWriter
	separator    rune
	headers      []string
	preambleRows [][]string // rows before data (header, preamble)
	blocks       map[string]*model.Block // keyed by "col.row"
	dataCells    map[string]string       // keyed by "col.row"
	maxCol       int
	maxRow       int
}

// NewWriter creates a new CSV writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "csv",
		},
		separator: ',',
		blocks:    make(map[string]*model.Block),
		dataCells: make(map[string]string),
	}
}

// NewTSVWriter creates a new TSV writer (tab-separated values).
func NewTSVWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "tsv",
		},
		separator: '\t',
		blocks:    make(map[string]*model.Block),
		dataCells: make(map[string]string),
	}
}

// SetSeparator sets the field delimiter for the writer.
func (w *Writer) SetSeparator(sep rune) {
	w.separator = sep
}

// Write consumes Parts from a channel and writes reconstructed CSV.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return w.flush()
			}
			if err := w.collectPart(part); err != nil {
				return err
			}
		}
	}
}

func (w *Writer) collectPart(part *model.Part) error {
	switch part.Type {
	case model.PartBlock:
		block, ok := part.Resource.(*model.Block)
		if !ok {
			return fmt.Errorf("csv writer: expected Block resource")
		}
		w.blocks[block.Name] = block
		// Track max row/col
		col := 0
		row := 0
		_, _ = fmt.Sscanf(block.Properties["column"], "%d", &col)
		_, _ = fmt.Sscanf(block.Properties["row"], "%d", &row)
		if col > w.maxCol {
			w.maxCol = col
		}
		if row > w.maxRow {
			w.maxRow = row
		}

	case model.PartData:
		data, ok := part.Resource.(*model.Data)
		if !ok {
			return fmt.Errorf("csv writer: expected Data resource")
		}
		if data.Name == "header-row" || strings.HasPrefix(data.Name, "preamble-row") {
			w.preambleRows = append(w.preambleRows, strings.Split(data.Properties["content"], string(w.separator)))
			if data.Name == "header-row" {
				w.headers = strings.Split(data.Properties["content"], string(w.separator))
			}
		} else {
			// Store data cell content
			w.dataCells[data.Name] = data.Properties["content"]
			col := 0
			row := 0
			_, _ = fmt.Sscanf(data.Properties["column"], "%d", &col)
			_, _ = fmt.Sscanf(data.Properties["row"], "%d", &row)
			if col > w.maxCol {
				w.maxCol = col
			}
			if row > w.maxRow {
				w.maxRow = row
			}
		}
	}
	return nil
}

func (w *Writer) flush() error {
	if w.Output == nil {
		return nil
	}

	csvWriter := csv.NewWriter(w.Output)
	csvWriter.Comma = w.separator

	// Write preamble rows (headers and any rows before data)
	for _, row := range w.preambleRows {
		if err := csvWriter.Write(row); err != nil {
			return fmt.Errorf("csv writer: writing preamble: %w", err)
		}
	}

	// Calculate dimensions
	numCols := max(len(w.headers), w.maxCol+1)

	// Write data rows
	for rowNum := 1; rowNum <= w.maxRow; rowNum++ {
		record := make([]string, numCols)
		for colIdx := 0; colIdx < numCols; colIdx++ {
			colName := fmt.Sprintf("col%d", colIdx)
			if colIdx < len(w.headers) {
				colName = w.headers[colIdx]
			}
			key := fmt.Sprintf("%s.row%d", colName, rowNum)

			if block, ok := w.blocks[key]; ok {
				text := block.SourceText()
				if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
					text = block.TargetText(w.Locale)
				}
				record[colIdx] = text
			} else if content, ok := w.dataCells[key]; ok {
				record[colIdx] = content
			}
		}
		if err := csvWriter.Write(record); err != nil {
			return fmt.Errorf("csv writer: writing row %d: %w", rowNum, err)
		}
	}

	csvWriter.Flush()
	return csvWriter.Error()
}
