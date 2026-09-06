package openxml

// okapi-filter: openxml

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SpreadsheetML rich text is CT_Rst: a sequence of <r> elements, each with its
// own <rPr> (ECMA-376 Part 1 §18.4.7). Five of an <rPr>'s children carry
// formatting the model names and become the codes a translator sees; the rest
// carry the colour, the size and the typeface, and travel as one opaque paired
// code holding their source bytes.
//
// Both halves matter. Without the codes a translator cannot see where the red
// stretch begins. Without the opaque half the colour is gone from the workbook
// and the runs merge into one, which is what #2478 reported.
//
// Okapi keeps the whole <rPr> as a Property on the run
// (okapi/filters/openxml/RunProperties.java); the WordprocessingML path in this
// package keeps it on a sidecar annotation (source_rpr.go). A shared string's
// runs are the block's entire content, so a code carries it here: the run
// boundary a colour creates is the inline structure, and an annotation cannot
// say where in the text it begins.

// richTextSST is the shape #2478 reported, taken from the upstream fixture
// 1363-cell-and-inline-styles.xlsx: one item of three runs whose middle run is
// red, and one plain item.
const richTextSST = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="2" uniqueCount="2">` +
	`<si><r><rPr><sz val="12"/><color rgb="FF000000"/><rFont val="Arial"/><family val="2"/></rPr>` +
	`<t xml:space="preserve">Normal and </t></r>` +
	`<r><rPr><b/><sz val="12"/><color rgb="FFFF0000"/><rFont val="Arial"/><family val="2"/></rPr>` +
	`<t>Red</t></r>` +
	`<r><rPr><sz val="12"/><color rgb="FF000000"/><rFont val="Arial"/><family val="2"/></rPr>` +
	`<t xml:space="preserve"> text</t></r></si>` +
	`<si><t>Plain</t></si>` +
	`</sst>`

func richTextWorkbook(t *testing.T) []byte {
	t.Helper()
	sheet := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
		`<sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row></sheetData>` +
		`</worksheet>`
	return buildFidelityPackage(t, []fidelityEntry{
		{"[Content_Types].xml", fidelityContentTypes},
		{"_rels/.rels", fidelityRootRels},
		{"xl/workbook.xml", fidelityWorkbook},
		{"xl/_rels/workbook.xml.rels", fidelityWorkbookRels},
		{"xl/worksheets/sheet1.xml", sheet},
		{"xl/sharedStrings.xml", richTextSST},
	})
}

// TestSMLRichText_RunPropertiesRoundTrip is the regression for #2478: the
// colour, the size and the typeface come back, and so does the run boundary
// the colour created.
func TestSMLRichText_RunPropertiesRoundTrip(t *testing.T) {
	pkg := richTextWorkbook(t)
	out := skeletonRoundtripBytes(t, pkg, "rich.xlsx")
	got := string(zipPartBytes(t, out, "xl/sharedStrings.xml"))

	assert.Contains(t, got, `<color rgb="FFFF0000"/>`, "the red run keeps its colour")
	assert.Contains(t, got, `<rFont val="Arial"/>`, "the runs keep their typeface")
	assert.Equal(t, 3, strings.Count(got, "<r>"),
		"the three source runs stay three runs rather than merging into one")
	assert.Equal(t, richTextSST, got,
		"an untranslated rich-text table must go back byte for byte")
}

// TestSMLRichText_TranslatedCellKeepsItsRuns checks the same properties survive
// a write that does replace the text.
func TestSMLRichText_TranslatedCellKeepsItsRuns(t *testing.T) {
	pkg := richTextWorkbook(t)
	out := richTextWriteBack(t, pkg)
	got := string(zipPartBytes(t, out, "xl/sharedStrings.xml"))

	assert.Contains(t, got, `<color rgb="FFFF0000"/>`, "the red run keeps its colour")
	assert.Contains(t, got, `<b/>`, "the red run keeps its bold")
	assert.Contains(t, got, "NORMAL AND", "the translation is written back")
	assert.Equal(t, 3, strings.Count(got, "<r>"), "the run structure survives the translation")
}

// TestSMLRichText_ModelRunsCarryNamedAndOpaqueCodes states what the block looks
// like: a declared code per named formatting type, and one opaque code holding
// the rest of the <rPr>.
func TestSMLRichText_ModelRunsCarryNamedAndOpaqueCodes(t *testing.T) {
	blocks := readSharedStringBlocks(t, richTextWorkbook(t))
	require.Len(t, blocks, 2)

	var opaque, bold int
	var opaqueData string
	for _, r := range blocks[0].Source {
		if r.PcOpen == nil {
			continue
		}
		switch r.PcOpen.Type {
		case TypeSMLRunProps:
			opaque++
			opaqueData = r.PcOpen.Attr(AttrSMLRPr)
		case TypeBold:
			bold++
		}
	}
	assert.Equal(t, 3, opaque, "one opaque code per run: the three runs carry different <rPr>")
	assert.Equal(t, 1, bold, "only the middle run is bold")
	assert.Contains(t, opaqueData, `<sz val="12"/>`, "the opaque code carries the source bytes")
	assert.NotContains(t, opaqueData, "<b/>", "the named formatting is not duplicated in the opaque code")
	for _, r := range blocks[0].Source {
		if r.PcOpen != nil {
			assert.Empty(t, r.PcOpen.Data,
				"the source bytes travel on AttrSMLRPr, not on Data, so no other format replays them")
		}
	}
	assert.Equal(t, "Normal and Red text", blocks[0].SourceText())
}

// TestSMLRichText_PairedCodesBalance checks the ids pair, which is what lets
// `kapi apply` accept an edit to a rich-text block (#2227).
func TestSMLRichText_PairedCodesBalance(t *testing.T) {
	blocks := readSharedStringBlocks(t, richTextWorkbook(t))
	require.NotEmpty(t, blocks)
	var open []string
	for _, r := range blocks[0].Source {
		switch {
		case r.PcOpen != nil:
			open = append(open, r.PcOpen.ID)
		case r.PcClose != nil:
			require.NotEmpty(t, open, "a close with nothing open")
			assert.Equal(t, open[len(open)-1], r.PcClose.ID, "a close matches the innermost open")
			open = open[:len(open)-1]
		}
	}
	assert.Empty(t, open, "every open code is closed")
}

// TestSMLRichText_UnderlineKeepsItsValue covers the named children whose source
// form carries more than the type: `<u val="double"/>` is not `<u/>`.
func TestSMLRichText_UnderlineKeepsItsValue(t *testing.T) {
	sst := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="1" uniqueCount="1">` +
		`<si><r><rPr><u val="double"/><color rgb="FF00FF00"/></rPr><t>Underlined</t></r></si>` +
		`</sst>`
	out := skeletonRoundtripBytes(t, richTextPackage(t, sst), "rich.xlsx")
	assert.Contains(t, string(zipPartBytes(t, out, "xl/sharedStrings.xml")), `<u val="double"/>`,
		"a named child goes back in the form it was read, not in a canonical one")
}

// TestSMLRichText_PropertiesDoNotLeakBetweenItems covers a shared string that
// holds a bare <t> and so opens no <r>: it must not inherit the run properties
// of the item before it. 1096.xlsx is the upstream case.
func TestSMLRichText_PropertiesDoNotLeakBetweenItems(t *testing.T) {
	sst := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="2" uniqueCount="2">` +
		`<si><r><rPr><b/><color rgb="FFFF0000"/></rPr><t>Bold red</t></r></si>` +
		`<si><t>Plain</t></si>` +
		`</sst>`
	out := skeletonRoundtripBytes(t, richTextPackage(t, sst), "rich.xlsx")
	got := string(zipPartBytes(t, out, "xl/sharedStrings.xml"))
	assert.Contains(t, got, `<si><t>Plain</t></si>`,
		"the plain item stays plain")
	assert.Equal(t, sst, got, "the table goes back byte for byte")
}

// TestSMLRichText_InlineStringRunProperties covers the other CT_Rst carrier: a
// cell that holds its text inline (ECMA-376 Part 1 §18.3.1.4), routed through
// the same reader and writer since #2480.
func TestSMLRichText_InlineStringRunProperties(t *testing.T) {
	sheet := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
		`<sheetData><row r="1"><c r="A1" t="inlineStr"><is>` +
		`<r><rPr><i/><sz val="11"/><color theme="1"/><rFont val="Calibri"/></rPr><t xml:space="preserve">Rich </t></r>` +
		`<r><rPr><sz val="11"/><color rgb="FF0070C0"/><rFont val="Calibri"/></rPr><t>inline</t></r>` +
		`</is></c></row></sheetData></worksheet>`
	pkg := buildFidelityPackage(t, []fidelityEntry{
		{"[Content_Types].xml", fidelityContentTypes},
		{"_rels/.rels", fidelityRootRels},
		{"xl/workbook.xml", fidelityWorkbook},
		{"xl/_rels/workbook.xml.rels", fidelityWorkbookRels},
		{"xl/worksheets/sheet1.xml", sheet},
		{"xl/sharedStrings.xml", fidelitySharedStrings},
	})

	out := skeletonRoundtripBytes(t, pkg, "inline.xlsx")
	assert.Equal(t, sheet, string(zipPartBytes(t, out, "xl/worksheets/sheet1.xml")),
		"an untranslated inline string must go back byte for byte")

	translated := richTextWriteBack(t, pkg)
	got := string(zipPartBytes(t, translated, "xl/worksheets/sheet1.xml"))
	assert.Contains(t, got, `<color rgb="FF0070C0"/>`, "the blue run keeps its colour")
	assert.Contains(t, got, "RICH", "the translation is written back")
}

// richTextWriteBack translates every block by upper-casing its text runs and
// leaving its codes where they are, which is what an editor produces: the codes
// travel with the target, so the writer knows where each run's <rPr> belongs.
func richTextWriteBack(t *testing.T, pkg []byte) []byte {
	t.Helper()
	return skeletonWriteBackRuns(t, pkg, model.LocaleID("qps"), func(src []model.Run) []model.Run {
		out := make([]model.Run, len(src))
		for i, r := range src {
			if r.Text != nil {
				out[i] = model.TextR(strings.ToUpper(r.Text.Text))
				continue
			}
			out[i] = r
		}
		return out
	})
}

// skeletonWriteBackRuns is skeletonWriteBack with a run-level translation, so a
// block's inline codes reach the writer the way an editor sends them back.
func skeletonWriteBackRuns(t *testing.T, original []byte, locale model.LocaleID,
	translate func([]model.Run) []model.Run) []byte {
	t.Helper()

	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := NewReader()
	reader.SetSkeletonStore(skelStore)
	require.NoError(t, reader.Open(t.Context(), &model.RawDocument{
		URI:          "rich.xlsx",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(original),
	}))
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	require.NoError(t, reader.Close())

	for _, b := range testutil.FilterBlocks(parts) {
		if b.Translatable {
			b.SetTargetRuns(locale, translate(b.Source))
		}
	}

	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	writer.SetSkeletonStore(skelStore)
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(t.Context(), testutil.PartsToChannel(parts)))
	require.NoError(t, writer.Close())
	return buf.Bytes()
}

// richTextPackage builds a workbook around one shared-string table.
func richTextPackage(t *testing.T, sst string) []byte {
	t.Helper()
	sheet := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
		`<sheetData><row r="1"><c r="A1" t="s"><v>0</v></c></row></sheetData></worksheet>`
	return buildFidelityPackage(t, []fidelityEntry{
		{"[Content_Types].xml", fidelityContentTypes},
		{"_rels/.rels", fidelityRootRels},
		{"xl/workbook.xml", fidelityWorkbook},
		{"xl/_rels/workbook.xml.rels", fidelityWorkbookRels},
		{"xl/worksheets/sheet1.xml", sheet},
		{"xl/sharedStrings.xml", sst},
	})
}

// readSharedStringBlocks reads a workbook and returns the blocks its
// shared-string table produced, in order.
func readSharedStringBlocks(t *testing.T, pkg []byte) []*model.Block {
	t.Helper()
	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := NewReader()
	reader.SetSkeletonStore(skelStore)
	require.NoError(t, reader.Open(t.Context(), &model.RawDocument{
		URI:          "rich.xlsx",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(pkg),
	}))
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	require.NoError(t, reader.Close())

	var out []*model.Block
	for _, b := range testutil.FilterBlocks(parts) {
		if b.Properties["partPath"] == "xl/sharedStrings.xml" {
			out = append(out, b)
		}
	}
	return out
}
