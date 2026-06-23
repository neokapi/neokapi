package openxml

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMergeCells(t *testing.T) {
	data := []byte(`<worksheet><sheetData/><mergeCells count="2">` +
		`<mergeCell ref="A1:C1"/><mergeCell ref="B3:B5"/></mergeCells></worksheet>`)
	m := parseMergeCells(data)
	require.Len(t, m, 2)
	assert.Equal(t, mergeSpan{cols: 3, rows: 1}, m["A1"], "A1:C1 spans 3 cols")
	assert.Equal(t, mergeSpan{cols: 1, rows: 3}, m["B3"], "B3:B5 spans 3 rows")
}

// rezipWithMergedHeader rebuilds the fixture xlsx, injecting a <mergeCells>
// range that merges the A1:B1 header into one banner cell.
func rezipWithMergedHeader(t *testing.T, src []byte) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(src), int64(len(src)))
	require.NoError(t, err)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		rc, err := f.Open()
		require.NoError(t, err)
		body, err := io.ReadAll(rc)
		rc.Close()
		require.NoError(t, err)
		if f.Name == "xl/worksheets/sheet1.xml" {
			body = []byte(strings.Replace(string(body), "</sheetData>",
				`</sheetData><mergeCells count="1"><mergeCell ref="A1:B1"/></mergeCells>`, 1))
		}
		w, err := zw.Create(f.Name)
		require.NoError(t, err)
		_, err = w.Write(body)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func TestWorksheetMergedCellGeometry(t *testing.T) {
	src, err := os.ReadFile("testdata/EksempelFiltrering.xlsx")
	require.NoError(t, err)
	merged := rezipWithMergedHeader(t, src)

	r := NewReader()
	doc := &model.RawDocument{URI: "merged.xlsx", SourceLocale: model.LocaleEnglish, Reader: io.NopCloser(bytes.NewReader(merged))}
	require.NoError(t, r.Open(context.Background(), doc))

	var a1 *model.Block
	for res := range r.Read(context.Background()) {
		require.NoError(t, res.Error)
		if res.Part == nil || res.Part.Type != model.PartBlock {
			continue
		}
		b, _ := res.Part.Resource.(*model.Block)
		if b != nil && b.Properties["cell"] == "A1" && b.Properties["partPath"] == "xl/worksheets/sheet1.xml" {
			a1 = b
		}
	}
	require.NotNil(t, a1, "A1 cell anchor should be emitted")
	g, ok := a1.Geometry()
	require.True(t, ok)
	assert.Equal(t, float64(2), g.BBox.W, "merged A1:B1 spans two columns in geometry")
	assert.Equal(t, float64(1), g.BBox.H)
}
