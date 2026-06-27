package projection

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cellBlock builds a table-cell (or header) block with plain text.
func cellBlock(id, text string, header bool) *model.Block {
	b := model.NewBlock(id, text)
	if header {
		b.SetSemanticRole(model.RoleTableHeader, 0)
	} else {
		b.SetSemanticRole(model.RoleTableCell, 0)
	}
	return b
}

func groupStart(id, typ string) *model.Part {
	return &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: id, Name: typ, Type: typ}}
}
func groupEnd(id string) *model.Part {
	return &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: id}}
}
func blockPart(b *model.Block) *model.Part {
	return &model.Part{Type: model.PartBlock, Resource: b}
}

func TestProjectBlock_Fragment(t *testing.T) {
	b := model.NewBlock("h1", "Title")
	b.SetSemanticRole(model.RoleHeading, 2)
	n := ProjectBlock(b)
	assert.Equal(t, model.RoleHeading, n.Role)
	assert.Equal(t, 2, n.Level)
	assert.Equal(t, "h1", n.BlockID)
	assert.Equal(t, "Title", n.Text())
	assert.True(t, n.IsLeaf())
}

func TestProjectBlock_RoleFallsBackToType(t *testing.T) {
	// A reader that tags only Block.Type (no SemanticRole) still projects.
	b := model.NewBlock("c1", "x")
	b.Type = model.RoleTableCell
	n := ProjectBlock(b)
	assert.Equal(t, model.RoleTableCell, n.Role)
}

func TestProjectStream_GroupDrivenTable(t *testing.T) {
	parts := []*model.Part{
		groupStart("t", "table"),
		groupStart("r0", "table-row"),
		blockPart(cellBlock("a", "A", true)),
		blockPart(cellBlock("b", "B", true)),
		groupEnd("r0"),
		groupStart("r1", "table-row"),
		blockPart(cellBlock("c", "1", false)),
		blockPart(cellBlock("d", "2", false)),
		groupEnd("r1"),
		groupEnd("t"),
	}
	root := ProjectStream(parts)

	require.Len(t, root.Children, 1)
	table := root.Children[0]
	assert.Equal(t, model.RoleTable, table.Role)
	require.Len(t, table.Children, 2, "two rows")

	r0 := table.Children[0]
	assert.Equal(t, RoleTableRow, r0.Role)
	require.Len(t, r0.Children, 2)
	assert.True(t, r0.Children[0].Header, "header cell flagged")
	assert.Equal(t, "A", r0.Children[0].Text())

	r1 := table.Children[1]
	require.Len(t, r1.Children, 2)
	assert.False(t, r1.Children[0].Header)
	assert.Equal(t, "1", r1.Children[0].Text())
	assert.Equal(t, "2", r1.Children[1].Text())
}

func TestProjectStream_CellSpans(t *testing.T) {
	merged := cellBlock("m", "wide", false)
	s, _ := merged.Structure()
	if s == nil {
		s = &model.StructureAnnotation{Role: model.RoleTableCell}
	}
	s.ColSpan = 2
	s.RowSpan = 3
	merged.SetStructure(s)

	parts := []*model.Part{
		groupStart("t", "table"),
		groupStart("r0", "table-row"),
		blockPart(merged),
		groupEnd("r0"),
		groupEnd("t"),
	}
	cell := ProjectStream(parts).Children[0].Children[0].Children[0]
	assert.Equal(t, 2, cell.ColSpan)
	assert.Equal(t, 3, cell.RowSpan)
}

// A reader that emits a flat run of table-cell blocks with no row groups
// (markdown/html today) degrades to a single-row table, not standalone blocks.
func TestProjectStream_FlatCellsFallback(t *testing.T) {
	parts := []*model.Part{
		blockPart(cellBlock("a", "A", true)),
		blockPart(cellBlock("b", "B", true)),
		blockPart(cellBlock("c", "1", false)),
		blockPart(cellBlock("d", "2", false)),
	}
	root := ProjectStream(parts)
	require.Len(t, root.Children, 1)
	table := root.Children[0]
	assert.Equal(t, model.RoleTable, table.Role)
	require.Len(t, table.Children, 1, "one synthetic row without row hints")
	assert.Len(t, table.Children[0].Children, 4)
}

// With a per-cell row hint a flat reader recovers multiple rows.
func TestProjectStream_FlatCellsWithRowHint(t *testing.T) {
	mk := func(id, text, row string) *model.Block {
		b := cellBlock(id, text, false)
		b.Properties = map[string]string{propFlatRow: row}
		return b
	}
	parts := []*model.Part{
		blockPart(mk("a", "A", "0")),
		blockPart(mk("b", "B", "0")),
		blockPart(mk("c", "1", "1")),
		blockPart(mk("d", "2", "1")),
	}
	table := ProjectStream(parts).Children[0]
	require.Len(t, table.Children, 2)
	assert.Len(t, table.Children[0].Children, 2)
	assert.Len(t, table.Children[1].Children, 2)
}

// A flat cell run that ends at a non-cell block flushes before the next block.
func TestProjectStream_FlatCellsFlushBeforeParagraph(t *testing.T) {
	para := model.NewBlock("p", "after")
	para.SetSemanticRole(model.RoleParagraph, 0)
	parts := []*model.Part{
		blockPart(cellBlock("a", "A", false)),
		blockPart(cellBlock("b", "B", false)),
		blockPart(para),
	}
	root := ProjectStream(parts)
	require.Len(t, root.Children, 2)
	assert.Equal(t, model.RoleTable, root.Children[0].Role)
	assert.Equal(t, model.RoleParagraph, root.Children[1].Role)
}

func TestProjectStream_List(t *testing.T) {
	item := func(id, text string) *model.Block {
		b := model.NewBlock(id, text)
		b.SetSemanticRole(model.RoleListItem, 1)
		return b
	}
	parts := []*model.Part{
		groupStart("l", "ordered-list"),
		blockPart(item("i1", "first")),
		blockPart(item("i2", "second")),
		groupEnd("l"),
	}
	root := ProjectStream(parts)
	require.Len(t, root.Children, 1)
	list := root.Children[0]
	assert.Equal(t, model.RoleList, list.Role)
	assert.True(t, list.Ordered)
	require.Len(t, list.Children, 2)
	assert.Equal(t, "first", list.Children[0].Text())
}

// Layers are transparent: blocks inside an embedded-content layer attach to the
// layer's parent in the render tree.
func TestProjectStream_LayerTransparent(t *testing.T) {
	para := model.NewBlock("p", "hi")
	para.SetSemanticRole(model.RoleParagraph, 0)
	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "L", Name: "doc"}},
		blockPart(para),
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "L"}},
	}
	root := ProjectStream(parts)
	require.Len(t, root.Children, 1)
	assert.Equal(t, model.RoleParagraph, root.Children[0].Role)
}

func TestProjectStream_UnknownGroupPreserved(t *testing.T) {
	para := model.NewBlock("p", "caption text")
	para.SetSemanticRole(model.RoleCaption, 0)
	parts := []*model.Part{
		groupStart("s", "section"),
		blockPart(para),
		groupEnd("s"),
	}
	root := ProjectStream(parts)
	require.Len(t, root.Children, 1)
	sec := root.Children[0]
	assert.Equal(t, "section", sec.Role)
	require.Len(t, sec.Children, 1)
	assert.Equal(t, model.RoleCaption, sec.Children[0].Role)
}

func TestRenderNode_Walk(t *testing.T) {
	parts := []*model.Part{
		groupStart("t", "table"),
		groupStart("r0", "table-row"),
		blockPart(cellBlock("a", "A", false)),
		groupEnd("r0"),
		groupEnd("t"),
	}
	root := ProjectStream(parts)
	var roles []string
	root.Walk(func(n *RenderNode) bool {
		roles = append(roles, n.Role)
		return true
	})
	assert.Equal(t, []string{RoleDocument, model.RoleTable, RoleTableRow, model.RoleTableCell}, roles)
}

func TestAssembleTable(t *testing.T) {
	parts := []*model.Part{
		blockPart(model.NewBlock("before", "intro")), // ignored: outside the table
		groupStart("t", "table"),
		groupStart("r0", "table-row"),
		blockPart(cellBlock("a", "A", true)),
		blockPart(cellBlock("b", "B", true)),
		groupEnd("r0"),
		groupStart("r1", "table-row"),
		blockPart(cellBlock("c", "1", false)),
		blockPart(cellBlock("d", "2", false)),
		groupEnd("r1"),
		groupEnd("t"),
		blockPart(model.NewBlock("after", "tail")), // ignored: after the table
	}
	end, rows := AssembleTable(parts, 1) // start at the table GroupStart
	assert.Equal(t, 10, end, "end should be the table GroupEnd index")
	require.Len(t, rows, 2)
	assert.True(t, rows[0].Header, "header row flagged from cell roles")
	require.Len(t, rows[0].Cells, 2)
	assert.True(t, rows[0].Cells[0].Header)
	assert.Equal(t, "A", model.RunsText(rows[0].Cells[0].Block.Source))
	assert.False(t, rows[1].Header)
	assert.Equal(t, "2", model.RunsText(rows[1].Cells[1].Block.Source))
}
