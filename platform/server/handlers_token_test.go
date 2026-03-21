package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/platform/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTokenTestServer creates a test server with auth configured.
func newTokenTestServer(t *testing.T) (*Server, string, string) {
	t.Helper()
	cfg := DefaultServerConfig()

	cfg.JWTSecret = "test-token-secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)

	require.NotNil(t, srv.Services)
	require.NotNil(t, srv.Services.Auth)
	require.NotNil(t, srv.AuthStore)

	ctx := t.Context()

	// Create a user.
	user := &platauth.User{Email: "tokenuser@example.com", Name: "Token User"}
	require.NoError(t, srv.AuthStore.CreateUser(ctx, user))

	// Create a workspace with Pro plan (token routes require FeatureAPIAccess)
	// and add user as owner.
	ws := &platauth.Workspace{Name: "Token WS", Slug: "token-ws", Plan: "pro"}
	require.NoError(t, srv.AuthStore.CreateWorkspace(ctx, ws))
	require.NoError(t, srv.AuthStore.AddMember(ctx, ws.ID, user.ID, platauth.RoleOwner))

	// Generate JWT for the user.
	token, err := platauth.GenerateToken(user, cfg.JWTSecret, 1*time.Hour)
	require.NoError(t, err)

	return srv, token, ws.Slug
}

func TestCreateToken(t *testing.T) {
	srv, jwt, wsSlug := newTokenTestServer(t)
	e := srv.GetEcho()

	body := `{"name":"CI Token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+wsSlug+"/tokens",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var resp CreateTokenResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.ID)
	assert.Equal(t, "CI Token", resp.Name)
	assert.True(t, strings.HasPrefix(resp.Token, "bwt_"))
	assert.Equal(t, resp.Token[:8], resp.TokenPrefix)
	assert.Equal(t, `["*"]`, resp.Scopes) // default full access
	assert.Nil(t, resp.ExpiresAt)
}

func TestCreateTokenWithExpiration(t *testing.T) {
	srv, jwt, wsSlug := newTokenTestServer(t)
	e := srv.GetEcho()

	body := `{"name":"Short-lived","expire_days":30}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+wsSlug+"/tokens",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var resp CreateTokenResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotNil(t, resp.ExpiresAt)
	assert.True(t, resp.ExpiresAt.After(time.Now()))
}

func TestCreateTokenWithScopes(t *testing.T) {
	srv, jwt, wsSlug := newTokenTestServer(t)
	e := srv.GetEcho()

	body := `{"name":"Scoped Token","scopes":["translate:fr,de"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+wsSlug+"/tokens",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var resp CreateTokenResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "Scoped Token", resp.Name)
	assert.Equal(t, `["translate:fr,de"]`, resp.Scopes)
}

func TestCreateTokenWithInvalidScopes(t *testing.T) {
	srv, jwt, wsSlug := newTokenTestServer(t)
	e := srv.GetEcho()

	body := `{"name":"Bad Scopes","scopes":["delete"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+wsSlug+"/tokens",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateTokenWithMultipleScopes(t *testing.T) {
	srv, jwt, wsSlug := newTokenTestServer(t)
	e := srv.GetEcho()

	body := `{"name":"Multi Scope","scopes":["read","translate:fr"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+wsSlug+"/tokens",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var resp CreateTokenResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, `["read","translate:fr"]`, resp.Scopes)
}

func TestCreateTokenScopesPersistedInList(t *testing.T) {
	srv, jwt, wsSlug := newTokenTestServer(t)
	e := srv.GetEcho()

	// Create a scoped token.
	body := `{"name":"Review Token","scopes":["review"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+wsSlug+"/tokens",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	// List tokens and verify scopes persisted.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+wsSlug+"/tokens", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var tokens []*platauth.APIToken
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tokens))
	require.Len(t, tokens, 1)
	assert.Equal(t, `["review"]`, tokens[0].Scopes)
}

func TestCreateTokenMissingName(t *testing.T) {
	srv, jwt, wsSlug := newTokenTestServer(t)
	e := srv.GetEcho()

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+wsSlug+"/tokens",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListTokens(t *testing.T) {
	srv, jwt, wsSlug := newTokenTestServer(t)
	e := srv.GetEcho()

	// Create 2 tokens.
	for _, name := range []string{"Token A", "Token B"} {
		body := `{"name":"` + name + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+wsSlug+"/tokens",
			strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+jwt)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)
	}

	// List tokens.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+wsSlug+"/tokens", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var tokens []*platauth.APIToken
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tokens))
	assert.Len(t, tokens, 2)
}

func TestDeleteToken(t *testing.T) {
	srv, jwt, wsSlug := newTokenTestServer(t)
	e := srv.GetEcho()

	// Create a token.
	body := `{"name":"Doomed Token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+wsSlug+"/tokens",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var created CreateTokenResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	// Delete it.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/workspaces/"+wsSlug+"/tokens/"+created.ID, nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// List should be empty.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+wsSlug+"/tokens", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var tokens []*platauth.APIToken
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tokens))
	assert.Empty(t, tokens)
}

func TestUseAPITokenForAuth(t *testing.T) {
	srv, jwt, wsSlug := newTokenTestServer(t)
	e := srv.GetEcho()

	// Create a token via the API.
	body := `{"name":"Auth Token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+wsSlug+"/tokens",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var created CreateTokenResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	// Use the API token to access a protected endpoint (e.g., /auth/me).
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+created.Token)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var meResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &meResp))
	assert.Equal(t, "tokenuser@example.com", meResp["email"])
}
