package openxml

import (
	"bytes"
	"context"
	"io"
	"os"
	"strconv"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A worksheet has no row container to bracket — a row is an addressing fact,
// not a markup element — so cells arrive as a flat stream. The reader records
// each cell's geometry, but without a table-cell role nothing downstream saw a
// grid: core/projection assembles tables from cell roles, so every cell fell
// through as a standalone block and a spreadsheet exported as loose paragraphs.
//
// These pin the role and the row hint the flat-cell path groups on.

// readXLSXBlocks reads a .xlsx fixture and returns its blocks in stream order.
func readXLSXBlocks(t *testing.T, path string) []*model.Block {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	r := NewReader()
	ctx := context.Background()
	require.NoError(t, r.Open(ctx, &model.RawDocument{
		URI:          path,
		SourceLocale: model.LocaleEnglish,
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}))
	t.Cleanup(func() { _ = r.Close() })

	var blocks []*model.Block
	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part == nil {
			continue
		}
		if b, ok := pr.Part.Resource.(*model.Block); ok {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

func TestWorksheetCellsCarryTableCellRole(t *testing.T) {
	blocks := readXLSXBlocks(t, "testdata/EksempelFiltrering.xlsx")

	var cells int
	for _, b := range blocks {
		if b.Type != "cell" {
			continue
		}
		cells++
		assert.Equal(t, model.RoleTableCell, b.SemanticRole(),
			"cell %s should carry the canonical table-cell role", b.Properties["cell"])
		assert.NotEmpty(t, b.Properties[projection.PropFlatRow],
			"cell %s should carry the row hint flat-cell assembly groups on", b.Properties["cell"])
	}
	require.NotZero(t, cells, "fixture should contain worksheet cells")
}

// The row hint has to be the cell's actual row, or the grid reassembles wrong.
func TestWorksheetRowHintMatchesTheCellAddress(t *testing.T) {
	blocks := readXLSXBlocks(t, "testdata/EksempelFiltrering.xlsx")

	checked := 0
	for _, b := range blocks {
		ref := b.Properties["cell"]
		if b.Type != "cell" || ref == "" {
			continue
		}
		_, row, ok := parseCellRefA1(ref)
		require.True(t, ok, "unparseable cell reference %q", ref)
		assert.Equal(t, strconv.Itoa(row), b.Properties[projection.PropFlatRow],
			"cell %s row hint should match its address", ref)
		checked++
	}
	require.NotZero(t, checked)
}

// Cells of one row must project into one row, and distinct rows into distinct
// rows — the property the whole fix exists to deliver.
func TestWorksheetProjectsToAGrid(t *testing.T) {
	blocks := readXLSXBlocks(t, "testdata/EksempelFiltrering.xlsx")

	parts := make([]*model.Part, 0, len(blocks))
	for _, b := range blocks {
		if b.Type != "cell" {
			continue
		}
		parts = append(parts, &model.Part{Type: model.PartBlock, Resource: b})
	}
	require.NotEmpty(t, parts)

	root := projection.ProjectStream(parts)

	var table *projection.RenderNode
	for _, c := range root.Children {
		if c.Role == model.RoleTable {
			table = c
			break
		}
	}
	require.NotNil(t, table, "worksheet cells should project to a table")
	require.Greater(t, len(table.Children), 1,
		"a multi-row sheet should project to multiple rows, not one flat row")

	for _, row := range table.Children {
		assert.Equal(t, projection.RoleTableRow, row.Role)
		assert.NotEmpty(t, row.Children, "every row should hold cells")
	}
}

// A workbook is a sequence of sheets and that boundary is real: without it
// every sheet's cells arrive as one undifferentiated run, and a multi-sheet
// workbook exports as a single merged table whose rows come from different
// grids under one sheet's header.
func TestWorksheetsAreNamedAndSeparated(t *testing.T) {
	blocks := readXLSXBlocks(t, "testdata/EksempelFiltrering.xlsx")

	var names []string
	for _, b := range blocks {
		if b.Type == "sheet-name" {
			names = append(names, b.SourceText())
			assert.False(t, b.Translatable,
				"a sheet-name heading is surfaced for export, not for a translator")
			assert.Equal(t, model.RoleHeading, b.SemanticRole())
		}
	}
	assert.Equal(t, []string{"Data", "Selektering"}, names,
		"each worksheet should be labelled with the tab name a reader sees")

	// The heading must precede its own sheet's cells, or it labels the wrong grid.
	seen := ""
	byPart := map[string]string{}
	for _, b := range blocks {
		part := b.Properties["partPath"]
		if b.Type == "sheet-name" {
			seen = b.SourceText()
			byPart[part] = seen
			continue
		}
		if b.Type == "cell" && part != "" {
			assert.Equal(t, byPart[part], seen,
				"cell %s in %s is under heading %q", b.Properties["cell"], part, seen)
		}
	}
}

// Gated like every other additive surfacing: with the flag off the part stream
// is what the parity runner expects.
func TestSheetHeadingIsGatedOnNonTranslatableSurfacing(t *testing.T) {
	data, err := os.ReadFile("testdata/EksempelFiltrering.xlsx")
	require.NoError(t, err)

	r := NewReader()
	cfg, ok := r.Config().(*Config)
	require.True(t, ok)
	cfg.SetExtractNonTranslatableContent(false)

	ctx := context.Background()
	require.NoError(t, r.Open(ctx, &model.RawDocument{
		URI: "wb.xlsx", SourceLocale: model.LocaleEnglish,
		Reader: io.NopCloser(bytes.NewReader(data)),
	}))
	t.Cleanup(func() { _ = r.Close() })

	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part == nil {
			continue
		}
		if b, ok := pr.Part.Resource.(*model.Block); ok {
			assert.NotEqual(t, "sheet-name", b.Type)
		}
	}
}
