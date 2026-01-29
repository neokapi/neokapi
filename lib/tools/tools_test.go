package tools_test

import (
	"context"
	"testing"

	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/lib/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// processPart is a helper that sends a single Part through a tool and returns the result.
func processPart(t *testing.T, tl interface {
	Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error
}, part *model.Part) *model.Part {
	t.Helper()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- part
	close(in)

	err := tl.Process(context.Background(), in, out)
	close(out)
	require.NoError(t, err)

	result := <-out
	require.NotNil(t, result)
	return result
}

// --- PseudoTranslateTool Tests ---

func TestPseudoTranslateTool(t *testing.T) {
	cfg := &tools.PseudoConfig{
		ExpansionPercent: 0,
		Prefix:           "[",
		Suffix:           "]",
		TargetLocale:     "qps",
	}
	tl := tools.NewPseudoTranslateTool(cfg)

	assert.Equal(t, "pseudo-translate", tl.Name())
	assert.Contains(t, tl.Description(), "pseudo")

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	targetText := resultBlock.TargetText("qps")

	// Should be wrapped in brackets.
	assert.True(t, len(targetText) > 0)
	assert.Equal(t, '[', rune(targetText[0]))
	assert.Equal(t, ']', rune(targetText[len(targetText)-1]))

	// Should contain accented characters, not the original ASCII.
	assert.NotContains(t, targetText, "Hello")
	// The 'e' in "Hello" should have been replaced with 'é'.
	assert.Contains(t, targetText, "\u00e9")
	// The 'o' in "Hello" should have been replaced with 'ö'.
	assert.Contains(t, targetText, "\u00f6")
}

func TestPseudoTranslateToolWithExpansion(t *testing.T) {
	cfg := &tools.PseudoConfig{
		ExpansionPercent: 50,
		Prefix:           "[",
		Suffix:           "]",
		TargetLocale:     "qps",
	}
	tl := tools.NewPseudoTranslateTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	targetText := resultBlock.TargetText("qps")

	// With 50% expansion on 5 chars, should add padding of 2 tildes + space.
	// Total should be longer than just accented + brackets.
	assert.Contains(t, targetText, "~~")
	assert.True(t, len([]rune(targetText)) > len([]rune("[Ĥéļļö]")))
}

func TestPseudoTranslateToolSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.PseudoConfig{
		TargetLocale: "qps",
		Prefix:       "[",
		Suffix:       "]",
	}
	tl := tools.NewPseudoTranslateTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget("qps"))
}

func TestPseudoTranslateToolCustomPrefixSuffix(t *testing.T) {
	cfg := &tools.PseudoConfig{
		Prefix:       "<<",
		Suffix:       ">>",
		TargetLocale: "qps",
	}
	tl := tools.NewPseudoTranslateTool(cfg)

	block := model.NewBlock("tu1", "Test")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	targetText := resultBlock.TargetText("qps")

	assert.True(t, len(targetText) >= 4)
	assert.Equal(t, "<<", targetText[:2])
	assert.Equal(t, ">>", targetText[len(targetText)-2:])
}

func TestPseudoConfigValidation(t *testing.T) {
	cfg := &tools.PseudoConfig{ExpansionPercent: -1, TargetLocale: "qps"}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ExpansionPercent")

	cfg.ExpansionPercent = 0
	cfg.TargetLocale = ""
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.TargetLocale = "qps"
	err = cfg.Validate()
	assert.NoError(t, err)
}

// --- WordCountTool Tests ---

func TestWordCountTool(t *testing.T) {
	cfg := &tools.WordCountConfig{
		Locale: model.LocaleFrench,
	}
	tl := tools.NewWordCountTool(cfg)

	assert.Equal(t, "word-count", tl.Name())

	block := model.NewBlock("tu1", "Hello beautiful world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le beau monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "3", resultBlock.Properties[tools.PropWordCountSource])
	assert.Equal(t, "4", resultBlock.Properties[tools.PropWordCountTarget])
}

func TestWordCountToolSourceOnly(t *testing.T) {
	cfg := &tools.WordCountConfig{}
	tl := tools.NewWordCountTool(cfg)

	block := model.NewBlock("tu1", "One two three four")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "4", resultBlock.Properties[tools.PropWordCountSource])
	// No target set, no locale configured -> no target count.
	_, hasTargetCount := resultBlock.Properties[tools.PropWordCountTarget]
	assert.False(t, hasTargetCount)
}

func TestWordCountToolEmptyText(t *testing.T) {
	cfg := &tools.WordCountConfig{}
	tl := tools.NewWordCountTool(cfg)

	block := model.NewBlock("tu1", "")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "0", resultBlock.Properties[tools.PropWordCountSource])
}

func TestWordCountToolSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.WordCountConfig{}
	tl := tools.NewWordCountTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasSourceCount := resultBlock.Properties[tools.PropWordCountSource]
	assert.False(t, hasSourceCount)
}

// --- SearchReplaceTool Tests ---

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

// --- CharCountTool Tests ---

func TestCharCountTool(t *testing.T) {
	cfg := &tools.CharCountConfig{
		Locale: model.LocaleFrench,
	}
	tl := tools.NewCharCountTool(cfg)

	assert.Equal(t, "char-count", tl.Name())

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// "Hello world" = 11 chars, 10 without spaces.
	assert.Equal(t, "11", resultBlock.Properties[tools.PropCharCountSource])
	assert.Equal(t, "10", resultBlock.Properties[tools.PropCharCountSourceNospace])
	// "Bonjour le monde" = 16 chars, 14 without spaces.
	assert.Equal(t, "16", resultBlock.Properties[tools.PropCharCountTarget])
	assert.Equal(t, "14", resultBlock.Properties[tools.PropCharCountTargetNospace])
}

func TestCharCountToolSourceOnly(t *testing.T) {
	cfg := &tools.CharCountConfig{}
	tl := tools.NewCharCountTool(cfg)

	block := model.NewBlock("tu1", "Test text")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// "Test text" = 9 chars, 8 without spaces.
	assert.Equal(t, "9", resultBlock.Properties[tools.PropCharCountSource])
	assert.Equal(t, "8", resultBlock.Properties[tools.PropCharCountSourceNospace])
	// No target count since no locale and no target.
	_, hasTargetCount := resultBlock.Properties[tools.PropCharCountTarget]
	assert.False(t, hasTargetCount)
}

func TestCharCountToolUnicode(t *testing.T) {
	cfg := &tools.CharCountConfig{}
	tl := tools.NewCharCountTool(cfg)

	// Unicode text: "Bonjour" = 7 chars.
	block := model.NewBlock("tu1", "\u00e9l\u00e8ve") // "eleve" with accents.
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "5", resultBlock.Properties[tools.PropCharCountSource])
	assert.Equal(t, "5", resultBlock.Properties[tools.PropCharCountSourceNospace])
}

func TestCharCountToolSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.CharCountConfig{}
	tl := tools.NewCharCountTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasSourceCount := resultBlock.Properties[tools.PropCharCountSource]
	assert.False(t, hasSourceCount)
}

func TestCharCountToolEmptyText(t *testing.T) {
	cfg := &tools.CharCountConfig{}
	tl := tools.NewCharCountTool(cfg)

	block := model.NewBlock("tu1", "")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "0", resultBlock.Properties[tools.PropCharCountSource])
	assert.Equal(t, "0", resultBlock.Properties[tools.PropCharCountSourceNospace])
}
