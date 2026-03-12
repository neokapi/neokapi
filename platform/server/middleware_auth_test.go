package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/platform/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-jwt-secret-32-bytes-long!!!"

func TestAuthMiddleware(t *testing.T) {
	user := &platauth.User{ID: "user-1", Email: "test@example.com", Name: "Test"}
	token, err := platauth.GenerateToken(user, testSecret, 1*time.Hour)
	require.NoError(t, err)

	mw := AuthMiddleware(testSecret, nil)

	t.Run("valid token", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		var userID string
		handler := mw(func(c echo.Context) error {
			userID = c.Get("user_id").(string)
			return c.NoContent(http.StatusOK)
		})
		err := handler(c)
		assert.NoError(t, err)
		assert.Equal(t, "user-1", userID)
	})

	t.Run("missing header", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := mw(func(c echo.Context) error {
			return c.NoContent(http.StatusOK)
		})
		_ = handler(c)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("invalid token", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := mw(func(c echo.Context) error {
			return c.NoContent(http.StatusOK)
		})
		_ = handler(c)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("wrong format", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := mw(func(c echo.Context) error {
			return c.NoContent(http.StatusOK)
		})
		_ = handler(c)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestAuthMiddleware_APIToken(t *testing.T) {
	store, err := auth.NewSQLiteAuthStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	// Create a user and workspace.
	user := &platauth.User{Email: "apiuser@example.com", Name: "API User"}
	require.NoError(t, store.CreateUser(t.Context(), user))
	ws := &platauth.Workspace{Name: "Test WS", Slug: "test-ws"}
	require.NoError(t, store.CreateWorkspace(t.Context(), ws))

	// Create an API token.
	plaintext := "bwt_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	hash := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(hash[:])

	apiToken := &platauth.APIToken{
		UserID:      user.ID,
		WorkspaceID: ws.ID,
		Name:        "CI Token",
		TokenPrefix: plaintext[:8],
		Scopes:      `["*"]`,
	}
	require.NoError(t, store.CreateAPIToken(t.Context(), apiToken, tokenHash))

	mw := AuthMiddleware(testSecret, store)

	t.Run("valid api token", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		var userID, tokenID string
		handler := mw(func(c echo.Context) error {
			userID = c.Get("user_id").(string)
			tokenID = c.Get("api_token_id").(string)
			return c.NoContent(http.StatusOK)
		})
		err := handler(c)
		assert.NoError(t, err)
		assert.Equal(t, user.ID, userID)
		assert.Equal(t, apiToken.ID, tokenID)
	})

	t.Run("invalid api token", func(t *testing.T) {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer bwt_invalid")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := mw(func(c echo.Context) error {
			return c.NoContent(http.StatusOK)
		})
		_ = handler(c)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("expired api token", func(t *testing.T) {
		// Create an expired token.
		expiredPlaintext := "bwt_expired456789abcdef0123456789abcdef0123456789abcdef0123456789"
		expHash := sha256.Sum256([]byte(expiredPlaintext))
		expTokenHash := hex.EncodeToString(expHash[:])

		past := time.Now().Add(-1 * time.Hour)
		expToken := &platauth.APIToken{
			UserID:      user.ID,
			WorkspaceID: ws.ID,
			Name:        "Expired Token",
			TokenPrefix: expiredPlaintext[:8],
			Scopes:      `["*"]`,
			ExpiresAt:   &past,
		}
		require.NoError(t, store.CreateAPIToken(t.Context(), expToken, expTokenHash))

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+expiredPlaintext)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := mw(func(c echo.Context) error {
			return c.NoContent(http.StatusOK)
		})
		_ = handler(c)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}
