package commands

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli"
	coreproj "github.com/neokapi/neokapi/core/project"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	bproject "github.com/neokapi/neokapi/host/venue/project"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// returnLegServer serves a workspace holding one reviewed decision (a governed
// term status a platform admits only from a merged change-set) and one ordinary
// draft concept that no reviewer has touched.
func returnLegServer(t *testing.T) *httptest.Server {
	t.Helper()
	concepts := []apiclient.ConceptInfo{
		{
			ID:         "c-terms",
			Domain:     "product",
			Definition: "The project's vocabulary.",
			Terms: []apiclient.TermInfo{
				{Text: "terms", Locale: "en", Status: "preferred"},
				{Text: "termbase", Locale: "en", Status: "forbidden"},
			},
			CreatedAt: "2026-01-01T10:00:00Z",
			UpdatedAt: "2026-02-01T10:00:00Z",
		},
		{
			ID:         "c-draft",
			Domain:     "ui",
			Definition: "Somebody's scratch entry.",
			Terms: []apiclient.TermInfo{
				{Text: "scratchpad", Locale: "en", Status: "proposed"},
			},
			CreatedAt: "2026-01-01T10:00:00Z",
			UpdatedAt: "2026-01-01T10:00:00Z",
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/{ws}/concepts", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.ConceptSearchResult{Concepts: concepts, TotalCount: len(concepts)})
	})
	mux.HandleFunc("GET /api/v1/{ws}/concepts/{cid}/relations", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]terms.ConceptRelation{})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newReturnLegProject writes a workspace-connected project whose committed terms
// source already carries an authored concept the workspace has never seen.
func newReturnLegProject(t *testing.T, srvURL string) (*bproject.Project, string) {
	t.Helper()
	root := t.TempDir()
	proj, err := bproject.InitProject(root, &bproject.Recipe{
		Defaults: coreproj.Defaults{
			SourceLanguage: "en",
			TermsSource:    coreproj.RelStatePath(ktb.ConventionalName),
		},
		Server: &bproject.ServerSpec{URL: srvURL + "/acme/proj1", Stream: "main"},
	})
	require.NoError(t, err)

	authored := ktb.FromConcepts([]terms.Concept{{
		ID:         "c-authored",
		Source:     terms.TermSourceTerminology,
		Definition: "Only this checkout knows about it.",
		Terms:      []terms.Term{{Text: "faithful", Locale: "en", Status: "preferred"}},
	}})
	data, err := ktb.Marshal(authored)
	require.NoError(t, err)
	srcPath := filepath.Join(root, coreproj.RelStatePath(ktb.ConventionalName))
	require.NoError(t, os.MkdirAll(filepath.Dir(srcPath), 0o755))
	require.NoError(t, os.WriteFile(srcPath, data, 0o644))
	return proj, srcPath
}

func readTermsSource(t *testing.T, path string) *ktb.File {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	file, err := ktb.Unmarshal(data)
	require.NoError(t, err)
	return file
}

func hasConcept(f *ktb.File, id string) bool {
	for _, c := range f.Concepts {
		if c.ID == id {
			return true
		}
	}
	return false
}

// TestConceptPull_ProjectsReviewedDecisionsIntoGit is the terminology return
// leg end to end: a pull writes the workspace's vocabulary into the store as it
// always did, AND merges the reviewed decisions among it into the committed
// terms source — without touching the authored concept the workspace does not
// have, and without adopting the draft nobody approved.
func TestConceptPull_ProjectsReviewedDecisionsIntoGit(t *testing.T) {
	t.Setenv("BOWRAIN_AUTH_TOKEN", "tok")
	prev := app
	app = &cli.App{}
	t.Cleanup(func() {
		app.Shutdown()
		app = prev
	})

	srv := returnLegServer(t)
	proj, srcPath := newReturnLegProject(t, srv.URL)

	res, baseline, err := conceptPull(context.Background(), proj, false)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, baseline)
	assert.Equal(t, 2, res.Concepts, "both workspace concepts reached the store")

	assert.True(t, res.Projection.Written, "the reviewed decision reached the committed source")
	assert.Equal(t, 1, res.Projection.Added)
	assert.Equal(t, 1, res.Projection.Considered, "only the governed concept carried a decision")

	file := readTermsSource(t, srcPath)
	assert.True(t, hasConcept(file, "c-authored"), "the authored concept survived")
	assert.True(t, hasConcept(file, "c-terms"), "the reviewed decision landed")
	assert.False(t, hasConcept(file, "c-draft"), "raw workspace state stayed out of git")
}

// TestConceptPull_SecondPullWritesNothing: the nightly runs this every night,
// and a night with no new decisions must leave the working tree byte-identical.
// A file that churned on its own would manufacture `.kapi/` backing for derived
// artifacts nothing decided.
func TestConceptPull_SecondPullWritesNothing(t *testing.T) {
	t.Setenv("BOWRAIN_AUTH_TOKEN", "tok")
	prev := app
	app = &cli.App{}
	t.Cleanup(func() {
		app.Shutdown()
		app = prev
	})

	srv := returnLegServer(t)
	proj, srcPath := newReturnLegProject(t, srv.URL)

	_, _, err := conceptPull(context.Background(), proj, false)
	require.NoError(t, err)
	first, err := os.ReadFile(srcPath)
	require.NoError(t, err)

	res, _, err := conceptPull(context.Background(), proj, false)
	require.NoError(t, err)
	assert.False(t, res.Projection.Written, "the second pull wrote nothing")
	assert.Empty(t, cli.FormatTermsProjection(res.Projection), "and reported nothing")

	second, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second))
}
