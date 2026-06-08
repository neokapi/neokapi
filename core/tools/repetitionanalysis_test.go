package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repFacet fetches the repetition facet from a block, requiring it to be present.
func repFacet(t *testing.T, b *model.Block) *tools.RepetitionFacet {
	t.Helper()
	rf, ok := model.AnnoAs[*tools.RepetitionFacet](b, string(model.FacetRepetition))
	require.True(t, ok, "block should have repetition facet")
	return rf
}

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
		rf := repFacet(t, r.Resource.(*model.Block))
		assert.Equal(t, "first-occurrence", rf.Status,
			"block %d should be first-occurrence", i)
		assert.Equal(t, 1, rf.Count,
			"block %d count should be 1", i)
		assert.Equal(t, 1, rf.Index,
			"block %d index should be 1", i)
	}

	// All unique texts should have different group keys.
	groups := make(map[string]bool)
	for _, r := range results {
		groups[repFacet(t, r.Resource.(*model.Block)).Group] = true
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

	rf0 := repFacet(t, results[0].Resource.(*model.Block))
	rf1 := repFacet(t, results[1].Resource.(*model.Block))
	rf2 := repFacet(t, results[2].Resource.(*model.Block))

	// First is "first-occurrence".
	assert.Equal(t, "first-occurrence", rf0.Status)
	assert.Equal(t, 1, rf0.Count)
	assert.Equal(t, 1, rf0.Index)

	// Second is "repetition".
	assert.Equal(t, "repetition", rf1.Status)
	assert.Equal(t, 2, rf1.Count)
	assert.Equal(t, 2, rf1.Index)

	// Third is "repetition".
	assert.Equal(t, "repetition", rf2.Status)
	assert.Equal(t, 3, rf2.Count)
	assert.Equal(t, 3, rf2.Index)

	// All share the same group key.
	assert.Equal(t, rf0.Group, rf1.Group)
	assert.Equal(t, rf1.Group, rf2.Group)
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

	rf0 := repFacet(t, results[0].Resource.(*model.Block))
	rf1 := repFacet(t, results[1].Resource.(*model.Block))
	rf2 := repFacet(t, results[2].Resource.(*model.Block))

	assert.Equal(t, "first-occurrence", rf0.Status)
	assert.Equal(t, "repetition", rf1.Status)
	assert.Equal(t, "repetition", rf2.Status)

	// All share the same group key.
	assert.Equal(t, rf0.Group, rf1.Group)
	assert.Equal(t, rf1.Group, rf2.Group)
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

	rf0 := repFacet(t, results[0].Resource.(*model.Block))
	rf1 := repFacet(t, results[1].Resource.(*model.Block))

	// Case-sensitive: these are different texts.
	assert.Equal(t, "first-occurrence", rf0.Status)
	assert.Equal(t, "first-occurrence", rf1.Status)
	assert.NotEqual(t, rf0.Group, rf1.Group)
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
	_, hasStatus := model.AnnoAs[*tools.RepetitionFacet](resultBlock, string(model.FacetRepetition))
	assert.False(t, hasStatus, "non-translatable block should not have repetition facet")
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
		statuses[i] = repFacet(t, r.Resource.(*model.Block)).Status
	}

	assert.Equal(t, "first-occurrence", statuses[0], "Alpha first")
	assert.Equal(t, "first-occurrence", statuses[1], "Beta first")
	assert.Equal(t, "repetition", statuses[2], "Alpha repeated")
	assert.Equal(t, "first-occurrence", statuses[3], "Gamma first")
	assert.Equal(t, "repetition", statuses[4], "Beta repeated")

	// Alpha group should link tu1 and tu3.
	rf0 := repFacet(t, results[0].Resource.(*model.Block))
	rf2 := repFacet(t, results[2].Resource.(*model.Block))
	assert.Equal(t, rf0.Group, rf2.Group)

	// Beta group should link tu2 and tu5.
	rf1 := repFacet(t, results[1].Resource.(*model.Block))
	rf4 := repFacet(t, results[4].Resource.(*model.Block))
	assert.Equal(t, rf1.Group, rf4.Group)

	// Alpha and Beta groups should differ.
	assert.NotEqual(t, rf0.Group, rf1.Group)
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
	rf0 := repFacet(t, results[1].Resource.(*model.Block))
	rf1 := repFacet(t, results[3].Resource.(*model.Block))
	assert.Equal(t, "first-occurrence", rf0.Status)
	assert.Equal(t, "repetition", rf1.Status)
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

	rf0 := repFacet(t, results[0].Resource.(*model.Block))
	rf1 := repFacet(t, results[1].Resource.(*model.Block))

	// After trimming, these should be considered the same.
	assert.Equal(t, "first-occurrence", rf0.Status)
	assert.Equal(t, "repetition", rf1.Status)
	assert.Equal(t, rf0.Group, rf1.Group)
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
