package html

import (
	"testing"

	"github.com/neokapi/neokapi/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOkapiHTMLTransform_Registration(t *testing.T) {
	assert.True(t, config.DefaultTransforms.Has(
		config.OkapiFilterConfigKind("html"), config.FormatConfigKind("html")))
}

func TestOkapiHTMLTransform_DropsOkapiOnlyParams(t *testing.T) {
	from := config.OkapiFilterConfigKind("html")
	to := config.FormatConfigKind("html")
	spec := map[string]any{
		"quoteMode":        3,
		"quoteModeDefined": true,
		"parser": map[string]any{
			"preserveWhitespace": true,
			"assumeWellformed":   true,
		},
		"elements": map[string]any{
			"pre": map[string]any{"ruleTypes": []string{"EXCLUDE"}},
		},
		"useCodeFinder":   true,
		"codeFinderRules": []string{`<\/?[a-z]+>`},
	}

	result, err := config.DefaultTransforms.Transform(from, to, spec)
	require.NoError(t, err)

	// Okapi-only top-level params dropped
	assert.Nil(t, result["quote_mode"])
	assert.Nil(t, result["quote_mode_defined"])

	// Parser: assumeWellformed dropped, preserveWhitespace kept
	parser, ok := result["parser"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, parser["preserve_whitespace"])
	assert.Nil(t, parser["assume_wellformed"])

	// Shared params kept
	assert.NotNil(t, result["elements"])
	assert.Equal(t, true, result["use_code_finder"])
	assert.NotNil(t, result["code_finder_rules"])
}

func TestOkapiHTMLTransform_EmptySpec(t *testing.T) {
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("html"), config.FormatConfigKind("html"), map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestOkapiHTMLTransform_ParserAllOkapiOnly(t *testing.T) {
	spec := map[string]any{
		"parser": map[string]any{
			"assumeWellformed": true,
		},
	}
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("html"), config.FormatConfigKind("html"), spec)
	require.NoError(t, err)
	assert.Nil(t, result["parser"])
}

func TestOkapiHTMLTransform_NonMapParser(t *testing.T) {
	spec := map[string]any{
		"parser": "invalid",
	}
	result, err := config.DefaultTransforms.Transform(
		config.OkapiFilterConfigKind("html"), config.FormatConfigKind("html"), spec)
	require.NoError(t, err)
	assert.Equal(t, "invalid", result["parser"])
}
