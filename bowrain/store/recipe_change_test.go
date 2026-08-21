package store

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPendingRecipeChanges(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	set := func(path, value string) error {
		raw, err := json.Marshal(value)
		require.NoError(t, err)
		return s.ProposeRecipeChange(ctx, &PendingRecipeChange{
			WorkspaceID: "ws1", ProjectID: "p1", Path: path, Value: raw, CreatedBy: "ada",
		})
	}

	require.NoError(t, set("defaults.coordinates.brand", "acme"))
	require.NoError(t, set("defaults.coordinates.market", "emea"))

	pending, err := s.PendingRecipeChanges(ctx, "p1")
	require.NoError(t, err)
	require.Len(t, pending, 2)
	assert.Equal(t, "defaults.coordinates.brand", pending[0].Path, "oldest first, so a pull applies them as decided")
	assert.JSONEq(t, `"acme"`, string(pending[0].Value))
	assert.Equal(t, RecipeChangePending, pending[0].Status)

	// Another project's tree is not offered this project's decisions.
	other, err := s.PendingRecipeChanges(ctx, "p2")
	require.NoError(t, err)
	assert.Empty(t, other)

	// Re-approving the same axis restates the line rather than queueing a
	// second one: what waits to be applied is a decision, not a history of it.
	require.NoError(t, set("defaults.coordinates.brand", "northwind"))
	pending, err = s.PendingRecipeChanges(ctx, "p1")
	require.NoError(t, err)
	require.Len(t, pending, 2, "the same path must not queue twice")
	byPath := map[string]string{}
	for _, ch := range pending {
		byPath[ch.Path] = string(ch.Value)
	}
	assert.JSONEq(t, `"northwind"`, byPath["defaults.coordinates.brand"], "the last decision is the one waiting")

	// Applying settles it, and a second pull is not offered it again. Address
	// the row by path rather than by index: restating a decision bumps its
	// recency, so position is not stable across an approval.
	brandID := ""
	for _, ch := range pending {
		if ch.Path == "defaults.coordinates.brand" {
			brandID = ch.ID
		}
	}
	require.NotEmpty(t, brandID)
	require.NoError(t, s.MarkRecipeChangeApplied(ctx, brandID))

	rest, err := s.PendingRecipeChanges(ctx, "p1")
	require.NoError(t, err)
	require.Len(t, rest, 1)
	assert.Equal(t, "defaults.coordinates.market", rest[0].Path)

	// Marking an already-applied change is a no-op rather than an error: a
	// retried pull must converge, not fail.
	require.NoError(t, s.MarkRecipeChangeApplied(ctx, brandID))

	// A settled path can be proposed again — a later approval of the same axis
	// is a new decision, not a duplicate of a spent one.
	require.NoError(t, set("defaults.coordinates.brand", "acme"))
	rest, err = s.PendingRecipeChanges(ctx, "p1")
	require.NoError(t, err)
	assert.Len(t, rest, 2)
}
