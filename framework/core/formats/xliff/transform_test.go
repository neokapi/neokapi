package xliff

import (
	"testing"

	"github.com/neokapi/neokapi/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOkapiXLIFFTransform_Registration(t *testing.T) {
	assert.True(t, config.DefaultTransforms.Has(
		config.OkapiFilterConfigKind("xliff"), config.FormatConfigKind("xliff")))
}

func TestOkapiXLIFFTransform_DropsOkapiOnlyParams(t *testing.T) {
	from := config.OkapiFilterConfigKind("xliff")
	to := config.FormatConfigKind("xliff")
	spec := map[string]any{
		"inlineCdata":            true,
		"bilingualMode":          true,
		"generateTarget":         true,
		"escapeGT":               true,
		"addTargetLanguage":      true,
		"copySource":             true,
		"allowEmptyTargets":      true,
		"overrideTargetLanguage": true,
		"useCustomParser":        false,
		"protectApproved":        true,
		"cdataSubfilter":         "okf_html",
		"pcdataSubfilter":        "okf_html",
		"useCodeFinder":          true,
		"codeFinderRules":        "pattern",
		"escapingOutput":         false,
		"useSdlProperties":       true,
		"segmentationType":       "ICU4J",
		"useSegSource":           true,
	}

	result, err := config.DefaultTransforms.Transform(from, to, spec)
	require.NoError(t, err)

	// All Okapi-only params should be dropped
	for key := range spec {
		assert.Nil(t, result[key], "param %q should be dropped", key)
	}
}

func TestOkapiXLIFFTransform_EmptySpec(t *testing.T) {
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("xliff"), config.FormatConfigKind("xliff"), map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestOkapiXLIFFTransform_UnknownParamsPassThrough(t *testing.T) {
	from := config.OkapiFilterConfigKind("xliff")
	to := config.FormatConfigKind("xliff")
	spec := map[string]any{
		"customParam":    "value",
		"generateTarget": true, // should be dropped
	}

	result, err := config.DefaultTransforms.Transform(from, to, spec)
	require.NoError(t, err)

	assert.Equal(t, "value", result["customParam"])
	assert.Nil(t, result["generateTarget"])
}
