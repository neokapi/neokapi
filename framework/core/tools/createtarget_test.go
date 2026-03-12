package tools_test

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateTargetTool(t *testing.T) {
	cfg := &tools.CreateTargetConfig{
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewCreateTargetTool(cfg)

	assert.Equal(t, "create-target", tl.Name())

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	require.True(t, resultBlock.HasTarget(model.LocaleFrench))
	assert.Equal(t, "", resultBlock.TargetText(model.LocaleFrench))
}

func TestCreateTargetToolCopySource(t *testing.T) {
	cfg := &tools.CreateTargetConfig{
		TargetLocale: model.LocaleFrench,
		CopySource:   true,
	}
	tl := tools.NewCreateTargetTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	require.True(t, resultBlock.HasTarget(model.LocaleFrench))
	assert.Equal(t, "Hello world", resultBlock.TargetText(model.LocaleFrench))
}

func TestCreateTargetToolSkipsExisting(t *testing.T) {
	cfg := &tools.CreateTargetConfig{
		TargetLocale: model.LocaleFrench,
		CopySource:   true,
	}
	tl := tools.NewCreateTargetTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// Should not overwrite existing target.
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
}

func TestCreateTargetToolOverwrite(t *testing.T) {
	cfg := &tools.CreateTargetConfig{
		TargetLocale: model.LocaleFrench,
		CopySource:   true,
		Overwrite:    true,
	}
	tl := tools.NewCreateTargetTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// Should overwrite with source text.
	assert.Equal(t, "Hello world", resultBlock.TargetText(model.LocaleFrench))
}

func TestCreateTargetToolSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.CreateTargetConfig{
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewCreateTargetTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestCreateTargetToolPassesThroughNonBlock(t *testing.T) {
	cfg := &tools.CreateTargetConfig{
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewCreateTargetTool(cfg)

	layer := &model.Layer{ID: "doc1"}
	part := &model.Part{Type: model.PartLayerStart, Resource: layer}
	result := processPart(t, tl, part)

	assert.Equal(t, model.PartLayerStart, result.Type)
	assert.Equal(t, layer, result.Resource)
}

func TestCreateTargetConfigValidation(t *testing.T) {
	cfg := &tools.CreateTargetConfig{}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.TargetLocale = model.LocaleFrench
	err = cfg.Validate()
	assert.NoError(t, err)
}
