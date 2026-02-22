package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	platauth "github.com/gokapi/gokapi/platform/auth"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-jwt-secret-32-bytes-long!!!"

func TestAuthMiddleware(t *testing.T) {
	user := &platauth.User{ID: "user-1", Email: "test@example.com", Name: "Test"}
	token, err := platauth.GenerateToken(user, testSecret, 1*time.Hour)
	require.NoError(t, err)

	mw := AuthMiddleware(testSecret)

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
