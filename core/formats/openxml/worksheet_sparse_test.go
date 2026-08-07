package openxml

// A worksheet omits empty cells rather than writing blanks, so a row that skips
// columns arrives as fewer cells than the grid is wide. Every consumer of the
// grid places cells by the address the reader stamps on them; without it they go
// in sequence and the values land under the wrong headings (#1711).

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sparseSheetXLSX builds a one-sheet workbook from A1-style cell references, so
// a test can express a grid with holes in it directly.
func sparseSheetXLSX(t *testing.T, cells map[string]string) []byte {
	t.Helper()

	byRow := map[int][]string{}
	var order []int
	for ref, text := range cells {
		_, row, ok := parseCellRefA1(ref)
		require.True(t, ok, "bad reference %q", ref)
		if _, seen := byRow[row]; !seen {
			order = append(order, row)
		}
		byRow[row] = append(byRow[row],
			fmt.Sprintf(`<c r="%s" t="inlineStr"><is><t>%s</t></is></c>`, ref, text))
	}
	// A worksheet lists rows and cells in ascending order; the map that built
	// them did not.
	slices.Sort(order)

	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for _, row := range order {
		cs := byRow[row]
		slices.Sort(cs)
		fmt.Fprintf(&sheet, `<row r="%d">%s</row>`, row, strings.Join(cs, ""))
	}
	sheet.WriteString(`</sheetData></worksheet>`)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, body string) {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(body))
		require.NoError(t, err)
	}
	add("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`+
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`+
		`<Default Extension="xml" ContentType="application/xml"/>`+
		`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`+
		`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`+
		`</Types>`)
	add("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>`+
		`</Relationships>`)
	add("xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" `+
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`+
		`<sheets><sheet name="Grid" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	add("xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>`+
		`</Relationships>`)
	add("xl/worksheets/sheet1.xml", sheet.String())
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func TestSparseCellsCarryTheirColumn(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/sparse.xlsx"
	require.NoError(t, writeFile(path, sparseSheetXLSX(t, map[string]string{
		"A1": "Region", "B1": "Q1", "C1": "Q2", "D1": "Q3", "E1": "Q4",
		"A2": "EMEA", "C2": "118", "E2": "204", // B and D empty
	})))

	byText := map[string]string{}
	for _, b := range readXLSXBlocks(t, path) {
		if b.Type == "cell" {
			byText[b.SourceText()] = b.Properties["column"]
		}
	}
	// 118 is in column C and 204 in column E, whatever order they arrived in.
	assert.Equal(t, "2", byText["118"], "a cell must carry its own column, not its position in the row")
	assert.Equal(t, "4", byText["204"])
	assert.Equal(t, "0", byText["EMEA"])
}

// The declared width has to cover the furthest column, not the fullest row, or
// a sparse row's trailing cells would be placed past the end and dropped.
func TestDeclaredWidthCoversTheFurthestColumn(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/sparse.xlsx"
	require.NoError(t, writeFile(path, sparseSheetXLSX(t, map[string]string{
		"A1": "one", "E1": "five", // two cells, five columns wide
	})))

	declared := -1
	for _, p := range readXLSXParts(t, path) {
		if g, ok := p.Resource.(*model.GroupStart); ok && g.Type == "table" {
			n, err := strconv.Atoi(g.Properties["columns"])
			require.NoError(t, err)
			declared = n
			break
		}
	}
	assert.Equal(t, 5, declared,
		"a row of two cells at A and E occupies five columns, not two")
}

// The streamed and buffered worksheet paths must agree: one reads the part from
// its zip entry, the other from a copy of it, and nothing else differs.
func TestStreamedWorksheetMatchesBuffered(t *testing.T) {
	data := sparseSheetXLSX(t, map[string]string{
		"A1": "Region", "B1": "Q1", "C1": "Q2",
		"A2": "EMEA", "C2": "118",
		"A3": "APAC", "B3": "77", "C3": "91",
	})
	dir := t.TempDir()
	path := dir + "/sheet.xlsx"
	require.NoError(t, writeFile(path, data))

	// A file source streams the part; a plain stream buffers it.
	streamed := partSignature(readXLSXParts(t, path))
	buffered := partSignature(readXLSXPartsBuffered(t, data))
	assert.Equal(t, buffered, streamed)
}

// writeFile is os.WriteFile at the permissions these fixtures use.
func writeFile(path string, data []byte) error { return os.WriteFile(path, data, 0o644) }

// readXLSXPartsBuffered reads a workbook from a plain stream, which gives the
// reader no random access and so takes the buffered part path.
func readXLSXPartsBuffered(t *testing.T, data []byte) []*model.Part {
	t.Helper()
	r := NewReader()
	require.NoError(t, r.Open(t.Context(), &model.RawDocument{
		URI:          "buffered.xlsx",
		SourceLocale: model.LocaleEnglish,
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}))
	t.Cleanup(func() { _ = r.Close() })

	var parts []*model.Part
	for pr := range r.Read(t.Context()) {
		require.NoError(t, pr.Error)
		if pr.Part != nil {
			parts = append(parts, pr.Part)
		}
	}
	return parts
}

func partSignature(parts []*model.Part) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		switch r := p.Resource.(type) {
		case *model.Block:
			out = append(out, fmt.Sprintf("block %s %s col=%s", r.Type, r.SourceText(), r.Properties["column"]))
		case *model.GroupStart:
			out = append(out, "start "+r.Type+" cols="+r.Properties["columns"])
		case *model.GroupEnd:
			out = append(out, "end")
		}
	}
	return out
}
