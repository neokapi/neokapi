package store

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSourceProposalStore(t *testing.T) *SourceProposalStore {
	t.Helper()
	db := pgtest.NewTestDB(t)
	_, err := NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	return NewSourceProposalStore(db.DB)
}

func TestSourceProposalStore_CRUD(t *testing.T) {
	store := newTestSourceProposalStore(t)
	ctx := t.Context()

	p := &ProposedSourceChange{
		WorkspaceID:    "ws-1",
		ProjectID:      "proj-1",
		ItemName:       "ui.json",
		BlockID:        "b1",
		OriginalSource: "Colour picker",
		ProposedSource: "Color picker",
		Rationale:      "US spelling",
		FoundInLocale:  "fr",
		FinderUser:     "reviewer-1",
	}
	require.NoError(t, store.Create(ctx, p))
	assert.NotEmpty(t, p.ID)
	assert.Equal(t, SourceProposalOpen, p.Status)
	assert.Equal(t, SourceProposalTextFix, p.Kind, "kind defaults to text-fix")
	assert.Equal(t, "main", p.Stream, "stream defaults to main")

	got, err := store.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "Colour picker", got.OriginalSource)
	assert.Equal(t, "Color picker", got.ProposedSource)
	assert.Equal(t, "fr", got.FoundInLocale)
	assert.Nil(t, got.DecidedAt)

	// ListOpen returns the open proposal.
	open, err := store.ListOpen(ctx, "proj-1")
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, p.ID, open[0].ID)

	// A proposal for a different project is not listed.
	other := &ProposedSourceChange{WorkspaceID: "ws-1", ProjectID: "proj-2", BlockID: "b9", ProposedSource: "x"}
	require.NoError(t, store.Create(ctx, other))
	open, err = store.ListOpen(ctx, "proj-1")
	require.NoError(t, err)
	assert.Len(t, open, 1, "ListOpen is scoped to the project")

	// Resolve moves it to approved and stamps the decider.
	require.NoError(t, store.Resolve(ctx, p.ID, SourceProposalApproved, "owner-1", "looks right"))
	got, err = store.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, SourceProposalApproved, got.Status)
	assert.Equal(t, "owner-1", got.DecidedBy)
	assert.Equal(t, "looks right", got.DecisionReason)
	require.NotNil(t, got.DecidedAt)

	// It is no longer open.
	open, err = store.ListOpen(ctx, "proj-1")
	require.NoError(t, err)
	assert.Empty(t, open)

	// Resolving an already-resolved proposal is a no-op (stays approved).
	require.NoError(t, store.Resolve(ctx, p.ID, SourceProposalRejected, "owner-2", ""))
	got, err = store.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, SourceProposalApproved, got.Status, "resolve only acts on an open proposal")
}
