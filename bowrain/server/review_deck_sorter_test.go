package server

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

// The sorter classifies every translation waiting for review, past the
// batch a deck request classifies at once, so the deck's counts hold no
// unclassified part afterwards. A second sweep finds nothing to do.
func TestDeckSorter_SortsEveryWaitingTranslation(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	const n = deckClassifyBatch*2 + 50
	blocks := make([]*model.Block, 0, n)
	for i := range n {
		b := &model.Block{ID: fmt.Sprintf("b%d", i), Translatable: true}
		b.SetSourceText(fmt.Sprintf("Sentence number %d.", i))
		b.SetTargetText("fr", fmt.Sprintf("Phrase numéro %d.", i))
		blocks = append(blocks, b)
	}
	pid, _ := seedReviewProject(t, cs, blocks)

	decks, ok := store.ReviewDeckStores(srv.ContentStore)
	require.True(t, ok)
	q := store.ReviewDeckQuery{ProjectID: pid, Stream: "main", Locale: "fr", Policy: deckPolicyKey(srv.deckPolicy())}
	before, err := decks.CountReviewDeck(t.Context(), q)
	require.NoError(t, err)
	assert.Equal(t, n, before.Unclassified, "nothing is sorted before the sweep")

	d := &deckSorter{s: srv, db: cs.SQLDB(), stop: make(chan struct{})}
	d.sweep()

	after, err := decks.CountReviewDeck(t.Context(), q)
	require.NoError(t, err)
	assert.Zero(t, after.Unclassified, "the sweep sorts every waiting translation")
	sorted := 0
	for _, c := range after.ByRank {
		sorted += c
	}
	assert.Equal(t, n, sorted)

	again, err := srv.sortDecks(t.Context())
	require.NoError(t, err)
	assert.Zero(t, again, "a second sweep finds nothing to sort")
}
