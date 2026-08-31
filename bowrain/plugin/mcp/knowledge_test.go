package bowrainmcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	coreproj "github.com/neokapi/neokapi/core/project"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	bproject "github.com/neokapi/neokapi/host/venue/project"
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
		assert.Equal(t, testWorkspace, r.PathValue("ws"))
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
			ID: r.PathValue("id"), Name: "Retire cockpit", Status: "in_review", CreatedBy: "alice",
			Governed: true,
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
		Defaults: coreproj.Defaults{SourceLanguage: "en"},
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

// blastRadiusServer serves the change-set detail surface with a blast-radius
// endpoint the caller controls, so a test can say what the server answers.
func blastRadiusServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/{ws}/changesets/{id}", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.ChangeSetDetail{
			ID: r.PathValue("id"), Name: "Retire cockpit", Status: "in_review",
			Governed: true,
		})
	})
	mux.HandleFunc("GET /api/v1/{ws}/changesets/{id}/blast-radius", handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestHandleExperimentStatusCarriesPartialBlastRadius pins the qualification on
// a truncated scan. The server's walk has a time budget; when it runs out, the
// counts it returns are lower bounds, and it says so with "partial". Dropping
// that on the way through this tool hands an assistant a smaller blast radius
// than the change really has, with nothing marking it as incomplete — and the
// assistant is summarising the change for someone deciding whether to approve
// it.
//
// "Lower bound" is itself too gentle, which the wording has to carry: the walk
// is one sequential pass aborted from its innermost loop, so a project it never
// reached contributes nothing at all. The shortfall is whole projects missing,
// not every project counted slightly low.
func TestHandleExperimentStatusCarriesPartialBlastRadius(t *testing.T) {
	srv := blastRadiusServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.ChangeSetImpact{
			TotalBlocks: 17500, AffectedBlocks: 900, Words: 12000,
			Partial: true,
			// Mirrors the server's own wording (bowrain/knowledge —
			// blastradius.go), which states the CAUSE only, so the composed
			// sentence below is the one a reader actually gets. The assertions
			// are on shape rather than on those exact words: a server-side
			// rewording must not fail this test, only a client that stops
			// embedding the reason should.
			PartialReason: "the scan reached this preview's time budget before it had covered the workspace",
			// A truncated walk still returns the projects it DID reach. They
			// must not surface as though they were the affected set.
			Projects: []apiclient.ProjectImpact{
				{ProjectID: "p-docs", ProjectName: "Docs", AffectedBlocks: 900},
			},
		})
	})
	setupKnowledgeProject(t, srv)

	_, out, err := handleExperimentStatus(context.Background(), MCPExperimentStatusInput{ChangesetID: "x-1"})
	require.NoError(t, err)
	require.NotNil(t, out.BlastRadius)
	assert.Equal(t, 900, out.BlastRadius.AffectedBlocks)
	assert.True(t, out.BlastRadius.Partial)
	assert.Contains(t, out.BlastRadius.CountsAre, "lower bounds")
	assert.Contains(t, out.BlastRadius.CountsAre, "did not reach",
		"the qualification must say that unreached projects contribute nothing, not merely that counts are low")
	assert.Contains(t, out.BlastRadius.CountsAre, "time budget",
		"the server's cause must survive into the sentence, not be replaced by this surface's own account of it")

	// The summary carries no per-project breakdown. Under a partial walk that
	// list is the projects EXAMINED, not the projects affected — absence from
	// it means "not reached". Serialising it would let an assistant name two
	// projects it never looked past.
	blob, merr := json.Marshal(out.BlastRadius)
	require.NoError(t, merr)
	assert.NotContains(t, string(blob), "p-docs")
	assert.NotContains(t, string(blob), "projects")
}

// TestHandleExperimentStatusReportsBlastRadiusFailure pins the other half: a
// blast-radius call that fails must not produce the same output as a change-set
// that touches nothing. A bare absent field for both reads to an assistant as
// "no impact" — the reassuring reading, and the wrong one.
func TestHandleExperimentStatusReportsBlastRadiusFailure(t *testing.T) {
	srv := blastRadiusServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"blast radius walk was cancelled"}`))
	})
	setupKnowledgeProject(t, srv)

	_, out, err := handleExperimentStatus(context.Background(), MCPExperimentStatusInput{ChangesetID: "x-1"})

	// The change-set detail is still worth returning; the radius is not silently absent.
	require.NoError(t, err)
	require.NotNil(t, out.Experiment)
	assert.Nil(t, out.BlastRadius)
	assert.NotEmpty(t, out.BlastRadiusError,
		"a radius that could not be computed must not read as a change that affects nothing")
}

func TestHandleConceptSearchRequiresWorkspace(t *testing.T) {
	srv := knowledgeTestServer(t)

	root := t.TempDir()
	recipe := &bproject.Recipe{
		Defaults: coreproj.Defaults{SourceLanguage: "en"},
		Server:   &bproject.ServerSpec{URL: srv.URL + "/projects/" + testProjectID},
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

// TestExperimentStatusCarriesTheReviewLink covers what an assistant hands back
// to the person who has to act on a change-set.
//
// The tool reported an id and nothing else, so the human reading the answer had
// no way to open the change-set: the review surface had to be found by grepping
// the frontend's routes. Both branches of the tool — the list and the single
// change-set detail — carry the deep link now.
func TestExperimentStatusCarriesTheReviewLink(t *testing.T) {
	srv := knowledgeTestServer(t)
	setupKnowledgeProject(t, srv)
	want := srv.URL + "/" + testWorkspace + "/context/changes/x-1"

	t.Run("the list", func(t *testing.T) {
		_, out, err := handleExperimentStatus(context.Background(), MCPExperimentStatusInput{})
		require.NoError(t, err)
		require.Len(t, out.Experiments, 1)
		assert.Equal(t, want, out.Experiments[0].ReviewURL)
	})

	t.Run("one change-set's detail", func(t *testing.T) {
		_, out, err := handleExperimentStatus(context.Background(), MCPExperimentStatusInput{ChangesetID: "x-1"})
		require.NoError(t, err)
		require.NotNil(t, out.Experiment)
		assert.Equal(t, want, out.Experiment.ReviewURL)
	})
}

// TestChangesetReviewURLDegradesWithoutAServerBase pins the omission contract:
// a project the link cannot be built for reports the id alone.
//
// A URL missing its host or its workspace segment resolves to a page that does
// not exist, and an assistant would hand that link on as if it worked — which
// reads to the reviewer as a proposal that went nowhere rather than as a hub
// the project is not connected to.
func TestChangesetReviewURLDegradesWithoutAServerBase(t *testing.T) {
	tests := []struct {
		name string
		proj *bproject.Project
	}{
		{name: "no project", proj: nil},
		{name: "no recipe", proj: &bproject.Project{}},
		{name: "no server block", proj: &bproject.Project{Recipe: &bproject.Recipe{}}},
		{
			name: "a server URL with no workspace segment",
			proj: &bproject.Project{Recipe: &bproject.Recipe{
				Server: &bproject.ServerSpec{URL: "https://bowrain.cloud"},
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Empty(t, changesetReviewURL(tc.proj, "x-1"))
		})
	}

	t.Run("an empty change-set id has nothing to link to", func(t *testing.T) {
		proj := &bproject.Project{Recipe: &bproject.Recipe{
			Server: &bproject.ServerSpec{URL: "https://bowrain.cloud/acme/proj-123"},
		}}
		assert.Empty(t, changesetReviewURL(proj, ""))
		assert.Equal(t, "https://bowrain.cloud/acme/context/changes/x-1", changesetReviewURL(proj, "x-1"))
	})
}
