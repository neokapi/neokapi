package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefResolutionMiddleware_NoRefParam(t *testing.T) {
	e := echo.New()
	srv := NewServer(DefaultServerConfig())
	initTestStores(t, srv)

	called := false
	handler := func(c echo.Context) error {
		called = true
		return c.String(http.StatusOK, "ok")
	}

	mw := RefResolutionMiddleware(srv.ContentStore)
	e.GET("/test", mw(handler))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.True(t, called, "handler should be called when no ref param")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRefResolutionMiddleware_MainDefault(t *testing.T) {
	e := echo.New()
	srv := NewServer(DefaultServerConfig())
	initTestStores(t, srv)

	var resolved *ResolvedRef
	handler := func(c echo.Context) error {
		resolved = c.Get("ref").(*ResolvedRef)
		return c.String(http.StatusOK, "ok")
	}

	mw := RefResolutionMiddleware(srv.ContentStore)
	e.GET("/blocks/:ref", mw(handler))

	req := httptest.NewRequest(http.MethodGet, "/blocks/main", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, resolved)
	assert.Equal(t, "main", resolved.Name)
	assert.Equal(t, RefKindStream, resolved.Kind)
}

func TestRefResolutionMiddleware_NoContentStore(t *testing.T) {
	e := echo.New()

	var resolved *ResolvedRef
	handler := func(c echo.Context) error {
		resolved = c.Get("ref").(*ResolvedRef)
		return c.String(http.StatusOK, "ok")
	}

	mw := RefResolutionMiddleware(nil)
	e.GET("/blocks/:ref", mw(handler))

	req := httptest.NewRequest(http.MethodGet, "/blocks/feature-branch", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, resolved)
	assert.Equal(t, "feature-branch", resolved.Name)
	assert.Equal(t, RefKindStream, resolved.Kind)
}

func TestRequireWritableRef_AllowsStream(t *testing.T) {
	e := echo.New()

	handler := func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}

	mw := RequireWritableRef()
	e.PUT("/blocks/:ref/:bid", func(c echo.Context) error {
		c.Set("ref", &ResolvedRef{Name: "main", Kind: RefKindStream})
		return mw(handler)(c)
	})

	req := httptest.NewRequest(http.MethodPut, "/blocks/main/block1", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequireWritableRef_BlocksTag(t *testing.T) {
	e := echo.New()

	handler := func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}

	mw := RequireWritableRef()
	e.PUT("/blocks/:ref/:bid", func(c echo.Context) error {
		c.Set("ref", &ResolvedRef{Name: "v1.2.0", Kind: RefKindTag})
		return mw(handler)(c)
	})

	req := httptest.NewRequest(http.MethodPut, "/blocks/v1.2.0/block1", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)

	var resp ErrorResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp.Error, "read-only")
}
