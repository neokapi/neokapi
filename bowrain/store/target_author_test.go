package store

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLastTargetAuthors covers what separation of duties at approval asks the
// store: who wrote this translation, per locale. Three things decide the answer,
// and each has been wrong in a way that would silently disable the four-eyes
// check or refuse an innocent approval.
func TestLastTargetAuthors(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	require.NoError(t, s.StoreItem(ctx, p.ID, "main", &platstore.Item{
		Name: "greetings.txt", Format: "txt", ItemType: "file",
	}))

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "greetings.txt", []*model.Block{b}))
	stored, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: "main"})
	require.NoError(t, err)
	require.Len(t, stored, 1)
	bid := stored[0].Block.ID
	unit := stored[0].SourceID

	write := func(actor, locale, text string) {
		sb, err := s.GetBlock(ctx, p.ID, "main", bid)
		require.NoError(t, err)
		sb.Block.SetTargetText(model.LocaleID(locale), text)
		wctx := WithChangeContext(ctx, ChangeContext{Actor: actor})
		require.NoError(t, s.StoreBlocks(wctx, p.ID, "main", []*model.Block{sb.Block}))
	}

	t.Run("machine-authored targets report nobody", func(t *testing.T) {
		write("", "fr", "Bonjour")
		got, err := s.LastTargetAuthors(ctx, p.ID, "main", []string{bid}, []string{"fr"})
		require.NoError(t, err)
		assert.Empty(t, got, "a run writes with no acting user, so there is nobody to conflict with")
	})

	t.Run("the last person to write the target is the author", func(t *testing.T) {
		write("u-first", "fr", "Salut")
		write("u-second", "fr", "Bonjour à tous")
		got, err := s.LastTargetAuthors(ctx, p.ID, "main", []string{bid}, []string{"fr"})
		require.NoError(t, err)
		assert.Equal(t, "u-second", got[platstore.TargetRef{BlockID: bid, Locale: "fr"}])
	})

	t.Run("authorship is per locale", func(t *testing.T) {
		write("u-german", "de", "Guten Tag")
		got, err := s.LastTargetAuthors(ctx, p.ID, "main", []string{bid}, []string{"fr", "de"})
		require.NoError(t, err)
		assert.Equal(t, "u-second", got[platstore.TargetRef{BlockID: bid, Locale: "fr"}])
		assert.Equal(t, "u-german", got[platstore.TargetRef{BlockID: bid, Locale: "de"}])
	})

	t.Run("a decision does not make the decider the author", func(t *testing.T) {
		// The ledger files its own block_history row, carrying the DECIDER.
		// Reading the newest attributed row of any kind would name them as the
		// author and refuse everyone else's next approval.
		_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
			ItemName:    "greetings.txt",
			Unit:        unit,
			Variant:     "fr",
			Status:      string(model.TargetStatusReviewed),
			ReviewState: "approved",
			DecidedBy:   "reviewer@example.test",
			DecidedAt:   "2026-01-01T00:00:00Z",
			Updated:     "2026-01-01T00:00:00Z",
		}})
		require.NoError(t, err)

		got, err := s.LastTargetAuthors(ctx, p.ID, "main", []string{bid}, []string{"fr"})
		require.NoError(t, err)
		assert.Equal(t, "u-second", got[platstore.TargetRef{BlockID: bid, Locale: "fr"}],
			"the translator stays the author after the block is approved")
	})

	t.Run("an empty request reads nothing", func(t *testing.T) {
		got, err := s.LastTargetAuthors(ctx, p.ID, "main", nil, []string{"fr"})
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}
