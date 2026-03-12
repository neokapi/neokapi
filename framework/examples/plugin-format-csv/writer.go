package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// CSVWriter reconstructs CSV output from Parts.
// It collects Blocks with row/col properties and writes them as CSV rows.
type CSVWriter struct {
	format.BaseFormatWriter
	headers []string
	cells   map[int]map[int]string // row -> col -> text
	maxCol  int
}

// NewCSVWriter creates a new CSVWriter instance.
func NewCSVWriter() *CSVWriter {
	return &CSVWriter{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "csv",
		},
		cells: make(map[int]map[int]string),
	}
}

// Write consumes Parts from the channel and builds the CSV output.
func (w *CSVWriter) Write(ctx context.Context, parts <-chan *model.Part) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return w.flush()
			}
			switch part.Type {
			case model.PartData:
				if data, ok := part.Resource.(*model.Data); ok {
					if data.Name == "header-row" {
						if content, exists := data.Properties["content"]; exists {
							w.headers = splitCSVLine(content)
						}
					}
				}
			case model.PartBlock:
				if block, ok := part.Resource.(*model.Block); ok {
					w.collectBlock(block)
				}
			}
		}
	}
}

func (w *CSVWriter) collectBlock(block *model.Block) {
	rowStr := block.Properties["row"]
	colStr := block.Properties["col"]
	if colStr == "" {
		colStr = block.Properties["column"]
	}

	row, _ := strconv.Atoi(rowStr)
	col, _ := strconv.Atoi(colStr)

	if row == 0 {
		row = 1
	}
	if col == 0 {
		col = 1
	}

	if w.cells[row] == nil {
		w.cells[row] = make(map[int]string)
	}

	// Use target text if locale is set and target exists, otherwise source.
	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}
	w.cells[row][col] = text

	if col > w.maxCol {
		w.maxCol = col
	}
}

func (w *CSVWriter) flush() error {
	if w.Output == nil {
		return fmt.Errorf("csv writer: output not set")
	}

	csvWriter := csv.NewWriter(w.Output)
	defer csvWriter.Flush()

	// Write headers if present.
	if len(w.headers) > 0 {
		if err := csvWriter.Write(w.headers); err != nil {
			return fmt.Errorf("csv writer: writing headers: %w", err)
		}
	}

	// Determine the number of columns.
	numCols := max(len(w.headers), w.maxCol)

	// Collect and sort row numbers.
	var rows []int
	for r := range w.cells {
		rows = append(rows, r)
	}
	sort.Ints(rows)

	// Write each row.
	for _, rowIdx := range rows {
		record := make([]string, numCols)
		for colIdx := 1; colIdx <= numCols; colIdx++ {
			if text, ok := w.cells[rowIdx][colIdx]; ok {
				record[colIdx-1] = text
			}
		}
		if err := csvWriter.Write(record); err != nil {
			return fmt.Errorf("csv writer: writing row %d: %w", rowIdx, err)
		}
	}

	return nil
}

// splitCSVLine splits a simple CSV header line by comma.
func splitCSVLine(line string) []string {
	return strings.Split(line, ",")
}

// Ensure CSVWriter satisfies the interface at compile time.
var _ format.DataFormatWriter = (*CSVWriter)(nil)
