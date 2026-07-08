package backend

import (
	"context"
	"errors"
	"time"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
)

// CollabUser identifies the current user for a collaboration session.
// The shape mirrors the @neokapi/ui useCollaboration `user` option.
type CollabUser struct {
	UserID    string `json:"userId"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl,omitempty"`
}

// CollabSession carries everything the frontend needs to open the Yjs
// presence WebSocket directly from the webview, exactly like the web app.
//
// The desktop frontend talks to the Go backend over Wails bindings (not REST),
// and the auth token lives in the OS keychain — never exposed to the frontend.
// Presence is the one case where the webview must talk to the server directly
// (the Yjs y-websocket provider runs in the browser), so we surface the token
// here. This is the same trust boundary as the web app: the webview *is* the
// app, the token already authenticated this process, and it travels only to the
// same server the user explicitly connected to.
type CollabSession struct {
	ServerURL string     `json:"serverUrl"`
	AuthToken string     `json:"authToken"`
	Workspace string     `json:"workspace"`
	User      CollabUser `json:"user"`
}

// GetCollabSession returns the presence-collaboration session info for the
// currently connected server + workspace. It surfaces the keychain auth token,
// the HTTP server URL, the active workspace slug, and the current user so the
// frontend can open the Yjs awareness WebSocket (params.token) just like the
// web translate view.
//
// Returns an error when not connected to a server — presence is a server
// feature, so there is nothing to join in local/standalone mode.
func (a *App) GetCollabSession() (CollabSession, error) {
	if !a.isConnected() {
		return CollabSession{}, errNotConnected
	}

	a.mu.RLock()
	serverURL := a.serverURL
	workspace := a.activeWS
	auth := a.authInfo
	a.mu.RUnlock()

	if auth == nil || auth.AccessToken == "" {
		return CollabSession{}, errors.New("no auth token available for collaboration")
	}

	user := CollabUser{
		UserID: auth.User.ID,
		Name:   auth.User.Name,
	}
	if user.Name == "" {
		user.Name = auth.User.Email
	}

	// Prefer the server's authoritative user record (it carries the avatar URL
	// and a stable ID, which the cached desktop auth metadata may lack). Fall
	// back silently to the cached values on any error so presence still works
	// offline-of-identity-service. This uses the shared REST /auth/me client.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if u, err := apiclient.FetchUser(ctx, serverURL, auth.AccessToken); err == nil && u != nil {
		if u.ID != "" {
			user.UserID = u.ID
		}
		if u.Name != "" {
			user.Name = u.Name
		} else if u.Email != "" {
			user.Name = u.Email
		}
		user.AvatarURL = u.AvatarURL
	}

	return CollabSession{
		ServerURL: serverURL,
		AuthToken: auth.AccessToken,
		Workspace: workspace,
		User:      user,
	}, nil
}
