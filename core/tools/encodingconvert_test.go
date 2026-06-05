package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodingConvertSetsProperty(t *testing.T) {
	t.Parallel()
	cfg := &tools.EncodingConvertConfig{
		TargetEncoding: "iso-8859-1",
		ApplyTarget:    true,
		TargetLocale:   model.LocaleFrench,
	}
	tl := tools.NewEncodingConvertTool(cfg)

	assert.Equal(t, "encoding-convert", tl.Name())

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText(model.LocaleFrench, "Bonjour")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "iso-8859-1", resultBlock.Properties[tools.PropEncodingTarget])
	// ASCII text survives roundtrip through ISO-8859-1 unchanged.
	assert.Equal(t, "Bonjour", resultBlock.TargetText(model.LocaleFrench))
}

func TestEncodingConvertRoundtrip(t *testing.T) {
	t.Parallel()
	cfg := &tools.EncodingConvertConfig{
		TargetEncoding: "iso-8859-1",
		ApplyTarget:    true,
		TargetLocale:   model.LocaleFrench,
	}
	tl := tools.NewEncodingConvertTool(cfg)

	// Accented characters that exist in ISO-8859-1.
	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
	assert.Equal(t, "iso-8859-1", resultBlock.Properties[tools.PropEncodingTarget])
}

func TestEncodingConvertApplySource(t *testing.T) {
	t.Parallel()
	cfg := &tools.EncodingConvertConfig{
		TargetEncoding: "us-ascii",
		ApplySource:    true,
		ApplyTarget:    false,
	}
	tl := tools.NewEncodingConvertTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "us-ascii", resultBlock.Properties[tools.PropEncodingTarget])
	// ASCII text survives US-ASCII roundtrip.
	assert.Equal(t, "Hello world", resultBlock.SourceText())
}

func TestEncodingConvertSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	cfg := &tools.EncodingConvertConfig{
		TargetEncoding: "iso-8859-1",
		ApplyTarget:    true,
		TargetLocale:   model.LocaleFrench,
	}
	tl := tools.NewEncodingConvertTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	block.SetTargetText(model.LocaleFrench, "Bonjour")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// No property set on non-translatable blocks.
	_, hasProperty := resultBlock.Properties[tools.PropEncodingTarget]
	assert.False(t, hasProperty)
}

func TestEncodingConvertConfigValidation(t *testing.T) {
	t.Parallel()
	// Missing TargetEncoding.
	cfg := &tools.EncodingConvertConfig{}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TargetEncoding")

	// Invalid encoding name.
	cfg.TargetEncoding = "not-a-real-encoding"
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported encoding")

	// Valid encoding but missing TargetLocale when ApplyTarget is true.
	cfg.TargetEncoding = "utf-8"
	cfg.ApplyTarget = true
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	// All valid.
	cfg.TargetLocale = model.LocaleFrench
	err = cfg.Validate()
	require.NoError(t, err)
}

func TestEncodingConvertConfigReset(t *testing.T) {
	t.Parallel()
	cfg := &tools.EncodingConvertConfig{
		TargetEncoding: "shift-jis",
		ApplySource:    true,
		ApplyTarget:    false,
		TargetLocale:   model.LocaleFrench,
	}
	cfg.Reset()

	assert.Empty(t, cfg.TargetEncoding)
	assert.False(t, cfg.ApplySource)
	assert.True(t, cfg.ApplyTarget)
	assert.True(t, cfg.TargetLocale.IsEmpty())
}

func TestEncodingConvertSkipsNoTarget(t *testing.T) {
	t.Parallel()
	cfg := &tools.EncodingConvertConfig{
		TargetEncoding: "iso-8859-1",
		ApplyTarget:    true,
		TargetLocale:   model.LocaleFrench,
	}
	tl := tools.NewEncodingConvertTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	// No target set — tool should still set property but not fail.
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "iso-8859-1", resultBlock.Properties[tools.PropEncodingTarget])
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}
