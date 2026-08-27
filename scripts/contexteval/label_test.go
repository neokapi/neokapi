package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// What makes a kappa mean anything is this loop, not the arithmetic after it.
// A labeller shown the judge's verdict agrees with it; one shown the variant
// scores the variant. Both failures produce a healthy-looking number, which is
// why they are asserted rather than trusted to the reviewer's eye.

func writeCandidates(t *testing.T, dir string, items ...outputItem) string {
	t.Helper()
	body, err := json.Marshal(outputLog{Items: items})
	require.NoError(t, err)
	p := filepath.Join(dir, "candidates.json")
	require.NoError(t, os.WriteFile(p, body, 0o644))
	return p
}

// TestNothingIdentifyingReachesTheLabeller.
//
// The candidate carries the model and the variant so the loop can drop them,
// not so it can show them. If either reaches the screen the labeller is
// scoring provenance, and the agreement measured is with a hint.
func TestNothingIdentifyingReachesTheLabeller(t *testing.T) {
	dir := t.TempDir()
	path := writeCandidates(t, dir, outputItem{
		Provider: "claude-code", Model: "sonnet", Target: "de", Variant: "steered",
		Fixture: "f1", Source: "Sign in.", Translation: "Melden Sie sich an.",
	})

	cands, err := loadCandidates(path)
	require.NoError(t, err)
	require.Len(t, cands, 1)

	// The fields a labeller sees, gathered the way askOne prints them.
	shown := cands[0].target + cands[0].source + cands[0].translation
	for _, secret := range []string{"sonnet", "steered", "claude-code", "f1"} {
		assert.NotContains(t, shown, secret,
			"%q is visible to the labeller, which makes this a measurement of agreement with a hint", secret)
	}
	assert.Equal(t, "sonnet", cands[0].hiddenModel, "kept for provenance, not for display")
	assert.Equal(t, "steered", cands[0].hiddenVariant)
}

// TestSameLanguageItemsAreNotOffered: sweeps do not judge same-language pairs,
// so agreement measured on them would validate the judge on a distribution it
// never scores.
func TestSameLanguageItemsAreNotOffered(t *testing.T) {
	dir := t.TempDir()
	path := writeCandidates(t, dir,
		outputItem{Target: "de", Source: "Sign in.", Translation: "Melden Sie sich an."},
		outputItem{Target: "en", Source: "Sign in.", Translation: "Sign in."},
		outputItem{Target: "en-GB", Source: "Sign in.", Translation: "Sign in."},
	)

	cands, err := loadCandidates(path)
	require.NoError(t, err)
	for _, c := range cands {
		assert.True(t, judgeableTarget(c.target), "%s is not judged in sweeps", c.target)
	}
}

// TestEmptyAndDuplicateItemsAreDropped: an empty translation cannot be scored,
// and the same text twice is one item, because the labeller is scoring text
// rather than provenance.
func TestEmptyAndDuplicateItemsAreDropped(t *testing.T) {
	dir := t.TempDir()
	path := writeCandidates(t, dir,
		outputItem{Model: "a", Target: "de", Source: "Sign in.", Translation: "Melden Sie sich an."},
		outputItem{Model: "b", Target: "de", Source: "Sign in.", Translation: "Melden Sie sich an."},
		outputItem{Model: "c", Target: "de", Source: "Nothing.", Translation: "   "},
	)

	cands, err := loadCandidates(path)
	require.NoError(t, err)
	assert.Len(t, cands, 1, "identical text from two models is one item; an empty translation is none")
}

// TestOrderIsShuffledButStable.
//
// Fatigue rises through a session. If the order correlates with model or
// variant then so does the tiredness, and the difference measured is partly
// the labeller's attention. Deterministic so a resumed session continues in
// the same sequence rather than reshuffling.
func TestOrderIsShuffledButStable(t *testing.T) {
	build := func() []candidate {
		var out []candidate
		for _, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
			out = append(out, candidate{id: id, target: "de"})
		}
		return out
	}

	first, second := build(), build()
	shuffleDeterministic(first, "seed-1")
	shuffleDeterministic(second, "seed-1")
	assert.Equal(t, ids(first), ids(second), "the same seed must give the same order, or a resume reshuffles")

	other := build()
	shuffleDeterministic(other, "seed-2")
	assert.NotEqual(t, ids(first), ids(other), "a different rubric should not reuse the old order")

	original := build()
	assert.NotEqual(t, ids(original), ids(first), "input order must not survive, or the shuffle is decorative")
}

func ids(cs []candidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.id
	}
	return out
}

// TestARelabelledRubricIsRefused: rewording a criterion changes the question,
// so the old answers are answers to a different one. Silently mixing them
// would produce a kappa over two rubrics at once.
func TestARelabelledRubricIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := writeCandidates(t, dir,
		outputItem{Target: "de", Source: "Sign in.", Translation: "Melden Sie sich an."})

	sessionPath := filepath.Join(dir, "labels.json")
	stale := labelSession{Rubric: "deadbeef0000", Items: []labelItem{{ID: "x"}}}
	body, err := json.Marshal(stale)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(sessionPath, body, 0o644))

	err = runLabelling(path, sessionPath, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rubric")
	assert.Contains(t, strings.ToLower(err.Error()), "different one")
}

// TestTheItemFloorIsNotThirty: kappa over thirty items carries an interval wide
// enough to span "substantial" and "poor", so quoting it without one is the
// thing this whole exercise exists to avoid.
func TestTheItemFloorIsNotThirty(t *testing.T) {
	assert.GreaterOrEqual(t, MinJudgeItems, 100,
		"the reliability literature puts the useful floor near 100")
	assert.Greater(t, TargetJudgeItems, MinJudgeItems,
		"a session should aim past the floor, not at it")
}
