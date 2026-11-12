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

	assert.Equal(t, true, result["translate_doc_properties"])
	assert.Equal(t, true, result["translate_hidden_text"])
	assert.Equal(t, false, result["aggressive_cleanup"])
	assert.Equal(t, true, result["tab_as_character"])
	assert.Equal(t, false, result["translate_slide_notes"])
	assert.Equal(t, []string{"FF0000", "00FF00"}, result["exclude_colors"])
	assert.Equal(t, []string{"CodeBlock"}, result["exclude_styles"])
	assert.Equal(t, []string{"A", "B"}, result["excluded_columns"])
	assert.Equal(t, []int{1, 3, 5}, result["included_slides"])
	assert.Equal(t, "\\n", result["line_separator_replacement"])
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

	assert.Equal(t, true, result["translate_doc_properties"])
	assert.Equal(t, false, result["translate_slide_notes"])
	assert.Equal(t, true, result["translate_slide_masters"])
	assert.Equal(t, true, result["translate_hidden_slides"])
	assert.Equal(t, true, result["translate_sheet_names"])
	assert.Equal(t, []string{"Heading1"}, result["exclude_styles"])
	assert.Equal(t, []string{"Normal"}, result["include_styles"])
	assert.Equal(t, []string{"C"}, result["excluded_columns"])
	assert.Equal(t, []string{"Sheet2"}, result["excluded_sheets"])
	assert.Equal(t, []any{float64(1), float64(2)}, result["included_slides"])
	assert.Equal(t, "\n", result["line_separator_replacement"])
	assert.Equal(t, true, result["replace_line_separator"])
	assert.Equal(t, false, result["aggressive_cleanup"])
	assert.Equal(t, true, result["tab_as_character"])
	assert.Equal(t, true, result["translate_charts"])
	assert.Equal(t, true, result["translate_diagrams"])
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
	assert.Equal(t, true, result["translate_doc_properties"])
	assert.Equal(t, "HYPERLINK", result["complex_field_definitions_to_extract"]) // key dropped its ts- prefix
	assert.Equal(t, "some-mapping", result["font_mappings"])                     // passed through
	assert.Equal(t, true, result["extract_run_fonts_info"])                      // passed through

	// codeFinder composite object → extracted into flat params
	assert.Equal(t, true, result["use_code_finder"])
	assert.Equal(t, []string{"<br>"}, result["code_finder_rules"])
	assert.Nil(t, result["code_finder"]) // composite object itself not passed through

	// Dropped (Okapi-only, no native equivalent)
	assert.Nil(t, result["translate_excel_drawings"])
	// Word Style Optimisation was removed (native is faithful); the Okapi
	// param is dropped silently rather than passed through.
	assert.Nil(t, result["optimise_word_styles"])

	// Renamed keys should not appear under old names
	assert.Nil(t, result["ts_complex_field_definitions_to_extract"])
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
	assert.Equal(t, true, result["use_code_finder"])
	assert.Equal(t, []string{"\\{[0-9]+\\}"}, result["code_finder_rules"])
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
	assert.Equal(t, "someValue", result["future_param"])
}
