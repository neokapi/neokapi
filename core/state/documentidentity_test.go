package state_test

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newIdentityStore(t *testing.T) *state.WorkStore {
	t.Helper()
	dir := t.TempDir()
	w, err := state.OpenWork(t.Context(), filepath.Join(dir, "work.db"), filepath.Join(dir, "committed.json"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })
	return w
}

func decisionAt(scope, unit string) state.UnitState {
	return state.UnitState{
		Scope:    scope,
		Unit:     unit,
		Variant:  model.Variant("nb"),
		Status:   "approved",
		Decision: state.Decision{ReviewState: "approved", By: "reviewer"},
	}
}

func read(t *testing.T, w *state.WorkStore, scope, unit string) (state.UnitState, bool) {
	t.Helper()
	return w.Get(t.Context(), state.Key{Scope: scope, Unit: unit, Variant: model.Variant("nb")})
}

// TestAdoptDocumentsCarriesDecisionsThroughARename is the defect this exists
// for. A decision is filed against the document it was made in; while that
// document's identity WAS its path, renaming the file detached every approval
// inside it — silently, because nothing failed and the loop simply re-approved
// from scratch.
func TestAdoptDocumentsCarriesDecisionsThroughARename(t *testing.T) {
	w := newIdentityStore(t)
	const unit = "greeting"

	// A decision recorded before this document had an identity: filed under its
	// path, which is what the project did and what an existing store holds.
	require.NoError(t, w.Put(t.Context(), decisionAt("docs/intro.md", unit)))

	// The first read gives it one, and the decision moves onto it.
	keys, err := w.AdoptDocuments(t.Context(), []reconcile.DocUnit{
		{Path: "docs/intro.md", Content: []string{"h1", "h2"}},
	})
	require.NoError(t, err)
	key := keys["docs/intro.md"]
	require.NotEmpty(t, key)
	require.NotEqual(t, "docs/intro.md", key, "the identity is not the address")

	got, found := read(t, w, key, unit)
	require.True(t, found, "the decision followed its document onto the document's key")
	assert.Equal(t, "approved", got.Decision.ReviewState)
	_, stillAtPath := read(t, w, "docs/intro.md", unit)
	assert.False(t, stillAtPath, "and is not left behind at the address as a second copy")

	// The file moves, its contents mostly survive. The key does not change, so
	// the decision does not have to.
	keys, err = w.AdoptDocuments(t.Context(), []reconcile.DocUnit{
		{Path: "guides/intro.md", Content: []string{"h1", "h2"}},
	})
	require.NoError(t, err)
	assert.Equal(t, key, keys["guides/intro.md"], "a renamed document keeps its identity")

	got, found = read(t, w, key, unit)
	require.True(t, found, "so the approval inside it survives the rename")
	assert.Equal(t, "reviewer", got.Decision.By)
}

// TestAdoptDocumentsMintsPerDocument: two files are two documents, and neither
// takes the other's decisions.
func TestAdoptDocumentsMintsPerDocument(t *testing.T) {
	w := newIdentityStore(t)

	keys, err := w.AdoptDocuments(t.Context(), []reconcile.DocUnit{
		{Path: "docs/a.md", Content: []string{"alpha"}},
		{Path: "docs/b.md", Content: []string{"bravo"}},
	})
	require.NoError(t, err)
	require.Len(t, keys, 2)
	assert.NotEqual(t, keys["docs/a.md"], keys["docs/b.md"])

	require.NoError(t, w.Put(t.Context(), decisionAt(keys["docs/a.md"], "p1")))
	_, found := read(t, w, keys["docs/b.md"], "p1")
	assert.False(t, found, "one document's decision is not the other's")
}

// TestAdoptDocumentsKeepsTheDecisionMadeAgainstTheIdentity: re-keying is a
// migration of an address into an identity, and must never overwrite a decision
// that was recorded against the identity itself.
func TestAdoptDocumentsKeepsTheDecisionMadeAgainstTheIdentity(t *testing.T) {
	w := newIdentityStore(t)
	const unit = "greeting"

	keys, err := w.AdoptDocuments(t.Context(), []reconcile.DocUnit{
		{Path: "docs/intro.md", Content: []string{"h1"}},
	})
	require.NoError(t, err)
	key := keys["docs/intro.md"]

	// One decision under the identity, a stale one still under the address.
	byIdentity := decisionAt(key, unit)
	byIdentity.Decision.Note = "the current one"
	require.NoError(t, w.Put(t.Context(), byIdentity))
	byAddress := decisionAt("docs/intro.md", unit)
	byAddress.Decision.Note = "the superseded one"
	require.NoError(t, w.Put(t.Context(), byAddress))

	_, err = w.AdoptDocuments(t.Context(), []reconcile.DocUnit{
		{Path: "docs/intro.md", Content: []string{"h1"}},
	})
	require.NoError(t, err)

	got, found := read(t, w, key, unit)
	require.True(t, found)
	assert.Equal(t, "the current one", got.Decision.Note)
	_, leftover := read(t, w, "docs/intro.md", unit)
	assert.False(t, leftover, "and the address holds no second answer for the same unit")
}
