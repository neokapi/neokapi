package markdown

import (
	"fmt"
	"io"
	"strconv"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projection"
)

// isStreamableTable reports whether a part opens a table whose reader declared
// its column count. That declaration is what makes streaming possible: GFM puts
// the delimiter row second, so the width has to be known before the first row is
// written, and a reader that measured its grid up front already knows it.
func isStreamableTable(part *model.Part) bool {
	if part.Type != model.PartGroupStart {
		return false
	}
	g, ok := part.Resource.(*model.GroupStart)
	if !ok || g.Type != "table" {
		return false
	}
	n, err := strconv.Atoi(g.Properties["columns"])
	return err == nil && n > 0
}

// streamTable renders a declared-width table one row at a time.
//
// It holds exactly one row, plus the first row until the second arrives: a lone
// opening cell above a wider grid is the table's title rather than a row, and
// that is decidable as soon as the second row is known. Everything else is
// written straight through, so a worksheet with a million cells costs one row
// of memory instead of a million blocks.
type streamTable struct {
	w       *Writer
	out     io.Writer
	numCols int
	depth   int // open groups within this table, so the matching end is found

	inRow   bool
	row     []mdCell
	held    []mdCell // the first row, until the second decides its meaning
	haveOne bool
	started bool // the header and its delimiter have been written
}

func (w *Writer) newStreamTable(part *model.Part, out io.Writer) *streamTable {
	g, _ := part.Resource.(*model.GroupStart)
	n, _ := strconv.Atoi(g.Properties["columns"])
	return &streamTable{w: w, out: out, numCols: n, depth: 1}
}

// consume feeds one part to the table, reporting whether the table just ended.
func (s *streamTable) consume(part *model.Part) (bool, error) {
	switch part.Type {
	case model.PartGroupStart:
		s.depth++
		if g, ok := part.Resource.(*model.GroupStart); ok && g.Type == projection.RoleTableRow {
			s.inRow = true
			s.row = s.row[:0]
		}
	case model.PartGroupEnd:
		s.depth--
		if s.depth == 0 {
			return true, s.finish()
		}
		if s.inRow {
			s.inRow = false
			if err := s.flushRow(); err != nil {
				return false, err
			}
		}
	case model.PartBlock:
		block, ok := part.Resource.(*model.Block)
		if !ok || !s.inRow {
			return false, nil
		}
		col := -1
		if v, ok := block.Properties["column"]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				col = n
			}
		}
		s.row = append(s.row, mdCell{
			col:  col,
			text: escapeTableCell(renderInlineMarkdown(s.w.cellRuns(block))),
		})
	}
	return false, nil
}

// flushRow commits the row that just closed.
func (s *streamTable) flushRow() error {
	row := make([]mdCell, len(s.row))
	copy(row, s.row)

	if !s.haveOne {
		s.held, s.haveOne = row, true
		return nil
	}
	if !s.started {
		// A lone cell opening a wider grid is the table's title, not a row —
		// left in place it would be promoted to the header and pad out with
		// blanks, demoting the real header to body. numCols is the widest row
		// in populated cells, so "wider grid" is decidable here without waiting
		// to see one.
		if len(s.held) == 1 && s.held[0].text != "" && s.numCols > 1 {
			if err := s.writeCaption(s.held[0].text); err != nil {
				return err
			}
			return s.begin(row)
		}
		if err := s.begin(s.held); err != nil {
			return err
		}
	}
	return s.writeRow(row)
}

// begin writes the header row and the delimiter beneath it.
func (s *streamTable) begin(header []mdCell) error {
	if err := s.separate(); err != nil {
		return err
	}
	s.started = true

	if _, err := io.WriteString(s.out, tableRowLine(placeCells(header, s.numCols))); err != nil {
		return err
	}
	sep := make([]string, s.numCols)
	for i := range sep {
		sep[i] = "---"
	}
	_, err := io.WriteString(s.out, "\n"+tableRowLine(sep))
	return err
}

func (s *streamTable) writeRow(row []mdCell) error {
	_, err := io.WriteString(s.out, "\n"+tableRowLine(placeCells(row, s.numCols)))
	return err
}

// writeCaption renders a lone opening cell as the table's title.
func (s *streamTable) writeCaption(text string) error {
	if err := s.separate(); err != nil {
		return err
	}
	_, err := fmt.Fprint(s.out, "**"+text+"**")
	return err
}

// separate opens a new block, blank-line separated from whatever preceded it.
func (s *streamTable) separate() error {
	if !s.w.firstBlock {
		if _, err := fmt.Fprint(s.out, "\n\n"); err != nil {
			return err
		}
	}
	s.w.firstBlock = false
	return nil
}

// finish writes whatever the table still holds. A single-row table is that row
// as a header, matching the buffered path.
func (s *streamTable) finish() error {
	if s.started || !s.haveOne {
		return nil
	}
	return s.begin(s.held)
}

// rendersNothing reports whether the generative path emits nothing for a block,
// so the collector can drop it instead of holding it until the render skips it.
//
// Each of these is content that also appears in the output at a real position:
// an equation's prose spans are already in its LaTeX; a spreadsheet's
// shared-string table is deduplicated storage, re-emitted as the worksheet cells
// that use it; an Excel ListObject's column names are definitions whose text is
// the worksheet's own header row. Rendering them would put the content in twice.
// They remain the right extraction unit for translation — this is
// generative-path only.
func rendersNothing(b *model.Block) bool {
	switch b.Type {
	case "omml-nor", "shared-string", "table-column":
		return true
	}
	return false
}
