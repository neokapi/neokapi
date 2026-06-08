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
	cf, ok := model.AnnoAs[*tools.CharCountAnnotation](resultBlock, string(model.AnnoCharCount))
	assert.True(t, ok)
	// "Hello world" = 11 chars, 10 without spaces.
	assert.Equal(t, 11, cf.Source)
	assert.Equal(t, 10, cf.SourceNoSpace)
	// "Bonjour le monde" = 16 chars, 14 without spaces.
	assert.Equal(t, 16, cf.Targets[model.LocaleFrench])
	assert.Equal(t, 14, cf.TargetsNoSpace[model.LocaleFrench])
}

func TestCharCountToolSourceOnly(t *testing.T) {
	t.Parallel()
	cfg := &tools.CharCountConfig{}
	tl := tools.NewCharCountTool(cfg)

	block := model.NewBlock("tu1", "Test text")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	cf, ok := model.AnnoAs[*tools.CharCountAnnotation](resultBlock, string(model.AnnoCharCount))
	assert.True(t, ok)
	// "Test text" = 9 chars, 8 without spaces.
	assert.Equal(t, 9, cf.Source)
	assert.Equal(t, 8, cf.SourceNoSpace)
	// No target count since no locale and no target.
	assert.Empty(t, cf.Targets)
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
	cf, ok := model.AnnoAs[*tools.CharCountAnnotation](resultBlock, string(model.AnnoCharCount))
	assert.True(t, ok)
	assert.Equal(t, 5, cf.Source)
	assert.Equal(t, 5, cf.SourceNoSpace)
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
	_, ok := model.AnnoAs[*tools.CharCountAnnotation](resultBlock, string(model.AnnoCharCount))
	assert.False(t, ok)
}

func TestCharCountToolEmptyText(t *testing.T) {
	t.Parallel()
	cfg := &tools.CharCountConfig{}
	tl := tools.NewCharCountTool(cfg)

	block := model.NewBlock("tu1", "")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	cf, ok := model.AnnoAs[*tools.CharCountAnnotation](resultBlock, string(model.AnnoCharCount))
	assert.True(t, ok)
	assert.Equal(t, 0, cf.Source)
	assert.Equal(t, 0, cf.SourceNoSpace)
}
