package openxml

// okapi-filter: openxml

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A CT_Rst element says the same thing in several spellings, and the writer
// rebuilt it in one of them. Excel writes xml:space="preserve" on text that has
// no whitespace to preserve; a producer writes `&#8217;` where the decoder
// reports the character; a carriage return inside <t> is normalised on the way
// in (XML 1.0 §2.11); a workbook saved with indentation writes newlines between
// <si>, <r>, <rPr> and <t>. None of it is recoverable from the model, and every
// one of them left a byte difference on a shared-string table nobody had
// touched.
//
// The reader keeps the source content for a block the writer would spell
// differently, and the writer replays it for content nothing has changed. A
// translated cell is rebuilt, because its runs are not the source's runs and
// the layout between them has nowhere to go.

func sourceFormSST(items string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="1" uniqueCount="1">` +
		items + `</sst>`
}

// assertSSTRoundTripsWhole is the statement each form makes: untranslated, the
// table goes back as it was written.
func assertSSTRoundTripsWhole(t *testing.T, sst string) {
	t.Helper()
	out := skeletonRoundtripBytes(t, richTextPackage(t, sst), "form.xlsx")
	assert.Equal(t, sst, string(zipPartBytes(t, out, "xl/sharedStrings.xml")))
}

// TestSMLSourceForm_XMLSpaceOnTextThatDoesNotNeedIt covers the attribute Excel
// writes more liberally than the writer would.
func TestSMLSourceForm_XMLSpaceOnTextThatDoesNotNeedIt(t *testing.T) {
	assertSSTRoundTripsWhole(t, sourceFormSST(`<si><t xml:space="preserve">Plain</t></si>`))
}

// TestSMLSourceForm_CharacterReferenceSpelling covers a reference the decoder
// reports as its character: same text, different bytes.
func TestSMLSourceForm_CharacterReferenceSpelling(t *testing.T) {
	assertSSTRoundTripsWhole(t, sourceFormSST(`<si><t>Don&#8217;t</t></si>`))

	blocks := readSharedStringBlocks(t, richTextPackage(t, sourceFormSST(`<si><t>Don&#8217;t</t></si>`)))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Don’t", blocks[0].SourceText(), "the reader still reads the character")
}

// TestSMLSourceForm_CarriageReturnInsideText covers XML 1.0 §2.11: the decoder
// normalises CRLF, so the writer emits a line feed where the source had both.
func TestSMLSourceForm_CarriageReturnInsideText(t *testing.T) {
	assertSSTRoundTripsWhole(t,
		sourceFormSST(`<si><t xml:space="preserve">first&#13;&#10;second</t></si>`))
}

// TestSMLSourceForm_PrettyPrintedItemKeepsItsIndentation covers the whitespace
// between a rich item's elements, which belongs to none of them.
func TestSMLSourceForm_PrettyPrintedItemKeepsItsIndentation(t *testing.T) {
	assertSSTRoundTripsWhole(t, sourceFormSST("<si>\r\n  <r>\r\n    <rPr>\r\n      <b/>\r\n"+
		"      <sz val=\"11\"/>\r\n    </rPr>\r\n    <t>Bold</t>\r\n  </r>\r\n</si>"))
}

// TestSMLSourceForm_InlineStringKeepsItsLayout covers the other carrier, and
// the indentation of the cell around it.
func TestSMLSourceForm_InlineStringKeepsItsLayout(t *testing.T) {
	sheet := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` + "\r\n" +
		"  <sheetData>\r\n    <row r=\"1\">\r\n      <c r=\"A1\" t=\"inlineStr\">\r\n" +
		"        <is>\r\n          <t>Inline</t>\r\n        </is>\r\n      </c>\r\n" +
		"    </row>\r\n  </sheetData>\r\n</worksheet>"
	pkg := buildFidelityPackage(t, []fidelityEntry{
		{"[Content_Types].xml", fidelityContentTypes},
		{"_rels/.rels", fidelityRootRels},
		{"xl/workbook.xml", fidelityWorkbook},
		{"xl/_rels/workbook.xml.rels", fidelityWorkbookRels},
		{"xl/worksheets/sheet1.xml", sheet},
		{"xl/sharedStrings.xml", fidelitySharedStrings},
	})
	out := skeletonRoundtripBytes(t, pkg, "form.xlsx")
	assert.Equal(t, sheet, string(zipPartBytes(t, out, "xl/worksheets/sheet1.xml")),
		"an untranslated inline string keeps the layout of its cell")
}

// TestSMLSourceForm_TargetEqualToSourceStillReplays covers the ordinary write:
// kapi sets a target on every block, and a target saying what the source said
// leaves the bytes untranslated.
func TestSMLSourceForm_TargetEqualToSourceStillReplays(t *testing.T) {
	sst := sourceFormSST(`<si><t xml:space="preserve">Plain</t></si>`)
	out := skeletonWriteBackRuns(t, richTextPackage(t, sst), model.LocaleID("qps"),
		func(src []model.Run) []model.Run { return src })
	assert.Equal(t, sst, string(zipPartBytes(t, out, "xl/sharedStrings.xml")))
}

// TestSMLSourceForm_TranslatedItemIsRebuilt is the other half of the rule: a
// target that says something else gets the writer's own spelling, because the
// source's layout describes runs that are no longer there.
func TestSMLSourceForm_TranslatedItemIsRebuilt(t *testing.T) {
	sst := sourceFormSST("<si>\r\n  <t xml:space=\"preserve\">Plain</t>\r\n</si>")
	got := string(zipPartBytes(t, richTextWriteBack(t, richTextPackage(t, sst)), "xl/sharedStrings.xml"))
	assert.Contains(t, got, `<si><t>PLAIN</t></si>`, "the translation is written back")
	assert.NotContains(t, got, "\r\n  <t", "the source's indentation is not replayed around new text")
}

// TestSMLSourceForm_TextNeedingSpacePreserveIsRepaired names the one form the
// source keeps and the writer overrides. A <t> whose text has edge whitespace
// and no xml:space="preserve" is asking for that whitespace to be dropped
// (ECMA-376 Part 1 §18.4.12), so the writer adds the attribute rather than
// replaying the source's spelling.
func TestSMLSourceForm_TextNeedingSpacePreserveIsRepaired(t *testing.T) {
	sst := sourceFormSST(`<si><t>trailing </t></si>`)
	out := skeletonRoundtripBytes(t, richTextPackage(t, sst), "form.xlsx")
	assert.Contains(t, string(zipPartBytes(t, out, "xl/sharedStrings.xml")),
		`<si><t xml:space="preserve">trailing </t></si>`)
}

// TestSMLRunProps_OffStateChildSurvives covers an <rPr> child that names a
// formatting type and turns it off. No declared code carries it, so it belongs
// with the rest of the element. 948-3.xlsx writes a whole run this way.
func TestSMLRunProps_OffStateChildSurvives(t *testing.T) {
	sst := sourceFormSST(`<si><r><rPr><b val="false"/><i val="false"/>` +
		`<vertAlign val="baseline"/><sz val="11"/></rPr><t>Plain</t></r></si>`)
	assertSSTRoundTripsWhole(t, sst)

	got := string(zipPartBytes(t, richTextWriteBack(t, richTextPackage(t, sst)), "xl/sharedStrings.xml"))
	assert.Contains(t, got, `<b val="false"/>`, "a translated run keeps its explicit off state")
	assert.Contains(t, got, `<vertAlign val="baseline"/>`)
}

// TestSMLRunProps_UnderlineNoneIsNotAnUnderline covers ST_UnderlineValues
// (ECMA-376 Part 1 §18.18.86): `none` is the absence of an underline, so the
// run gets no underline code and its child travels with the rest of the <rPr>.
func TestSMLRunProps_UnderlineNoneIsNotAnUnderline(t *testing.T) {
	sst := sourceFormSST(`<si><r><rPr><u val="none"/><sz val="11"/></rPr><t>Plain</t></r></si>`)
	blocks := readSharedStringBlocks(t, richTextPackage(t, sst))
	require.Len(t, blocks, 1)
	for _, r := range blocks[0].Source {
		if r.PcOpen != nil {
			assert.NotEqual(t, TypeUnderline, r.PcOpen.Type,
				"a run that says it is not underlined shows a translator no underline")
		}
	}
	assertSSTRoundTripsWhole(t, sst)
}

// xlsxSourceFormExclusions names the fixtures whose untranslated round trip is
// deliberately not byte-identical, and why.
var xlsxSourceFormExclusions = map[string]string{
	"948-3.xlsx": "a <t> holding `unformatted ` declares no xml:space=\"preserve\", " +
		"so the writer adds the attribute rather than replaying a spelling that " +
		"invites a consumer to drop the trailing space (ECMA-376 Part 1 §18.4.12). " +
		"TestSMLSourceForm_TextNeedingSpacePreserveIsRepaired states that rule.",
}

// TestSMLSourceForm_CorpusIsByteIdentical is the corpus-wide statement: every
// xlsx fixture upstream ships goes back byte for byte on an untranslated round
// trip, one named exclusion aside.
func TestSMLSourceForm_CorpusIsByteIdentical(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(testdataDir(t), "*.xlsx"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	sort.Strings(files)

	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if why, skip := xlsxSourceFormExclusions[filepath.Base(path)]; skip {
				t.Skip(why)
			}
			original, err := os.ReadFile(path)
			require.NoError(t, err)
			out := skeletonRoundtripBytes(t, original, filepath.Base(path))
			assertSamePackageBytes(t, original, out)
		})
	}
}

// assertSamePackageBytes compares every entry of two OpenXML packages.
func assertSamePackageBytes(t *testing.T, want, got []byte) {
	t.Helper()
	wantZR, err := zip.NewReader(bytes.NewReader(want), int64(len(want)))
	require.NoError(t, err)
	gotZR, err := zip.NewReader(bytes.NewReader(got), int64(len(got)))
	require.NoError(t, err)

	gotParts := map[string][]byte{}
	for _, f := range gotZR.File {
		gotParts[f.Name] = zipEntryBytes(t, f)
	}
	for _, f := range wantZR.File {
		data := zipEntryBytes(t, f)
		if !bytes.Equal(data, gotParts[f.Name]) {
			assert.Failf(t, "part rewritten",
				"%s: an untranslated round trip must go back byte for byte\n%s",
				f.Name, firstByteDifference(data, gotParts[f.Name]))
		}
	}
}

func zipEntryBytes(t *testing.T, f *zip.File) []byte {
	t.Helper()
	rc, err := f.Open()
	require.NoError(t, err)
	defer rc.Close()
	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	return data
}
