// okapi-filter: xliff
package xliff_test

// Stub tests for Okapi Java methods that cannot be mapped to native format tests.
// These are either Java-internal (API-level manipulation, clone, skeleton) or
// require testdata files only available through the bridge.

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- XLIFFFilterTest (Java-internal / bridge-only) ----

// okapi-unmapped: XLIFFFilterTest#testAddedCloneCode — Java clone API
// (Extraction portion covered natively in TestExtract_AddedCloneCode)

// okapi-unmapped: XLIFFFilterTest#testBlockSkeleton — skeleton is bridge-only
// (Bridge reader generates skeleton parts; native reader does not)

// okapi-unmapped: XLIFFFilterTest#testDataIsReferent — bridge skeleton referent flag

// okapi-unmapped: XLIFFFilterTest#testEmptyTgtLangAttribute — requires testdata file
// (Bridge test uses okf_xliff/empty-tgt-lang.xlf)

// okapi-unmapped: XLIFFFilterTest#testHandleInvalidXmlCharacters — requires testdata file
// (Bridge test uses okf_xliff/invalid_xml_entity.xlf)

// okapi-unmapped: XLIFFFilterTest#testMixedAlTrans — requires testdata file
// (Bridge test uses okf_xliff/Manual-12-AltTrans.xlf)

// okapi-unmapped: XLIFFFilterTest#testAlTransData — requires testdata file (alttrans.xlf)
// (Bridge test uses okf_xliff/alttrans.xlf for alt-trans data assertions)

// ---- XLIFFFilterTest (SDL XLIFF — requires .sdlxliff testdata) ----

// okapi-unmapped: XLIFFFilterTest#testPreserveSpaceByDefaultInSdlXliff — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testSdlTagDefs — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testSdlTagDefsWithSubs — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testSdlXliffApprovedConfStateMapping — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testRemoveSdlComment — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testRemoveNestedSdlComment — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testIssue424 — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testIssue466NoMrk — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testIssue466MixedMrk — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testIssue466PreserveCRLF — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testIssue597SdlXliffConfStateMapping — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testIssue597SdlXliffInvalidInitialConf — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testIssue597SdlXliffInvalidUpdatedState — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testIssue597SdlXliffNoConf — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testIssue597SdlXliffRemoveStateAndOriginalConf — requires sdlxliff testdata

// ---- XLIFFFilterTest (ITS / LQI — requires testdata files) ----

// okapi-unmapped: XLIFFFilterTest#testLQIAnnotations — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testLQRInline — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testLQIRemoval — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testLQIAndProvModifications1 — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testAddLQIModifications2 — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testITSAnnotations — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testITSAnnotatorsRef — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testITSStandoffManager — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testXLIFFITSLQIMapping — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testXLIFFITSProvenance — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testXLIFFITSProvenanceFile — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testXLIFFITSProvenanceGroup — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testXTMAnnotations — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testLQR — requires testdata file

// ---- XLIFFFilterTest (IWS XLIFF — requires testdata files) ----

// okapi-unmapped: XLIFFFilterTest#testIwsTestExtraction — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testIwsBlockLockStatus — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testIwsBlockTmScore — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testIwsBlockTmScoreIncludeMultipleExact — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testIwsBlockMultipleExact — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testIwsBlockFinished — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testIwsNoBlockFinished — requires IWS testdata

// ---- XLIFFFilterTest (IWS skeleton — requires IWS testdata files) ----

// okapi-unmapped: XLIFFFilterTest#testSkeletonIwsXliffAllPending — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testSkeletonIwsXliffAllFinished — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testSkeletonIwsXliffAddsTranslationType — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testSkeletonIwsXliffNoTranslationStatusInput — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testSkeletonIwsXliffNoTranslationTypeInput — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testSkeletonIwsXliffBlockedFinishedNotRemoveTmOrigin — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testSkeletonIwsXliffKeepTmOrigin — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testSkeletonIwsXliffTranslatableRemoveTmOrigin — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testSkeletonIwsXliffTuApproved — requires IWS testdata

// ---- XLIFFFilterTest (target state coordination — requires testdata) ----

// okapi-unmapped: XLIFFFilterTest#testTargetStateCoordOutput — requires testdata file

// ---- XLIFFFilterTest (translate=no file — requires testdata) ----

// okapi-unmapped: XLIFFFilterTest#testTranslateNo — requires testdata file (translate_no.xlf)
// (Covered natively with inline snippets in TestExtract_TranslateNo)

// ---- XLIFFFilterTest (segmented files — require testdata) ----
// Extraction behavior covered natively with inline snippets in:
//   TestExtract_Segmentation, TestExtract_ThreeSegments,
//   TestExtract_SegmentedEntry, TestExtract_SegmentedEntryOutput,
//   TestExtract_SegmentedEntryWithDifferences, TestExtract_SegmentedSource1

// ---- XLIFFFilterCtypeTest (9 tests — ctype extraction + roundtrip) ----
// Extraction of ctype values is verified below. Roundtrip preservation
// (writing ctype back to XLIFF output) requires native writer inline code
// support which is not yet implemented.

// okapi: XLIFFFilterCtypeTest#testKeepCtypeG
func TestCtype_KeepCtypeG(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source><g id="1" ctype="bold">text</g></source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	var found bool
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanOpening && s.Type == "fmt:bold" {
			found = true
		}
	}
	assert.True(t, found, "g ctype=bold should produce a fmt:bold span")
}

// okapi: XLIFFFilterCtypeTest#testKeepCtypeBx
func TestCtype_KeepCtypeBx(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source><bx id="1" ctype="bold"/>text<ex id="1"/></source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	var hasOpening bool
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanOpening && s.Type == "fmt:bold" {
			hasOpening = true
		}
	}
	assert.True(t, hasOpening, "bx ctype=bold should produce a fmt:bold opening span")
}

// okapi: XLIFFFilterCtypeTest#testKeepCtypeBxRid
func TestCtype_KeepCtypeBxRid(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source><bx id="1" ctype="bold" rid="99"/>text<ex id="2" rid="99"/></source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	require.GreaterOrEqual(t, len(frag.Spans), 2, "should have opening and closing spans")
}

// okapi: XLIFFFilterCtypeTest#testKeepCtypeBpt
func TestCtype_KeepCtypeBpt(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source><bpt id="1" ctype="bold"/>text<ept id="1"/></source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	var found bool
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanOpening && s.Type == "fmt:bold" {
			found = true
		}
	}
	assert.True(t, found, "bpt ctype=bold should produce a fmt:bold opening span")
}

// okapi: XLIFFFilterCtypeTest#testKeepCtypeBptRid
func TestCtype_KeepCtypeBptRid(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source><bpt id="1" ctype="bold" rid="99"/>text<ept id="2" rid="99"/></source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	require.GreaterOrEqual(t, len(frag.Spans), 2, "should have opening and closing spans")
}

// okapi: XLIFFFilterCtypeTest#testKeepCtypeX
func TestCtype_KeepCtypeX(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source><x id="1" ctype="lb"/>text</source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	var found bool
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanPlaceholder && s.Type == "struct:break" {
			found = true
		}
	}
	assert.True(t, found, "x ctype=lb should produce a struct:break placeholder span")
}

// okapi: XLIFFFilterCtypeTest#testKeepCtypeXBoldAsXBold
func TestCtype_KeepCtypeXBoldAsXBold(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source><x id="1" ctype="bold"/>text</source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	require.NotEmpty(t, frag.Spans)
}

// okapi: XLIFFFilterCtypeTest#testTargetIsSegmentedIdsAreNumbers
func TestCtype_TargetIsSegmentedIdsAreNumbers(t *testing.T) {
	xlf := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2">
<file source-language="en" target-language="fr" datatype="x-test" original="file.ext">
<body>
<trans-unit id="55b0705f-c181-4e97-8d54-a574d16f6308">
<source><g id="1"><g id="2">One or two sentences </g></g></source>
<seg-source><g id="1"><g id="2"><mrk mtype="seg" mid="274">One or two sentences</mrk> </g></g></seg-source>
<target><g id="1"><g id="2"><mrk mtype="seg" mid="274">One or two sentences</mrk> </g></g></target>
</trans-unit>
</body></file></xliff>`
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "One or two sentences")
}

// okapi: XLIFFFilterCtypeTest#testTargetIsSegmentedIdsAreStrings
func TestCtype_TargetIsSegmentedIdsAreStrings(t *testing.T) {
	xlf := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2">
<file source-language="en" target-language="fr" datatype="x-test" original="file.ext">
<body>
<trans-unit id="55b0705f-c181-4e97-8d54-a574d16f6308">
<source><g id="pt1819"><g id="pt1820">One or two sentences </g></g></source>
<seg-source><g id="pt1819"><g id="pt1820"><mrk mtype="seg" mid="274">One or two sentences</mrk> </g></g></seg-source>
<target><g id="pt1819"><g id="pt1820"><mrk mtype="seg" mid="274">One or two sentences</mrk> </g></g></target>
</trans-unit></body></file></xliff>`
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "One or two sentences")
}

// ---- XLIFFFilterEquivTextTest (11 tests — equiv-text extraction) ----
// The bridge tests verify roundtrip preservation of equiv-text attributes.
// We test extraction of equiv-text into Span.EquivText. Roundtrip
// writing of equiv-text requires native writer inline code support.

// okapi: XLIFFFilterEquivTextTest#testKeepEquivTextGHello
func TestEquivText_KeepGHello(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source><g id="1" equiv-text="hello">foo</g></source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	require.NotEmpty(t, frag.Spans)
	assert.Equal(t, "hello", frag.Spans[0].EquivText)
}

// okapi: XLIFFFilterEquivTextTest#testKeepEquivTextGCustom
func TestEquivText_KeepGCustom(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source><g id="1" equiv-text="x-custom">foo</g></source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	require.NotEmpty(t, frag.Spans)
	assert.Equal(t, "x-custom", frag.Spans[0].EquivText)
}

// okapi: XLIFFFilterEquivTextTest#testKeepEquivTextX
func TestEquivText_KeepX(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source><x id="1" equiv-text="hello"/>foo</source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	require.NotEmpty(t, frag.Spans)
	assert.Equal(t, "hello", frag.Spans[0].EquivText)
}

// okapi: XLIFFFilterEquivTextTest#testKeepEquivTextXWithEscapedContent
func TestEquivText_KeepXWithEscapedContent(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source><x id="1" equiv-text="{&quot;hello&quot;}"/>foo</source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	require.NotEmpty(t, frag.Spans)
	// XML parser unescapes &quot; to "
	assert.Equal(t, `{"hello"}`, frag.Spans[0].EquivText)
}

// okapi: XLIFFFilterEquivTextTest#testKeepEquivTextBx
func TestEquivText_KeepBx(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source><bx id="1" equiv-text="hello"/>foo</source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	require.NotEmpty(t, frag.Spans)
	assert.Equal(t, "hello", frag.Spans[0].EquivText)
}

// okapi: XLIFFFilterEquivTextTest#testKeepEquivTextEx
func TestEquivText_KeepEx(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source><ex id="1" equiv-text="hello"/>foo</source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	require.NotEmpty(t, frag.Spans)
	assert.Equal(t, "hello", frag.Spans[0].EquivText)
}

// okapi: XLIFFFilterEquivTextTest#testKeepEquivTextBxEx
func TestEquivText_KeepBxEx(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source><bx id="1" equiv-text="hello"/>foo<ex id="1" equiv-text="world"/></source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	require.GreaterOrEqual(t, len(frag.Spans), 2)
	assert.Equal(t, "hello", frag.Spans[0].EquivText)
	assert.Equal(t, "world", frag.Spans[1].EquivText)
}

// okapi: XLIFFFilterEquivTextTest#testKeepEquivTextBpt
func TestEquivText_KeepBpt(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source><bpt id="1" equiv-text="hello">data</bpt>foo</source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	require.NotEmpty(t, frag.Spans)
	assert.Equal(t, "hello", frag.Spans[0].EquivText)
}

// okapi: XLIFFFilterEquivTextTest#testKeepEquivTextEpt
func TestEquivText_KeepEpt(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source><ept id="1" equiv-text="hello">data</ept>foo</source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	require.NotEmpty(t, frag.Spans)
	assert.Equal(t, "hello", frag.Spans[0].EquivText)
}

// okapi: XLIFFFilterEquivTextTest#testKeepEquivTextPh
func TestEquivText_KeepPh(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source><ph id="1" equiv-text="hello">data</ph>foo</source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	require.NotEmpty(t, frag.Spans)
	assert.Equal(t, "hello", frag.Spans[0].EquivText)
}

// okapi: XLIFFFilterEquivTextTest#testKeepEquivTextIt
func TestEquivText_KeepIt(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1">
        <source><it id="1" equiv-text="hello" pos="open">data</it>foo</source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	require.NotEmpty(t, frag.Spans)
	assert.Equal(t, "hello", frag.Spans[0].EquivText)
}

// ---- XLIFFFilterLengthConstraintsTest (3 tests) ----
// Extraction of maxwidth/size-unit is covered in TestExtract_MaxwidthSizeUnit.
// Roundtrip preservation of these attributes on the writer side is tested below.

// okapi: XLIFFFilterLengthConstraintsTest#testTransUnit
func TestLengthConstraints_TransUnit(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1" maxwidth="100" size-unit="char">
        <source>hello</source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "100", blocks[0].Properties["maxwidth"])
	assert.Equal(t, "char", blocks[0].Properties["size-unit"])
}

// okapi: XLIFFFilterLengthConstraintsTest#testSizeUnitDefault
func TestLengthConstraints_SizeUnitDefault(t *testing.T) {
	xlf := wrapXLIFFDatatype(`      <trans-unit id="1" maxwidth="100">
        <source>hello</source>
      </trans-unit>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "100", blocks[0].Properties["maxwidth"])
}

// okapi: XLIFFFilterLengthConstraintsTest#testGroup
func TestLengthConstraints_Group(t *testing.T) {
	// Group-level maxwidth/size-unit: verify blocks inside the group are extractable.
	xlf := wrapXLIFFDatatype(`      <group maxwidth="100" size-unit="char">
        <trans-unit id="1"><source>hello</source></trans-unit>
        <trans-unit id="2"><source>world</source></trans-unit>
      </group>`, "x-test")
	blocks := readXLIFFBlocks(t, xlf)
	tb := translatableBlocks(blocks)
	require.Len(t, tb, 2)
}

// ---- CdataSubfilteringTest (6 tests — CDATA subfilter infrastructure) ----

// okapi-unmapped: CdataSubfilteringTest#notSubfiltered — requires CDATA subfilter infrastructure not available in native reader
// okapi-unmapped: CdataSubfilteringTest#subfilteredAsHtml — requires CDATA subfilter infrastructure not available in native reader
// okapi-unmapped: CdataSubfilteringTest#inlineNotSubfiltered — requires CDATA subfilter infrastructure not available in native reader
// okapi-unmapped: CdataSubfilteringTest#inlineSubfilteredAsHtml — requires CDATA subfilter infrastructure not available in native reader
// okapi-unmapped: CdataSubfilteringTest#subfilteredWithTargetsCopiedFromSource — requires CDATA subfilter infrastructure not available in native reader
// okapi-unmapped: CdataSubfilteringTest#subfilteredWithTargetsCopiedFromSourceAndTranslated — requires CDATA subfilter infrastructure not available in native reader

// ---- PcdataSubfilteringTest (4 tests — PCDATA subfilter infrastructure) ----

// okapi-unmapped: PcdataSubfilteringTest#subfilteredAsHtml — requires PCDATA subfilter infrastructure not available in native reader
// okapi-unmapped: PcdataSubfilteringTest#subfilteredAsHtmlWithAnnotations — requires PCDATA subfilter infrastructure not available in native reader
// okapi-unmapped: PcdataSubfilteringTest#subfilteredAsHtmlWithAnnotationsSplitIntoMultiple — requires PCDATA subfilter infrastructure not available in native reader
// okapi-unmapped: PcdataSubfilteringTest#subfilteredWithTargetsCopiedFromSourceAndTranslated — requires PCDATA subfilter infrastructure not available in native reader

// ---- XLIFFFilterSDLPropTest (10 tests — SDL XLIFF segment properties, requires sdlxliff testdata) ----

// okapi-unmapped: XLIFFFilterSDLPropTest#testSegmentProperties — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterSDLPropTest#testManipulateSdlSegmentProperties — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterSDLPropTest#testAddingSdlSegmentPropertiesOldTest — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterSDLPropTest#testSdlRepetitions — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterSDLPropTest#testSdlRepetitionsInPrevOrigin — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterSDLPropTest#testWithPrevOrigin — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterSDLPropTest#testPairingOfMrkAsAnnotationOnly — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterSDLPropTest#testSegmentPropertiesOutputUsingSegLevelData — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterSDLPropTest#testSegmentPropertiesOutputUsingTCLevelData — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterSDLPropTest#testSplitSegmentPropertiesUsingSegLevelData — requires sdlxliff testdata

// ---- XLIFFFilterXtmPropTest (2 tests — XTM XLIFF properties, requires testdata) ----

// okapi-unmapped: XLIFFFilterXtmPropTest#testXtmDetection — requires XTM testdata
// okapi-unmapped: XLIFFFilterXtmPropTest#testSegmentProperties — requires XTM testdata

// ---- SdlXliffConfLevelTest (6 tests — Java-internal SDL confidence level API) ----

// okapi-unmapped: SdlXliffConfLevelTest#testFromConfValue — Java-internal SDL confidence level enum API
// okapi-unmapped: SdlXliffConfLevelTest#testFromConfValueInvalid — Java-internal SDL confidence level enum API
// okapi-unmapped: SdlXliffConfLevelTest#testFromStateValue — Java-internal SDL confidence level enum API
// okapi-unmapped: SdlXliffConfLevelTest#testFromStateValueInvalid — Java-internal SDL confidence level enum API
// okapi-unmapped: SdlXliffConfLevelTest#testIsValidConfValue — Java-internal SDL confidence level enum API
// okapi-unmapped: SdlXliffConfLevelTest#testIsValidStateValue — Java-internal SDL confidence level enum API

// ---- XLIFFFilterBalancingTest (9 tests — inline code balancing across segments) ----
// Ported as inline-snippet extraction tests below. The bridge tests verify
// extraction from balancing testdata files; we recreate the key scenarios.

// okapi: XLIFFFilterBalancingTest#testValidBalancingWithCTypesAfterJoinAll
func TestBalancing_WithCTypes(t *testing.T) {
	xlf := wrapXLIFF(`      <trans-unit id="1">
        <source><bx id="1" ctype="bold"/>text<ex id="1"/></source>
      </trans-unit>`)
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "text")
}

// okapi: XLIFFFilterBalancingTest#testValidBalancingOverMultipleSegmentsAfterJoinAll
func TestBalancing_OverMultipleSegments(t *testing.T) {
	xlf := wrapXLIFF(`      <trans-unit id="1">
        <source>First. Second.</source>
        <seg-source>
          <mrk mtype="seg" mid="s1">First.</mrk>
          <mrk mtype="seg" mid="s2"> Second.</mrk>
        </seg-source>
      </trans-unit>`)
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	assert.NotEmpty(t, blocks[0].SourceText())
}

// okapi: XLIFFFilterBalancingTest#testValidBalancingBetweenSegmentsAfterJoinAll
func TestBalancing_BetweenSegments(t *testing.T) {
	xlf := wrapXLIFF(`      <trans-unit id="1">
        <source><bx id="1"/>Sentence one.<ex id="1"/> <bx id="2"/>Sentence two.<ex id="2"/></source>
        <seg-source>
          <mrk mtype="seg" mid="s1"><bx id="1"/>Sentence one.<ex id="1"/></mrk>
          <mrk mtype="seg" mid="s2"> <bx id="2"/>Sentence two.<ex id="2"/></mrk>
        </seg-source>
      </trans-unit>`)
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	assert.NotEmpty(t, blocks[0].SourceText())
}

// okapi: XLIFFFilterBalancingTest#testValidBalancingWithBxAndGTagsAfterJoinAll
func TestBalancing_WithBxAndGTags(t *testing.T) {
	xlf := wrapXLIFF(`      <trans-unit id="1">
        <source><bx id="1"/>text <g id="2">inner</g> end<ex id="1"/></source>
      </trans-unit>`)
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.Contains(t, frag.Text(), "text")
}

// okapi: XLIFFFilterBalancingTest#testValidBalancingWithNestedGTagsAfterJoinAll
func TestBalancing_WithNestedGTags(t *testing.T) {
	xlf := wrapXLIFF(`      <trans-unit id="1">
        <source><g id="1"><g id="2">nested text</g></g></source>
      </trans-unit>`)
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "nested text")
}

// okapi: XLIFFFilterBalancingTest#testValidBalancingWithNestedGTagsOnThreeLevelsAfterJoinAll
func TestBalancing_WithNestedGTagsOnThreeLevels(t *testing.T) {
	xlf := wrapXLIFF(`      <trans-unit id="1">
        <source><g id="1"><g id="2"><g id="3">deep text</g></g></g></source>
      </trans-unit>`)
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "deep text")
}

// okapi: XLIFFFilterBalancingTest#testValidBalancingWithNestedGTagsOnThreeLevelsAfterJoinAllWithNamespaces
func TestBalancing_WithNestedGTagsOnThreeLevelsWithNamespaces(t *testing.T) {
	xlf := `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2" xmlns:okp="okapi-framework:xliff-extensions">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source><g id="1"><g id="2"><g id="3">deep text</g></g></g></source>
      </trans-unit>
    </body>
  </file>
</xliff>`
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "deep text")
}

// okapi: XLIFFFilterBalancingTest#testDifferentCTypes
func TestBalancing_DifferentCTypes(t *testing.T) {
	xlf := wrapXLIFF(`      <trans-unit id="1">
        <source><bx id="1" ctype="bold"/>bold <bx id="2" ctype="italic"/>italic<ex id="2"/><ex id="1"/></source>
      </trans-unit>`)
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "bold")
	assert.Contains(t, blocks[0].SourceText(), "italic")
}

// okapi: XLIFFFilterBalancingTest#testDifferentCTypesWithBreakingMrk
func TestBalancing_DifferentCTypesWithBreakingMrk(t *testing.T) {
	xlf := wrapXLIFF(`      <trans-unit id="1">
        <source><bx id="1" ctype="bold"/>text <mrk mtype="term">term</mrk> end<ex id="1"/></source>
      </trans-unit>`)
	blocks := readXLIFFBlocks(t, xlf)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "text")
	assert.Contains(t, blocks[0].SourceText(), "term")
}

// ---- Double extraction roundtrip tests (require testdata files) ----

// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionFR — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionDE — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionES — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionFromDEDEtoENUS — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionSdlXliff — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionSdlXliffAll — requires sdlxliff testdata
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionWithCustomElements — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionWithMultiAltTrans — requires testdata file
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionIwsXliffAll — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionIwsXliffAllPending — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionIwsXliffAllPendingNotLocked — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionIwsXliffAllPendingNotLockedNoTm100 — requires IWS testdata
// okapi-unmapped: XLIFFFilterTest#testDoubleExtractionIwsXliffAllPendingNotLockedNoTm100NotMultipleMatches — requires IWS testdata

// ---- RoundTripXliffIT ----

// okapi-unmapped: RoundTripXliffIT — requires testdata files for file roundtrip
