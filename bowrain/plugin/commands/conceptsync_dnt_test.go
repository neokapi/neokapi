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

// TestConceptSync_CarriesDoNotTranslate pulls a do-not-translate concept, then
// pushes a new one and a change to the pulled one. The flag arrives in the
// project's terms, and each push sends it.
func TestConceptSync_CarriesDoNotTranslate(t *testing.T) {
	var mu sync.Mutex
	var creates []map[string]any
	updates := map[string]map[string]any{}

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
	require.Len(t, creates, 1)
	assert.Equal(t, true, creates[0]["do_not_translate"], "a pushed concept carries the flag")
	require.Contains(t, updates, "c-kapi", "clearing the flag is a change the push sends")
	assert.Equal(t, false, updates["c-kapi"]["do_not_translate"])
}
