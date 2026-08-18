package memory_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// answerAt writes one approved answer for "Recycle" at a point.
func answerAt(t *testing.T, tm *memory.SQLiteStore, id, target, point string) {
	t.Helper()
	require.NoError(t, tm.Add(context.Background(), memory.Entry{
		ID:          id,
		HintSrcLang: "en",
		Point:       point,
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Recycle"}}},
			"nb": {{Text: &model.TextRun{Text: target}}},
		},
	}))
}

func recycleQuery() *model.Block {
	b := &model.Block{ID: "q", Translatable: true}
	b.SetSourceRuns([]model.Run{{Text: &model.TextRun{Text: "Recycle"}}})
	return b
}

// TestSQLiteStore_LookupResolvesToTheNearestApproval: two reviewed answers for
// one source cannot both be THE translation, and the corpus used to say so by
// demoting both — leaving a full-score fill policy with nothing and a reader
// with no way to choose. Where the caller can say where it is asking from, the
// approval nearest that point is the one that governs there.
func TestSQLiteStore_LookupResolvesToTheNearestApproval(t *testing.T) {
	tm, err := memory.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	cli := memory.NewPoint("neokapi", "cli", "neokapi-cli")
	engine := memory.NewPoint("neokapi", "engine", "neokapi-engine")
	answerAt(t, tm, "a", "Gjenbruk", cli)
	answerAt(t, tm, "b", "Bruk om igjen", engine)

	ctx := context.Background()
	for _, tc := range []struct{ name, at, want string }{
		{"asking from the CLI", cli, "Gjenbruk"},
		{"asking from the engine", engine, "Bruk om igjen"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matches, err := tm.Lookup(ctx, recycleQuery(), "en", "nb",
				memory.LookupOptions{MinScore: 1.0, MaxResults: 10, Point: tc.at})
			require.NoError(t, err)
			require.Len(t, matches, 1, "one answer governs here")
			assert.False(t, matches[0].Ambiguous)
			assert.Equal(t, tc.want, matches[0].Entry.VariantText("nb"))
		})
	}

	t.Run("the stored point survives the round trip", func(t *testing.T) {
		entries, err := tm.Entries(ctx)
		require.NoError(t, err)
		byID := map[string]string{}
		for _, e := range entries {
			byID[e.ID] = e.Point
		}
		assert.Equal(t, map[string]string{"a": cli, "b": engine}, byID)
	})

	t.Run("a caller with no point in hand is told it is ambiguous", func(t *testing.T) {
		matches, err := tm.Lookup(ctx, recycleQuery(), "en", "nb",
			memory.LookupOptions{MinScore: 0.7, MaxResults: 10})
		require.NoError(t, err)
		require.Len(t, matches, 2)
		for _, m := range matches {
			assert.True(t, m.Ambiguous)
			assert.InEpsilon(t, memory.ScoreNearExact, m.Score, 0.0001)
		}
	})
}

// TestSQLiteStore_EqualDistanceFallsToTheAnswersOwnText pins the fallback. Two
// approvals the ladder cannot separate must not be settled by anything that
// moves when the rest of the corpus does — that is the defect the ladder was
// added to fix — so the answers decide it between themselves.
func TestSQLiteStore_EqualDistanceFallsToTheAnswersOwnText(t *testing.T) {
	tm, err := memory.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	// Two collections on one channel: equally far from a third on the same one.
	answerAt(t, tm, "a", "Gjenbruk", memory.NewPoint("neokapi", "cli", "neokapi-cli"))
	answerAt(t, tm, "b", "Bruk om igjen", memory.NewPoint("neokapi", "cli", "neokapi-help"))

	at := memory.NewPoint("neokapi", "cli", "neokapi-chrome")
	matches, err := tm.Lookup(context.Background(), recycleQuery(), "en", "nb",
		memory.LookupOptions{MinScore: 1.0, MaxResults: 10, Point: at})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "Bruk om igjen", matches[0].Entry.VariantText("nb"),
		"the smaller text, and nothing about how often the corpus repeats it")
}
