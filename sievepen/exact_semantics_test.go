package sievepen

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The regression class behind these tests: a docs heading "Install" was
// filled from a desktop UI entry ("Install" -> "{=m0} Installer") that
// happened to sort first among two equal-score exact matches. Exact text
// equality must not outrank structural identity, and equal-score exacts
// with differing targets must not be silently picked by storage order.

func newExactTM(t *testing.T) *SQLiteTM {
	t.Helper()
	tm, err := NewSQLiteTM(filepath.Join(t.TempDir(), "tm.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = tm.Close() })
	return tm
}

func textEntry(id, src, tgt string) TMEntry {
	return TMEntry{
		ID: id,
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: src}}},
			"nb": {{Text: &model.TextRun{Text: tgt}}},
		},
	}
}

// TestExactMatch_StructuralMismatchIsNearExact: a plain-text query must
// score 1.0 only against an entry with the same inline-code structure; an
// entry whose source carried codes is capped at ScoreNearExact.
func TestExactMatch_StructuralMismatchIsNearExact(t *testing.T) {
	ctx := context.Background()
	tm := newExactTM(t)

	// Code-structured source: **Install** (paired markup codes). Paired
	// codes flatten to their inner content, so the PLAIN key collides with
	// the bare text "Install" while the STRUCTURAL key differs.
	iconEntry := TMEntry{
		ID: "e-icon",
		Variants: map[model.LocaleID][]model.Run{
			"en": {
				{PcOpen: &model.PcOpenRun{ID: "m0", Data: "**"}},
				{Text: &model.TextRun{Text: "Install"}},
				{PcClose: &model.PcCloseRun{ID: "m0", Data: "**"}},
			},
			"nb": {
				{PcOpen: &model.PcOpenRun{ID: "m0", Data: "**"}},
				{Text: &model.TextRun{Text: "Installer"}},
				{PcClose: &model.PcCloseRun{ID: "m0", Data: "**"}},
			},
		},
	}
	require.NoError(t, tm.Add(ctx, iconEntry))

	matches, err := tm.LookupText(ctx, "Install", "en", "nb", LookupOptions{})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, ScoreNearExact, matches[0].Score,
		"plain-text equality across differing code structure is a near match, not 1.0")

	// With the structurally identical entry present, it wins at full score.
	require.NoError(t, tm.Add(ctx, textEntry("e-plain", "Install", "Installering")))
	matches, err = tm.LookupText(ctx, "Install", "en", "nb", LookupOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.Equal(t, 1.0, matches[0].Score)
	assert.Equal(t, "Installering", matches[0].Entry.VariantText("nb"))
	assert.False(t, matches[0].Ambiguous)
}

// TestExactMatch_AmbiguityDemotesAll: two structurally identical exacts
// with DIFFERENT targets both demote to ScoreNearExact and flag Ambiguous;
// identical targets keep 1.0.
func TestExactMatch_AmbiguityDemotesAll(t *testing.T) {
	ctx := context.Background()
	tm := newExactTM(t)
	require.NoError(t, tm.Add(ctx, textEntry("e-a", "Install", "Installering")))
	require.NoError(t, tm.Add(ctx, textEntry("e-b", "Install", "Installer")))

	matches, err := tm.LookupText(ctx, "Install", "en", "nb", LookupOptions{MaxResults: 5})
	require.NoError(t, err)
	require.Len(t, matches, 2)
	for _, m := range matches {
		assert.Equal(t, ScoreNearExact, m.Score)
		assert.True(t, m.Ambiguous, "differing targets at full score must be flagged")
	}

	// An exact-only policy (MinScore 1.0) gets nothing rather than a coin flip.
	strict, err := tm.LookupText(ctx, "Install", "en", "nb", LookupOptions{MinScore: 1.0})
	require.NoError(t, err)
	assert.Empty(t, strict)

	// Identical targets are not ambiguous.
	tm2 := newExactTM(t)
	require.NoError(t, tm2.Add(ctx, textEntry("e-a", "Save", "Lagre")))
	require.NoError(t, tm2.Add(ctx, textEntry("e-b", "Save", "Lagre")))
	matches, err = tm2.LookupText(ctx, "Save", "en", "nb", LookupOptions{MaxResults: 5})
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	assert.Equal(t, 1.0, matches[0].Score)
	assert.False(t, matches[0].Ambiguous)
}

// TestExactMatch_DeterministicOrder: equal candidates order by entry ID,
// not storage order.
func TestExactMatch_DeterministicOrder(t *testing.T) {
	ctx := context.Background()
	tm := newExactTM(t)
	// Insert in reverse-ID order.
	require.NoError(t, tm.Add(ctx, textEntry("e-z", "Run", "Kjøring")))
	require.NoError(t, tm.Add(ctx, textEntry("e-a", "Run", "Kjøring")))

	matches, err := tm.LookupText(ctx, "Run", "en", "nb", LookupOptions{MaxResults: 5})
	require.NoError(t, err)
	require.Len(t, matches, 2)
	assert.Equal(t, "e-a", matches[0].Entry.ID)
	assert.Equal(t, "e-z", matches[1].Entry.ID)
}

// TestExactMatch_InMemoryParity: the in-memory backend applies the same
// structural cap and ambiguity rule.
func TestExactMatch_InMemoryParity(t *testing.T) {
	ctx := context.Background()
	tm := NewInMemoryTM()
	require.NoError(t, tm.Add(ctx, textEntry("e-a", "Install", "Installering")))
	require.NoError(t, tm.Add(ctx, textEntry("e-b", "Install", "Installer")))

	matches, err := tm.LookupText(ctx, "Install", "en", "nb", LookupOptions{MaxResults: 5})
	require.NoError(t, err)
	require.Len(t, matches, 2)
	for _, m := range matches {
		assert.Equal(t, ScoreNearExact, m.Score)
		assert.True(t, m.Ambiguous)
	}
}
