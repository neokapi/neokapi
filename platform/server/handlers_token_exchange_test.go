package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/platform/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleTokenExchange_WithAPIToken(t *testing.T) {
	srv, _, wsSlug := newTokenTestServer(t)
	e := srv.GetEcho()

	// Create an API token via the API using a JWT.
	jwtToken, err := platauth.GenerateToken(
		&platauth.User{ID: "ignored"}, // we need the real user
		srv.Config.JWTSecret, 1*time.Hour)
	// Instead, get JWT from newTokenTestServer and create API token.
	_ = jwtToken

	// Re-setup: create user, workspace, API token directly.
	ctx := t.Context()
	user := &platauth.User{Email: "exchange@example.com", Name: "Exchange User"}
	require.NoError(t, srv.AuthStore.CreateUser(ctx, user))

	ws := &platauth.Workspace{Name: "Exchange WS", Slug: "exchange-ws"}
	require.NoError(t, srv.AuthStore.CreateWorkspace(ctx, ws))
	require.NoError(t, srv.AuthStore.AddMember(ctx, ws.ID, user.ID, platauth.RoleOwner))

	plaintext := "bwt_exchange123456789abcdef0123456789abcdef0123456789abcdef01234567"
	hash := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(hash[:])

	apiToken := &platauth.APIToken{
		UserID:      user.ID,
		WorkspaceID: ws.ID,
		Name:        "Exchange Token",
		TokenPrefix: plaintext[:8],
		Scopes:      `["*"]`,
	}
	require.NoError(t, srv.AuthStore.CreateAPIToken(ctx, apiToken, tokenHash))
	_ = wsSlug

	// Exchange the API token for a JWT.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token/exchange", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp TokenExchangeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.AccessToken)
	assert.Equal(t, "Bearer", resp.TokenType)
	assert.Equal(t, 3600, resp.ExpiresIn)

	// Validate the returned JWT contains the correct claims.
	claims, err := platauth.ValidateToken(resp.AccessToken, srv.Config.JWTSecret)
	require.NoError(t, err)
	assert.Equal(t, user.ID, claims.Subject)
	assert.Equal(t, "exchange@example.com", claims.Email)
	assert.Equal(t, "Exchange User", claims.Name)

	// Verify the JWT expires in ~1 hour.
	expiresAt := claims.ExpiresAt.Time
	assert.WithinDuration(t, time.Now().Add(1*time.Hour), expiresAt, 5*time.Second)
}

func TestHandleTokenExchange_WithJWT(t *testing.T) {
	// A regular JWT should also work (the middleware accepts both).
	srv, jwt, _ := newTokenTestServer(t)
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token/exchange", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp TokenExchangeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.AccessToken)
	assert.Equal(t, "Bearer", resp.TokenType)
}

func TestHandleTokenExchange_Unauthorized(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.JWTSecret = "test-exchange-secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	e := srv.GetEcho()

	// No auth header at all.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token/exchange", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandleTokenExchange_InvalidAPIToken(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.JWTSecret = "test-exchange-secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token/exchange", nil)
	req.Header.Set("Authorization", "Bearer bwt_invalid_token_that_does_not_exist")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandleTokenExchange_ReturnedJWTWorksForAuth(t *testing.T) {
	srv, _, _ := newTokenTestServer(t)
	e := srv.GetEcho()

	ctx := t.Context()
	user := &platauth.User{Email: "roundtrip@example.com", Name: "Roundtrip User"}
	require.NoError(t, srv.AuthStore.CreateUser(ctx, user))

	// Create API token.
	plaintext := "bwt_roundtrip23456789abcdef0123456789abcdef0123456789abcdef01234567"
	hash := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(hash[:])

	ws := &platauth.Workspace{Name: "RT WS", Slug: "rt-ws"}
	require.NoError(t, srv.AuthStore.CreateWorkspace(ctx, ws))
	require.NoError(t, srv.AuthStore.AddMember(ctx, ws.ID, user.ID, platauth.RoleOwner))

	apiToken := &platauth.APIToken{
		UserID:      user.ID,
		WorkspaceID: ws.ID,
		Name:        "Roundtrip Token",
		TokenPrefix: plaintext[:8],
		Scopes:      `["*"]`,
	}
	require.NoError(t, srv.AuthStore.CreateAPIToken(ctx, apiToken, tokenHash))

	// Step 1: Exchange API token for JWT.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token/exchange", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp TokenExchangeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	// Step 2: Use the returned JWT to call /auth/me.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req2.Header.Set("Authorization", "Bearer "+resp.AccessToken)
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)

	require.Equal(t, http.StatusOK, rec2.Code)
	var meResp map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &meResp))
	assert.Equal(t, "roundtrip@example.com", meResp["email"])
	assert.Equal(t, "Roundtrip User", meResp["name"])
}
