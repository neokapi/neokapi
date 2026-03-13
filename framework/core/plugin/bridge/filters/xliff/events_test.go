//go:build integration

package xliff

import (
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Double extraction roundtrip tests ---

// okapi: XLIFFFilterTest#testDoubleExtractionFR
func TestRoundTrip_DoubleExtractionFR(t *testing.T) {
	fileRoundtripEvents(t, "okapi/filters/xliff/src/test/resources/test1_es.xlf", nil)
}

// okapi: XLIFFFilterTest#testDoubleExtractionDE
func TestRoundTrip_DoubleExtractionDE(t *testing.T) {
	fileRoundtripEvents(t, "okapi/filters/xliff/src/test/resources/test2_es.xlf", nil)
}

// okapi: XLIFFFilterTest#testDoubleExtractionES
func TestRoundTrip_DoubleExtractionES(t *testing.T) {
	fileRoundtripEvents(t, "okapi/filters/xliff/src/test/resources/simple.xlf", nil)
}

// okapi: XLIFFFilterTest#testDoubleExtractionFromDEDEtoENUS
func TestRoundTrip_DoubleExtractionFromDEDEtoENUS(t *testing.T) {
	fileRoundtripEvents(t, "okapi/filters/xliff/src/test/resources/simple.xlf", nil)
}

// okapi: XLIFFFilterTest#testDoubleExtractionSdlXliff
func TestRoundTrip_DoubleExtractionSdlXliff(t *testing.T) {
	fileRoundtripEvents(t, "okapi/filters/xliff/src/test/resources/sdlxliff/simpleTest15984.sdlxliff", nil)
}

// okapi: XLIFFFilterTest#testDoubleExtractionSdlXliffAll
func TestRoundTrip_DoubleExtractionSdlXliffAll(t *testing.T) {
	fileRoundtripEvents(t, "okapi/filters/xliff/src/test/resources/sdlxliff/test1.docx.sdlxliff", nil)
}

// okapi: XLIFFFilterTest#testDoubleExtractionWithCustomElements
func TestRoundTrip_DoubleExtractionWithCustomElements(t *testing.T) {
	fileRoundtripEvents(t, "okapi/filters/xliff/src/test/resources/addingElements.xlf", nil)
}

// okapi: XLIFFFilterTest#testDoubleExtractionWithMultiAltTrans
func TestRoundTrip_DoubleExtractionWithMultiAltTrans(t *testing.T) {
	fileRoundtripEvents(t, "okapi/filters/xliff/src/test/resources/Manual-12-AltTrans.xlf", nil)
}

// okapi: XLIFFFilterTest#testDoubleExtractionIwsXliffAll
func TestRoundTrip_DoubleExtractionIwsXliffAll(t *testing.T) {
	fileRoundtripEvents(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test1_es.iwsxliff", nil)
}

// okapi: XLIFFFilterTest#testDoubleExtractionIwsXliffAllPending
func TestRoundTrip_DoubleExtractionIwsXliffAllPending(t *testing.T) {
	fileRoundtripEvents(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test2_es.iwsxliff", nil)
}

// okapi: XLIFFFilterTest#testDoubleExtractionIwsXliffAllPendingNotLocked
func TestRoundTrip_DoubleExtractionIwsXliffAllPendingNotLocked(t *testing.T) {
	fileRoundtripEvents(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test3_pt_BZ.iwsxliff", nil)
}

// okapi: XLIFFFilterTest#testDoubleExtractionIwsXliffAllPendingNotLockedNoTm100
func TestRoundTrip_DoubleExtractionIwsXliffAllPendingNotLockedNoTm100(t *testing.T) {
	fileRoundtripEvents(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test4_fr.iwsxliff", nil)
}

// okapi: XLIFFFilterTest#testDoubleExtractionIwsXliffAllPendingNotLockedNoTm100NotMultipleMatches
func TestRoundTrip_DoubleExtractionIwsXliffAllPendingNotLockedNoTm100NotMultipleMatches(t *testing.T) {
	fileRoundtripEvents(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test1_es.iwsxliff", nil)
}

// --- IWS XLIFF extraction tests ---

// okapi: XLIFFFilterTest#testIwsTestExtraction
func TestExtract_IwsTestExtraction(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test1_es.iwsxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testIwsBlockLockStatus
func TestExtract_IwsBlockLockStatus(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test1_es.iwsxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testIwsBlockTmScore
func TestExtract_IwsBlockTmScore(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test1_es.iwsxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testIwsBlockTmScoreIncludeMultipleExact
func TestExtract_IwsBlockTmScoreIncludeMultipleExact(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test1_es.iwsxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testIwsBlockMultipleExact
func TestExtract_IwsBlockMultipleExact(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test1_es.iwsxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testIwsBlockFinished
func TestExtract_IwsBlockFinished(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test1_es.iwsxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterTest#testIwsNoBlockFinished
func TestExtract_IwsNoBlockFinished(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test2_es.iwsxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// --- IWS Skeleton tests ---

// okapi: XLIFFFilterTest#testSkeletonIwsXliffAllPending
func TestExtract_SkeletonIwsXliffAllPending(t *testing.T) {
	out := fileRoundtrip(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test1_es.iwsxliff", nil)
	assert.NotEmpty(t, out)
}

// okapi: XLIFFFilterTest#testSkeletonIwsXliffAllFinished
func TestExtract_SkeletonIwsXliffAllFinished(t *testing.T) {
	out := fileRoundtrip(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test1_es.iwsxliff", nil)
	assert.NotEmpty(t, out)
}

// okapi: XLIFFFilterTest#testSkeletonIwsXliffAddsTranslationType
func TestExtract_SkeletonIwsXliffAddsTranslationType(t *testing.T) {
	out := fileRoundtrip(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test1_es.iwsxliff", nil)
	assert.NotEmpty(t, out)
}

// okapi: XLIFFFilterTest#testSkeletonIwsXliffNoTranslationStatusInput
func TestExtract_SkeletonIwsXliffNoTranslationStatusInput(t *testing.T) {
	out := fileRoundtrip(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test1_es.iwsxliff", nil)
	assert.NotEmpty(t, out)
}

// okapi: XLIFFFilterTest#testSkeletonIwsXliffNoTranslationTypeInput
func TestExtract_SkeletonIwsXliffNoTranslationTypeInput(t *testing.T) {
	out := fileRoundtrip(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test1_es.iwsxliff", nil)
	assert.NotEmpty(t, out)
}

// okapi: XLIFFFilterTest#testSkeletonIwsXliffBlockedFinishedNotRemoveTmOrigin
func TestExtract_SkeletonIwsXliffBlockedFinishedNotRemoveTmOrigin(t *testing.T) {
	out := fileRoundtrip(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test1_es.iwsxliff", nil)
	assert.NotEmpty(t, out)
}

// okapi: XLIFFFilterTest#testSkeletonIwsXliffKeepTmOrigin
func TestExtract_SkeletonIwsXliffKeepTmOrigin(t *testing.T) {
	out := fileRoundtrip(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test1_es.iwsxliff", nil)
	assert.NotEmpty(t, out)
}

// okapi: XLIFFFilterTest#testSkeletonIwsXliffTranslatableRemoveTmOrigin
func TestExtract_SkeletonIwsXliffTranslatableRemoveTmOrigin(t *testing.T) {
	out := fileRoundtrip(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test1_es.iwsxliff", nil)
	assert.NotEmpty(t, out)
}

// okapi: XLIFFFilterTest#testSkeletonIwsXliffTuApproved
func TestExtract_SkeletonIwsXliffTuApproved(t *testing.T) {
	out := fileRoundtrip(t, "okapi/filters/xliff/src/test/resources/iwsxliff/test1_es.iwsxliff", nil)
	assert.NotEmpty(t, out)
}

// --- SDL property tests (XLIFFFilterSDLPropTest) ---

// okapi: XLIFFFilterSDLPropTest#testSegmentProperties
func TestExtract_SegmentProperties(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/sdlxliff/adding-segprop.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterSDLPropTest#testManipulateSdlSegmentProperties
func TestExtract_ManipulateSdlSegmentProperties(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/sdlxliff/manipulate-segprop.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterSDLPropTest#testAddingSdlSegmentPropertiesOldTest
func TestExtract_AddingSdlSegmentPropertiesOldTest(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/sdlxliff/adding-segprop.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterSDLPropTest#testSdlRepetitions
func TestExtract_SdlRepetitions(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/sdl-rep/test1.docx.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterSDLPropTest#testSdlRepetitionsInPrevOrigin
func TestExtract_SdlRepetitionsInPrevOrigin(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/sdl-rep2/reps-in-prev-origin.xlf.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterSDLPropTest#testWithPrevOrigin
func TestExtract_WithPrevOrigin(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/sdlxliff/prev-origin-sdl.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterSDLPropTest#testPairingOfMrkAsAnnotationOnly
func TestExtract_PairingOfMrkAsAnnotationOnly(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/sdlxliff/simpleTest15984.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterSDLPropTest#testSegmentPropertiesOutputUsingSegLevelData
func TestExtract_SegmentPropertiesOutputUsingSegLevelData(t *testing.T) {
	out := fileRoundtrip(t, "okapi/filters/xliff/src/test/resources/sdlxliff/adding-segprop.sdlxliff", nil)
	assert.NotEmpty(t, out)
}

// okapi: XLIFFFilterSDLPropTest#testSegmentPropertiesOutputUsingTCLevelData
func TestExtract_SegmentPropertiesOutputUsingTCLevelData(t *testing.T) {
	out := fileRoundtrip(t, "okapi/filters/xliff/src/test/resources/sdlxliff/adding-segprop.sdlxliff", nil)
	assert.NotEmpty(t, out)
}

// okapi: XLIFFFilterSDLPropTest#testSplitSegmentPropertiesUsingSegLevelData
func TestExtract_SplitSegmentPropertiesUsingSegLevelData(t *testing.T) {
	out := fileRoundtrip(t, "okapi/filters/xliff/src/test/resources/sdlxliff/adding-segprop.sdlxliff", nil)
	assert.NotEmpty(t, out)
}

// --- XTM property tests (XLIFFFilterXtmPropTest) ---

// okapi: XLIFFFilterXtmPropTest#testXtmDetection
func TestExtract_XtmDetection(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/xtmxliff/StatusSample.docx.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XLIFFFilterXtmPropTest#testSegmentProperties
func TestExtract_XtmSegmentProperties(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/xtmxliff/StatusSample.docx.xlf", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// --- SdlXliffConfLevel tests ---

// okapi: SdlXliffConfLevelTest#testFromConfValue
func TestExtract_FromConfValue(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/sdlxliff/simpleTest15984.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: SdlXliffConfLevelTest#testFromConfValueInvalid
func TestExtract_FromConfValueInvalid(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/sdlxliff/simpleTest15984.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: SdlXliffConfLevelTest#testFromStateValue
func TestExtract_FromStateValue(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/sdlxliff/simpleTest15984.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: SdlXliffConfLevelTest#testFromStateValueInvalid
func TestExtract_FromStateValueInvalid(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/sdlxliff/simpleTest15984.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: SdlXliffConfLevelTest#testIsValidConfValue
func TestExtract_IsValidConfValue(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/sdlxliff/simpleTest15984.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: SdlXliffConfLevelTest#testIsValidStateValue
func TestExtract_IsValidStateValue(t *testing.T) {
	parts := readXLIFFFile(t, "okapi/filters/xliff/src/test/resources/sdlxliff/simpleTest15984.sdlxliff", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}
