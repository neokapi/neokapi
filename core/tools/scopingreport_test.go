package tools_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopingReportToolRepetition(t *testing.T) {
	t.Parallel()
	cfg := &tools.ScopingReportConfig{}
	tl := tools.NewScopingReportTool(cfg)

	assert.Equal(t, "scoping-report", tl.Name())

	block := model.NewBlock("tu1", "Hello world")
	block.SetAnno(string(model.AnnoRepetition), &tools.RepetitionAnnotation{Status: "repetition"})
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "repetition", resultBlock.Properties[tools.PropScopingCategory])
}

func TestScopingReportToolExactMatch(t *testing.T) {
	t.Parallel()
	cfg := &tools.ScopingReportConfig{}
	tl := tools.NewScopingReportTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Properties["diff-leverage-status"] = "unchanged"
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "exact-match", resultBlock.Properties[tools.PropScopingCategory])
}

func TestScopingReportToolFuzzyMatch(t *testing.T) {
	t.Parallel()
	cfg := &tools.ScopingReportConfig{}
	tl := tools.NewScopingReportTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Properties["diff-leverage-status"] = "leveraged"
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "fuzzy-match", resultBlock.Properties[tools.PropScopingCategory])
}

func TestScopingReportToolNew(t *testing.T) {
	t.Parallel()
	cfg := &tools.ScopingReportConfig{}
	tl := tools.NewScopingReportTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "new", resultBlock.Properties[tools.PropScopingCategory])
}

func TestScopingReportToolSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	cfg := &tools.ScopingReportConfig{}
	tl := tools.NewScopingReportTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasCategory := resultBlock.Properties[tools.PropScopingCategory]
	assert.False(t, hasCategory)
}

// --- ScopingCollector Tests ---

func TestScopingCollector(t *testing.T) {
	t.Parallel()
	sc := tools.NewScopingCollector()

	item := &flow.Item{
		Input:        &model.RawDocument{URI: "doc1.html"},
		TargetLocale: model.LocaleFrench,
	}

	block1 := model.NewBlock("tu1", "Hello beautiful world")
	block1.Properties[tools.PropScopingCategory] = "new"
	block1.SetAnno(string(model.AnnoWordCount), &tools.WordCountAnnotation{Source: 3})

	block2 := model.NewBlock("tu2", "Goodbye")
	block2.Properties[tools.PropScopingCategory] = "repetition"
	block2.SetAnno(string(model.AnnoWordCount), &tools.WordCountAnnotation{Source: 1})

	block3 := model.NewBlock("tu3", "See you later")
	block3.Properties[tools.PropScopingCategory] = "new"
	block3.SetAnno(string(model.AnnoWordCount), &tools.WordCountAnnotation{Source: 3})

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1"}},
		{Type: model.PartBlock, Resource: block1},
		{Type: model.PartBlock, Resource: block2},
		{Type: model.PartBlock, Resource: block3},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}},
	}

	err := sc.Collect(t.Context(), item, parts)
	require.NoError(t, err)

	result, err := sc.Result()
	require.NoError(t, err)
	assert.Equal(t, "scoping-report", result.Name)

	summary := result.Data.(*tools.ScopingSummary)
	assert.Equal(t, 7, summary.TotalWords)
	assert.Equal(t, 3, summary.TotalBlocks)
	assert.Equal(t, 1, summary.DocumentCount)

	assert.Equal(t, 2, summary.Categories["new"].BlockCount)
	assert.Equal(t, 6, summary.Categories["new"].WordCount)
	assert.Equal(t, 1, summary.Categories["repetition"].BlockCount)
	assert.Equal(t, 1, summary.Categories["repetition"].WordCount)
}

func TestScopingCollectorMultipleDocuments(t *testing.T) {
	t.Parallel()
	sc := tools.NewScopingCollector()

	for _, uri := range []string{"a.html", "b.html"} {
		item := &flow.Item{
			Input: &model.RawDocument{URI: uri},
		}
		block := model.NewBlock("tu1", "text")
		block.Properties[tools.PropScopingCategory] = "exact-match"
		block.SetAnno(string(model.AnnoWordCount), &tools.WordCountAnnotation{Source: 2})

		parts := []*model.Part{
			{Type: model.PartBlock, Resource: block},
		}
		err := sc.Collect(t.Context(), item, parts)
		require.NoError(t, err)
	}

	result, err := sc.Result()
	require.NoError(t, err)

	summary := result.Data.(*tools.ScopingSummary)
	assert.Equal(t, 4, summary.TotalWords)
	assert.Equal(t, 2, summary.TotalBlocks)
	assert.Equal(t, 2, summary.DocumentCount)
	assert.Equal(t, 2, summary.Categories["exact-match"].BlockCount)
	assert.Equal(t, 4, summary.Categories["exact-match"].WordCount)
}

func TestScopingCollectorSkipsNonBlocks(t *testing.T) {
	t.Parallel()
	sc := tools.NewScopingCollector()

	item := &flow.Item{
		Input: &model.RawDocument{URI: "doc.html"},
	}

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1"}},
		{Type: model.PartData, Resource: &model.Data{ID: "d1"}},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}},
	}

	err := sc.Collect(t.Context(), item, parts)
	require.NoError(t, err)

	result, err := sc.Result()
	require.NoError(t, err)

	summary := result.Data.(*tools.ScopingSummary)
	assert.Equal(t, 0, summary.TotalWords)
	assert.Equal(t, 0, summary.TotalBlocks)
	assert.Empty(t, summary.Categories)
}

func TestScopingCollectorSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	sc := tools.NewScopingCollector()

	item := &flow.Item{
		Input: &model.RawDocument{URI: "doc.html"},
	}

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	block.Properties[tools.PropScopingCategory] = "new"
	block.SetAnno(string(model.AnnoWordCount), &tools.WordCountAnnotation{Source: 2})

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: block},
	}

	err := sc.Collect(t.Context(), item, parts)
	require.NoError(t, err)

	result, err := sc.Result()
	require.NoError(t, err)

	summary := result.Data.(*tools.ScopingSummary)
	assert.Equal(t, 0, summary.TotalWords)
	assert.Equal(t, 0, summary.TotalBlocks)
}

func TestScopingSummaryFormatTable(t *testing.T) {
	t.Parallel()
	summary := &tools.ScopingSummary{
		TotalWords:    15,
		TotalBlocks:   5,
		DocumentCount: 2,
		Categories: map[string]*tools.ScopingCategory{
			"new":        {Name: "new", WordCount: 10, BlockCount: 3},
			"repetition": {Name: "repetition", WordCount: 5, BlockCount: 2},
		},
	}

	var buf strings.Builder
	summary.FormatTable(&buf)
	output := buf.String()

	assert.Contains(t, output, "CATEGORY")
	assert.Contains(t, output, "BLOCKS")
	assert.Contains(t, output, "WORDS")
	assert.Contains(t, output, "new")
	assert.Contains(t, output, "repetition")
	assert.Contains(t, output, "Total (2 files)")
}

func TestScopingCollectorDefaultCategoryWhenMissing(t *testing.T) {
	t.Parallel()
	sc := tools.NewScopingCollector()

	item := &flow.Item{
		Input: &model.RawDocument{URI: "doc.html"},
	}

	// Block with no scoping-category property — should default to "new".
	block := model.NewBlock("tu1", "Hello world")
	block.SetAnno(string(model.AnnoWordCount), &tools.WordCountAnnotation{Source: 2})

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: block},
	}

	err := sc.Collect(t.Context(), item, parts)
	require.NoError(t, err)

	result, err := sc.Result()
	require.NoError(t, err)

	summary := result.Data.(*tools.ScopingSummary)
	assert.Equal(t, 1, summary.Categories["new"].BlockCount)
	assert.Equal(t, 2, summary.Categories["new"].WordCount)
}
