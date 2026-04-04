package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWhitespaceCorrectNormalizeSpaces(t *testing.T) {
	cfg := &tools.WhitespaceCorrectConfig{
		TargetLocale:          model.LocaleFrench,
		NormalizeSpaces:       true,
		MatchSourceWhitespace: false,
	}
	tl := tools.NewWhitespaceCorrectTool(cfg)

	assert.Equal(t, "whitespace-correct", tl.Name())

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour   le    monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
}

func TestWhitespaceCorrectTrimLeading(t *testing.T) {
	cfg := &tools.WhitespaceCorrectConfig{
		TargetLocale:          model.LocaleFrench,
		TrimLeading:           true,
		MatchSourceWhitespace: false,
	}
	tl := tools.NewWhitespaceCorrectTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText(model.LocaleFrench, "  \tBonjour")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour", resultBlock.TargetText(model.LocaleFrench))
}

func TestWhitespaceCorrectTrimTrailing(t *testing.T) {
	cfg := &tools.WhitespaceCorrectConfig{
		TargetLocale:          model.LocaleFrench,
		TrimTrailing:          true,
		MatchSourceWhitespace: false,
	}
	tl := tools.NewWhitespaceCorrectTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText(model.LocaleFrench, "Bonjour  \t")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour", resultBlock.TargetText(model.LocaleFrench))
}

func TestWhitespaceCorrectMatchSourceWhitespace(t *testing.T) {
	cfg := &tools.WhitespaceCorrectConfig{
		TargetLocale:          model.LocaleFrench,
		MatchSourceWhitespace: true,
	}
	tl := tools.NewWhitespaceCorrectTool(cfg)

	block := model.NewBlock("tu1", "  Hello  ")
	block.SetTargetText(model.LocaleFrench, "Bonjour")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// Target should now have source's leading/trailing whitespace.
	assert.Equal(t, "  Bonjour  ", resultBlock.TargetText(model.LocaleFrench))
}

func TestWhitespaceCorrectRemoveZeroWidthChars(t *testing.T) {
	cfg := &tools.WhitespaceCorrectConfig{
		TargetLocale:          model.LocaleFrench,
		RemoveZeroWidthChars:  true,
		MatchSourceWhitespace: false,
	}
	tl := tools.NewWhitespaceCorrectTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	// Insert zero-width chars: U+200B, U+200C, U+200D, U+FEFF
	block.SetTargetText(model.LocaleFrench, "Bon\u200Bjour\u200C le\u200D mon\uFEFFde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
}

func TestWhitespaceCorrectSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.WhitespaceCorrectConfig{
		TargetLocale:    model.LocaleFrench,
		NormalizeSpaces: true,
	}
	tl := tools.NewWhitespaceCorrectTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	block.SetTargetText(model.LocaleFrench, "Bonjour   le   monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// Should be unchanged because block is non-translatable.
	assert.Equal(t, "Bonjour   le   monde", resultBlock.TargetText(model.LocaleFrench))
}

func TestWhitespaceCorrectSkipsNoTarget(t *testing.T) {
	cfg := &tools.WhitespaceCorrectConfig{
		TargetLocale:    model.LocaleFrench,
		NormalizeSpaces: true,
	}
	tl := tools.NewWhitespaceCorrectTool(cfg)

	block := model.NewBlock("tu1", "Hello   world")
	// No target set.
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestWhitespaceCorrectCombined(t *testing.T) {
	cfg := &tools.WhitespaceCorrectConfig{
		TargetLocale:          model.LocaleFrench,
		NormalizeSpaces:       true,
		RemoveZeroWidthChars:  true,
		TrimLeading:           false,
		TrimTrailing:          false,
		MatchSourceWhitespace: true,
	}
	tl := tools.NewWhitespaceCorrectTool(cfg)

	block := model.NewBlock("tu1", " Hello world ")
	block.SetTargetText(model.LocaleFrench, "  Bon\u200Bjour   le   monde  ")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// Zero-width removed, spaces normalized, then source whitespace matched.
	assert.Equal(t, " Bonjour le monde ", resultBlock.TargetText(model.LocaleFrench))
}

func TestWhitespaceCorrectConfigValidation(t *testing.T) {
	cfg := &tools.WhitespaceCorrectConfig{}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.TargetLocale = model.LocaleFrench
	err = cfg.Validate()
	require.NoError(t, err)
}

func TestWhitespaceCorrectConfigReset(t *testing.T) {
	cfg := &tools.WhitespaceCorrectConfig{
		TargetLocale:    model.LocaleFrench,
		NormalizeSpaces: false,
		TrimLeading:     true,
	}
	cfg.Reset()

	assert.True(t, cfg.TargetLocale.IsEmpty())
	assert.True(t, cfg.NormalizeSpaces)
	assert.False(t, cfg.TrimLeading)
	assert.False(t, cfg.TrimTrailing)
	assert.True(t, cfg.MatchSourceWhitespace)
	assert.True(t, cfg.RemoveZeroWidthChars)
}
