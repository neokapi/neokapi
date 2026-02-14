package tools_test

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/lib/tools"
	"github.com/stretchr/testify/assert"
)

func TestSegCountTool(t *testing.T) {
	cfg := &tools.SegCountConfig{Locale: model.LocaleFrench}
	tl := tools.NewSegCountTool(cfg)

	assert.Equal(t, "segment-count", tl.Name())

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText(model.LocaleFrench, "Bonjour")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "1", resultBlock.Properties[tools.PropSegCountSource])
	assert.Equal(t, "1", resultBlock.Properties[tools.PropSegCountTarget])
}

func TestSegCountToolSourceOnly(t *testing.T) {
	cfg := &tools.SegCountConfig{}
	tl := tools.NewSegCountTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "1", resultBlock.Properties[tools.PropSegCountSource])
	_, hasTarget := resultBlock.Properties[tools.PropSegCountTarget]
	assert.False(t, hasTarget)
}
