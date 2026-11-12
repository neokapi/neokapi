package json

import (
	"testing"

	"github.com/neokapi/neokapi/core/config"
	coreschema "github.com/neokapi/neokapi/core/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOkapiJSONTransform_Registration(t *testing.T) {
	t.Parallel()
	assert.True(t, config.DefaultTransforms.Has(
		config.OkapiFilterConfigKind("json"), config.FormatConfigKind("json")))
}

func TestOkapiJSONTransform_DropsOkapiOnlyParams(t *testing.T) {
	t.Parallel()
	from := config.OkapiFilterConfigKind("json")
	to := config.FormatConfigKind("json")
	spec := map[string]any{
		"extractAllPairs":     true,
		"useFullKeyPath":      true,
		"escapeExtendedChars": true,
		"bom":                 true,
	}

	result, err := config.DefaultTransforms.Transform(from, to, spec)
	require.NoError(t, err)

	// Okapi-only params dropped
	assert.Nil(t, result["escape_extended_chars"])
	assert.Nil(t, result["bom"])

	// Shared params kept
	assert.Equal(t, true, result["extract_all_pairs"])
	assert.Equal(t, true, result["use_full_key_path"])
}

func TestOkapiJSONTransform_EmptySpec(t *testing.T) {
	t.Parallel()
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("json"), config.FormatConfigKind("json"), map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestOkapiJSONTransform_AllParamsPassThrough(t *testing.T) {
	t.Parallel()
	from := config.OkapiFilterConfigKind("json")
	to := config.FormatConfigKind("json")
	spec := map[string]any{
		"extractAllPairs":          true,
		"exceptions":               "^_",
		"extractIsolatedStrings":   false,
		"useKeyAsName":             true,
		"useFullKeyPath":           false,
		"useLeadingSlashOnKeyPath": true,
		"escapeForwardSlashes":     true,
		"noteRules":                "^description$",
		"idRules":                  "^id$",
		"useIdStack":               false,
		"genericMetaRules":         "^meta",
		"extractionRules":          "^message$",
		"useCodeFinder":            true,
		"codeFinderRules":          []string{`<\/?[a-z]+>`},
	}

	result, err := config.DefaultTransforms.Transform(from, to, spec)
	require.NoError(t, err)

	for key, val := range spec {
		assert.Equal(t, val, result[coreschema.SnakeCase(key)], "param %s should pass through under its snake_case key", key)
	}
}
