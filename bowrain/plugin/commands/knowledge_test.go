package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	bproject "github.com/neokapi/neokapi/bowrain/core/project"
	clioutput "github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/termbase"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testWorkspace = "acme"
	testProjectID = "proj-123"
)

// knowledgeTestServer serves the read-only knowledge-graph REST surface
// (Bowrain AD-021) that the concepts/experiments/terms commands exercise.
func knowledgeTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	concepts := []apiclient.ConceptInfo{
		{
			ID:         "c-dashboard",
			Domain:     "ui",
			Definition: "The product's main landing screen.",
			Terms: []apiclient.TermInfo{
				{Text: "Dashboard", Locale: "en", Status: "preferred"},
				{Text: "Tableau de bord", Locale: "fr", Status: "preferred"},
				{Text: "Cockpit", Locale: "en", Status: "forbidden"},
			},
			CreatedAt: "2024-01-01T10:00:00Z",
			UpdatedAt: "2024-02-01T10:00:00Z",
		},
		{
			ID:         "c-cockpit",
			Domain:     "ui",
			Definition: "Deprecated synonym for the dashboard.",
			Terms: []apiclient.TermInfo{
				{Text: "Cockpit", Locale: "en", Status: "deprecated"},
			},
			CreatedAt: "2024-01-03T10:00:00Z",
			UpdatedAt: "2024-01-03T10:00:00Z",
		},
	}

	relation := termbase.ConceptRelation{
		ID:           "r-1",
		SourceID:     "c-cockpit",
		TargetID:     "c-dashboard",
		RelationType: graph.LabelReplacedBy,
		Note:         "renamed in 2024",
		CreatedAt:    time.Date(2024, 1, 3, 10, 0, 0, 0, time.UTC),
	}

	submitted := time.Date(2024, 3, 1, 9, 0, 0, 0, time.UTC)
	changeset := apiclient.ChangeSet{
		ID:        "x-1",
		Name:      "Retire cockpit",
		Status:    "in_review",
		CreatedBy: "alice",
		CreatedAt: time.Date(2024, 2, 20, 9, 0, 0, 0, time.UTC),
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/{ws}/concepts", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, testWorkspace, r.PathValue("ws"))
		_ = json.NewEncoder(w).Encode(apiclient.ConceptSearchResult{
			Concepts:   concepts,
			TotalCount: len(concepts),
		})
	})
	mux.HandleFunc("GET /api/v1/{ws}/concepts/{cid}", func(w http.ResponseWriter, r *http.Request) {
		cid := r.PathValue("cid")
		for _, c := range concepts {
			if c.ID == cid {
				_ = json.NewEncoder(w).Encode(c)
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	})
	mux.HandleFunc("GET /api/v1/{ws}/concepts/{cid}/story", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.ConceptStory{
			ConceptID: r.PathValue("cid"),
			Entries: []apiclient.ConceptStoryEntry{
				{Kind: "revision", At: time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC), Actor: "alice", Summary: "created"},
				{Kind: "changeset", At: time.Date(2024, 3, 1, 9, 0, 0, 0, time.UTC), Actor: "bob", Summary: "retire cockpit", Ref: "x-1"},
			},
		})
	})
	mux.HandleFunc("GET /api/v1/{ws}/concepts/{cid}/relations", func(w http.ResponseWriter, r *http.Request) {
		cid := r.PathValue("cid")
		if cid == "c-dashboard" || cid == "c-cockpit" {
			_ = json.NewEncoder(w).Encode([]termbase.ConceptRelation{relation})
			return
		}
		_ = json.NewEncoder(w).Encode([]termbase.ConceptRelation{})
	})

	mux.HandleFunc("GET /api/v1/{ws}/changesets", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]apiclient.ChangeSet{changeset})
	})
	mux.HandleFunc("GET /api/v1/{ws}/changesets/{id}", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.ChangeSetDetail{
			ChangeSet: apiclient.ChangeSet{
				ID:          r.PathValue("id"),
				Name:        "Retire cockpit",
				Description: "Forbid cockpit, prefer dashboard",
				Status:      "in_review",
				CreatedBy:   "alice",
				CreatedAt:   changeset.CreatedAt,
				SubmittedAt: &submitted,
			},
			Governed: true,
			Ops: []apiclient.ChangeSetOp{
				{Seq: 1, Op: "set_term_status"},
				{Seq: 2, Op: "add_relation"},
			},
			Reviews: []apiclient.ChangeSetReview{
				{Reviewer: "bob", Verdict: "approve", Comment: "ship it"},
			},
			Pilots: []apiclient.Pilot{
				{ProjectID: "proj-123", Stream: "main"},
			},
		})
	})
	mux.HandleFunc("GET /api/v1/{ws}/changesets/{id}/blast-radius", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.ChangeSetImpact{
			TotalBlocks:    100,
			AffectedBlocks: 7,
			NewViolations:  5,
			Resolved:       2,
			Words:          120,
			Projects: []apiclient.ProjectImpact{
				{ProjectID: "proj-123", ProjectName: "Web", AffectedBlocks: 7, NewViolations: 5, Resolved: 2, Words: 120},
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// setupKnowledgeProject scaffolds a claimed, workspace-scoped kapi project
// pointed at srv, chdirs into it, and supplies a CI auth token, so the
// knowledge commands resolve a workspace client exactly as in production.
func setupKnowledgeProject(t *testing.T, srv *httptest.Server) *bproject.Project {
	t.Helper()

	root := t.TempDir()
	recipe := &bproject.Recipe{
		KapiProject: coreproj.KapiProject{
			Defaults: coreproj.Defaults{SourceLanguage: "en"},
		},
		Server: &bproject.ServerSpec{
			URL:    srv.URL + "/" + testWorkspace + "/" + testProjectID,
			Stream: "main",
		},
	}
	proj, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)

	t.Chdir(root)
	t.Setenv("BOWRAIN_AUTH_TOKEN", "test-token")
	t.Setenv("BOWRAIN_SERVER_URL", "")
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())

	return proj
}

// runKnowledgeCmd builds a fresh root with the persistent output flags and a
// fresh knowledge command tree (factories, so no flag state leaks between runs),
// executes it, and returns captured stdout.
func runKnowledgeCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()

	root := &cobra.Command{Use: "kapi"}
	clioutput.AddPersistentFlags(root)
	root.AddCommand(newConceptsCmd(), newExperimentsCmd(), newTermsCmd())

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)

	err := root.Execute()
	return out.String(), err
}

func TestConceptsList(t *testing.T) {
	srv := knowledgeTestServer(t)
	setupKnowledgeProject(t, srv)

	t.Run("text", func(t *testing.T) {
		out, err := runKnowledgeCmd(t, "concepts", "list")
		require.NoError(t, err)
		assert.Contains(t, out, "c-dashboard")
		assert.Contains(t, out, "Dashboard [en]")
		assert.Contains(t, out, "2 concept(s)")
	})

	t.Run("json", func(t *testing.T) {
		out, err := runKnowledgeCmd(t, "concepts", "list", "--json")
		require.NoError(t, err)
		var got struct {
			Concepts   []struct{ ID string } `json:"concepts"`
			TotalCount int                   `json:"total_count"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &got))
		assert.Equal(t, 2, got.TotalCount)
		require.Len(t, got.Concepts, 2)
		assert.Equal(t, "c-dashboard", got.Concepts[0].ID)
	})
}

func TestConceptsShow(t *testing.T) {
	srv := knowledgeTestServer(t)
	setupKnowledgeProject(t, srv)

	t.Run("text", func(t *testing.T) {
		out, err := runKnowledgeCmd(t, "concepts", "show", "c-dashboard")
		require.NoError(t, err)
		assert.Contains(t, out, "Concept: c-dashboard")
		assert.Contains(t, out, "Tableau de bord")
		assert.Contains(t, out, "Cockpit (forbidden)")
		// The relation endpoint is folded into show output.
		assert.Contains(t, out, "REPLACED_BY")
	})

	t.Run("json", func(t *testing.T) {
		out, err := runKnowledgeCmd(t, "concepts", "show", "c-dashboard", "--json")
		require.NoError(t, err)
		var got struct {
			ID        string `json:"id"`
			Terms     []struct{ Text, Locale, Status string }
			Relations []struct {
				Type     string `json:"type"`
				SourceID string `json:"source_id"`
			} `json:"relations"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &got))
		assert.Equal(t, "c-dashboard", got.ID)
		assert.Len(t, got.Terms, 3)
		require.Len(t, got.Relations, 1)
		assert.Equal(t, "REPLACED_BY", got.Relations[0].Type)
	})
}

func TestConceptsStory(t *testing.T) {
	srv := knowledgeTestServer(t)
	setupKnowledgeProject(t, srv)

	out, err := runKnowledgeCmd(t, "concepts", "story", "c-dashboard")
	require.NoError(t, err)
	assert.Contains(t, out, "Story of concept c-dashboard")
	assert.Contains(t, out, "[revision]")
	assert.Contains(t, out, "[changeset]")
	assert.Contains(t, out, "(x-1)")

	jsonOut, err := runKnowledgeCmd(t, "concepts", "story", "c-dashboard", "--json")
	require.NoError(t, err)
	var got struct {
		ConceptID string `json:"concept_id"`
		Entries   []struct{ Kind string }
	}
	require.NoError(t, json.Unmarshal([]byte(jsonOut), &got))
	assert.Equal(t, "c-dashboard", got.ConceptID)
	assert.Len(t, got.Entries, 2)
}

func TestExperimentsListAndShow(t *testing.T) {
	srv := knowledgeTestServer(t)
	setupKnowledgeProject(t, srv)

	listOut, err := runKnowledgeCmd(t, "experiments", "list")
	require.NoError(t, err)
	assert.Contains(t, listOut, "x-1")
	assert.Contains(t, listOut, "in_review")
	assert.Contains(t, listOut, "Retire cockpit")

	showOut, err := runKnowledgeCmd(t, "experiments", "show", "x-1")
	require.NoError(t, err)
	assert.Contains(t, showOut, "Experiment: x-1")
	assert.Contains(t, showOut, "Governed: yes")
	assert.Contains(t, showOut, "set_term_status")
	assert.Contains(t, showOut, "bob: approve")
	assert.Contains(t, showOut, "proj-123 @ main")
}

func TestExperimentsBlastRadius(t *testing.T) {
	srv := knowledgeTestServer(t)
	setupKnowledgeProject(t, srv)

	out, err := runKnowledgeCmd(t, "experiments", "blast-radius", "x-1")
	require.NoError(t, err)
	assert.Contains(t, out, "Blast radius of experiment x-1")
	assert.Contains(t, out, "Affected blocks: 7 of 100")
	assert.Contains(t, out, "New violations:  5")

	jsonOut, err := runKnowledgeCmd(t, "experiments", "blast-radius", "x-1", "--json")
	require.NoError(t, err)
	var got struct {
		AffectedBlocks int `json:"affected_blocks"`
		Projects       []struct {
			ProjectName    string `json:"project_name"`
			AffectedBlocks int    `json:"affected_blocks"`
		} `json:"projects"`
	}
	require.NoError(t, json.Unmarshal([]byte(jsonOut), &got))
	assert.Equal(t, 7, got.AffectedBlocks)
	require.Len(t, got.Projects, 1)
	assert.Equal(t, "Web", got.Projects[0].ProjectName)
}

func TestTermsPull(t *testing.T) {
	srv := knowledgeTestServer(t)
	proj := setupKnowledgeProject(t, srv)

	out, err := runKnowledgeCmd(t, "terms", "pull")
	require.NoError(t, err)
	assert.Contains(t, out, "Pulled 2 concept(s)")
	assert.Contains(t, out, "1 relation(s)")
	assert.Contains(t, out, "kapi verify --terms")

	// The concepts must be queryable in the project's bound termbase so a
	// subsequent offline `kapi verify --terms` gates against them.
	dbPath := filepath.Join(proj.StateDir(), "termbase.db")
	_, statErr := os.Stat(dbPath)
	require.NoError(t, statErr, "terms pull must create the bound termbase")

	tb, err := termbase.NewSQLiteTermBase(dbPath)
	require.NoError(t, err)
	defer tb.Close()

	ctx := context.Background()
	concepts, err := tb.Concepts(ctx)
	require.NoError(t, err)
	assert.Len(t, concepts, 2)

	dash, found, err := tb.GetConcept(ctx, "c-dashboard")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "ui", dash.Domain)

	// A forbidden term landed with its status intact, ready for term gating.
	var forbidden bool
	for _, term := range dash.Terms {
		if term.Text == "Cockpit" && term.Status == model.TermForbidden {
			forbidden = true
		}
	}
	assert.True(t, forbidden, "forbidden term status must survive the pull")

	// The typed relation between the two pulled concepts must be present.
	rels, err := tb.RelationsOf(ctx, "c-dashboard", nil)
	require.NoError(t, err)
	require.Len(t, rels, 1)
	assert.Equal(t, graph.LabelReplacedBy, rels[0].RelationType)
}

// TestKnowledgeRequiresWorkspace verifies the knowledge commands refuse a
// project that is not claimed into a workspace (claim-token/direct project),
// since the graph is workspace-scoped.
func TestKnowledgeRequiresWorkspace(t *testing.T) {
	srv := knowledgeTestServer(t)

	root := t.TempDir()
	recipe := &bproject.Recipe{
		KapiProject: coreproj.KapiProject{
			Defaults: coreproj.Defaults{SourceLanguage: "en"},
		},
		Server: &bproject.ServerSpec{
			URL: srv.URL + "/projects/" + testProjectID, // direct project, no workspace
		},
	}
	_, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)
	t.Chdir(root)
	t.Setenv("BOWRAIN_AUTH_TOKEN", "test-token")
	t.Setenv("BOWRAIN_SERVER_URL", "")
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())

	_, err = runKnowledgeCmd(t, "concepts", "list")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace")
}
