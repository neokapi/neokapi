package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdminAuthMiddleware verifies the admin BFF middleware: a valid admin
// session cookie authenticates and sets the admin identity; CSRF is enforced on
// state-changing cookie requests; a user session token (wrong audience) does not
// satisfy the admin cookie path; and any non-cookie request falls through to the
// Bearer AdminGuard.
func TestAdminAuthMiddleware(t *testing.T) {
	s := &Server{Config: Config{JWTSecret: testSecret}}

	adminTok, err := platauth.GenerateAdminToken("admin-sub", "admin@x.com", "Admin", testSecret, time.Hour)
	require.NoError(t, err)
	userTok, err := platauth.GenerateToken(&platauth.User{ID: "u1", Email: "u@x.com", Name: "U"}, testSecret, time.Hour)
	require.NoError(t, err)

	guardCalled := false
	guard := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			guardCalled = true
			return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "no bearer"})
		}
	}
	mw := s.adminAuthMiddleware(guard)

	run := func(method string, cookieVal string, csrf bool) (*httptest.ResponseRecorder, echo.Context, bool) {
		guardCalled = false
		e := echo.New()
		req := httptest.NewRequest(method, "/api/admin/x", nil)
		if cookieVal != "" {
			req.AddCookie(&http.Cookie{Name: adminSessionCookieName, Value: cookieVal})
		}
		if csrf {
			req.Header.Set(csrfHeaderName, "1")
		}
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		reached := false
		h := mw(func(c echo.Context) error { reached = true; return c.NoContent(http.StatusOK) })
		_ = h(c)
		return rec, c, reached
	}

	t.Run("valid admin cookie GET passes, sets identity, no guard", func(t *testing.T) {
		rec, c, reached := run(http.MethodGet, adminTok, false)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.True(t, reached)
		assert.False(t, guardCalled)
		assert.Equal(t, "admin@x.com", c.Get("admin_email"))
		assert.Equal(t, "Admin", c.Get("admin_name"))
	})

	t.Run("admin cookie POST without CSRF is rejected", func(t *testing.T) {
		rec, _, reached := run(http.MethodPost, adminTok, false)
		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.False(t, reached)
		assert.False(t, guardCalled)
	})

	t.Run("admin cookie POST with CSRF passes", func(t *testing.T) {
		rec, _, reached := run(http.MethodPost, adminTok, true)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.True(t, reached)
	})

	t.Run("user session token in admin cookie falls back to guard", func(t *testing.T) {
		rec, _, reached := run(http.MethodGet, userTok, false)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.False(t, reached)
		assert.True(t, guardCalled, "wrong-audience token must not satisfy the admin cookie path")
	})

	t.Run("no cookie falls back to guard", func(t *testing.T) {
		rec, _, reached := run(http.MethodGet, "", false)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.False(t, reached)
		assert.True(t, guardCalled)
	})
}

func TestHandleAdminAuthExchangeNotConfigured(t *testing.T) {
	// No AdminVerifier configured → nothing to exchange into.
	s := &Server{Config: Config{JWTSecret: testSecret}}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/auth/exchange", strings.NewReader(`{"id_token":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, s.HandleAdminAuthExchange(c))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHandleAdminAuthMe(t *testing.T) {
	s := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth/me", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("admin_email", "a@x.com")
	c.Set("admin_name", "Admin")
	require.NoError(t, s.HandleAdminAuthMe(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "a@x.com")
}
