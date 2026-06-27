package openxml

import (
	"testing"

	"github.com/neokapi/neokapi/core/internal/testutil"
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

func TestDocxVMergeRowspan(t *testing.T) {
	// A 2x2 table whose first column is vertically merged: the restart cell
	// (row 1, col 0) spans both rows; the continuation cell (row 2, col 0) is
	// empty and must not appear as its own cell.
	const doc = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:tbl>
<w:tr>
<w:tc><w:tcPr><w:vMerge w:val="restart"/></w:tcPr><w:p><w:r><w:t>Merged</w:t></w:r></w:p></w:tc>
<w:tc><w:p><w:r><w:t>R1C2</w:t></w:r></w:p></w:tc>
</w:tr>
<w:tr>
<w:tc><w:tcPr><w:vMerge/></w:tcPr><w:p/></w:tc>
<w:tc><w:p><w:r><w:t>R2C2</w:t></w:r></w:p></w:tc>
</w:tr>
</w:tbl>
</w:body></w:document>`

	input := docxWithDocumentXML(t, doc)
	reader := NewReader() // no skeleton store → cross-format path emits groups + roles
	require.NoError(t, reader.Open(t.Context(), &model.RawDocument{
		URI: "t.docx", SourceLocale: model.LocaleEnglish, Encoding: "UTF-8",
		Reader: readCloserFromBytes(input),
	}))
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	reader.Close()

	// The merged cell carries RowSpan == 2.
	var merged *model.Block
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && model.RunsText(b.Source) == "Merged" {
			merged = b
		}
	}
	require.NotNil(t, merged, "merged cell block not found")
	s, ok := merged.Structure()
	require.True(t, ok)
	assert.Equal(t, 2, s.RowSpan, "restart cell should span 2 rows")

	// Projection: row 0 has two cells (Merged, R1C2); row 1 has one (R2C2) —
	// the empty continuation cell is dropped.
	var table *projection.RenderNode
	projection.ProjectStream(parts).Walk(func(n *projection.RenderNode) bool {
		if n.Role == model.RoleTable && table == nil {
			table = n
		}
		return true
	})
	require.NotNil(t, table)
	require.Len(t, table.Children, 2, "two rows")
	assert.Len(t, table.Children[0].Children, 2, "row 1: merged + R1C2")
	require.Len(t, table.Children[1].Children, 1, "row 2: continuation dropped, only R2C2")
	assert.Equal(t, "R2C2", table.Children[1].Children[0].Text())
	assert.Equal(t, 2, table.Children[0].Children[0].RowSpan, "projected merged cell rowspan")
}
