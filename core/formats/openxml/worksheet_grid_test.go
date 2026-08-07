package openxml

import (
	"archive/zip"
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

// A worksheet is a table and its rows are its rows, so the reader brackets them
// (table / table-row Groups) rather than stamping each cell with a row hint and
// leaving every consumer to rebuild the topology by buffering the whole grid —
// the un-normalized shape core/projection.flushFlatCells exists to salvage.
//
// These pin the brackets, the declared width that lets a consumer commit to a
// column count before writing its first row, and the flat fallback that
// survives for readers which do not bracket.

// readXLSXParts reads a .xlsx fixture and returns its full part stream.
func readXLSXParts(t *testing.T, path string) []*model.Part {
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

	var parts []*model.Part
	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part != nil {
			parts = append(parts, pr.Part)
		}
	}
	return parts
}

// readXLSXBlocks reads a .xlsx fixture and returns its blocks in stream order.
func readXLSXBlocks(t *testing.T, path string) []*model.Block {
	t.Helper()
	var blocks []*model.Block
	for _, p := range readXLSXParts(t, path) {
		if b, ok := p.Resource.(*model.Block); ok {
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
		assert.Empty(t, b.Properties[projection.PropFlatRow],
			"cell %s should not also carry a row hint — the bracket is the topology",
			b.Properties["cell"])
	}
	require.NotZero(t, cells, "fixture should contain worksheet cells")
}

// Every cell inside one table-row bracket must come from one sheet row, or the
// grid reassembles wrong.
func TestWorksheetRowsBracketTheirCells(t *testing.T) {
	parts := readXLSXParts(t, "testdata/EksempelFiltrering.xlsx")

	rows, inRow, rowNum := 0, false, -1
	for _, p := range parts {
		switch p.Type {
		case model.PartGroupStart:
			if g, ok := p.Resource.(*model.GroupStart); ok && g.Type == "table-row" {
				inRow, rowNum, rows = true, -1, rows+1
			}
		case model.PartGroupEnd:
			inRow = false
		case model.PartBlock:
			b, ok := p.Resource.(*model.Block)
			if !ok || b.Type != "cell" {
				continue
			}
			require.True(t, inRow, "cell %s is outside any row bracket", b.Properties["cell"])
			_, row, ok := parseCellRefA1(b.Properties["cell"])
			require.True(t, ok, "unparseable cell reference %q", b.Properties["cell"])
			if rowNum < 0 {
				rowNum = row
			}
			assert.Equal(t, rowNum, row, "one bracket should hold one sheet row")
		}
	}
	require.Greater(t, rows, 1, "a multi-row sheet should emit a bracket per row")
}

// The width travels with the table so a writer that must commit to a column
// count before its first row does not have to see the last cell first.
func TestWorksheetTableDeclaresItsWidth(t *testing.T) {
	parts := readXLSXParts(t, "testdata/EksempelFiltrering.xlsx")

	widest, declared := 0, -1
	inRow, cells := false, 0
	for _, p := range parts {
		switch p.Type {
		case model.PartGroupStart:
			g, ok := p.Resource.(*model.GroupStart)
			if !ok {
				continue
			}
			switch g.Type {
			case "table":
				n, err := strconv.Atoi(g.Properties["columns"])
				require.NoError(t, err, "a table group should declare its column count")
				if declared < 0 {
					declared = n
				}
			case "table-row":
				inRow, cells = true, 0
			}
		case model.PartGroupEnd:
			if inRow && cells > widest {
				widest = cells
			}
			inRow = false
		case model.PartBlock:
			if b, ok := p.Resource.(*model.Block); ok && b.Type == "cell" && inRow {
				cells++
			}
		}
	}
	require.Positive(t, declared, "no table group found")
	assert.GreaterOrEqual(t, declared, widest,
		"the declared width must cover the widest row, or a row would be truncated")
}

// Cells of one row must project into one row, and distinct rows into distinct
// rows — the property the whole grid shape exists to deliver.
func TestWorksheetProjectsToAGrid(t *testing.T) {
	parts := readXLSXParts(t, "testdata/EksempelFiltrering.xlsx")
	require.NotEmpty(t, parts)

	root := projection.ProjectStream(parts)

	var table *projection.RenderNode
	var find func(n *projection.RenderNode)
	find = func(n *projection.RenderNode) {
		if table != nil {
			return
		}
		for _, c := range n.Children {
			if c.Role == model.RoleTable {
				table = c
				return
			}
			find(c)
		}
	}
	find(root)
	require.NotNil(t, table, "worksheet cells should project to a table")
	require.Greater(t, len(table.Children), 1,
		"a multi-row sheet should project to multiple rows, not one flat row")

	for _, row := range table.Children {
		assert.Equal(t, projection.RoleTableRow, row.Role)
		assert.NotEmpty(t, row.Children, "every row should hold cells")
	}
}

// A reader that cannot bracket rows still stamps the hint, so the flat-cell
// fallback in core/projection keeps working. The sml parser brackets only when
// the reader wired an emitter for structure parts.
func TestUnbracketedCellsKeepTheRowHint(t *testing.T) {
	data, err := os.ReadFile("testdata/EksempelFiltrering.xlsx")
	require.NoError(t, err)

	blockCounter := 0
	cfg := &Config{}
	cfg.Reset()
	p := &smlParser{cfg: cfg, blockCounter: &blockCounter}

	sheet := xlsxPart(t, data, "xl/worksheets/sheet1.xml")
	var cells int
	require.NoError(t, p.parseWorksheet(sheet, "xl/worksheets/sheet1.xml", func(b *model.Block) {
		if b.Type != "cell" {
			return
		}
		cells++
		_, row, ok := parseCellRefA1(b.Properties["cell"])
		require.True(t, ok)
		assert.Equal(t, strconv.Itoa(row), b.Properties[projection.PropFlatRow],
			"an unbracketed cell should carry the row hint the fallback groups on")
	}))
	require.NotZero(t, cells)
}

// xlsxPart returns one entry of a .xlsx by name.
func xlsxPart(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err)
		defer rc.Close()
		b, err := io.ReadAll(rc)
		require.NoError(t, err)
		return b
	}
	t.Fatalf("%s not found in fixture", name)
	return nil
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
