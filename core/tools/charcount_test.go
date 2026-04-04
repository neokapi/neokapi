package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
)

func TestCharCountTool(t *testing.T) {
	t.Parallel()
	cfg := &tools.CharCountConfig{
		Locale: model.LocaleFrench,
	}
	tl := tools.NewCharCountTool(cfg)

	assert.Equal(t, "char-count", tl.Name())

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// "Hello world" = 11 chars, 10 without spaces.
	assert.Equal(t, "11", resultBlock.Properties[tools.PropCharCountSource])
	assert.Equal(t, "10", resultBlock.Properties[tools.PropCharCountSourceNospace])
	// "Bonjour le monde" = 16 chars, 14 without spaces.
	assert.Equal(t, "16", resultBlock.Properties[tools.PropCharCountTarget])
	assert.Equal(t, "14", resultBlock.Properties[tools.PropCharCountTargetNospace])
}

func TestCharCountToolSourceOnly(t *testing.T) {
	t.Parallel()
	cfg := &tools.CharCountConfig{}
	tl := tools.NewCharCountTool(cfg)

	block := model.NewBlock("tu1", "Test text")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// "Test text" = 9 chars, 8 without spaces.
	assert.Equal(t, "9", resultBlock.Properties[tools.PropCharCountSource])
	assert.Equal(t, "8", resultBlock.Properties[tools.PropCharCountSourceNospace])
	// No target count since no locale and no target.
	_, hasTargetCount := resultBlock.Properties[tools.PropCharCountTarget]
	assert.False(t, hasTargetCount)
}

func TestCharCountToolUnicode(t *testing.T) {
	t.Parallel()
	cfg := &tools.CharCountConfig{}
	tl := tools.NewCharCountTool(cfg)

	// Unicode text: "Bonjour" = 7 chars.
	block := model.NewBlock("tu1", "\u00e9l\u00e8ve") // "eleve" with accents.
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "5", resultBlock.Properties[tools.PropCharCountSource])
	assert.Equal(t, "5", resultBlock.Properties[tools.PropCharCountSourceNospace])
}

func TestCharCountToolSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	cfg := &tools.CharCountConfig{}
	tl := tools.NewCharCountTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasSourceCount := resultBlock.Properties[tools.PropCharCountSource]
	assert.False(t, hasSourceCount)
}

func TestCharCountToolEmptyText(t *testing.T) {
	t.Parallel()
	cfg := &tools.CharCountConfig{}
	tl := tools.NewCharCountTool(cfg)

	block := model.NewBlock("tu1", "")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "0", resultBlock.Properties[tools.PropCharCountSource])
	assert.Equal(t, "0", resultBlock.Properties[tools.PropCharCountSourceNospace])
}
