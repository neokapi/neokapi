package server

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestServerWithMaxConns is newTestServer over a database with an explicit
// connection ceiling, so a request burst can be made to exceed the pool. It
// returns the database too, so a test can read what the pool is doing.
func newTestServerWithMaxConns(t *testing.T, maxConns int32) (*Server, *storage.PgDB, string) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	s := shutdownOnCleanup(t, NewServer(cfg))
	db := pgtest.NewTestDBWithMaxConns(t, maxConns)
	initTestStoresOnDB(t, s, db)

	require.NotNil(t, s.AuthStore, "auth store should be initialized")

	ctx := t.Context()
	user := &platauth.User{ID: "test-user", Email: "test@example.com", Name: "Test"}
	require.NoError(t, s.AuthStore.CreateUser(ctx, user))
	ws := &platauth.Workspace{ID: "test-ws", Name: "Test", Slug: "test", Type: platauth.WorkspaceTypePersonal}
	require.NoError(t, s.AuthStore.CreateWorkspace(ctx, ws))
	require.NoError(t, s.AuthStore.AddMember(ctx, ws.ID, user.ID, platauth.RoleOwner))

	token, err := platauth.GenerateToken(user, cfg.JWTSecret, 24*time.Hour)
	require.NoError(t, err)
	return s, db, token
}

// TestWorkspaceEventsSSEDoesNotBlockTheSession asserts that the page-load burst
// the web shell issues alongside the event stream is answered while the stream
// is still live.
//
// The burst is deliberately larger than the workspace's connection pool, which
// is the condition every page load meets: five requests leave the browser in
// the same instant and the pool has fewer connections than that. What the test
// pins down is that the surplus waits somewhere a returned connection wakes it.
// A wait that nothing wakes does not fail the request — it holds it for as long
// as the browser keeps the page open, and answers 503 only when the navigation
// that abandons the stream cancels it too, all four in the same millisecond,
// while requests issued later are served in milliseconds throughout.
func TestWorkspaceEventsSSEDoesNotBlockTheSession(t *testing.T) {
	srv, db, token := newTestServerWithMaxConns(t, 2)
	ts := httptest.NewServer(srv.GetEcho())
	defer ts.Close()

	streamCtx, closeStream := context.WithCancel(t.Context())
	defer closeStream()

	// Registered after the two above, so it runs before them: a burst request
	// still in flight would otherwise hold the test server open past the
	// assertion that reported it, turning a clear failure into a timeout.
	burstCtx, abandonBurst := context.WithCancel(t.Context())
	defer abandonBurst()

	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, ts.URL+"/api/v1/test/events", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	stream, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer stream.Body.Close()
	require.Equal(t, http.StatusOK, stream.StatusCode)

	// The stream is live once its opening comment has been flushed.
	opening, err := bufio.NewReader(stream.Body).ReadString('\n')
	require.NoError(t, err)
	require.Contains(t, opening, "connected")

	// The queries the web shell issues alongside the stream, in the same instant.
	tests := []struct {
		name string
		path string
	}{
		{"my open tasks", "/api/v1/test/tasks?assignee_id=me&status=open"},
		{"task counts", "/api/v1/test/tasks/counts"},
		{"activity feed", "/api/v1/test/activities?limit=20"},
		{"projects", "/api/v1/test/projects"},
	}

	type outcome struct {
		name    string
		status  int
		elapsed time.Duration
		err     error
	}
	results := make(chan outcome, len(tests))
	release := make(chan struct{})
	for _, tt := range tests {
		go func() {
			<-release
			started := time.Now()
			r, err := http.NewRequestWithContext(burstCtx, http.MethodGet, ts.URL+tt.path, nil)
			if err != nil {
				results <- outcome{name: tt.name, err: err}
				return
			}
			r.Header.Set("Authorization", "Bearer "+token)
			resp, err := http.DefaultClient.Do(r)
			if err != nil {
				results <- outcome{name: tt.name, elapsed: time.Since(started), err: err}
				return
			}
			resp.Body.Close()
			results <- outcome{name: tt.name, status: resp.StatusCode, elapsed: time.Since(started)}
		}()
	}
	close(release)

	// Generous next to the ~10ms these queries take, and far below the minutes
	// the stream would otherwise hold them for.
	const bound = 15 * time.Second
	deadline := time.After(bound)
	for range tests {
		select {
		case got := <-results:
			assert.NoError(t, got.err, "%s failed while the event stream was open", got.name)
			assert.Equal(t, http.StatusOK, got.status, "%s answered %d while the event stream was open", got.name, got.status)
			t.Logf("%s: %d in %v", got.name, got.status, got.elapsed.Round(time.Millisecond))
		case <-deadline:
			t.Fatalf("a page-load query did not complete within %v while the event stream was open", bound)
		}
	}

	// The stream was live throughout, not closed early by the burst.
	assert.Equal(t, 1, srv.changeRelay.ClientCount())

	// And a live stream is not a held connection. The handler resolves the
	// workspace and its membership at connect and then streams from the relay,
	// so once the opening comment is out it owns nothing the next request needs.
	assert.Zero(t, db.Stats().InUse,
		"the event stream must hold no database connection while it streams")
}
