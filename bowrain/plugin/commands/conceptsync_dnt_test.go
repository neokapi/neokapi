package commands

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/terms"
)

// TestConceptSync_DoNotTranslateTravelsGoverned pulls a do-not-translate
// concept, then pushes a new concept carrying the flag and a local change that
// clears it on the pulled one. The pull keeps the flag. The push proposes both
// changes as governed change-set ops, and no direct create or update carries
// the flag.
func TestConceptSync_DoNotTranslateTravelsGoverned(t *testing.T) {
	var mu sync.Mutex
	var creates []map[string]any
	updates := map[string]map[string]any{}
	var ops []recordedOp
	submitted := 0

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/{ws}/concepts", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"concepts": [{"id": "c-kapi", "domain": "product", "definition": "The command-line tool.",
			"do_not_translate": true,
			"terms": [{"text": "kapi", "locale": "en", "status": "approved"}],
			"created_at": "2026-01-01T10:00:00Z", "updated_at": "2026-01-02T10:00:00Z"}], "total_count": 1}`))
	})
	mux.HandleFunc("GET /api/v1/{ws}/concepts/{cid}/relations", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	mux.HandleFunc("POST /api/v1/{ws}/concepts", func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(r)
		mu.Lock()
		creates = append(creates, body)
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id": "c-created"}`))
	})
	mux.HandleFunc("PUT /api/v1/{ws}/concepts/{cid}", func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(r)
		mu.Lock()
		updates[r.PathValue("cid")] = body
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/{ws}/changesets", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id": "cs-1", "name": "kapi push", "status": "draft"}`))
	})
	mux.HandleFunc("POST /api/v1/{ws}/changesets/{id}/ops", func(w http.ResponseWriter, r *http.Request) {
		body := decodeBody(r)
		op := recordedOp{csID: r.PathValue("id")}
		op.op, _ = body["op"].(string)
		op.payload, _ = body["payload"].(map[string]any)
		mu.Lock()
		ops = append(ops, op)
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"seq": 1}`))
	})
	mux.HandleFunc("POST /api/v1/{ws}/changesets/{id}/submit", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		submitted++
		mu.Unlock()
		_, _ = w.Write([]byte(`{"id": "cs-1", "status": "in_review"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ctx := context.Background()
	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	tb := newProjectTerms(t)

	_, baseline, err := PullConcepts(ctx, client, tb, "", false)
	require.NoError(t, err)
	pulled, ok, err := tb.GetConcept(ctx, "c-kapi")
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, pulled.DoNotTranslate, "a pull keeps the flag")

	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID:             "c-new",
		Domain:         "product",
		DoNotTranslate: true,
		Terms:          []terms.Term{{Text: "Bowrain", Locale: "en", Status: model.TermProposed}},
	}))
	pulled.DoNotTranslate = false
	require.NoError(t, tb.AddConcept(ctx, pulled))

	_, err = PushConcepts(ctx, client, tb, baseline, "", false)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t, creates, "a concept carrying the flag is not created directly")
	for cid, body := range updates {
		assert.NotContains(t, body, "do_not_translate", "the direct update of %s does not carry the flag", cid)
	}

	var created, updated map[string]any
	for _, op := range ops {
		switch op.op {
		case opConceptCreate:
			created = op.payload
		case "concept.update":
			updated = op.payload
		}
	}
	require.NotNil(t, created, "the new concept is proposed as a governed create: %+v", ops)
	concept, _ := created["concept"].(map[string]any)
	assert.Equal(t, true, concept["do_not_translate"])
	require.NotNil(t, updated, "clearing the flag is proposed as a governed update: %+v", ops)
	assert.Equal(t, "c-kapi", updated["concept_id"])
	assert.Equal(t, false, updated["do_not_translate"])
	assert.Equal(t, 1, submitted, "the proposal is submitted for review")
}
