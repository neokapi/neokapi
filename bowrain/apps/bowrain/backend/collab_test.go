package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCollabSessionNotConnected(t *testing.T) {
	app := newTestApp(t)

	_, err := app.GetCollabSession()
	require.Error(t, err)
	assert.ErrorIs(t, err, errNotConnected)
}

func TestGetCollabSessionConnectedFallsBackToCachedUser(t *testing.T) {
	app := newTestApp(t)

	// A grpc.NewClient client connects lazily, so it never dials here; the
	// GetCurrentUser RPC inside GetCollabSession will fail (no server), which
	// exercises the cached-auth fallback path deterministically.
	client, err := NewServerClient("127.0.0.1:1", "tok-abc", false)
	require.NoError(t, err)

	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = "http://localhost:8080"
	app.activeWS = "acme"
	app.remote = client
	app.authInfo = &storedDesktopAuth{
		ServerURL:   "http://localhost:8080",
		AccessToken: "tok-abc",
		User: storedDesktopUser{
			ID:    "user-42",
			Email: "alice@acme.test",
			Name:  "Alice",
		},
	}
	app.mu.Unlock()

	sess, err := app.GetCollabSession()
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8080", sess.ServerURL)
	assert.Equal(t, "tok-abc", sess.AuthToken)
	assert.Equal(t, "acme", sess.Workspace)
	assert.Equal(t, "user-42", sess.User.UserID)
	assert.Equal(t, "Alice", sess.User.Name)
}

func TestGetCollabSessionConnectedNoTokenErrors(t *testing.T) {
	app := newTestApp(t)

	client, err := NewServerClient("127.0.0.1:1", "", false)
	require.NoError(t, err)

	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = "http://localhost:8080"
	app.activeWS = "acme"
	app.remote = client
	app.authInfo = &storedDesktopAuth{ServerURL: "http://localhost:8080"} // no AccessToken
	app.mu.Unlock()

	_, err = app.GetCollabSession()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no auth token")
}

func TestGetCollabSessionUsesEmailWhenNameEmpty(t *testing.T) {
	app := newTestApp(t)

	client, err := NewServerClient("127.0.0.1:1", "tok", false)
	require.NoError(t, err)

	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = "http://localhost:8080"
	app.activeWS = "acme"
	app.remote = client
	app.authInfo = &storedDesktopAuth{
		ServerURL:   "http://localhost:8080",
		AccessToken: "tok",
		User:        storedDesktopUser{ID: "u1", Email: "bob@acme.test"}, // no Name
	}
	app.mu.Unlock()

	sess, err := app.GetCollabSession()
	require.NoError(t, err)
	assert.Equal(t, "bob@acme.test", sess.User.Name)
}
