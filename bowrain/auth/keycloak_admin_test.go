package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeycloakAdminConfig_Validate(t *testing.T) {
	full := KeycloakAdminConfig{
		BaseURL:      "https://auth.example.com",
		Realm:        "bowrain",
		ClientID:     "admin-svc",
		ClientSecret: "secret",
	}
	tests := []struct {
		name    string
		mutate  func(c *KeycloakAdminConfig)
		wantErr string
	}{
		{"valid", func(c *KeycloakAdminConfig) {}, ""},
		{"missing-base-url", func(c *KeycloakAdminConfig) { c.BaseURL = "" }, "base URL is required"},
		{"blank-base-url", func(c *KeycloakAdminConfig) { c.BaseURL = "   " }, "base URL is required"},
		{"missing-realm", func(c *KeycloakAdminConfig) { c.Realm = "" }, "realm is required"},
		{"missing-client-id", func(c *KeycloakAdminConfig) { c.ClientID = "" }, "client ID is required"},
		{"missing-secret", func(c *KeycloakAdminConfig) { c.ClientSecret = "" }, "client secret is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := full
			tt.mutate(&cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestNewKeycloakAdminClient_RejectsInvalidConfig(t *testing.T) {
	_, err := NewKeycloakAdminClient(KeycloakAdminConfig{})
	require.Error(t, err)
}

func TestNewKeycloakAdminClient_OK(t *testing.T) {
	c, err := NewKeycloakAdminClient(KeycloakAdminConfig{
		BaseURL:      "https://auth.example.com",
		Realm:        "bowrain",
		ClientID:     "admin-svc",
		ClientSecret: "secret",
	})
	require.NoError(t, err)
	require.NotNil(t, c)
}

// newKeycloakTestServer stands up a fake Keycloak that issues a client-credentials
// token and accepts user-email updates. tokenHits and putHits count requests.
func newKeycloakTestServer(t *testing.T, tokenHits, putHits *int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/realms/bowrain/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(tokenHits, 1)
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "client_credentials", r.Form.Get("grant_type"))
		assert.Equal(t, "admin-svc", r.Form.Get("client_id"))
		assert.Equal(t, "secret", r.Form.Get("client_secret"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-abc",
			"expires_in":   300,
		})
	})

	mux.HandleFunc("/admin/realms/bowrain/users/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(putHits, 1)
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "Bearer tok-abc", r.Header.Get("Authorization"))
		var body kcUserRep
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "new@example.com", body.Email)
		assert.True(t, body.EmailVerified)
		w.WriteHeader(http.StatusNoContent)
	})

	return srv
}

func newKeycloakClient(t *testing.T, baseURL string) *KeycloakAdminClient {
	t.Helper()
	c, err := NewKeycloakAdminClient(KeycloakAdminConfig{
		BaseURL:      baseURL,
		Realm:        "bowrain",
		ClientID:     "admin-svc",
		ClientSecret: "secret",
	})
	require.NoError(t, err)
	return c
}

func TestKeycloakAdminClient_UpdateUserEmail(t *testing.T) {
	var tokenHits, putHits int32
	srv := newKeycloakTestServer(t, &tokenHits, &putHits)
	c := newKeycloakClient(t, srv.URL)

	require.NoError(t, c.UpdateUserEmail(context.Background(), "user-sub-1", "new@example.com"))
	assert.Equal(t, int32(1), atomic.LoadInt32(&putHits))
	assert.Equal(t, int32(1), atomic.LoadInt32(&tokenHits))
}

func TestKeycloakAdminClient_TokenCachedAcrossCalls(t *testing.T) {
	var tokenHits, putHits int32
	srv := newKeycloakTestServer(t, &tokenHits, &putHits)
	c := newKeycloakClient(t, srv.URL)

	// Two updates should reuse the same (non-expired) token: only one token fetch.
	require.NoError(t, c.UpdateUserEmail(context.Background(), "user-sub-1", "new@example.com"))
	require.NoError(t, c.UpdateUserEmail(context.Background(), "user-sub-2", "new@example.com"))
	assert.Equal(t, int32(2), atomic.LoadInt32(&putHits))
	assert.Equal(t, int32(1), atomic.LoadInt32(&tokenHits), "token should be cached and reused")
}

func TestKeycloakAdminClient_UpdateUserEmail_Validation(t *testing.T) {
	c := newKeycloakClient(t, "https://auth.example.com")
	require.Error(t, c.UpdateUserEmail(context.Background(), "", "new@example.com"))
	require.Error(t, c.UpdateUserEmail(context.Background(), "sub", ""))
}

func TestKeycloakAdminClient_RetriesOn401(t *testing.T) {
	var tokenHits, putHits int32
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/realms/bowrain/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&tokenHits, 1)
		w.Header().Set("Content-Type", "application/json")
		// Return distinct tokens so the retry path uses a freshly-fetched one.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": map[int32]string{1: "stale", 2: "fresh"}[n],
			"expires_in":   300,
		})
	})
	mux.HandleFunc("/admin/realms/bowrain/users/", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&putHits, 1)
		if n == 1 {
			// First attempt: pretend the cached token was rejected.
			assert.Equal(t, "Bearer stale", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Retry: should carry the freshly fetched token and the same body.
		assert.Equal(t, "Bearer fresh", r.Header.Get("Authorization"))
		var body kcUserRep
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "new@example.com", body.Email)
		w.WriteHeader(http.StatusNoContent)
	})

	c := newKeycloakClient(t, srv.URL)
	require.NoError(t, c.UpdateUserEmail(context.Background(), "user-sub-1", "new@example.com"))
	assert.Equal(t, int32(2), atomic.LoadInt32(&putHits), "PUT should be retried once after 401")
	assert.Equal(t, int32(2), atomic.LoadInt32(&tokenHits), "token should be refetched after 401")
}

func TestKeycloakAdminClient_TokenError(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/realms/bowrain/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	})

	c := newKeycloakClient(t, srv.URL)
	err := c.UpdateUserEmail(context.Background(), "user-sub-1", "new@example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keycloak token")
}

func TestKeycloakAdminClient_EmptyAccessToken(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/realms/bowrain/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "", "expires_in": 300})
	})

	c := newKeycloakClient(t, srv.URL)
	err := c.UpdateUserEmail(context.Background(), "user-sub-1", "new@example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty access token")
}

func TestKeycloakAdminClient_UpdateUserEmail_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/realms/bowrain/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 300})
	})
	mux.HandleFunc("/admin/realms/bowrain/users/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("user not found"))
	})

	c := newKeycloakClient(t, srv.URL)
	err := c.UpdateUserEmail(context.Background(), "missing-sub", "new@example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keycloak update user")
	assert.Contains(t, err.Error(), "404")
}
