package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamingWordCountCollector_SingleDocument(t *testing.T) {
	t.Parallel()
	wc := tools.NewStreamingWordCountCollector()

	// Set document context.
	item := &flow.Item{
		Input: &model.RawDocument{URI: "doc1.html"},
	}
	err := wc.Collect(t.Context(), item, nil)
	require.NoError(t, err)

	// Simulate observing parts inline.
	block1 := model.NewBlock("tu1", "Hello beautiful world")
	block1.SetAnno(string(model.FacetWordCount), &tools.WordCountFacet{Source: 3, Targets: map[model.LocaleID]int{model.LocaleFrench: 4}})
	wc.Observe(&model.Part{Type: model.PartBlock, Resource: block1})

	block2 := model.NewBlock("tu2", "Goodbye")
	block2.SetAnno(string(model.FacetWordCount), &tools.WordCountFacet{Source: 1})
	wc.Observe(&model.Part{Type: model.PartBlock, Resource: block2})

	// Non-block parts should be ignored.
	wc.Observe(&model.Part{Type: model.PartData, Resource: &model.Data{ID: "d1"}})
	wc.Observe(&model.Part{Type: model.PartLayerStart, Resource: &model.Layer{ID: "l1"}})

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

func TestStreamingWordCountCollector_MultipleDocuments(t *testing.T) {
	t.Parallel()
	wc := tools.NewStreamingWordCountCollector()

	for _, uri := range []string{"a.html", "b.html"} {
		item := &flow.Item{
			Input: &model.RawDocument{URI: uri},
		}
		err := wc.Collect(t.Context(), item, nil)
		require.NoError(t, err)

		block := model.NewBlock("tu1", "text")
		block.SetAnno(string(model.FacetWordCount), &tools.WordCountFacet{Source: 2, Targets: map[model.LocaleID]int{model.LocaleFrench: 3}})
		wc.Observe(&model.Part{Type: model.PartBlock, Resource: block})
	}

	result, err := wc.Result()
	require.NoError(t, err)

	summary := result.Data.(*tools.WordCountSummary)
	assert.Equal(t, 4, summary.TotalSourceWords)
	assert.Equal(t, 6, summary.TotalTargetWords[model.LocaleFrench])
	assert.Equal(t, 2, summary.DocumentCount)
}

func TestStreamingWordCountCollector_WithTappingTool(t *testing.T) {
	t.Parallel()
	// End-to-end: WordCountTool + TappingTool + StreamingWordCountCollector.
	wc := tools.NewStreamingWordCountCollector()

	// Set document context.
	item := &flow.Item{
		Input: &model.RawDocument{URI: "test.json"},
	}
	err := wc.Collect(t.Context(), item, nil)
	require.NoError(t, err)

	// Create the word count tool, then wrap with TappingTool.
	wordCountTool := tools.NewWordCountTool(&tools.WordCountConfig{})
	tapped := flow.NewTappingTool(wordCountTool, wc)

	// Process parts through the tapped tool.
	parts := []*model.Part{
		{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello world")},
		{Type: model.PartData, Resource: &model.Data{ID: "d1", Name: "structural"}},
		{Type: model.PartBlock, Resource: model.NewBlock("tu2", "One two three")},
	}

	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))
	for _, p := range parts {
		in <- p
	}
	close(in)

	err = tapped.Process(t.Context(), in, out)
	require.NoError(t, err)
	close(out)

	// Verify all parts passed through.
	var results []*model.Part
	for p := range out {
		results = append(results, p)
	}
	assert.Len(t, results, 3)

	// Verify streaming collector accumulated counts.
	result, err := wc.Result()
	require.NoError(t, err)

	summary := result.Data.(*tools.WordCountSummary)
	assert.Equal(t, 5, summary.TotalSourceWords) // 2 + 3
	assert.Equal(t, 1, summary.DocumentCount)

	doc := summary.Documents["test.json"]
	assert.Equal(t, 5, doc.SourceWords)
	assert.Equal(t, 2, doc.BlockCount)
}

func TestStreamingWordCountCollector_SkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	wc := tools.NewStreamingWordCountCollector()

	item := &flow.Item{
		Input: &model.RawDocument{URI: "doc.html"},
	}
	err := wc.Collect(t.Context(), item, nil)
	require.NoError(t, err)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	block.SetAnno(string(model.FacetWordCount), &tools.WordCountFacet{Source: 2})
	wc.Observe(&model.Part{Type: model.PartBlock, Resource: block})

	result, err := wc.Result()
	require.NoError(t, err)

	summary := result.Data.(*tools.WordCountSummary)
	assert.Equal(t, 0, summary.TotalSourceWords)
}

// Ensure StreamingWordCountCollector implements flow.StreamingCollector.
func TestStreamingWordCountCollector_Interface(t *testing.T) {
	t.Parallel()
	var _ flow.StreamingCollector = tools.NewStreamingWordCountCollector()
	var _ flow.Collector = tools.NewStreamingWordCountCollector()

	// Ensure it can be used as a flow.Collector and detected as StreamingCollector.
	var c flow.Collector = tools.NewStreamingWordCountCollector()
	_, ok := c.(flow.StreamingCollector)
	assert.True(t, ok)
}
