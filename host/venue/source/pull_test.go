package source

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/ref/refcache"
	"github.com/neokapi/neokapi/core/registry"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/config"
	bproject "github.com/neokapi/neokapi/host/venue/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pullTestServer serves the minimal Bowrain REST surface that Pull exercises:
// project metadata and a single page of rich pull blocks. The blocks carry a
// translated target for every requested locale so writeTranslatedFile has work
// to do for each one.
func pullTestServer(t *testing.T, projectID string, targetLangs []string, pullCursor int64) *httptest.Server {
	t.Helper()

	syncBlock := func(name, source string) apiclient.SyncBlock {
		targets := map[string][]apiclient.SyncSegment{}
		for _, loc := range targetLangs {
			targets[loc] = []apiclient.SyncSegment{
				{Runs: []apiclient.SyncRun{
					{Text: &apiclient.SyncTextRun{Text: source + "-" + loc}},
				}},
			}
		}
		return apiclient.SyncBlock{
			ItemName:   "locales/en.json",
			Name:       name,
			SourceText: source,
			Targets:    targets,
		}
	}

	mux := http.NewServeMux()
	metaPath := "/api/v1/projects/" + projectID
	mux.HandleFunc(metaPath, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.ProjectMetadata{
			ID:                    projectID,
			DefaultSourceLanguage: "en",
			TargetLanguages:       targetLangs,
		})
	})
	mux.HandleFunc(metaPath+"/sync/main/pull", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.RichPullResponse{
			Cursor:  pullCursor,
			HasMore: false,
			Blocks: []apiclient.SyncBlock{
				syncBlock("greeting", "Hello"),
				syncBlock("farewell", "Goodbye"),
			},
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// newPullTestConnector scaffolds a JSON-source project pointed at srv and a
// connector with a real BowrainClient (no keychain/config dependency), with the
// sync cursor seeded so a fresh pull is forward-only.
func newPullTestConnector(t *testing.T, srv *httptest.Server, targetLangs []string, startCursor int64) *BowrainSourceConnector {
	t.Helper()
	root := t.TempDir()

	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	langs := make([]model.LocaleID, len(targetLangs))
	for i, l := range targetLangs {
		langs[i] = model.LocaleID(l)
	}

	projectID := "proj123"
	recipe := &bproject.Recipe{
		Defaults: coreproj.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: langs,
		},
		Collections: []coreproj.Collection{
			{Path: "locales/en.json", Format: &coreproj.FormatSpec{Name: "json"}},
		},
		Server: &bproject.ServerSpec{
			URL:    srv.URL + "/projects/" + projectID,
			Stream: "main",
		},
	}
	proj, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)

	abs := filepath.Join(root, "locales", "en.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(`{"greeting":"Hello","farewell":"Goodbye"}`), 0o644))

	serverURL := config.NormalizeServerURL(srv.URL)
	refs := refcache.Load(proj.Layout, serverURL, projectID)
	refs.Consume("main", startCursor)
	require.NoError(t, refs.Save(proj.Layout))

	client := apiclient.NewProjectBearerClient(srv.URL, projectID, "test-token")
	client.SetStream("main")

	return &BowrainSourceConnector{
		project:   proj,
		client:    client,
		formatReg: reg,
		cache:     bproject.LoadSyncCache(proj.Layout),
		refs:      refcache.Load(proj.Layout, serverURL, projectID),
		stream:    "main",
		maxBatch:  1000,
	}
}

// reloadRefs reads the connector's freshness refs back from disk.
func reloadRefs(t *testing.T, conn *BowrainSourceConnector) *refcache.Cache {
	t.Helper()
	return refcache.Load(conn.project.Layout,
		config.NormalizeServerURL(conn.project.Recipe.Server.ServerURL()),
		conn.project.Recipe.Server.ProjectID())
}

// TestPull_WriteFailureDoesNotAdvanceCursor verifies the core regression: when a
// target file fails to write, Pull must NOT advance/save the stream cursor past
// the unwritten change (the server's change feed is forward-only, so a swallowed
// failure would permanently lose that translation). The successful locale's file
// must still persist, and Pull must surface the failure as a non-nil error.
func TestPull_WriteFailureDoesNotAdvanceCursor(t *testing.T) {
	const startCursor = int64(5)
	const serverCursor = int64(42)
	targetLangs := []string{"fr", "de"}

	srv := pullTestServer(t, "proj123", targetLangs, serverCursor)
	conn := newPullTestConnector(t, srv, targetLangs, startCursor)
	defer conn.Close()

	// Force the "de" target write to fail: the default resolveTargetPath maps
	// the source "en" to the locale, so "de" lands at locales/de.json. Pre-create
	// that path as a directory so the writer cannot create the output file.
	deAsDir := filepath.Join(conn.project.Root, "locales", "de.json")
	require.NoError(t, os.MkdirAll(deAsDir, 0o755))

	res, err := conn.Pull(context.Background(), bowrainconn.PullOptions{})

	// Pull must report the failure rather than silently succeed.
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "cursor not advanced")

	// The cursor must remain at its pre-pull value so the next pull re-delivers
	// the changes the failed write dropped (forward-only feed).
	assert.Equal(t, startCursor, reloadRefs(t, conn).Ref("main").Content,
		"the position must not advance past an unwritten change")

	// The partial success (fr) must still have been written to disk.
	frPath := filepath.Join(conn.project.Root, "locales", "fr.json")
	frBytes, readErr := os.ReadFile(frPath)
	require.NoError(t, readErr, "successful locale write must persist")
	assert.Contains(t, string(frBytes), "Hello-fr")
}

// TestSetConceptBaseline_PersistsThroughCloseWithPullState pins the fix for the
// pull-ordering bug: a concept baseline recorded on the connector via
// SetConceptBaseline must survive the connector's single deferred Close()
// alongside the block-sync cursor that a real Pull advanced. The bug was that a
// concept pull saved the baseline to disk independently, only for the deferred
// conn.Close() to re-save the connector's own cache (which never carried the
// baseline) and erase it — leaving the next push permanently inert.
func TestSetConceptBaseline_PersistsThroughCloseWithPullState(t *testing.T) {
	const startCursor = int64(5)
	const serverCursor = int64(42)
	targetLangs := []string{"fr", "de"}

	srv := pullTestServer(t, "proj123", targetLangs, serverCursor)
	conn := newPullTestConnector(t, srv, targetLangs, startCursor)

	baseline := &bproject.ConceptBaseline{
		Concepts: map[string]bproject.BaselineConcept{
			"c-greeting": {Domain: "ui", Definition: "A salutation."},
		},
		Relations: map[string]bproject.BaselineRelation{
			"r-1": {SourceID: "c-greeting", TargetID: "c-cta", RelationType: "RELATED"},
		},
	}

	// Reproduce the runPull ordering exactly: a real block Pull advances + saves
	// the connector's in-memory cursor, then the folded concept pull records its
	// baseline on the same connector, and a single deferred Close() flushes both.
	func() {
		defer conn.Close()
		_, err := conn.Pull(context.Background(), bowrainconn.PullOptions{})
		require.NoError(t, err)
		conn.SetConceptBaseline(baseline)
	}()

	assert.Equal(t, serverCursor, reloadRefs(t, conn).Ref("main").Content,
		"the block-sync position must persist through the deferred Close")
	reloaded := bproject.LoadSyncCache(conn.project.Layout)
	require.NotNil(t, reloaded.ConceptBaseline,
		"concept baseline must survive the deferred conn.Close(), not be erased by it")
	assert.Len(t, reloaded.ConceptBaseline.Concepts, 1)
	assert.Len(t, reloaded.ConceptBaseline.Relations, 1)
}

// TestPull_UnreadableFormatHoldsTheCursor pins the other way a translation
// could be lost without a word: the server has targets for an item whose format
// this checkout cannot read — no recipe entry names one and no installed plugin
// claims the extension. That skip was a bare `continue`, so the pull wrote
// nothing for the item, advanced the cursor past it, and exited cleanly with
// "pulled N blocks, wrote 0 files". The stream is forward-only, so the
// translations were never offered again.
//
// It is the ordinary shape of a machine missing a format plugin, not an exotic
// one, and the honest outcome is the same as any other unwritable target: hold
// the cursor and say so, so the pull works once the format can be read.
func TestPull_UnreadableFormatHoldsTheCursor(t *testing.T) {
	const startCursor = int64(5)
	const serverCursor = int64(42)
	targetLangs := []string{"fr"}

	srv := pullTestServer(t, "proj123", targetLangs, serverCursor)
	conn := newPullTestConnector(t, srv, targetLangs, startCursor)
	defer conn.Close()

	// Strip the recipe's format binding and give the item an extension no
	// reader claims, so detectFormat finds nothing — exactly what a checkout
	// without the plugin for this format sees.
	conn.project.Recipe.Collections = nil
	conn.formatReg = registry.NewFormatRegistry()

	res, err := conn.Pull(context.Background(), bowrainconn.PullOptions{})

	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "no format is registered")
	assert.Contains(t, err.Error(), "cursor not advanced")

	assert.Equal(t, startCursor, reloadRefs(t, conn).Ref("main").Content,
		"the cursor must not advance past translations that could not be written")
}

// TestPull_AllWritesSucceedAdvancesCursor is the happy-path counterpart: when
// every target file writes cleanly, the cursor advances to the server cursor and
// is saved, and Pull returns the written-file count.
func TestPull_AllWritesSucceedAdvancesCursor(t *testing.T) {
	const startCursor = int64(5)
	const serverCursor = int64(42)
	targetLangs := []string{"fr", "de"}

	srv := pullTestServer(t, "proj123", targetLangs, serverCursor)
	conn := newPullTestConnector(t, srv, targetLangs, startCursor)
	defer conn.Close()

	res, err := conn.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, 2, res.FilesWritten)

	assert.Equal(t, serverCursor, reloadRefs(t, conn).Ref("main").Content,
		"cursor must advance to the server cursor when all writes succeed")

	for _, loc := range targetLangs {
		p := filepath.Join(conn.project.Root, "locales", loc+".json")
		b, readErr := os.ReadFile(p)
		require.NoError(t, readErr)
		assert.True(t, strings.Contains(string(b), "Hello-"+loc))
	}
}

// TestPull_CrossFormatTargetWritesTheTargetFormat pins the projection rule on
// the pull venue: a json source with a `catalogs/{lang}.mo` target must write a
// GNU MO catalog, not translated source-format bytes under a .mo name. The
// writer comes from the output path's extension (registry.WriterFormatFor),
// the same rule the host flow runner applies.
func TestPull_CrossFormatTargetWritesTheTargetFormat(t *testing.T) {
	targetLangs := []string{"nb"}
	srv := pullTestServer(t, "proj123", targetLangs, 42)
	conn := newPullTestConnector(t, srv, targetLangs, 5)
	defer conn.Close()

	conn.project.Recipe.Collections[0].Target = "catalogs/{lang}.mo"

	res, err := conn.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, 1, res.FilesWritten)

	out := filepath.Join(conn.project.Root, "catalogs", "nb.mo")
	b, readErr := os.ReadFile(out)
	require.NoError(t, readErr)
	require.GreaterOrEqual(t, len(b), 4, "catalog too short to carry the MO magic")
	assert.Equal(t, []byte{0xde, 0x12, 0x04, 0x95}, b[:4],
		"the .mo target must be a little-endian GNU MO catalog, not %q…", string(b[:min(len(b), 16)]))
	assert.True(t, strings.Contains(string(b), "Hello-nb"),
		"the catalog must carry the pulled translation")
}
