package openxml

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const stylesheetFixture = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <numFmts count="2">
    <numFmt numFmtId="164" formatCode="0.0%"/>
    <numFmt numFmtId="14" formatCode="yyyy-mm-dd"/>
    <numFmt numFmtId="165" formatCode="&quot;$&quot;#,##0.00"/>
  </numFmts>
  <fonts count="1"><font><sz val="11"/><name val="Calibri"/></font></fonts>
  <cellStyleXfs count="2">
    <xf numFmtId="0" fontId="0"/>
    <xf numFmtId="9" fontId="0"/>
  </cellStyleXfs>
  <cellXfs count="6">
    <xf numFmtId="0" fontId="0" xfId="0"/>
    <xf numFmtId="14" fontId="0" xfId="0" applyNumberFormat="1"/>
    <xf numFmtId="164" fontId="0" xfId="0" applyNumberFormat="1"/>
    <xf numFmtId="27" fontId="0" xfId="0" applyNumberFormat="1"/>
    <xf numFmtId="10" fontId="0" xfId="0" applyNumberFormat="1"/>
    <xf numFmtId="165" fontId="0" xfId="0" applyNumberFormat="1"/>
  </cellXfs>
</styleSheet>`

func TestCellStylesResolveFormatCodes(t *testing.T) {
	s := &cellStyles{numFmts: map[int]string{}}
	s.parseStylesheet(strings.NewReader(stylesheetFixture))

	require.Len(t, s.xfNumFmt, 6, "only cellXfs entries are indexed, never cellStyleXfs")
	assert.Equal(t, map[int]string{164: "0.0%", 14: "yyyy-mm-dd", 165: `"$"#,##0.00`}, s.numFmts)

	cases := []struct {
		style string
		code  string
		ok    bool
	}{
		{"0", "General", true},
		{"1", "yyyy-mm-dd", true}, // a stylesheet may redefine a built-in id
		{"2", "0.0%", true},
		{"3", "General", false}, // built-in 27 has no invariant code
		{"4", "0.00%", true},
		{"5", `"$"#,##0.00`, true}, // attribute entities are decoded
		{"", "General", true},
		{"99", "General", true},
		{"x", "General", true},
	}
	for _, c := range cases {
		code, ok := s.formatCode(c.style)
		assert.Equal(t, c.code, code, "style %q", c.style)
		assert.Equal(t, c.ok, ok, "style %q", c.style)
	}
}

func TestCellStylesNilResolvesGeneral(t *testing.T) {
	var s *cellStyles
	code, ok := s.formatCode("3")
	assert.Equal(t, "General", code)
	assert.True(t, ok)
	assert.False(t, s.epoch1904())
}

func TestCellStylesMalformedKeepsWhatWasRead(t *testing.T) {
	s := &cellStyles{numFmts: map[int]string{}}
	s.parseStylesheet(strings.NewReader(`<styleSheet><cellXfs><xf numFmtId="9"/><xf numFmtId="14"/><xf numFmtId=`))
	assert.Equal(t, []int{9, 14}, s.xfNumFmt)
}

// zipWithWorkbook builds a zip holding one xl/workbook.xml with the given
// workbookPr element.
func zipWithWorkbook(t *testing.T, workbookPr string) *zip.Reader {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("xl/workbook.xml")
	require.NoError(t, err)
	_, err = w.Write([]byte(`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` + workbookPr + `<sheets/></workbook>`))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	return zr
}

func TestParseDate1904(t *testing.T) {
	cases := []struct {
		workbookPr string
		want       bool
	}{
		{`<workbookPr date1904="1"/>`, true},
		{`<workbookPr date1904="true"/>`, true},
		{`<workbookPr date1904="0"/>`, false},
		{`<workbookPr date1904="false" defaultThemeVersion="1"/>`, false},
		{`<workbookPr defaultThemeVersion="124226"/>`, false},
		{``, false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, parseDate1904(zipWithWorkbook(t, c.workbookPr)), "workbookPr %q", c.workbookPr)
	}

	var empty bytes.Buffer
	zw := zip.NewWriter(&empty)
	require.NoError(t, zw.Close())
	zr, err := zip.NewReader(bytes.NewReader(empty.Bytes()), int64(empty.Len()))
	require.NoError(t, err)
	assert.False(t, parseDate1904(zr), "no workbook part means the 1900 system")
}

func TestParseCellStylesFollowsTheStylesRelationship(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"xl/workbook.xml": `<workbook><workbookPr date1904="1"/></workbook>`,
		"xl/theme/custom-styles.xml": `<styleSheet><numFmts><numFmt numFmtId="164" formatCode="0%"/></numFmts>` +
			`<cellXfs><xf numFmtId="164"/></cellXfs></styleSheet>`,
	} {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)

	rels := map[string][]relationship{
		"xl/_rels/workbook.xml.rels": {{
			ID:     "rId9",
			Type:   "http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles",
			Target: "theme/custom-styles.xml",
		}},
	}
	s := parseCellStyles(zr, rels)
	assert.True(t, s.epoch1904())
	code, ok := s.formatCode("0")
	assert.True(t, ok)
	assert.Equal(t, "0%", code)
}
