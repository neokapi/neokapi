package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAuthStore implements a minimal auth.AuthStore for admin user handler tests.
type mockAuthStore struct {
	auth.AuthStore
	users      map[string]*platauth.User
	emails     map[string]*platauth.User
	workspaces map[string][]*platauth.Workspace
	allUsers   []*platauth.User
}

func newMockAuthStore() *mockAuthStore {
	return &mockAuthStore{
		users:      make(map[string]*platauth.User),
		emails:     make(map[string]*platauth.User),
		workspaces: make(map[string][]*platauth.Workspace),
	}
}

func (m *mockAuthStore) GetUser(_ context.Context, id string) (*platauth.User, error) {
	if u, ok := m.users[id]; ok {
		return u, nil
	}
	return nil, assert.AnError
}

func (m *mockAuthStore) GetUserByEmail(_ context.Context, email string) (*platauth.User, error) {
	if u, ok := m.emails[email]; ok {
		return u, nil
	}
	return nil, assert.AnError
}

func (m *mockAuthStore) ListWorkspaces(_ context.Context, userID string) ([]*platauth.Workspace, error) {
	if ws, ok := m.workspaces[userID]; ok {
		return ws, nil
	}
	return nil, nil
}

func (m *mockAuthStore) SearchUsers(_ context.Context, query string, limit int) ([]*platauth.User, error) {
	var results []*platauth.User
	for _, u := range m.emails {
		if strings.Contains(strings.ToLower(u.Email), strings.ToLower(query)) ||
			strings.Contains(strings.ToLower(u.Name), strings.ToLower(query)) {
			results = append(results, u)
		}
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (m *mockAuthStore) ListUsers(_ context.Context, limit, offset int) ([]*platauth.User, error) {
	if offset >= len(m.allUsers) {
		return nil, nil
	}
	end := min(offset+limit, len(m.allUsers))
	return m.allUsers[offset:end], nil
}

func (m *mockAuthStore) Close() error { return nil }

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
	assert.Equal(t, "ws-1", resp["id"])
	assert.Equal(t, "pro", resp["plan"])
	assert.Equal(t, "active", resp["status"])
	assert.NotNil(t, resp["members"])
	assert.NotNil(t, resp["recent_activity"])
}

func TestHandleAdminGetWorkspace_WithAuthStore(t *testing.T) {
	billingStore := &mockBillingStore{
		sub: &billing.Subscription{
			WorkspaceID: "ws-1",
			Plan:        billing.PlanPro,
			Status:      "active",
			SeatCount:   3,
		},
		alloc: &billing.CreditAllocation{
			CreditsTotal: 10000,
			CreditsUsed:  2500,
		},
	}
	authStore := newMockAuthStoreForBilling()
	authStore.workspaces["ws-1"] = &platauth.Workspace{
		ID:   "ws-1",
		Name: "Acme Corp",
		Slug: "acme",
	}
	authStore.members["ws-1"] = []*platauth.Membership{
		{UserID: "u-1", WorkspaceID: "ws-1", Role: platauth.RoleOwner, JoinedAt: time.Now()},
		{UserID: "u-2", WorkspaceID: "ws-1", Role: platauth.RoleMember, JoinedAt: time.Now()},
	}
	authStore.users["u-1"] = &platauth.User{ID: "u-1", Email: "owner@acme.com", Name: "Owner"}
	authStore.users["u-2"] = &platauth.User{ID: "u-2", Email: "member@acme.com", Name: "Member"}

	s := &Server{BillingStore: billingStore, AuthStore: authStore}

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
	assert.Equal(t, "Acme Corp", resp["name"])
	assert.Equal(t, "acme", resp["slug"])
	assert.Equal(t, "owner@acme.com", resp["owner_email"])
	assert.Equal(t, float64(2), resp["member_count"])
	assert.Equal(t, float64(2500), resp["credits_used"])
	assert.Equal(t, float64(10000), resp["credits_total"])
	assert.Equal(t, 25.0, resp["credit_usage_percent"])

	members := resp["members"].([]any)
	assert.Len(t, members, 2)
	firstMember := members[0].(map[string]any)
	assert.Equal(t, "owner@acme.com", firstMember["email"])
}

func TestHandleAdminGetLedger(t *testing.T) {
	store := &mockBillingStore{
		ledger: []billing.LedgerEntry{
			{ID: 1, WorkspaceID: "ws-1", Amount: 1000, BalanceAfter: 9000, Operation: "grant"},
			{ID: 2, WorkspaceID: "ws-1", Amount: -500, BalanceAfter: 8500, Operation: "ai_translation"},
		},
	}
	s := newBillingTestServer(store)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/workspaces/ws-1/ledger", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("ws-1")

	err := s.HandleAdminGetLedger(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var entries []billing.LedgerEntry
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &entries))
	assert.Len(t, entries, 2)
	assert.Equal(t, int64(1000), entries[0].Amount)
}

func TestHandleAdminGetLedger_NilStore(t *testing.T) {
	s := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/workspaces/ws-1/ledger", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("ws-1")

	err := s.HandleAdminGetLedger(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
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

// ---------------------------------------------------------------------------
// Admin user handler tests
// ---------------------------------------------------------------------------

func TestHandleAdminListUsers_NilAuthStore(t *testing.T) {
	s := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users?q=test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleAdminListUsers(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHandleAdminListUsers_EmptyQuery_ReturnsAllUsers(t *testing.T) {
	authStore := newMockAuthStore()
	authStore.allUsers = []*platauth.User{
		{ID: "u-1", Email: "alice@acme.com", Name: "Alice"},
		{ID: "u-2", Email: "bob@acme.com", Name: "Bob"},
	}
	s := &Server{AuthStore: authStore}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleAdminListUsers(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, float64(2), resp["total"])
	users := resp["users"].([]any)
	assert.Len(t, users, 2)
}

func TestHandleAdminListUsers_EmptyQuery_NoUsers(t *testing.T) {
	authStore := newMockAuthStore()
	s := &Server{AuthStore: authStore}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleAdminListUsers(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["total"])
}

func TestHandleAdminListUsers_FoundByEmail(t *testing.T) {
	authStore := newMockAuthStore()
	user := &platauth.User{
		ID:        "u-1",
		Email:     "alice@acme.com",
		Name:      "Alice",
		CreatedAt: time.Now().UTC(),
	}
	authStore.emails["alice@acme.com"] = user
	s := &Server{AuthStore: authStore}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users?q=alice@acme.com", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleAdminListUsers(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["total"])
	users := resp["users"].([]any)
	firstUser := users[0].(map[string]any)
	assert.Equal(t, "alice@acme.com", firstUser["email"])
}

func TestHandleAdminListUsers_NotFound(t *testing.T) {
	authStore := newMockAuthStore()
	s := &Server{AuthStore: authStore}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users?q=nobody@example.com", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.HandleAdminListUsers(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["total"])
}

func TestHandleAdminGetUser_NilAuthStore(t *testing.T) {
	s := &Server{}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/u-1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("u-1")

	err := s.HandleAdminGetUser(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHandleAdminGetUser_NotFound(t *testing.T) {
	authStore := newMockAuthStore()
	s := &Server{AuthStore: authStore}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/u-999", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("u-999")

	err := s.HandleAdminGetUser(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleAdminGetUser_Found(t *testing.T) {
	authStore := newMockAuthStore()
	user := &platauth.User{
		ID:        "u-1",
		Email:     "alice@acme.com",
		Name:      "Alice",
		CreatedAt: time.Now().UTC(),
	}
	authStore.users["u-1"] = user
	authStore.workspaces["u-1"] = []*platauth.Workspace{
		{ID: "ws-1", Name: "Acme Corp", Slug: "acme", Plan: "pro"},
		{ID: "ws-2", Name: "Side Project", Slug: "side", Plan: "free"},
	}
	s := &Server{AuthStore: authStore}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/u-1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("u-1")

	err := s.HandleAdminGetUser(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	userResp := resp["user"].(map[string]any)
	assert.Equal(t, "alice@acme.com", userResp["email"])
	workspaces := resp["workspaces"].([]any)
	assert.Len(t, workspaces, 2)
}

func TestHandleAdminGetUser_NoWorkspaces(t *testing.T) {
	authStore := newMockAuthStore()
	user := &platauth.User{
		ID:    "u-2",
		Email: "bob@example.com",
		Name:  "Bob",
	}
	authStore.users["u-2"] = user
	// No workspaces for this user.
	s := &Server{AuthStore: authStore}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/u-2", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("u-2")

	err := s.HandleAdminGetUser(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotNil(t, resp["user"])
}
