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

// Table is a fully assembled table: its rows of cells plus any Lead blocks —
// direct children of the table group that are not inside a row (a <caption>, an
// index title), in stream order. A serializer renders the Lead blocks (where its
// format has a place for them) and then the rows.
type Table struct {
	Lead []*model.Block
	Rows []TableRow
}

// AssembleTable groups a table's Part stream into rows of cell blocks — the
// shared generative table-assembly primitive (the table counterpart of
// WalkInline). It consumes parts starting at a table GroupStart (parts[start])
// and returns the assembled table plus the index of the matching table
// GroupEnd. Cells are gathered per table-row group; a cell's header flag comes
// from its RoleTableHeader role or its row group's "header" property; direct
// table-child blocks outside any row (captions) collect into Lead. Serializers
// render their own markup (GFM, <table>, |===) from one assembly instead of each
// re-deriving rows from the event stream.
func AssembleTable(parts []*model.Part, start int) (end int, table Table) {
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
				table.Rows = append(table.Rows, TableRow{Header: g.Properties["header"] == "true"})
				inRow = true
			}
		case model.PartGroupEnd:
			depth--
			if depth == 0 {
				return j, table
			}
			if depth == 1 {
				inRow = false
			}
		case model.PartBlock:
			b, ok := p.Resource.(*model.Block)
			if !ok {
				continue
			}
			if !inRow {
				// A direct table-child block outside any row (a caption).
				if depth == 1 {
					table.Lead = append(table.Lead, b)
				}
				continue
			}
			if len(table.Rows) == 0 {
				continue
			}
			// Drop a cell that only continues a vertical merge from above — the
			// originating cell's RowSpan already covers this position.
			if b.Properties[model.PropTableVMerge] == "continue" {
				continue
			}
			hdr := b.SemanticRole() == model.RoleTableHeader
			if hdr {
				table.Rows[len(table.Rows)-1].Header = true
			}
			row := &table.Rows[len(table.Rows)-1]
			row.Cells = append(row.Cells, TableCell{Block: b, Header: hdr})
		}
	}
	return len(parts) - 1, table
}
