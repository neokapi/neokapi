package csv

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for CSV files.
type Writer struct {
	format.BaseFormatWriter
	separator     rune
	headers       []string
	headerByCol   map[int]string           // header cell text keyed by column index
	preambleRows  [][]string               // rows before the header row
	blocks        map[cellRef]*model.Block // grid cells carrying translatable text
	dataCells     map[cellRef]string       // grid cells carrying non-translatable text
	maxCol        int
	maxRow        int
	skeletonStore *format.SkeletonStore
}

// cellRef is a cell's position in the rebuilt grid: the row and column the
// reader recorded on the part.
//
// The grid is keyed by position rather than by block name because a name is an
// identity, not a join key — it follows the row's key column when the table has
// one (see naming.go), so it cannot be recomputed from a column header and a
// row ordinal. Row/column properties are what the reader records for exactly
// this purpose, and they say the same thing under every naming scheme.
type cellRef struct{ row, col int }

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new CSV writer.
func NewWriter() *Writer {
	return &Writer{
		FormatName:  "csv",
		separator:   ',',
		headerByCol: make(map[int]string),
		blocks:      make(map[cellRef]*model.Block),
		dataCells:   make(map[cellRef]string),
	}
}

// NewTSVWriter creates a new TSV writer (tab-separated values).
func NewTSVWriter() *Writer {
	return &Writer{
		FormatName:  "tsv",
		separator:   '\t',
		headerByCol: make(map[int]string),
		blocks:      make(map[cellRef]*model.Block),
		dataCells:   make(map[cellRef]string),
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// SetSeparator sets the field delimiter for the writer.
func (w *Writer) SetSeparator(sep rune) {
	w.separator = sep
}

// Write consumes Parts from a channel and writes reconstructed CSV.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		return w.writeWithSkeleton(ctx, parts)
	}

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

// writeWithSkeleton collects all blocks, then reconstructs output from skeleton entries.
func (w *Writer) writeWithSkeleton(ctx context.Context, parts <-chan *model.Part) error {
	blocksByID := make(map[string]*model.Block)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			if part.Type == model.PartBlock {
				if block, ok := part.Resource.(*model.Block); ok {
					blocksByID[block.ID] = block
				}
			}
		}
	}
done:
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("csv writer: flush skeleton: %w", err)
	}
	return w.writeFromSkeleton(blocksByID)
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton(blocks map[string]*model.Block) error {
	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("csv writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.blockText(block)
				if block.Properties["target-cell"] == "true" {
					text = w.targetCellText(block)
				}
				// Re-escape double quotes for cells that were originally quoted:
				// the skeleton already carries the surrounding delimiters.
				if block.Properties["quoted"] == "true" {
					text = strings.ReplaceAll(text, "\"", "\"\"")
				} else {
					text = w.quoteIfStructural(text)
				}
				if _, err := io.WriteString(w.Output, text); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// quoteIfStructural wraps a cell whose text carries the field delimiter or a
// line break in RFC 4180 quotes, doubling any embedded quote.
//
// It applies to cells the source left unquoted, which is where a modified value
// breaks the grid: written raw, a delimiter splits one cell into two and a line
// break splits one row into two, re-keying every cell after it. A bare `"` is
// left alone — it round-trips in an unquoted field, and quoting it would move
// the bytes of a cell nothing edited.
func (w *Writer) quoteIfStructural(text string) string {
	sep := w.separator
	if sep == 0 {
		sep = ','
	}
	if !strings.ContainsRune(text, sep) && !strings.ContainsAny(text, "\r\n") {
		return text
	}
	return "\"" + strings.ReplaceAll(text, "\"", "\"\"") + "\""
}

// targetCellText returns the text for a bilingual-mode target cell: the
// block's target for the writer locale when present, otherwise the target
// cell's original content (recorded by the reader). Source text is never
// written into a target column.
//
// When the writer has NO active locale it is reproducing the table rather than
// merging a translation into it, and the column still belongs to the locale the
// reader attached its content under. The model's target for that locale is the
// authority — the same rule #1471 established for .kbf.json, here under a non-write
// locale (#1482). Without it, every tool that edits a target it was not asked to
// write (a whitespace correction, a post-edit pass, an unredact) was a silent
// no-op through a bilingual .csv: `kapi apply` and the MCP edit tool pass no
// write locale at all, so the captured pre-edit cell always won.
func (w *Writer) targetCellText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetText(w.Locale)
	}
	if loc, ok := format.VerbatimSlotLocale(block, propExistingTarget); ok && block.HasTarget(loc) {
		return block.TargetText(loc)
	}
	return block.Properties[propExistingTarget]
}

// blockText returns the appropriate text for a block (target if available, else source).
func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetText(w.Locale)
	}
	return block.SourceText()
}

// gridCoord reads a cell's row or column index off the properties the reader
// records for exactly that purpose.
//
// A missing or unparseable coordinate is an error rather than a zero, because
// zero is not a position this writer can emit: data rows are 1-based (the
// reader counts from the first row after the header), so a cell at row 0 falls
// outside the emit loop and is dropped. Defaulting therefore turned "these
// parts do not describe a grid" into an empty file and a nil error — the shape
// a whole-document conversion into CSV takes, since blocks from another format
// carry no row or column at all.
func gridCoord(props map[string]string, key, owner string) (int, error) {
	raw, ok := props[key]
	if !ok || raw == "" {
		return 0, fmt.Errorf("csv writer: %q has no %s property: a cell needs a grid position, which parts from a non-tabular source do not carry", owner, key)
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("csv writer: %q has %s=%q, which is not a cell index: %w", owner, key, raw, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("csv writer: %q has %s=%d, which is not a cell index", owner, key, n)
	}
	return n, nil
}

func (w *Writer) collectPart(part *model.Part) error {
	switch part.Type {
	case model.PartBlock:
		block, ok := part.Resource.(*model.Block)
		if !ok {
			return errors.New("csv writer: expected Block resource")
		}
		// Preamble rows surfaced as non-translatable content blocks
		// (extractNonTranslatableContent on) rebuild the preamble exactly like
		// the legacy Data path.
		if !block.Translatable && strings.HasPrefix(block.Name, "preamble-row") {
			w.preambleRows = append(w.preambleRows, strings.Split(w.blockText(block), string(w.separator)))
			return nil
		}
		isHeader := block.SemanticRole() == model.RoleTableHeader || block.Properties["header"] == "true"
		col, err := gridCoord(block.Properties, "column", block.ID)
		if err != nil {
			return err
		}
		// A header cell rebuilds the header row and has no row of its own.
		row := 0
		if !isHeader {
			if row, err = gridCoord(block.Properties, "row", block.ID); err != nil {
				return err
			}
		}
		// Header cells carry the column labels and rebuild the header row; data
		// cells fill the grid at their recorded position. A bilingual row block
		// spans two columns rather than occupying one, so it has no single grid
		// cell — the byte-exact skeleton path is what reproduces those tables.
		switch {
		case isHeader:
			w.headerByCol[col] = w.blockText(block)
		case block.Properties["target-cell"] == "true":
		default:
			w.blocks[cellRef{row: row, col: col}] = block
		}
		if col > w.maxCol {
			w.maxCol = col
		}
		if row > w.maxRow {
			w.maxRow = row
		}

	case model.PartData:
		data, ok := part.Resource.(*model.Data)
		if !ok {
			return errors.New("csv writer: expected Data resource")
		}
		if data.Name == "header-row" || strings.HasPrefix(data.Name, "preamble-row") {
			w.preambleRows = append(w.preambleRows, strings.Split(data.Properties["content"], string(w.separator)))
			if data.Name == "header-row" {
				w.headers = strings.Split(data.Properties["content"], string(w.separator))
			}
		} else {
			// Store data cell content
			col, err := gridCoord(data.Properties, "column", data.Name)
			if err != nil {
				return err
			}
			row, err := gridCoord(data.Properties, "row", data.Name)
			if err != nil {
				return err
			}
			w.dataCells[cellRef{row: row, col: col}] = data.Properties["content"]
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

	// Write preamble rows (any rows before the header row).
	for _, row := range w.preambleRows {
		if err := csvWriter.Write(row); err != nil {
			return fmt.Errorf("csv writer: writing preamble: %w", err)
		}
	}

	// Rebuild the header row from the collected header cells. headerByCol is
	// empty when the source had no header (headerless CSV) or carried the
	// header as preamble Data (legacy stream).
	if len(w.headerByCol) > 0 {
		hcols := 0
		for c := range w.headerByCol {
			if c+1 > hcols {
				hcols = c + 1
			}
		}
		w.headers = make([]string, hcols)
		for c, v := range w.headerByCol {
			w.headers[c] = v
		}
	}

	// Calculate dimensions
	numCols := max(len(w.headers), w.maxCol+1)

	// Write the header row reconstructed from header cells.
	if len(w.headerByCol) > 0 {
		record := make([]string, numCols)
		for c := range numCols {
			record[c] = w.headerByCol[c]
		}
		if err := csvWriter.Write(record); err != nil {
			return fmt.Errorf("csv writer: writing header: %w", err)
		}
	}

	// Write data rows
	for rowNum := 1; rowNum <= w.maxRow; rowNum++ {
		record := make([]string, numCols)
		for colIdx := range numCols {
			key := cellRef{row: rowNum, col: colIdx}

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
