package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/flow"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/lib/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestWordCountToolAllLocales(t *testing.T) {
	// Empty locale → count all target locales.
	cfg := &tools.WordCountConfig{}
	tl := tools.NewWordCountTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	block.SetTargetText(model.LocaleGerman, "Hallo Welt")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// Source always counted.
	assert.Equal(t, "2", resultBlock.Properties[tools.PropWordCountSource])
	// Legacy single key should NOT be set.
	_, hasLegacy := resultBlock.Properties[tools.PropWordCountTarget]
	assert.False(t, hasLegacy)
	// Per-locale keys should be set.
	assert.Equal(t, "3", resultBlock.Properties[tools.PropWordCountTargetPrefix+"fr"])
	assert.Equal(t, "2", resultBlock.Properties[tools.PropWordCountTargetPrefix+"de"])
}

func TestWordCountToolSingleLocaleBackwardCompat(t *testing.T) {
	// With locale set → legacy behavior.
	cfg := &tools.WordCountConfig{Locale: model.LocaleFrench}
	tl := tools.NewWordCountTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	block.SetTargetText(model.LocaleGerman, "Hallo Welt")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "2", resultBlock.Properties[tools.PropWordCountSource])
	// Legacy single-locale key set.
	assert.Equal(t, "3", resultBlock.Properties[tools.PropWordCountTarget])
	// Per-locale keys should NOT be set.
	_, hasPerLocale := resultBlock.Properties[tools.PropWordCountTargetPrefix+"fr"]
	assert.False(t, hasPerLocale)
}

// --- WordCountCollector Tests ---

func TestWordCountCollector(t *testing.T) {
	wc := tools.NewWordCountCollector()

	item := &flow.FlowItem{
		Input:        &model.RawDocument{URI: "doc1.html"},
		TargetLocale: model.LocaleFrench,
	}

	block1 := model.NewBlock("tu1", "Hello beautiful world")
	block1.Properties[tools.PropWordCountSource] = "3"
	block1.Properties[tools.PropWordCountTarget] = "4"

	block2 := model.NewBlock("tu2", "Goodbye")
	block2.Properties[tools.PropWordCountSource] = "1"

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1"}},
		{Type: model.PartBlock, Resource: block1},
		{Type: model.PartBlock, Resource: block2},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}},
	}

	err := wc.Collect(context.Background(), item, parts)
	require.NoError(t, err)

	result, err := wc.Result()
	require.NoError(t, err)
	assert.Equal(t, "word-count", result.Name)

	summary := result.Data.(*tools.WordCountSummary)
	assert.Equal(t, 4, summary.TotalSourceWords)
	assert.Equal(t, 4, summary.TotalTargetWords[model.LocaleFrench])
	assert.Equal(t, 1, summary.DocumentCount)

	doc := summary.Documents["doc1.html"]
	assert.Equal(t, 4, doc.SourceWords)
	assert.Equal(t, 4, doc.TargetWords[model.LocaleFrench])
	assert.Equal(t, 2, doc.BlockCount)
}

func TestWordCountCollectorMultipleDocuments(t *testing.T) {
	wc := tools.NewWordCountCollector()

	for _, uri := range []string{"a.html", "b.html", "c.html"} {
		item := &flow.FlowItem{
			Input:        &model.RawDocument{URI: uri},
			TargetLocale: model.LocaleFrench,
		}
		block := model.NewBlock("tu1", "text")
		block.Properties[tools.PropWordCountSource] = "2"
		block.Properties[tools.PropWordCountTarget] = "3"

		parts := []*model.Part{
			{Type: model.PartBlock, Resource: block},
		}
		err := wc.Collect(context.Background(), item, parts)
		require.NoError(t, err)
	}

	result, err := wc.Result()
	require.NoError(t, err)

	summary := result.Data.(*tools.WordCountSummary)
	assert.Equal(t, 6, summary.TotalSourceWords)
	assert.Equal(t, 9, summary.TotalTargetWords[model.LocaleFrench])
	assert.Equal(t, 3, summary.DocumentCount)
	assert.Len(t, summary.Documents, 3)
}

func TestWordCountCollectorSkipsNonBlocks(t *testing.T) {
	wc := tools.NewWordCountCollector()

	item := &flow.FlowItem{
		Input: &model.RawDocument{URI: "doc.html"},
	}

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1"}},
		{Type: model.PartData, Resource: &model.Data{ID: "d1"}},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}},
	}

	err := wc.Collect(context.Background(), item, parts)
	require.NoError(t, err)

	result, err := wc.Result()
	require.NoError(t, err)

	summary := result.Data.(*tools.WordCountSummary)
	assert.Equal(t, 0, summary.TotalSourceWords)
	assert.Empty(t, summary.TotalTargetWords)
	assert.Equal(t, 0, summary.Documents["doc.html"].BlockCount)
}

func TestWordCountCollectorSkipsNonTranslatable(t *testing.T) {
	wc := tools.NewWordCountCollector()

	item := &flow.FlowItem{
		Input: &model.RawDocument{URI: "doc.html"},
	}

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	block.Properties[tools.PropWordCountSource] = "2"

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: block},
	}

	err := wc.Collect(context.Background(), item, parts)
	require.NoError(t, err)

	result, err := wc.Result()
	require.NoError(t, err)

	summary := result.Data.(*tools.WordCountSummary)
	assert.Equal(t, 0, summary.TotalSourceWords)
}

func TestWordCountCollectorPerLocaleProperties(t *testing.T) {
	wc := tools.NewWordCountCollector()

	item := &flow.FlowItem{
		Input: &model.RawDocument{URI: "doc.html"},
	}

	block := model.NewBlock("tu1", "Hello world")
	block.Properties[tools.PropWordCountSource] = "2"
	block.Properties[tools.PropWordCountTargetPrefix+"fr"] = "3"
	block.Properties[tools.PropWordCountTargetPrefix+"de"] = "2"

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: block},
	}

	err := wc.Collect(context.Background(), item, parts)
	require.NoError(t, err)

	result, err := wc.Result()
	require.NoError(t, err)

	summary := result.Data.(*tools.WordCountSummary)
	assert.Equal(t, 2, summary.TotalSourceWords)
	assert.Equal(t, 3, summary.TotalTargetWords[model.LocaleFrench])
	assert.Equal(t, 2, summary.TotalTargetWords[model.LocaleGerman])
	assert.Equal(t, 1, summary.DocumentCount)

	doc := summary.Documents["doc.html"]
	assert.Equal(t, 3, doc.TargetWords[model.LocaleFrench])
	assert.Equal(t, 2, doc.TargetWords[model.LocaleGerman])
}

func TestWordCountSummaryFormatTable(t *testing.T) {
	summary := &tools.WordCountSummary{
		TotalSourceWords: 10,
		TotalTargetWords: map[model.LocaleID]int{
			model.LocaleFrench: 12,
		},
		DocumentCount: 2,
		Documents: map[string]tools.DocumentWordCount{
			"a.html": {
				URI: "a.html", SourceWords: 5, BlockCount: 2,
				TargetWords: map[model.LocaleID]int{model.LocaleFrench: 6},
			},
			"b.html": {
				URI: "b.html", SourceWords: 5, BlockCount: 3,
				TargetWords: map[model.LocaleID]int{model.LocaleFrench: 6},
			},
		},
	}

	var buf strings.Builder
	summary.FormatTable(&buf)
	output := buf.String()

	// Should contain header.
	assert.Contains(t, output, "FILE")
	assert.Contains(t, output, "BLOCKS")
	assert.Contains(t, output, "SOURCE WORDS")
	assert.Contains(t, output, "TARGET (fr)")
	// Should contain document rows.
	assert.Contains(t, output, "a.html")
	assert.Contains(t, output, "b.html")
	// Should contain total row.
	assert.Contains(t, output, "Total (2 files)")
}
