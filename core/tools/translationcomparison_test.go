package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranslationComparisonIdentical(t *testing.T) {
	cfg := &tools.TranslationComparisonConfig{
		Locale1: "fr-FR",
		Locale2: "fr-CA",
	}
	tl := tools.NewTranslationComparisonTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText("fr-FR", "Bonjour")
	block.SetTargetText("fr-CA", "Bonjour")
	part := &model.Part{Type: model.PartBlock, Resource: block}

	result := processPart(t, tl, part)
	resultBlock := result.Resource.(*model.Block)

	assert.Equal(t, "identical", resultBlock.Properties[tools.PropComparisonStatus])
	assert.Contains(t, resultBlock.Properties[tools.PropComparisonDiff], "identical")
}

func TestTranslationComparisonDifferent(t *testing.T) {
	cfg := &tools.TranslationComparisonConfig{
		Locale1: "fr-FR",
		Locale2: "fr-CA",
	}
	tl := tools.NewTranslationComparisonTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText("fr-FR", "Bonjour")
	block.SetTargetText("fr-CA", "Salut")
	part := &model.Part{Type: model.PartBlock, Resource: block}

	result := processPart(t, tl, part)
	resultBlock := result.Resource.(*model.Block)

	assert.Equal(t, "different", resultBlock.Properties[tools.PropComparisonStatus])
	assert.Contains(t, resultBlock.Properties[tools.PropComparisonDiff], "Bonjour")
	assert.Contains(t, resultBlock.Properties[tools.PropComparisonDiff], "Salut")
}

func TestTranslationComparisonMissingLocale1(t *testing.T) {
	cfg := &tools.TranslationComparisonConfig{
		Locale1: "fr-FR",
		Locale2: "fr-CA",
	}
	tl := tools.NewTranslationComparisonTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText("fr-CA", "Salut")
	part := &model.Part{Type: model.PartBlock, Resource: block}

	result := processPart(t, tl, part)
	resultBlock := result.Resource.(*model.Block)

	assert.Equal(t, "missing-locale1", resultBlock.Properties[tools.PropComparisonStatus])
	assert.Contains(t, resultBlock.Properties[tools.PropComparisonDiff], "fr-FR")
}

func TestTranslationComparisonMissingLocale2(t *testing.T) {
	cfg := &tools.TranslationComparisonConfig{
		Locale1: "fr-FR",
		Locale2: "fr-CA",
	}
	tl := tools.NewTranslationComparisonTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText("fr-FR", "Bonjour")
	part := &model.Part{Type: model.PartBlock, Resource: block}

	result := processPart(t, tl, part)
	resultBlock := result.Resource.(*model.Block)

	assert.Equal(t, "missing-locale2", resultBlock.Properties[tools.PropComparisonStatus])
	assert.Contains(t, resultBlock.Properties[tools.PropComparisonDiff], "fr-CA")
}

func TestTranslationComparisonMissingBoth(t *testing.T) {
	cfg := &tools.TranslationComparisonConfig{
		Locale1: "fr-FR",
		Locale2: "fr-CA",
	}
	tl := tools.NewTranslationComparisonTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}

	result := processPart(t, tl, part)
	resultBlock := result.Resource.(*model.Block)

	assert.Equal(t, "missing-both", resultBlock.Properties[tools.PropComparisonStatus])
	assert.Contains(t, resultBlock.Properties[tools.PropComparisonDiff], "fr-FR")
	assert.Contains(t, resultBlock.Properties[tools.PropComparisonDiff], "fr-CA")
}

func TestTranslationComparisonSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.TranslationComparisonConfig{
		Locale1: "fr-FR",
		Locale2: "fr-CA",
	}
	tl := tools.NewTranslationComparisonTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	block.SetTargetText("fr-FR", "Bonjour")
	block.SetTargetText("fr-CA", "Salut")
	part := &model.Part{Type: model.PartBlock, Resource: block}

	result := processPart(t, tl, part)
	resultBlock := result.Resource.(*model.Block)

	_, hasStatus := resultBlock.Properties[tools.PropComparisonStatus]
	assert.False(t, hasStatus, "non-translatable blocks should not have comparison status")
}

func TestTranslationComparisonConfigValidation(t *testing.T) {
	cfg := &tools.TranslationComparisonConfig{}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Locale1")

	cfg.Locale1 = "fr-FR"
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Locale2")

	cfg.Locale2 = "fr-CA"
	err = cfg.Validate()
	require.NoError(t, err)

	assert.Equal(t, "translation-comparison", cfg.ToolName())

	cfg.Reset()
	assert.True(t, cfg.Locale1.IsEmpty())
	assert.True(t, cfg.Locale2.IsEmpty())
}
