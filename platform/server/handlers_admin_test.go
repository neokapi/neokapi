package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleAdminListWorkspaces_NilStore(t *testing.T) {
	s := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/workspaces", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleAdminListWorkspaces(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHandleAdminListWorkspaces_WithData(t *testing.T) {
	store := &mockBillingStore{
		subs: []*billing.Subscription{
			{WorkspaceID: "ws-1", Plan: billing.PlanFree, Status: "active"},
			{WorkspaceID: "ws-2", Plan: billing.PlanPro, Status: "active"},
			{WorkspaceID: "ws-3", Plan: billing.PlanPro, Status: "canceled"},
		},
	}
	s := newBillingTestServer(store)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/workspaces", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleAdminListWorkspaces(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, float64(3), resp["total"])
}

func TestHandleAdminListWorkspaces_FilterByPlan(t *testing.T) {
	store := &mockBillingStore{
		subs: []*billing.Subscription{
			{WorkspaceID: "ws-1", Plan: billing.PlanFree, Status: "active"},
			{WorkspaceID: "ws-2", Plan: billing.PlanPro, Status: "active"},
		},
	}
	s := newBillingTestServer(store)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/workspaces?plan=pro", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleAdminListWorkspaces(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["total"])
}

func TestHandleAdminListWorkspaces_FilterByStatus(t *testing.T) {
	store := &mockBillingStore{
		subs: []*billing.Subscription{
			{WorkspaceID: "ws-1", Plan: billing.PlanPro, Status: "active"},
			{WorkspaceID: "ws-2", Plan: billing.PlanPro, Status: "canceled"},
		},
	}
	s := newBillingTestServer(store)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/workspaces?status=active", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleAdminListWorkspaces(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["total"])
}

func TestHandleAdminGetWorkspace(t *testing.T) {
	store := &mockBillingStore{
		sub: &billing.Subscription{
			WorkspaceID: "ws-1",
			Plan:        billing.PlanPro,
			Status:      "active",
		},
		overrides: []billing.FeatureOverride{
			{Feature: billing.FeatureConnectorsGit, Enabled: true},
		},
		notes: []billing.WorkspaceNote{
			{Content: "test note"},
		},
	}
	s := newBillingTestServer(store)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/workspaces/ws-1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("ws-1")

	err := s.HandleAdminGetWorkspace(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ws-1", resp["workspace_id"])
	assert.NotNil(t, resp["subscription"])
}

func TestHandleAdminUpdatePlan_InvalidPlan(t *testing.T) {
	store := &mockBillingStore{}
	s := newBillingTestServer(store)

	e := echo.New()
	body := `{"plan": "ultra"}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/workspaces/ws-1/plan", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("ws-1")
	c.Set("admin_email", "admin@bowrain.cloud")

	err := s.HandleAdminUpdatePlan(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleAdminUpdatePlan_ValidPlan(t *testing.T) {
	store := &mockBillingStore{
		sub: &billing.Subscription{
			WorkspaceID: "ws-1",
			Plan:        billing.PlanFree,
			Status:      "active",
		},
	}
	s := newBillingTestServer(store)

	e := echo.New()
	body := `{"plan": "pro"}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/workspaces/ws-1/plan", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("ws-1")
	c.Set("admin_email", "admin@bowrain.cloud")

	err := s.HandleAdminUpdatePlan(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, billing.PlanPro, store.upsertedSub.Plan)
}

func TestHandleAdminUpdatePlan_NewSubscription(t *testing.T) {
	store := &mockBillingStore{} // no existing sub
	s := newBillingTestServer(store)

	e := echo.New()
	body := `{"plan": "team"}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/workspaces/ws-new/plan", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("ws-new")
	c.Set("admin_email", "admin@bowrain.cloud")

	err := s.HandleAdminUpdatePlan(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, store.upsertedSub)
	assert.Equal(t, billing.PlanTeam, store.upsertedSub.Plan)
	assert.Equal(t, "ws-new", store.upsertedSub.WorkspaceID)
}

func TestHandleAdminGrantCredits_InvalidAmount(t *testing.T) {
	store := &mockBillingStore{}
	s := newBillingTestServer(store)

	e := echo.New()
	body := `{"amount": 0, "reason": "test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/workspaces/ws-1/credits", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("ws-1")

	err := s.HandleAdminGrantCredits(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleAdminGrantCredits_Success(t *testing.T) {
	store := &mockBillingStore{}
	s := newBillingTestServer(store)

	e := echo.New()
	body := `{"amount": 100000, "reason": "support compensation"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/workspaces/ws-1/credits", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("ws-1")
	c.Set("admin_email", "admin@bowrain.cloud")

	err := s.HandleAdminGrantCredits(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, int64(100000), store.grantedAmount)
	assert.Equal(t, "grant", store.grantedSource)
}

func TestHandleAdminGetFeatureOverrides(t *testing.T) {
	store := &mockBillingStore{
		overrides: []billing.FeatureOverride{
			{Feature: billing.FeatureConnectorsGit, Enabled: true},
		},
	}
	s := newBillingTestServer(store)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/workspaces/ws-1/feature-overrides", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("ws-1")

	err := s.HandleAdminGetFeatureOverrides(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleAdminSetFeatureOverrides(t *testing.T) {
	store := &mockBillingStore{}
	s := newBillingTestServer(store)

	e := echo.New()
	body := `{"feature": "api-access", "enabled": true, "reason": "beta test"}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/workspaces/ws-1/feature-overrides", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("ws-1")
	c.Set("admin_email", "admin@bowrain.cloud")

	err := s.HandleAdminSetFeatureOverrides(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, store.setOverride)
	assert.Equal(t, billing.FeatureAPIAccess, store.setOverride.Feature)
	assert.True(t, store.setOverride.Enabled)
	assert.Equal(t, "ws-1", store.setOverride.WorkspaceID)
}

func TestHandleAdminGetNotes(t *testing.T) {
	store := &mockBillingStore{
		notes: []billing.WorkspaceNote{
			{Content: "note 1"},
			{Content: "note 2"},
		},
	}
	s := newBillingTestServer(store)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/workspaces/ws-1/notes", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("ws-1")

	err := s.HandleAdminGetNotes(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleAdminAddNote_EmptyContent(t *testing.T) {
	store := &mockBillingStore{}
	s := newBillingTestServer(store)

	e := echo.New()
	body := `{"content": ""}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/workspaces/ws-1/notes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("ws-1")

	err := s.HandleAdminAddNote(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleAdminAddNote_Success(t *testing.T) {
	store := &mockBillingStore{}
	s := newBillingTestServer(store)

	e := echo.New()
	body := `{"content": "Customer wants to upgrade"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/workspaces/ws-1/notes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("ws-1")
	c.Set("admin_email", "admin@bowrain.cloud")

	err := s.HandleAdminAddNote(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, rec.Code)
	require.NotNil(t, store.addedNote)
	assert.Equal(t, "Customer wants to upgrade", store.addedNote.Content)
}

func TestHandleAdminGetMetrics(t *testing.T) {
	store := &mockBillingStore{
		metrics: &billing.PlatformMetrics{
			MRR:              12500,
			ActiveWorkspaces: 150,
			NewSignups7d:     23,
			ChurnRate:        2.3,
		},
	}
	s := newBillingTestServer(store)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/metrics", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleAdminGetMetrics(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleAdminGetMetrics_Error(t *testing.T) {
	store := &mockBillingStore{} // nil metrics => error
	s := newBillingTestServer(store)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/metrics", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleAdminGetMetrics(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestHandleAdminListEvents(t *testing.T) {
	store := &mockBillingStore{
		events: []billing.BillingEvent{
			{EventType: "subscription_created", WorkspaceID: "ws-1"},
			{EventType: "payment_failed", WorkspaceID: "ws-2"},
		},
	}
	s := newBillingTestServer(store)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/events", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleAdminListEvents(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleAdminListEvents_DefaultLimit(t *testing.T) {
	store := &mockBillingStore{}
	s := newBillingTestServer(store)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/events", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleAdminListEvents(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleAdminGetUpsells(t *testing.T) {
	store := &mockBillingStore{
		upsells: []billing.UpsellOpportunity{
			{WorkspaceID: "ws-1", Signal: "credit_exhaustion", Score: 90},
		},
	}
	s := newBillingTestServer(store)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/upsells", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleAdminGetUpsells(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleAdminListOverrides(t *testing.T) {
	store := &mockBillingStore{
		allOverrides: []billing.FeatureOverride{
			{WorkspaceID: "ws-1", Feature: billing.FeatureAPIAccess, Enabled: true},
			{WorkspaceID: "ws-2", Feature: billing.FeatureConnectorsGit, Enabled: false},
		},
	}
	s := newBillingTestServer(store)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/overrides", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleAdminListOverrides(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}
