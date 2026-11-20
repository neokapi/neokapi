package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	bproject "github.com/neokapi/neokapi/host/venue/project"
	"github.com/neokapi/neokapi/terms"
)

// TestPullConceptsAsksForTheProjectsTerms: a pull asks the server for the
// concepts that apply in the client's project, and a concept a later pull
// no longer brings (deleted, or now limited to another project) leaves the
// local store, so the local checks stop enforcing it and a push does not
// propose it again.
func TestPullConceptsAsksForTheProjectsTerms(t *testing.T) {
	var (
		mu       sync.Mutex
		asked    []string
		concepts = []apiclient.ConceptInfo{
			{ID: "c-all", Terms: []apiclient.TermInfo{{Text: "dashboard", Locale: "en", Status: "approved"}}},
			{ID: "c-mine", Terms: []apiclient.TermInfo{{Text: "flowgraph", Locale: "en", Status: "approved"}}},
		}
	)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/{ws}/concepts", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked = append(asked, r.URL.Query().Get("project"))
		page := concepts
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(apiclient.ConceptSearchResult{Concepts: page, TotalCount: len(page)})
	})
	mux.HandleFunc("GET /api/v1/{ws}/concepts/{cid}/relations", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]terms.ConceptRelation{})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ctx := t.Context()
	client := apiclient.NewWorkspaceBowrainClient(srv.URL, "acme", "proj1", "tok")
	tb := newProjectTerms(t)
	// A concept of the checkout's own, which no pull brought.
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{ID: "c-local", Terms: []terms.Term{{Text: "workbench", Locale: "en", Status: model.TermApproved}}}))

	_, before, err := PullConcepts(ctx, client, tb, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"proj1"}, asked, "the pull names the project")

	// c-mine is limited to another project now; the server stops
	// answering it for proj1.
	mu.Lock()
	concepts = concepts[:1]
	mu.Unlock()
	_, after, err := PullConcepts(ctx, client, tb, false)
	require.NoError(t, err)
	require.NoError(t, dropWithdrawnTerms(ctx, tb, before, after))

	for cid, want := range map[string]bool{"c-all": true, "c-mine": false, "c-local": true} {
		_, held, err := tb.GetConcept(ctx, cid)
		require.NoError(t, err)
		assert.Equal(t, want, held, cid)
	}
	// A second drop finds nothing left to delete.
	require.NoError(t, dropWithdrawnTerms(ctx, tb, before, after))
}

func TestPullRemovesWithdrawnRelationsBetweenRetainedConcepts(t *testing.T) {
	ctx := t.Context()
	tb := newProjectTerms(t)
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, tb.AddConcept(ctx, terms.Concept{ID: id}))
	}
	for _, rel := range []terms.ConceptRelation{
		{ID: "withdrawn", SourceID: "a", TargetID: "b", RelationType: graph.LabelRelated},
		{ID: "retained", SourceID: "b", TargetID: "c", RelationType: graph.LabelRelated},
		{ID: "local", SourceID: "a", TargetID: "c", RelationType: graph.LabelRelated},
	} {
		require.NoError(t, tb.AddRelation(ctx, rel))
	}
	before := &bproject.ConceptBaseline{Relations: map[string]bproject.BaselineRelation{"withdrawn": {}, "retained": {}}}
	after := &bproject.ConceptBaseline{Relations: map[string]bproject.BaselineRelation{"retained": {}}}
	for range 2 {
		require.NoError(t, dropWithdrawnTerms(ctx, tb, before, after))
		relations, err := tb.ListRelations(ctx, nil)
		require.NoError(t, err)
		var ids []string
		for _, rel := range relations {
			ids = append(ids, rel.ID)
		}
		assert.ElementsMatch(t, []string{"local", "retained"}, ids)
	}
}
