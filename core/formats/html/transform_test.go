package html

import (
	"testing"

	"github.com/gokapi/gokapi/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOkapiHTMLTransform_Registration(t *testing.T) {
	assert.True(t, config.DefaultTransforms.Has("okapi/html-v1", "gokapi/html-v1"))
}

func TestOkapiHTMLTransform_DropsOkapiOnlyParams(t *testing.T) {
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
		"useCodeFinder":  true,
		"codeFinderRules": []string{`<\/?[a-z]+>`},
	}

	result, err := config.DefaultTransforms.Transform("okapi/html-v1", "gokapi/html-v1", spec)
	require.NoError(t, err)

	// Okapi-only top-level params dropped
	assert.Nil(t, result["quoteMode"])
	assert.Nil(t, result["quoteModeDefined"])

	// Parser: assumeWellformed dropped, preserveWhitespace kept
	parser, ok := result["parser"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, parser["preserveWhitespace"])
	assert.Nil(t, parser["assumeWellformed"])

	// Shared params kept
	assert.NotNil(t, result["elements"])
	assert.Equal(t, true, result["useCodeFinder"])
	assert.NotNil(t, result["codeFinderRules"])
}

func TestOkapiHTMLTransform_EmptySpec(t *testing.T) {
	result, err := config.DefaultTransforms.Transform("okapi/html-v1", "gokapi/html-v1", map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestOkapiHTMLTransform_ParserAllOkapiOnly(t *testing.T) {
	spec := map[string]any{
		"parser": map[string]any{
			"assumeWellformed": true,
		},
	}
	result, err := config.DefaultTransforms.Transform("okapi/html-v1", "gokapi/html-v1", spec)
	require.NoError(t, err)
	// Parser section removed entirely when all params are okapi-only
	assert.Nil(t, result["parser"])
}

func TestOkapiHTMLTransform_NonMapParser(t *testing.T) {
	spec := map[string]any{
		"parser": "invalid",
	}
	result, err := config.DefaultTransforms.Transform("okapi/html-v1", "gokapi/html-v1", spec)
	require.NoError(t, err)
	// Non-map parser value passed through unchanged
	assert.Equal(t, "invalid", result["parser"])
}
