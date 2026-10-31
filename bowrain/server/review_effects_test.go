package server

import (
	"encoding/json"
	"testing"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessDecisionSideEffects_ApproveEntityDNT(t *testing.T) {
	rqs, _ := newTestReviewStoreForServer(t)

	s := &Server{
		ReviewQueueStore: rqs,
	}

	data, _ := json.Marshal(model.EntityAnnotation{
		Text: "Acme Corp",
		Type: model.EntityOrganization,
		DNT:  true,
	})

	item := &bstore.ReviewItem{
		ID:        "item-4",
		ProjectID: "proj-1",
		Type:      bstore.ReviewItemEntityReview,
		Status:    bstore.ReviewItemApproved,
		Data:      data,
		Locale:    "en-US",
	}

	s.processDecisionSideEffects(t.Context(), item, "ws")

	entries, err := rqs.ListDNTEntries(t.Context(), "proj-1")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "Acme Corp", entries[0].Text)
	assert.Equal(t, string(model.EntityOrganization), entries[0].EntityType)
}

// newTestReviewStoreForServer creates a ReviewQueueStore for tests.
func newTestReviewStoreForServer(t *testing.T) (*bstore.ReviewQueueStore, *bstore.PostgresStore) {
	t.Helper()
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)

	rqs := bstore.NewReviewQueueStore(db.DB)
	return rqs, cs
}

func newTestTermsStore() *testTermStore {
	return &testTermStore{InMemoryStore: terms.NewInMemoryStore()}
}
