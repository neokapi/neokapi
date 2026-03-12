// okapi-filter: xml
package xml

import (
	"testing"

	"github.com/gokapi/gokapi/core/config"
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
	assert.Equal(t, true, result["excludeByDefault"])
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
	assert.Equal(t, false, result["excludeByDefault"])
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
	_, has := result["configFile"]
	assert.False(t, has)
	_, has = result["configId"]
	assert.False(t, has)
	_, has = result["useCodeFinder"]
	assert.False(t, has)
	_, has = result["codeFinderRules"]
	assert.False(t, has)
	_, has = result["quoteMode"]
	assert.False(t, has)
	_, has = result["quoteModeDefined"]
	assert.False(t, has)
	_, has = result["global_cdata_subfilter"]
	assert.False(t, has)
	_, has = result["global_pcdata_subfilter"]
	assert.False(t, has)

	// Shared params should be kept (renamed)
	assert.Equal(t, true, result["excludeByDefault"])
}

func TestTransform_DropsParserAssumeWellformed(t *testing.T) {
	from := config.OkapiFilterConfigKind("xml")
	to := config.FormatConfigKind("xml")

	input := map[string]any{
		"parser": map[string]any{
			"assumeWellformed":    true,
			"preserveWhitespace": true,
		},
	}

	result, err := config.DefaultTransforms.Transform(from, to, input)
	require.NoError(t, err)

	parser, ok := result["parser"].(map[string]any)
	require.True(t, ok)
	_, has := parser["assumeWellformed"]
	assert.False(t, has, "assumeWellformed should be dropped")
	assert.Equal(t, true, parser["preserveWhitespace"])
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
