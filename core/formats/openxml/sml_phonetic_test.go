package openxml

// okapi-filter: openxml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A CT_Rst element can carry a phonetic guide beside its text: `<rPh>` holds
// the reading of a stretch of the base text (furigana over kanji), and
// `<phoneticPr>` names the font and the conversion mode the guide displays
// under. Both hold their own `<t>`.
//
// That `<t>` reads the string; it is no part of it. A reader that walks every
// `<t>` inside a CT_Rst hands the translator the base text with the reading
// glued to its end, and writes that back as the cell's whole value, which is
// what #2518 reported for japanese_phonetic_run_property.xlsx.
//
// Upstream Okapi keeps the two elements out of the extracted string the same
// way, by listing them as skippable
// (SkippableElement.PhoneticInline in
// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/openxml/SkippableElement.java,
// consumed by StringItemParser). Okapi discards them; the writer here replays
// their source bytes, which is what keeps an untranslated workbook byte for
// byte.

// phoneticSST holds the two shapes a phonetic guide comes in: over a bare
// `<t>`, and over a sequence of rich-text runs.
const phoneticSST = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="2" uniqueCount="2">` +
	`<si><t>東京</t><rPh sb="0" eb="2"><t>トウキョウ</t></rPh><phoneticPr fontId="1"/></si>` +
	`<si><r><rPr><b/><color rgb="FFFF0000"/></rPr><t>大阪</t></r>` +
	`<r><t>行き</t></r>` +
	`<rPh sb="0" eb="2"><t>オオサカ</t></rPh><phoneticPr fontId="1" type="noConversion"/></si>` +
	`</sst>`

func phoneticWorkbook(t *testing.T) []byte {
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
		{"xl/sharedStrings.xml", phoneticSST},
	})
}

// TestSMLPhonetic_ExtractedTextIsTheBaseText is the content half of #2518: a
// translator is handed the string, not the string plus its reading.
func TestSMLPhonetic_ExtractedTextIsTheBaseText(t *testing.T) {
	blocks := readSharedStringBlocks(t, phoneticWorkbook(t))
	require.Len(t, blocks, 2)

	assert.Equal(t, "東京", blocks[0].SourceText(),
		"the phonetic guide reads the base text and stays out of it")
	assert.Equal(t, "大阪行き", blocks[1].SourceText(),
		"a guide over rich-text runs stays out of them too")
	for _, b := range blocks {
		assert.NotContains(t, b.SourceText(), "トウキョウ")
		assert.NotContains(t, b.SourceText(), "オオサカ")
	}
}

// TestSMLPhonetic_UntranslatedRoundTripIsByteIdentical is the fidelity half:
// the guide goes back where it was, in the bytes it was written in.
func TestSMLPhonetic_UntranslatedRoundTripIsByteIdentical(t *testing.T) {
	out := skeletonRoundtripBytes(t, phoneticWorkbook(t), "phonetic.xlsx")
	assert.Equal(t, phoneticSST, string(zipPartBytes(t, out, "xl/sharedStrings.xml")),
		"an untranslated table holding phonetic runs must go back byte for byte")
}

// TestSMLPhonetic_TranslatedItemKeepsItsPhoneticRuns covers the write that does
// replace the text. The guide stays attached to the item it was authored on.
func TestSMLPhonetic_TranslatedItemKeepsItsPhoneticRuns(t *testing.T) {
	out := richTextWriteBack(t, phoneticWorkbook(t))
	got := string(zipPartBytes(t, out, "xl/sharedStrings.xml"))

	assert.Contains(t, got, `<rPh sb="0" eb="2"><t>トウキョウ</t></rPh><phoneticPr fontId="1"/></si>`,
		"the guide over a bare <t> keeps its bytes and its place at the end of the item")
	assert.Contains(t, got, `<rPh sb="0" eb="2"><t>オオサカ</t></rPh><phoneticPr fontId="1" type="noConversion"/></si>`,
		"the guide over rich-text runs follows the last run")
	assert.Contains(t, got, `<color rgb="FFFF0000"/>`, "the run properties still survive")
	assert.Equal(t, 2, strings.Count(got, "<rPh "), "one guide per item, and no more")
}

// TestSMLPhonetic_ItemWithOnlyAGuideIsReplayed covers a CT_Rst holding phonetic
// markup and no base text: nothing is translatable, so the item goes back as
// the source wrote it.
func TestSMLPhonetic_ItemWithOnlyAGuideIsReplayed(t *testing.T) {
	sst := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="1" uniqueCount="1">` +
		`<si><rPh sb="0" eb="2"><t>トウキョウ</t></rPh><phoneticPr fontId="1"/></si>` +
		`</sst>`
	out := skeletonRoundtripBytes(t, richTextPackage(t, sst), "phonetic.xlsx")
	assert.Equal(t, sst, string(zipPartBytes(t, out, "xl/sharedStrings.xml")))

	blocks := readSharedStringBlocks(t, richTextPackage(t, sst))
	assert.Empty(t, blocks, "a guide on its own is not a translatable string")
}

// TestSMLPhonetic_InlineStringKeepsItsGuide covers the other CT_Rst carrier: a
// cell that holds its text inline (ECMA-376 Part 1 §18.3.1.4).
func TestSMLPhonetic_InlineStringKeepsItsGuide(t *testing.T) {
	sheet := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
		`<sheetData><row r="1"><c r="A1" t="inlineStr"><is>` +
		`<t>東京</t><rPh sb="0" eb="2"><t>トウキョウ</t></rPh><phoneticPr fontId="1"/>` +
		`</is></c></row></sheetData></worksheet>`
	pkg := buildFidelityPackage(t, []fidelityEntry{
		{"[Content_Types].xml", fidelityContentTypes},
		{"_rels/.rels", fidelityRootRels},
		{"xl/workbook.xml", fidelityWorkbook},
		{"xl/_rels/workbook.xml.rels", fidelityWorkbookRels},
		{"xl/worksheets/sheet1.xml", sheet},
		{"xl/sharedStrings.xml", fidelitySharedStrings},
	})

	assert.Equal(t, sheet, string(zipPartBytes(t, skeletonRoundtripBytes(t, pkg, "inline.xlsx"), "xl/worksheets/sheet1.xml")),
		"an untranslated inline string holding a guide must go back byte for byte")

	got := string(zipPartBytes(t, richTextWriteBack(t, pkg), "xl/worksheets/sheet1.xml"))
	assert.Contains(t, got, `<rPh sb="0" eb="2"><t>トウキョウ</t></rPh><phoneticPr fontId="1"/></is>`,
		"a translated inline cell keeps its guide inside the <is> wrapper")
	assert.Contains(t, got, `<is><t>東京</t>`, "the base text is written back ahead of the guide")
}

// TestSMLPhonetic_CellAnchorShowsTheBaseText covers the second reader of the
// shared-string table: the pre-parsed table a worksheet cell's grid anchor
// displays. It concatenated every <t> in the item, guide included.
func TestSMLPhonetic_CellAnchorShowsTheBaseText(t *testing.T) {
	parts := readPhoneticParts(t, phoneticWorkbook(t))
	var anchors []string
	for _, b := range testutil.FilterBlocks(parts) {
		if b.Type == "cell" && !b.Translatable {
			anchors = append(anchors, b.SourceText())
		}
	}
	require.Len(t, anchors, 2, "each shared-string cell is anchored in the grid")
	assert.Equal(t, []string{"東京", "大阪行き"}, anchors,
		"a grid anchor shows the cell's text, not the text plus its reading")
}

// TestSMLPhonetic_UpstreamFixture is #2518 on the file it was reported against.
func TestSMLPhonetic_UpstreamFixture(t *testing.T) {
	path := filepath.Join(testdataDir(t), "japanese_phonetic_run_property.xlsx")
	original, err := os.ReadFile(path)
	require.NoError(t, err)

	blocks := readSharedStringBlocks(t, original)
	require.Len(t, blocks, 1)
	assert.Equal(t, "test", blocks[0].SourceText(),
		"the item's base text is `test`; `ad` is the reading of its second and third characters")

	out := skeletonRoundtripBytes(t, original, filepath.Base(path))
	assert.Equal(t,
		string(zipPartBytes(t, original, "xl/sharedStrings.xml")),
		string(zipPartBytes(t, out, "xl/sharedStrings.xml")),
		"the untranslated table goes back byte for byte")

	translated := richTextWriteBack(t, original)
	got := string(zipPartBytes(t, translated, "xl/sharedStrings.xml"))
	assert.Contains(t, got, `<t>TEST</t><rPh sb="1" eb="3"><t>ad</t></rPh><phoneticPr fontId="1"/>`,
		"the translation is written back and the guide stays behind it")
}

// readPhoneticParts reads a workbook and returns every part it emitted, so a
// test can reach the non-translatable grid anchors as well as the blocks.
func readPhoneticParts(t *testing.T, pkg []byte) []*model.Part {
	t.Helper()
	reader := NewReader()
	require.NoError(t, reader.Open(t.Context(), &model.RawDocument{
		URI:          "phonetic.xlsx",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(pkg),
	}))
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	require.NoError(t, reader.Close())
	return parts
}
