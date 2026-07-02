package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func scopingBlock(id, text, category string, words int) *model.Block {
	b := model.NewBlock(id, text)
	if category != "" {
		b.Properties[tools.PropScopingCategory] = category
	}
	b.SetAnno(string(model.AnnoWordCount), &tools.WordCountAnnotation{Source: words})
	return b
}

// TestScopingCollector_MergesCategoriesAcrossFiles: the same category seen in
// several files accumulates into one bucket while distinct categories stay
// separate — the cross-file merge the scoping report is built on.
func TestScopingCollector_MergesCategoriesAcrossFiles(t *testing.T) {
	t.Parallel()
	sc := tools.NewScopingCollector()

	files := map[string][]*model.Part{
		"a.json": {
			{Type: model.PartBlock, Resource: scopingBlock("tu1", "one two", tools.ScopingNew, 2)},
			{Type: model.PartBlock, Resource: scopingBlock("tu2", "three", "exact-match", 1)},
		},
		"b.json": {
			{Type: model.PartBlock, Resource: scopingBlock("tu1", "four five six", tools.ScopingNew, 3)},
		},
		"c.json": {
			{Type: model.PartBlock, Resource: scopingBlock("tu1", "seven", "repetition", 1)},
		},
	}
	for uri, parts := range files {
		item := &flow.Item{Input: &model.RawDocument{URI: uri}}
		require.NoError(t, sc.Collect(t.Context(), item, parts))
	}

	result, err := sc.Result()
	require.NoError(t, err)
	summary := result.Data.(*tools.ScopingSummary)

	assert.Equal(t, 3, summary.DocumentCount)
	assert.Equal(t, 4, summary.TotalBlocks)
	assert.Equal(t, 7, summary.TotalWords)

	require.Contains(t, summary.Categories, tools.ScopingNew)
	assert.Equal(t, 2, summary.Categories[tools.ScopingNew].BlockCount, "new merges across a.json and b.json")
	assert.Equal(t, 5, summary.Categories[tools.ScopingNew].WordCount)
	assert.Equal(t, 1, summary.Categories["exact-match"].BlockCount)
	assert.Equal(t, 1, summary.Categories["repetition"].BlockCount)
}

// TestScopingCollector_EmptyStream: a file with no parts still counts as a
// processed document, with no categories fabricated.
func TestScopingCollector_EmptyStream(t *testing.T) {
	t.Parallel()
	sc := tools.NewScopingCollector()

	require.NoError(t, sc.Collect(t.Context(), &flow.Item{Input: &model.RawDocument{URI: "empty.json"}}, nil))

	result, err := sc.Result()
	require.NoError(t, err)
	summary := result.Data.(*tools.ScopingSummary)
	assert.Equal(t, 1, summary.DocumentCount)
	assert.Empty(t, summary.Categories)
	assert.Equal(t, 0, summary.TotalBlocks)
	assert.Equal(t, 0, summary.TotalWords)
}

// TestScopingCollector_ResultSnapshotIsolation: mutating a returned summary's
// category buckets must not corrupt the collector.
func TestScopingCollector_ResultSnapshotIsolation(t *testing.T) {
	t.Parallel()
	sc := tools.NewScopingCollector()

	item := &flow.Item{Input: &model.RawDocument{URI: "doc.json"}}
	parts := []*model.Part{{Type: model.PartBlock, Resource: scopingBlock("tu1", "one two", tools.ScopingNew, 2)}}
	require.NoError(t, sc.Collect(t.Context(), item, parts))

	first, err := sc.Result()
	require.NoError(t, err)
	first.Data.(*tools.ScopingSummary).Categories[tools.ScopingNew].WordCount = 999

	second, err := sc.Result()
	require.NoError(t, err)
	assert.Equal(t, 2, second.Data.(*tools.ScopingSummary).Categories[tools.ScopingNew].WordCount,
		"mutating a returned summary must not affect the collector")
}
