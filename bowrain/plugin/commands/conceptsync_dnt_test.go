package commands

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/terms"
)

// TestConceptSync_DoNotTranslateTravelsGoverned pulls a do-not-translate
// concept, then plans a push of a new concept carrying the flag and a local
// change that clears it on the pulled one. The pull keeps the flag. The plan
// proposes both changes as governed change-set ops, and no direct create or
// update carries the flag.
func TestConceptSync_DoNotTranslateTravelsGoverned(t *testing.T) {
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
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ctx := context.Background()
	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	tb := newProjectTerms(t)

	_, baseline, err := PullConcepts(ctx, client, tb, false)
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

	edits, err := PlanConcepts(ctx, client, tb, baseline, "")
	require.NoError(t, err)
	require.NotNil(t, edits)

	assert.Empty(t, edits.Creates, "a concept carrying the flag is not created directly")
	for _, u := range edits.Updates {
		assert.Nil(t, u.Concept.DoNotTranslate, "the direct update of %s does not carry the flag", u.ConceptID)
	}

	require.NotNil(t, edits.Proposal, "the flag travels in the proposal")
	var created, updated map[string]any
	for _, op := range edits.Proposal.Ops {
		var payload map[string]any
		require.NoError(t, json.Unmarshal(op.Payload, &payload))
		switch op.Op {
		case opConceptCreate:
			created = payload
		case opConceptUpdate:
			updated = payload
		}
	}
	require.NotNil(t, created, "the new concept is proposed as a governed create: %+v", edits.Proposal.Ops)
	concept, _ := created["concept"].(map[string]any)
	assert.Equal(t, true, concept["do_not_translate"])
	require.NotNil(t, updated, "clearing the flag is proposed as a governed update: %+v", edits.Proposal.Ops)
	assert.Equal(t, "c-kapi", updated["concept_id"])
	assert.Equal(t, false, updated["do_not_translate"])
	assert.Equal(t, conceptPushOrigin, edits.Proposal.Origin)
}
