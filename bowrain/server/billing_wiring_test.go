package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAuthStoreForBilling implements auth.AuthStore with only the methods
// needed by planSyncAdapter and ownerEmailResolver. All other methods panic
// if called, ensuring tests stay focused.
type mockAuthStoreForBilling struct {
	auth.AuthStore // embed to satisfy the full interface

	workspaces map[string]*platauth.Workspace
	members    map[string][]*platauth.Membership
	users      map[string]*platauth.User

	getWorkspaceErr  error
	updateWorkspaceW *platauth.Workspace // captures the workspace passed to UpdateWorkspace
	updateErr        error
	listMembersErr   error
	getUserErr       error
}

func newMockAuthStoreForBilling() *mockAuthStoreForBilling {
	return &mockAuthStoreForBilling{
		workspaces: make(map[string]*platauth.Workspace),
		members:    make(map[string][]*platauth.Membership),
		users:      make(map[string]*platauth.User),
	}
}

func (m *mockAuthStoreForBilling) GetWorkspace(_ context.Context, id string) (*platauth.Workspace, error) {
	if m.getWorkspaceErr != nil {
		return nil, m.getWorkspaceErr
	}
	w, ok := m.workspaces[id]
	if !ok {
		return nil, errors.New("workspace not found")
	}
	// Return a copy so callers can mutate without affecting the store directly.
	cp := *w
	return &cp, nil
}

func (m *mockAuthStoreForBilling) UpdateWorkspace(_ context.Context, w *platauth.Workspace) error {
	m.updateWorkspaceW = w
	if m.updateErr != nil {
		return m.updateErr
	}
	m.workspaces[w.ID] = w
	return nil
}

func (m *mockAuthStoreForBilling) ListMembers(_ context.Context, workspaceID string) ([]*platauth.Membership, error) {
	if m.listMembersErr != nil {
		return nil, m.listMembersErr
	}
	return m.members[workspaceID], nil
}

func (m *mockAuthStoreForBilling) GetUser(_ context.Context, userID string) (*platauth.User, error) {
	if m.getUserErr != nil {
		return nil, m.getUserErr
	}
	u, ok := m.users[userID]
	if !ok {
		return nil, errors.New("user not found")
	}
	return u, nil
}

func (m *mockAuthStoreForBilling) ListUsers(_ context.Context, _, _ int) ([]*platauth.User, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// planSyncAdapter tests
// ---------------------------------------------------------------------------

func TestPlanSyncAdapter_SyncWorkspacePlan(t *testing.T) {
	store := newMockAuthStoreForBilling()
	store.workspaces["ws-1"] = &platauth.Workspace{
		ID:               "ws-1",
		Name:             "Acme",
		Plan:             "free",
		StripeCustomerID: "",
	}

	adapter := &planSyncAdapter{authStore: store}
	err := adapter.SyncWorkspacePlan(t.Context(), "ws-1", "pro", "cus_123")
	require.NoError(t, err)

	updated := store.workspaces["ws-1"]
	assert.Equal(t, "pro", updated.Plan)
	assert.Equal(t, "cus_123", updated.StripeCustomerID)
}

func TestPlanSyncAdapter_SyncWorkspacePlan_PreservesExistingCustomerID(t *testing.T) {
	store := newMockAuthStoreForBilling()
	store.workspaces["ws-1"] = &platauth.Workspace{
		ID:               "ws-1",
		Plan:             "pro",
		StripeCustomerID: "cus_existing",
	}

	adapter := &planSyncAdapter{authStore: store}
	err := adapter.SyncWorkspacePlan(t.Context(), "ws-1", "enterprise", "")
	require.NoError(t, err)

	updated := store.workspaces["ws-1"]
	assert.Equal(t, "enterprise", updated.Plan)
	assert.Equal(t, "cus_existing", updated.StripeCustomerID, "existing customer ID should be preserved when new value is empty")
}

func TestPlanSyncAdapter_SyncWorkspacePlan_WorkspaceNotFound(t *testing.T) {
	store := newMockAuthStoreForBilling()

	adapter := &planSyncAdapter{authStore: store}
	err := adapter.SyncWorkspacePlan(t.Context(), "ws-missing", "pro", "cus_123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ws-missing")
}

// ---------------------------------------------------------------------------
// ownerEmailResolver tests
// ---------------------------------------------------------------------------

func TestOwnerEmailResolver_FindsOwner(t *testing.T) {
	store := newMockAuthStoreForBilling()
	store.members["ws-1"] = []*platauth.Membership{
		{UserID: "u-viewer", WorkspaceID: "ws-1", Role: platauth.RoleViewer, JoinedAt: time.Now()},
		{UserID: "u-owner", WorkspaceID: "ws-1", Role: platauth.RoleOwner, JoinedAt: time.Now()},
		{UserID: "u-member", WorkspaceID: "ws-1", Role: platauth.RoleMember, JoinedAt: time.Now()},
	}
	store.users["u-owner"] = &platauth.User{ID: "u-owner", Email: "owner@example.com"}

	resolver := &ownerEmailResolver{authStore: store}
	email := resolver.GetOwnerEmail(t.Context(), "ws-1")
	assert.Equal(t, "owner@example.com", email)
}

func TestOwnerEmailResolver_NoOwner(t *testing.T) {
	store := newMockAuthStoreForBilling()
	store.members["ws-1"] = []*platauth.Membership{
		{UserID: "u-member", WorkspaceID: "ws-1", Role: platauth.RoleMember},
		{UserID: "u-admin", WorkspaceID: "ws-1", Role: platauth.RoleAdmin},
	}

	resolver := &ownerEmailResolver{authStore: store}
	email := resolver.GetOwnerEmail(t.Context(), "ws-1")
	assert.Empty(t, email)
}

func TestOwnerEmailResolver_ListMembersError(t *testing.T) {
	store := newMockAuthStoreForBilling()
	store.listMembersErr = errors.New("db connection lost")

	resolver := &ownerEmailResolver{authStore: store}
	email := resolver.GetOwnerEmail(t.Context(), "ws-1")
	assert.Empty(t, email)
}
