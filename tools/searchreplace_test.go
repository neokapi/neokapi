package tools_test

import (
	"testing"

	"github.com/gokapi/gokapi/model"
	"github.com/gokapi/gokapi/tools"
	"github.com/stretchr/testify/assert"
)

func TestSearchReplaceTool(t *testing.T) {
	cfg := &tools.SearchReplaceConfig{
		Pairs: []tools.ReplacePair{
			{Search: "Hello", Replace: "Hi"},
			{Search: "world", Replace: "earth"},
		},
	}
	tl := tools.NewSearchReplaceTool(cfg)

	assert.Equal(t, "search-replace", tl.Name())

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hi earth", resultBlock.SourceText())
}

func TestSearchReplaceToolRegex(t *testing.T) {
	cfg := &tools.SearchReplaceConfig{
		Pairs: []tools.ReplacePair{
			{Search: `\b\d{3}\b`, Replace: "XXX", IsRegex: true},
		},
	}
	tl := tools.NewSearchReplaceTool(cfg)

	block := model.NewBlock("tu1", "Call 555 now or 123 later")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Call XXX now or XXX later", resultBlock.SourceText())
}

func TestSearchReplaceToolTarget(t *testing.T) {
	cfg := &tools.SearchReplaceConfig{
		Pairs: []tools.ReplacePair{
			{Search: "monde", Replace: "terre"},
		},
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewSearchReplaceTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello world", resultBlock.SourceText()) // Source unchanged (no match).
	assert.Equal(t, "Bonjour le terre", resultBlock.TargetText(model.LocaleFrench))
}

func TestSearchReplaceToolSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.SearchReplaceConfig{
		Pairs: []tools.ReplacePair{
			{Search: "Hello", Replace: "Hi"},
		},
	}
	tl := tools.NewSearchReplaceTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello world", resultBlock.SourceText()) // Unchanged.
}

func TestSearchReplaceConfigValidation(t *testing.T) {
	// Empty search string.
	cfg := &tools.SearchReplaceConfig{
		Pairs: []tools.ReplacePair{
			{Search: "", Replace: "x"},
		},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty search")

	// Invalid regex.
	cfg = &tools.SearchReplaceConfig{
		Pairs: []tools.ReplacePair{
			{Search: "[invalid", Replace: "x", IsRegex: true},
		},
	}
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid regex")

	// Valid config.
	cfg = &tools.SearchReplaceConfig{
		Pairs: []tools.ReplacePair{
			{Search: `\d+`, Replace: "NUM", IsRegex: true},
			{Search: "foo", Replace: "bar"},
		},
	}
	err = cfg.Validate()
	assert.NoError(t, err)
}

func TestSearchReplaceToolNoPairs(t *testing.T) {
	cfg := &tools.SearchReplaceConfig{}
	tl := tools.NewSearchReplaceTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Hello world", resultBlock.SourceText()) // Unchanged.
}
