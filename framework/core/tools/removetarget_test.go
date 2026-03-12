package tools_test

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tools"
	"github.com/stretchr/testify/assert"
)

func TestRemoveTargetTool(t *testing.T) {
	cfg := &tools.RemoveTargetConfig{
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewRemoveTargetTool(cfg)

	assert.Equal(t, "remove-target", tl.Name())

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	block.SetTargetText(model.LocaleGerman, "Hallo Welt")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
	// German target should remain.
	assert.True(t, resultBlock.HasTarget(model.LocaleGerman))
	assert.Equal(t, "Hallo Welt", resultBlock.TargetText(model.LocaleGerman))
}

func TestRemoveTargetToolAllLocales(t *testing.T) {
	cfg := &tools.RemoveTargetConfig{
		TargetLocale: "", // Empty means remove all.
	}
	tl := tools.NewRemoveTargetTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	block.SetTargetText(model.LocaleGerman, "Hallo Welt")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
	assert.False(t, resultBlock.HasTarget(model.LocaleGerman))
	assert.Empty(t, resultBlock.Targets)
}

func TestRemoveTargetToolSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.RemoveTargetConfig{
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewRemoveTargetTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// Target should remain since block is non-translatable.
	assert.True(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestRemoveTargetToolPassesThroughNonBlock(t *testing.T) {
	cfg := &tools.RemoveTargetConfig{
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewRemoveTargetTool(cfg)

	data := &model.Data{ID: "d1"}
	part := &model.Part{Type: model.PartData, Resource: data}
	result := processPart(t, tl, part)

	assert.Equal(t, model.PartData, result.Type)
	assert.Equal(t, data, result.Resource)
}

func TestRemoveTargetToolNoTarget(t *testing.T) {
	cfg := &tools.RemoveTargetConfig{
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewRemoveTargetTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// Should not error when target doesn't exist.
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestRemoveTargetConfigValidation(t *testing.T) {
	cfg := &tools.RemoveTargetConfig{}
	err := cfg.Validate()
	assert.NoError(t, err) // No required fields.
}
