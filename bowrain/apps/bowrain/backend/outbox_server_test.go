package backend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/editorclient"
	"github.com/neokapi/neokapi/bowrain/server"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host/venue/config"
)

// liveServer is the real Bowrain server, on a PostgreSQL database of its own,
// served over HTTP: the server the desktop replays its outbox to.
type liveServer struct {
	url string
	ws  string
	// tokens are the bearer tokens of the workspace's members, by name.
	tokens map[string]string
}

// startLiveServer starts the server as it runs in production (its stores
// opened from a database URL) with a workspace "acme" that Alice and Bob own.
func startLiveServer(t *testing.T) liveServer {
	t.Helper()
	db := pgtest.NewTestDB(t)
	// The test's database is a copy of the template under a name of its own;
	// the connection string names the database the copy was made in.
	var name string
	require.NoError(t, db.QueryRow("SELECT current_database()").Scan(&name))
	dbURL, err := url.Parse(db.ConnStr())
	require.NoError(t, err)
	dbURL.Path = "/" + name
	cfg := server.DefaultConfig()
	cfg.DatabaseURL = dbURL.String()
	cfg.JWTSecret = "outbox-test-secret"
	srv := server.NewServer(cfg)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	require.NotNil(t, srv.AuthStore, "the server opened its stores on the database")

	ctx := t.Context()
	ws := &platauth.Workspace{ID: "ws-acme", Name: "Acme", Slug: "acme", Type: platauth.WorkspaceTypeTeam}
	require.NoError(t, srv.AuthStore.CreateWorkspace(ctx, ws))
	tokens := map[string]string{}
	for _, m := range []struct {
		user *platauth.User
		role platauth.Role
	}{
		{&platauth.User{ID: "u-alice", Email: "alice@example.com", Name: "Alice"}, platauth.RoleOwner},
		{&platauth.User{ID: "u-bob", Email: "bob@example.com", Name: "Bob"}, platauth.RoleOwner},
	} {
		require.NoError(t, srv.AuthStore.CreateUser(ctx, m.user))
		require.NoError(t, srv.AuthStore.AddMember(ctx, ws.ID, m.user.ID, m.role))
		token, err := platauth.GenerateToken(m.user, cfg.JWTSecret, time.Hour)
		require.NoError(t, err)
		tokens[m.user.Name] = token
	}

	ts := httptest.NewServer(srv.GetEcho())
	t.Cleanup(ts.Close)
	return liveServer{url: ts.URL, ws: ws.Slug, tokens: tokens}
}

// client is the editor client of one member.
func (s liveServer) client(name string) *editorclient.EditorClient {
	return editorclient.New(s.url, s.tokens[name])
}

// apply sends ops to the server as one member and returns the result.
func (s liveServer) apply(t *testing.T, name, projectID string, ops ...op) *change.Result {
	t.Helper()
	res, err := s.client(name).ApplyChanges(t.Context(), s.ws, projectID, editorStream, []byte(changeSet(t, ops...)))
	require.NoError(t, err)
	return res
}

// connectedApp is a desktop app signed in to the server as one member.
func (s liveServer) connectedApp(t *testing.T, name string) *App {
	t.Helper()
	app := newTestApp(t)
	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = s.url
	app.activeWS = s.ws
	app.remoteHTTP = s.client(name)
	app.authInfo = &config.StoredAuth{ServerURL: s.url, AccessToken: s.tokens[name]}
	app.mu.Unlock()
	return app
}

// An edit made offline replays with the revision it was made against. A
// translation someone changed on the server while the app was offline keeps
// their wording: the server refuses the queued edit stale, and the outbox marks
// it failed. An offline edit to a translation nobody touched lands.
func TestOfflineEditToATranslationChangedOnTheServerIsRefusedOnReplay(t *testing.T) {
	live := startLiveServer(t)
	ctx := t.Context()
	alice := live.client("Alice")

	var proj struct {
		ID string `json:"id"`
	}
	require.NoError(t, alice.DoJSON(ctx, http.MethodPost, "/api/v1/"+live.ws+"/projects", nil, map[string]any{
		"name": "Outbox", "default_source_language": "en", "target_languages": []string{"fr"},
	}, &proj))
	_, err := alice.UploadItems(ctx, live.ws, proj.ID, map[string][]byte{"hello.txt": []byte("Hello world\n\nGood night\n")})
	require.NoError(t, err)

	app := live.connectedApp(t, "Alice")
	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q

	// Alice opens the item while connected: the blocks come from the server,
	// each translation's revision with them, and the cache keeps a copy.
	blocks, err := app.GetItemBlocks(proj.ID, "hello.txt")
	require.NoError(t, err)
	require.Len(t, blocks, 2)
	hello, night := blocks[0], blocks[1]
	for _, b := range blocks {
		require.Equal(t, "absent", b.TargetRevisions["fr"])
	}
	res := live.apply(t, "Alice", proj.ID,
		setTarget("hello.txt", hello.ID, "fr", "absent", "Bonjour le monde"),
		setTarget("hello.txt", night.ID, "fr", "absent", "Bonne nuit"))
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	blocks, err = app.GetItemBlocks(proj.ID, "hello.txt")
	require.NoError(t, err)
	helloRead, nightRead := blocks[0].TargetRevisions["fr"], blocks[1].TargetRevisions["fr"]

	// The connection drops. Alice edits both translations; each edit applies
	// to the cache and queues with the revision she read.
	app.mu.Lock()
	app.connState = StateOffline
	app.mu.Unlock()
	for _, edit := range []op{
		setTarget("hello.txt", hello.ID, "fr", helloRead, "Salut tout le monde"),
		setTarget("hello.txt", night.ID, "fr", nightRead, "Bonne nuit à tous"),
	} {
		out := applyChanges(t, app, proj.ID, edit)
		require.Equal(t, change.SetApplied, out.Status)
	}
	require.Equal(t, 2, q.PendingCount())

	// Meanwhile Bob rewrites the first translation on the server.
	res = live.apply(t, "Bob", proj.ID, setTarget("hello.txt", hello.ID, "fr", helloRead, "Bonjour à tous"))
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)

	// The connection returns and the outbox replays.
	app.mu.Lock()
	app.connState = StateConnected
	app.mu.Unlock()
	app.replayPendingChanges(ctx)

	assert.Equal(t, 0, q.PendingCount(), "the outbox drained")
	assert.Equal(t, 1, q.FailedCount(), "the stale edit is refused and kept as failed")
	var lastError string
	require.NoError(t, q.db.QueryRow(`SELECT last_error FROM pending_changes WHERE status = 'failed'`).Scan(&lastError))
	assert.Contains(t, lastError, "HTTP 409", "the server refused it stale")

	served, err := alice.GetEditorBlocks(ctx, live.ws, proj.ID, "hello.txt")
	require.NoError(t, err)
	require.Len(t, served, 2)
	assert.Equal(t, "Bonjour à tous", served[0].Targets["fr"].Text, "Bob's wording stands")
	assert.Equal(t, "Bonne nuit à tous", served[1].Targets["fr"].Text, "the edit nobody raced landed")
}

// A translation saved while connected reaches the server as a change set with
// the revision the editor read, and the cache follows the server.
func TestApplyChangesConnectedSavesOnTheServer(t *testing.T) {
	live := startLiveServer(t)
	ctx := t.Context()
	alice := live.client("Alice")
	var proj struct {
		ID string `json:"id"`
	}
	require.NoError(t, alice.DoJSON(ctx, http.MethodPost, "/api/v1/"+live.ws+"/projects", nil, map[string]any{
		"name": "Online", "default_source_language": "en", "target_languages": []string{"fr"},
	}, &proj))
	_, err := alice.UploadItems(ctx, live.ws, proj.ID, map[string][]byte{"hello.txt": []byte("Hello world\n")})
	require.NoError(t, err)

	app := live.connectedApp(t, "Alice")
	blocks, err := app.GetItemBlocks(proj.ID, "hello.txt")
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	read := blocks[0].TargetRevisions["fr"]

	res := applyChanges(t, app, proj.ID, setTarget("hello.txt", blocks[0].ID, "fr", read, "Bonjour le monde"))
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops)
	require.NotNil(t, res.Record, "the server recorded the change")

	// A second save on the revision the first one read is stale, and the
	// result carries the wording that stands.
	res = applyChanges(t, app, proj.ID, setTarget("hello.txt", blocks[0].ID, "fr", read, "Salut"))
	require.Equal(t, change.SetRefused, res.Status)
	require.NotNil(t, res.Ops[0].Current)
	assert.Equal(t, "Bonjour le monde", res.Ops[0].Current.Text)

	// The cache followed the server: it holds the translation as saved, at the
	// revision the server holds.
	cached, err := app.store.GetBlock(ctx, proj.ID, editorStream, blocks[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "Bonjour le monde", cached.Block.TargetText("fr"))
	assert.Equal(t, res.Ops[0].Current.Rev, model.EditionRevision(cached.Block, model.EditionKey{Locale: "fr"}))
}
