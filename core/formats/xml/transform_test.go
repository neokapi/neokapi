// okapi-filter: xml
package xml

import (
	"testing"

	"github.com/neokapi/neokapi/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransform_OkapiXMLToXML(t *testing.T) {
	from := config.OkapiFilterConfigKind("xml")
	to := config.FormatConfigKind("xml")
	assert.True(t, config.DefaultTransforms.Has(from, to))

	input := map[string]any{
		"exclude_by_default": true,
		"elements": map[string]any{
			"p": map[string]any{
				"ruleTypes": []any{"TEXTUNIT"},
			},
		},
	}

	result, err := config.DefaultTransforms.Transform(from, to, input)
	require.NoError(t, err)
	assert.Equal(t, true, result["exclude_by_default"])
	assert.NotNil(t, result["elements"])
}

func TestTransform_OkapiXMLStreamToXML(t *testing.T) {
	from := config.OkapiFilterConfigKind("xmlstream")
	to := config.FormatConfigKind("xml")
	assert.True(t, config.DefaultTransforms.Has(from, to))

	result, err := config.DefaultTransforms.Transform(from, to, map[string]any{
		"exclude_by_default": false,
	})
	require.NoError(t, err)
	assert.Equal(t, false, result["exclude_by_default"])
}

func TestTransform_DropsOkapiOnlyParams(t *testing.T) {
	from := config.OkapiFilterConfigKind("xml")
	to := config.FormatConfigKind("xml")

	input := map[string]any{
		"configFile":              "myconfig.yml",
		"configId":                "abc",
		"useCodeFinder":           true,
		"codeFinderRules":         []any{"rule1"},
		"quoteMode":               0,
		"quoteModeDefined":        true,
		"global_cdata_subfilter":  "html",
		"global_pcdata_subfilter": "html",
		"exclude_by_default":      true,
	}

	result, err := config.DefaultTransforms.Transform(from, to, input)
	require.NoError(t, err)

	// Okapi-only params should be dropped
	_, has := result["config_file"]
	assert.False(t, has)
	_, has = result["config_id"]
	assert.False(t, has)
	_, has = result["use_code_finder"]
	assert.False(t, has)
	_, has = result["code_finder_rules"]
	assert.False(t, has)
	_, has = result["quote_mode"]
	assert.False(t, has)
	_, has = result["quote_mode_defined"]
	assert.False(t, has)
	_, has = result["global_cdata_subfilter"]
	assert.False(t, has)
	_, has = result["global_pcdata_subfilter"]
	assert.False(t, has)

	// Shared params should be kept (renamed)
	assert.Equal(t, true, result["exclude_by_default"])
}

func TestTransform_DropsParserAssumeWellformed(t *testing.T) {
	from := config.OkapiFilterConfigKind("xml")
	to := config.FormatConfigKind("xml")

	input := map[string]any{
		"parser": map[string]any{
			"assumeWellformed":   true,
			"preserveWhitespace": true,
		},
	}

	result, err := config.DefaultTransforms.Transform(from, to, input)
	require.NoError(t, err)

	parser, ok := result["parser"].(map[string]any)
	require.True(t, ok)
	_, has := parser["assume_wellformed"]
	assert.False(t, has, "assumeWellformed should be dropped")
	assert.Equal(t, true, parser["preserve_whitespace"])
}

func TestTransform_EmptyParserDropped(t *testing.T) {
	from := config.OkapiFilterConfigKind("xml")
	to := config.FormatConfigKind("xml")

	input := map[string]any{
		"parser": map[string]any{
			"assumeWellformed": true,
		},
	}

	result, err := config.DefaultTransforms.Transform(from, to, input)
	require.NoError(t, err)

	// Parser section should be dropped if only assumeWellformed was present
	_, has := result["parser"]
	assert.False(t, has, "empty parser section should be dropped")
}
