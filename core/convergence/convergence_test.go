package convergence_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

func TestBlockKey_NameThenID(t *testing.T) {
	assert.Equal(t, "greeting", convergence.BlockKey(&model.Block{Name: "greeting", ID: "x"}))
	assert.Equal(t, "x", convergence.BlockKey(&model.Block{ID: "x"}), "falls back to ID when unnamed")
}

func TestTargetState_PresenceBaselineAndCommitted(t *testing.T) {
	b := model.NewBlock("a", "Apple")
	b.Translatable = true
	assert.Empty(t, convergence.TargetState(b, "nb"), "no target → below every rung")
	b.SetTargetText("nb", "Eple")
	assert.Equal(t, string(model.TargetStatusTranslated), convergence.TargetState(b, "nb"),
		"a present target counts as translated (presence baseline)")
	b.StampTargetProvenance("nb", model.TargetStatusReviewed, model.Origin{})
	assert.Equal(t, string(model.TargetStatusReviewed), convergence.TargetState(b, "nb"),
		"a committed status is authoritative")
}

func TestSourceState_PresenceBaselineAndCommitted(t *testing.T) {
	b := model.NewBlock("a", "Apple")
	assert.Equal(t, string(model.SourceStatusAuthored), convergence.SourceState(b))
	b.SourceStatus = model.SourceStatusApproved
	assert.Equal(t, string(model.SourceStatusApproved), convergence.SourceState(b))
	assert.Empty(t, convergence.SourceState(model.NewBlock("e", "  ")), "empty source is below every rung")
}

func TestPreview_TrimsAndCollapses(t *testing.T) {
	assert.Equal(t, "a b c", convergence.Preview("  a   b\nc  "))
	long := convergence.Preview(string(make([]byte, 100)))
	assert.LessOrEqual(t, len([]rune(long)), 72)
}

func TestSummarizeReviewLanguages_CountsPerLanguageSourceFirst(t *testing.T) {
	cases := []struct {
		name  string
		items []convergence.ReviewQueueItem
		want  []convergence.ReviewLanguage
	}{
		{name: "an empty queue summarises nothing"},
		{
			name: "the source language leads, then the targets in tag order",
			items: []convergence.ReviewQueueItem{
				{Language: "nb"}, {Language: "fr"}, {Language: "nb"},
				{Language: "en", IsSource: true}, {Language: "en", IsSource: true},
			},
			want: []convergence.ReviewLanguage{
				{Language: "en", Pending: 2, Source: true},
				{Language: "fr", Pending: 1},
				{Language: "nb", Pending: 2},
			},
		},
		{
			name:  "an item written before Language falls back to its locale",
			items: []convergence.ReviewQueueItem{{Locale: "de"}},
			want:  []convergence.ReviewLanguage{{Language: "de", Pending: 1}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, convergence.SummarizeReviewLanguages(tc.items))
		})
	}
}

func TestEnsureSourceLanguage(t *testing.T) {
	cases := []struct {
		name       string
		langs      []convergence.ReviewLanguage
		sourceLang string
		want       []convergence.ReviewLanguage
	}{
		{
			name:       "an empty tag adds nothing",
			langs:      []convergence.ReviewLanguage{{Language: "nb", Pending: 2}},
			sourceLang: "",
			want:       []convergence.ReviewLanguage{{Language: "nb", Pending: 2}},
		},
		{
			name:       "an absent source is added at zero and leads",
			langs:      []convergence.ReviewLanguage{{Language: "nb", Pending: 2}, {Language: "fr", Pending: 1}},
			sourceLang: "en",
			want: []convergence.ReviewLanguage{
				{Language: "en", Pending: 0, Source: true},
				{Language: "fr", Pending: 1},
				{Language: "nb", Pending: 2},
			},
		},
		{
			name:       "a present source keeps its count and gains the marker",
			langs:      []convergence.ReviewLanguage{{Language: "en", Pending: 3}, {Language: "nb", Pending: 2}},
			sourceLang: "en",
			want: []convergence.ReviewLanguage{
				{Language: "en", Pending: 3, Source: true},
				{Language: "nb", Pending: 2},
			},
		},
		{
			name:       "an empty summary yields just the source",
			langs:      nil,
			sourceLang: "en",
			want:       []convergence.ReviewLanguage{{Language: "en", Pending: 0, Source: true}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, convergence.EnsureSourceLanguage(tc.langs, tc.sourceLang))
		})
	}
}

func TestSourceLanguageOf(t *testing.T) {
	got, ok := convergence.SourceLanguageOf([]convergence.ReviewLanguage{
		{Language: "nb", Pending: 2},
		{Language: "en", Pending: 0, Source: true},
	})
	assert.True(t, ok)
	assert.Equal(t, "en", got)

	_, ok = convergence.SourceLanguageOf([]convergence.ReviewLanguage{{Language: "nb", Pending: 2}})
	assert.False(t, ok, "a queue with no source marker names no source")
}

func TestSortReviewQueue_SourceFirstThenLanguageFileKey(t *testing.T) {
	items := []convergence.ReviewQueueItem{
		{Language: "nb", File: "b.json", Key: "k"},
		{Language: "nb", File: "a.json", Key: "z"},
		{Language: "en", IsSource: true, File: "src.json", Key: "b"},
		{Language: "fr", File: "a.json", Key: "a"},
		{Language: "en", IsSource: true, File: "src.json", Key: "a"},
	}
	convergence.SortReviewQueue(items)
	got := make([][2]string, 0, len(items))
	for _, it := range items {
		got = append(got, [2]string{it.LanguageTag(), it.Key})
	}
	assert.Equal(t, [][2]string{
		{"en", "a"}, {"en", "b"}, {"fr", "a"}, {"nb", "z"}, {"nb", "k"},
	}, got)
}
