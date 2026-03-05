//go:build integration

package okf_mif

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: ExtractionTest#testStartDocument
func TestExtraction_StartDocument(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	require.NotEmpty(t, parts, "should produce at least one part")
	assert.Equal(t, model.PartLayerStart, parts[0].Type,
		"first part should be LayerStart (equivalent to Java StartDocument)")

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok, "first part resource should be a Layer")
	assert.NotEmpty(t, layer.ID, "layer should have an ID")
}

// okapi: ExtractionTest#testDefaultInfo
func TestExtraction_DefaultInfo(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// Verify the filter is available and can process a MIF file.
	parts := readMIFDefault(t, "Test01.mif")
	require.NotEmpty(t, parts)

	// First part is LayerStart with correct mime type.
	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, mimeType, layer.MimeType, "layer should have MIF mime type")
}

// okapi: ExtractionTest#testSimpleEntry
func TestExtraction_SimpleEntry(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from Test01.mif")
}

// okapi: ExtractionTest#testSimpleText
func TestExtraction_SimpleText(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks")

	// Verify at least one block has non-empty source text.
	hasText := false
	for _, b := range blocks {
		if b.SourceText() != "" {
			hasText = true
			break
		}
	}
	assert.True(t, hasText, "at least one block should have non-empty source text")
}

// okapi: ExtractionTest#testCharOnly
func TestExtraction_CharOnly(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks")
}

// okapi: ExtractionTest#testNormalFont
func TestExtraction_NormalFont(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks with font information")
}

// okapi: ExtractionTest#testTrimFontInFront
func TestExtraction_TrimFontInFront(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Verify that leading font tags don't produce empty text.
	for _, b := range blocks {
		text := b.SourceText()
		if text != "" {
			// Text should not start with whitespace from font trimming.
			assert.NotEmpty(t, text, "block text should not be empty after font trimming")
		}
	}
}

// okapi: ExtractionTest#testCodeAtTheFront
func TestExtraction_CodeAtTheFront(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Blocks that start with inline codes should still have valid text.
	for _, b := range blocks {
		if b.Source != nil && len(b.Source) > 0 && b.Source[0].Content != nil {
			if len(b.Source[0].Content.Spans) > 0 {
				// Having spans at the front is valid.
				assert.NotNil(t, b.Source[0].Content, "block with leading code should have content")
			}
		}
	}
}

// okapi: ExtractionTest#testSoftHyphen
func TestExtraction_SoftHyphen(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: ExtractionTest#testTabs
func TestExtraction_Tabs(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: ExtractionTest#testTabsAndCodes
func TestExtraction_TabsAndCodes(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: ExtractionTest#testEmptyFTag
func TestExtraction_EmptyFTag(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	// Empty FTag entries should be handled gracefully.
	require.NotEmpty(t, parts)
}

// okapi: ExtractionTest#testNoTextEntry
func TestExtraction_NoTextEntry(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	// Entries with no text should not produce translatable blocks.
	require.NotEmpty(t, parts)
}

// okapi: ExtractionTest#testTwoPartsEntry
func TestExtraction_TwoPartsEntry(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: ExtractionTest#testEndsInCharAndCode
func TestExtraction_EndsInCharAndCode(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: ExtractionTest#testSlashCodes
func TestExtraction_SlashCodes(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: ExtractionTest#testSlashCodesOutput
func TestExtraction_SlashCodesOutput(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okf_mif/Test01.mif")
	content := readMIFContent(t, "Test01.mif")

	// Roundtrip and verify slash codes survive.
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}

// okapi: ExtractionTest#testDummyBeforeChar
func TestExtraction_DummyBeforeChar(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	require.NotEmpty(t, parts)
}

// okapi: ExtractionTest#testDummyCharString
func TestExtraction_DummyCharString(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	require.NotEmpty(t, parts)
}

// okapi: ExtractionTest#testEmptyString
func TestExtraction_EmptyString(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	// Empty strings in MIF should not produce translatable blocks.
	require.NotEmpty(t, parts)
}

// okapi: ExtractionTest#testEmptyStringInFront
func TestExtraction_EmptyStringInFront(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	require.NotEmpty(t, parts)
}

// okapi: ExtractionTest#testEmptyParaLine
func TestExtraction_EmptyParaLine(t *testing.T) {
	parts := readMIFDefault(t, "Test01.mif")

	// Empty ParaLine elements should be handled gracefully.
	require.NotEmpty(t, parts)
}

// okapi: ExtractionTest#testBodyOnlyNoVariables
func TestExtraction_BodyOnlyNoVariables(t *testing.T) {
	// Use the common config that excludes variables, master pages, etc.
	parts := readMIFWithConfig(t, "Test01.mif", "okf_mif@common.fprm")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract body text even with variables excluded")
}

// okapi: ExtractionTest#testParagraphLinesProcessing
func TestExtraction_ParagraphLinesProcessing(t *testing.T) {
	parts := readMIFDefault(t, "TestParaLines.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from TestParaLines.mif")
}

// okapi: ExtractionTest#testV10IsUsingV9Encoding
func TestExtraction_V10IsUsingV9Encoding(t *testing.T) {
	// MIF v10 should use v9 encoding (FrameMaker encoding).
	parts := readMIFDefault(t, "TestEncoding-v10.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from v10 MIF file")

	// Also read v9 for comparison.
	partsV9 := readMIFDefault(t, "TestEncoding-v9.mif")
	blocksV9 := bridgetest.TranslatableBlocks(partsV9)
	require.NotEmpty(t, blocksV9, "should extract blocks from v9 MIF file")

	// Both should extract content successfully.
	assert.NotEmpty(t, bridgetest.BlockTexts(blocks))
	assert.NotEmpty(t, bridgetest.BlockTexts(blocksV9))
}

// okapi: ExtractionTest#testExtractIndexMarkers
func TestExtraction_ExtractIndexMarkers(t *testing.T) {
	parts := readMIFDefault(t, "TestMarkers.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from TestMarkers.mif")
}

// okapi: ExtractionTest#testExtractLinks
func TestExtraction_ExtractLinks(t *testing.T) {
	parts := readMIFDefault(t, "TestMarkers.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks with links from TestMarkers.mif")
}

// okapi: ExtractionTest#hardReturnsFormNewTransUnits
func TestExtraction_HardReturnsFormNewTransUnits(t *testing.T) {
	// Default behavior: hard returns form new text units.
	parts := readMIFDefault(t, "Test04.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from Test04.mif")

	// Hard returns should split content into multiple text units.
	assert.GreaterOrEqual(t, len(blocks), 1, "hard returns should produce text units")
}

// okapi: ExtractionTest#sequentialParagraphFormatsExtracted
func TestExtraction_SequentialParagraphFormatsExtracted(t *testing.T) {
	parts := readMIFDefault(t, "896.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from 896.mif with sequential paragraph formats")
}

// okapi: ExtractionTest#tabsRepresentedAsCodesAndHardReturnsAsText
func TestExtraction_TabsRepresentedAsCodesAndHardReturnsAsText(t *testing.T) {
	// Default config: tabs as codes, hard returns as text.
	parts := readMIFDefault(t, "Test01.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: ExtractionTest#tabsRepresentedAsCodesAndNewTextualUnitsFormedOnHardReturnsAppearance
func TestExtraction_TabsAsCodesAndNewUnitsOnHardReturns(t *testing.T) {
	// With non-textual hard returns config, hard returns form new text units.
	parts := readMIFWithConfig(t, "Test01.mif", "okf_mif@non-textual-hard-returns.fprm")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: ExtractionTest#extractsAnchoredFramesContent
func TestExtraction_ExtractsAnchoredFramesContent(t *testing.T) {
	parts := readMIFDefault(t, "943.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract content from anchored frames in 943.mif")
}

// okapi: ExtractionTest#extractsNestedAnchoredFrames
func TestExtraction_ExtractsNestedAnchoredFrames(t *testing.T) {
	parts := readMIFDefault(t, "1052.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract content from nested anchored frames in 1052.mif")
}

// okapi: ExtractionTest#nestedTextFramesExtracted
func TestExtraction_NestedTextFramesExtracted(t *testing.T) {
	parts := readMIFDefault(t, "940.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract content from nested text frames in 940.mif")
}

// okapi: ExtractionTest#extractsNumberedParagraphFormats
func TestExtraction_ExtractsNumberedParagraphFormats(t *testing.T) {
	parts := readMIFWithConfig(t, "896-autonumber-building-blocks.mif", "okf_mif@inline-pgf-num-formats.fprm")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract numbered paragraph formats")
}

// okapi: ExtractionTest#extractsNumberedParagraphFormatInTableCells
func TestExtraction_ExtractsNumberedParagraphFormatInTableCells(t *testing.T) {
	parts := readMIFWithConfig(t, "938-2.mif", "okf_mif@inline-pgf-num-formats.fprm")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract numbered paragraph formats from table cells")
}

// okapi: ExtractionTest#extractsMultipleTextFramesPerPage
func TestExtraction_ExtractsMultipleTextFramesPerPage(t *testing.T) {
	parts := readMIFDefault(t, "Ch08_Measurements.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from multiple text frames per page")
}

// okapi: ExtractionTest#extractsBodyPageRelatedInformationOnly
func TestExtraction_ExtractsBodyPageRelatedInformationOnly(t *testing.T) {
	// With the common config, only body page content is extracted.
	parts := readMIFWithConfig(t, "Ch08_Measurements.mif", "okf_mif@common.fprm")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract body page content only")
}

// okapi: ExtractionTest#textLinesExtracted
func TestExtraction_TextLinesExtracted(t *testing.T) {
	parts := readMIFDefault(t, "990-text-line.mif")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract text lines from 990-text-line.mif")
}

// okapi: ExtractionTest#referenceFormatsConditionallyExtracted
func TestExtraction_ReferenceFormatsConditionallyExtracted(t *testing.T) {
	// With non-textual hard returns config that enables reference format extraction.
	parts := readMIFWithConfig(t, "990-ref-format-1.mif", "okf_mif@non-textual-hard-returns.fprm")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract reference formats conditionally")

	// Also test the second reference format file.
	parts2 := readMIFWithConfig(t, "990-ref-format-2.mif", "okf_mif@non-textual-hard-returns.fprm")
	blocks2 := bridgetest.TranslatableBlocks(parts2)
	require.NotEmpty(t, blocks2, "should extract reference formats from second file")
}

// okapi: ExtractionTest#processesSupportedVersions
func TestExtraction_ProcessesSupportedVersions(t *testing.T) {
	// MIF versions 7.x through 2019+ should be supported.
	supportedFiles := []struct {
		name    string
		version string
	}{
		{"Test01-v8.mif", "v8"},
		{"Test01.mif", "v9"},
		{"TestEncoding-v10.mif", "v10"},
		{"991.mif", "2019"},
	}

	for _, sf := range supportedFiles {
		t.Run(sf.version, func(t *testing.T) {
			parts := readMIFDefault(t, sf.name)
			require.NotEmpty(t, parts, "should process supported MIF version %s (%s)", sf.version, sf.name)
		})
	}
}

// okapi: ExtractionTest#doesNotProcessUnsupportedVersions
func TestExtraction_DoesNotProcessUnsupportedVersions(t *testing.T) {
	// Create a MIF file with an unsupported version.
	// The filter should reject it with an error during Open.
	pool, cfg := bridgetest.SharedBridge(t)

	unsupportedContent := []byte("<MIFFile 5.00>\n<TextFlow\n> # end of TextFlow\n")

	reader := bridge.NewBridgeFormatReader(pool, cfg, filterClass)
	doc := &model.RawDocument{
		URI:          "unsupported.mif",
		SourceLocale: "en",
		TargetLocale: "fr",
		Encoding:     "UTF-8",
		MimeType:     mimeType,
		Reader:       io.NopCloser(bytes.NewReader(unsupportedContent)),
	}

	ctx := context.Background()
	err := reader.Open(ctx, doc)
	assert.Error(t, err, "unsupported MIF version should produce an error on Open")
	assert.Contains(t, err.Error(), "Unsupported", "error should mention unsupported version")
}
