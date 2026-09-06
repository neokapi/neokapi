package formats_test

// A spreadsheet's rich text carries two kinds of run property: the ones the
// model names (bold, italic, …) and the ones it does not (colour, size,
// typeface). A cross-format export renders the first and drops the second,
// because Markdown and HTML have nothing to say about a workbook's <rFont>.
//
// The opaque code the reader uses for the second kind must therefore stay
// invisible outside OpenXML. It carries SpreadsheetML source bytes, and a
// conversion that echoed them would put `<sz val="12"/>` in the output.

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// richTextXLSX is a one-cell workbook whose shared string mixes a plain run, a
// bold red run and a plain run.
func richTextXLSX(t *testing.T) []byte {
	t.Helper()
	const nsMain = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	const nsRel = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	const nsPkg = "http://schemas.openxmlformats.org/package/2006/relationships"
	const ctBase = "application/vnd.openxmlformats-officedocument.spreadsheetml"

	parts := [][2]string{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/xl/workbook.xml" ContentType="` + ctBase + `.sheet.main+xml"/>` +
			`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="` + ctBase + `.worksheet+xml"/>` +
			`<Override PartName="/xl/sharedStrings.xml" ContentType="` + ctBase + `.sharedStrings+xml"/>` +
			`</Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="` + nsPkg + `">` +
			`<Relationship Id="rId1" Type="` + nsRel + `/officeDocument" Target="xl/workbook.xml"/>` +
			`</Relationships>`},
		{"xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<workbook xmlns="` + nsMain + `" xmlns:r="` + nsRel + `">` +
			`<sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="` + nsPkg + `">` +
			`<Relationship Id="rId1" Type="` + nsRel + `/worksheet" Target="worksheets/sheet1.xml"/>` +
			`<Relationship Id="rId2" Type="` + nsRel + `/sharedStrings" Target="sharedStrings.xml"/>` +
			`</Relationships>`},
		{"xl/worksheets/sheet1.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<worksheet xmlns="` + nsMain + `">` +
			`<sheetData><row r="1"><c r="A1" t="s"><v>0</v></c></row></sheetData></worksheet>`},
		{"xl/sharedStrings.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<sst xmlns="` + nsMain + `" count="1" uniqueCount="1">` +
			`<si><r><rPr><sz val="12"/><rFont val="Arial"/></rPr><t xml:space="preserve">Normal and </t></r>` +
			`<r><rPr><b/><sz val="12"/><color rgb="FFFF0000"/><rFont val="Arial"/></rPr><t>Red</t></r>` +
			`<r><rPr><sz val="12"/><rFont val="Arial"/></rPr><t xml:space="preserve"> text</t></r></si>` +
			`</sst>`},
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range parts {
		w, err := zw.Create(p[0])
		require.NoError(t, err)
		_, err = w.Write([]byte(p[1]))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// TestSpreadsheetRichTextExportsNamedFormattingOnly checks both halves of the
// export: the bold the model names reaches HTML as <strong>, and the
// SpreadsheetML the model does not name reaches neither writer.
//
// Markdown shows the cell's text without the bold because a GFM table cell
// renders through the cell's display property (projection.DisplayRuns), which
// is the same for every source format and is not what this test is about.
func TestSpreadsheetRichTextExportsNamedFormattingOnly(t *testing.T) {
	reg := convRegistry()
	data := richTextXLSX(t)
	f := convFixture{name: "xlsx/rich-text", path: "rich.xlsx", format: "openxml"}

	md, err := convertOnce(reg, data, f)
	require.NoError(t, err)
	out := string(md)
	assert.Contains(t, out, "Normal and Red text", "the cell's text reaches the Markdown table")
	assertNoSpreadsheetMarkup(t, out)

	html, err := convertTo(reg, data, f, "html")
	require.NoError(t, err)
	outHTML := string(html)
	assert.Contains(t, outHTML, "Normal and <strong>Red</strong> text",
		"the bold run exports as the HTML element the vocabulary names")
	assertNoSpreadsheetMarkup(t, outHTML)
}

// assertNoSpreadsheetMarkup fails when a run property the model does not name
// has leaked into a cross-format export.
func assertNoSpreadsheetMarkup(t *testing.T, out string) {
	t.Helper()
	for _, leak := range []string{`<sz `, `<rFont `, `<color `, `rgb="FFFF0000"`, "val=\"12\""} {
		assert.NotContains(t, out, leak,
			"the opaque run-property code must not spell SpreadsheetML outside OpenXML")
	}
}

// convertTo is convertOnce with the target format named, so one fixture can be
// exported through more than one writer.
func convertTo(reg *registry.FormatRegistry, data []byte, f convFixture, target string) ([]byte, error) {
	r, err := reg.NewReader(registry.FormatID(f.format))
	if err != nil {
		return nil, err
	}
	if sa, ok := r.(format.SubfilterAware); ok {
		sa.SetSubfilterResolver(reg)
	}
	ctx := context.Background()
	if err := r.Open(ctx, rawDoc(data, f.path, f.format)); err != nil {
		return nil, err
	}
	var parts []*model.Part
	for pr := range r.Read(ctx) {
		if pr.Error != nil {
			_ = r.Close()
			return nil, pr.Error
		}
		parts = append(parts, pr.Part)
	}
	if err := r.Close(); err != nil {
		return nil, err
	}

	w, err := reg.NewWriter(registry.FormatID(target))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := w.SetOutputWriter(&buf); err != nil {
		return nil, err
	}
	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	if err := w.Write(ctx, ch); err != nil {
		return nil, err
	}
	return buf.Bytes(), w.Close()
}
