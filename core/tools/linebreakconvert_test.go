package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLineBreakConvertToolLF(t *testing.T) {
	cfg := &tools.LineBreakConvertConfig{
		Mode:        tools.LineBreakLF,
		ApplySource: true,
		ApplyTarget: true,
	}
	tl := tools.NewLineBreakConvertTool(cfg)

	assert.Equal(t, "linebreak-convert", tl.Name())

	block := model.NewBlock("tu1", "Hello\r\nworld\r\nfoo")
	block.SetTargetText(model.LocaleFrench, "Bonjour\r\nle monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello\nworld\nfoo", resultBlock.SourceText())
	assert.Equal(t, "Bonjour\nle monde", resultBlock.TargetText(model.LocaleFrench))
}

func TestLineBreakConvertToolCRLF(t *testing.T) {
	cfg := &tools.LineBreakConvertConfig{
		Mode:        tools.LineBreakCRLF,
		ApplySource: true,
		ApplyTarget: true,
	}
	tl := tools.NewLineBreakConvertTool(cfg)

	block := model.NewBlock("tu1", "Hello\nworld\nfoo")
	block.SetTargetText(model.LocaleFrench, "Bonjour\nle monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello\r\nworld\r\nfoo", resultBlock.SourceText())
	assert.Equal(t, "Bonjour\r\nle monde", resultBlock.TargetText(model.LocaleFrench))
}

func TestLineBreakConvertToolCR(t *testing.T) {
	cfg := &tools.LineBreakConvertConfig{
		Mode:        tools.LineBreakCR,
		ApplySource: true,
		ApplyTarget: true,
	}
	tl := tools.NewLineBreakConvertTool(cfg)

	block := model.NewBlock("tu1", "Hello\nworld")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello\rworld", resultBlock.SourceText())
}

func TestLineBreakConvertToolMixedLineEndings(t *testing.T) {
	cfg := &tools.LineBreakConvertConfig{
		Mode:        tools.LineBreakLF,
		ApplySource: true,
		ApplyTarget: true,
	}
	tl := tools.NewLineBreakConvertTool(cfg)

	// Mix of \r\n, \r, and \n
	block := model.NewBlock("tu1", "line1\r\nline2\rline3\nline4")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "line1\nline2\nline3\nline4", resultBlock.SourceText())
}

func TestLineBreakConvertToolNoOpPlainText(t *testing.T) {
	cfg := &tools.LineBreakConvertConfig{
		Mode:        tools.LineBreakLF,
		ApplySource: true,
		ApplyTarget: true,
	}
	tl := tools.NewLineBreakConvertTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello world", resultBlock.SourceText())
}

func TestLineBreakConvertToolSourceOnly(t *testing.T) {
	cfg := &tools.LineBreakConvertConfig{
		Mode:        tools.LineBreakLF,
		ApplySource: true,
		ApplyTarget: false,
	}
	tl := tools.NewLineBreakConvertTool(cfg)

	block := model.NewBlock("tu1", "Hello\r\nworld")
	block.SetTargetText(model.LocaleFrench, "Bonjour\r\nle monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello\nworld", resultBlock.SourceText())
	// Target should remain unchanged.
	assert.Equal(t, "Bonjour\r\nle monde", resultBlock.TargetText(model.LocaleFrench))
}

func TestLineBreakConvertToolSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.LineBreakConvertConfig{
		Mode:        tools.LineBreakLF,
		ApplySource: true,
		ApplyTarget: true,
	}
	tl := tools.NewLineBreakConvertTool(cfg)

	block := model.NewBlock("tu1", "Hello\r\nworld")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello\r\nworld", resultBlock.SourceText())
}

func TestLineBreakConvertConfigValidation(t *testing.T) {
	cfg := &tools.LineBreakConvertConfig{Mode: "invalid"}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Mode")

	cfg.Mode = tools.LineBreakLF
	err = cfg.Validate()
	require.NoError(t, err)
}

func TestLineBreakConvertConfigReset(t *testing.T) {
	cfg := &tools.LineBreakConvertConfig{
		Mode:        tools.LineBreakCRLF,
		ApplySource: false,
		ApplyTarget: false,
	}
	cfg.Reset()
	assert.Equal(t, tools.LineBreakLF, cfg.Mode)
	assert.True(t, cfg.ApplySource)
	assert.True(t, cfg.ApplyTarget)
}

func TestLineBreakConvertConfigToolName(t *testing.T) {
	cfg := &tools.LineBreakConvertConfig{}
	assert.Equal(t, "linebreak-convert", cfg.ToolName())
}
