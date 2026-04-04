package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepetitionAnalysisToolName(t *testing.T) {
	t.Parallel()
	cfg := &tools.RepetitionAnalysisConfig{CaseSensitive: true}
	tl := tools.NewRepetitionAnalysisTool(cfg)
	assert.Equal(t, "repetition-analysis", tl.Name())
}

func TestRepetitionAnalysisAllUnique(t *testing.T) {
	t.Parallel()
	cfg := &tools.RepetitionAnalysisConfig{CaseSensitive: true}
	tl := tools.NewRepetitionAnalysisTool(cfg)

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello world")},
		{Type: model.PartBlock, Resource: model.NewBlock("tu2", "Goodbye world")},
		{Type: model.PartBlock, Resource: model.NewBlock("tu3", "Something else")},
	}

	results := processMultipleParts(t, tl, parts)
	require.Len(t, results, 3)

	for i, r := range results {
		block := r.Resource.(*model.Block)
		assert.Equal(t, "first-occurrence", block.Properties[tools.PropRepetitionStatus],
			"block %d should be first-occurrence", i)
		assert.Equal(t, "1", block.Properties[tools.PropRepetitionCount],
			"block %d count should be 1", i)
		assert.Equal(t, "1", block.Properties[tools.PropRepetitionIndex],
			"block %d index should be 1", i)
	}

	// All unique texts should have different group keys.
	groups := make(map[string]bool)
	for _, r := range results {
		block := r.Resource.(*model.Block)
		groups[block.Properties[tools.PropRepetitionGroup]] = true
	}
	assert.Len(t, groups, 3, "all blocks should have different group keys")
}

func TestRepetitionAnalysisThreeIdentical(t *testing.T) {
	t.Parallel()
	cfg := &tools.RepetitionAnalysisConfig{CaseSensitive: true}
	tl := tools.NewRepetitionAnalysisTool(cfg)

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello world")},
		{Type: model.PartBlock, Resource: model.NewBlock("tu2", "Hello world")},
		{Type: model.PartBlock, Resource: model.NewBlock("tu3", "Hello world")},
	}

	results := processMultipleParts(t, tl, parts)
	require.Len(t, results, 3)

	b0 := results[0].Resource.(*model.Block)
	b1 := results[1].Resource.(*model.Block)
	b2 := results[2].Resource.(*model.Block)

	// First is "first-occurrence".
	assert.Equal(t, "first-occurrence", b0.Properties[tools.PropRepetitionStatus])
	assert.Equal(t, "1", b0.Properties[tools.PropRepetitionCount])
	assert.Equal(t, "1", b0.Properties[tools.PropRepetitionIndex])

	// Second is "repetition".
	assert.Equal(t, "repetition", b1.Properties[tools.PropRepetitionStatus])
	assert.Equal(t, "2", b1.Properties[tools.PropRepetitionCount])
	assert.Equal(t, "2", b1.Properties[tools.PropRepetitionIndex])

	// Third is "repetition".
	assert.Equal(t, "repetition", b2.Properties[tools.PropRepetitionStatus])
	assert.Equal(t, "3", b2.Properties[tools.PropRepetitionCount])
	assert.Equal(t, "3", b2.Properties[tools.PropRepetitionIndex])

	// All share the same group key.
	assert.Equal(t, b0.Properties[tools.PropRepetitionGroup], b1.Properties[tools.PropRepetitionGroup])
	assert.Equal(t, b1.Properties[tools.PropRepetitionGroup], b2.Properties[tools.PropRepetitionGroup])
}

func TestRepetitionAnalysisCaseInsensitive(t *testing.T) {
	t.Parallel()
	cfg := &tools.RepetitionAnalysisConfig{CaseSensitive: false}
	tl := tools.NewRepetitionAnalysisTool(cfg)

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello World")},
		{Type: model.PartBlock, Resource: model.NewBlock("tu2", "hello world")},
		{Type: model.PartBlock, Resource: model.NewBlock("tu3", "HELLO WORLD")},
	}

	results := processMultipleParts(t, tl, parts)
	require.Len(t, results, 3)

	b0 := results[0].Resource.(*model.Block)
	b1 := results[1].Resource.(*model.Block)
	b2 := results[2].Resource.(*model.Block)

	assert.Equal(t, "first-occurrence", b0.Properties[tools.PropRepetitionStatus])
	assert.Equal(t, "repetition", b1.Properties[tools.PropRepetitionStatus])
	assert.Equal(t, "repetition", b2.Properties[tools.PropRepetitionStatus])

	// All share the same group key.
	assert.Equal(t, b0.Properties[tools.PropRepetitionGroup], b1.Properties[tools.PropRepetitionGroup])
	assert.Equal(t, b1.Properties[tools.PropRepetitionGroup], b2.Properties[tools.PropRepetitionGroup])
}

func TestRepetitionAnalysisCaseSensitive(t *testing.T) {
	t.Parallel()
	cfg := &tools.RepetitionAnalysisConfig{CaseSensitive: true}
	tl := tools.NewRepetitionAnalysisTool(cfg)

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello World")},
		{Type: model.PartBlock, Resource: model.NewBlock("tu2", "hello world")},
	}

	results := processMultipleParts(t, tl, parts)
	require.Len(t, results, 2)

	b0 := results[0].Resource.(*model.Block)
	b1 := results[1].Resource.(*model.Block)

	// Case-sensitive: these are different texts.
	assert.Equal(t, "first-occurrence", b0.Properties[tools.PropRepetitionStatus])
	assert.Equal(t, "first-occurrence", b1.Properties[tools.PropRepetitionStatus])
	assert.NotEqual(t, b0.Properties[tools.PropRepetitionGroup], b1.Properties[tools.PropRepetitionGroup])
}

func TestRepetitionAnalysisSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	cfg := &tools.RepetitionAnalysisConfig{CaseSensitive: true}
	tl := tools.NewRepetitionAnalysisTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	parts := []*model.Part{
		{Type: model.PartBlock, Resource: block},
	}

	results := processMultipleParts(t, tl, parts)
	require.Len(t, results, 1)

	resultBlock := results[0].Resource.(*model.Block)
	_, hasStatus := resultBlock.Properties[tools.PropRepetitionStatus]
	assert.False(t, hasStatus, "non-translatable block should not have repetition status")
}

func TestRepetitionAnalysisMixedUniqueAndRepeated(t *testing.T) {
	t.Parallel()
	cfg := &tools.RepetitionAnalysisConfig{CaseSensitive: true}
	tl := tools.NewRepetitionAnalysisTool(cfg)

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Alpha")},
		{Type: model.PartBlock, Resource: model.NewBlock("tu2", "Beta")},
		{Type: model.PartBlock, Resource: model.NewBlock("tu3", "Alpha")},
		{Type: model.PartBlock, Resource: model.NewBlock("tu4", "Gamma")},
		{Type: model.PartBlock, Resource: model.NewBlock("tu5", "Beta")},
	}

	results := processMultipleParts(t, tl, parts)
	require.Len(t, results, 5)

	statuses := make([]string, len(results))
	for i, r := range results {
		block := r.Resource.(*model.Block)
		statuses[i] = block.Properties[tools.PropRepetitionStatus]
	}

	assert.Equal(t, "first-occurrence", statuses[0], "Alpha first")
	assert.Equal(t, "first-occurrence", statuses[1], "Beta first")
	assert.Equal(t, "repetition", statuses[2], "Alpha repeated")
	assert.Equal(t, "first-occurrence", statuses[3], "Gamma first")
	assert.Equal(t, "repetition", statuses[4], "Beta repeated")

	// Alpha group should link tu1 and tu3.
	b0 := results[0].Resource.(*model.Block)
	b2 := results[2].Resource.(*model.Block)
	assert.Equal(t, b0.Properties[tools.PropRepetitionGroup], b2.Properties[tools.PropRepetitionGroup])

	// Beta group should link tu2 and tu5.
	b1 := results[1].Resource.(*model.Block)
	b4 := results[4].Resource.(*model.Block)
	assert.Equal(t, b1.Properties[tools.PropRepetitionGroup], b4.Properties[tools.PropRepetitionGroup])

	// Alpha and Beta groups should differ.
	assert.NotEqual(t, b0.Properties[tools.PropRepetitionGroup], b1.Properties[tools.PropRepetitionGroup])
}

func TestRepetitionAnalysisPassesThroughNonBlocks(t *testing.T) {
	t.Parallel()
	cfg := &tools.RepetitionAnalysisConfig{CaseSensitive: true}
	tl := tools.NewRepetitionAnalysisTool(cfg)

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1"}},
		{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")},
		{Type: model.PartData, Resource: &model.Data{ID: "d1"}},
		{Type: model.PartBlock, Resource: model.NewBlock("tu2", "Hello")},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}},
	}

	results := processMultipleParts(t, tl, parts)
	require.Len(t, results, 5)

	// Non-block parts pass through unchanged.
	assert.Equal(t, model.PartLayerStart, results[0].Type)
	assert.Equal(t, model.PartData, results[2].Type)
	assert.Equal(t, model.PartLayerEnd, results[4].Type)

	// Block parts are annotated.
	b0 := results[1].Resource.(*model.Block)
	b1 := results[3].Resource.(*model.Block)
	assert.Equal(t, "first-occurrence", b0.Properties[tools.PropRepetitionStatus])
	assert.Equal(t, "repetition", b1.Properties[tools.PropRepetitionStatus])
}

func TestRepetitionAnalysisWhitespaceNormalization(t *testing.T) {
	t.Parallel()
	cfg := &tools.RepetitionAnalysisConfig{CaseSensitive: true}
	tl := tools.NewRepetitionAnalysisTool(cfg)

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: model.NewBlock("tu1", "  Hello world  ")},
		{Type: model.PartBlock, Resource: model.NewBlock("tu2", "Hello world")},
	}

	results := processMultipleParts(t, tl, parts)
	require.Len(t, results, 2)

	b0 := results[0].Resource.(*model.Block)
	b1 := results[1].Resource.(*model.Block)

	// After trimming, these should be considered the same.
	assert.Equal(t, "first-occurrence", b0.Properties[tools.PropRepetitionStatus])
	assert.Equal(t, "repetition", b1.Properties[tools.PropRepetitionStatus])
	assert.Equal(t, b0.Properties[tools.PropRepetitionGroup], b1.Properties[tools.PropRepetitionGroup])
}

func TestRepetitionAnalysisConfigToolName(t *testing.T) {
	t.Parallel()
	cfg := &tools.RepetitionAnalysisConfig{}
	assert.Equal(t, "repetition-analysis", cfg.ToolName())
}

func TestRepetitionAnalysisConfigReset(t *testing.T) {
	t.Parallel()
	cfg := &tools.RepetitionAnalysisConfig{CaseSensitive: false}
	cfg.Reset()
	assert.True(t, cfg.CaseSensitive, "Reset should set CaseSensitive to true")
}

func TestRepetitionAnalysisConfigValidate(t *testing.T) {
	t.Parallel()
	cfg := &tools.RepetitionAnalysisConfig{}
	require.NoError(t, cfg.Validate())
}
