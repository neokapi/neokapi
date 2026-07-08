package backend

import (
	"testing"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/bowrain/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unreachableURL points at a port nothing listens on, so the /auth/me lookup
// inside GetCollabSession fails fast (connection refused) and the cached-auth
// fallback path is exercised deterministically.
const unreachableURL = "http://127.0.0.1:1"

func TestGetCollabSessionNotConnected(t *testing.T) {
	app := newTestApp(t)

	_, err := app.GetCollabSession()
	require.Error(t, err)
	assert.ErrorIs(t, err, errNotConnected)
}

func TestGetCollabSessionConnectedFallsBackToCachedUser(t *testing.T) {
	app := newTestApp(t)

	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = unreachableURL
	app.activeWS = "acme"
	app.remoteHTTP = apiclient.NewEditorClient(unreachableURL, "tok-abc")
	app.authInfo = &config.StoredAuth{
		ServerURL:   unreachableURL,
		AccessToken: "tok-abc",
		User: config.StoredUser{
			ID:    "user-42",
			Email: "alice@acme.test",
			Name:  "Alice",
		},
	}
	app.mu.Unlock()

	sess, err := app.GetCollabSession()
	require.NoError(t, err)
	assert.Equal(t, unreachableURL, sess.ServerURL)
	assert.Equal(t, "tok-abc", sess.AuthToken)
	assert.Equal(t, "acme", sess.Workspace)
	assert.Equal(t, "user-42", sess.User.UserID)
	assert.Equal(t, "Alice", sess.User.Name)
}

func TestGetCollabSessionConnectedNoTokenErrors(t *testing.T) {
	app := newTestApp(t)

	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = unreachableURL
	app.activeWS = "acme"
	app.remoteHTTP = apiclient.NewEditorClient(unreachableURL, "")
	app.authInfo = &config.StoredAuth{ServerURL: unreachableURL} // no AccessToken
	app.mu.Unlock()

	_, err := app.GetCollabSession()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no auth token")
}

func TestGetCollabSessionUsesEmailWhenNameEmpty(t *testing.T) {
	app := newTestApp(t)

	app.mu.Lock()
	app.connState = StateConnected
	app.serverURL = unreachableURL
	app.activeWS = "acme"
	app.remoteHTTP = apiclient.NewEditorClient(unreachableURL, "tok")
	app.authInfo = &config.StoredAuth{
		ServerURL:   unreachableURL,
		AccessToken: "tok",
		User:        config.StoredUser{ID: "u1", Email: "bob@acme.test"}, // no Name
	}
	app.mu.Unlock()

	sess, err := app.GetCollabSession()
	require.NoError(t, err)
	assert.Equal(t, "bob@acme.test", sess.User.Name)
}
