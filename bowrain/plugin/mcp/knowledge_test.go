package bowrainmcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	bproject "github.com/neokapi/neokapi/bowrain/core/project"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testWorkspace = "acme"
	testProjectID = "proj-123"
)

// knowledgeTestServer serves the read-only knowledge-graph REST surface the MCP
// handlers exercise.
func knowledgeTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	concepts := []apiclient.ConceptInfo{
		{
			ID:         "c-dashboard",
			Domain:     "ui",
			Definition: "The product's main landing screen.",
			Terms: []apiclient.TermInfo{
				{Text: "Dashboard", Locale: "en", Status: "preferred"},
				{Text: "Cockpit", Locale: "en", Status: "forbidden"},
			},
			CreatedAt: "2024-01-01T10:00:00Z",
			UpdatedAt: "2024-02-01T10:00:00Z",
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/{ws}/concepts", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, testWorkspace, r.PathValue("ws"))
		_ = json.NewEncoder(w).Encode(apiclient.ConceptSearchResult{Concepts: concepts, TotalCount: len(concepts)})
	})
	mux.HandleFunc("GET /api/v1/{ws}/concepts/{cid}/story", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.ConceptStory{
			ConceptID: r.PathValue("cid"),
			Entries: []apiclient.ConceptStoryEntry{
				{Kind: "revision", At: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC), Actor: "alice", Summary: "created"},
			},
		})
	})
	mux.HandleFunc("GET /api/v1/{ws}/changesets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]apiclient.ChangeSet{
			{ID: "x-1", Name: "Retire cockpit", Status: "in_review", CreatedBy: "alice", CreatedAt: time.Now().UTC()},
		})
	})
	mux.HandleFunc("GET /api/v1/{ws}/changesets/{id}", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.ChangeSetDetail{
			ChangeSet: apiclient.ChangeSet{ID: r.PathValue("id"), Name: "Retire cockpit", Status: "in_review", CreatedBy: "alice"},
			Governed:  true,
		})
	})
	mux.HandleFunc("GET /api/v1/{ws}/changesets/{id}/blast-radius", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.ChangeSetImpact{
			TotalBlocks: 100, AffectedBlocks: 7, NewViolations: 5, Resolved: 2, Words: 120,
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// setupKnowledgeProject scaffolds a claimed, workspace-scoped project pointed at
// srv, chdirs into it, and supplies a CI auth token.
func setupKnowledgeProject(t *testing.T, srv *httptest.Server) {
	t.Helper()

	root := t.TempDir()
	recipe := &bproject.Recipe{
		KapiProject: coreproj.KapiProject{Defaults: coreproj.Defaults{SourceLanguage: "en"}},
		Server: &bproject.ServerSpec{
			URL:    srv.URL + "/" + testWorkspace + "/" + testProjectID,
			Stream: "main",
		},
	}
	_, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)

	t.Chdir(root)
	t.Setenv("BOWRAIN_AUTH_TOKEN", "test-token")
	t.Setenv("BOWRAIN_SERVER_URL", "")
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())
}

func TestHandleConceptSearch(t *testing.T) {
	srv := knowledgeTestServer(t)
	setupKnowledgeProject(t, srv)

	_, out, err := handleConceptSearch(context.Background(), MCPConceptSearchInput{Query: "dash"})
	require.NoError(t, err)
	assert.Equal(t, 1, out.TotalCount)
	require.Len(t, out.Concepts, 1)
	assert.Equal(t, "c-dashboard", out.Concepts[0].ID)
	require.Len(t, out.Concepts[0].Terms, 2)
	assert.Equal(t, "forbidden", out.Concepts[0].Terms[1].Status)
}

func TestHandleConceptStory(t *testing.T) {
	srv := knowledgeTestServer(t)
	setupKnowledgeProject(t, srv)

	_, out, err := handleConceptStory(context.Background(), MCPConceptStoryInput{ConceptID: "c-dashboard"})
	require.NoError(t, err)
	assert.Equal(t, "c-dashboard", out.ConceptID)
	require.Len(t, out.Entries, 1)
	assert.Equal(t, "revision", out.Entries[0].Kind)
}

func TestHandleExperimentStatusList(t *testing.T) {
	srv := knowledgeTestServer(t)
	setupKnowledgeProject(t, srv)

	_, out, err := handleExperimentStatus(context.Background(), MCPExperimentStatusInput{})
	require.NoError(t, err)
	require.Len(t, out.Experiments, 1)
	assert.Equal(t, "x-1", out.Experiments[0].ID)
	assert.Nil(t, out.Experiment)
	assert.Nil(t, out.BlastRadius)
}

func TestHandleExperimentStatusDetail(t *testing.T) {
	srv := knowledgeTestServer(t)
	setupKnowledgeProject(t, srv)

	_, out, err := handleExperimentStatus(context.Background(), MCPExperimentStatusInput{ChangesetID: "x-1"})
	require.NoError(t, err)
	require.NotNil(t, out.Experiment)
	assert.Equal(t, "x-1", out.Experiment.ID)
	assert.True(t, out.Experiment.Governed)
	require.NotNil(t, out.BlastRadius)
	assert.Equal(t, 7, out.BlastRadius.AffectedBlocks)
}

func TestHandleConceptSearchRequiresWorkspace(t *testing.T) {
	srv := knowledgeTestServer(t)

	root := t.TempDir()
	recipe := &bproject.Recipe{
		KapiProject: coreproj.KapiProject{Defaults: coreproj.Defaults{SourceLanguage: "en"}},
		Server:      &bproject.ServerSpec{URL: srv.URL + "/projects/" + testProjectID},
	}
	_, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)
	t.Chdir(root)
	t.Setenv("BOWRAIN_AUTH_TOKEN", "test-token")
	t.Setenv("BOWRAIN_SERVER_URL", "")
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())

	_, _, err = handleConceptSearch(context.Background(), MCPConceptSearchInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace")
}
