package json

import (
	"testing"

	"github.com/gokapi/gokapi/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOkapiJSONTransform_Registration(t *testing.T) {
	assert.True(t, config.DefaultTransforms.Has(
		config.OkapiFilterConfigKind("json"), config.FormatConfigKind("json")))
}

func TestOkapiJSONTransform_DropsOkapiOnlyParams(t *testing.T) {
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
	assert.Nil(t, result["escapeExtendedChars"])
	assert.Nil(t, result["bom"])

	// Shared params kept
	assert.Equal(t, true, result["extractAllPairs"])
	assert.Equal(t, true, result["useFullKeyPath"])
}

func TestOkapiJSONTransform_EmptySpec(t *testing.T) {
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("json"), config.FormatConfigKind("json"), map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestOkapiJSONTransform_AllParamsPassThrough(t *testing.T) {
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
		assert.Equal(t, val, result[key], "param %s should pass through", key)
	}
}
