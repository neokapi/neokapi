package tools_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSegCountCollector(t *testing.T) {
	t.Parallel()
	sc := tools.NewSegCountCollector()

	item := &flow.Item{
		Input:        &model.RawDocument{URI: "doc1.html"},
		TargetLocale: model.LocaleFrench,
	}

	block1 := model.NewBlock("tu1", "Hello. World.")
	block1.SetAnno(string(model.AnnoSegCount), &tools.SegCountFacet{Source: 2, Target: 2})

	block2 := model.NewBlock("tu2", "Goodbye")
	block2.SetAnno(string(model.AnnoSegCount), &tools.SegCountFacet{Source: 1})

	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1"}},
		{Type: model.PartBlock, Resource: block1},
		{Type: model.PartBlock, Resource: block2},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}},
	}

	require.NoError(t, sc.Collect(t.Context(), item, parts))

	result, err := sc.Result()
	require.NoError(t, err)
	assert.Equal(t, "segment-count", result.Name)

	summary := result.Data.(*tools.SegCountSummary)
	assert.Equal(t, 3, summary.TotalSourceSegments)
	assert.Equal(t, 2, summary.TotalTargetSegments)
	assert.Equal(t, 1, summary.DocumentCount)

	doc := summary.Documents["doc1.html"]
	assert.Equal(t, 3, doc.SourceSegments)
	assert.Equal(t, 2, doc.TargetSegments)
	assert.Equal(t, 2, doc.BlockCount)
}

func TestSegCountCollectorMultipleDocuments(t *testing.T) {
	t.Parallel()
	sc := tools.NewSegCountCollector()

	for _, uri := range []string{"a.html", "b.html", "c.html"} {
		item := &flow.Item{Input: &model.RawDocument{URI: uri}}
		block := model.NewBlock("tu1", "Sentence one. Sentence two.")
		block.SetAnno(string(model.AnnoSegCount), &tools.SegCountFacet{Source: 2})
		parts := []*model.Part{{Type: model.PartBlock, Resource: block}}
		require.NoError(t, sc.Collect(t.Context(), item, parts))
	}

	result, err := sc.Result()
	require.NoError(t, err)

	summary := result.Data.(*tools.SegCountSummary)
	assert.Equal(t, 6, summary.TotalSourceSegments)
	assert.Equal(t, 3, summary.DocumentCount)
	assert.Len(t, summary.Documents, 3)
}

func TestSegCountCollectorSkipsNonBlocks(t *testing.T) {
	t.Parallel()
	sc := tools.NewSegCountCollector()

	item := &flow.Item{Input: &model.RawDocument{URI: "doc.html"}}
	parts := []*model.Part{
		{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc"}},
		{Type: model.PartData, Resource: &model.Data{ID: "d1"}},
		{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc"}},
	}
	require.NoError(t, sc.Collect(t.Context(), item, parts))

	result, err := sc.Result()
	require.NoError(t, err)
	summary := result.Data.(*tools.SegCountSummary)
	assert.Equal(t, 0, summary.TotalSourceSegments)
	assert.Equal(t, 0, summary.Documents["doc.html"].BlockCount)
}

func TestSegCountCollectorSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	sc := tools.NewSegCountCollector()

	item := &flow.Item{Input: &model.RawDocument{URI: "doc.html"}}
	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	block.SetAnno(string(model.AnnoSegCount), &tools.SegCountFacet{Source: 1})
	parts := []*model.Part{{Type: model.PartBlock, Resource: block}}
	require.NoError(t, sc.Collect(t.Context(), item, parts))

	result, err := sc.Result()
	require.NoError(t, err)
	summary := result.Data.(*tools.SegCountSummary)
	assert.Equal(t, 0, summary.TotalSourceSegments)
}

// TestStreamingSegCountCollector exercises the streaming path used by the CLI
// (segment-count is wired into CollectorFactories with this collector). It
// must match the buffered collector's totals.
func TestStreamingSegCountCollector(t *testing.T) {
	t.Parallel()
	sc := tools.NewStreamingSegCountCollector()

	item := &flow.Item{
		Input:        &model.RawDocument{URI: "doc1.html"},
		TargetLocale: model.LocaleFrench,
	}
	require.NoError(t, sc.Collect(t.Context(), item, nil))

	block1 := model.NewBlock("tu1", "Hello. World.")
	block1.SetAnno(string(model.AnnoSegCount), &tools.SegCountFacet{Source: 2, Target: 2})
	sc.Observe(&model.Part{Type: model.PartBlock, Resource: block1})

	block2 := model.NewBlock("tu2", "Goodbye")
	block2.SetAnno(string(model.AnnoSegCount), &tools.SegCountFacet{Source: 1})
	sc.Observe(&model.Part{Type: model.PartBlock, Resource: block2})

	// Non-block and non-translatable parts must be ignored.
	sc.Observe(&model.Part{Type: model.PartData, Resource: &model.Data{ID: "d1"}})
	nt := model.NewBlock("tu3", "skip")
	nt.Translatable = false
	nt.SetAnno(string(model.AnnoSegCount), &tools.SegCountFacet{Source: 9})
	sc.Observe(&model.Part{Type: model.PartBlock, Resource: nt})

	result, err := sc.Result()
	require.NoError(t, err)
	assert.Equal(t, "segment-count", result.Name)

	summary := result.Data.(*tools.SegCountSummary)
	assert.Equal(t, 3, summary.TotalSourceSegments)
	assert.Equal(t, 2, summary.TotalTargetSegments)
	assert.Equal(t, 1, summary.DocumentCount)
	doc := summary.Documents["doc1.html"]
	assert.Equal(t, 3, doc.SourceSegments)
	assert.Equal(t, 2, doc.TargetSegments)
	assert.Equal(t, 2, doc.BlockCount)
}

// TestStreamingSegCountCollector_IsStreamingCollector ensures the type wired
// into CollectorFactories satisfies the streaming-collector contract, so the
// CLI taps it inline rather than discarding the segment-count output (the
// #721 "segment-count returns empty" bug).
func TestStreamingSegCountCollector_IsStreamingCollector(t *testing.T) {
	t.Parallel()
	var c flow.Collector = tools.NewStreamingSegCountCollector()
	_, ok := c.(flow.StreamingCollector)
	assert.True(t, ok, "streaming segment-count collector must implement flow.StreamingCollector")
}

// TestSegCountSummary_FormatTable renders the text table and asserts the
// totals appear — proving non-JSON output is non-empty (the visible symptom of
// the original bug was empty output).
func TestSegCountSummary_FormatTable(t *testing.T) {
	t.Parallel()
	summary := &tools.SegCountSummary{
		TotalSourceSegments: 5,
		DocumentCount:       1,
		Documents: map[string]tools.DocumentSegCount{
			"page.html": {URI: "page.html", SourceSegments: 5, BlockCount: 5},
		},
	}
	var buf bytes.Buffer
	summary.FormatTable(&buf)
	out := buf.String()
	require.NotEmpty(t, out)
	assert.Contains(t, out, "page.html")
	assert.Contains(t, out, "SOURCE SEGMENTS")
	assert.Contains(t, out, "5")
	assert.Contains(t, out, "Total (1 files)")
}
