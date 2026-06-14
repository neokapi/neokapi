package openxml

// okapi-filter: openxml

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	doclangfmt "github.com/neokapi/neokapi/core/formats/doclang"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Pure helper unit tests ---------------------------------------------------

func TestEMUToPt(t *testing.T) {
	// 914400 EMU = 1 inch = 72 pt; 12700 EMU = 1 pt.
	assert.Equal(t, 72.0, emuToPt(914400))
	assert.Equal(t, 1.0, emuToPt(12700))
	assert.Equal(t, 0.0, emuToPt(0))
	assert.Equal(t, 0.5, emuToPt(6350))
}

func TestPPTXSlideNum(t *testing.T) {
	cases := []struct {
		path string
		want int
	}{
		{"ppt/slides/slide1.xml", 1},
		{"ppt/slides/slide12.xml", 12},
		{"ppt/slideLayouts/slideLayout1.xml", 0}, // layouts have no page
		{"ppt/slideMasters/slideMaster1.xml", 0}, // masters have no page
		{"ppt/notesSlides/notesSlide1.xml", 0},   // notes have no page
		{"ppt/presentation.xml", 0},
		{"ppt/slides/slide.xml", 0}, // no number
		{"ppt/slides/slideX.xml", 0},
		{"", 0},
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, pptxSlideNum(c.path), "pptxSlideNum(%q)", c.path)
	}
}

func TestParseCellRefA1(t *testing.T) {
	cases := []struct {
		ref      string
		col, row int
		ok       bool
	}{
		{"A1", 0, 0, true},
		{"B3", 1, 2, true},
		{"Z1", 25, 0, true},
		{"AA1", 26, 0, true},
		{"AB12", 27, 11, true},
		{"a1", 0, 0, true}, // case-insensitive
		{"", 0, 0, false},
		{"A", 0, 0, false},   // no row
		{"1", 0, 0, false},   // no column
		{"A0", 0, 0, false},  // row < 1
		{"1A", 0, 0, false},  // digits before letters
		{"A1B", 0, 0, false}, // trailing letters
		{"A+1", 0, 0, false}, // signed row (Atoi would accept; regex does not)
		{"A-1", 0, 0, false}, // signed row
	}
	for _, c := range cases {
		col, row, ok := parseCellRefA1(c.ref)
		assert.Equalf(t, c.ok, ok, "ok for %q", c.ref)
		if c.ok {
			assert.Equalf(t, c.col, col, "col for %q", c.ref)
			assert.Equalf(t, c.row, row, "row for %q", c.ref)
		}
	}
}

func TestSheetNumFromPath(t *testing.T) {
	cases := []struct {
		path string
		want int
	}{
		{"xl/worksheets/sheet1.xml", 1},
		{"xl/worksheets/sheet10.xml", 10},
		{"xl/tables/table1.xml", 0},
		{"xl/sharedStrings.xml", 0},
		{"xl/workbook.xml", 0},
		{"xl/worksheets/sheet.xml", 0},
		{"", 0},
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, sheetNumFromPath(c.path), "sheetNumFromPath(%q)", c.path)
	}
}

// --- PPTX shape geometry (integration) ---------------------------------------

// minimalPPTX builds the smallest PPTX ZIP the reader will accept: a content
// type that triggers PresentationML detection, a slide relationship, and one
// slide part with the given <p:spTree> body.
func minimalPPTX(t *testing.T, slideBody string) *bytes.Reader {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZipEntry(t, zw, "[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>
  <Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>
</Types>`)
	writeZipEntry(t, zw, "_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/>
</Relationships>`)
	writeZipEntry(t, zw, "ppt/presentation.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"/>`)
	writeZipEntry(t, zw, "ppt/_rels/presentation.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide1.xml"/>
</Relationships>`)
	writeZipEntry(t, zw, "ppt/slides/slide1.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
  <p:cSld><p:spTree>`+slideBody+`</p:spTree></p:cSld>
</p:sld>`)
	require.NoError(t, zw.Close())
	return bytes.NewReader(buf.Bytes())
}

// sp builds a <p:sp> with an optional <a:xfrm> and a single text paragraph.
func sp(off, ext, text string) string {
	xfrm := ""
	if off != "" || ext != "" {
		xfrm = `<a:xfrm>` + off + ext + `</a:xfrm>`
	}
	return `<p:sp><p:spPr>` + xfrm + `</p:spPr><p:txBody><a:bodyPr/><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></p:txBody></p:sp>`
}

func blockByText(blocks []*model.Block, text string) *model.Block {
	for _, b := range blocks {
		if b.SourceText() == text {
			return b
		}
	}
	return nil
}

func TestNative_PPTXShapeGeometry(t *testing.T) {
	body := sp(`<a:off x="914400" y="457200"/>`, `<a:ext cx="1828800" cy="685800"/>`, "Positioned title") +
		sp("", "", "No transform here") +
		// A group shape whose own transform is present but whose child shape
		// carries child-space coordinates — geometry must be omitted for the child.
		`<p:grpSp><p:grpSpPr><a:xfrm><a:off x="100" y="200"/><a:ext cx="300" cy="400"/><a:chOff x="0" y="0"/><a:chExt cx="300" cy="400"/></a:xfrm></p:grpSpPr>` +
		sp(`<a:off x="100" y="100"/>`, `<a:ext cx="100" cy="100"/>`, "Inside a group") +
		`</p:grpSp>`

	reader := NewReader()
	doc := testutil.RawDocFromReader(minimalPPTX(t, body), "test.pptx", model.LocaleEnglish)
	require.NoError(t, reader.Open(t.Context(), doc))
	defer reader.Close()
	blocks := translatableBlocks(testutil.CollectParts(t, reader.Read(t.Context())))

	// The positioned top-level shape carries geometry derived from its xfrm:
	// 914400 EMU = 72pt, 457200 = 36pt, 1828800 = 144pt, 685800 = 54pt.
	pos := blockByText(blocks, "Positioned title")
	require.NotNil(t, pos, "expected the positioned title block")
	g, ok := pos.Geometry()
	require.True(t, ok, "positioned shape should carry geometry")
	assert.Equal(t, 1, g.Page, "page = slide number")
	assert.Equal(t, "top-left", g.Origin)
	assert.Equal(t, 0, g.Resolution, "absolute points → resolution 0")
	assert.Equal(t, model.Rect{X: 72, Y: 36, W: 144, H: 54}, g.BBox)

	// A shape without its own <a:xfrm> inherits no box.
	noxfrm := blockByText(blocks, "No transform here")
	require.NotNil(t, noxfrm)
	_, ok = noxfrm.Geometry()
	assert.False(t, ok, "shape without xfrm should have no geometry")

	// A shape inside a group is skipped (child-space coords need an affine remap).
	grouped := blockByText(blocks, "Inside a group")
	require.NotNil(t, grouped)
	_, ok = grouped.Geometry()
	assert.False(t, ok, "grouped shape should have no geometry in v1")
}

// TestNative_PPTXGeometryNotOnNonSlideParts confirms geometry is attached only
// for ppt/slides/slideN.xml parts. A slide master (slideNum == 0) gets none even
// though its shapes carry xfrm transforms.
func TestNative_PPTXGeometryNonSlideOff(t *testing.T) {
	// A non-slide part path → pptxSlideNum returns 0 → attachShapeGeometry is a
	// no-op. We exercise the parser directly to avoid wiring a master relationship.
	p := &dmlParser{blockCounter: new(int), slideNum: 0}
	var got []*model.Block
	body := []byte(`<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree>` +
		sp(`<a:off x="914400" y="457200"/>`, `<a:ext cx="914400" cy="914400"/>`, "Master text") +
		`</p:spTree></p:cSld></p:sld>`)
	require.NoError(t, p.parsePart(body, "ppt/slideMasters/slideMaster1.xml", func(b *model.Block) { got = append(got, b) }))
	require.Len(t, got, 1)
	_, ok := got[0].Geometry()
	assert.False(t, ok, "non-slide part should derive no geometry")
}

// --- XLSX cell-grid geometry (integration) -----------------------------------

func minimalXLSX(t *testing.T, sheetData, sharedStrings string) *bytes.Reader {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZipEntry(t, zw, "[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
</Types>`)
	writeZipEntry(t, zw, "_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`)
	writeZipEntry(t, zw, "xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheets><sheet name="Sheet1" sheetId="1" r:id="rId1" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"/></sheets></workbook>`)
	writeZipEntry(t, zw, "xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/>
</Relationships>`)
	writeZipEntry(t, zw, "xl/worksheets/sheet1.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`+sheetData+`</sheetData></worksheet>`)
	writeZipEntry(t, zw, "xl/sharedStrings.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`+sharedStrings+`</sst>`)
	require.NoError(t, zw.Close())
	return bytes.NewReader(buf.Bytes())
}

func TestNative_XLSXCellGeometry(t *testing.T) {
	sheetData := `<row r="1">` +
		`<c r="A1" t="inlineStr"><is><t>Inline label</t></is></c>` +
		`<c r="B1" t="s"><v>0</v></c>` +
		`</row>` +
		`<row r="3"><c r="C3" t="inlineStr"><is><t>Another cell</t></is></c></row>`
	sharedStrings := `<si><t>Shared label</t></si>`

	reader := NewReader()
	doc := testutil.RawDocFromReader(minimalXLSX(t, sheetData, sharedStrings), "test.xlsx", model.LocaleEnglish)
	require.NoError(t, reader.Open(t.Context(), doc))
	defer reader.Close()
	blocks := translatableBlocks(testutil.CollectParts(t, reader.Read(t.Context())))

	// A1 inline cell → zero-based (col 0, row 0), one-cell box, cell-grid origin.
	a1 := blockByText(blocks, "Inline label")
	require.NotNil(t, a1, "expected the A1 inline cell block")
	g, ok := a1.Geometry()
	require.True(t, ok, "inline-string cell should carry cell-grid geometry")
	assert.Equal(t, 1, g.Page, "page = sheet number")
	assert.Equal(t, "cell-grid", g.Origin)
	assert.Equal(t, model.Rect{X: 0, Y: 0, W: 1, H: 1}, g.BBox)

	// C3 → (col 2, row 2).
	c3 := blockByText(blocks, "Another cell")
	require.NotNil(t, c3)
	g, ok = c3.Geometry()
	require.True(t, ok)
	assert.Equal(t, model.Rect{X: 2, Y: 2, W: 1, H: 1}, g.BBox)

	// The shared string is deduplicated in sharedStrings.xml — one block backs
	// potentially many cells — so it carries no single position.
	shared := blockByText(blocks, "Shared label")
	require.NotNil(t, shared, "expected the shared-string block")
	_, ok = shared.Geometry()
	assert.False(t, ok, "shared-string cell has no single position → no geometry")
}

// --- End-to-end: derived PPTX geometry → DocLang <location> export -----------

// TestNative_PPTXGeometryToDocLang reads a positioned PPTX shape and projects
// the resulting block through the DocLang writer, asserting the derived box
// surfaces as the four <location> values DocLang uses (x0,y0,x1,y1).
func TestNative_PPTXGeometryToDocLang(t *testing.T) {
	body := sp(`<a:off x="914400" y="457200"/>`, `<a:ext cx="1828800" cy="685800"/>`, "Positioned title")

	reader := NewReader()
	doc := testutil.RawDocFromReader(minimalPPTX(t, body), "test.pptx", model.LocaleEnglish)
	require.NoError(t, reader.Open(t.Context(), doc))
	defer reader.Close()
	blocks := translatableBlocks(testutil.CollectParts(t, reader.Read(t.Context())))
	pos := blockByText(blocks, "Positioned title")
	require.NotNil(t, pos)
	if _, ok := pos.Geometry(); !ok {
		t.Fatal("precondition: derived block must carry geometry")
	}

	w := doclangfmt.NewWriter()
	var out bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&out))
	layer := &model.Layer{ID: "doc1", Format: "doclang"}
	ch := make(chan *model.Part, 4)
	ch <- &model.Part{Type: model.PartLayerStart, Resource: layer}
	ch <- &model.Part{Type: model.PartBlock, Resource: pos}
	ch <- &model.Part{Type: model.PartLayerEnd, Resource: layer}
	close(ch)
	require.NoError(t, w.Write(context.Background(), ch))

	// x0=72, y0=36, x1=72+144=216, y1=36+54=90.
	got := out.String()
	for _, v := range []string{
		`<location value="72"/>`,
		`<location value="36"/>`,
		`<location value="216"/>`,
		`<location value="90"/>`,
	} {
		assert.Containsf(t, got, v, "DocLang export should carry %s\n%s", v, got)
	}
}
