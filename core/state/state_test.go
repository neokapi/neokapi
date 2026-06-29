package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func approved(unit, locale, targetHash string) state.UnitState {
	return state.UnitState{
		Unit:       unit,
		Variant:    model.Variant(model.LocaleID(locale)),
		Status:     model.TargetStatusReviewed,
		TargetHash: targetHash,
		Decision:   state.Decision{ReviewState: "approved", By: "alice", At: "2026-06-29T00:00:00Z"},
		Updated:    "2026-06-29T00:00:00Z",
	}
}

// TestFileStore_RoundTripAndAuthoritative verifies the committed file is the
// source of truth: a recorded approval persists across Open/Save, and a derived
// reader rebuilds the identical state from the committed file alone.
func TestFileStore_RoundTripAndAuthoritative(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	s, err := state.Open(path)
	require.NoError(t, err)
	require.Empty(t, s.All(), "a project with no recorded state opens empty")

	s.Put(approved("h1", "fr-FR", "sha256:aaa"))
	assert.True(t, s.Pending(), "an un-exported decision is pending")
	require.NoError(t, s.Export())
	assert.False(t, s.Pending(), "export clears pending")
	require.FileExists(t, path, "Save writes the committed source-of-truth file")

	// Reopen: the decision survives entirely from the committed file.
	s2, err := state.Open(path)
	require.NoError(t, err)
	got, ok := s2.Get(state.Key{Unit: "h1", Variant: model.Variant("fr-FR")})
	require.True(t, ok)
	assert.Equal(t, model.TargetStatusReviewed, got.Status)
	assert.Equal(t, "approved", got.Decision.ReviewState)
	assert.Equal(t, "alice", got.Decision.By)
}

// TestUnitState_StaleOnTranslationChange verifies the targetHash link: an
// approval no longer applies once the translation it blessed changes.
func TestUnitState_StaleOnTranslationChange(t *testing.T) {
	u := approved("h1", "fr-FR", "sha256:aaa")
	assert.True(t, u.Reviewed("sha256:aaa"), "reviewed for the translation it blessed")
	assert.False(t, u.Reviewed("sha256:bbb"), "a changed translation invalidates the approval")
	assert.True(t, u.Stale("sha256:bbb"))
	assert.False(t, u.Stale("sha256:aaa"))
}

// TestFileStore_DeterministicSerialization verifies the committed file is stable
// (sorted) regardless of insertion order, so it diffs cleanly under git.
func TestFileStore_DeterministicSerialization(t *testing.T) {
	write := func(order []string) []byte {
		path := filepath.Join(t.TempDir(), "state.json")
		s, err := state.Open(path)
		require.NoError(t, err)
		for _, u := range order {
			s.Put(approved(u, "fr-FR", "sha256:"+u))
		}
		require.NoError(t, s.Export())
		b, err := os.ReadFile(path)
		require.NoError(t, err)
		return b
	}
	assert.Equal(t, string(write([]string{"a", "b", "c"})), string(write([]string{"c", "a", "b"})),
		"serialization is insertion-order-independent (deterministic, diff-friendly)")
}
