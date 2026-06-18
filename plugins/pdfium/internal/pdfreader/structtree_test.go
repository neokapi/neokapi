//go:build pdfium_experimental

package pdfreader

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// Tier-1 (tagged-PDF structure tree) requires the experimental PDFium MCID APIs,
// which go-pdfium only wires under the pdfium_experimental build tag — so this
// test is gated to that tag. tagged_table.pdf is a self-authored fixture
// (Chrome --export-tagged-pdf of a heading + paragraph + 3×3 table + paragraph);
// it exercises the StructTree → MCID → text bridge end to end.
func TestReadParts_TaggedStructTree(t *testing.T) {
	data, err := os.ReadFile("testdata/tagged_table.pdf")
	require.NoError(t, err)

	parts, err := ReadParts(data, model.LocaleEnglish, "tagged_table.pdf", Options{Geometry: true})
	require.NoError(t, err)
	requireBalancedLayers(t, parts)

	// The heading and surrounding prose come through as blocks with semantic roles.
	var (
		headingText  string
		sawParagraph bool
	)
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		switch b.SemanticRole() {
		case model.RoleHeading:
			headingText = b.SourceText()
		case model.RoleParagraph:
			if strings.Contains(b.SourceText(), "table below") {
				sawParagraph = true
			}
		}
	}
	assert.Equal(t, "Quarterly Results", headingText, "H1 read from the struct tree")
	assert.True(t, sawParagraph, "intro paragraph read from the struct tree")

	// The HTML table becomes a table group: 4 rows (1 header + 3 body), 3 cols.
	tbl := extractTable(t, parts)
	require.NotNil(t, tbl, "tagged table must be emitted as a table group")
	require.Len(t, tbl.rows, 4, "header row + 3 body rows")
	for ci := range tbl.rows {
		require.Len(t, tbl.rows[ci], 3, "three columns per row")
	}

	// Header row: TH cells carry the table-header role and the column labels.
	hdr := tbl.rows[0]
	for _, c := range hdr {
		assert.Equal(t, model.RoleTableHeader, c.role, "first row cells are table headers")
	}
	assert.Equal(t, []string{"Metric", "2024", "2025"}, []string{hdr[0].text, hdr[1].text, hdr[2].text})

	// A body row: TD cells carry the table-cell role and the data.
	body := tbl.rows[1]
	for _, c := range body {
		assert.Equal(t, model.RoleTableCell, c.role, "body cells are table cells")
	}
	assert.Equal(t, []string{"Revenue", "10", "12"}, []string{body[0].text, body[1].text, body[2].text})
}

type cellInfo struct {
	text string
	role string
}

type tableInfo struct {
	rows [][]cellInfo
}

// extractTable walks the part stream for the first table group and reconstructs
// its rows of cells (text + semantic role) from the nested table-row groups.
func extractTable(t *testing.T, parts []*model.Part) *tableInfo {
	t.Helper()
	var (
		tbl     *tableInfo
		inTable bool
		curRow  []cellInfo
		inRow   bool
	)
	for _, p := range parts {
		switch p.Type {
		case model.PartGroupStart:
			g := p.Resource.(*model.GroupStart)
			switch g.Type {
			case "table":
				if tbl == nil {
					tbl = &tableInfo{}
					inTable = true
				}
			case "table-row":
				if inTable {
					inRow = true
					curRow = nil
				}
			}
		case model.PartGroupEnd:
			if inRow {
				tbl.rows = append(tbl.rows, curRow)
				inRow = false
			} else if inTable && tbl != nil {
				inTable = false
			}
		case model.PartBlock:
			if inRow {
				b := p.Resource.(*model.Block)
				curRow = append(curRow, cellInfo{text: b.SourceText(), role: b.SemanticRole()})
			}
		}
	}
	return tbl
}
