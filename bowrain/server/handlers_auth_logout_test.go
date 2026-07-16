package server

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposeEndSessionURL(t *testing.T) {
	t.Run("cognito uses client_id + logout_uri (not the OIDC pair)", func(t *testing.T) {
		got := composeEndSessionURL(
			"https://d.auth.eu-north-1.amazoncognito.com/logout",
			AuthProviderCognito, "client-123", "https://app.bowrain.cloud/", "ignored-id-token",
		)
		u, err := url.Parse(got)
		require.NoError(t, err)
		q := u.Query()
		assert.Equal(t, "/logout", u.Path)
		assert.Equal(t, "client-123", q.Get("client_id"))
		assert.Equal(t, "https://app.bowrain.cloud/", q.Get("logout_uri"))
		// Cognito must NOT receive the OIDC params (it 400s → /login).
		assert.Empty(t, q.Get("post_logout_redirect_uri"))
		assert.Empty(t, q.Get("id_token_hint"))
	})

	t.Run("keycloak uses the standard OIDC params", func(t *testing.T) {
		got := composeEndSessionURL(
			"https://kc/realms/bowrain/protocol/openid-connect/logout",
			AuthProviderKeycloak, "client-123", "https://app.example.com/", "raw-id-token",
		)
		u, err := url.Parse(got)
		require.NoError(t, err)
		q := u.Query()
		assert.Equal(t, "https://app.example.com/", q.Get("post_logout_redirect_uri"))
		assert.Equal(t, "raw-id-token", q.Get("id_token_hint"))
		assert.Equal(t, "client-123", q.Get("client_id"))
		assert.Empty(t, q.Get("logout_uri"))
	})

	t.Run("bad endpoint yields empty", func(t *testing.T) {
		assert.Empty(t, composeEndSessionURL("://bad", AuthProviderCognito, "c", "/", ""))
	})
}
