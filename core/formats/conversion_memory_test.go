package formats_test

// A cross-format conversion streams: the reader's Parts go straight into the
// writer, and a writer that can render as they arrive does. Peak memory is then
// a function of the widest row, not of how many rows there are.
//
// This is the conversion-path counterpart to openxml's
// TestPackageReadBoundedMemory, and it guards the pair of buffers #1710 removed
// — the runner's collect-then-replay slice and the writer's own event list.
// Either one alone pins the whole block graph, so a regression in either brings
// the scaling back.

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/require"
)

// writeGridXLSX builds a .xlsx of rows × 8 inline-string cells, each carrying
// cellText characters, so a caller can vary the cell count while holding the
// sheet's byte size roughly constant.
func writeGridXLSX(t *testing.T, dir string, rows, cellText int) (path string, sheetBytes int) {
	t.Helper()

	body := strings.Repeat("word ", cellText/5)
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
		fmt.Sprintf(`<dimension ref="A1:H%d"/><sheetData>`, rows))
	cols := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	for r := 1; r <= rows; r++ {
		fmt.Fprintf(&sheet, `<row r="%d">`, r)
		for c, name := range cols {
			fmt.Fprintf(&sheet,
				`<c r="%s%d" t="inlineStr"><is><t>cell %d/%d %s</t></is></c>`,
				name, r, r, c, body)
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)

	path = filepath.Join(dir, fmt.Sprintf("grid-%d-%d.xlsx", rows, cellText))
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	zw := zip.NewWriter(f)
	add := func(name, body string) {
		w, werr := zw.Create(name)
		require.NoError(t, werr)
		_, werr = w.Write([]byte(body))
		require.NoError(t, werr)
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
	return path, sheet.Len()
}

// convertStreaming mirrors what host.convertDocument does: configure the writer,
// then pipe the reader into it. Peak live heap is sampled as parts flow.
func convertStreaming(t *testing.T, reg *registry.FormatRegistry, path string) (peak uint64) {
	t.Helper()

	r, err := reg.NewReader("openxml")
	require.NoError(t, err)
	if sa, ok := r.(format.SubfilterAware); ok {
		sa.SetSubfilterResolver(reg)
	}
	w, err := reg.NewWriter("markdown")
	require.NoError(t, err)
	require.NoError(t, w.SetOutputWriter(io.Discard))

	ctx := context.Background()
	doc := testutil.RawDocFromFile(t, path, model.LocaleEnglish)
	require.NoError(t, r.Open(ctx, doc))
	t.Cleanup(func() { _ = r.Close() })

	liveHeap := func() uint64 {
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.HeapAlloc
	}
	base := liveHeap()

	ch := make(chan *model.Part, 64)
	go func() {
		defer close(ch)
		n := 0
		for res := range r.Read(ctx) {
			if res.Error != nil || res.Part == nil {
				continue
			}
			ch <- res.Part
			// Sample often enough to catch a buffer growing, rarely enough that
			// the forced GC does not dominate the run.
			if n++; n%512 == 0 {
				if h := liveHeap(); h > base && h-base > peak {
					peak = h - base
				}
			}
		}
	}()
	require.NoError(t, w.Write(ctx, ch))
	require.NoError(t, w.Close())
	return peak
}

// Peak must be a function of how much text a sheet holds, not of how many
// blocks it becomes.
//
// Two sheets of the same size carry it differently: one as 160k small cells,
// the other as 16k cells around ten times as long. Holding the byte size
// constant takes the reader's per-part read buffer out of the comparison — that
// buffer is still O(document) and is a separate problem — leaving the block
// graph as the only thing that differs between the runs. A writer or runner
// that collects the Part stream retains ten times as much for the first sheet
// as for the second; a streaming pair retains neither.
func TestSpreadsheetConversionDoesNotRetainBlocks(t *testing.T) {
	if testing.Short() {
		t.Skip("memory test skipped in -short")
	}
	defer debug.SetGCPercent(debug.SetGCPercent(20))

	dir := t.TempDir()
	reg := convRegistry()

	manyPath, manyBytes := writeGridXLSX(t, dir, 20_000, 20) // 160k cells
	fewPath, fewBytes := writeGridXLSX(t, dir, 2_000, 740)   // 16k cells

	// The comparison only isolates block retention if the reader's per-part
	// buffer is the same on both sides, so hold the sheets to the same size
	// rather than assuming it.
	ratio := float64(manyBytes) / float64(fewBytes)
	require.InDelta(t, 1.0, ratio, 0.15,
		"the two sheets must be comparable in size (%d vs %d bytes)", manyBytes, fewBytes)
	t.Logf("sheet XML: 160k cells = %d KiB, 16k cells = %d KiB", manyBytes/1024, fewBytes/1024)

	many := convertStreaming(t, reg, manyPath)
	few := convertStreaming(t, reg, fewPath)
	t.Logf("peak: 160k cells = %d KiB, 16k cells = %d KiB", many/1024, few/1024)

	// A tenfold difference in block count must not be a tenfold difference in
	// peak. The floor keeps a run whose peak is mostly fixed overhead from
	// being judged on a ratio of noise.
	floor := max(few, uint64(8<<20))
	require.LessOrEqual(t, many, floor*2,
		"peak scaled with block count (%d KiB for 160k cells vs %d KiB for 16k), "+
			"which is what collecting the Part stream looks like",
		many/1024, few/1024)
}

// The streamed table must render the same bytes the buffered path did — the
// grid is the same grid however the writer got to it.
func TestStreamedGridRendersEveryRow(t *testing.T) {
	dir := t.TempDir()
	reg := convRegistry()
	path, _ := writeGridXLSX(t, dir, 40, 20)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	md, err := convertOnce(reg, data, convFixture{name: "grid", path: path, format: "openxml"})
	require.NoError(t, err)
	out := string(md)

	// 40 sheet rows render as 40 table rows plus the GFM delimiter.
	rows := 0
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "|") {
			rows++
		}
	}
	require.Equal(t, 41, rows, "every sheet row should reach the output exactly once")
	require.Contains(t, out, "| --- | --- | --- | --- | --- | --- | --- | --- |",
		"the delimiter row should carry the declared width")
	require.Contains(t, out, "| cell 40/0 ", "the last row should be present")
	require.NotContains(t, out, "|  |  |  |  |  |  |  |  |",
		"no empty rows should be invented")
}
