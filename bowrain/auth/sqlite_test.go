package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *SQLiteAuthStore {
	t.Helper()
	s, err := NewSQLiteAuthStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCreateAndGetUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	u := &User{Email: "alice@example.com", Name: "Alice"}
	require.NoError(t, s.CreateUser(ctx, u))
	assert.NotEmpty(t, u.ID)
	assert.False(t, u.CreatedAt.IsZero())

	got, err := s.GetUser(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
	assert.Equal(t, "alice@example.com", got.Email)
	assert.Equal(t, "Alice", got.Name)
}

func TestGetUserByEmail(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	u := &User{Email: "bob@example.com", Name: "Bob"}
	require.NoError(t, s.CreateUser(ctx, u))

	got, err := s.GetUserByEmail(ctx, "bob@example.com")
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
}

func TestUpdateUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	u := &User{Email: "carol@example.com", Name: "Carol"}
	require.NoError(t, s.CreateUser(ctx, u))

	u.Name = "Carol Updated"
	u.AvatarURL = "https://example.com/avatar.png"
	require.NoError(t, s.UpdateUser(ctx, u))

	got, err := s.GetUser(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, "Carol Updated", got.Name)
	assert.Equal(t, "https://example.com/avatar.png", got.AvatarURL)
}

func TestCreateAndGetWorkspace(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	w := &Workspace{Name: "My Team", Slug: "my-team", Description: "Test workspace"}
	require.NoError(t, s.CreateWorkspace(ctx, w))
	assert.NotEmpty(t, w.ID)

	got, err := s.GetWorkspace(ctx, w.ID)
	require.NoError(t, err)
	assert.Equal(t, "My Team", got.Name)
	assert.Equal(t, "my-team", got.Slug)
}

func TestGetWorkspaceBySlug(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	w := &Workspace{Name: "Acme Corp", Slug: "acme-corp"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	got, err := s.GetWorkspaceBySlug(ctx, "acme-corp")
	require.NoError(t, err)
	assert.Equal(t, w.ID, got.ID)
}

func TestUpdateWorkspace(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	w := &Workspace{Name: "Old Name", Slug: "old-name"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	w.Name = "New Name"
	w.Slug = "new-name"
	require.NoError(t, s.UpdateWorkspace(ctx, w))

	got, err := s.GetWorkspace(ctx, w.ID)
	require.NoError(t, err)
	assert.Equal(t, "New Name", got.Name)
	assert.Equal(t, "new-name", got.Slug)
}

func TestDeleteWorkspace(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	w := &Workspace{Name: "Ephemeral", Slug: "ephemeral"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	require.NoError(t, s.DeleteWorkspace(ctx, w.ID))

	_, err := s.GetWorkspace(ctx, w.ID)
	assert.Error(t, err)
}

func TestMembership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	u := &User{Email: "dan@example.com", Name: "Dan"}
	require.NoError(t, s.CreateUser(ctx, u))

	w := &Workspace{Name: "Team", Slug: "team"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	// Add member.
	require.NoError(t, s.AddMember(ctx, w.ID, u.ID, RoleMember))

	// Get membership.
	m, err := s.GetMembership(ctx, w.ID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, RoleMember, m.Role)

	// List members.
	members, err := s.ListMembers(ctx, w.ID)
	require.NoError(t, err)
	require.Len(t, members, 1)
	assert.Equal(t, u.ID, members[0].UserID)

	// Update role.
	require.NoError(t, s.UpdateRole(ctx, w.ID, u.ID, RoleAdmin))
	m, err = s.GetMembership(ctx, w.ID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, RoleAdmin, m.Role)

	// Remove member.
	require.NoError(t, s.RemoveMember(ctx, w.ID, u.ID))
	members, err = s.ListMembers(ctx, w.ID)
	require.NoError(t, err)
	assert.Empty(t, members)
}

func TestListWorkspacesByUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	u := &User{Email: "eve@example.com", Name: "Eve"}
	require.NoError(t, s.CreateUser(ctx, u))

	w1 := &Workspace{Name: "Alpha", Slug: "alpha"}
	w2 := &Workspace{Name: "Beta", Slug: "beta"}
	require.NoError(t, s.CreateWorkspace(ctx, w1))
	require.NoError(t, s.CreateWorkspace(ctx, w2))

	require.NoError(t, s.AddMember(ctx, w1.ID, u.ID, RoleOwner))
	require.NoError(t, s.AddMember(ctx, w2.ID, u.ID, RoleMember))

	workspaces, err := s.ListWorkspaces(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, workspaces, 2)
	assert.Equal(t, "Alpha", workspaces[0].Name)
	assert.Equal(t, "Beta", workspaces[1].Name)
}

func TestInvalidRole(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	u := &User{Email: "test@example.com", Name: "Test"}
	require.NoError(t, s.CreateUser(ctx, u))
	w := &Workspace{Name: "WS", Slug: "ws"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	err := s.AddMember(ctx, w.ID, u.ID, Role("superadmin"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid role")
}

func TestDeleteWorkspaceCascadesMemberships(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	u := &User{Email: "frank@example.com", Name: "Frank"}
	require.NoError(t, s.CreateUser(ctx, u))
	w := &Workspace{Name: "Cascade", Slug: "cascade"}
	require.NoError(t, s.CreateWorkspace(ctx, w))
	require.NoError(t, s.AddMember(ctx, w.ID, u.ID, RoleMember))

	require.NoError(t, s.DeleteWorkspace(ctx, w.ID))

	// Membership should be gone due to CASCADE.
	members, err := s.ListMembers(ctx, w.ID)
	require.NoError(t, err)
	assert.Empty(t, members)
}
