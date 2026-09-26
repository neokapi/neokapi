package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/contextop"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
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

// newReturnLegProject writes a workspace-connected project whose checkout still
// carries a terms bundle from the old layout, so a test can watch the pull
// leave it alone.
func newReturnLegProject(t *testing.T, srvURL string) (*bproject.Project, string) {
	t.Helper()
	root := t.TempDir()
	proj, err := bproject.InitProject(root, &bproject.Recipe{
		KapiProject: coreproj.KapiProject{ //nolint:modernize // the embedded type is named, so the recipe's two halves read as two things
			ID:   "prj_returnleg22222222222222",
			Name: "return leg",
			Defaults: coreproj.Defaults{
				SourceLanguage: "en",
			},
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

// returnLegApp installs a host App for the duration of one test, so the pull
// reaches a project store and a context log of its own.
func returnLegApp(t *testing.T) {
	t.Helper()
	// A workspace of its own, so one test's record is not another's.
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	prev := app
	app = &cli.App{}
	t.Cleanup(func() {
		app.Shutdown()
		app = prev
	})
}

// storedConceptIDs reads back what the project's terms store holds.
func storedConceptIDs(t *testing.T, proj *bproject.Project) []string {
	t.Helper()
	tb, err := projectTerms(t.Context(), proj)
	require.NoError(t, err)
	held, err := tb.Concepts(t.Context())
	require.NoError(t, err)
	ids := make([]string, 0, len(held))
	for _, c := range held {
		ids = append(ids, c.ID)
	}
	return ids
}

// contextLog reads the project's recorded operations, newest first.
func contextLog(t *testing.T, proj *bproject.Project) []host.ContextOperation {
	t.Helper()
	list, err := app.ContextOperations(t.Context(), host.ContextLogRequest{Project: proj.Layout.RecipePath})
	require.NoError(t, err)
	return list.Operations
}

// TestConceptPull_WritesTheStoreAndRecordsTheArrival: a pull puts the
// workspace's terminology in the project's terms store, leaves every file in
// the checkout alone, and says on the project's record where the terminology
// came from.
func TestConceptPull_WritesTheStoreAndRecordsTheArrival(t *testing.T) {
	t.Setenv("BOWRAIN_AUTH_TOKEN", "tok")
	returnLegApp(t)

	srv := returnLegServer(t)
	proj, srcPath := newReturnLegProject(t, srv.URL)
	before, err := os.ReadFile(srcPath)
	require.NoError(t, err)

	res, baseline, err := conceptPull(t.Context(), proj, false)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, baseline)
	assert.Equal(t, 2, res.Concepts, "both workspace concepts reached the store")
	assert.ElementsMatch(t, []string{"c-terms", "c-draft"}, storedConceptIDs(t, proj))

	after, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "the pull writes the store and no file")

	ops := contextLog(t, proj)
	require.Len(t, ops, 1, "the arrival is one line on the record")
	assert.Equal(t, res.Recorded, ops[0].ID)
	assert.Equal(t, contextop.ActorTool, ops[0].Actor.Kind, "nobody at this keyboard decided it")
	assert.Equal(t, "acme", ops[0].Actor.Name, "the workspace it came from")
	assert.Equal(t, contextop.KindObserve, ops[0].Kind)
	assert.Contains(t, ops[0].Subject.Text, "2 concepts")
}

// TestConceptPull_SecondPullRecordsNothing: the nightly runs this every night,
// and a night with no new terminology must leave the record where it stood. A
// log growing a line a night would bury the entries a person wants to read.
func TestConceptPull_SecondPullRecordsNothing(t *testing.T) {
	t.Setenv("BOWRAIN_AUTH_TOKEN", "tok")
	returnLegApp(t)

	srv := returnLegServer(t)
	proj, _ := newReturnLegProject(t, srv.URL)

	first, baseline, err := conceptPull(t.Context(), proj, false)
	require.NoError(t, err)
	require.NotNil(t, baseline)
	assert.NotEmpty(t, first.Recorded)

	// The connector persists the baseline on Close, which nothing here opens,
	// so the second pull is handed the same baseline the first one produced.
	cache := bproject.LoadSyncCache(proj.Layout)
	cache.ConceptBaseline = baseline
	require.NoError(t, cache.Save(proj.Layout))

	second, _, err := conceptPull(t.Context(), proj, false)
	require.NoError(t, err)
	assert.Empty(t, second.Recorded, "the second pull recorded nothing")
	assert.Len(t, contextLog(t, proj), 1, "and left the record where it stood")
}
