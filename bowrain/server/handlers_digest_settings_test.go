package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupDigestTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	srv := NewServer(cfg)
	initTestStores(t, srv)

	sqliteStore := srv.ContentStore.(*bstore.SQLiteStore)
	db := sqliteStore.DB()
	srv.DigestStore = bstore.NewDigestStore(db)

	return srv
}

func TestHandleGetDigestSettings_Default(t *testing.T) {
	srv := setupDigestTestServer(t)
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws-1/digest-settings", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("ws-1")
	c.Set("user_id", "user-1")

	err := srv.HandleGetDigestSettings(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var settings bstore.DigestSettings
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &settings))
	assert.Equal(t, bstore.DigestDaily, settings.Frequency)
	assert.Equal(t, "user-1", settings.UserID)
	assert.Equal(t, "ws-1", settings.WorkspaceID)
	assert.Equal(t, "UTC", settings.Timezone)
}

func TestHandleGetDigestSettings_NilStore(t *testing.T) {
	cfg := DefaultConfig()
	srv := NewServer(cfg)
	initTestStores(t, srv)
	// DigestStore is nil by default.

	e := srv.GetEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws-1/digest-settings", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("ws-1")
	c.Set("user_id", "user-1")

	err := srv.HandleGetDigestSettings(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var settings bstore.DigestSettings
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &settings))
	assert.Equal(t, bstore.DigestDaily, settings.Frequency)
}

func TestHandleUpdateDigestSettings_Valid(t *testing.T) {
	srv := setupDigestTestServer(t)
	e := srv.GetEcho()

	body := `{"frequency":"weekly","timezone":"America/New_York","quiet_start":"22:00","quiet_end":"08:00"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/ws-1/digest-settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("ws-1")
	c.Set("user_id", "user-1")

	err := srv.HandleUpdateDigestSettings(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var settings bstore.DigestSettings
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &settings))
	assert.Equal(t, bstore.DigestWeekly, settings.Frequency)
	assert.Equal(t, "America/New_York", settings.Timezone)
	assert.Equal(t, "22:00", settings.QuietStart)
	assert.Equal(t, "08:00", settings.QuietEnd)
	assert.Equal(t, "user-1", settings.UserID)
	assert.Equal(t, "ws-1", settings.WorkspaceID)
}

func TestHandleUpdateDigestSettings_InvalidFrequency(t *testing.T) {
	srv := setupDigestTestServer(t)
	e := srv.GetEcho()

	body := `{"frequency":"hourly"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/ws-1/digest-settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("ws-1")
	c.Set("user_id", "user-1")

	err := srv.HandleUpdateDigestSettings(c)
	require.Error(t, err)

	var he *echo.HTTPError
	require.ErrorAs(t, err, &he)
	assert.Equal(t, http.StatusBadRequest, he.Code)
}

func TestHandleUpdateDigestSettings_Off(t *testing.T) {
	srv := setupDigestTestServer(t)
	e := srv.GetEcho()

	body := `{"frequency":"off"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/ws-1/digest-settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("ws-1")
	c.Set("user_id", "user-1")

	err := srv.HandleUpdateDigestSettings(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var settings bstore.DigestSettings
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &settings))
	assert.Equal(t, bstore.DigestOff, settings.Frequency)
}

func TestHandleUpdateDigestSettings_NilStore(t *testing.T) {
	cfg := DefaultConfig()
	srv := NewServer(cfg)
	initTestStores(t, srv)
	// DigestStore is nil by default.

	e := srv.GetEcho()
	body := `{"frequency":"daily"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/ws-1/digest-settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("ws-1")
	c.Set("user_id", "user-1")

	err := srv.HandleUpdateDigestSettings(c)
	require.Error(t, err)

	var he *echo.HTTPError
	require.ErrorAs(t, err, &he)
	assert.Equal(t, http.StatusServiceUnavailable, he.Code)
}

func TestHandleUpdateDigestSettings_Roundtrip(t *testing.T) {
	srv := setupDigestTestServer(t)
	e := srv.GetEcho()

	// Update settings.
	body := `{"frequency":"weekly","timezone":"Europe/Berlin","quiet_start":"23:00","quiet_end":"07:00"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/ws-1/digest-settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("ws-1")
	c.Set("user_id", "user-1")

	err := srv.HandleUpdateDigestSettings(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Read back via GET.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/ws-1/digest-settings", nil)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("ws")
	c2.SetParamValues("ws-1")
	c2.Set("user_id", "user-1")

	err = srv.HandleGetDigestSettings(c2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec2.Code)

	var settings bstore.DigestSettings
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &settings))
	assert.Equal(t, bstore.DigestWeekly, settings.Frequency)
	assert.Equal(t, "Europe/Berlin", settings.Timezone)
	assert.Equal(t, "23:00", settings.QuietStart)
	assert.Equal(t, "07:00", settings.QuietEnd)
}
