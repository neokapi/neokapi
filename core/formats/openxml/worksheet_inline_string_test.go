package openxml

// okapi-filter: openxml

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A worksheet cell may hold its text in three places: the shared-string table
// (t="s", the cell keeps an index in <v>), a value element (<v>, for numbers,
// booleans and cached formula results), or the cell itself (t="inlineStr", the
// text in an <is> element per ECMA-376 Part 1 §18.3.1.4). Each goes back the
// way it was read; writing an inline string as a value element leaves a
// workbook a spreadsheet application repairs on open.
//
// Okapi's filter takes the other road: OpenXMLFilter rewrites every inline
// string as a shared string (Cell.readWith swaps the t attribute for "s" and
// moves the text into sharedStrings.xml). That is valid, and it loses the
// author's choice of storage on every round trip, so neokapi keeps the cell as
// it found it.

// inlineStringWorkbook is the fixture these tests share: a header row of shared
// strings, a value row, and a row of inline strings: plain, rich, and one
// whose leading and trailing spaces need xml:space="preserve".
func inlineStringWorkbook(t testing.TB) []byte {
	t.Helper()
	return testutil.BuildXLSX(t, testutil.XLSX{
		SharedStrings: []string{"Name", "Note"},
		CellXfs:       []int{0, 14},
		Sheets: []testutil.XLSXSheet{{
			Name: "Data",
			SheetData: `<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row>` +
				`<row r="2"><c r="A2" s="1"><v>44197</v></c><c r="B2"><v>-12.5</v></c><c r="C2" t="b"><v>1</v></c>` +
				`<c r="D2" t="str"><f>A1</f><v>Name</v></c></row>` +
				`<row r="3"><c r="A3" t="inlineStr"><is><t>plain inline</t></is></c>` +
				`<c r="B3" t="inlineStr"><is><r><rPr><b/></rPr><t>bold</t></r><r><t xml:space="preserve"> and plain</t></r></is></c>` +
				`<c r="C3" t="inlineStr"><is><t xml:space="preserve"> padded </t></is></c>` +
				`<c r="D3" s="1"></c></row>`,
		}},
	})
}

// skeletonWriteBack reads a workbook and writes it back through the skeleton,
// translating every translatable block with translate when it is non-nil.
func skeletonWriteBack(t *testing.T, original []byte, locale model.LocaleID, translate func(*model.Block) string) []byte {
	t.Helper()

	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := NewReader()
	reader.SetSkeletonStore(skelStore)
	require.NoError(t, reader.Open(t.Context(), &model.RawDocument{
		URI:          "inline.xlsx",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(original),
	}))
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	require.NoError(t, reader.Close())

	if translate != nil {
		for _, b := range testutil.FilterBlocks(parts) {
			if b.Translatable {
				b.SetTargetText(locale, translate(b))
			}
		}
	}

	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	writer.SetSkeletonStore(skelStore)
	if locale != "" {
		writer.SetLocale(locale)
	}
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(t.Context(), testutil.PartsToChannel(parts)))
	require.NoError(t, writer.Close())
	return buf.Bytes()
}

// The skeleton carries every cell's markup, so an untranslated workbook comes
// back byte for byte, inline strings included.
func TestInlineStringCellsRoundTripByteForByte(t *testing.T) {
	original := inlineStringWorkbook(t)
	out := skeletonWriteBack(t, original, "", nil)

	for _, name := range []string{"xl/worksheets/sheet1.xml", "xl/sharedStrings.xml"} {
		assert.Equal(t, string(zipPart(t, original, name)), string(zipPart(t, out, name)),
			"%s round-trips byte for byte", name)
	}
}

// The reader reads an inline string's CT_Rst runs the way it reads a shared
// string's: run properties become inline codes, so a bold stretch survives as
// a code pair rather than being flattened into the text.
func TestInlineStringCellKeepsItsRichRuns(t *testing.T) {
	cells := sheetCells(readXLSXBytes(t, inlineStringWorkbook(t)))

	require.NotNil(t, cells["B3"])
	assert.Equal(t, "bold and plain", model.RunsText(cells["B3"].Source))
	assert.True(t, model.RunsHaveInlineCodes(cells["B3"].Source),
		"the bold run survives as an inline code")

	require.NotNil(t, cells["A3"])
	assert.Equal(t, "plain inline", model.RunsText(cells["A3"].Source))
	assert.False(t, model.RunsHaveInlineCodes(cells["A3"].Source))

	require.NotNil(t, cells["C3"])
	assert.Equal(t, " padded ", model.RunsText(cells["C3"].Source),
		"xml:space=preserve keeps the padding")
}

// A translated inline-string cell stays an inline string: the target text goes
// into <is>, the cell keeps its t="inlineStr", and no value element appears.
func TestTranslatedInlineStringCellStaysAnInlineString(t *testing.T) {
	nb := model.LocaleID("nb-NO")
	out := skeletonWriteBack(t, inlineStringWorkbook(t), nb, func(b *model.Block) string {
		return "NB: " + b.SourceText()
	})

	sheet := string(zipPart(t, out, "xl/worksheets/sheet1.xml"))
	assert.Contains(t, sheet, `<c r="A3" t="inlineStr"><is><t>NB: plain inline</t></is></c>`)
	assert.Contains(t, sheet, `<c r="C3" t="inlineStr"><is><t xml:space="preserve">NB:  padded </t></is></c>`,
		"trailing space keeps its xml:space attribute")

	assertInlineStringCellsWellFormed(t, out)
}

// A cell that carries no value element gets none written for it. Excel writes
// styled blank cells as `<c r="A1" s="2"/>`, and a value element invented for
// one leaves a workbook whose blank cells hold an empty string.
func TestCellWithoutAValueGetsNoValueElement(t *testing.T) {
	original := testutil.BuildXLSX(t, testutil.XLSX{
		SharedStrings: []string{"Name"},
		CellXfs:       []int{0, 14},
		Sheets: []testutil.XLSXSheet{{
			Name: "Data",
			SheetData: `<row r="1"><c r="A1" t="s"></c><c r="B1" t="inlineStr"></c>` +
				`<c r="C1" s="1"></c><c r="D1" t="s"><v>0</v></c></row>`,
		}},
	})

	sheet := string(zipPart(t, skeletonWriteBack(t, original, "", nil), "xl/worksheets/sheet1.xml"))
	assert.Contains(t, sheet, `<c r="A1" t="s"></c>`)
	assert.Contains(t, sheet, `<c r="B1" t="inlineStr"></c>`)
	assert.Contains(t, sheet, `<c r="C1" s="1"></c>`)
	assert.Contains(t, sheet, `<c r="D1" t="s"><v>0</v></c>`, "a cell that had a value keeps it")
}

// A shared string whose text starts or ends with whitespace is written with
// xml:space="preserve", so the padding survives the round trip.
func TestSharedStringKeepsEdgeWhitespace(t *testing.T) {
	original := testutil.BuildXLSX(t, testutil.XLSX{
		SharedStrings: []string{" leading", "trailing ", "no padding"},
		Sheets: []testutil.XLSXSheet{{
			Name:      "Data",
			SheetData: `<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="s"><v>2</v></c></row>`,
		}},
	})

	out := skeletonWriteBack(t, original, "", nil)
	sst := string(zipPart(t, out, "xl/sharedStrings.xml"))
	assert.Contains(t, sst, `<si><t xml:space="preserve"> leading</t></si>`)
	assert.Contains(t, sst, `<si><t xml:space="preserve">trailing </t></si>`)
	assert.Contains(t, sst, `<si><t>no padding</t></si>`)

	blocks := readXLSXBytes(t, out)
	var texts []string
	for _, b := range blocks {
		if b.Type == "shared-string" {
			texts = append(texts, model.RunsText(b.Source))
		}
	}
	assert.Equal(t, []string{" leading", "trailing ", "no padding"}, texts)
}

// assertInlineStringCellsWellFormed checks every worksheet cell in a written
// package against ECMA-376 Part 1 §18.3.1.4: a t="inlineStr" cell carries its
// text in an <is> element holding at least one <t>, and no <v>.
func assertInlineStringCellsWellFormed(t *testing.T, pkg []byte) {
	t.Helper()

	for _, part := range xmlPartNames(t, pkg) {
		if !strings.HasPrefix(part, "xl/worksheets/") {
			continue
		}
		d := xml.NewDecoder(bytes.NewReader(zipPart(t, pkg, part)))
		for {
			tok, err := d.Token()
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)
			start, ok := tok.(xml.StartElement)
			if !ok || start.Name.Local != "c" || attrVal(start, "t") != "inlineStr" {
				continue
			}
			ref := attrVal(start, "r")
			raw, err := captureRawElement(d, start)
			require.NoError(t, err)
			if !strings.Contains(raw, "<is>") {
				continue // an empty inline-string cell holds nothing at all
			}
			assert.NotContains(t, raw, "<v>", "cell %s in %s writes a value element", ref, part)
			assert.Regexp(t, `<is>.*<t[ >].*</is>`, raw, "cell %s in %s", ref, part)
		}
	}
}

// xmlPartNames lists the XML entries of a package.
func xmlPartNames(t *testing.T, pkg []byte) []string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(pkg), int64(len(pkg)))
	require.NoError(t, err)
	var names []string
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".xml") {
			names = append(names, f.Name)
		}
	}
	return names
}
