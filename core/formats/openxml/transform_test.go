package openxml

import (
	"testing"

	"github.com/neokapi/neokapi/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOkapiOpenXMLTransform_Registration(t *testing.T) {
	assert.True(t, config.DefaultTransforms.Has(
		config.OkapiFilterConfigKind("openxml"), config.FormatConfigKind("openxml")))
}

func TestOkapiOpenXMLTransform_DirectMappings(t *testing.T) {
	from := config.OkapiFilterConfigKind("openxml")
	to := config.FormatConfigKind("openxml")
	spec := map[string]any{
		"translateDocProperties":   true,
		"translateHiddenText":      true,
		"aggressiveCleanup":        false,
		"tabAsCharacter":           true,
		"translateSlideNotes":      false,
		"excludeColors":            []string{"FF0000", "00FF00"},
		"excludeStyles":            []string{"CodeBlock"},
		"excludedColumns":          []string{"A", "B"},
		"includedSlides":           []int{1, 3, 5},
		"lineSeparatorReplacement": "\\n",
	}

	result, err := config.DefaultTransforms.Transform(from, to, spec)
	require.NoError(t, err)

	assert.Equal(t, true, result["translateDocProperties"])
	assert.Equal(t, true, result["translateHiddenText"])
	assert.Equal(t, false, result["aggressiveCleanup"])
	assert.Equal(t, true, result["tabAsCharacter"])
	assert.Equal(t, false, result["translateSlideNotes"])
	assert.Equal(t, []string{"FF0000", "00FF00"}, result["excludeColors"])
	assert.Equal(t, []string{"CodeBlock"}, result["excludeStyles"])
	assert.Equal(t, []string{"A", "B"}, result["excludedColumns"])
	assert.Equal(t, []int{1, 3, 5}, result["includedSlides"])
	assert.Equal(t, "\\n", result["lineSeparatorReplacement"])
}

func TestOkapiOpenXMLTransform_RenamedKeys(t *testing.T) {
	from := config.OkapiFilterConfigKind("openxml")
	to := config.FormatConfigKind("openxml")
	spec := map[string]any{
		"translateDocumentProperties": true,
		"extractNotes":                false,
		"extractMasterPages":          true,
		"extractHiddenSlides":         true,
		"translateExcelSheetNames":    true,
		"tsExcludedStyles":            []string{"Heading1"},
		"tsIncludedStyles":            []string{"Normal"},
		"tsExcludedColumns":           []string{"C"},
		"tsExcludedSheets":            []string{"Sheet2"},
		"tsIncludedSlides":            []any{float64(1), float64(2)},
		"tsLineSeparatorReplacement":  "\n",
		"tsReplaceLineSeparator":      true,
		"tsAggressiveCleanup":         false,
		"tsTabAsCharacter":            true,
		"tsExtractChartStrings":       true,
		"tsExtractDiagramData":        true,
	}

	result, err := config.DefaultTransforms.Transform(from, to, spec)
	require.NoError(t, err)

	assert.Equal(t, true, result["translateDocProperties"])
	assert.Equal(t, false, result["translateSlideNotes"])
	assert.Equal(t, true, result["translateSlideMasters"])
	assert.Equal(t, true, result["translateHiddenSlides"])
	assert.Equal(t, true, result["translateSheetNames"])
	assert.Equal(t, []string{"Heading1"}, result["excludeStyles"])
	assert.Equal(t, []string{"Normal"}, result["includeStyles"])
	assert.Equal(t, []string{"C"}, result["excludedColumns"])
	assert.Equal(t, []string{"Sheet2"}, result["excludedSheets"])
	assert.Equal(t, []any{float64(1), float64(2)}, result["includedSlides"])
	assert.Equal(t, "\n", result["lineSeparatorReplacement"])
	assert.Equal(t, true, result["replaceLineSeparator"])
	assert.Equal(t, false, result["aggressiveCleanup"])
	assert.Equal(t, true, result["tabAsCharacter"])
	assert.Equal(t, true, result["translateCharts"])
	assert.Equal(t, true, result["translateDiagrams"])
}

func TestOkapiOpenXMLTransform_AdvancedParams(t *testing.T) {
	from := config.OkapiFilterConfigKind("openxml")
	to := config.FormatConfigKind("openxml")
	spec := map[string]any{
		"translateDocProperties":             true,
		"translateExcelDrawings":             true,
		"tsComplexFieldDefinitionsToExtract": "HYPERLINK",
		"fontMappings":                       "some-mapping",
		"optimiseWordStyles":                 true,
		"extractRunFontsInfo":                true,
		"codeFinder":                         map[string]any{"rules": []string{"<br>"}},
	}

	result, err := config.DefaultTransforms.Transform(from, to, spec)
	require.NoError(t, err)

	// Kept / transformed
	assert.Equal(t, true, result["translateDocProperties"])
	assert.Equal(t, "HYPERLINK", result["complexFieldDefinitionsToExtract"]) // renamed from ts-prefixed
	assert.Equal(t, "some-mapping", result["fontMappings"])                  // passed through
	assert.Equal(t, true, result["extractRunFontsInfo"])                     // passed through

	// codeFinder composite object → extracted into flat params
	assert.Equal(t, true, result["useCodeFinder"])
	assert.Equal(t, []string{"<br>"}, result["codeFinderRules"])
	assert.Nil(t, result["codeFinder"]) // composite object itself not passed through

	// Dropped (Okapi-only, no native equivalent)
	assert.Nil(t, result["translateExcelDrawings"])
	// Word Style Optimisation was removed (native is faithful); the Okapi
	// param is dropped silently rather than passed through.
	assert.Nil(t, result["optimiseWordStyles"])

	// Renamed keys should not appear under old names
	assert.Nil(t, result["tsComplexFieldDefinitionsToExtract"])
}

func TestOkapiOpenXMLTransform_CodeFinderWithExistingRules(t *testing.T) {
	// When both codeFinder object AND flat codeFinderRules are present,
	// the flat params should win (they come later in iteration or are already set)
	from := config.OkapiFilterConfigKind("openxml")
	to := config.FormatConfigKind("openxml")
	spec := map[string]any{
		"useCodeFinder":   true,
		"codeFinderRules": []string{"\\{[0-9]+\\}"},
	}

	result, err := config.DefaultTransforms.Transform(from, to, spec)
	require.NoError(t, err)
	assert.Equal(t, true, result["useCodeFinder"])
	assert.Equal(t, []string{"\\{[0-9]+\\}"}, result["codeFinderRules"])
}

func TestOkapiOpenXMLTransform_EmptySpec(t *testing.T) {
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("openxml"), config.FormatConfigKind("openxml"), map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestOkapiOpenXMLTransform_UnknownKeysPassThrough(t *testing.T) {
	from := config.OkapiFilterConfigKind("openxml")
	to := config.FormatConfigKind("openxml")
	spec := map[string]any{
		"futureParam": "someValue",
	}

	result, err := config.DefaultTransforms.Transform(from, to, spec)
	require.NoError(t, err)
	assert.Equal(t, "someValue", result["futureParam"])
}
