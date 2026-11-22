package openxml

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	nsPML = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"`
	nsWML = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"`
)

// readBlocksWith opens data with cfg applied and returns every block,
// translatable or not.
func readBlocksWith(t *testing.T, data []byte, uri string, cfg func(*Config)) []*model.Block {
	t.Helper()
	reader := NewReader()
	if cfg != nil {
		cfg(reader.cfg)
	}
	doc := testutil.RawDocFromReader(bytes.NewReader(data), uri, model.LocaleEnglish)
	require.NoError(t, reader.Open(t.Context(), doc))
	defer reader.Close()
	var blocks []*model.Block
	for _, p := range testutil.CollectParts(t, reader.Read(t.Context())) {
		if b, ok := p.Resource.(*model.Block); ok && p.Type == model.PartBlock {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// twoSlideDeck builds a PPTX whose second slide is hidden (show="0") and
// carries speaker notes.
func twoSlideDeck(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZipEntry(t, zw, "[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>
</Types>`)
	writeZipEntry(t, zw, "_rels/.rels", `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/>
</Relationships>`)
	writeZipEntry(t, zw, "ppt/presentation.xml", `<?xml version="1.0" encoding="UTF-8"?>
<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"/>`)
	writeZipEntry(t, zw, "ppt/_rels/presentation.xml.rels", `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide2.xml"/>
</Relationships>`)
	writeZipEntry(t, zw, "ppt/slides/slide1.xml", `<p:sld `+nsPML+`><p:cSld><p:spTree>`+sp("", "", "Visible slide")+`</p:spTree></p:cSld></p:sld>`)
	writeZipEntry(t, zw, "ppt/slides/slide2.xml", `<p:sld `+nsPML+` show="0"><p:cSld><p:spTree>`+sp("", "", "Hidden slide")+`</p:spTree></p:cSld></p:sld>`)
	writeZipEntry(t, zw, "ppt/slides/_rels/slide2.xml.rels", `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/notesSlide" Target="../notesSlides/notesSlide2.xml"/>
</Relationships>`)
	writeZipEntry(t, zw, "ppt/notesSlides/notesSlide2.xml", `<p:notes `+nsPML+`><p:cSld><p:spTree>`+sp("", "", "Hidden slide note")+`</p:spTree></p:cSld></p:notes>`)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// A hidden slide and its notes are left out by default and extracted, marked
// hidden, with translate_hidden_slides.
func TestNative_PPTXHiddenSlide(t *testing.T) {
	data := twoSlideDeck(t)

	blocks := readBlocksWith(t, data, "deck.pptx", nil)
	assert.NotNil(t, blockByText(blocks, "Visible slide"))
	assert.Nil(t, blockByText(blocks, "Hidden slide"), "hidden slide skipped by default")
	assert.Nil(t, blockByText(blocks, "Hidden slide note"), "hidden slide's notes skipped by default")

	blocks = readBlocksWith(t, data, "deck.pptx", func(c *Config) { c.TranslateHiddenSlides = true })
	visible := blockByText(blocks, "Visible slide")
	require.NotNil(t, visible)
	assert.Equal(t, model.VisibilityVisible, visible.Visibility())
	for _, text := range []string{"Hidden slide", "Hidden slide note"} {
		b := blockByText(blocks, text)
		require.NotNilf(t, b, "%q extracted with translate_hidden_slides", text)
		assert.Equalf(t, model.VisibilityHidden, b.Visibility(), "%q marked hidden", text)
	}
}

// Every paragraph of a slide table cell carries the cell's 0-based column and
// row, in the keys the worksheet reader uses.
func TestNative_PPTXTableCellAddress(t *testing.T) {
	cell := func(attrs, paras string) string {
		return `<a:tc` + attrs + `><a:txBody><a:bodyPr/>` + paras + `</a:txBody><a:tcPr/></a:tc>`
	}
	para := func(text string) string { return `<a:p><a:r><a:t>` + text + `</a:t></a:r></a:p>` }
	body := `<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="4" name="Table"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr>` +
		`<p:xfrm><a:off x="0" y="0"/><a:ext cx="5486400" cy="914400"/></p:xfrm>` +
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/table"><a:tbl>` +
		`<a:tblGrid><a:gridCol w="1828800"/><a:gridCol w="1828800"/><a:gridCol w="1828800"/></a:tblGrid>` +
		`<a:tr h="457200">` + cell("", para("A")) + cell("", para("B")) + cell("", para("C")) + `</a:tr>` +
		`<a:tr h="457200">` + cell(` gridSpan="2"`, para("Wide")) + cell(` hMerge="1"`, "") + cell("", para("First")+para("Second")) + `</a:tr>` +
		`</a:tbl></a:graphicData></a:graphic></p:graphicFrame>`

	data, err := io.ReadAll(minimalPPTX(t, body))
	require.NoError(t, err)
	blocks := readBlocksWith(t, data, "t.pptx", nil)

	cases := map[string][2]string{ // text → {row, column}
		"A": {"0", "0"}, "B": {"0", "1"}, "C": {"0", "2"},
		"Wide": {"1", "0"}, "First": {"1", "2"}, "Second": {"1", "2"},
	}
	for text, want := range cases {
		b := blockByText(blocks, text)
		require.NotNilf(t, b, "block %q", text)
		assert.Equalf(t, want[0], b.Properties[projection.PropFlatRow], "%q row", text)
		assert.Equalf(t, want[1], b.Properties["column"], "%q column", text)
	}
}

// hiddenWorkbook builds an XLSX with a visible sheet holding a hidden column
// (B) and a hidden row (2), and a second sheet the workbook marks hidden.
func hiddenWorkbook(t *testing.T, state string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZipEntry(t, zw, "[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
</Types>`)
	writeZipEntry(t, zw, "_rels/.rels", `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`)
	writeZipEntry(t, zw, "xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>`+
		`<sheet name="Shown" sheetId="1" r:id="rId1"/><sheet name="Internal" sheetId="2" state="`+state+`" r:id="rId2"/></sheets></workbook>`)
	writeZipEntry(t, zw, "xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/>
</Relationships>`)
	inline := func(ref, text string) string {
		return `<c r="` + ref + `" t="inlineStr"><is><t>` + text + `</t></is></c>`
	}
	writeZipEntry(t, zw, "xl/worksheets/sheet1.xml", `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`+
		`<cols><col min="1" max="1" width="10"/><col min="2" max="2" width="10" hidden="1"/></cols><sheetData>`+
		`<row r="1">`+inline("A1", "Plain cell")+inline("B1", "Hidden column cell")+`</row>`+
		`<row r="2" hidden="1">`+inline("A2", "Hidden row cell")+`</row>`+
		`<row r="3">`+inline("C3", "After the hidden row")+`</row>`+
		`</sheetData></worksheet>`)
	writeZipEntry(t, zw, "xl/worksheets/sheet2.xml", `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`+
		`<row r="1">`+inline("A1", "Hidden sheet cell")+`</row></sheetData></worksheet>`)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// Cells on a hidden sheet, in a hidden column or in a hidden row are
// extracted and marked hidden; the sheet's name heading is marked too.
func TestNative_XLSXHiddenContent(t *testing.T) {
	for _, state := range []string{"hidden", "veryHidden"} {
		t.Run(state, func(t *testing.T) {
			blocks := readBlocksWith(t, hiddenWorkbook(t, state), "book.xlsx", nil)
			want := map[string]string{
				"Plain cell":           model.VisibilityVisible,
				"After the hidden row": model.VisibilityVisible,
				"Shown":                model.VisibilityVisible,
				"Hidden column cell":   model.VisibilityHidden,
				"Hidden row cell":      model.VisibilityHidden,
				"Hidden sheet cell":    model.VisibilityHidden,
				"Internal":             model.VisibilityHidden,
			}
			for text, vis := range want {
				b := blockByText(blocks, text)
				require.NotNilf(t, b, "block %q (have %v)", text, testutil.BlockTexts(blocks))
				assert.Equalf(t, vis, b.Visibility(), "%q visibility", text)
			}
		})
	}
}

// docxWithRunningParts builds a DOCX with a header, a footer and a comment.
func docxWithRunningParts(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZipEntry(t, zw, "[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`)
	writeZipEntry(t, zw, "_rels/.rels", `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`)
	writeZipEntry(t, zw, "word/_rels/document.xml.rels", `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/comments" Target="comments.xml"/>
</Relationships>`)
	writeZipEntry(t, zw, "word/document.xml", `<w:document `+nsWML+`><w:body>`+body+`</w:body></w:document>`)
	// A header paragraph styled as a heading is still a running header.
	writeZipEntry(t, zw, "word/header1.xml", `<w:hdr `+nsWML+`><w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Running header</w:t></w:r></w:p></w:hdr>`)
	writeZipEntry(t, zw, "word/footer1.xml", `<w:ftr `+nsWML+`><w:p><w:r><w:t>Running footer</w:t></w:r></w:p></w:ftr>`)
	writeZipEntry(t, zw, "word/comments.xml", `<w:comments `+nsWML+`><w:comment w:id="0" w:author="A"><w:p><w:r><w:t>Reviewer comment</w:t></w:r></w:p></w:comment></w:comments>`)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// Header and footer paragraphs carry the page-header and page-footer roles on
// the furniture plane; a comment is metadata with no role.
func TestNative_DocxRunningPartRoles(t *testing.T) {
	blocks := readBlocksWith(t, docxWithRunningParts(t, `<w:p><w:r><w:t>Body text</w:t></w:r></w:p>`), "d.docx", nil)

	cases := []struct{ text, role, layer string }{
		{"Body text", "", ""},
		{"Running header", model.RolePageHeader, model.LayerFurniture},
		{"Running footer", model.RolePageFooter, model.LayerFurniture},
		{"Reviewer comment", "", model.LayerMetadata},
	}
	for _, c := range cases {
		b := blockByText(blocks, c.text)
		require.NotNilf(t, b, "block %q (have %v)", c.text, testutil.BlockTexts(blocks))
		assert.Equalf(t, c.role, b.SemanticRole(), "%q role", c.text)
		assert.Equalf(t, c.layer, b.LayoutLayer(), "%q layer", c.text)
	}
}

// With the default translate_hidden_text=false a fully hidden paragraph yields
// no block.
func TestNative_DocxHiddenParagraphSkippedByDefault(t *testing.T) {
	body := `<w:p><w:r><w:rPr><w:vanish/></w:rPr><w:t>Hidden line</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>Visible line</w:t></w:r></w:p>`
	blocks := readBlocksWith(t, docxWithRunningParts(t, body), "d.docx", nil)
	assert.Nil(t, blockByText(blocks, "Hidden line"))
	assert.NotNil(t, blockByText(blocks, "Visible line"))
}
