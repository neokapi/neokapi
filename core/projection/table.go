package projection

import "github.com/neokapi/neokapi/core/model"

// TableCell is one assembled table cell: the source Block plus whether it is a
// header cell (from its RoleTableHeader role or its row group's "header"
// property). A serializer reads the block to pick source vs target runs by its
// own locale and to read merge extents (Block.Structure ColSpan/RowSpan), so
// AssembleTable stays locale-agnostic.
type TableCell struct {
	Block  *model.Block
	Header bool
}

// TableRow is one assembled row: its cells plus whether the row is a header row.
type TableRow struct {
	Header bool
	Cells  []TableCell
}

// AssembleTable groups a table's Part stream into rows of cell blocks — the
// shared generative table-assembly primitive (the table counterpart of
// WalkInline). It consumes parts starting at a table GroupStart (parts[start])
// and returns the rows plus the index of the matching table GroupEnd. Cells are
// gathered per table-row group; a cell's header flag comes from its
// RoleTableHeader role or its row group's "header" property. Serializers render
// their own markup (GFM, <table>, |===) from one assembly instead of each
// re-deriving rows from the event stream.
func AssembleTable(parts []*model.Part, start int) (end int, rows []TableRow) {
	depth := 0
	inRow := false
	for j := start; j < len(parts); j++ {
		p := parts[j]
		if p == nil {
			continue
		}
		switch p.Type {
		case model.PartGroupStart:
			depth++
			g, _ := p.Resource.(*model.GroupStart)
			if depth >= 2 && g != nil && g.Type == RoleTableRow {
				rows = append(rows, TableRow{Header: g.Properties["header"] == "true"})
				inRow = true
			}
		case model.PartGroupEnd:
			depth--
			if depth == 0 {
				return j, rows
			}
			if depth == 1 {
				inRow = false
			}
		case model.PartBlock:
			if !inRow || len(rows) == 0 {
				continue
			}
			b, ok := p.Resource.(*model.Block)
			if !ok {
				continue
			}
			hdr := b.SemanticRole() == model.RoleTableHeader
			if hdr {
				rows[len(rows)-1].Header = true
			}
			rows[len(rows)-1].Cells = append(rows[len(rows)-1].Cells, TableCell{Block: b, Header: hdr})
		}
	}
	return len(parts) - 1, rows
}
