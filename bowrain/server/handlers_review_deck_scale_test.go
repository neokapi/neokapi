package server

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/host/reviewdeck"
)

// blockReadCounter is the PostgreSQL store with a count of the block ids each
// GetBlocks call reads whole.
type blockReadCounter struct {
	*bstore.PostgresStore
	ids int
}

func (s *blockReadCounter) GetBlocks(ctx context.Context, q platstore.BlockQuery) ([]*venue.StoredBlock, error) {
	s.ids += len(q.IDs)
	if len(q.IDs) == 0 {
		// A read naming no ids reads its whole scope.
		s.ids += 1 << 20
	}
	return s.PostgresStore.GetBlocks(ctx, q)
}

// Once a deck's translations are classified, a request reads blocks whole only
// for the page it returns, however long the queue.
func TestReviewDeck_ReadsBlocksOnlyForThePage(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	const n = 300
	blocks := make([]*model.Block, 0, n)
	for i := range n {
		blocks = append(blocks, pendingFrBlock(fmt.Sprintf("b%03d", i), fmt.Sprintf("Source %03d", i), fmt.Sprintf("Cible %03d", i)))
	}
	projID, _ := seedGovernedProject(t, s, wsID, blocks)
	counter := &blockReadCounter{PostgresStore: s.ContentStore.(*bstore.PostgresStore)}
	s.ContentStore = counter

	first := readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}, "limit": {"10"}})
	assert.Equal(t, n, first.Counts.Total)
	assert.Zero(t, first.Counts.Unclassified)
	assert.Equal(t, n+10, counter.ids, "the first request classifies the queue in batches, then reads the page")

	counter.ids = 0
	second := readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}, "limit": {"10"}, "cursor": {first.NextCursor}})
	require.Len(t, second.Cards, 10)
	assert.Equal(t, 10, counter.ids, "a classified deck reads only the page's blocks")
	assert.NotEqual(t, first.Cards[0].BlockID, second.Cards[0].BlockID)
}

// A translation is classified again when a row its group was read from
// changes: a pre-review recorded after the first read moves it from closer to
// fast, and a new draft moves it back.
func TestReviewDeck_ReclassifiesWhatChanged(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{pendingFrBlock("b1", "Hello", "Bonjour")})
	bid := ids["Hello"]

	deck := readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}})
	require.Len(t, deck.Cards, 1)
	assert.Equal(t, reviewdeck.DeckCloser, deck.Cards[0].Group, "no pre-review score")

	recordScore(t, s, projID, bid, 95)
	deck = readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}})
	require.Len(t, deck.Cards, 1)
	assert.Equal(t, reviewdeck.DeckFast, deck.Cards[0].Group)
	require.NotNil(t, deck.Cards[0].PreReview)

	attributeTarget(t, s, projID, bid, "fr", "u-translator", "Bonjour à tous")
	deck = readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}})
	require.Len(t, deck.Cards, 1)
	assert.Equal(t, reviewdeck.DeckCloser, deck.Cards[0].Group, "the score judged the old draft")
	assert.Nil(t, deck.Cards[0].PreReview)
}
