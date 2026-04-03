package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
)

func TestURIConvertDecode(t *testing.T) {
	cfg := &tools.URIConvertConfig{
		Mode:         tools.URIDecode,
		ApplyTarget:  true,
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewURIConvertTool(cfg)

	assert.Equal(t, "uri-convert", tl.Name())

	block := model.NewBlock("tu1", "Hello World")
	block.SetTargetText(model.LocaleFrench, "Bonjour%20le%20monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
	// Source should be unchanged.
	assert.Equal(t, "Hello World", resultBlock.SourceText())
}

func TestURIConvertEncode(t *testing.T) {
	cfg := &tools.URIConvertConfig{
		Mode:         tools.URIEncode,
		ApplyTarget:  true,
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewURIConvertTool(cfg)

	block := model.NewBlock("tu1", "Hello World")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour%20le%20monde", resultBlock.TargetText(model.LocaleFrench))
}

func TestURIConvertAlreadyEncoded(t *testing.T) {
	cfg := &tools.URIConvertConfig{
		Mode:         tools.URIEncode,
		ApplyTarget:  true,
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewURIConvertTool(cfg)

	block := model.NewBlock("tu1", "test")
	// Already-encoded text: encoding it again should double-encode the percent signs.
	block.SetTargetText(model.LocaleFrench, "Hello%20World")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello%2520World", resultBlock.TargetText(model.LocaleFrench))
}

func TestURIConvertSpecialCharacters(t *testing.T) {
	cfg := &tools.URIConvertConfig{
		Mode:         tools.URIEncode,
		ApplyTarget:  true,
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewURIConvertTool(cfg)

	block := model.NewBlock("tu1", "test")
	block.SetTargetText(model.LocaleFrench, "hello/world?foo=bar&baz=qux")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	encoded := resultBlock.TargetText(model.LocaleFrench)
	// PathEscape encodes / and ? but preserves = and &.
	assert.Contains(t, encoded, "%2F") // / is encoded
	assert.Contains(t, encoded, "%3F") // ? is encoded
}

func TestURIConvertDecodeSpecialCharacters(t *testing.T) {
	cfg := &tools.URIConvertConfig{
		Mode:         tools.URIDecode,
		ApplyTarget:  true,
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewURIConvertTool(cfg)

	block := model.NewBlock("tu1", "test")
	block.SetTargetText(model.LocaleFrench, "caf%C3%A9")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "caf\u00e9", resultBlock.TargetText(model.LocaleFrench))
}

func TestURIConvertApplySource(t *testing.T) {
	cfg := &tools.URIConvertConfig{
		Mode:        tools.URIDecode,
		ApplySource: true,
		ApplyTarget: false,
	}
	tl := tools.NewURIConvertTool(cfg)

	block := model.NewBlock("tu1", "Hello%20World")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello World", resultBlock.SourceText())
}

func TestURIConvertSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.URIConvertConfig{
		Mode:         tools.URIDecode,
		ApplyTarget:  true,
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewURIConvertTool(cfg)

	block := model.NewBlock("tu1", "test")
	block.Translatable = false
	block.SetTargetText(model.LocaleFrench, "Hello%20World")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// Should be unchanged because the block is not translatable.
	assert.Equal(t, "Hello%20World", resultBlock.TargetText(model.LocaleFrench))
}

func TestURIConvertConfigValidation(t *testing.T) {
	cfg := &tools.URIConvertConfig{Mode: "invalid"}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Mode")

	cfg = &tools.URIConvertConfig{Mode: tools.URIDecode, ApplyTarget: true, TargetLocale: ""}
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg = &tools.URIConvertConfig{Mode: tools.URIEncode, ApplySource: true, ApplyTarget: false}
	err = cfg.Validate()
	assert.NoError(t, err)
}
