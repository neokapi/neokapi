package backend

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/editorclient"
	"github.com/neokapi/neokapi/host/venue/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// --- Backoff arithmetic ---

func TestNextBackoff(t *testing.T) {
	tests := []struct {
		name  string
		cur   time.Duration
		limit time.Duration
		want  time.Duration
	}{
		{"doubles below the cap", 2 * time.Second, 60 * time.Second, 4 * time.Second},
		{"stops at the cap", 40 * time.Second, 60 * time.Second, 60 * time.Second},
		{"stays at the cap", 60 * time.Second, 60 * time.Second, 60 * time.Second},
		{"overflow lands on the cap", 1<<62 - 1, 60 * time.Second, 60 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, nextBackoff(tt.cur, tt.limit))
		})
	}
}

func TestJitteredStaysInTheUpperHalf(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		frac float64
		want time.Duration
	}{
		{"floor at half", 8 * time.Second, 0, 4 * time.Second},
		{"midpoint", 8 * time.Second, 0.5, 6 * time.Second},
		{"ceiling just under the full wait", 8 * time.Second, 1, 8 * time.Second},
		{"zero stays zero", 0, 0.5, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, jittered(tt.d, tt.frac))
		})
	}
}

func TestStreamBackoffAfter(t *testing.T) {
	// A stream that lived long enough to have carried events resets the wait;
	// one that died on arrival keeps growing it.
	assert.Equal(t, streamInitialBackoff, streamBackoffAfter(streamMaxBackoff, streamHealthyFor))
	assert.Equal(t, streamInitialBackoff, streamBackoffAfter(streamMaxBackoff, time.Hour))
	assert.Equal(t, streamMaxBackoff, streamBackoffAfter(streamMaxBackoff, time.Millisecond))
}

// --- Connecting is a round trip, not an assumption ---

// authOnlyServer answers the reachability probe and counts how many arrived.
func authOnlyServer(t *testing.T, status int) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var probes atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/me" {
			probes.Add(1)
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"id":"u1","email":"a@example.com","name":"A"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv, &probes
}

func TestConnectToServerProbesTheServer(t *testing.T) {
	srv, probes := authOnlyServer(t, http.StatusOK)

	app := newTestApp(t)
	app.mu.Lock()
	app.authInfo = &config.StoredAuth{ServerURL: srv.URL, AccessToken: "tok"}
	app.mu.Unlock()

	require.NoError(t, app.ConnectToServer(srv.URL))
	assert.Equal(t, StateConnected, app.GetConnectionState().State)
	assert.Equal(t, int64(1), probes.Load(), "the connection is confirmed with a round trip")
}

func TestConnectToServerFailsWhenTheServerIsUnreachable(t *testing.T) {
	srv, _ := authOnlyServer(t, http.StatusOK)
	serverURL := srv.URL
	srv.Close()

	app := newTestApp(t)
	app.mu.Lock()
	app.authInfo = &config.StoredAuth{ServerURL: serverURL, AccessToken: "tok"}
	app.connState = StateOffline
	app.mu.Unlock()

	err := app.ConnectToServer(serverURL)
	require.Error(t, err)
	require.NotErrorIs(t, err, errAuthRequired, "an unreachable server is worth retrying")
	assert.Equal(t, StateOffline, app.GetConnectionState().State,
		"a failed attempt leaves an offline working copy offline, not signed out")
}

func TestConnectToServerReportsARejectedSessionAsTerminal(t *testing.T) {
	srv, _ := authOnlyServer(t, http.StatusUnauthorized)

	app := newTestApp(t)
	app.mu.Lock()
	app.authInfo = &config.StoredAuth{ServerURL: srv.URL, AccessToken: "stale"}
	app.connState = StateOffline
	app.mu.Unlock()

	err := app.ConnectToServer(srv.URL)
	require.ErrorIs(t, err, errAuthRequired)
	assert.Equal(t, StateOffline, app.GetConnectionState().State)
}

// TestConnectToServerUsesTheInMemoryToken is the #2596 regression. A session
// established from BOWRAIN_TOKEN holds its credentials only in memory, and a
// reconnect that read the keychain alone found nothing for the server it was
// connected to and failed every attempt, silently, for the life of the app.
func TestConnectToServerUsesTheInMemoryToken(t *testing.T) {
	srv, probes := authOnlyServer(t, http.StatusOK)
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir()) // an empty keychain-backed store
	t.Setenv("BOWRAIN_SERVER_URL", srv.URL)
	t.Setenv("BOWRAIN_TOKEN", "ci-token")

	app := newTestApp(t)
	require.Equal(t, StateConnected, app.GetConnectionState().State, "auto-connect via BOWRAIN_TOKEN")

	// Lose the connection, then reconnect on the same credentials.
	app.mu.Lock()
	app.connState = StateOffline
	app.mu.Unlock()

	require.NoError(t, app.tryReconnect(context.Background()))
	assert.Equal(t, StateConnected, app.GetConnectionState().State)
	assert.Positive(t, probes.Load())
}

func TestCredentialsForRejectsAnotherServersToken(t *testing.T) {
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())

	app := newTestApp(t)
	app.mu.Lock()
	app.authInfo = &config.StoredAuth{ServerURL: "https://other.example", AccessToken: "tok"}
	app.mu.Unlock()

	_, err := app.credentialsFor("https://bowrain.example")
	require.ErrorIs(t, err, errAuthRequired)
}

func TestCredentialsForKeepsAnExpiredTokenThatCanRefresh(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("BOWRAIN_CONFIG_DIR", tmpDir)
	require.NoError(t, config.SaveAuth(config.StoredAuth{
		ServerURL:    "https://bowrain.example",
		AccessToken:  "expired",
		RefreshToken: "refresh-me",
		Expiry:       time.Now().Add(-time.Hour),
	}))

	app := newTestApp(t)
	creds, err := app.credentialsFor("https://bowrain.example")
	require.NoError(t, err)
	assert.Equal(t, "refresh-me", creds.RefreshToken,
		"the transport spends the refresh on the first 401; an outage must not force a re-login")
}

func TestCredentialsForRejectsAnExpiredTokenWithNothingToRefreshIt(t *testing.T) {
	// A fresh keychain: SaveAuth leaves an existing refresh entry for a server
	// alone, and the mock keychain is shared by the whole package.
	keyring.MockInit()
	tmpDir := t.TempDir()
	t.Setenv("BOWRAIN_CONFIG_DIR", tmpDir)
	require.NoError(t, config.SaveAuth(config.StoredAuth{
		ServerURL:   "https://no-refresh.example",
		AccessToken: "expired",
		Expiry:      time.Now().Add(-time.Hour),
	}))

	app := newTestApp(t)
	_, err := app.credentialsFor("https://no-refresh.example")
	require.ErrorIs(t, err, errAuthRequired)
}

// --- The reconnect loop ---

func TestReconnectLoopStopsWhenOnlyASignInCanHelp(t *testing.T) {
	srv, _ := authOnlyServer(t, http.StatusUnauthorized)
	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())

	app := newTestApp(t)
	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = srv.URL
	app.authInfo = &config.StoredAuth{ServerURL: srv.URL, AccessToken: "stale"}
	app.mu.Unlock()

	app.goOffline()

	require.Eventually(t, func() bool {
		return app.GetConnectionState().State == StateDisconnected
	}, 15*time.Second, 50*time.Millisecond, "a rejected session ends at the sign-in screen, not in a retry loop")
}

func TestRetryConnectionAttemptsWithoutWaitingOutTheBackoff(t *testing.T) {
	srv, probes := authOnlyServer(t, http.StatusOK)

	app := newTestApp(t)
	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = srv.URL
	app.authInfo = &config.StoredAuth{ServerURL: srv.URL, AccessToken: "tok"}
	app.mu.Unlock()

	app.goOffline()
	require.Equal(t, StateOffline, app.RetryConnection().State)

	require.Eventually(t, func() bool {
		return app.GetConnectionState().State == StateConnected
	}, 10*time.Second, 20*time.Millisecond)
	assert.Positive(t, probes.Load())
}

func TestRetryConnectionStartsALoopThatIsNotRunning(t *testing.T) {
	srv, _ := authOnlyServer(t, http.StatusOK)

	// An app that launched into offline mode never called goOffline, so there
	// is no loop for a nudge to reach.
	app := newTestApp(t)
	app.mu.Lock()
	app.connState = StateOffline
	app.serverURL = srv.URL
	app.authInfo = &config.StoredAuth{ServerURL: srv.URL, AccessToken: "tok"}
	app.mu.Unlock()

	app.RetryConnection()

	require.Eventually(t, func() bool {
		return app.GetConnectionState().State == StateConnected
	}, 10*time.Second, 20*time.Millisecond)
}

func TestNudgeReconnectWithNoLoopIsANoOp(t *testing.T) {
	app := newTestApp(t)
	app.nudgeReconnect() // must not panic or block
	assert.Equal(t, StateDisconnected, app.GetConnectionState().State)
}

// --- Subscriptions come back with the connection ---

func TestUpdatePresenceRemembersTheFocus(t *testing.T) {
	app := newTestApp(t)
	app.UpdatePresence("p1", "hello.txt", "b1")

	app.mu.RLock()
	defer app.mu.RUnlock()
	assert.Equal(t, presenceFocus{ProjectID: "p1", ItemName: "hello.txt", BlockID: "b1"}, app.presence)
}

func TestStopWatchingForgetsTheProject(t *testing.T) {
	app := newTestApp(t)
	app.StartWatching("p1")
	app.mu.RLock()
	watched := app.watchedProject
	app.mu.RUnlock()
	require.Equal(t, "p1", watched)

	app.StopWatching()
	app.mu.RLock()
	defer app.mu.RUnlock()
	assert.Empty(t, app.watchedProject, "navigating away must not be undone by a later reconnect")
}

func TestGoOfflineKeepsTheProjectForResubscription(t *testing.T) {
	app := newTestApp(t)
	app.mu.Lock()
	app.connState = StateConnected
	app.mu.Unlock()
	app.StartWatching("p1")

	app.goOffline()

	app.mu.RLock()
	defer app.mu.RUnlock()
	assert.Equal(t, "p1", app.watchedProject)
	assert.Nil(t, app.watcher, "the stream died with the connection")
}

// --- End to end: the network goes away and comes back ---

// tcpRelay forwards loopback connections to a target address and can be cut and
// restored on the same port. It is the shape of the recording harness's own
// relay (harness/src/driver/record-desktop.ts), which is how #2596 was found:
// the desktop backend talks to the relay, and cutting it is a real outage
// rather than a mocked one.
type tcpRelay struct {
	addr   string
	target string

	mu    sync.Mutex
	ln    net.Listener
	conns []net.Conn
}

func newTCPRelay(t *testing.T, targetAddr string) *tcpRelay {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	r := &tcpRelay{addr: ln.Addr().String(), target: targetAddr, ln: ln}
	go r.serve(ln)
	t.Cleanup(r.cut)
	return r
}

// URL is the base URL callers should use in place of the real server's.
func (r *tcpRelay) URL() string { return "http://" + r.addr }

func (r *tcpRelay) serve(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		r.track(conn)
		go r.pipe(conn)
	}
}

func (r *tcpRelay) pipe(client net.Conn) {
	upstream, err := net.Dial("tcp", r.target)
	if err != nil {
		client.Close()
		return
	}
	r.track(upstream)
	go func() {
		_, _ = io.Copy(upstream, client)
		upstream.Close()
	}()
	_, _ = io.Copy(client, upstream)
	client.Close()
}

func (r *tcpRelay) track(c net.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conns = append(r.conns, c)
}

// cut stops accepting and destroys every live socket, so an in-flight request
// fails the way a severed network makes it fail.
func (r *tcpRelay) cut() {
	r.mu.Lock()
	ln, conns := r.ln, r.conns
	r.ln, r.conns = nil, nil
	r.mu.Unlock()

	if ln != nil {
		ln.Close()
	}
	for _, c := range conns {
		c.Close()
	}
}

// restore listens again on the same address. Nothing tells the client.
func (r *tcpRelay) restore(t *testing.T) {
	t.Helper()
	ln, err := net.Listen("tcp", r.addr)
	require.NoError(t, err)

	r.mu.Lock()
	r.ln = ln
	r.mu.Unlock()
	go r.serve(ln)
}

// TestReconnectAfterTheRelayDropsAndReturns is #2596 end to end: a connection
// cut mid-session, edits queued behind it, and the connection back a few
// seconds later with nobody to tell the client. The backend has to notice on
// its own, replay what it queued, and put the project subscription back.
func TestReconnectAfterTheRelayDropsAndReturns(t *testing.T) {
	var replays atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/me":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"u1","email":"a@example.com","name":"A"}`))
		case "/api/v1/acme/events":
			// The restored change-event subscription. Hold it open until the
			// watcher's context ends, the way the server's SSE relay does.
			<-r.Context().Done()
		default:
			replays.Add(1)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	target, ok := srv.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	relay := newTCPRelay(t, target.String())

	t.Setenv("BOWRAIN_CONFIG_DIR", t.TempDir())
	t.Setenv("BOWRAIN_SERVER_URL", relay.URL())
	t.Setenv("BOWRAIN_TOKEN", "ci-token")

	app := newTestApp(t)
	require.Equal(t, StateConnected, app.GetConnectionState().State)

	q := newTestQueue(t)
	if app.offlineQueue != nil {
		app.offlineQueue.Close()
	}
	app.offlineQueue = q
	app.mu.Lock()
	app.activeWS = "acme"
	app.watchedProject = "p1"
	app.mu.Unlock()

	// The outage. Two edits land in the outbox behind it.
	relay.cut()
	app.goOffline()
	require.Equal(t, StateOffline, app.GetConnectionState().State)
	for _, text := range []string{"Bonjour", "Bonsoir"} {
		app.enqueue(updateBlockTargetOp{UpdateBlockRequest{
			ProjectID: "p1", ItemName: "hello.txt", BlockID: "b1",
			TargetLocale: "fr", Text: text,
		}})
	}
	require.Equal(t, 2, q.PendingCount())

	// Attempts keep failing while the relay is down.
	require.Never(t, func() bool {
		return app.GetConnectionState().State == StateConnected
	}, 3*time.Second, 100*time.Millisecond)

	relay.restore(t)

	require.Eventually(t, func() bool {
		return app.GetConnectionState().State == StateConnected && q.PendingCount() == 0
	}, 30*time.Second, 100*time.Millisecond,
		"the connection came back on its own and the queue drained behind it")
	assert.Equal(t, int64(2), replays.Load(), "both queued edits reached the server")

	// The project subscription is back on the new client.
	require.Eventually(t, func() bool {
		app.mu.RLock()
		defer app.mu.RUnlock()
		return app.watcher != nil
	}, 5*time.Second, 50*time.Millisecond)

	// Release the held SSE request so the deferred srv.Close can return.
	app.StopWatching()
}

// TestReconnectRestoresTheStreamAndPresence covers resubscribe on its own: the
// watcher opens against the client the reconnect installed, and this user's
// editing focus is reported again so other watchers stop seeing them as gone.
func TestReconnectRestoresTheStreamAndPresence(t *testing.T) {
	var presenceReports atomic.Int64
	streams := make(chan struct{}, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/auth/me":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"u1"}`))
		case r.URL.Path == "/api/v1/acme/events":
			select {
			case streams <- struct{}{}:
			default:
			}
			<-r.Context().Done()
		default:
			presenceReports.Add(1)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	app := newTestApp(t)
	app.mu.Lock()
	app.connState = StateOffline
	app.serverURL = srv.URL
	app.activeWS = "acme"
	app.authInfo = &config.StoredAuth{ServerURL: srv.URL, AccessToken: "tok"}
	app.watchedProject = "p1"
	app.presence = presenceFocus{ProjectID: "p1", ItemName: "hello.txt", BlockID: "b1"}
	app.mu.Unlock()

	require.NoError(t, app.tryReconnect(context.Background()))
	app.resubscribe()

	select {
	case <-streams:
	case <-time.After(10 * time.Second):
		t.Fatal("the change-event subscription was not reopened")
	}
	assert.Equal(t, int64(1), presenceReports.Load())

	app.StopWatching()
}

// TestCheckStillReachableGoesOfflineWhenTheProbeFails covers the watcher's own
// outage detection: a dropped stream is only a symptom, and the probe is what
// decides whether it meant the server is gone.
func TestCheckStillReachableGoesOfflineWhenTheProbeFails(t *testing.T) {
	srv, _ := authOnlyServer(t, http.StatusOK)
	serverURL := srv.URL
	srv.Close()

	app := newTestApp(t)
	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = serverURL
	app.authInfo = &config.StoredAuth{ServerURL: serverURL, AccessToken: "tok"}
	app.remoteHTTP = editorclient.New(serverURL, "tok")
	app.mu.Unlock()

	app.checkStillReachable(context.Background())
	assert.Equal(t, StateOffline, app.GetConnectionState().State)
	app.stopReconnect()
}

func TestCheckStillReachableStaysConnectedWhenTheServerAnswers(t *testing.T) {
	srv, probes := authOnlyServer(t, http.StatusOK)

	app := newTestApp(t)
	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = srv.URL
	app.authInfo = &config.StoredAuth{ServerURL: srv.URL, AccessToken: "tok"}
	app.remoteHTTP = editorclient.New(srv.URL, "tok")
	app.mu.Unlock()

	app.checkStillReachable(context.Background())
	assert.Equal(t, StateConnected, app.GetConnectionState().State)
	assert.Equal(t, int64(1), probes.Load())
}
