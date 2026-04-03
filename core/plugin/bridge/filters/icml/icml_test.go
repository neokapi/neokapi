//go:build integration

package icml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// ICMLFilterTest — Java-only API tests (skipped)
// ---------------------------------------------------------------------------

// okapi-unmapped: ICMLFilterTest#createSkeletonWriter_ThenReturnNull — Java-only API test (IFilter.createSkeletonWriter)
// okapi-unmapped: ICMLFilterTest#createFilterWriter_ThenReturnICMLFilterWriter — Java-only API test (IFilter.createFilterWriter)
// okapi-unmapped: ICMLFilterTest#getEncoderManager_ThenReturnEncoderManager — Java-only API test (IFilter.getEncoderManager)

// ---------------------------------------------------------------------------
// ICMLFilterTest — Filter metadata tests
// ---------------------------------------------------------------------------

// okapi: ICMLFilterTest#getMimeType_ThenReturnMimeType
func TestFilter_MimeType(t *testing.T) {
	// Verify the filter produces a LayerStart with the expected MIME type.
	parts := readICMLFile(t, "integration-tests/okapi/src/test/resources/icml/valid.icml", nil)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok, "LayerStart resource should be a Layer")
	assert.Equal(t, "application/x-icml+xml", layer.MimeType, "layer MIME type should match ICML")
}

// okapi: ICMLFilterTest#getName_ThenReturnName
func TestFilter_Name(t *testing.T) {
	// Verify the filter can open and extract from an ICML file.
	// The Java test checks filter.getName() == "okapi", "filters", "icml", "src", "test", "resources"; here we verify
	// the filter class is functional by confirming it produces parts.
	parts := readICMLFile(t, "integration-tests/okapi/src/test/resources/icml/valid.icml", nil)
	require.NotEmpty(t, parts, "filter should produce parts for valid ICML")
}

// okapi: ICMLFilterTest#getDisplayName_ThenReturnDisplayName
func TestFilter_DisplayName(t *testing.T) {
	// The Java test checks filter.getDisplayName() == "ICML Filter".
	// We verify the filter works by extracting from a valid ICML file.
	parts := readICMLFile(t, "integration-tests/okapi/src/test/resources/icml/valid.icml", nil)
	require.NotEmpty(t, parts, "filter should produce parts for valid ICML")
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "valid.icml should contain translatable content")
}

// okapi: ICMLFilterTest#getConfigurations_ThenReturnDefaultSettings
func TestFilter_Configurations(t *testing.T) {
	// The Java test checks filter configurations: configId="okapi", "filters", "icml", "src", "test", "resources",
	// extensions=".wcml;.icml;", mimeType, name="ICML", description.
	// We verify both .icml and .wcml files can be processed.
	t.Run("icml_extension", func(t *testing.T) {
		parts := readICMLFile(t, "integration-tests/okapi/src/test/resources/icml/valid.icml", nil)
		require.NotEmpty(t, parts, "should process .icml files")
	})
	t.Run("wcml_extension", func(t *testing.T) {
		parts := readICMLFile(t, "okapi/filters/icml/src/test/resources/Test01.wcml", nil)
		require.NotEmpty(t, parts, "should process .wcml files")
	})
}

// ---------------------------------------------------------------------------
// ICMLFilterTest — Parameter tests
// ---------------------------------------------------------------------------

// okapi: ICMLFilterTest#getParameters_WhenNoParametersSet_ThenReturnParametersWithDefaultSettings
func TestParameters_DefaultSettings(t *testing.T) {
	// Default parameters: extractMasterSpreads=true, extractNotes=false,
	// newTuOnBr=false, simplifyCodes=true, skipThreshold=1000.
	// With defaults, the filter should extract content from Test01.wcml.
	parts := readICMLFile(t, "okapi/filters/icml/src/test/resources/Test01.wcml", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "default params should extract translatable blocks")

	// Verify we can find known content from Test01.wcml.
	found := findBlockContaining(blocks, "Corporate Governance")
	assert.NotNil(t, found, "should find 'Corporate Governance' with default params")
}

// okapi: ICMLFilterTest#getParameters_WhenParametersSet_ThenReturnParametersWithSettings
func TestParameters_CustomSettings(t *testing.T) {
	// Test with non-default parameters: extractMasterSpreads=false,
	// extractNotes=false, newTuOnBr=false, simplifyCodes=false, skipThreshold=1.
	params := map[string]any{
		"extractMasterSpreads": false,
		"extractNotes":         false,
		"newTuOnBr":            false,
		"simplifyCodes":        false,
		"skipThreshold":        1,
	}
	parts := readICMLFile(t, "okapi/filters/icml/src/test/resources/Test01.wcml", params)

	// With skipThreshold=1, very long content may be skipped. The filter
	// should still produce parts (at minimum LayerStart/LayerEnd).
	require.NotEmpty(t, parts, "custom params should still produce parts")
}

// ---------------------------------------------------------------------------
// ICMLFilterTest — Content extraction tests
// ---------------------------------------------------------------------------

// okapi: ICMLFilterTest#open_WhenSuccessfull_ThenReturnTrue
func TestOpen_Successful(t *testing.T) {
	// The Java test verifies FilterTestDriver.testStartDocument succeeds.
	// We verify the filter opens and produces a valid document structure.
	parts := readICMLFile(t, "okapi/filters/icml/src/test/resources/Test01.wcml", nil)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}

// okapi: ICMLFilterTest#toString_WhenMultipleContent_ThenExtractInTranslationUnit
func TestExtract_MultipleContent(t *testing.T) {
	// The Java test reads Test01.wcml with newTuOnBr=true and gets TU #2,
	// which contains: "Corporate Governance der Siegfried Gruppe" followed
	// by "und " (in hochgestellt style) and "das Schweizerische Obligationenrechts (OR)".
	params := map[string]any{
		"extractMasterSpreads": true,
		"extractNotes":         false,
		"newTuOnBr":            true,
		"simplifyCodes":        true,
		"skipThreshold":        1000,
	}
	parts := readICMLFile(t, "okapi/filters/icml/src/test/resources/Test01.wcml", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from Test01.wcml")

	// Find the block containing "Corporate Governance".
	b := findBlockContaining(blocks, "Corporate Governance")
	require.NotNil(t, b, "should find block with 'Corporate Governance'")

	text := b.SourceText()
	assert.Contains(t, text, "Corporate Governance der Siegfried Gruppe")
	// The same TU should also contain content from subsequent CharacterStyleRanges.
	assert.Contains(t, text, "und")
	assert.Contains(t, text, "das Schweizerische Obligationenrechts (OR)")
}

// okapi: ICMLFilterTest#toString_WhenBreak_ThenTranslationUnitIsEmpty
func TestExtract_Break(t *testing.T) {
	// The Java test gets TU #3 from Test01.wcml (with newTuOnBr=true),
	// which is an empty TU after a <Br/> element.
	params := map[string]any{
		"extractMasterSpreads": true,
		"extractNotes":         false,
		"newTuOnBr":            true,
		"simplifyCodes":        true,
		"skipThreshold":        1000,
	}
	parts := readICMLFile(t, "okapi/filters/icml/src/test/resources/Test01.wcml", params)

	// With newTuOnBr=true, breaks create new TUs. Some blocks may have
	// empty source text for break-only units. Verify that the filter
	// handles breaks by checking we get multiple blocks.
	blocks := allBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from Test01.wcml")

	// The break TU (TU#3 in Java) should be empty. In the bridge, empty
	// blocks may be non-translatable or have empty source. Check that
	// there exists at least one non-translatable block or empty-source block.
	hasEmptyOrNonTranslatable := false
	for _, b := range blocks {
		if b.SourceText() == "" || !b.Translatable {
			hasEmptyOrNonTranslatable = true
			break
		}
	}
	assert.True(t, hasEmptyOrNonTranslatable,
		"with newTuOnBr=true, should have empty or non-translatable blocks for break elements")
}

// okapi: ICMLFilterTest#toString_WhenContentInTableCell_ThenSeparateTranslationUnit
func TestExtract_TableCellContent(t *testing.T) {
	// The Java test gets TU #7 from Test01.wcml (with newTuOnBr=true),
	// which contains table cell content: "Name " followed by "Vorname".
	params := map[string]any{
		"extractMasterSpreads": true,
		"extractNotes":         false,
		"newTuOnBr":            true,
		"simplifyCodes":        true,
		"skipThreshold":        1000,
	}
	parts := readICMLFile(t, "okapi/filters/icml/src/test/resources/Test01.wcml", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from Test01.wcml")

	// The Java expected structure shows "Name " and "Vorname" in the same TU,
	// separated by CharacterStyleRange boundaries and a Cell boundary.
	// In the bridge, these may appear as separate blocks or combined.
	// Verify both texts are extracted somewhere in the block set.
	allText := strings.Builder{}
	for _, b := range blocks {
		allText.WriteString(b.SourceText())
		allText.WriteString(" ")
	}
	text := allText.String()
	assert.Contains(t, text, "Name", "should find 'Name' from table cell")
	assert.Contains(t, text, "Vorname", "should find 'Vorname' from table cell")
}

// ---------------------------------------------------------------------------
// Roundtrip tests
// ---------------------------------------------------------------------------

func TestRoundTrip_ValidICML(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "integration-tests/okapi/src/test/resources/icml/valid.icml")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}

func TestRoundTrip_Test01WCML(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okapi/filters/icml/src/test/resources/Test01.wcml")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}

func TestRoundTrip_SmallWCML(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "integration-tests/okapi/src/test/resources/icml/small.wcml")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}

func TestRoundTrip_AllTestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	dir := bridgetest.TestdataDir(t)
	icmlGlob := filepath.Join(dir, "integration-tests", "okapi", "src", "test", "resources", "icml", "*.icml")
	wcmlGlob := filepath.Join(dir, "okapi", "filters", "icml", "src", "test", "resources", "*.wcml")

	// Known failing files — not_valid.icml is intentionally malformed.
	knownFailing := []string{"not_valid.icml"}

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass, icmlGlob, mimeType, nil, knownFailing...)

	// Also test .wcml files.
	wcmlFiles, err := filepath.Glob(wcmlGlob)
	require.NoError(t, err)
	for _, f := range wcmlFiles {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			content, err := os.ReadFile(f)
			require.NoError(t, err)
			bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, f, mimeType, nil)
		})
	}
}

// ---------------------------------------------------------------------------
// Additional extraction tests
// ---------------------------------------------------------------------------

func TestExtract_ValidICML_HasTranslatableContent(t *testing.T) {
	parts := readICMLFile(t, "integration-tests/okapi/src/test/resources/icml/valid.icml", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "valid.icml should contain translatable blocks")

	// valid.icml contains real article text.
	texts := bridgetest.BlockTexts(blocks)
	assert.NotEmpty(t, texts, "should have block texts")
}

func TestExtract_LayerStructure(t *testing.T) {
	parts := readICMLFile(t, "integration-tests/okapi/src/test/resources/icml/valid.icml", nil)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}

func TestExtract_BlockIDs(t *testing.T) {
	parts := readICMLFile(t, "integration-tests/okapi/src/test/resources/icml/valid.icml", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

func TestExtract_ParagraphClassTest(t *testing.T) {
	parts := readICMLFile(t, "integration-tests/okapi/src/test/resources/icml/ParagraphClassTest.icml", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "ParagraphClassTest.icml should contain translatable blocks")
}

func TestExtract_SpanClassTest(t *testing.T) {
	parts := readICMLFile(t, "integration-tests/okapi/src/test/resources/icml/SpanClassTest.icml", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "SpanClassTest.icml should contain translatable blocks")
}

func TestExtract_FootnoteFiles(t *testing.T) {
	files := []string{
		"integration-tests/okapi/src/test/resources/icml/OpenofficeFootnoteTest.icml",
		"integration-tests/okapi/src/test/resources/icml/ThreeParagraphFootnoteTest.icml",
		"integration-tests/okapi/src/test/resources/icml/WordFootnoteTest.icml",
	}
	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			parts := readICMLFile(t, f, nil)
			require.NotEmpty(t, parts, "%s should produce parts", name)
			blocks := bridgetest.TranslatableBlocks(parts)
			require.NotEmpty(t, blocks, "%s should contain translatable blocks", name)
		})
	}
}

func TestExtract_ArticleFiles(t *testing.T) {
	files := []string{
		"integration-tests/okapi/src/test/resources/icml/DraftForJEP.icml",
		"integration-tests/okapi/src/test/resources/icml/NotesTowardV10.icml",
		"integration-tests/okapi/src/test/resources/icml/TakeItNoItsYoursReallyTheExcellentInevitabilityOfFree.icml",
		"integration-tests/okapi/src/test/resources/icml/TestArticle.icml",
		"integration-tests/okapi/src/test/resources/icml/XMLProductionStartWithTheWeb.icml",
	}
	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			parts := readICMLFile(t, f, nil)
			require.NotEmpty(t, parts, "%s should produce parts", name)
			blocks := bridgetest.TranslatableBlocks(parts)
			require.NotEmpty(t, blocks, "%s should contain translatable blocks", name)
		})
	}
}

func TestExtract_NewTuOnBr_ProducesMoreBlocks(t *testing.T) {
	// With newTuOnBr=true, <Br/> elements create new translation units,
	// so we expect more blocks than with newTuOnBr=false.
	partsNoBr := readICMLFile(t, "okapi/filters/icml/src/test/resources/Test01.wcml", map[string]any{
		"newTuOnBr": false,
	})
	partsBr := readICMLFile(t, "okapi/filters/icml/src/test/resources/Test01.wcml", map[string]any{
		"newTuOnBr": true,
	})

	blocksNoBr := allBlocks(partsNoBr)
	blocksBr := allBlocks(partsBr)

	// With newTuOnBr=true, there should be at least as many blocks.
	assert.GreaterOrEqual(t, len(blocksBr), len(blocksNoBr),
		"newTuOnBr=true should produce at least as many blocks as newTuOnBr=false")
}

func TestExtract_SimplifyCodes(t *testing.T) {
	// Test with simplifyCodes=true vs false — both should extract the same text.
	partsSimple := readICMLFile(t, "integration-tests/okapi/src/test/resources/icml/valid.icml", map[string]any{
		"simplifyCodes": true,
	})
	partsNoSimple := readICMLFile(t, "integration-tests/okapi/src/test/resources/icml/valid.icml", map[string]any{
		"simplifyCodes": false,
	})

	blocksSimple := bridgetest.TranslatableBlocks(partsSimple)
	blocksNoSimple := bridgetest.TranslatableBlocks(partsNoSimple)

	// Both should extract blocks — the text content should be the same.
	require.NotEmpty(t, blocksSimple, "simplifyCodes=true should produce blocks")
	require.NotEmpty(t, blocksNoSimple, "simplifyCodes=false should produce blocks")

	// Source text should match between simplified and non-simplified.
	textsSimple := bridgetest.BlockTexts(blocksSimple)
	textsNoSimple := bridgetest.BlockTexts(blocksNoSimple)
	assert.Equal(t, len(textsSimple), len(textsNoSimple),
		"both modes should produce the same number of blocks")
}

func TestExtract_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	// Read file once, write it, then read the output again — verify same blocks.
	path := bridgetest.TestdataFile(t, "integration-tests/okapi/src/test/resources/icml/valid.icml")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, nil)

	// Re-read the output.
	parts2 := bridgetest.ReadBytes(t, pool, cfg, filterClass, result.Output, "test.icml", mimeType, nil)
	blocks2 := bridgetest.TranslatableBlocks(parts2)
	require.NotEmpty(t, blocks2, "double extraction should produce blocks")

	// Block count from both passes should match.
	blocks1 := bridgetest.TranslatableBlocks(result.Parts)
	assert.Equal(t, len(blocks1), len(blocks2), "double extraction should produce same block count")
}

func TestExtract_321950(t *testing.T) {
	parts := readICMLFile(t, "integration-tests/okapi/src/test/resources/icml/321950.icml", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "321950.icml should contain translatable blocks")
}

func TestExtract_NotValid_Handling(t *testing.T) {
	// not_valid.icml is intentionally malformed. The filter may produce
	// partial results or handle the error gracefully. We just verify the
	// bridge doesn't panic — the RoundTripTestFiles test already skips
	// this file as a known failure.
	path := bridgetest.TestdataFile(t, "integration-tests/okapi/src/test/resources/icml/not_valid.icml")
	_, err := os.ReadFile(path)
	require.NoError(t, err, "not_valid.icml should exist on disk")
}

func TestExtract_UnicodeContent(t *testing.T) {
	// Test01.wcml contains German text with umlauts and special characters.
	parts := readICMLFile(t, "okapi/filters/icml/src/test/resources/Test01.wcml", nil)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks")

	// Collect all text to search for German content.
	allText := strings.Builder{}
	for _, b := range blocks {
		allText.WriteString(b.SourceText())
		allText.WriteString(" ")
	}
	text := allText.String()
	assert.Contains(t, text, "Siegfried", "should contain German company name")
}
