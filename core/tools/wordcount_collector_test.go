package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func wcBlock(id, text string, source int, targets map[model.LocaleID]int) *model.Block {
	b := model.NewBlock(id, text)
	b.SetAnno(string(model.AnnoWordCount), &tools.WordCountAnnotation{Source: source, Targets: targets})
	return b
}

// TestWordCountCollector_ArchiveEntryBucketing: blocks read from inside an
// archive carry PropContainerEntry and are bucketed per `<archive>!<entry>`;
// the synthetic base row is dropped when the archive produced entry rows.
func TestWordCountCollector_ArchiveEntryBucketing(t *testing.T) {
	t.Parallel()
	wc := tools.NewWordCountCollector()

	item := &flow.Item{Input: &model.RawDocument{URI: "bundle.zip"}}

	b1 := wcBlock("tu1", "Hello world", 2, nil)
	b1.Properties[model.PropContainerEntry] = "a/en.json"
	b2 := wcBlock("tu2", "One two three", 3, nil)
	b2.Properties[model.PropContainerEntry] = "a/en.json"
	b3 := wcBlock("tu3", "Four five", 2, nil)
	b3.Properties[model.PropContainerEntry] = "b/en.json"

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: b1},
		{Type: model.PartBlock, Resource: b2},
		{Type: model.PartBlock, Resource: b3},
	}
	require.NoError(t, wc.Collect(t.Context(), item, parts))

	result, err := wc.Result()
	require.NoError(t, err)
	summary := result.Data.(*tools.WordCountSummary)

	assert.Equal(t, 7, summary.TotalSourceWords)
	assert.Equal(t, 2, summary.DocumentCount, "one row per archive entry, no synthetic base row")
	require.Contains(t, summary.Documents, "bundle.zip!a/en.json")
	require.Contains(t, summary.Documents, "bundle.zip!b/en.json")
	assert.NotContains(t, summary.Documents, "bundle.zip",
		"empty base row is dropped when the archive produced entry rows")

	entryA := summary.Documents["bundle.zip!a/en.json"]
	assert.Equal(t, 5, entryA.SourceWords)
	assert.Equal(t, 2, entryA.BlockCount)
	entryB := summary.Documents["bundle.zip!b/en.json"]
	assert.Equal(t, 2, entryB.SourceWords)
	assert.Equal(t, 1, entryB.BlockCount)
}

// TestWordCountCollector_ArchiveMixedWithBaseBlocks: blocks without a
// container entry stay on the base row alongside the entry rows.
func TestWordCountCollector_ArchiveMixedWithBaseBlocks(t *testing.T) {
	t.Parallel()
	wc := tools.NewWordCountCollector()

	item := &flow.Item{Input: &model.RawDocument{URI: "bundle.zip"}}
	inEntry := wcBlock("tu1", "Hello world", 2, nil)
	inEntry.Properties[model.PropContainerEntry] = "en.json"
	atBase := wcBlock("tu2", "Plain top-level text", 3, nil)

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: inEntry},
		{Type: model.PartBlock, Resource: atBase},
	}
	require.NoError(t, wc.Collect(t.Context(), item, parts))

	result, err := wc.Result()
	require.NoError(t, err)
	summary := result.Data.(*tools.WordCountSummary)

	assert.Equal(t, 2, summary.DocumentCount)
	assert.Equal(t, 2, summary.Documents["bundle.zip!en.json"].SourceWords)
	assert.Equal(t, 3, summary.Documents["bundle.zip"].SourceWords,
		"a base-level block keeps the base row alive")
	assert.Equal(t, 5, summary.TotalSourceWords)
}

// TestWordCountCollector_EmptyStream: a file that produced no parts at all
// still yields its zero row so the report lists every input.
func TestWordCountCollector_EmptyStream(t *testing.T) {
	t.Parallel()
	wc := tools.NewWordCountCollector()

	require.NoError(t, wc.Collect(t.Context(), &flow.Item{Input: &model.RawDocument{URI: "empty.json"}}, nil))

	result, err := wc.Result()
	require.NoError(t, err)
	summary := result.Data.(*tools.WordCountSummary)
	assert.Equal(t, 1, summary.DocumentCount)
	doc, ok := summary.Documents["empty.json"]
	require.True(t, ok, "an empty file still gets its row")
	assert.Equal(t, 0, doc.SourceWords)
	assert.Equal(t, 0, doc.BlockCount)
	assert.Equal(t, 0, summary.TotalSourceWords)
}

// TestWordCountCollector_ResultSnapshotIsolation: mutating a returned summary
// must not corrupt the collector's internal accumulators (Result can be
// called repeatedly, e.g. for intermediate progress).
func TestWordCountCollector_ResultSnapshotIsolation(t *testing.T) {
	t.Parallel()
	wc := tools.NewWordCountCollector()

	item := &flow.Item{Input: &model.RawDocument{URI: "doc.json"}}
	parts := []*model.Part{{Type: model.PartBlock, Resource: wcBlock("tu1", "Hello world", 2, map[model.LocaleID]int{model.LocaleFrench: 3})}}
	require.NoError(t, wc.Collect(t.Context(), item, parts))

	first, err := wc.Result()
	require.NoError(t, err)
	firstSummary := first.Data.(*tools.WordCountSummary)
	firstSummary.TotalTargetWords[model.LocaleFrench] = 999
	doc := firstSummary.Documents["doc.json"]
	doc.TargetWords[model.LocaleFrench] = 999

	second, err := wc.Result()
	require.NoError(t, err)
	secondSummary := second.Data.(*tools.WordCountSummary)
	assert.Equal(t, 3, secondSummary.TotalTargetWords[model.LocaleFrench],
		"mutating a returned summary must not affect the collector")
	assert.Equal(t, 3, secondSummary.Documents["doc.json"].TargetWords[model.LocaleFrench])
}
