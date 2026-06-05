package server

import (
	"encoding/json"
	"testing"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessDecisionSideEffects_ApproveTermCandidate(t *testing.T) {
	tb := newTestTBStore()
	ws := &workspaceStores{
		stores: map[string]*workspaceTMTB{
			"test-ws": {tb: tb},
		},
	}

	s := &Server{
		wsStores: ws,
	}

	data, _ := json.Marshal(model.TermCandidateAnnotation{
		Text:            "Dashboard",
		Definition:      "Main screen of the application",
		Category:        model.TermCategoryUI,
		Translatability: model.TranslatabilityConsistent,
		Confidence:      0.9,
		Locale:          "en-US",
		Source:          model.ExtractionSourceLLM,
	})

	item := &bstore.ReviewItem{
		ID:        "item-1",
		ProjectID: "proj-1",
		Type:      bstore.ReviewItemTermCandidate,
		Status:    bstore.ReviewItemApproved,
		Data:      data,
		Locale:    "en-US",
	}

	s.processDecisionSideEffects(t.Context(), item, "test-ws")

	// Verify term was added to termbase.
	concepts, err := tb.Concepts(t.Context())
	require.NoError(t, err)
	require.Len(t, concepts, 1)
	assert.Equal(t, "Main screen of the application", concepts[0].Definition)
	assert.Equal(t, "ui", concepts[0].Domain)
	require.Len(t, concepts[0].Terms, 1)
	assert.Equal(t, "Dashboard", concepts[0].Terms[0].Text)
	assert.Equal(t, model.TermApproved, concepts[0].Terms[0].Status)
}

func TestProcessDecisionSideEffects_ApproveTermDNT(t *testing.T) {
	tb := newTestTBStore()
	ws := &workspaceStores{
		stores: map[string]*workspaceTMTB{
			"ws": {tb: tb},
		},
	}

	rqs, cs := newTestReviewStoreForServer(t)

	s := &Server{
		wsStores:         ws,
		ReviewQueueStore: rqs,
	}
	_ = cs

	data, _ := json.Marshal(model.TermCandidateAnnotation{
		Text:            "Bowrain",
		Definition:      "Product name",
		Category:        model.TermCategoryBrand,
		Translatability: model.TranslatabilityDNT,
		Locale:          "en-US",
	})

	item := &bstore.ReviewItem{
		ID:        "item-2",
		ProjectID: "proj-1",
		Type:      bstore.ReviewItemTermCandidate,
		Status:    bstore.ReviewItemApproved,
		Data:      data,
		Locale:    "en-US",
	}

	s.processDecisionSideEffects(t.Context(), item, "ws")

	// Verify term was added with Preferred status.
	concepts, err := tb.Concepts(t.Context())
	require.NoError(t, err)
	require.Len(t, concepts, 1)
	assert.Equal(t, model.TermPreferred, concepts[0].Terms[0].Status)

	// Verify DNT entry was created.
	entries, err := rqs.ListDNTEntries(t.Context(), "proj-1")
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "Bowrain", entries[0].Text)
}

func TestProcessDecisionSideEffects_RejectTermCandidate(t *testing.T) {
	rqs, _ := newTestReviewStoreForServer(t)

	s := &Server{
		ReviewQueueStore: rqs,
	}

	data, _ := json.Marshal(map[string]any{
		"text":       "Widget",
		"definition": "A small component",
	})

	item := &bstore.ReviewItem{
		ID:        "item-3",
		ProjectID: "proj-1",
		Type:      bstore.ReviewItemTermCandidate,
		Status:    bstore.ReviewItemRejected,
		Data:      data,
		Locale:    "en-US",
	}

	s.processDecisionSideEffects(t.Context(), item, "ws")

	// Verify rejected term was recorded.
	rejected, err := rqs.IsRejected(t.Context(), "proj-1", "Widget", "en-US")
	require.NoError(t, err)
	assert.True(t, rejected)
}

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

func newTestTBStore() *testTermStore {
	return &testTermStore{InMemoryTermBase: termbase.NewInMemoryTermBase()}
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
