package tools_test

import (
	"testing"

	"github.com/gokapi/gokapi/model"
	"github.com/gokapi/gokapi/tools"
	"github.com/stretchr/testify/assert"
)

func TestEncodingDetectToolASCII(t *testing.T) {
	cfg := &tools.EncodingDetectConfig{}
	tl := tools.NewEncodingDetectTool(cfg)

	assert.Equal(t, "encoding-detect", tl.Name())

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "ascii", resultBlock.Properties[tools.PropEncodingDetected])
	assert.Equal(t, "true", resultBlock.Properties[tools.PropEncodingIsASCII])
	assert.Equal(t, "true", resultBlock.Properties[tools.PropEncodingIsUTF8])
}

func TestEncodingDetectToolUTF8(t *testing.T) {
	cfg := &tools.EncodingDetectConfig{}
	tl := tools.NewEncodingDetectTool(cfg)

	block := model.NewBlock("tu1", "Héllo wörld")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "utf-8", resultBlock.Properties[tools.PropEncodingDetected])
	assert.Equal(t, "false", resultBlock.Properties[tools.PropEncodingIsASCII])
	assert.Equal(t, "true", resultBlock.Properties[tools.PropEncodingIsUTF8])
}

func TestEncodingDetectToolSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.EncodingDetectConfig{}
	tl := tools.NewEncodingDetectTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasEncoding := resultBlock.Properties[tools.PropEncodingDetected]
	assert.False(t, hasEncoding)
}
