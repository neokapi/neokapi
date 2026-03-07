package csv_test

import (
	"testing"

	"github.com/gokapi/gokapi/core/config"
	_ "github.com/gokapi/gokapi/core/formats/csv" // registers transform
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransform_FieldDelimiter_Tab(t *testing.T) {
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("commaseparatedvalues"),
		config.FormatConfigKind("csv"),
		map[string]any{"fieldDelimiter": "\\t"},
	)
	require.NoError(t, err)
	assert.Equal(t, "\t", result["separator"])
}

func TestTransform_FieldDelimiter_Comma(t *testing.T) {
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("commaseparatedvalues"),
		config.FormatConfigKind("csv"),
		map[string]any{"fieldDelimiter": ","},
	)
	require.NoError(t, err)
	assert.Equal(t, ",", result["separator"])
}

func TestTransform_FieldDelimiter_Space(t *testing.T) {
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("commaseparatedvalues"),
		config.FormatConfigKind("csv"),
		map[string]any{"fieldDelimiter": "\\s"},
	)
	require.NoError(t, err)
	assert.Equal(t, " ", result["separator"])
}

func TestTransform_ColumnNamesLineNum(t *testing.T) {
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("commaseparatedvalues"),
		config.FormatConfigKind("csv"),
		map[string]any{"columnNamesLineNum": float64(2)},
	)
	require.NoError(t, err)
	assert.Equal(t, 2, result["columnNamesRow"])
	assert.Equal(t, true, result["hasHeader"])
}

func TestTransform_ValuesStartLineNum(t *testing.T) {
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("commaseparatedvalues"),
		config.FormatConfigKind("csv"),
		map[string]any{"valuesStartLineNum": float64(3)},
	)
	require.NoError(t, err)
	assert.Equal(t, 3, result["valuesStartRow"])
}

func TestTransform_SourceColumns(t *testing.T) {
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("commaseparatedvalues"),
		config.FormatConfigKind("csv"),
		map[string]any{"sourceColumns": "1,3"},
	)
	require.NoError(t, err)
	cols, ok := result["translatableColumns"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{0, 2}, cols) // 1-based to 0-based
}

func TestTransform_SourceIdColumns(t *testing.T) {
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("commaseparatedvalues"),
		config.FormatConfigKind("csv"),
		map[string]any{"sourceIdColumns": "1"},
	)
	require.NoError(t, err)
	cols, ok := result["keyColumns"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{0}, cols)
}

func TestTransform_CommentColumns(t *testing.T) {
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("commaseparatedvalues"),
		config.FormatConfigKind("csv"),
		map[string]any{"commentColumns": "3"},
	)
	require.NoError(t, err)
	cols, ok := result["commentColumns"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{2}, cols)
}

func TestTransform_TrimMode(t *testing.T) {
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("commaseparatedvalues"),
		config.FormatConfigKind("csv"),
		map[string]any{"trimMode": "ALL"},
	)
	require.NoError(t, err)
	assert.Equal(t, true, result["trimValues"])
}

func TestTransform_TrimModeNone(t *testing.T) {
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("commaseparatedvalues"),
		config.FormatConfigKind("csv"),
		map[string]any{"trimMode": "NONE"},
	)
	require.NoError(t, err)
	_, hasTrim := result["trimValues"]
	assert.False(t, hasTrim, "NONE should not set trimValues")
}

func TestTransform_DropsOkapiOnlyParams(t *testing.T) {
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("commaseparatedvalues"),
		config.FormatConfigKind("csv"),
		map[string]any{
			"textQualifier":    "\"",
			"removeQualifiers": true,
			"addQualifiers":    true,
			"escapingMode":     "DOUBLEDQUALIFIER",
			"targetColumns":    "2",
			"targetLanguages":  "fr",
			"parametersClass":  "csv.Parameters",
			"useCodeFinder":    true,
			"codeFinderRules":  "pattern",
			"sendColumnsV":     true,
			"recordIdColumns":  "1",
			"bom":              true,
			"subfilter":        "html",
		},
	)
	require.NoError(t, err)
	// All these should be dropped
	assert.Empty(t, result)
}

func TestTransform_TableKind(t *testing.T) {
	// okf_table kind should also be registered
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("table"),
		config.FormatConfigKind("csv"),
		map[string]any{"fieldDelimiter": ";"},
	)
	require.NoError(t, err)
	assert.Equal(t, ";", result["separator"])
}

func TestTransform_EmptySpec(t *testing.T) {
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("commaseparatedvalues"),
		config.FormatConfigKind("csv"),
		map[string]any{},
	)
	require.NoError(t, err)
	assert.Empty(t, result)
}
