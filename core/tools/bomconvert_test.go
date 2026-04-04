package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBOMConvertToolAddBOM(t *testing.T) {
	t.Parallel()
	cfg := &tools.BOMConvertConfig{AddBOM: true}
	tl := tools.NewBOMConvertTool(cfg)

	assert.Equal(t, "bom-convert", tl.Name())

	layer := &model.Layer{ID: "doc1", HasBOM: false}
	part := &model.Part{Type: model.PartLayerStart, Resource: layer}
	result := processPart(t, tl, part)

	resultLayer := result.Resource.(*model.Layer)
	assert.True(t, resultLayer.HasBOM)
}

func TestBOMConvertToolRemoveBOM(t *testing.T) {
	t.Parallel()
	cfg := &tools.BOMConvertConfig{AddBOM: false}
	tl := tools.NewBOMConvertTool(cfg)

	layer := &model.Layer{ID: "doc1", HasBOM: true}
	part := &model.Part{Type: model.PartLayerStart, Resource: layer}
	result := processPart(t, tl, part)

	resultLayer := result.Resource.(*model.Layer)
	assert.False(t, resultLayer.HasBOM)
}

func TestBOMConvertToolAlreadyHasBOM(t *testing.T) {
	t.Parallel()
	cfg := &tools.BOMConvertConfig{AddBOM: true}
	tl := tools.NewBOMConvertTool(cfg)

	layer := &model.Layer{ID: "doc1", HasBOM: true}
	part := &model.Part{Type: model.PartLayerStart, Resource: layer}
	result := processPart(t, tl, part)

	resultLayer := result.Resource.(*model.Layer)
	assert.True(t, resultLayer.HasBOM)
}

func TestBOMConvertToolAlreadyNoBOM(t *testing.T) {
	t.Parallel()
	cfg := &tools.BOMConvertConfig{AddBOM: false}
	tl := tools.NewBOMConvertTool(cfg)

	layer := &model.Layer{ID: "doc1", HasBOM: false}
	part := &model.Part{Type: model.PartLayerStart, Resource: layer}
	result := processPart(t, tl, part)

	resultLayer := result.Resource.(*model.Layer)
	assert.False(t, resultLayer.HasBOM)
}

func TestBOMConvertConfigToolName(t *testing.T) {
	t.Parallel()
	cfg := &tools.BOMConvertConfig{}
	assert.Equal(t, "bom-convert", cfg.ToolName())
}

func TestBOMConvertConfigReset(t *testing.T) {
	t.Parallel()
	cfg := &tools.BOMConvertConfig{AddBOM: true}
	cfg.Reset()
	assert.False(t, cfg.AddBOM)
}

func TestBOMConvertConfigValidate(t *testing.T) {
	t.Parallel()
	cfg := &tools.BOMConvertConfig{}
	require.NoError(t, cfg.Validate())
}
