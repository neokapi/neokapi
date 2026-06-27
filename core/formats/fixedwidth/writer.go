package fixedwidth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for fixed-width column files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	columns       []ColumnDef
	headerRow     string
	rows          map[int]map[string]cellEntry // row -> colName -> entry
	maxRow        int
}

// Ensure Writer implements SkeletonStoreConsumer and StreamingWriter.
var (
	_ format.SkeletonStoreConsumer = (*Writer)(nil)
	_ format.StreamingWriter       = (*Writer)(nil)
)

// StreamingWriter marks this writer as able to consume a streaming skeleton
// interleaved with the Part stream (Write → StreamSkeletonWrite). See [AD-005].
func (w *Writer) StreamingWriter() bool { return true }

type cellEntry struct {
	value string
	col   ColumnDef
}

// NewWriter creates a new fixed-width writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "fixedwidth",
		},
		rows: make(map[int]map[string]cellEntry),
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// SetColumns sets the column definitions for output formatting.
func (w *Writer) SetColumns(cols []ColumnDef) {
	w.columns = cols
}

// Write consumes Parts from a channel and writes reconstructed fixed-width output.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		if w.skeletonStore.IsStreaming() {
			return format.StreamSkeletonWrite(ctx, w.skeletonStore, parts, w.Output, w.renderRef, nil)
		}
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
		return fmt.Errorf("fixedwidth writer: flush skeleton: %w", err)
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
			return fmt.Errorf("fixedwidth writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			data, err := w.renderRef(blocks[string(entry.Data)])
			if err != nil {
				return err
			}
			if _, err := w.Output.Write(data); err != nil {
				return err
			}
		}
	}
	return nil
}

// renderRef returns the bytes a SkeletonRef contributes for the given block,
// shared by the buffered and streaming skeleton paths so both produce identical
// output. A nil block contributes nothing, matching the buffered path's map miss.
func (w *Writer) renderRef(block *model.Block) ([]byte, error) {
	if block == nil {
		return nil, nil
	}
	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}
	return []byte(text), nil
}

func (w *Writer) collectPart(part *model.Part) error {
	switch part.Type {
	case model.PartBlock:
		block, ok := part.Resource.(*model.Block)
		if !ok {
			return errors.New("fixedwidth writer: expected Block resource")
		}

		text := block.SourceText()
		if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
			text = block.TargetText(w.Locale)
		}

		// A non-translatable header / column-label block (surfaced when
		// ExtractNonTranslatableContent is on) carries the whole header line
		// as its source — route it to the header slot like the Data form does.
		if block.Name == "header-row" {
			w.headerRow = text
			return nil
		}

		row := 0
		_, _ = fmt.Sscanf(block.Properties["row"], "%d", &row)
		colName := block.Properties["column"]

		start := 0
		width := 0
		_, _ = fmt.Sscanf(block.Properties["start"], "%d", &start)
		_, _ = fmt.Sscanf(block.Properties["width"], "%d", &width)

		if w.rows[row] == nil {
			w.rows[row] = make(map[string]cellEntry)
		}
		w.rows[row][colName] = cellEntry{
			value: text,
			col:   ColumnDef{Name: colName, Start: start, Width: width},
		}
		if row > w.maxRow {
			w.maxRow = row
		}

	case model.PartData:
		data, ok := part.Resource.(*model.Data)
		if !ok {
			return errors.New("fixedwidth writer: expected Data resource")
		}
		if data.Name == "header-row" {
			w.headerRow = data.Properties["content"]
		} else {
			row := 0
			_, _ = fmt.Sscanf(data.Properties["row"], "%d", &row)
			colName := data.Properties["column"]

			// Try to get column def from writer's columns config
			var col ColumnDef
			for _, c := range w.columns {
				if c.Name == colName {
					col = c
					break
				}
			}

			if w.rows[row] == nil {
				w.rows[row] = make(map[string]cellEntry)
			}
			w.rows[row][colName] = cellEntry{
				value: data.Properties["content"],
				col:   col,
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

	// Write header row if present
	if w.headerRow != "" {
		if _, err := fmt.Fprintln(w.Output, w.headerRow); err != nil {
			return fmt.Errorf("fixedwidth writer: writing header: %w", err)
		}
	}

	// Determine the total line width from columns
	lineWidth := 0
	for _, col := range w.columns {
		end := col.Start + col.Width
		if end > lineWidth {
			lineWidth = end
		}
	}

	// Also check from collected data
	for _, rowCells := range w.rows {
		for _, entry := range rowCells {
			end := entry.col.Start + entry.col.Width
			if end > lineWidth {
				lineWidth = end
			}
		}
	}

	for rowNum := 1; rowNum <= w.maxRow; rowNum++ {
		line := []rune(strings.Repeat(" ", lineWidth))

		rowCells := w.rows[rowNum]
		if rowCells == nil {
			if _, err := fmt.Fprintln(w.Output, string(line)); err != nil {
				return fmt.Errorf("fixedwidth writer: writing row %d: %w", rowNum, err)
			}
			continue
		}

		// Place each cell value into the line at its fixed position
		for _, entry := range rowCells {
			value := []rune(entry.value)
			start := entry.col.Start
			width := entry.col.Width

			// Pad or truncate to fit width
			for i := 0; i < width && start+i < len(line); i++ {
				if i < len(value) {
					line[start+i] = value[i]
				}
				// spaces are already there from initialization
			}
		}

		if _, err := fmt.Fprintln(w.Output, string(line)); err != nil {
			return fmt.Errorf("fixedwidth writer: writing row %d: %w", rowNum, err)
		}
	}

	return nil
}
