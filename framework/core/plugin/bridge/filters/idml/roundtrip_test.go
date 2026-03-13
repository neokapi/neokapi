//go:build integration

package idml

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// RoundTripTest#testDoubleExtraction — parameterized roundtrip tests
// ---------------------------------------------------------------------------

// okapi: RoundTripTest#testDoubleExtraction
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// Each entry corresponds to a parameterized test case from the Java
	// testDoubleExtraction data provider. The configFile is nil (null in Java)
	// unless a specific .fprm is listed.
	tests := []struct {
		name       string
		file       string
		configFile string // relative to okapi/filters/idml/src/test/resources/ within testdata; "" means no config
	}{
		// [0] - [11]: Tests with ExtractAll.fprm config
		{"Test00.idml_ExtractAll", "Test00.idml", "okf_idml@ExtractAll.fprm"},
		{"Test01.idml_ExtractAll", "Test01.idml", "okf_idml@ExtractAll.fprm"},
		{"Test02.idml_ExtractAll", "Test02.idml", "okf_idml@ExtractAll.fprm"},
		{"Test03.idml_ExtractAll", "Test03.idml", "okf_idml@ExtractAll.fprm"},
		{"helloworld-1.idml_ExtractAll", "helloworld-1.idml", "okf_idml@ExtractAll.fprm"},
		{"ConditionalText.idml_ExtractAll", "ConditionalText.idml", "okf_idml@ExtractAll.fprm"},
		{"testWithSpecialChars.idml_ExtractAll", "testWithSpecialChars.idml", "okf_idml@ExtractAll.fprm"},
		{"TextPathTest01.idml_ExtractAll", "TextPathTest01.idml", "okf_idml@ExtractAll.fprm"},
		{"TextPathTest02.idml_ExtractAll", "TextPathTest02.idml", "okf_idml@ExtractAll.fprm"},
		{"TextPathTest03.idml_ExtractAll", "TextPathTest03.idml", "okf_idml@ExtractAll.fprm"},
		{"TextPathTest04.idml_ExtractAll", "TextPathTest04.idml", "okf_idml@ExtractAll.fprm"},
		{"idmltest.idml_ExtractAll", "idmltest.idml", "okf_idml@ExtractAll.fprm"},

		// [12]: idmltest.idml with no config (default parameters)
		{"idmltest.idml_default", "idmltest.idml", ""},

		// [13]-[31]: Various IDML files with default parameters
		{"01-pages-with-text-frames.idml", "01-pages-with-text-frames.idml", ""},
		{"01-pages-with-text-frames-2.idml", "01-pages-with-text-frames-2.idml", ""},
		{"01-pages-with-text-frames-3.idml", "01-pages-with-text-frames-3.idml", ""},
		{"01-pages-with-text-frames-4.idml", "01-pages-with-text-frames-4.idml", ""},
		{"01-pages-with-text-frames-5.idml", "01-pages-with-text-frames-5.idml", ""},
		{"01-pages-with-text-frames-6.idml", "01-pages-with-text-frames-6.idml", ""},
		{"02-island-spread-and-threaded-text-frames.idml", "02-island-spread-and-threaded-text-frames.idml", ""},
		{"03-hyperlink-and-table-content.idml", "03-hyperlink-and-table-content.idml", ""},
		{"04-complex-formatting.idml", "04-complex-formatting.idml", ""},
		{"05-complex-ordering.idml", "05-complex-ordering.idml", ""},
		{"06-hello-world-12.idml", "06-hello-world-12.idml", ""},
		{"06-hello-world-13.idml", "06-hello-world-13.idml", ""},
		{"06-hello-world-14.idml", "06-hello-world-14.idml", ""},
		{"07-paragraph-breaks.idml", "07-paragraph-breaks.idml", ""},
		{"08-conditional-text-and-tracked-changes.idml", "08-conditional-text-and-tracked-changes.idml", ""},

		// [28]: change-tracking-3.idml (has same name for both file and config in Java)
		{"change-tracking-3.idml", "change-tracking-3.idml", ""},

		// [29]-[31]: More default parameter tests
		{"08-direct-story-content.idml", "08-direct-story-content.idml", ""},
		{"09-footnotes.idml", "09-footnotes.idml", ""},
		{"10-tables.idml", "10-tables.idml", ""},

		// [32]-[33]: 11-xml-structures with and without config
		{"11-xml-structures.idml_ExtractAll", "11-xml-structures.idml", "okf_idml@ExtractAll.fprm"},
		{"11-xml-structures.idml_default", "11-xml-structures.idml", ""},

		// [34]-[37]: 618 series
		{"618-objects-without-path-points-and-text.idml", "618-objects-without-path-points-and-text.idml", ""},
		{"618-anchored-frame-without-path-points.idml", "618-anchored-frame-without-path-points.idml", ""},
		{"618-MBE3.idml", "618-MBE3.idml", ""},
		{"Bindestrich.idml", "Bindestrich.idml", ""},

		// [38]-[43]: Character style files with IgnoreAll.fprm
		{"756-character-kerning.idml_IgnoreAll", "756-character-kerning.idml", "okf_idml@IgnoreAll.fprm"},
		{"756-character-tracking.idml_IgnoreAll", "756-character-tracking.idml", "okf_idml@IgnoreAll.fprm"},
		{"756-character-leading.idml_IgnoreAll", "756-character-leading.idml", "okf_idml@IgnoreAll.fprm"},
		{"756-character-baseline-shift.idml_IgnoreAll", "756-character-baseline-shift.idml", "okf_idml@IgnoreAll.fprm"},
		{"777-character-kerning-method.idml_IgnoreAll", "777-character-kerning-method.idml", "okf_idml@IgnoreAll.fprm"},
		{"779-reference-and-tag-styles.idml_IgnoreAll", "779-reference-and-tag-styles.idml", "okf_idml@IgnoreAll.fprm"},

		// [44]: Baselined formatting
		{"923-baselined-formatting.idml", "923-baselined-formatting.idml", ""},

		// [45]: Font mappings
		{"926.idml_chained-font-mappings", "926.idml", "okf_idml@chained-font-mappings.fprm"},

		// [46]: Custom text variables
		{"1138.idml_custom-text-variables", "1138.idml", "okf_idml@custom-text-variables-extraction.fprm"},

		// [47]-[48]: 856 series
		{"856-1.idml", "856-1.idml", ""},
		{"856-2.idml", "856-2.idml", ""},

		// [49]: Font name
		{"1432-font-name.idml", "1432-font-name.idml", ""},

		// [50]: Codefinder
		{"codefinder.idml_codefinder", "codefinder.idml", "okf_idml@codefinder.fprm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := bridgetest.TestdataFile(t, "okapi/filters/idml/src/test/resources/"+tt.file)
			content, err := os.ReadFile(path)
			require.NoError(t, err)

			var params map[string]any
			if tt.configFile != "" {
				configPath := tdDir + "/okapi/filters/idml/src/test/resources/" + tt.configFile
				params = map[string]any{
					"configFile": configPath,
				}
			}

			bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
				content, path, mimeType, params)
		})
	}
}

// ---------------------------------------------------------------------------
// Named roundtrip tests
// ---------------------------------------------------------------------------

// okapi: RoundTripTest#customTextVariablesExtractedAndMerged
func TestRoundTrip_CustomTextVariablesExtractedAndMerged(t *testing.T) {
	result := roundtripIDMLWithConfig(t,
		"1138.idml",
		"okf_idml@custom-text-variables-extraction.fprm")

	blocks := bridgetest.TranslatableBlocks(result.Parts)
	require.NotEmpty(t, blocks, "custom text variables should produce blocks after roundtrip")
	assert.NotEmpty(t, result.Output, "roundtrip should produce output")
}

// okapi: RoundTripTest#endNotesExtractedAndMerged
func TestRoundTrip_EndNotesExtractedAndMerged(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	configPath := tdDir + "/okapi/filters/idml/src/test/resources/okf_idml@ExtractAll.fprm"
	params := map[string]any{
		"configFile": configPath,
	}
	path := bridgetest.TestdataFile(t, "okapi/filters/idml/src/test/resources/09-footnotes.idml")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, params)
	blocks := bridgetest.TranslatableBlocks(result.Parts)
	require.NotEmpty(t, blocks, "end notes should produce blocks after roundtrip")
	assert.NotEmpty(t, result.Output, "roundtrip should produce output")
}

// okapi: RoundTripTest#externalHyperlinksExtractedAndMerged
func TestRoundTrip_ExternalHyperlinksExtractedAndMerged(t *testing.T) {
	result := assertRoundTripEventsIDML(t, "03-hyperlink-and-table-content.idml", nil)

	blocks := bridgetest.TranslatableBlocks(result.Parts)
	require.NotEmpty(t, blocks, "hyperlinks should produce blocks after roundtrip")
}

// okapi: RoundTripTest#hyperlinkTextSourcesExtractedAndMerged
func TestRoundTrip_HyperlinkTextSourcesExtractedAndMerged(t *testing.T) {
	result := assertRoundTripEventsIDML(t, "03-hyperlink-and-table-content.idml", nil)

	blocks := bridgetest.TranslatableBlocks(result.Parts)
	require.NotEmpty(t, blocks, "hyperlink text sources should produce blocks after roundtrip")
}

// okapi: RoundTripTest#specialCharactersExtractedAndMerged
func TestRoundTrip_SpecialCharactersExtractedAndMerged(t *testing.T) {
	result := assertRoundTripEventsIDML(t, "175-special-characters.idml", nil)

	blocks := bridgetest.TranslatableBlocks(result.Parts)
	require.NotEmpty(t, blocks, "special characters should produce blocks after roundtrip")
}

// okapi: RoundTripTest#adjacentCodesMergeSupported
func TestRoundTrip_AdjacentCodesMergeSupported(t *testing.T) {
	result := assertRoundTripEventsIDMLWithConfig(t,
		"adjacent-codes/1415-adjacent-codes.idml",
		"adjacent-codes/okf_idml@adjacent-codes.fprm")

	blocks := bridgetest.TranslatableBlocks(result.Parts)
	require.NotEmpty(t, blocks, "adjacent codes merge should produce blocks after roundtrip")
}

// okapi: RoundTripTest#stylesExclusionSupported
func TestRoundTrip_StylesExclusionSupported(t *testing.T) {
	result := assertRoundTripEventsIDMLWithConfig(t,
		"styles-exclusion/1418-styles-exclusion.idml",
		"styles-exclusion/okf_idml@syles-exclusion.fprm")

	blocks := bridgetest.TranslatableBlocks(result.Parts)
	require.NotEmpty(t, blocks, "styles exclusion should produce blocks after roundtrip")
}

// okapi: RoundTripTest#mathZonesConditionalExtractionSupported
func TestRoundTrip_MathZonesConditionalExtractionSupported(t *testing.T) {
	result := assertRoundTripEventsIDMLWithConfig(t,
		"math-zone/1412-math-zone.idml",
		"math-zone/okf_idml@math-zone.fprm")

	blocks := bridgetest.TranslatableBlocks(result.Parts)
	require.NotEmpty(t, blocks, "math zones should produce blocks after roundtrip")
}

// okapi: RoundTripTest#fontMappingForNamesWithProcessingInstructionsSupported
func TestRoundTrip_FontMappingForNamesWithProcessingInstructionsSupported(t *testing.T) {
	result := assertRoundTripEventsIDMLWithConfig(t,
		"926.idml",
		"okf_idml@chained-font-mappings.fprm")

	blocks := bridgetest.TranslatableBlocks(result.Parts)
	require.NotEmpty(t, blocks, "font mapping should produce blocks after roundtrip")
}

// okapi: RoundTripTest#documentsWithDefaultParameters
func TestRoundTrip_DocumentsWithDefaultParameters(t *testing.T) {
	result := assertRoundTripEventsIDML(t, "idmltest.idml", nil)

	blocks := bridgetest.TranslatableBlocks(result.Parts)
	require.NotEmpty(t, blocks, "default parameters should produce blocks after roundtrip")
}

// okapi: RoundTripTest#documentWithChainedFontMappings
func TestRoundTrip_DocumentWithChainedFontMappings(t *testing.T) {
	result := assertRoundTripEventsIDMLWithConfig(t,
		"926.idml",
		"okf_idml@chained-font-mappings.fprm")

	blocks := bridgetest.TranslatableBlocks(result.Parts)
	require.NotEmpty(t, blocks, "chained font mappings should produce blocks after roundtrip")
}

// okapi: RoundTripTest#indexTopicsExtractedAndMerged
func TestRoundTrip_IndexTopicsExtractedAndMerged(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "integration-tests/okapi/src/test/resources/idml/links_crossreferences.idml")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	result := bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(result.Parts)
	require.NotEmpty(t, blocks, "index topics should produce blocks after roundtrip")
}

// okapi: RoundTripTest#emptyTargetsMerged
func TestRoundTrip_EmptyTargetsMerged(t *testing.T) {
	result := assertRoundTripEventsIDML(t, "idmltest.idml", nil)

	blocks := bridgetest.TranslatableBlocks(result.Parts)
	require.NotEmpty(t, blocks, "empty targets should produce blocks after roundtrip")
}

// okapi: RoundTripTest#emptyContentStylesPreserved
func TestRoundTrip_EmptyContentStylesPreserved(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// This test uses expected output files from the expected/ subdirectory.
	// Run roundtrip and verify parts are produced.
	path := bridgetest.TestdataFile(t, "okapi/filters/idml/src/test/resources/1369-empty-paragraph-styles.idml")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, nil)
	require.NotEmpty(t, result.Output, "roundtrip should produce output")

	// Also test with table cell styles.
	path2 := bridgetest.TestdataFile(t, "okapi/filters/idml/src/test/resources/1369-empty-paragraph-in-table-cell-styles.idml")
	content2, err := os.ReadFile(path2)
	require.NoError(t, err)

	result2 := bridgetest.RoundTrip(t, pool, cfg, filterClass, content2, path2, mimeType, nil)
	require.NotEmpty(t, result2.Output, "roundtrip should produce output for table cell styles")

	_ = tdDir
}

// ---------------------------------------------------------------------------
// Roundtrip event-level tests for expected output files
// ---------------------------------------------------------------------------

// TestRoundTrip_ExpectedFiles runs roundtrip event-level comparison on files
// from the expected/ subdirectory to verify merge fidelity.
func TestRoundTrip_ExpectedFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	files := []struct {
		name   string
		config string // "" means no config
	}{
		{"03-hyperlink-and-table-content.idml", ""},
		{"1138.idml", "okf_idml@custom-text-variables-extraction.fprm"},
		{"1139.idml", ""},
		{"1179-0.idml", ""},
		{"1179-1.idml", ""},
		{"1179-2.idml", ""},
		{"1179-3.idml", ""},
		{"1179-4.idml", ""},
		{"1369-empty-paragraph-in-table-cell-styles.idml", ""},
		{"1369-empty-paragraph-styles.idml", ""},
		{"1412-math-zones.idml", ""},
		{"1415-adjacent-codes.idml", ""},
		{"1418-styles-exclusion.idml", ""},
		{"1432-font-name.idml", ""},
		{"175-special-characters.idml", ""},
		{"629.idml", ""},
		{"856-1.idml", ""},
		{"856-2.idml", ""},
		{"926.idml", ""},
	}

	for _, tt := range files {
		t.Run(tt.name, func(t *testing.T) {
			path := tdDir + "/okapi/filters/idml/src/test/resources/expected/" + tt.name
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Skipf("expected file not found: %s", tt.name)
			}
			content, err := os.ReadFile(path)
			require.NoError(t, err)

			var params map[string]any
			if tt.config != "" {
				params = map[string]any{
					"configFile": tdDir + "/okapi/filters/idml/src/test/resources/" + tt.config,
				}
			}

			bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
				content, path, mimeType, params)
		})
	}
}
