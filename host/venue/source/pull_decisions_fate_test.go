package source

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/ref/refcache"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	"github.com/neokapi/neokapi/host"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/config"
	bproject "github.com/neokapi/neokapi/host/venue/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decisionLedgerServer serves the pull route the way the venue does: a page of
// changes addressed by the cursor, and the decision ledger in full beside it,
// whatever the cursor says. It records the cursor of every request so a test can
// see what the client asked for.
type decisionLedgerServer struct {
	*httptest.Server
	mu      sync.Mutex
	cursors []string
}

func (s *decisionLedgerServer) seenCursors() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.cursors...)
}

func newDecisionLedgerServer(t *testing.T, projectID string, decisions []venue.UnitDecision) *decisionLedgerServer {
	t.Helper()
	s := &decisionLedgerServer{}

	mux := http.NewServeMux()
	metaPath := "/api/v1/projects/" + projectID
	mux.HandleFunc(metaPath, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.ProjectMetadata{
			ID:                    projectID,
			DefaultSourceLanguage: "en",
			TargetLanguages:       []string{"fr"},
		})
	})
	mux.HandleFunc(metaPath+"/sync/main/pull", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.cursors = append(s.cursors, r.URL.Query().Get("cursor"))
		s.mu.Unlock()
		_ = json.NewEncoder(w).Encode(apiclient.RichPullResponse{
			Cursor:    42,
			HasMore:   false,
			Decisions: decisions,
		})
	})

	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Server.Close)
	return s
}

// decisionsProject scaffolds a project bound to srv, with no source files: this
// suite is about the decision ledger, and a pull with no blocks still carries
// one.
func decisionsProject(t *testing.T, srv *httptest.Server, projectID string) *bproject.Project {
	t.Helper()
	proj, err := bproject.InitProject(t.TempDir(), &bproject.Recipe{
		Defaults: coreproj.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
		},
		Server: &bproject.ServerSpec{
			URL:    srv.URL + "/projects/" + projectID,
			Stream: "main",
		},
	})
	require.NoError(t, err)
	return proj
}

// decisionsConnector builds a connector over an existing project, reading the
// refs from disk so a second connector picks up what the first one saved.
func decisionsConnector(t *testing.T, a *host.App, proj *bproject.Project, srv *httptest.Server, projectID string) *BowrainSourceConnector {
	t.Helper()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	client := apiclient.NewProjectBearerClient(srv.URL, projectID, "test-token")
	client.SetStream("main")

	return &BowrainSourceConnector{
		app:       a,
		project:   proj,
		client:    client,
		formatReg: reg,
		cache:     bproject.LoadSyncCache(proj.Layout),
		refs:      refcache.Load(proj.Layout, config.NormalizeServerURL(srv.URL), projectID),
		stream:    "main",
		maxBatch:  1000,
	}
}

// TestPull_StagedDecisionsSurviveDeletingTheStore: the working store and the
// ref cache sit in the same disposable directory and are deleted independently.
// A decision pulled into the store and not yet committed lives only there, so
// deleting the store must leave it recoverable: the next pull stages it again.
func TestPull_StagedDecisionsSurviveDeletingTheStore(t *testing.T) {
	const projectID = "proj-decisions"
	key := state.Key{Scope: "locales/en.json", Unit: "greeting", Variant: model.Variant("fr")}

	srv := newDecisionLedgerServer(t, projectID, []venue.UnitDecision{{
		ItemName:    "locales/en.json",
		Unit:        "greeting",
		Variant:     "fr",
		Status:      string(model.TargetStatusReviewed),
		ReviewState: "approved",
		DecidedBy:   "reviewer@example.test",
		Updated:     "2026-09-05T10:00:00Z",
	}})

	first := &host.App{}
	proj := decisionsProject(t, srv.Server, projectID)
	conn := decisionsConnector(t, first, proj, srv.Server, projectID)

	res, err := conn.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.DecisionsStaged)
	require.NoError(t, conn.Close())

	st, err := first.OpenProjectState(t.Context(), proj.Root)
	require.NoError(t, err)
	diff, err := st.RecordDiff(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, diff.Changed(), "the pulled decision is recorded, and the shards do not carry it yet")

	// Everything the pull recorded about its position is on disk, and so is the
	// store that position vouches for.
	saved := refcache.Load(proj.Layout, config.NormalizeServerURL(srv.Server.URL), projectID)
	require.Equal(t, int64(42), saved.Ref("main").Content)
	require.NotEmpty(t, saved.StoreID, "the position names the store that consumed it")

	// Delete the store alone. The ref cache under work/cache/ survives, which is
	// the shape this guards: one file gone, the other still claiming what it
	// held.
	first.Shutdown()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(proj.Layout.StorePath() + suffix)
	}
	require.NoFileExists(t, proj.Layout.StorePath())
	require.FileExists(t, filepath.Join(proj.Layout.CacheDir(), refcache.Filename))

	second := &host.App{}
	defer second.Shutdown()
	again := decisionsConnector(t, second, proj, srv.Server, projectID)

	res, err = again.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.DecisionsStaged, "the decision is staged again")
	require.NoError(t, again.Close())

	st, err = second.OpenProjectState(t.Context(), proj.Root)
	require.NoError(t, err)
	recovered, found := st.Get(t.Context(), key)
	require.True(t, found, "the decision is back in the project's working set")
	assert.Equal(t, "approved", recovered.Decision.ReviewState)
	assert.Equal(t, "reviewer@example.test", recovered.Decision.By)
	diff, err = st.RecordDiff(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, diff.Changed(), "and `kapi commit` still writes it into the record")

	// The second pull asked from the beginning: the position it held described a
	// store that no longer exists, so it made no claim about this one.
	assert.Equal(t, []string{"0", "0"}, srv.seenCursors(),
		"a store built again replays the feed rather than resuming")
}

// TestPull_RecordingTheSameLedgerTwiceChangesNothing: replaying the feed is
// only safe because recording an identical record is a no-op, so a project that
// pulls twice reports one decision and then none.
func TestPull_RecordingTheSameLedgerTwiceChangesNothing(t *testing.T) {
	const projectID = "proj-idempotent"

	srv := newDecisionLedgerServer(t, projectID, []venue.UnitDecision{{
		ItemName:    "locales/en.json",
		Unit:        "greeting",
		Variant:     "fr",
		ReviewState: "approved",
		Updated:     "2026-09-05T10:00:00Z",
	}})

	a := &host.App{}
	defer a.Shutdown()
	proj := decisionsProject(t, srv.Server, projectID)
	conn := decisionsConnector(t, a, proj, srv.Server, projectID)

	res, err := conn.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, res.DecisionsStaged)

	res, err = conn.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)
	assert.Zero(t, res.DecisionsStaged, "an identical record is left where it is")
	require.NoError(t, conn.Close())

	st, err := a.OpenProjectState(t.Context(), proj.Root)
	require.NoError(t, err)
	diff, err := st.RecordDiff(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, diff.Changed(), "one decision, however many times it arrived")
}

// TestPull_AnUnchangedStoreKeepsItsPosition: the guard fires on a store that was
// replaced and on nothing else, or every pull would replay the whole feed.
func TestPull_AnUnchangedStoreKeepsItsPosition(t *testing.T) {
	const projectID = "proj-position"

	srv := newDecisionLedgerServer(t, projectID, nil)

	a := &host.App{}
	defer a.Shutdown()
	proj := decisionsProject(t, srv.Server, projectID)
	conn := decisionsConnector(t, a, proj, srv.Server, projectID)

	_, err := conn.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	again := decisionsConnector(t, a, proj, srv.Server, projectID)
	_, err = again.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)
	require.NoError(t, again.Close())

	assert.Equal(t, []string{"0", "42"}, srv.seenCursors(),
		"the second pull resumes from what the first consumed")
}
