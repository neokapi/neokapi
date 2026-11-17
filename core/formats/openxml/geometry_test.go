package openxml

// okapi-filter: openxml

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
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
		// A group placed at (2in, 1in), 4in × 2in on the slide, whose child
		// space is 8in × 4in: its children are drawn at half scale.
		`<p:grpSp><p:grpSpPr><a:xfrm><a:off x="1828800" y="914400"/><a:ext cx="3657600" cy="1828800"/><a:chOff x="0" y="0"/><a:chExt cx="7315200" cy="3657600"/></a:xfrm></p:grpSpPr>` +
		sp(`<a:off x="914400" y="914400"/>`, `<a:ext cx="1828800" cy="914400"/>`, "Inside a group") +
		`</p:grpSp>` +
		// A group with no transform of its own cannot be placed.
		`<p:grpSp><p:grpSpPr/>` +
		sp(`<a:off x="914400" y="914400"/>`, `<a:ext cx="914400" cy="914400"/>`, "Inside an unplaced group") +
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

	// A grouped shape is mapped out of the group's child space: the child at
	// (1in, 1in), 2in × 1in lands at (2in + 0.5in, 1in + 0.5in), 1in × 0.5in.
	grouped := blockByText(blocks, "Inside a group")
	require.NotNil(t, grouped)
	g, ok = grouped.Geometry()
	require.True(t, ok, "grouped shape should be placed through the group transform")
	assert.Equal(t, model.Rect{X: 180, Y: 108, W: 72, H: 36}, g.BBox)

	unplaced := blockByText(blocks, "Inside an unplaced group")
	require.NotNil(t, unplaced)
	_, ok = unplaced.Geometry()
	assert.False(t, ok, "a shape in a group without a transform should have no geometry")
}

// pptxWithLayout builds a one-slide PPTX whose slide inherits from a layout,
// and the layout from a master, so placeholder frames can be resolved.
func pptxWithLayout(t *testing.T, slideBody, layoutBody, masterBody string) *bytes.Reader {
	t.Helper()
	const ns = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"`
	const rel = `http://schemas.openxmlformats.org/officeDocument/2006/relationships/`
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZipEntry(t, zw, "[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>
  <Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>
  <Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/>
  <Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>
</Types>`)
	writeZipEntry(t, zw, "_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="`+rel+`officeDocument" Target="ppt/presentation.xml"/>
</Relationships>`)
	writeZipEntry(t, zw, "ppt/presentation.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:presentation `+ns+`/>`)
	writeZipEntry(t, zw, "ppt/_rels/presentation.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="`+rel+`slideMaster" Target="slideMasters/slideMaster1.xml"/>
  <Relationship Id="rId2" Type="`+rel+`slide" Target="slides/slide1.xml"/>
</Relationships>`)
	writeZipEntry(t, zw, "ppt/slides/_rels/slide1.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="`+rel+`slideLayout" Target="../slideLayouts/slideLayout1.xml"/>
</Relationships>`)
	writeZipEntry(t, zw, "ppt/slideLayouts/_rels/slideLayout1.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="`+rel+`slideMaster" Target="../slideMasters/slideMaster1.xml"/>
</Relationships>`)
	writeZipEntry(t, zw, "ppt/slideMasters/_rels/slideMaster1.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="`+rel+`slideLayout" Target="../slideLayouts/slideLayout1.xml"/>
</Relationships>`)
	writeZipEntry(t, zw, "ppt/slides/slide1.xml", `<p:sld `+ns+`><p:cSld><p:spTree>`+slideBody+`</p:spTree></p:cSld></p:sld>`)
	writeZipEntry(t, zw, "ppt/slideLayouts/slideLayout1.xml", `<p:sldLayout `+ns+`><p:cSld><p:spTree>`+layoutBody+`</p:spTree></p:cSld></p:sldLayout>`)
	writeZipEntry(t, zw, "ppt/slideMasters/slideMaster1.xml", `<p:sldMaster `+ns+`><p:cSld><p:spTree>`+masterBody+`</p:spTree></p:cSld></p:sldMaster>`)
	require.NoError(t, zw.Close())
	return bytes.NewReader(buf.Bytes())
}

// phSp builds a placeholder <p:sp> with the given <p:ph> attributes, an
// optional transform and one paragraph of text.
func phSp(phAttrs, xfrm, text string) string {
	if xfrm != "" {
		xfrm = `<a:xfrm>` + xfrm + `</a:xfrm>`
	}
	return `<p:sp><p:nvSpPr><p:cNvPr id="2" name="ph"/><p:cNvSpPr/><p:nvPr><p:ph ` + phAttrs + `/></p:nvPr></p:nvSpPr>` +
		`<p:spPr>` + xfrm + `</p:spPr><p:txBody><a:bodyPr/><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></p:txBody></p:sp>`
}

// TestNative_PPTXPlaceholderGeometry checks that a placeholder with no
// transform of its own takes the frame of the matching placeholder on its
// layout, or on the layout's master when the layout states none either.
func TestNative_PPTXPlaceholderGeometry(t *testing.T) {
	inch := func(x, y, cx, cy int) string {
		return fmt.Sprintf(`<a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/>`, x*914400, y*914400, cx*914400, cy*914400)
	}
	slide := phSp(`type="ctrTitle"`, "", "Centred title") +
		phSp(`type="subTitle" idx="1"`, "", "Subtitle by idx") +
		phSp(`type="dt" idx="10"`, "", "Date from master") +
		phSp(`idx="13"`, "", "Unknown placeholder") +
		phSp(`type="ftr" idx="11"`, inch(5, 6, 2, 1), "Own frame wins")
	layout := phSp(`type="ctrTitle"`, inch(1, 2, 8, 1), "Layout title prompt") +
		phSp(`type="subTitle" idx="1"`, "", "Layout subtitle without frame") +
		phSp(`type="dt" sz="half" idx="10"`, "", "Layout date without frame")
	master := phSp(`type="title"`, inch(0, 0, 10, 1), "Master title") +
		phSp(`type="body" idx="1"`, inch(1, 3, 6, 3), "Master body") +
		phSp(`type="dt" sz="half" idx="2"`, inch(0, 7, 2, 1), "Master date")

	reader := NewReader()
	doc := testutil.RawDocFromReader(pptxWithLayout(t, slide, layout, master), "test.pptx", model.LocaleEnglish)
	require.NoError(t, reader.Open(t.Context(), doc))
	defer reader.Close()
	blocks := translatableBlocks(testutil.CollectParts(t, reader.Read(t.Context())))

	cases := []struct {
		text string
		want *model.Rect
	}{
		// The layout's centred title states a frame.
		{"Centred title", &model.Rect{X: 72, Y: 144, W: 576, H: 72}},
		// The layout's subtitle (matched by idx) has no frame, so the master's
		// body placeholder supplies it.
		{"Subtitle by idx", &model.Rect{X: 72, Y: 216, W: 432, H: 216}},
		// The date matches by idx on the layout, by type on the master, whose
		// date placeholder has a different idx.
		{"Date from master", &model.Rect{X: 0, Y: 504, W: 144, H: 72}},
		// An untyped placeholder no layout placeholder matches falls back to
		// the master's body.
		{"Unknown placeholder", &model.Rect{X: 72, Y: 216, W: 432, H: 216}},
		{"Own frame wins", &model.Rect{X: 360, Y: 432, W: 144, H: 72}},
	}
	for _, c := range cases {
		b := blockByText(blocks, c.text)
		require.NotNilf(t, b, "block %q", c.text)
		g, ok := b.Geometry()
		require.Truef(t, ok, "%q should carry geometry", c.text)
		assert.Equalf(t, 1, g.Page, "%q page", c.text)
		assert.Equalf(t, *c.want, g.BBox, "%q box", c.text)
	}
}

// TestNative_PPTXTableCellGeometry checks that each cell of a slide table gets
// its own frame from the grid's column widths and the rows' heights.
func TestNative_PPTXTableCellGeometry(t *testing.T) {
	cell := func(attrs, text string) string {
		return `<a:tc` + attrs + `><a:txBody><a:bodyPr/><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></a:txBody><a:tcPr/></a:tc>`
	}
	body := `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="4" name="Table"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr>` +
		`<p:xfrm><a:off x="914400" y="1828800"/><a:ext cx="5486400" cy="914400"/></p:xfrm>` +
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/table"><a:tbl><a:tblPr firstRow="1"/>` +
		`<a:tblGrid><a:gridCol w="1828800"/><a:gridCol w="1828800"/><a:gridCol w="1828800"/></a:tblGrid>` +
		`<a:tr h="457200">` + cell("", "Region") + cell("", "Units") + cell("", "Note") + `</a:tr>` +
		`<a:tr h="457200">` + cell("", "North") + cell(` gridSpan="2"`, "Spans two columns") + cell(` hMerge="1"`, "") + `</a:tr>` +
		`</a:tbl></a:graphicData></a:graphic></p:graphicFrame>`

	reader := NewReader()
	doc := testutil.RawDocFromReader(minimalPPTX(t, body), "test.pptx", model.LocaleEnglish)
	require.NoError(t, reader.Open(t.Context(), doc))
	defer reader.Close()
	blocks := translatableBlocks(testutil.CollectParts(t, reader.Read(t.Context())))

	cases := map[string]model.Rect{
		"Region":            {X: 72, Y: 144, W: 144, H: 36},
		"Units":             {X: 216, Y: 144, W: 144, H: 36},
		"Note":              {X: 360, Y: 144, W: 144, H: 36},
		"North":             {X: 72, Y: 180, W: 144, H: 36},
		"Spans two columns": {X: 216, Y: 180, W: 288, H: 36},
	}
	for text, want := range cases {
		b := blockByText(blocks, text)
		require.NotNilf(t, b, "block %q", text)
		g, ok := b.Geometry()
		require.Truef(t, ok, "%q should carry geometry", text)
		assert.Equalf(t, want, g.BBox, "%q box", text)
	}
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

	// The shared string is deduplicated in sharedStrings.xml: one block backs
	// potentially many cells, so it carries no single position.
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
