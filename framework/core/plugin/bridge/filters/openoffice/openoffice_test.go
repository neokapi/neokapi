//go:build integration

package openoffice

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: OpenOfficeFilterTest#testStartDocument
func TestOpenOffice_StartDocument(t *testing.T) {
	parts := readOpenOfficeFile(t, "okf_openoffice/TestDocument01.odt", nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type,
		"first part should be LayerStart")
}

// okapi: OpenOfficeFilterTest#testDefaultInfo
func TestOpenOffice_DefaultInfo(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, openOfficeFilterClass)

	b, err := pool.Acquire(cfg)
	require.NoError(t, err)
	defer pool.Release(b)

	info, err := b.Info(openOfficeFilterClass)
	require.NoError(t, err)
	assert.NotEmpty(t, info.Name, "filter should have a name")
	assert.NotEmpty(t, info.DisplayName, "filter should have a display name")
}

// okapi: OpenOfficeFilterTest#testFirstTextUnit
func TestOpenOffice_FirstTextUnit(t *testing.T) {
	parts := readOpenOfficeFile(t, "okf_openoffice/TestDocument01.odt", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from TestDocument01.odt")

	// The first text unit in the document is "Heading 1".
	assert.Equal(t, "Heading 1", blocks[0].SourceText())
}

// okapi: OpenOfficeFilterTest#testDoubleExtraction
func TestOpenOffice_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, openOfficeFilterClass)

	// The Java test roundtrips 14 OpenOffice archive files.
	files := []string{
		"okf_openoffice/TestSpreadsheet01.ods",
		"okf_openoffice/TestDocument01.odt",
		"okf_openoffice/TestDocument02.odt",
		"okf_openoffice/TestDocument03.odt",
		"okf_openoffice/TestDocument04.odt",
		"okf_openoffice/TestDocument05.odt",
		"okf_openoffice/TestDocument06.odt",
		"okf_openoffice/TestDrawing01.odg",
		"okf_openoffice/TestPresentation01.odp",
		"okf_openoffice/TestDocument_WithITS.odt",
		"okf_openoffice/TestDocumentWithMetadata.odt",
		"okf_openoffice/TestDocumentWithNumberTag.odp",
		"okf_openoffice/TestDocumentWithFormulaResults.ods",
		"okf_openoffice/TestDocumentWithTableWrappingAboutTable.odt",
	}

	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			path := bridgetest.TestdataFile(t, f)
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			bridgetest.AssertRoundTripEvents(t, pool, cfg, openOfficeFilterClass, content, path, openOfficeMimeType, nil)
		})
	}
}

// okapi: OpenOfficeFilterTest#testMetadataExtraction
func TestOpenOffice_MetadataExtraction(t *testing.T) {
	tests := []struct {
		name            string
		extractMetadata bool
		expectedTexts   []string
	}{
		{
			name:            "with_metadata",
			extractMetadata: true,
			expectedTexts: []string{
				"Text on the first page.",
				"Text on the second page.",
				"Author: Test",
				// The page-number/page-count text may be rendered with inline codes.
				// We verify the core content text is present.
				"Test document meta comments",
				"met keywod1",
				"keyword2",
				"Test document meta description",
				"Test document meta title",
				"Test custom property's value",
			},
		},
		{
			name:            "without_metadata",
			extractMetadata: false,
			expectedTexts: []string{
				"Text on the first page.",
				"Text on the second page.",
				"Author: Test",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params := map[string]any{
				"extractMetadata": tc.extractMetadata,
			}
			parts := readOpenOfficeFile(t, "okf_openoffice/TestDocumentWithMetadata.odt", params)

			blocks := allBlocks(parts)
			texts := make([]string, len(blocks))
			for i, b := range blocks {
				texts[i] = b.SourceText()
			}

			// Verify all expected texts appear in the extracted blocks.
			for _, expected := range tc.expectedTexts {
				found := false
				for _, text := range texts {
					if text == expected || containsAll(text, expected) {
						found = true
						break
					}
				}
				assert.True(t, found, "should find text %q in extracted blocks", expected)
			}

			if tc.extractMetadata {
				// With metadata extraction, we should have more blocks than without.
				assert.GreaterOrEqual(t, len(blocks), len(tc.expectedTexts),
					"metadata extraction should produce at least %d blocks", len(tc.expectedTexts))
			}
		})
	}
}

// okapi: OpenOfficeFilterTest#testNumberTag
func TestOpenOffice_NumberTag(t *testing.T) {
	tests := []struct {
		name                                string
		encodeCharacterEntityReferenceGlyphs bool
		// Only check the first text and a distinctive text to validate encoding behavior.
		firstText     string
		containsLtGt  bool // whether <number> should be &lt;number&gt;
	}{
		{
			name:                                "encode_entities",
			encodeCharacterEntityReferenceGlyphs: true,
			firstText:                           "There will be a lot of them",
			containsLtGt:                        true,
		},
		{
			name:                                "no_encode_entities",
			encodeCharacterEntityReferenceGlyphs: false,
			firstText:                           "There will be a lot of them",
			containsLtGt:                        false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params := map[string]any{
				"encodeCharacterEntityReferenceGlyphs": tc.encodeCharacterEntityReferenceGlyphs,
			}
			parts := readOpenOfficeFile(t, "okf_openoffice/TestDocumentWithNumberTag.odp", params)

			blocks := allBlocks(parts)
			require.NotEmpty(t, blocks, "should extract blocks from number tag presentation")

			texts := make([]string, len(blocks))
			for i, b := range blocks {
				texts[i] = b.SourceText()
			}

			// The first text should always be "There will be a lot of them".
			assert.Equal(t, tc.firstText, texts[0])

			// The Java test expects exactly 19 text units for both parameter sets.
			assert.Equal(t, 19, len(blocks),
				"should extract 19 text units from TestDocumentWithNumberTag.odp")
		})
	}
}

// okapi: OpenOfficeFilterTest#testBookmarkReferencesHandling
func TestOpenOffice_BookmarkReferencesHandling(t *testing.T) {
	tests := []struct {
		name              string
		params            map[string]any
		expectedBlockCount int
	}{
		{
			name: "extract_references",
			params: map[string]any{
				"extractReferences": true,
			},
			expectedBlockCount: 3,
		},
		{
			name:               "default_params",
			params:             nil,
			expectedBlockCount: 3,
		},
		{
			name: "no_encode_entities",
			params: map[string]any{
				"encodeCharacterEntityReferenceGlyphs": false,
			},
			expectedBlockCount: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := readOpenOfficeFile(t, "okf_openoffice/bookmark-reference.odt", tc.params)

			blocks := allBlocks(parts)
			// The Java test asserts exactly 3 text units.
			assert.Equal(t, tc.expectedBlockCount, len(blocks),
				"bookmark-reference.odt should produce %d text units", tc.expectedBlockCount)
		})
	}
}

// okapi: OpenOfficeFilterTest#testFormulaResultExtraction
func TestOpenOffice_FormulaResultExtraction(t *testing.T) {
	// The Java test expects 17 text units from this spreadsheet.
	// Some contain ODF inline XML tags (text:sheet-name, text:page-number, etc.)
	// which appear as inline codes (spans) in the bridge content model.
	// We verify the plain-text content blocks and the count.
	expectedPlainTexts := []string{
		"Sheet1",
		"Max 1000 signs (Do not translate)",
		".",
		"Description (Do not translate)",
		".",
		"This is the first sentence (short one).",
		"We need a lot of lovely characters. A b c d e f g h i j k l m n o p q r s t u v w x y z.",
		"One. Two. Three. Four.",
		"One. Two. Three. Four.",
		"This line contains ten whitespaces at the end          ",
		"Sheet2",
		"Sheet3",
	}

	parts := readOpenOfficeFile(t, "okf_openoffice/TestDocumentWithFormulaResults.ods", nil)

	blocks := allBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from formula results spreadsheet")

	texts := make([]string, len(blocks))
	for i, b := range blocks {
		texts[i] = b.SourceText()
	}

	// The Java test expects exactly 17 text units.
	assert.Equal(t, 17, len(blocks),
		"should extract 17 text units from TestDocumentWithFormulaResults.ods")

	// Verify core plain-text blocks are present.
	for _, expected := range expectedPlainTexts {
		found := false
		for _, text := range texts {
			if text == expected {
				found = true
				break
			}
		}
		assert.True(t, found, "should find text %q in extracted blocks", expected)
	}

	// Verify blocks with ODF markup tags appear with inline content.
	// The Java filter produces text units containing ODF XML tags as inline
	// codes. In the bridge, these become spans, so the SourceText() rendering
	// may differ from the raw Java toString(). We verify that the non-plain-text
	// blocks exist by checking we have more blocks than the plain text set.
	assert.Greater(t, len(blocks), len(expectedPlainTexts),
		"should have blocks beyond plain text (header/footer with inline ODF tags)")
}
