package source

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

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/ref"
	"github.com/neokapi/neokapi/core/ref/refcache"
	"github.com/neokapi/neokapi/core/registry"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/config"
	bproject "github.com/neokapi/neokapi/host/venue/project"
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
	// statusUnavailable makes the push-status route fail, so the ingest is
	// unconfirmable — a server with no worker, or one too old for the endpoint.
	statusUnavailable bool
	// afterCommit is what the ref answers once a commit has landed. It stands
	// in for the ordinary case where the server's fold and this client's differ
	// — an undeclared collection the server keeps, an approval it holds that
	// this client has not pulled.
	afterCommit *ref.Ref
	// unchanged makes the negotiation report the fast path, so a push commits
	// nothing at all.
	unchanged bool
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
		answer := rs.published
		if rs.commits > 0 && rs.afterCommit != nil {
			answer = *rs.afterCommit
		}
		_ = json.NewEncoder(w).Encode(answer)
	})
	mux.HandleFunc(base+"/sync/main/status", func(w http.ResponseWriter, _ *http.Request) {
		if rs.statusUnavailable {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"push_id": "p1", "status": "completed", "total": 1, "completed": 1,
		})
	})
	mux.HandleFunc(base+"/sync/main/pull", func(w http.ResponseWriter, _ *http.Request) {
		current := rs.published
		_ = json.NewEncoder(w).Encode(apiclient.RichPullResponse{
			Cursor: rs.published.Content, HasMore: false, Ref: &current,
		})
	})
	// A venue that holds nothing: every block the scan reads is missing, so the
	// push uploads. The producer diffs against this rather than against its own
	// cache, which is why the route has to exist for a push to behave at all.
	mux.HandleFunc(base+"/sync/main/tree", func(w http.ResponseWriter, _ *http.Request) {
		current := rs.published
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ref": current, "root_hash": "", "items": []any{},
		})
	})
	mux.HandleFunc(base+"/sync/main/push/chunks/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc(base+"/sync/main/push/init", func(w http.ResponseWriter, _ *http.Request) {
		current := rs.published
		status := "diff_computed"
		if rs.unchanged {
			status = apiclient.PushUnchanged
		}
		_ = json.NewEncoder(w).Encode(apiclient.PushInitResponse{
			UploadID: "u1", Status: status, ContextChanged: true, Ref: &current,
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
		Defaults:    coreproj.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
		Collections: []coreproj.Collection{{Path: "locales/en.json", Format: &coreproj.FormatSpec{Name: "json"}}},
		Server:      &bproject.ServerSpec{URL: srv.URL + "/projects/" + projectID, Stream: "main"},
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

// TestPush_LearnsTheGovernanceItDidNotWrite: a project that pushes before it
// has ever pulled has no cached ref, and the negotiation it already paid for is
// where the components this push does not write come from.
func TestPush_LearnsTheGovernanceItDidNotWrite(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Content: 99, Context: "ctx-server", Terms: "trm-server", Decisions: "dec-server"})
	conn := newRefConnector(t, srv.Server, "proj1")
	require.True(t, conn.refs.Ref("main").IsZero(), "a project that has never pulled claims nothing")

	conn.SetPushContext(apiclient.NewPushContext(nil))

	func() {
		defer conn.Close()
		_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
		require.NoError(t, err)
	}()

	assert.Equal(t, "trm-server", loadRefs(t, conn).Ref("main").Terms,
		"governance this push did not write comes from the negotiation")
}

// TestPush_RecordsTheServersGovernanceNotItsOwnFold is the lock-out guard.
//
// The server keeps a collection the recipe no longer declares, and keeps it
// recipe-owned — so the fold it publishes and the fold this client makes over
// what the recipe declares differ, permanently. If the push cached its own fold
// and then asserted it, every later governance push would be refused for a
// collection nobody moved, on a project that could never recover.
func TestPush_RecordsTheServersGovernanceNotItsOwnFold(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{})
	srv.afterCommit = &ref.Ref{Content: 5, Context: "ctx-with-leftover", Decisions: "dec-server"}

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
	assert.Equal(t, "ctx-with-leftover", got.Context, "the cached component is the server's, read back")
	assert.NotEqual(t, pushCtx.Hash, got.Context, "and not this client's own fold")
	assert.Equal(t, "dec-server", got.Decisions)
	assert.Equal(t, int64(31), got.Content,
		"a queued commit carries no position, and no position is never a rewind")

	// The next push asserts what the server answered, so it is accepted.
	require.NoError(t, conn.Close())
	_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	assert.Equal(t, "ctx-with-leftover", srv.commitAssert.Context)
}

// TestPush_ThatCommittedNothingKeepsWhatTheNegotiationReported: the fast path
// wrote nothing, so there is nothing to be unsure about — the negotiation's
// answer still describes the server, and clearing components over a push that
// never happened would send the next one back for no reason.
func TestPush_ThatCommittedNothingKeepsWhatTheNegotiationReported(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Context: "ctx-server", Terms: "trm-server", Decisions: "dec-server"})
	srv.unchanged = true

	conn := newRefConnector(t, srv.Server, "proj1")
	conn.SetPushContext(apiclient.NewPushContext(nil))

	func() {
		defer conn.Close()
		_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
		require.NoError(t, err)
	}()

	require.Zero(t, srv.commits, "the fast path commits nothing")
	got := loadRefs(t, conn).Ref("main")
	assert.Equal(t, "ctx-server", got.Context)
	assert.Equal(t, "trm-server", got.Terms)
	assert.Equal(t, "dec-server", got.Decisions)
}

// TestPush_ClaimsNothingWhenTheIngestIsUnconfirmed: a write the server has not
// confirmed leaves the components it carried empty rather than guessed. Empty
// asserts nothing and reads as worth sending again, so the next push re-sends an
// idempotent write instead of asserting a value that may never have landed.
func TestPush_ClaimsNothingWhenTheIngestIsUnconfirmed(t *testing.T) {
	srv := newRefServer(t, "proj1", ref.Ref{Context: "ctx-server", Terms: "trm-server", Decisions: "dec-server"})
	srv.statusUnavailable = true

	conn := newRefConnector(t, srv.Server, "proj1")
	conn.ObserveTermsRef("trm-pulled")
	conn.SetPushContext(apiclient.NewPushContext(nil))

	func() {
		defer conn.Close()
		_, err := conn.Push(context.Background(), bowrainconn.PushOptions{})
		require.NoError(t, err)
	}()

	got := loadRefs(t, conn).Ref("main")
	assert.Empty(t, got.Context, "an unconfirmed write claims nothing about what it carried")
	assert.Empty(t, got.Decisions)
	assert.Equal(t, "trm-pulled", got.Terms,
		"terminology is not the push path's to refresh, so what a concept pull observed stands")
	assert.True(t, conn.PushContextChanged(), "and the context is worth sending again")
}
