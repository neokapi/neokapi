package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/bowrain/core/config"
	bowrainconn "github.com/neokapi/neokapi/bowrain/core/connector"
	bproject "github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/core/refcache"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/registry"
)

// The client half of the freshness layer: what a pull records, what survives a
// deleted cache, and what a push asserts.

// refServer serves a pull that carries a freshness ref, and records the
// assertion the commit that follows sent.
type refServer struct {
	*httptest.Server
	published    ref.Ref
	commitAssert ref.Ref
	commits      int
}

func newRefServer(t *testing.T, projectID string, published ref.Ref) *refServer {
	t.Helper()
	rs := &refServer{published: published}

	mux := http.NewServeMux()
	base := "/api/v1/projects/" + projectID
	mux.HandleFunc(base, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.ProjectMetadata{
			ID: projectID, DefaultSourceLanguage: "en", TargetLanguages: []string{"fr"},
		})
	})
	mux.HandleFunc(base+"/sync/main/ref", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(rs.published)
	})
	mux.HandleFunc(base+"/sync/main/pull", func(w http.ResponseWriter, _ *http.Request) {
		current := rs.published
		_ = json.NewEncoder(w).Encode(apiclient.RichPullResponse{
			Cursor: rs.published.Content, HasMore: false, Ref: &current,
		})
	})
	mux.HandleFunc(base+"/sync/main/push/init", func(w http.ResponseWriter, _ *http.Request) {
		current := rs.published
		_ = json.NewEncoder(w).Encode(apiclient.PushInitResponse{
			UploadID: "u1", Status: "diff_computed", ContextChanged: true, Ref: &current,
		})
	})
	mux.HandleFunc(base+"/sync/main/push/commit", func(w http.ResponseWriter, r *http.Request) {
		var manifest apiclient.PushCommitRequest
		_ = json.NewDecoder(r.Body).Decode(&manifest)
		rs.commitAssert = manifest.ExpectedRef
		rs.commits++
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"push_id": "p1", "status": "queued"})
	})

	rs.Server = httptest.NewServer(mux)
	t.Cleanup(rs.Close)
	return rs
}

// newRefConnector scaffolds a project pointed at srv with a real client.
func newRefConnector(t *testing.T, srv *httptest.Server, projectID string) *BowrainSourceConnector {
	t.Helper()
	root := t.TempDir()
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	recipe := &bproject.Recipe{
		KapiProject: coreproj.KapiProject{
			Defaults:    coreproj.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
			Collections: []coreproj.Collection{{Path: "locales/en.json", Format: &coreproj.FormatSpec{Name: "json"}}},
		},
		Server: &bproject.ServerSpec{URL: srv.URL + "/projects/" + projectID, Stream: "main"},
	}
	proj, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)

	abs := filepath.Join(root, "locales", "en.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(`{"greeting":"Hello"}`), 0o644))

	client := apiclient.NewProjectBearerClient(srv.URL, projectID, "test-token")
	client.SetStream("main")

	return &BowrainSourceConnector{
		project:   proj,
		client:    client,
		formatReg: reg,
		cache:     bproject.LoadSyncCache(proj.Layout),
		refs:      refcache.Load(proj.Layout, config.NormalizeServerURL(srv.URL), projectID),
		stream:    "main",
		maxBatch:  1000,
	}
}

func loadRefs(t *testing.T, conn *BowrainSourceConnector) *refcache.Cache {
	t.Helper()
	return refcache.Load(conn.project.Layout,
		config.NormalizeServerURL(conn.project.Recipe.Server.ServerURL()),
		conn.project.Recipe.Server.ProjectID())
}

// TestPull_RecordsTheServersRef: a pull is a client's cheapest contact with the
// server, and it is where a cache that was deleted comes back.
func TestPull_RecordsTheServersRef(t *testing.T) {
	published := ref.Ref{Content: 42, Context: "ctx-1", Terms: "trm-1", Decisions: "dec-1"}
	srv := newRefServer(t, "proj1", published)
	conn := newRefConnector(t, srv.Server, "proj1")

	func() {
		defer conn.Close()
		_, err := conn.Pull(context.Background(), bowrainconn.PullOptions{})
		require.NoError(t, err)
	}()

	got := loadRefs(t, conn).Ref("main")
	assert.Equal(t, int64(42), got.Content, "the position is what this project consumed")
	assert.Equal(t, "ctx-1", got.Context)
	assert.Equal(t, "trm-1", got.Terms)
	assert.Equal(t, "dec-1", got.Decisions)
}

// TestDeletedRefCacheCostsOneRoundTripNotAWrongResult: the whole justification
// for the file being disposable.
func TestDeletedRefCacheCostsOneRoundTripNotAWrongResult(t *testing.T) {
	published := ref.Ref{Content: 42, Context: "ctx-1", Terms: "trm-1", Decisions: "dec-1"}
	srv := newRefServer(t, "proj1", published)
	conn := newRefConnector(t, srv.Server, "proj1")

	func() {
		defer conn.Close()
		_, err := conn.Pull(context.Background(), bowrainconn.PullOptions{})
		require.NoError(t, err)
	}()
	before := loadRefs(t, conn).Ref("main")

	require.NoError(t, os.Remove(refcache.PathFor(conn.project.Layout)))
	empty := loadRefs(t, conn).Ref("main")
	require.True(t, empty.IsZero(), "a deleted cache claims nothing")

	// One round trip restores every governance component. The position is this
	// project's own and legitimately starts over — which costs a re-pull of
	// content already on disk, and never a wrong answer.
	fetched, err := conn.client.Ref(context.Background())
	require.NoError(t, err)
	assert.Equal(t, before.Context, fetched.Context)
	assert.Equal(t, before.Terms, fetched.Terms)
	assert.Equal(t, before.Decisions, fetched.Decisions)
}

// TestPull_AServerWithNoRefLeavesTheCachedOneStanding is the rc18/rc21
// compatibility contract on the client side: an answer that says nothing about
// governance must not be recorded as governance being empty.
func TestPull_AServerWithNoRefLeavesTheCachedOneStanding(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 7})
	conn := newRefConnector(t, srv.Server, "proj1")
	conn.refs.Observe("main", ref.Ref{Context: "ctx-1", Terms: "trm-1", Decisions: "dec-1"})

	func() {
		defer conn.Close()
		_, err := conn.Pull(context.Background(), bowrainconn.PullOptions{})
		require.NoError(t, err)
	}()

	got := loadRefs(t, conn).Ref("main")
	assert.Equal(t, "ctx-1", got.Context)
	assert.Equal(t, "trm-1", got.Terms)
	assert.Equal(t, "dec-1", got.Decisions)
	assert.Equal(t, int64(7), got.Content)
}

// TestPush_AssertsTheObservedRef: the compare-and-swap carries what this
// project last observed, not a value fetched at the start of this push — the
// payload was built by diffing against the observed one.
func TestPush_AssertsTheObservedRef(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 99, Context: "ctx-server", Decisions: "dec-server"})
	conn := newRefConnector(t, srv.Server, "proj1")
	conn.refs.Observe("main", ref.Ref{Context: "ctx-observed", Terms: "trm-observed", Decisions: "dec-observed"})

	_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)

	require.Equal(t, 1, srv.commits)
	assert.Equal(t, "ctx-observed", srv.commitAssert.Context)
	assert.Equal(t, "dec-observed", srv.commitAssert.Decisions)
}

// TestPush_RecordsWhatItPutInForce: after a confirmed push the project's ref
// carries the governance the push carried, so the next push skips the round
// trip when nothing moved — and the position is never rewound by a queued
// commit that answers with no position at all.
func TestPush_RecordsWhatItPutInForce(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{})
	conn := newRefConnector(t, srv.Server, "proj1")
	conn.refs.Consume("main", 31)

	pushCtx := apiclient.NewPushContext(nil)
	conn.SetPushContext(pushCtx)

	func() {
		defer conn.Close()
		_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
		require.NoError(t, err)
	}()

	got := loadRefs(t, conn).Ref("main")
	assert.Equal(t, pushCtx.Hash, got.Context, "the push records the context it put in force")
	assert.Equal(t, int64(31), got.Content,
		"a queued commit carries no position, and no position is never a rewind")
	assert.False(t, conn.PushContextChanged(), "the same context again is not worth a round trip")
}
