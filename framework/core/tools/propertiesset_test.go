package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
)

func TestPropertiesSetTool(t *testing.T) {
	cfg := &tools.PropertiesSetConfig{
		Properties: map[string]string{
			"domain":   "legal",
			"priority": "high",
		},
		Overwrite:        true,
		OnlyTranslatable: true,
	}
	tl := tools.NewPropertiesSetTool(cfg)

	assert.Equal(t, "properties-set", tl.Name())

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "legal", resultBlock.Properties["domain"])
	assert.Equal(t, "high", resultBlock.Properties["priority"])
}

func TestPropertiesSetToolOverwriteTrue(t *testing.T) {
	cfg := &tools.PropertiesSetConfig{
		Properties: map[string]string{
			"domain": "medical",
		},
		Overwrite:        true,
		OnlyTranslatable: true,
	}
	tl := tools.NewPropertiesSetTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Properties["domain"] = "legal"
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "medical", resultBlock.Properties["domain"])
}

func TestPropertiesSetToolOverwriteFalse(t *testing.T) {
	cfg := &tools.PropertiesSetConfig{
		Properties: map[string]string{
			"domain":   "medical",
			"priority": "high",
		},
		Overwrite:        false,
		OnlyTranslatable: true,
	}
	tl := tools.NewPropertiesSetTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Properties["domain"] = "legal"
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// Existing property should NOT be overwritten.
	assert.Equal(t, "legal", resultBlock.Properties["domain"])
	// New property should be added.
	assert.Equal(t, "high", resultBlock.Properties["priority"])
}

func TestPropertiesSetToolSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.PropertiesSetConfig{
		Properties: map[string]string{
			"domain": "legal",
		},
		Overwrite:        true,
		OnlyTranslatable: true,
	}
	tl := tools.NewPropertiesSetTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasDomain := resultBlock.Properties["domain"]
	assert.False(t, hasDomain)
}

func TestPropertiesSetToolOnlyTranslatableFalse(t *testing.T) {
	cfg := &tools.PropertiesSetConfig{
		Properties: map[string]string{
			"domain": "legal",
		},
		Overwrite:        true,
		OnlyTranslatable: false,
	}
	tl := tools.NewPropertiesSetTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "legal", resultBlock.Properties["domain"])
}

func TestPropertiesSetConfigValidation(t *testing.T) {
	cfg := &tools.PropertiesSetConfig{
		Properties: nil,
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Properties")

	cfg.Properties = map[string]string{}
	err = cfg.Validate()
	assert.Error(t, err)

	cfg.Properties = map[string]string{"key": "value"}
	err = cfg.Validate()
	assert.NoError(t, err)
}
