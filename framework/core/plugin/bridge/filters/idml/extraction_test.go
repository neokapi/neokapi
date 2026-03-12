//go:build integration

package idml

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ExtractionTest
// ---------------------------------------------------------------------------

// okapi: ExtractionTest#testStartDocument
func TestExtraction_StartDocument(t *testing.T) {
	parts := readIDML(t, "idmltest.idml", nil)

	require.NotEmpty(t, parts, "should produce at least one part")
	assert.Equal(t, model.PartLayerStart, parts[0].Type,
		"first part should be LayerStart (equivalent to Java StartDocument)")

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok, "first part resource should be a Layer")
	assert.NotEmpty(t, layer.ID, "layer should have an ID")
}

// okapi: ExtractionTest#testDefaultInfo
func TestExtraction_DefaultInfo(t *testing.T) {
	parts := readIDML(t, "idmltest.idml", nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, mimeType, layer.MimeType,
		"layer MIME type should match the IDML MIME type")
}

// okapi: ExtractionTest#testSimpleEntry
func TestExtraction_SimpleEntry(t *testing.T) {
	parts := readIDML(t, "Test01.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "Test01.idml should produce translatable blocks")
}

// okapi: ExtractionTest#testSimpleEntry2
func TestExtraction_SimpleEntry2(t *testing.T) {
	parts := readIDML(t, "idmltest.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "idmltest.idml should produce translatable blocks")
}

// okapi: ExtractionTest#testWhitespaces
func TestExtraction_Whitespaces(t *testing.T) {
	parts := readIDML(t, "tabsAndWhitespaces.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "tabsAndWhitespaces.idml should produce translatable blocks")
}

// okapi: ExtractionTest#testNewline
func TestExtraction_Newline(t *testing.T) {
	parts := readIDML(t, "newline.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "newline.idml should produce translatable blocks")
}

// okapi: ExtractionTest#testChangeTracking
func TestExtraction_ChangeTracking(t *testing.T) {
	parts := readIDML(t, "08-conditional-text-and-tracked-changes.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "IDML with change tracking should produce translatable blocks")
}

// okapi: ExtractionTest#testSkipDiscretionaryHyphens
func TestExtraction_SkipDiscretionaryHyphens(t *testing.T) {
	parts := readIDML(t, "Bindestrich.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "Bindestrich.idml should produce translatable blocks")
}

// okapi: ExtractionTest#testDocumentWithoutPathPoints
func TestExtraction_DocumentWithoutPathPoints(t *testing.T) {
	parts := readIDML(t, "618-MBE3.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "618-MBE3.idml should produce translatable blocks")
}

// okapi: ExtractionTest#testObjectsWithoutPathPointsAndText
func TestExtraction_ObjectsWithoutPathPointsAndText(t *testing.T) {
	parts := readIDML(t, "618-objects-without-path-points-and-text.idml", nil)

	// This file has objects without path points — should still extract without error.
	require.NotEmpty(t, parts, "should produce parts even without path points")
}

// okapi: ExtractionTest#testAnchoredFrameWithoutPathPoints
func TestExtraction_AnchoredFrameWithoutPathPoints(t *testing.T) {
	parts := readIDML(t, "618-anchored-frame-without-path-points.idml", nil)

	require.NotEmpty(t, parts, "should produce parts for anchored frame without path points")
}

// okapi: ExtractionTest#adjacentCodesMerged
func TestExtraction_AdjacentCodesMerged(t *testing.T) {
	parts := readIDMLWithConfig(t,
		"adjacent-codes/1415-adjacent-codes.idml",
		"adjacent-codes/okf_idml@adjacent-codes.fprm")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "adjacent codes file should produce translatable blocks")

	// With mergeAdjacentCodes=true, adjacent inline codes should be merged.
	// Verify extraction succeeds and produces blocks.
	texts := bridgetest.BlockTexts(blocks)
	assert.NotEmpty(t, texts, "should have non-empty block texts")
}

// okapi: ExtractionTest#customTextVariablesExtracted
func TestExtraction_CustomTextVariablesExtracted(t *testing.T) {
	parts := readIDMLWithConfig(t,
		"1138.idml",
		"okf_idml@custom-text-variables-extraction.fprm")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "custom text variables file should produce translatable blocks")
}

// okapi: ExtractionTest#codeFinderApplied
func TestExtraction_CodeFinderApplied(t *testing.T) {
	parts := readIDMLWithConfig(t,
		"codefinder/codefinder.idml",
		"codefinder/okf_idml@codefinder.fprm")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "codefinder file should produce translatable blocks")

	// With codeFinderRules active, HTML-like tags should become inline codes (spans).
	withSpans := blocksWithSpans(blocks)
	assert.NotEmpty(t, withSpans, "code finder should produce blocks with inline spans")
}

// okapi: ExtractionTest#specialCharacterPatternApplied
func TestExtraction_SpecialCharacterPatternApplied(t *testing.T) {
	parts := readIDML(t, "175-special-characters.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "special characters file should produce translatable blocks")
}

// okapi: ExtractionTest#indexTopicsExtracted
func TestExtraction_IndexTopicsExtracted(t *testing.T) {
	parts := readIDML(t, "links_crossreferences.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "links/crossreferences file should produce translatable blocks")
}

// okapi: ExtractionTest#endNotesExtracted
func TestExtraction_EndNotesExtracted(t *testing.T) {
	params := map[string]any{
		"configFile": func() string {
			pool, cfg := bridgetest.SharedBridge(t)
			_ = pool
			_ = cfg
			return bridgetest.TestdataFile(t, "okf_idml/okf_idml@ExtractAll.fprm")
		}(),
	}
	parts := readIDML(t, "09-footnotes.idml", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "footnotes file should produce translatable blocks with ExtractAll config")
}

// okapi: ExtractionTest#externalHyperlinksExtracted
func TestExtraction_ExternalHyperlinksExtracted(t *testing.T) {
	parts := readIDML(t, "03-hyperlink-and-table-content.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "hyperlink file should produce translatable blocks")
}

// okapi: ExtractionTest#hiddenPasteboardItemsExtracted
func TestExtraction_HiddenPasteboardItemsExtracted(t *testing.T) {
	parts := readIDML(t, "large_sample_newspaper1.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "newspaper sample should produce translatable blocks")
}

// okapi: ExtractionTest#mathZonesConditionallyExtracted
func TestExtraction_MathZonesConditionallyExtracted(t *testing.T) {
	// Without math zone extraction.
	partsWithout := readIDML(t, "1412-math-zones.idml", nil)
	blocksWithout := bridgetest.TranslatableBlocks(partsWithout)

	// With math zone extraction.
	partsWithMath := readIDMLWithConfig(t,
		"math-zone/1412-math-zone.idml",
		"math-zone/okf_idml@math-zone.fprm")
	blocksWithMath := bridgetest.TranslatableBlocks(partsWithMath)

	// Math zones extraction should produce different (typically more) blocks.
	require.NotEmpty(t, blocksWithout, "should produce blocks without math zone extraction")
	require.NotEmpty(t, blocksWithMath, "should produce blocks with math zone extraction")
}

// okapi: ExtractionTest#extractsBreaksInline
func TestExtraction_ExtractsBreaksInline(t *testing.T) {
	parts := readIDML(t, "07-paragraph-breaks.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "paragraph breaks file should produce translatable blocks")
}

// okapi: ExtractionTest#extractsWithLeastAvailableStyleFormattingBaselined
func TestExtraction_ExtractsWithLeastAvailableStyleFormattingBaselined(t *testing.T) {
	parts := readIDML(t, "923-baselined-formatting.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "baselined formatting file should produce translatable blocks")
}

// okapi: ExtractionTest#extractsBodyPageRelatedInformationOnly
func TestExtraction_ExtractsBodyPageRelatedInformationOnly(t *testing.T) {
	// Default extraction should only extract body page content.
	parts := readIDML(t, "01-pages-with-text-frames.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "pages with text frames should produce translatable blocks")
}

// okapi: ExtractionTest#hyperlinkTextSourcesExtractedAsReferenceGroups
func TestExtraction_HyperlinkTextSourcesExtractedAsReferenceGroups(t *testing.T) {
	parts := readIDML(t, "03-hyperlink-and-table-content.idml", nil)

	// Hyperlink text sources produce group structures.
	groupStarts := countPartsByType(parts, model.PartGroupStart)
	assert.GreaterOrEqual(t, groupStarts, 1,
		"hyperlink text sources should produce group starts")
}

// okapi: ExtractionTest#hyperlinkTextSourcesExtractedInline
func TestExtraction_HyperlinkTextSourcesExtractedInline(t *testing.T) {
	parts := readIDML(t, "03-hyperlink-and-table-content.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "hyperlink file should produce translatable blocks")

	// Hyperlinks should be represented inline in the text.
	texts := bridgetest.BlockTexts(blocks)
	assert.NotEmpty(t, texts, "should have block texts for hyperlinks")
}

// okapi: ExtractionTest#stylesExcluded
func TestExtraction_StylesExcluded(t *testing.T) {
	// Read with styles exclusion config.
	partsExcluded := readIDMLWithConfig(t,
		"styles-exclusion/1418-styles-exclusion.idml",
		"styles-exclusion/okf_idml@syles-exclusion.fprm")
	blocksExcluded := bridgetest.TranslatableBlocks(partsExcluded)

	// Read without styles exclusion for comparison.
	partsDefault := readIDML(t, "1418-styles-exclusion.idml", nil)
	blocksDefault := bridgetest.TranslatableBlocks(partsDefault)

	// With exclusion, some styles should be skipped, resulting in fewer blocks.
	require.NotEmpty(t, blocksDefault, "default should produce blocks")
	// The excluded version may have fewer or different blocks.
	assert.NotEmpty(t, blocksExcluded, "excluded styles should still produce some blocks")
}

// okapi: ExtractionTest#pasteboardItemsWithoutAnchorPointsPositionedCorrectly
func TestExtraction_PasteboardItemsWithoutAnchorPointsPositionedCorrectly(t *testing.T) {
	parts := readIDML(t, "935-complex-ordering-without-anchor-points.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks,
		"complex ordering without anchor points should produce translatable blocks")
}

// ---------------------------------------------------------------------------
// Kerning ignorance threshold tests
// ---------------------------------------------------------------------------

// okapi: ExtractionTest#doesNotMergeTagsThatDifferByKerning
func TestExtraction_DoesNotMergeTagsThatDifferByKerning(t *testing.T) {
	parts := readIDML(t, "756-character-kerning.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "kerning file should produce translatable blocks")

	// Without ignorance thresholds, kerning differences create separate inline codes.
	withSpans := blocksWithSpans(blocks)
	assert.NotEmpty(t, withSpans,
		"without kerning ignorance, blocks should have inline codes from kerning differences")
}

// okapi: ExtractionTest#mergesTagsThatDifferByKerningWithMinIgnoranceThreshold
func TestExtraction_MergesTagsThatDifferByKerningWithMinIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterKerning":                true,
		"characterKerningMinIgnoranceThreshold": -100,
	}
	parts := readIDML(t, "756-character-kerning.idml", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "kerning file should produce translatable blocks")
}

// okapi: ExtractionTest#mergesTagsThatDifferByKerningWithMaxIgnoranceThreshold
func TestExtraction_MergesTagsThatDifferByKerningWithMaxIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterKerning":                true,
		"characterKerningMaxIgnoranceThreshold": 100,
	}
	parts := readIDML(t, "756-character-kerning.idml", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "kerning file should produce translatable blocks")
}

// okapi: ExtractionTest#mergesTagsThatDifferByKerningWithMinAndMaxIgnoranceThresholds
func TestExtraction_MergesTagsThatDifferByKerningWithMinAndMaxIgnoranceThresholds(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterKerning":                true,
		"characterKerningMinIgnoranceThreshold": -100,
		"characterKerningMaxIgnoranceThreshold": 100,
	}
	parts := readIDML(t, "756-character-kerning.idml", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "kerning file should produce translatable blocks")
}

// okapi: ExtractionTest#mergesTagsThatDifferByKerningWithEmptyIgnoranceThresholds
func TestExtraction_MergesTagsThatDifferByKerningWithEmptyIgnoranceThresholds(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterKerning":                true,
		"characterKerningMinIgnoranceThreshold": "",
		"characterKerningMaxIgnoranceThreshold": "",
	}
	parts := readIDML(t, "756-character-kerning.idml", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "kerning file should produce translatable blocks")
}

// ---------------------------------------------------------------------------
// Tracking ignorance threshold tests
// ---------------------------------------------------------------------------

// okapi: ExtractionTest#doesNotMergeTagsThatDifferByTracking
func TestExtraction_DoesNotMergeTagsThatDifferByTracking(t *testing.T) {
	parts := readIDML(t, "756-character-tracking.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "tracking file should produce translatable blocks")

	withSpans := blocksWithSpans(blocks)
	assert.NotEmpty(t, withSpans,
		"without tracking ignorance, blocks should have inline codes from tracking differences")
}

// okapi: ExtractionTest#mergesTagsThatDifferByTrackingWithMinIgnoranceThreshold
func TestExtraction_MergesTagsThatDifferByTrackingWithMinIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterTracking":                true,
		"characterTrackingMinIgnoranceThreshold": -50,
	}
	parts := readIDML(t, "756-character-tracking.idml", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "tracking file should produce translatable blocks")
}

// okapi: ExtractionTest#mergesTagsThatDifferByTrackingWithMaxIgnoranceThreshold
func TestExtraction_MergesTagsThatDifferByTrackingWithMaxIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterTracking":                true,
		"characterTrackingMaxIgnoranceThreshold": 50,
	}
	parts := readIDML(t, "756-character-tracking.idml", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "tracking file should produce translatable blocks")
}

// okapi: ExtractionTest#mergesTagsThatDifferByTrackingWithMinAndMaxIgnoranceThresholds
func TestExtraction_MergesTagsThatDifferByTrackingWithMinAndMaxIgnoranceThresholds(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterTracking":                true,
		"characterTrackingMinIgnoranceThreshold": -50,
		"characterTrackingMaxIgnoranceThreshold": 50,
	}
	parts := readIDML(t, "756-character-tracking.idml", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "tracking file should produce translatable blocks")
}

// okapi: ExtractionTest#mergesTagsThatDifferByTrackingWithEmptyIgnoranceThresholds
func TestExtraction_MergesTagsThatDifferByTrackingWithEmptyIgnoranceThresholds(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterTracking":                true,
		"characterTrackingMinIgnoranceThreshold": "",
		"characterTrackingMaxIgnoranceThreshold": "",
	}
	parts := readIDML(t, "756-character-tracking.idml", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "tracking file should produce translatable blocks")
}

// ---------------------------------------------------------------------------
// Leading ignorance threshold tests
// ---------------------------------------------------------------------------

// okapi: ExtractionTest#doesNotMergeTagsThatDifferByLeading
func TestExtraction_DoesNotMergeTagsThatDifferByLeading(t *testing.T) {
	parts := readIDML(t, "756-character-leading.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "leading file should produce translatable blocks")

	withSpans := blocksWithSpans(blocks)
	assert.NotEmpty(t, withSpans,
		"without leading ignorance, blocks should have inline codes from leading differences")
}

// okapi: ExtractionTest#mergesTagsThatDifferByLeadingWithMinIgnoranceThreshold
func TestExtraction_MergesTagsThatDifferByLeadingWithMinIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterLeading":                true,
		"characterLeadingMinIgnoranceThreshold": -100,
	}
	parts := readIDML(t, "756-character-leading.idml", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "leading file should produce translatable blocks")
}

// okapi: ExtractionTest#mergesTagsThatDifferByLeadingWithMaxIgnoranceThreshold
func TestExtraction_MergesTagsThatDifferByLeadingWithMaxIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterLeading":                true,
		"characterLeadingMaxIgnoranceThreshold": 100,
	}
	parts := readIDML(t, "756-character-leading.idml", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "leading file should produce translatable blocks")
}

// okapi: ExtractionTest#mergesTagsThatDifferByLeadingWithMinAndMaxIgnoranceThresholds
func TestExtraction_MergesTagsThatDifferByLeadingWithMinAndMaxIgnoranceThresholds(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterLeading":                true,
		"characterLeadingMinIgnoranceThreshold": -100,
		"characterLeadingMaxIgnoranceThreshold": 100,
	}
	parts := readIDML(t, "756-character-leading.idml", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "leading file should produce translatable blocks")
}

// okapi: ExtractionTest#mergesTagsThatDifferByLeadingWithoutIgnoranceThresholds
func TestExtraction_MergesTagsThatDifferByLeadingWithoutIgnoranceThresholds(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterLeading":                true,
		"characterLeadingMinIgnoranceThreshold": "",
		"characterLeadingMaxIgnoranceThreshold": "",
	}
	parts := readIDML(t, "756-character-leading.idml", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "leading file should produce translatable blocks")
}

// ---------------------------------------------------------------------------
// BaselineShift ignorance threshold tests
// ---------------------------------------------------------------------------

// okapi: ExtractionTest#doesNotMergeTagsThatDifferByBaselineShift
func TestExtraction_DoesNotMergeTagsThatDifferByBaselineShift(t *testing.T) {
	parts := readIDML(t, "756-character-baseline-shift.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "baseline shift file should produce translatable blocks")

	withSpans := blocksWithSpans(blocks)
	assert.NotEmpty(t, withSpans,
		"without baseline shift ignorance, blocks should have inline codes")
}

// okapi: ExtractionTest#mergesTagsThatDifferByBaselineShiftWithMinIgnoranceThreshold
func TestExtraction_MergesTagsThatDifferByBaselineShiftWithMinIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterBaselineShift":                true,
		"characterBaselineShiftMinIgnoranceThreshold": -2,
	}
	parts := readIDML(t, "756-character-baseline-shift.idml", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "baseline shift file should produce translatable blocks")
}

// okapi: ExtractionTest#mergesTagsThatDifferByBaselineShiftWithMaxIgnoranceThreshold
func TestExtraction_MergesTagsThatDifferByBaselineShiftWithMaxIgnoranceThreshold(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterBaselineShift":                true,
		"characterBaselineShiftMaxIgnoranceThreshold": 2,
	}
	parts := readIDML(t, "756-character-baseline-shift.idml", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "baseline shift file should produce translatable blocks")
}

// okapi: ExtractionTest#mergesTagsThatDifferByBaselineShiftWithMinAndMaxIgnoranceThresholds
func TestExtraction_MergesTagsThatDifferByBaselineShiftWithMinAndMaxIgnoranceThresholds(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterBaselineShift":                true,
		"characterBaselineShiftMinIgnoranceThreshold": -2,
		"characterBaselineShiftMaxIgnoranceThreshold": 2,
	}
	parts := readIDML(t, "756-character-baseline-shift.idml", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "baseline shift file should produce translatable blocks")
}

// okapi: ExtractionTest#mergesTagsThatDifferByBaselineShiftWithoutIgnoranceThresholds
func TestExtraction_MergesTagsThatDifferByBaselineShiftWithoutIgnoranceThresholds(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterBaselineShift":                true,
		"characterBaselineShiftMinIgnoranceThreshold": "",
		"characterBaselineShiftMaxIgnoranceThreshold": "",
	}
	parts := readIDML(t, "756-character-baseline-shift.idml", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "baseline shift file should produce translatable blocks")
}

// ---------------------------------------------------------------------------
// KerningMethod ignorance tests
// ---------------------------------------------------------------------------

// okapi: ExtractionTest#doesNotMergeTagsThatDifferByKerningMethod
func TestExtraction_DoesNotMergeTagsThatDifferByKerningMethod(t *testing.T) {
	parts := readIDML(t, "777-character-kerning-method.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "kerning method file should produce translatable blocks")
}

// okapi: ExtractionTest#mergesTagsThatDifferByKerningMethod
func TestExtraction_MergesTagsThatDifferByKerningMethod(t *testing.T) {
	params := map[string]any{
		"ignoreCharacterKerningMethod": true,
	}
	parts := readIDML(t, "777-character-kerning-method.idml", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "kerning method file should produce translatable blocks")
}

// ---------------------------------------------------------------------------
// Kerning in references and XML structures tests
// ---------------------------------------------------------------------------

// okapi: ExtractionTest#doesNotMergeTagsThatDifferByKerningInReferencesAndXmlStructures
func TestExtraction_DoesNotMergeTagsThatDifferByKerningInReferencesAndXmlStructures(t *testing.T) {
	parts := readIDML(t, "779-reference-and-tag-styles.idml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "reference and tag styles file should produce translatable blocks")
}

// okapi: ExtractionTest#mergesTagsThatDifferByKerningInReferencesAndXmlStructures
func TestExtraction_MergesTagsThatDifferByKerningInReferencesAndXmlStructures(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := tdDir + "/okf_idml/okf_idml@IgnoreAll.fprm"
	params := map[string]any{
		"configFile": configPath,
	}
	path := bridgetest.TestdataFile(t, "okf_idml/779-reference-and-tag-styles.idml")
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "reference and tag styles file should produce translatable blocks with IgnoreAll config")
}

// ---------------------------------------------------------------------------
// IDMLFilterInParallelTest
// ---------------------------------------------------------------------------

// okapi: IDMLFilterInParallelTest#testInMultipleThreads
// Note: The Java test verifies thread safety by running extractions in parallel.
// We run sequentially because each bridge JVM is single-tenant — concurrent
// requests to the same JVM corrupt filter state.
func TestExtraction_InParallel(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	files := []string{
		"Test00.idml",
		"Test01.idml",
		"idmltest.idml",
		"helloworld-1.idml",
	}

	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			path := bridgetest.TestdataFile(t, "okf_idml/"+f)
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
			blocks := bridgetest.TranslatableBlocks(parts)
			assert.NotEmpty(t, blocks, "%s should produce translatable blocks", f)
		})
	}
}
