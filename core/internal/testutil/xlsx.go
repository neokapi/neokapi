package testutil

import (
	"archive/zip"
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// XLSX describes a minimal workbook a test can synthesize: worksheets given as
// their sheetData rows, a stylesheet of number formats, the shared-string
// table, and the date system. The package it builds carries the parts and
// relationships a reader resolves, and nothing a spreadsheet application
// would add for its own use.
type XLSX struct {
	// Sheets are the worksheets in workbook order.
	Sheets []XLSXSheet
	// NumFmts declares custom number formats by id (custom ids start at 164;
	// a built-in id may be redefined).
	NumFmts map[int]string
	// CellXfs lists the cell styles by index; each entry is the numFmtId the
	// style displays through. A cell's `s` attribute indexes this list. A
	// workbook with neither styles nor formats gets no stylesheet part.
	CellXfs []int
	// SharedStrings is the shared-string table, by index.
	SharedStrings []string
	// Date1904 selects the 1904 date system.
	Date1904 bool
}

// XLSXSheet is one worksheet: its tab name, the <row> elements placed inside
// <sheetData>, and any elements that follow sheetData (mergeCells, …).
type XLSXSheet struct {
	Name      string
	SheetData string
	After     string
}

// BuildXLSX writes the workbook as a zip package.
func BuildXLSX(t testing.TB, spec XLSX) []byte {
	t.Helper()
	const (
		nsMain = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
		nsRel  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
		nsPkg  = "http://schemas.openxmlformats.org/package/2006/relationships"
		ctBase = "application/vnd.openxmlformats-officedocument.spreadsheetml"
	)
	hasStyles := len(spec.CellXfs) > 0 || len(spec.NumFmts) > 0
	hasStrings := len(spec.SharedStrings) > 0

	parts := map[string]string{}

	var ct strings.Builder
	ct.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	ct.WriteString(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	ct.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	ct.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	ct.WriteString(`<Override PartName="/xl/workbook.xml" ContentType="` + ctBase + `.sheet.main+xml"/>`)
	for i := range spec.Sheets {
		fmt.Fprintf(&ct, `<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="%s.worksheet+xml"/>`, i+1, ctBase)
	}
	if hasStyles {
		ct.WriteString(`<Override PartName="/xl/styles.xml" ContentType="` + ctBase + `.styles+xml"/>`)
	}
	if hasStrings {
		ct.WriteString(`<Override PartName="/xl/sharedStrings.xml" ContentType="` + ctBase + `.sharedStrings+xml"/>`)
	}
	ct.WriteString(`</Types>`)
	parts["[Content_Types].xml"] = ct.String()

	parts["_rels/.rels"] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<Relationships xmlns="` + nsPkg + `">` +
		`<Relationship Id="rId1" Type="` + nsRel + `/officeDocument" Target="xl/workbook.xml"/>` +
		`</Relationships>`

	var wb strings.Builder
	wb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	wb.WriteString(`<workbook xmlns="` + nsMain + `" xmlns:r="` + nsRel + `">`)
	if spec.Date1904 {
		wb.WriteString(`<workbookPr date1904="1"/>`)
	} else {
		wb.WriteString(`<workbookPr/>`)
	}
	wb.WriteString(`<sheets>`)
	for i, sh := range spec.Sheets {
		fmt.Fprintf(&wb, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, xmlAttr(sh.Name), i+1, i+1)
	}
	wb.WriteString(`</sheets></workbook>`)
	parts["xl/workbook.xml"] = wb.String()

	var rels strings.Builder
	rels.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	rels.WriteString(`<Relationships xmlns="` + nsPkg + `">`)
	next := 1
	for i := range spec.Sheets {
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="%s/worksheet" Target="worksheets/sheet%d.xml"/>`, next, nsRel, i+1)
		next++
	}
	if hasStyles {
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="%s/styles" Target="styles.xml"/>`, next, nsRel)
		next++
	}
	if hasStrings {
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="%s/sharedStrings" Target="sharedStrings.xml"/>`, next, nsRel)
	}
	rels.WriteString(`</Relationships>`)
	parts["xl/_rels/workbook.xml.rels"] = rels.String()

	for i, sh := range spec.Sheets {
		parts[fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1)] =
			`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
				`<worksheet xmlns="` + nsMain + `" xmlns:r="` + nsRel + `">` +
				`<sheetData>` + sh.SheetData + `</sheetData>` + sh.After + `</worksheet>`
	}

	if hasStyles {
		var st strings.Builder
		st.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
		st.WriteString(`<styleSheet xmlns="` + nsMain + `">`)
		if len(spec.NumFmts) > 0 {
			ids := make([]int, 0, len(spec.NumFmts))
			for id := range spec.NumFmts {
				ids = append(ids, id)
			}
			sort.Ints(ids)
			fmt.Fprintf(&st, `<numFmts count="%d">`, len(ids))
			for _, id := range ids {
				fmt.Fprintf(&st, `<numFmt numFmtId="%d" formatCode="%s"/>`, id, xmlAttr(spec.NumFmts[id]))
			}
			st.WriteString(`</numFmts>`)
		}
		st.WriteString(`<fonts count="1"><font><sz val="11"/><name val="Calibri"/></font></fonts>`)
		st.WriteString(`<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>`)
		st.WriteString(`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>`)
		st.WriteString(`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>`)
		xfs := spec.CellXfs
		if len(xfs) == 0 {
			xfs = []int{0}
		}
		fmt.Fprintf(&st, `<cellXfs count="%d">`, len(xfs))
		for _, id := range xfs {
			apply := ""
			if id != 0 {
				apply = ` applyNumberFormat="1"`
			}
			fmt.Fprintf(&st, `<xf numFmtId="%d" fontId="0" fillId="0" borderId="0" xfId="0"%s/>`, id, apply)
		}
		st.WriteString(`</cellXfs>`)
		st.WriteString(`<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>`)
		st.WriteString(`</styleSheet>`)
		parts["xl/styles.xml"] = st.String()
	}

	if hasStrings {
		var sst strings.Builder
		sst.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
		fmt.Fprintf(&sst, `<sst xmlns="%s" count="%d" uniqueCount="%d">`, nsMain, len(spec.SharedStrings), len(spec.SharedStrings))
		for _, s := range spec.SharedStrings {
			sst.WriteString(`<si><t>` + xmlText(s) + `</t></si>`)
		}
		sst.WriteString(`</sst>`)
		parts["xl/sharedStrings.xml"] = sst.String()
	}

	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	sort.Strings(names)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(parts[name]))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func xmlAttr(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

func xmlText(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

// XLSXNumberFormats is the workbook the number-format tests share: a header
// row of shared strings over value cells styled with built-in and custom
// formats, booleans, a formula's string result, an inline string, and a cell
// whose style index the stylesheet does not carry. Style indices: 1 is the
// built-in mm-dd-yy, 2 a custom 0.0%, 3 a custom currency with negative and
// zero sections, 4 the built-in #,##0, 5 a custom yyyy-mm-dd, 6 the built-in
// text format.
func XLSXNumberFormats(t testing.TB, date1904 bool) []byte {
	t.Helper()
	return BuildXLSX(t, XLSX{
		SharedStrings: []string{"Date", "Share", "Price", "Count", "Flag", "Note"},
		NumFmts: map[int]string{
			164: "0.0%",
			165: `"$"#,##0.00;("$"#,##0.00);"-"`,
			166: "yyyy-mm-dd",
		},
		CellXfs:  []int{0, 14, 164, 165, 3, 166, 49},
		Date1904: date1904,
		Sheets: []XLSXSheet{{
			Name: "Data",
			SheetData: `<row r="1">` +
				`<c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="s"><v>2</v></c>` +
				`<c r="D1" t="s"><v>3</v></c><c r="E1" t="s"><v>4</v></c><c r="F1" t="s"><v>5</v></c></row>` +
				`<row r="2">` +
				`<c r="A2" s="1"><v>44197</v></c><c r="B2" s="2"><v>0.125</v></c><c r="C2" s="3"><v>1234.5</v></c>` +
				`<c r="D2" s="4"><v>1234567</v></c><c r="E2" t="b"><v>1</v></c><c r="F2" s="6" t="str"><f>A1</f><v>Date</v></c></row>` +
				`<row r="3">` +
				`<c r="A3" s="5"><v>60</v></c><c r="B3" s="2"><v>-0.5</v></c><c r="C3" s="3"><v>-1234.5</v></c>` +
				`<c r="D3" s="4"><v>0</v></c><c r="E3" t="b"><v>0</v></c><c r="F3" t="inlineStr"><is><t>hello</t></is></c></row>` +
				`<row r="4">` +
				`<c r="A4"><v>1992</v></c><c r="B4" s="2"><v>0</v></c><c r="C4" s="3"><v>0</v></c><c r="D4" s="7"><v>5</v></c></row>`,
		}},
	})
}
