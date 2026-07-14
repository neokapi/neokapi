package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/platformconfig"
)

// envOnlyConfig builds a non-persistent (no DB) platform config service with the
// production-shaped bootstrap defaults, exactly like an un-provisioned instance.
func envOnlyConfig() *platformconfig.Service {
	return platformconfig.NewService(nil, platformconfig.Defaults{
		AIProvider:     "bedrock",
		AIDefaultModel: "eu.anthropic.claude-sonnet-4-6",
		SignupsOpen:    true,
		DefaultPlan:    "pro",
		TrialDays:      14,
	})
}

func TestHandleGetPublicConfig(t *testing.T) {
	s := &Server{PlatformConfig: envOnlyConfig()}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, s.HandleGetPublicConfig(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp platformconfig.PublicSnapshot
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.SignupsOpen)
	assert.False(t, resp.CustomerChoice)
	// With customer choice off, the enabled-model list is not exposed publicly.
	assert.Empty(t, resp.EnabledModels)
}

func TestHandleAdminGetPlatformConfig_EnvOnly(t *testing.T) {
	s := &Server{PlatformConfig: envOnlyConfig()}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/platform", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, s.HandleAdminGetPlatformConfig(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp platformConfigResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.False(t, resp.Persistent, "env-only instance is not persistent")
	assert.Equal(t, "bedrock", resp.Snapshot.AI.Provider)
	assert.NotEmpty(t, resp.Snapshot.ModelCatalog, "snapshot carries the model catalog for onboard/offboard")
}

func TestHandleAdminUpdatePlatformConfig_NotPersistent(t *testing.T) {
	s := &Server{PlatformConfig: envOnlyConfig()}
	e := echo.New()
	body := `{"signups":{"open":false}}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/platform", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("admin_email", "admin@example.com")

	require.NoError(t, s.HandleAdminUpdatePlatformConfig(c))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code,
		"a DB-less instance cannot persist config changes")
}
