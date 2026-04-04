package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
)

func TestXMLValidationToolValid(t *testing.T) {
	cfg := &tools.XMLValidationConfig{CheckSource: true, WrapRoot: true}
	tl := tools.NewXMLValidationTool(cfg)

	assert.Equal(t, "xml-validation", tl.Name())

	block := model.NewBlock("tu1", "Hello <b>world</b>")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "true", resultBlock.Properties[tools.PropXMLValid])
}

func TestXMLValidationToolInvalid(t *testing.T) {
	cfg := &tools.XMLValidationConfig{CheckSource: true, WrapRoot: true}
	tl := tools.NewXMLValidationTool(cfg)

	block := model.NewBlock("tu1", "Hello <b>world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropXMLValid])
	assert.NotEmpty(t, resultBlock.Properties[tools.PropXMLValidError])
}

func TestXMLValidationToolSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.XMLValidationConfig{CheckSource: true}
	tl := tools.NewXMLValidationTool(cfg)

	block := model.NewBlock("tu1", "<invalid")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasValid := resultBlock.Properties[tools.PropXMLValid]
	assert.False(t, hasValid)
}

func TestXMLValidationConfigValidation(t *testing.T) {
	cfg := &tools.XMLValidationConfig{CheckTarget: true}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "locale")

	cfg.Locale = model.LocaleFrench
	err = cfg.Validate()
	assert.NoError(t, err)
}
