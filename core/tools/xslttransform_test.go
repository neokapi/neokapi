package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXSLTTransformTool(t *testing.T) {
	t.Parallel()
	cfg := &tools.XSLTTransformConfig{
		Rules: []tools.TransformRule{
			{Pattern: `<b>(.*?)</b>`, Replace: `<strong>$1</strong>`},
		},
	}
	tl := tools.NewXSLTTransformTool(cfg)

	assert.Equal(t, "xslt-transform", tl.Name())

	block := model.NewBlock("tu1", "Hello <b>world</b>")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello <strong>world</strong>", resultBlock.SourceText())
}

func TestXSLTTransformToolMultipleRules(t *testing.T) {
	t.Parallel()
	cfg := &tools.XSLTTransformConfig{
		Rules: []tools.TransformRule{
			{Pattern: `<i>`, Replace: `<em>`},
			{Pattern: `</i>`, Replace: `</em>`},
		},
	}
	tl := tools.NewXSLTTransformTool(cfg)

	block := model.NewBlock("tu1", "<i>emphasis</i>")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "<em>emphasis</em>", resultBlock.SourceText())
}

func TestXSLTTransformConfigValidation(t *testing.T) {
	t.Parallel()
	cfg := &tools.XSLTTransformConfig{
		Rules: []tools.TransformRule{
			{Pattern: "", Replace: "x"},
		},
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty pattern")

	cfg = &tools.XSLTTransformConfig{
		Rules: []tools.TransformRule{
			{Pattern: "[invalid", Replace: "x"},
		},
	}
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pattern")

	cfg = &tools.XSLTTransformConfig{
		Rules: []tools.TransformRule{
			{Pattern: `\d+`, Replace: "NUM"},
		},
	}
	err = cfg.Validate()
	require.NoError(t, err)
}
