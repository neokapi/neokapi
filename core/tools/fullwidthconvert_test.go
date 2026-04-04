package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFullWidthConvertToHalf(t *testing.T) {
	cfg := &tools.FullWidthConvertConfig{
		Mode:         tools.FullWidthToHalf,
		ApplyTarget:  true,
		TargetLocale: model.LocaleJapanese,
	}
	tl := tools.NewFullWidthConvertTool(cfg)

	assert.Equal(t, "fullwidth-convert", tl.Name())

	block := model.NewBlock("tu1", "Hello")
	// Full-width "Hello" = \uFF28\uFF45\uFF4C\uFF4C\uFF4F
	block.SetTargetText(model.LocaleJapanese, "\uFF28\uFF45\uFF4C\uFF4C\uFF4F")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello", resultBlock.TargetText(model.LocaleJapanese))
	// Source should be unchanged.
	assert.Equal(t, "Hello", resultBlock.SourceText())
}

func TestFullWidthConvertToFull(t *testing.T) {
	cfg := &tools.FullWidthConvertConfig{
		Mode:         tools.FullWidthToFull,
		ApplyTarget:  true,
		TargetLocale: model.LocaleJapanese,
	}
	tl := tools.NewFullWidthConvertTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText(model.LocaleJapanese, "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "\uFF28\uFF45\uFF4C\uFF4C\uFF4F", resultBlock.TargetText(model.LocaleJapanese))
}

func TestFullWidthConvertMixedText(t *testing.T) {
	cfg := &tools.FullWidthConvertConfig{
		Mode:         tools.FullWidthToHalf,
		ApplyTarget:  true,
		TargetLocale: model.LocaleJapanese,
	}
	tl := tools.NewFullWidthConvertTool(cfg)

	block := model.NewBlock("tu1", "test")
	// Mix of full-width "AB" + regular "cd"
	block.SetTargetText(model.LocaleJapanese, "\uFF21\uFF22cd")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "ABcd", resultBlock.TargetText(model.LocaleJapanese))
}

func TestFullWidthConvertCJKUnchanged(t *testing.T) {
	cfg := &tools.FullWidthConvertConfig{
		Mode:         tools.FullWidthToHalf,
		ApplyTarget:  true,
		TargetLocale: model.LocaleJapanese,
	}
	tl := tools.NewFullWidthConvertTool(cfg)

	block := model.NewBlock("tu1", "test")
	// CJK characters should pass through unchanged.
	block.SetTargetText(model.LocaleJapanese, "\u6F22\u5B57\uFF21") // 漢字 + full-width A
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "\u6F22\u5B57A", resultBlock.TargetText(model.LocaleJapanese))
}

func TestFullWidthConvertSpaceMapping(t *testing.T) {
	cfg := &tools.FullWidthConvertConfig{
		Mode:         tools.FullWidthToFull,
		ApplyTarget:  true,
		TargetLocale: model.LocaleJapanese,
	}
	tl := tools.NewFullWidthConvertTool(cfg)

	block := model.NewBlock("tu1", "test")
	block.SetTargetText(model.LocaleJapanese, "A B")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// Space (0x20) should map to ideographic space (0x3000).
	assert.Equal(t, "\uFF21\u3000\uFF22", resultBlock.TargetText(model.LocaleJapanese))
}

func TestFullWidthConvertIdeographicSpaceToHalf(t *testing.T) {
	cfg := &tools.FullWidthConvertConfig{
		Mode:         tools.FullWidthToHalf,
		ApplyTarget:  true,
		TargetLocale: model.LocaleJapanese,
	}
	tl := tools.NewFullWidthConvertTool(cfg)

	block := model.NewBlock("tu1", "test")
	block.SetTargetText(model.LocaleJapanese, "\uFF21\u3000\uFF22")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "A B", resultBlock.TargetText(model.LocaleJapanese))
}

func TestFullWidthConvertApplySource(t *testing.T) {
	cfg := &tools.FullWidthConvertConfig{
		Mode:        tools.FullWidthToHalf,
		ApplySource: true,
		ApplyTarget: false,
	}
	tl := tools.NewFullWidthConvertTool(cfg)

	block := model.NewBlock("tu1", "\uFF28\uFF45\uFF4C\uFF4C\uFF4F")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello", resultBlock.SourceText())
}

func TestFullWidthConvertSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.FullWidthConvertConfig{
		Mode:         tools.FullWidthToHalf,
		ApplyTarget:  true,
		TargetLocale: model.LocaleJapanese,
	}
	tl := tools.NewFullWidthConvertTool(cfg)

	block := model.NewBlock("tu1", "test")
	block.Translatable = false
	block.SetTargetText(model.LocaleJapanese, "\uFF28\uFF45\uFF4C\uFF4C\uFF4F")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// Should be unchanged because the block is not translatable.
	assert.Equal(t, "\uFF28\uFF45\uFF4C\uFF4C\uFF4F", resultBlock.TargetText(model.LocaleJapanese))
}

func TestFullWidthConvertConfigValidation(t *testing.T) {
	cfg := &tools.FullWidthConvertConfig{Mode: "invalid"}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Mode")

	cfg = &tools.FullWidthConvertConfig{Mode: tools.FullWidthToHalf, ApplyTarget: true, TargetLocale: ""}
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg = &tools.FullWidthConvertConfig{Mode: tools.FullWidthToHalf, ApplySource: true, ApplyTarget: false}
	err = cfg.Validate()
	require.NoError(t, err)
}
