package mif_test

// This file ports the upstream Okapi (Java) MIF filter tests
// (net.sf.okapi.filters.mif, v1.48.0: ExtractionTest, DocumentTest,
// ExtractsTest, RoundTripTest) to native neokapi tests.
//
// Two structural facts about the okapi tests shape every port here:
//
//  1. Okapi's snippet tests cram the whole document onto one or two
//     physical lines (STARTMIF = "<MIFFile 9.00><TextFlow <Para "). The
//     native parseMIF is a line-oriented scanner (one statement nest
//     level per physical line), so the okapi single-line snippets do not
//     round-trip through it. Each ported snippet is therefore rewritten
//     in the equivalent multi-line MIF form — the SAME logical document,
//     just one statement per line. This is faithful to the MIF Reference
//     (Adobe FrameMaker Parameters/MIF Reference): newlines between
//     statements are insignificant.
//
//  2. Okapi represents a paragraph as ONE TextUnit whose TextFragment
//     carries inline Codes (rendered <1/>, <2/>, ...). The native reader
//     instead splits a paragraph into MULTIPLE Blocks at every inline-code
//     boundary (see extractParaRuns in reader.go). Tests that assert on
//     the single-unit-with-codes shape, or on Code.getData() skeleton
//     internals, cannot be expressed against the native model and are
//     recorded as TODO skips (real parity gaps tracked by #558 / #509),
//     never as false // okapi: claims.

import (
	"bytes"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/mif"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const okapiResDir = "/Users/asgeirf/src/okapi/Okapi/okapi/filters/mif/src/test/resources"

// openFixture opens an upstream okapi MIF fixture, skipping the test (per
// the repo convention used by idml/transtable/wiki ports) when the okapi
// checkout is not present on this machine.
func openFixture(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open(okapiResDir + "/" + name)
	if err != nil {
		t.Skipf("upstream okapi MIF fixture not present (%s): %v", name, err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// readSnippetBlocks parses a MIF snippet with the default config and
// returns the translatable blocks' source text.
func readSnippetBlocks(t *testing.T, input string) []string {
	t.Helper()
	ctx := t.Context()
	r := mif.NewReader()
	require.NoError(t, r.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	t.Cleanup(func() { r.Close() })
	blocks := testutil.CollectBlocks(t, r.Read(ctx))
	out := make([]string, len(blocks))
	for i, b := range blocks {
		out[i] = b.SourceText()
	}
	return out
}

// readFileBlocksCommon reads a fixture under the "Common" parameter set
// (master/reference/hidden pages and variables off — see okapi
// filters/mif Common.java) and returns block source texts.
func readFileBlocksCommon(t *testing.T, name string, apply func(*mif.Config)) []string {
	t.Helper()
	ctx := t.Context()
	f := openFixture(t, name)
	r := mif.NewReader()
	c := r.Config().(*mif.Config)
	c.ExtractMasterPages = false
	c.ExtractReferencePages = false
	c.ExtractHiddenPages = false
	c.ExtractVariables = false
	if apply != nil {
		apply(c)
	}
	require.NoError(t, r.Open(ctx, testutil.RawDocFromReader(f, name, model.LocaleEnglish)))
	t.Cleanup(func() { r.Close() })
	blocks := testutil.CollectBlocks(t, r.Read(ctx))
	out := make([]string, len(blocks))
	for i, b := range blocks {
		out[i] = b.SourceText()
	}
	return out
}

// =====================================================================
// PORTED — behavior verified to match upstream Okapi on the native model.
// =====================================================================

// okapi: ExtractionTest#processesSupportedVersions — a bare <MIFFile N> header
// of a supported version is accepted and yields exactly the StartDocument +
// version DocumentPart + EndDocument trio (3 events in okapi; LayerStart +
// version Data + LayerEnd = 3 parts natively).
func TestProcessesSupportedVersions(t *testing.T) {
	ctx := t.Context()
	for _, version := range []string{"8.00", "2015"} {
		r := mif.NewReader()
		require.NoError(t, r.Open(ctx, testutil.RawDocFromString("<MIFFile "+version+">", model.LocaleEnglish)))
		parts := testutil.CollectParts(t, r.Read(ctx))
		r.Close()

		require.Len(t, parts, 3, "version %s should emit LayerStart + Data + LayerEnd", version)
		assert.Equal(t, model.PartLayerStart, parts[0].Type)
		assert.Equal(t, model.PartData, parts[1].Type)
		assert.Equal(t, model.PartLayerEnd, parts[2].Type)

		data := parts[1].Resource.(*model.Data)
		assert.Equal(t, "MIFFile", data.Properties["tag"])
		assert.Equal(t, version, data.Properties["version"])
	}
}

// okapi: ExtractionTest#testV10IsUsingV9Encoding — the v9 and v10 encodings of
// the same document decode to identical translatable content (FrameMaker 10
// keeps the FrameMaker 9 byte/glyph encoding). We assert the two fixtures
// yield the same number of blocks and pairwise-equal source text.
func TestV10IsUsingV9Encoding(t *testing.T) {
	ctx := t.Context()
	read := func(name string) []string {
		f := openFixture(t, name)
		r := mif.NewReader()
		require.NoError(t, r.Open(ctx, testutil.RawDocFromReader(f, name, model.LocaleEnglish)))
		blocks := testutil.CollectBlocks(t, r.Read(ctx))
		r.Close()
		out := make([]string, len(blocks))
		for i, b := range blocks {
			out[i] = b.SourceText()
		}
		return out
	}
	v9 := read("TestEncoding-v9.mif")
	v10 := read("TestEncoding-v10.mif")
	require.Equal(t, len(v9), len(v10), "v9 and v10 must decode to the same number of blocks")
	assert.Equal(t, v9, v10, "v9 and v10 source content must be identical")
}

// okapi: ExtractionTest#testSimpleEntry — a single <String> with escaped
// backslash and ampersand decodes to "text \ and &", and the writer
// re-encodes it byte-exactly (`\\` round-trips). Okapi crams the snippet on
// one line; the native equivalent is the same statement nested across lines.
func TestSimpleEntry(t *testing.T) {
	ctx := t.Context()
	input := "<MIFFile 9.00>\n<TextFlow\n <Para\n  <Unique 12345>\n  <ParaLine\n   <String `text \\\\ and &'>\n  >\n >\n>\n"

	r := mif.NewReader()
	w := mif.NewWriter()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	r.SetSkeletonStore(store)
	w.SetSkeletonStore(store)

	require.NoError(t, r.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, r.Read(ctx))
	r.Close()

	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "text \\ and &", blocks[0].SourceText())

	var buf bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&buf))
	require.NoError(t, w.Write(ctx, testutil.PartsToChannel(parts)))
	w.Close()
	assert.Equal(t, input, buf.String(), "<String> escape must round-trip byte-exactly")
}

// okapi: ExtractionTest#testTrimFontInFront — a leading <Font> (an inline
// code) before the first <String> is trimmed; only the String text "Text" is
// extracted.
func TestTrimFontInFront(t *testing.T) {
	blocks := readSnippetBlocks(t,
		"<MIFFile 9.00>\n<TextFlow\n <Para\n  <Unique 12345>\n  <ParaLine\n   <Font 1>\n   <String `Text'>\n  >\n >\n>\n")
	require.Len(t, blocks, 1)
	assert.Equal(t, "Text", blocks[0])
}

// okapi: ExtractionTest#testEmptyStringInFront — a leading empty <String>
// followed by a <Font> code and a real <String> extracts just "Text".
func TestEmptyStringInFront(t *testing.T) {
	blocks := readSnippetBlocks(t,
		"<MIFFile 9.00>\n<TextFlow\n <Para\n  <Unique 12345>\n  <ParaLine\n   <String `'>\n   <Font 1>\n   <String `Text'>\n  >\n >\n>\n")
	require.Len(t, blocks, 1)
	assert.Equal(t, "Text", blocks[0])
}

// okapi: ExtractionTest#testNormalFont — a fully-spelled <Font> statement
// (with FTag/FLanguage/FLocked children) is treated as an inline code and
// trimmed in front; "Text" is extracted.
func TestNormalFont(t *testing.T) {
	blocks := readSnippetBlocks(t,
		"<MIFFile 9.00>\n<TextFlow\n <Para\n  <Unique 123>\n  <ParaLine\n   <Font\n    <FTag `'>\n    <FLanguage NoLanguage>\n    <FLocked No>\n   > # end of Font\n   <String `Text'>\n  >\n >\n>\n")
	require.Len(t, blocks, 1)
	assert.Equal(t, "Text", blocks[0])
}

// okapi: ExtractionTest#testSoftHyphen — <Char SoftHyphen> is dropped (okapi
// "we remove those", CharLiteralToken), and the two ParaLines merge so
// "How" + "ever." becomes "However.".
func TestSoftHyphen(t *testing.T) {
	blocks := readSnippetBlocks(t,
		"<MIFFile 9.00>\n<TextFlow\n <Para\n  <Unique 123>\n  <ParaLine\n   <TextRectID 20>\n   <String `How'>\n   <Char SoftHyphen>\n  >\n  <ParaLine\n   <String `ever.'>\n  >\n >\n>\n")
	require.Len(t, blocks, 1)
	assert.Equal(t, "However.", blocks[0])
}

// okapi: ExtractionTest#testEmptyParaLine — an empty <ParaLine> produces no
// translatable text unit.
func TestEmptyParaLine(t *testing.T) {
	blocks := readSnippetBlocks(t,
		"<MIFFile 9.00>\n<TextFlow\n <Para\n  <Unique 123>\n  <ParaLine > # end of ParaLine\n >\n>\n")
	assert.Empty(t, blocks)
}

// okapi: ExtractionTest#testTwoPartsEntry — two consecutive <ParaLine>s in a
// single Para merge into one text unit "Part 1 and part 2".
func TestTwoPartsEntry(t *testing.T) {
	blocks := readSnippetBlocks(t,
		"<MIFFile 9.00>\n<TextFlow\n <Para\n  <Unique 12345>\n  <ParaLine\n   <String `Part 1'>\n  >\n  <ParaLine\n   <String ` and part 2'>\n  >\n >\n>\n")
	require.Len(t, blocks, 1)
	assert.Equal(t, "Part 1 and part 2", blocks[0])
}

// =====================================================================
// NOT APPLICABLE to the native reader/writer surface (okapi-internal API).
// =====================================================================

// okapi-skip: DocumentTest#iteratesThroughTheStatementsOfASample — exercises okapi's internal Document/Statements statement-iterator API (verbatim re-serialization of every parsed statement). The native reader has no public statement-stream type; it parses to the content model and reconstructs source via the SkeletonStore, so there is no equivalent surface to assert against.
// okapi-skip: DocumentTest#iteratesThroughTheStatementsOfEveryResourceUnderTest — same internal Document/Statements API, run over every fixture; no native equivalent (the native round-trip is exercised by the SkeletonStore byte-exact tests in skeleton_test.go instead).
// okapi-skip: ExtractsTest#gathersExtractsFromEveryResourceUnderTest — exercises okapi's internal Extracts + FontTags collector classes against every fixture. The native reader has no analogue of these standalone helper types; extraction is verified through the public Read() block stream.

// =====================================================================
// TODO — real native parity gaps (tracked by #558 native MIF audit and
// #509 bridge MIF Char Tab). Each test documents the upstream okapi
// expectation; none carries a false // okapi: claim. See FINAL REPORT.
// =====================================================================

// Native divergence: <Char Tab> (and other glyph Chars) are inlined as
// literal text by extractParaRuns, so a paragraph whose only content is
// Tab/Char glyphs wrongly yields a translatable block. Okapi treats Tab as
// an inline code (codeFinder-protected) and emits NO text unit for a
// char-only paragraph.
// okapi: ExtractionTest#testCharOnly — #509 native inlines <Char Tab> as literal text instead of treating it as an inline code
func TestCharOnly(t *testing.T) {
	t.Skip("TODO(#509): native inlines <Char Tab> as literal text; okapi (testCharOnly) emits no text unit for a Char-only paragraph")
}

// Native divergence: same Tab-as-text issue — okapi (testEndsInCharAndCode)
// drops the trailing <Char Tab>+codes and yields one unit "aaa"; native
// emits an extra "\t" block after "aaa".
// okapi: ExtractionTest#testEndsInCharAndCode — #509 native emits an extra tab-only block; okapi drops the trailing Char/codes into one unit "aaa"
func TestEndsInCharAndCode(t *testing.T) {
	t.Skip("TODO(#509): native emits an extra tab-only block after \"aaa\"; okapi yields the single unit \"aaa\"")
}

// Native divergence: okapi (testDummyCharString) routes the leading
// AFrame+Tab into the skeleton and extracts just "aaa"; native inlines the
// tab into the text, producing "\taaa".
// okapi: ExtractionTest#testDummyCharString — #509 native inlines the leading <Char Tab> into the text; okapi routes AFrame+Tab into the skeleton
func TestDummyCharString(t *testing.T) {
	t.Skip("TODO(#509): native inlines the leading <Char Tab> into the text (\"\\taaa\"); okapi extracts \"aaa\" with the tab in the skeleton")
}

// Native divergence: okapi (testTabsAndCodes) emits no text unit and keeps
// the tabs as skeleton <String '\t'> codes; native emits two "\t" blocks.
// okapi: ExtractionTest#testTabsAndCodes — #509 native extracts tab-only blocks; okapi emits no unit and keeps tabs as skeleton strings
func TestTabsAndCodes(t *testing.T) {
	t.Skip("TODO(#509): native extracts tab-only blocks; okapi emits no text unit and keeps tabs as skeleton strings")
}

// Native split-model divergence: okapi (testEmptyString) yields ONE unit
// "Text 1<1/> <2/> end" with inline codes; native splits the paragraph
// into separate blocks at every inline-code boundary.
// okapi: ExtractionTest#testEmptyString — #558 native splits paragraphs into per-run blocks; okapi keeps one TextUnit with inline codes
func TestEmptyString(t *testing.T) {
	t.Skip("TODO(#558): native splits paragraphs into per-run blocks; okapi keeps one TextUnit with inline codes (<1/>, <2/>)")
}

// Native split-model divergence: okapi (testCodeAtTheFront) yields ONE unit
// "text 1<2/>text 2"; native splits into two blocks at the inter-String
// <Font> code.
// okapi: ExtractionTest#testCodeAtTheFront — #558 native splits paragraphs into per-run blocks; okapi keeps one TextUnit with an inline code
func TestCodeAtTheFront(t *testing.T) {
	t.Skip("TODO(#558): native splits paragraphs into per-run blocks; okapi keeps one TextUnit with an inline code")
}

// Native split-model divergence: okapi (testDummyBeforeChar) yields ONE unit
// "Text 1<1/> Text 2" with an inline code for the Dummy; native splits
// into two blocks.
// okapi: ExtractionTest#testDummyBeforeChar — #558 native splits paragraphs into per-run blocks; okapi keeps one TextUnit with an inline code for the Dummy
func TestDummyBeforeChar(t *testing.T) {
	t.Skip("TODO(#558): native splits paragraphs into per-run blocks; okapi keeps one TextUnit with an inline code")
}

// Native split-model divergence: okapi (testEmptyFTag) yields ONE unit
// "Text 1 <2/>text 2"; native splits into two blocks at the AFrame code.
// okapi: ExtractionTest#testEmptyFTag — #558 native splits paragraphs into per-run blocks; okapi keeps one TextUnit with an inline code
func TestEmptyFTag(t *testing.T) {
	t.Skip("TODO(#558): native splits paragraphs into per-run blocks; okapi keeps one TextUnit with an inline code")
}

// Native divergence: okapi (testSlashCodes, v10) models VariableDef building
// blocks as inline codes, yielding "<zBold><1/><Default Z Font> ‍<3/><2/>";
// native flattens them and keeps the raw \x0b escape.
// okapi: ExtractionTest#testSlashCodes — #558 native does not model <$paranum>/building-block VariableDef tokens as inline codes
func TestSlashCodes(t *testing.T) {
	t.Skip("TODO(#558): native does not model <$paranum>/building-block VariableDef tokens as inline codes")
}

// Native divergence: okapi (testSlashCodesOutput) re-encodes VariableDef
// building blocks on output; native's VariableDef round-trip differs.
// okapi: ExtractionTest#testSlashCodesOutput — #558 native VariableDef building-block output differs from okapi's code re-encoding
func TestSlashCodesOutput(t *testing.T) {
	t.Skip("TODO(#558): native VariableDef building-block output differs from okapi's code re-encoding")
}

// Native divergence: native does not validate the MIF version, so an
// unsupported <MIFFile 7.00> is accepted (no error). Okapi
// (doesNotProcessUnsupportedVersions) throws OkapiBadFilterInputException
// "Unsupported document version: 7.00".
// okapi: ExtractionTest#doesNotProcessUnsupportedVersions — #558 native reader does not reject unsupported MIF versions; okapi throws on <MIFFile 7.00>
func TestDoesNotProcessUnsupportedVersions(t *testing.T) {
	t.Skip("TODO(#558): native reader does not reject unsupported MIF versions; okapi throws on <MIFFile 7.00>")
}

// Native divergence (body-page filtering): okapi (testBodyOnlyNoVariables)
// extracts only body-page text under the Common params — first two units
// "Line 1\nLine 2" and "à=agrave". Native lacks body-page-only scoping
// and extracts catalog/all-page content (~206 blocks, first is empty).
// okapi: ExtractionTest#testBodyOnlyNoVariables — #558 native lacks body-page-only extraction scoping; it pulls in catalog and non-body-page content
func TestBodyOnlyNoVariables(t *testing.T) {
	t.Skip("TODO(#558): native lacks body-page-only extraction scoping; it pulls in catalog and non-body-page content")
}

// Native divergence (body-page filtering): okapi
// (extractsBodyPageRelatedInformationOnly) extracts a single body-page unit
// "Goes over the PgfCatalog." from 893.mif; native extracts >1000 blocks.
// okapi: ExtractionTest#extractsBodyPageRelatedInformationOnly — #558 native lacks body-page-only extraction scoping (893.mif: native >1000 blocks vs okapi 1)
func TestExtractsBodyPageRelatedInformationOnly(t *testing.T) {
	t.Skip("TODO(#558): native lacks body-page-only extraction scoping (893.mif: native >1000 blocks vs okapi 1)")
}

// Native divergence (body-page filtering): okapi
// (extractsMultipleTextFramesPerPage) extracts 7 body-page text-frame units
// from 895.mif; native extracts a different set/order.
// okapi: ExtractionTest#extractsMultipleTextFramesPerPage — #558 native body-page text-frame extraction differs from okapi (895.mif)
func TestExtractsMultipleTextFramesPerPage(t *testing.T) {
	t.Skip("TODO(#558): native body-page text-frame extraction differs from okapi (895.mif)")
}

// Native divergence (numbered paragraph formats + body filtering): okapi
// (extractsNumberedParagraphFormats) extracts 8/3/5 units across the 896*
// fixtures with autonumber building-block markers (); native
// extracts ~190 blocks without the building-block markers.
// okapi: ExtractionTest#extractsNumberedParagraphFormats — #558 native lacks body-page scoping and autonumber building-block markers (896*.mif)
func TestExtractsNumberedParagraphFormats(t *testing.T) {
	t.Skip("TODO(#558): native lacks body-page scoping and autonumber building-block markers (896*.mif)")
}

// Native divergence (table-cell numbered formats + body filtering): okapi
// (extractsNumberedParagraphFormatInTableCells) extracts 17 ordered units
// from 904.mif with building-block markers; native extracts ~200 blocks.
// okapi: ExtractionTest#extractsNumberedParagraphFormatInTableCells — #558 native lacks body-page scoping and building-block markers (904.mif)
func TestExtractsNumberedParagraphFormatInTableCells(t *testing.T) {
	t.Skip("TODO(#558): native lacks body-page scoping and building-block markers (904.mif)")
}

// Native divergence (anchored frames + body filtering): okapi
// (extractsAnchoredFramesContent) extracts 2/1/8 units across 902-*.mif;
// native extracts ~190 blocks.
// okapi: ExtractionTest#extractsAnchoredFramesContent — #558 native lacks body-page scoping for anchored-frame extraction (902-*.mif)
func TestExtractsAnchoredFramesContent(t *testing.T) {
	t.Skip("TODO(#558): native lacks body-page scoping for anchored-frame extraction (902-*.mif)")
}

// Native divergence (nested anchored frames + body filtering): okapi
// (extractsNestedAnchoredFrames) extracts 14/5/4 units across 909-*.mif;
// native extracts ~190 blocks.
// okapi: ExtractionTest#extractsNestedAnchoredFrames — #558 native lacks body-page scoping for nested anchored-frame extraction (909-*.mif)
func TestExtractsNestedAnchoredFrames(t *testing.T) {
	t.Skip("TODO(#558): native lacks body-page scoping for nested anchored-frame extraction (909-*.mif)")
}

// Native divergence (sequential paragraph formats + body filtering): okapi
// (sequentialParagraphFormatsExtracted) extracts 4 units from 940.mif with
// building-block markers; native extracts ~189 blocks.
// okapi: ExtractionTest#sequentialParagraphFormatsExtracted — #558 native lacks body-page scoping and building-block markers (940.mif)
func TestSequentialParagraphFormatsExtracted(t *testing.T) {
	t.Skip("TODO(#558): native lacks body-page scoping and building-block markers (940.mif)")
}

// Native divergence (reference formats + body filtering): okapi
// (referenceFormatsConditionallyExtracted) conditionally extracts XRef
// reference formats with building-block markers from 938/1052.mif; native
// extracts ~188 blocks without them.
// okapi: ExtractionTest#referenceFormatsConditionallyExtracted — #558 native lacks body-page scoping and conditional reference-format extraction (938/1052.mif)
func TestReferenceFormatsConditionallyExtracted(t *testing.T) {
	t.Skip("TODO(#558): native lacks body-page scoping and conditional reference-format extraction (938/1052.mif)")
}

// Native divergence (text lines + body filtering): okapi
// (textLinesExtracted) extracts 5/12 ordered TextLine units from 942-*.mif;
// native extracts ~190 blocks.
// okapi: ExtractionTest#textLinesExtracted — #558 native lacks body-page scoping for TextLine extraction (942-*.mif)
func TestTextLinesExtracted(t *testing.T) {
	t.Skip("TODO(#558): native lacks body-page scoping for TextLine extraction (942-*.mif)")
}

// Native divergence (nested text frames + body filtering): okapi
// (nestedTextFramesExtracted) extracts 5 ordered units from 943.mif; native
// extracts ~190 blocks.
// okapi: ExtractionTest#nestedTextFramesExtracted — #558 native lacks body-page scoping for nested text-frame extraction (943.mif)
func TestNestedTextFramesExtracted(t *testing.T) {
	t.Skip("TODO(#558): native lacks body-page scoping for nested text-frame extraction (943.mif)")
}

// Native divergence (hard returns form new units + body filtering): okapi
// (hardReturnsFormNewTransUnits, ExtractHardReturnsAsText=false) splits at
// hard returns into ordered units across 987/990*.mif; native extracts
// catalog/all-page content.
// okapi: ExtractionTest#hardReturnsFormNewTransUnits — #558 native lacks body-page scoping; hard-return unit splitting differs (987/990*.mif)
func TestHardReturnsFormNewTransUnits(t *testing.T) {
	t.Skip("TODO(#558): native lacks body-page scoping; hard-return unit splitting differs (987/990*.mif)")
}

// Native divergence (index markers + body filtering): okapi
// (testExtractIndexMarkers) extracts the index-marker text "Text of marker"
// (type x-index) as the first text unit; native extracts catalog content
// first and does not surface the marker at that position.
// okapi: ExtractionTest#testExtractIndexMarkers — #558 native lacks body-page scoping; index-marker extraction position differs (TestMarkers.mif)
func TestExtractIndexMarkers(t *testing.T) {
	t.Skip("TODO(#558): native lacks body-page scoping; index-marker extraction position differs (TestMarkers.mif)")
}

// Native divergence (hypertext links + body filtering): okapi
// (testExtractLinks) gates link text on ExtractLinks and surfaces it as the
// 5th text unit (type link); native extracts catalog content and the link
// count barely changes.
// okapi: ExtractionTest#testExtractLinks — #558 native lacks body-page scoping; hypertext-link extraction differs (TestMarkers.mif)
func TestExtractLinks(t *testing.T) {
	t.Skip("TODO(#558): native lacks body-page scoping; hypertext-link extraction differs (TestMarkers.mif)")
}

// Native divergence (tabs as codes + hard returns as text + building
// blocks): okapi (tabsRepresentedAsCodesAndHardReturnsAsText) yields 2 units
// with tabs/building-blocks as codes from 1188_crlf.mif; native inlines tabs
// and omits building-block markers, yielding 4 blocks (first empty).
// okapi: ExtractionTest#tabsRepresentedAsCodesAndHardReturnsAsText — #509/#558 native inlines tabs as text and omits building-block markers (1188_crlf.mif)
func TestTabsRepresentedAsCodesAndHardReturnsAsText(t *testing.T) {
	t.Skip("TODO(#509/#558): native inlines tabs as text and omits building-block markers (1188_crlf.mif)")
}

// Native divergence: okapi
// (tabsRepresentedAsCodesAndNewTextualUnitsFormedOnHardReturnsAppearance,
// ExtractHardReturnsAsText=false) yields 4 ordered units from 1188_crlf.mif;
// native's tab/hard-return handling produces a different block set.
// okapi: ExtractionTest#tabsRepresentedAsCodesAndNewTextualUnitsFormedOnHardReturnsAppearance — #509/#558 native tab/hard-return unit formation differs from okapi (1188_crlf.mif)
func TestTabsRepresentedAsCodesAndNewTextualUnitsFormedOnHardReturnsAppearance(t *testing.T) {
	t.Skip("TODO(#509/#558): native tab/hard-return unit formation differs from okapi (1188_crlf.mif)")
}

// Native round-trip gap: okapi (RoundTripTest#consequentialEmptyParaLinesMerged)
// round-trips 1187_crlf.mif line-for-line. Native writer is not byte-exact on
// this fixture (462 -> 431 bytes; content dropped).
// okapi: RoundTripTest#consequentialEmptyParaLinesMerged — #558 native round-trip of 1187_crlf.mif is not byte-exact (462 -> 431 bytes)
func TestConsequentialEmptyParaLinesMerged(t *testing.T) {
	t.Skip("TODO(#558): native round-trip of 1187_crlf.mif is not byte-exact (462 -> 431 bytes)")
}

// Native round-trip gap: okapi
// (RoundTripTest#tabsEncodedOnExtractionAndHardReturnsEncodedOnMerge)
// round-trips 1188_crlf.mif with both ExtractHardReturnsAsText settings.
// Native writer is not byte-exact (6807 -> 6740 bytes).
// okapi: RoundTripTest#tabsEncodedOnExtractionAndHardReturnsEncodedOnMerge — #558 native round-trip of 1188_crlf.mif is not byte-exact (6807 -> 6740 bytes)
func TestTabsEncodedOnExtractionAndHardReturnsEncodedOnMerge(t *testing.T) {
	t.Skip("TODO(#558): native round-trip of 1188_crlf.mif is not byte-exact (6807 -> 6740 bytes)")
}

// Native round-trip gap: okapi
// (RoundTripTest#hardReturnsAsNonTextualRoundTripped) round-trips the
// 987/990* fixtures with the non-textual-hard-returns config. Native writer
// is not byte-exact (e.g. 987.mif 235724 -> 234872 bytes).
// okapi: RoundTripTest#hardReturnsAsNonTextualRoundTripped — #558 native round-trip of 987/990* fixtures is not byte-exact
func TestHardReturnsAsNonTextualRoundTripped(t *testing.T) {
	t.Skip("TODO(#558): native round-trip of 987/990* fixtures is not byte-exact")
}

// Ensure helpers stay referenced even though most fixture-backed bodies are
// currently TODO skips; this keeps the readFileBlocksCommon path compiled
// and exercised so the body-page-scoping fix (#558) has a ready harness.
func TestReadFixtureSmoke(t *testing.T) {
	blocks := readFileBlocksCommon(t, "Test01.mif", nil)
	assert.NotEmpty(t, blocks, "Test01.mif should yield at least one block under Common params")
}
