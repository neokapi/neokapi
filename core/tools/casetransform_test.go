package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaseTransformToolUpper(t *testing.T) {
	cfg := &tools.CaseTransformConfig{
		Mode:        tools.CaseUpper,
		ApplySource: true,
	}
	tl := tools.NewCaseTransformTool(cfg)

	assert.Equal(t, "case-transform", tl.Name())

	block := model.NewBlock("tu1", "Hello World")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "HELLO WORLD", resultBlock.SourceText())
}

func TestCaseTransformToolLower(t *testing.T) {
	cfg := &tools.CaseTransformConfig{
		Mode:        tools.CaseLower,
		ApplySource: true,
	}
	tl := tools.NewCaseTransformTool(cfg)

	block := model.NewBlock("tu1", "Hello World")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "hello world", resultBlock.SourceText())
}

func TestCaseTransformToolTarget(t *testing.T) {
	cfg := &tools.CaseTransformConfig{
		Mode:         tools.CaseUpper,
		ApplySource:  false,
		ApplyTarget:  true,
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewCaseTransformTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText(model.LocaleFrench, "Bonjour")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello", resultBlock.SourceText()) // Unchanged
	assert.Equal(t, "BONJOUR", resultBlock.TargetText(model.LocaleFrench))
}

func TestCaseTransformConfigValidation(t *testing.T) {
	cfg := &tools.CaseTransformConfig{Mode: "invalid"}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Mode")

	cfg = &tools.CaseTransformConfig{Mode: tools.CaseUpper, ApplyTarget: true}
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg = &tools.CaseTransformConfig{Mode: tools.CaseLower}
	err = cfg.Validate()
	require.NoError(t, err)
}
