package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
)

func TestSegCountTool(t *testing.T) {
	t.Parallel()
	cfg := &tools.SegCountConfig{Locale: model.LocaleFrench}
	tl := tools.NewSegCountTool(cfg)

	assert.Equal(t, "segment-count", tl.Name())

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText(model.LocaleFrench, "Bonjour")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	sf, ok := model.AnnoAs[*tools.SegCountFacet](resultBlock, string(model.AnnoSegCount))
	assert.True(t, ok)
	assert.Equal(t, 1, sf.Source)
	assert.Equal(t, 1, sf.Target)
}

func TestSegCountToolSourceOnly(t *testing.T) {
	t.Parallel()
	cfg := &tools.SegCountConfig{}
	tl := tools.NewSegCountTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	sf, ok := model.AnnoAs[*tools.SegCountFacet](resultBlock, string(model.AnnoSegCount))
	assert.True(t, ok)
	assert.Equal(t, 1, sf.Source)
	assert.Equal(t, 0, sf.Target)
}
