package openxml

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A DOCX table must surface as the canonical table/table-row Group shape with
// RoleTableCell cells (preview-fidelity #3), so cross-format writers and the
// projection rebuild the grid instead of seeing flat paragraphs. The byte-exact
// docx round-trip is covered separately (roundtrip_test.go) and is unaffected,
// because the Groups carry no skeleton bytes.
func TestDocxTableCapture(t *testing.T) {
	parts := readDocx(t, "testdata/test_859.docx")

	var tableGroups, rowGroups, cellBlocks int
	for _, p := range parts {
		switch p.Type {
		case model.PartGroupStart:
			if g, ok := p.Resource.(*model.GroupStart); ok {
				switch g.Type {
				case "table":
					tableGroups++
				case "table-row":
					rowGroups++
				}
			}
		case model.PartBlock:
			if b, ok := p.Resource.(*model.Block); ok && b.SemanticRole() == model.RoleTableCell {
				cellBlocks++
			}
		}
	}
	require.Positive(t, tableGroups, "no table Group emitted for a table-bearing docx")
	assert.GreaterOrEqual(t, rowGroups, 2, "expected at least two table-row Groups")
	assert.Positive(t, cellBlocks, "no RoleTableCell blocks emitted")

	// The projection must rebuild a table with rows of cells.
	root := projection.ProjectStream(parts)
	var found *projection.RenderNode
	root.Walk(func(n *projection.RenderNode) bool {
		if n.Role == model.RoleTable && found == nil {
			found = n
		}
		return true
	})
	require.NotNil(t, found, "projection did not rebuild a table")
	require.GreaterOrEqual(t, len(found.Children), 2, "table should have >= 2 rows")
	assert.Equal(t, projection.RoleTableRow, found.Children[0].Role)
	assert.NotEmpty(t, found.Children[0].Children, "row should have cells")
}

func TestGridSpanFromTcPr(t *testing.T) {
	cases := map[string]int{
		`<w:tcPr><w:gridSpan w:val="3"/></w:tcPr>`:                   3,
		`<w:tcPr><w:tcW w:w="100"/><w:gridSpan w:val="2"/></w:tcPr>`: 2,
		`<w:tcPr></w:tcPr>`:                            0,
		`<w:tcPr><w:vMerge w:val="restart"/></w:tcPr>`: 0,
		`<w:tcPr><w:gridSpan w:val="1"/></w:tcPr>`:     1,
	}
	for raw, want := range cases {
		assert.Equal(t, want, gridSpanFromTcPr(raw), raw)
	}
}
