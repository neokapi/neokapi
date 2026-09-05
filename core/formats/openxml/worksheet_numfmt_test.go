package openxml

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A value cell stores a number and shows it through its number format. The
// reader keeps the stored value as the cell's text and stamps the display
// beside it, so the round-trip and the block's identity never see the format
// while every export and preview reads the way the sheet does.

// readXLSXBytes reads an in-memory workbook and returns its blocks.
func readXLSXBytes(t *testing.T, data []byte) []*model.Block {
	t.Helper()
	r := NewReader()
	ctx := context.Background()
	require.NoError(t, r.Open(ctx, &model.RawDocument{
		URI:          "numfmt.xlsx",
		SourceLocale: model.LocaleEnglish,
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}))
	t.Cleanup(func() { _ = r.Close() })
	return testutil.FilterBlocks(testutil.CollectParts(t, r.Read(ctx)))
}

// sheetCells indexes a workbook's first-sheet blocks by cell reference.
func sheetCells(blocks []*model.Block) map[string]*model.Block {
	out := map[string]*model.Block{}
	for _, b := range blocks {
		if b.Properties["partPath"] == "xl/worksheets/sheet1.xml" && b.Properties["cell"] != "" {
			out[b.Properties["cell"]] = b
		}
	}
	return out
}

func TestWorksheetValueCellsCarryTheirDisplay(t *testing.T) {
	cells := sheetCells(readXLSXBytes(t, testutil.XLSXNumberFormats(t, false)))

	cases := []struct {
		ref     string
		stored  string
		display string
		format  string
	}{
		{"A2", "44197", "01-01-21", "mm-dd-yy"},
		{"B2", "0.125", "12.5%", "0.0%"},
		{"C2", "1234.5", "$1,234.50", `"$"#,##0.00;("$"#,##0.00);"-"`},
		{"D2", "1234567", "1,234,567", "#,##0"},
		{"E2", "1", "TRUE", "General"},
		{"F2", "Date", "Date", "@"},
		{"A3", "60", "1900-02-29", "yyyy-mm-dd"},
		{"B3", "-0.5", "-50.0%", "0.0%"},
		{"C3", "-1234.5", "($1,234.50)", `"$"#,##0.00;("$"#,##0.00);"-"`},
		{"D3", "0", "0", "#,##0"},
		{"E3", "0", "FALSE", "General"},
		{"A4", "1992", "1992", "General"},
		{"B4", "0", "0.0%", "0.0%"},
		{"C4", "0", "-", `"$"#,##0.00;("$"#,##0.00);"-"`},
		{"D4", "5", "5", "General"}, // style index 7 is beyond the stylesheet
	}
	for _, c := range cases {
		b := cells[c.ref]
		require.NotNil(t, b, "cell %s should be surfaced", c.ref)
		assert.Equal(t, c.stored, model.RunsText(b.Source), "%s keeps the stored value", c.ref)
		assert.Equal(t, c.display, b.Properties[model.PropCellDisplay], "%s display", c.ref)
		assert.Equal(t, c.format, b.Properties[model.PropCellFormat], "%s format", c.ref)
		assert.False(t, b.Translatable, "%s is a value cell", c.ref)
	}

	// A text cell shows its text: no display is stamped on a shared-string
	// anchor or an inline string, so the translatable text stays the one
	// thing a consumer renders.
	for _, ref := range []string{"A1", "F3"} {
		b := cells[ref]
		require.NotNil(t, b, "cell %s should be surfaced", ref)
		_, has := b.Properties[model.PropCellDisplay]
		assert.False(t, has, "%s is a text cell and carries no display", ref)
	}
	assert.True(t, cells["F3"].Translatable)
	assert.Equal(t, "hello", model.RunsText(cells["F3"].Source))
}

func TestWorksheetDisplayFollowsTheDate1904System(t *testing.T) {
	cells := sheetCells(readXLSXBytes(t, testutil.XLSXNumberFormats(t, true)))
	assert.Equal(t, "01-02-25", cells["A2"].Properties[model.PropCellDisplay], "serial 44197 from 1904-01-01")
	assert.Equal(t, "1904-03-01", cells["A3"].Properties[model.PropCellDisplay], "serial 60 is a real day in the 1904 system")
	assert.Equal(t, "44197", model.RunsText(cells["A2"].Source))
}

// zipPart returns one entry of a zip package.
func zipPart(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	f := zipFileByName(zr, name)
	require.NotNil(t, f, "part %s", name)
	rc, err := f.Open()
	require.NoError(t, err)
	defer rc.Close()
	body, err := io.ReadAll(rc)
	require.NoError(t, err)
	return body
}

// The display is additive: the skeleton carries the stored value, so the
// written worksheet is byte-identical to the one read. The workbook holds
// shared-string and value cells only; an inline string is re-written by the
// writer as a value element, which is a separate matter from the display.
func TestWorksheetDisplayLeavesTheRoundTripUntouched(t *testing.T) {
	original := testutil.BuildXLSX(t, testutil.XLSX{
		SharedStrings: []string{"Date", "Share", "Price"},
		NumFmts:       map[int]string{164: "0.0%", 165: `"$"#,##0.00;("$"#,##0.00);"-"`},
		CellXfs:       []int{0, 14, 164, 165},
		Sheets: []testutil.XLSXSheet{{
			Name: "Data",
			SheetData: `<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="s"><v>2</v></c></row>` +
				`<row r="2"><c r="A2" s="1"><v>44197</v></c><c r="B2" s="2"><v>0.125</v></c><c r="C2" s="3"><v>-1234.5</v></c><c r="D2" t="b"><v>1</v></c></row>`,
		}},
	})

	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := NewReader()
	reader.SetSkeletonStore(skelStore)
	require.NoError(t, reader.Open(t.Context(), &model.RawDocument{
		URI:          "numfmt.xlsx",
		SourceLocale: model.LocaleEnglish,
		Reader:       readCloserFromBytes(original),
	}))
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	reader.Close()

	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	writer.SetSkeletonStore(skelStore)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(t.Context(), testutil.PartsToChannel(parts)))
	writer.Close()

	for _, name := range []string{"xl/worksheets/sheet1.xml", "xl/styles.xml", "xl/workbook.xml"} {
		assert.Equal(t, string(zipPart(t, original, name)), string(zipPart(t, buf.Bytes(), name)), "%s round-trips byte for byte", name)
	}
	assertSkeletonRoundtrip(t, original, "numfmt.xlsx")
}

// The fixture's cell styles all name the General format, so every value cell
// displays as stored; the stylesheet is parsed and resolved all the same.
func TestFixtureValueCellsDisplayAsGeneral(t *testing.T) {
	blocks := readXLSXBlocks(t, "testdata/EksempelFiltrering.xlsx")
	valueCells := 0
	for _, b := range blocks {
		if b.Properties["cell"] == "" || b.Translatable || b.Properties["siIndex"] != "" {
			continue
		}
		valueCells++
		assert.Equal(t, "General", b.Properties[model.PropCellFormat], "cell %s", b.Properties["cell"])
		assert.Equal(t, model.RunsText(b.Source), b.Properties[model.PropCellDisplay], "cell %s", b.Properties["cell"])
	}
	require.Greater(t, valueCells, 20, "the fixture holds a column of years and two of numbers")

	cells := sheetCells(blocks)
	require.NotNil(t, cells["E2"])
	assert.Equal(t, "1992", cells["E2"].Properties[model.PropCellDisplay])
}

// Without a stylesheet resolver (a parser built by hand) every cell displays
// through General.
func TestWorksheetWithoutStylesDisplaysGeneral(t *testing.T) {
	wb := testutil.BuildXLSX(t, testutil.XLSX{Sheets: []testutil.XLSXSheet{{
		Name:      "Sheet1",
		SheetData: `<row r="1"><c r="A1" s="3"><v>0.30000000000000004</v></c><c r="B1" t="b"><v>1</v></c></row>`,
	}}})
	cells := sheetCells(readXLSXBytes(t, wb))
	require.NotNil(t, cells["A1"])
	assert.Equal(t, "0.3", cells["A1"].Properties[model.PropCellDisplay])
	assert.Equal(t, "General", cells["A1"].Properties[model.PropCellFormat])
	assert.Equal(t, "0.30000000000000004", model.RunsText(cells["A1"].Source))
	assert.Equal(t, "TRUE", cells["B1"].Properties[model.PropCellDisplay])
}
