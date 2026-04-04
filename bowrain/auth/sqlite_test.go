package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
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
	ctx := t.Context()

	u := &platauth.User{Email: "alice@example.com", Name: "Alice"}
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
	ctx := t.Context()

	u := &platauth.User{Email: "bob@example.com", Name: "Bob"}
	require.NoError(t, s.CreateUser(ctx, u))

	got, err := s.GetUserByEmail(ctx, "bob@example.com")
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
}

func TestUpdateUser(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	u := &platauth.User{Email: "carol@example.com", Name: "Carol"}
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
	ctx := t.Context()

	w := &platauth.Workspace{Name: "My Team", Slug: "my-team", Description: "Test workspace"}
	require.NoError(t, s.CreateWorkspace(ctx, w))
	assert.NotEmpty(t, w.ID)

	got, err := s.GetWorkspace(ctx, w.ID)
	require.NoError(t, err)
	assert.Equal(t, "My Team", got.Name)
	assert.Equal(t, "my-team", got.Slug)
}

func TestGetWorkspaceBySlug(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	w := &platauth.Workspace{Name: "Acme Corp", Slug: "acme-corp"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	got, err := s.GetWorkspaceBySlug(ctx, "acme-corp")
	require.NoError(t, err)
	assert.Equal(t, w.ID, got.ID)
}

func TestUpdateWorkspace(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	w := &platauth.Workspace{Name: "Old Name", Slug: "old-name"}
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
	ctx := t.Context()

	w := &platauth.Workspace{Name: "Ephemeral", Slug: "ephemeral"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	require.NoError(t, s.DeleteWorkspace(ctx, w.ID))

	_, err := s.GetWorkspace(ctx, w.ID)
	assert.Error(t, err)
}

func TestMembership(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	u := &platauth.User{Email: "dan@example.com", Name: "Dan"}
	require.NoError(t, s.CreateUser(ctx, u))

	w := &platauth.Workspace{Name: "Team", Slug: "team"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	// Add member.
	require.NoError(t, s.AddMember(ctx, w.ID, u.ID, platauth.RoleMember))

	// Get membership.
	m, err := s.GetMembership(ctx, w.ID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, platauth.RoleMember, m.Role)

	// List members.
	members, err := s.ListMembers(ctx, w.ID)
	require.NoError(t, err)
	require.Len(t, members, 1)
	assert.Equal(t, u.ID, members[0].UserID)

	// Update role.
	require.NoError(t, s.UpdateRole(ctx, w.ID, u.ID, platauth.RoleAdmin))
	m, err = s.GetMembership(ctx, w.ID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, platauth.RoleAdmin, m.Role)

	// Remove member.
	require.NoError(t, s.RemoveMember(ctx, w.ID, u.ID))
	members, err = s.ListMembers(ctx, w.ID)
	require.NoError(t, err)
	assert.Empty(t, members)
}

func TestListWorkspacesByUser(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	u := &platauth.User{Email: "eve@example.com", Name: "Eve"}
	require.NoError(t, s.CreateUser(ctx, u))

	w1 := &platauth.Workspace{Name: "Alpha", Slug: "alpha"}
	w2 := &platauth.Workspace{Name: "Beta", Slug: "beta"}
	require.NoError(t, s.CreateWorkspace(ctx, w1))
	require.NoError(t, s.CreateWorkspace(ctx, w2))

	require.NoError(t, s.AddMember(ctx, w1.ID, u.ID, platauth.RoleOwner))
	require.NoError(t, s.AddMember(ctx, w2.ID, u.ID, platauth.RoleMember))

	workspaces, err := s.ListWorkspaces(ctx, u.ID)
	require.NoError(t, err)
	require.Len(t, workspaces, 2)
	assert.Equal(t, "Alpha", workspaces[0].Name)
	assert.Equal(t, "Beta", workspaces[1].Name)
}

func TestInvalidRole(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	u := &platauth.User{Email: "test@example.com", Name: "Test"}
	require.NoError(t, s.CreateUser(ctx, u))
	w := &platauth.Workspace{Name: "WS", Slug: "ws"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	err := s.AddMember(ctx, w.ID, u.ID, platauth.Role("superadmin"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid role")
}

func TestDeleteWorkspaceCascadesMemberships(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	u := &platauth.User{Email: "frank@example.com", Name: "Frank"}
	require.NoError(t, s.CreateUser(ctx, u))
	w := &platauth.Workspace{Name: "Cascade", Slug: "cascade"}
	require.NoError(t, s.CreateWorkspace(ctx, w))
	require.NoError(t, s.AddMember(ctx, w.ID, u.ID, platauth.RoleMember))

	require.NoError(t, s.DeleteWorkspace(ctx, w.ID))

	// Membership should be gone due to CASCADE.
	members, err := s.ListMembers(ctx, w.ID)
	require.NoError(t, err)
	assert.Empty(t, members)
}

// ---------------------------------------------------------------------------
// Refresh Tokens
// ---------------------------------------------------------------------------

func TestStoreAndValidateRefreshToken(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	u := &platauth.User{Email: "refresh@example.com", Name: "Refresh User"}
	require.NoError(t, s.CreateUser(ctx, u))

	tokenHash := "abc123hash"
	expiresAt := time.Now().Add(30 * 24 * time.Hour)

	id, err := s.StoreRefreshToken(ctx, u.ID, tokenHash, expiresAt)
	require.NoError(t, err)
	assert.NotEmpty(t, id)

	// Validate should return the user ID and consume the token.
	userID, err := s.ValidateRefreshTokenByHash(ctx, tokenHash)
	require.NoError(t, err)
	assert.Equal(t, u.ID, userID)

	// Second validation should fail (single-use rotation).
	_, err = s.ValidateRefreshTokenByHash(ctx, tokenHash)
	assert.Error(t, err)
}

func TestValidateRefreshTokenExpired(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	u := &platauth.User{Email: "expired@example.com", Name: "Expired"}
	require.NoError(t, s.CreateUser(ctx, u))

	tokenHash := "expiredhash"
	expiresAt := time.Now().Add(-1 * time.Hour) // already expired

	_, err := s.StoreRefreshToken(ctx, u.ID, tokenHash, expiresAt)
	require.NoError(t, err)

	_, err = s.ValidateRefreshTokenByHash(ctx, tokenHash)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestRevokeRefreshToken(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	u := &platauth.User{Email: "revoke@example.com", Name: "Revoke"}
	require.NoError(t, s.CreateUser(ctx, u))

	tokenHash := "revokehash"
	expiresAt := time.Now().Add(30 * 24 * time.Hour)

	id, err := s.StoreRefreshToken(ctx, u.ID, tokenHash, expiresAt)
	require.NoError(t, err)

	require.NoError(t, s.RevokeRefreshToken(ctx, id))

	// Should no longer be valid.
	_, err = s.ValidateRefreshTokenByHash(ctx, tokenHash)
	assert.Error(t, err)
}

func TestRevokeUserRefreshTokens(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	u := &platauth.User{Email: "revokeall@example.com", Name: "RevokeAll"}
	require.NoError(t, s.CreateUser(ctx, u))

	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	_, err := s.StoreRefreshToken(ctx, u.ID, "hash1", expiresAt)
	require.NoError(t, err)
	_, err = s.StoreRefreshToken(ctx, u.ID, "hash2", expiresAt)
	require.NoError(t, err)

	require.NoError(t, s.RevokeUserRefreshTokens(ctx, u.ID))

	// Both tokens should be revoked.
	_, err = s.ValidateRefreshTokenByHash(ctx, "hash1")
	require.Error(t, err)
	_, err = s.ValidateRefreshTokenByHash(ctx, "hash2")
	require.Error(t, err)
}

func TestDeleteUserCascadesRefreshTokens(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	u := &platauth.User{Email: "cascade-rt@example.com", Name: "CascadeRT"}
	require.NoError(t, s.CreateUser(ctx, u))

	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	_, err := s.StoreRefreshToken(ctx, u.ID, "cascadehash", expiresAt)
	require.NoError(t, err)

	// Deleting user via raw SQL (since there's no DeleteUser method).
	_, err = s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, u.ID)
	require.NoError(t, err)

	// Refresh token should be cascaded.
	_, err = s.ValidateRefreshTokenByHash(ctx, "cascadehash")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// API Tokens
// ---------------------------------------------------------------------------

func makeTokenHash(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:])
}

func TestCreateAndGetAPIToken(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	u := &platauth.User{Email: "token@example.com", Name: "Token User"}
	require.NoError(t, s.CreateUser(ctx, u))
	w := &platauth.Workspace{Name: "Token WS", Slug: "token-ws"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	plaintext := "bwt_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	tokenHash := makeTokenHash(plaintext)

	tok := &platauth.APIToken{
		UserID:      u.ID,
		WorkspaceID: w.ID,
		Name:        "CI Token",
		TokenPrefix: plaintext[:8],
		Scopes:      `["*"]`,
	}
	require.NoError(t, s.CreateAPIToken(ctx, tok, tokenHash))
	assert.NotEmpty(t, tok.ID)
	assert.False(t, tok.CreatedAt.IsZero())

	got, err := s.GetAPITokenByHash(ctx, tokenHash)
	require.NoError(t, err)
	assert.Equal(t, tok.ID, got.ID)
	assert.Equal(t, u.ID, got.UserID)
	assert.Equal(t, w.ID, got.WorkspaceID)
	assert.Equal(t, "CI Token", got.Name)
	assert.Equal(t, "bwt_0123", got.TokenPrefix)
	assert.Equal(t, `["*"]`, got.Scopes)
	assert.Nil(t, got.LastUsedAt)
	assert.Nil(t, got.ExpiresAt)
}

func TestListAPITokens(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	u := &platauth.User{Email: "list-tok@example.com", Name: "List"}
	require.NoError(t, s.CreateUser(ctx, u))
	w := &platauth.Workspace{Name: "List WS", Slug: "list-ws"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	tok1 := &platauth.APIToken{UserID: u.ID, WorkspaceID: w.ID, Name: "Token 1", TokenPrefix: "bwt_0001"}
	tok2 := &platauth.APIToken{UserID: u.ID, WorkspaceID: w.ID, Name: "Token 2", TokenPrefix: "bwt_0002"}
	require.NoError(t, s.CreateAPIToken(ctx, tok1, "hash1"))
	require.NoError(t, s.CreateAPIToken(ctx, tok2, "hash2"))

	tokens, err := s.ListAPITokens(ctx, w.ID)
	require.NoError(t, err)
	assert.Len(t, tokens, 2)
}

func TestDeleteAPIToken(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	u := &platauth.User{Email: "del-tok@example.com", Name: "Del"}
	require.NoError(t, s.CreateUser(ctx, u))
	w := &platauth.Workspace{Name: "Del WS", Slug: "del-ws"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	tok := &platauth.APIToken{UserID: u.ID, WorkspaceID: w.ID, Name: "Temp", TokenPrefix: "bwt_temp"}
	require.NoError(t, s.CreateAPIToken(ctx, tok, "temphash"))

	require.NoError(t, s.DeleteAPIToken(ctx, tok.ID))

	_, err := s.GetAPITokenByHash(ctx, "temphash")
	assert.Error(t, err)
}

func TestDeleteAPITokenNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	err := s.DeleteAPIToken(ctx, "nonexistent-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestUpdateAPITokenLastUsed(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	u := &platauth.User{Email: "lastused@example.com", Name: "LastUsed"}
	require.NoError(t, s.CreateUser(ctx, u))
	w := &platauth.Workspace{Name: "LU WS", Slug: "lu-ws"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	tok := &platauth.APIToken{UserID: u.ID, WorkspaceID: w.ID, Name: "LU Token", TokenPrefix: "bwt_lu00"}
	require.NoError(t, s.CreateAPIToken(ctx, tok, "luhash"))

	require.NoError(t, s.UpdateAPITokenLastUsed(ctx, tok.ID))

	got, err := s.GetAPITokenByHash(ctx, "luhash")
	require.NoError(t, err)
	assert.NotNil(t, got.LastUsedAt)
}

func TestAPITokenWithExpiration(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	u := &platauth.User{Email: "exp-tok@example.com", Name: "Exp"}
	require.NoError(t, s.CreateUser(ctx, u))
	w := &platauth.Workspace{Name: "Exp WS", Slug: "exp-ws"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	expiry := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)
	tok := &platauth.APIToken{
		UserID:      u.ID,
		WorkspaceID: w.ID,
		Name:        "Expiring Token",
		TokenPrefix: "bwt_exp0",
		ExpiresAt:   &expiry,
	}
	require.NoError(t, s.CreateAPIToken(ctx, tok, "exphash"))

	got, err := s.GetAPITokenByHash(ctx, "exphash")
	require.NoError(t, err)
	require.NotNil(t, got.ExpiresAt)
	assert.WithinDuration(t, expiry, *got.ExpiresAt, time.Second)
}

func TestDeleteUserCascadesAPITokens(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	u := &platauth.User{Email: "cascade-at@example.com", Name: "CascadeAT"}
	require.NoError(t, s.CreateUser(ctx, u))
	w := &platauth.Workspace{Name: "Cascade AT WS", Slug: "cascade-at-ws"}
	require.NoError(t, s.CreateWorkspace(ctx, w))
	require.NoError(t, s.AddMember(ctx, w.ID, u.ID, platauth.RoleMember))

	tok := &platauth.APIToken{UserID: u.ID, WorkspaceID: w.ID, Name: "Cascade", TokenPrefix: "bwt_casc"}
	require.NoError(t, s.CreateAPIToken(ctx, tok, "cascadehash_at"))

	// Delete user via raw SQL.
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, u.ID)
	require.NoError(t, err)

	// Token should be cascaded.
	_, err = s.GetAPITokenByHash(ctx, "cascadehash_at")
	assert.Error(t, err)
}

func TestDeleteWorkspaceCascadesAPITokens(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	u := &platauth.User{Email: "cascade-ws@example.com", Name: "CascadeWS"}
	require.NoError(t, s.CreateUser(ctx, u))
	w := &platauth.Workspace{Name: "Cascade WS", Slug: "cascade-ws-at"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	tok := &platauth.APIToken{UserID: u.ID, WorkspaceID: w.ID, Name: "WSCascade", TokenPrefix: "bwt_wcas"}
	require.NoError(t, s.CreateAPIToken(ctx, tok, "cascadehash_ws"))

	require.NoError(t, s.DeleteWorkspace(ctx, w.ID))

	// Token should be cascaded.
	_, err := s.GetAPITokenByHash(ctx, "cascadehash_ws")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// Role Templates
// ---------------------------------------------------------------------------

func setupWorkspaceWithRoles(t *testing.T, s *SQLiteAuthStore) (workspaceID string, roles []*platauth.RoleTemplate) {
	t.Helper()
	ctx := t.Context()

	w := &platauth.Workspace{Name: "Roles WS", Slug: "roles-ws"}
	require.NoError(t, s.CreateWorkspace(ctx, w))
	require.NoError(t, s.SeedDefaultRoleTemplates(ctx, w.ID))

	templates, err := s.ListRoleTemplates(ctx, w.ID)
	require.NoError(t, err)
	return w.ID, templates
}

func TestRoleTemplateCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	w := &platauth.Workspace{Name: "RT CRUD", Slug: "rt-crud"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	// Create with all fields, check ID is auto-generated.
	rt := &platauth.RoleTemplate{
		WorkspaceID: w.ID,
		Name:        "custom-role",
		DisplayName: "Custom Role",
		Description: "A custom role for testing",
		Permissions: platauth.PermViewContent | platauth.PermTranslate,
		IsBuiltin:   false,
		Position:    10,
	}
	require.NoError(t, s.CreateRoleTemplate(ctx, rt))
	assert.NotEmpty(t, rt.ID, "ID should be auto-generated")
	assert.False(t, rt.CreatedAt.IsZero())
	assert.False(t, rt.UpdatedAt.IsZero())

	// Get by workspace_id + role_id.
	got, err := s.GetRoleTemplate(ctx, w.ID, rt.ID)
	require.NoError(t, err)
	assert.Equal(t, rt.ID, got.ID)
	assert.Equal(t, w.ID, got.WorkspaceID)
	assert.Equal(t, "custom-role", got.Name)
	assert.Equal(t, "Custom Role", got.DisplayName)
	assert.Equal(t, "A custom role for testing", got.Description)
	assert.Equal(t, platauth.PermViewContent|platauth.PermTranslate, got.Permissions)
	assert.False(t, got.IsBuiltin)
	assert.Equal(t, 10, got.Position)

	// Create a second template to verify list ordering.
	rt2 := &platauth.RoleTemplate{
		WorkspaceID: w.ID,
		Name:        "earlier-role",
		DisplayName: "Earlier Role",
		Permissions: platauth.PermViewContent,
		Position:    5,
	}
	require.NoError(t, s.CreateRoleTemplate(ctx, rt2))

	// List returns ordered by position.
	list, err := s.ListRoleTemplates(ctx, w.ID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "earlier-role", list[0].Name, "position 5 should come first")
	assert.Equal(t, "custom-role", list[1].Name, "position 10 should come second")

	// Update name, permissions, position.
	rt.Name = "updated-role"
	rt.DisplayName = "Updated Role"
	rt.Permissions = platauth.PermViewContent | platauth.PermTranslate | platauth.PermReview
	rt.Position = 20
	require.NoError(t, s.UpdateRoleTemplate(ctx, rt))

	got, err = s.GetRoleTemplate(ctx, w.ID, rt.ID)
	require.NoError(t, err)
	assert.Equal(t, "updated-role", got.Name)
	assert.Equal(t, "Updated Role", got.DisplayName)
	assert.Equal(t, platauth.PermViewContent|platauth.PermTranslate|platauth.PermReview, got.Permissions)
	assert.Equal(t, 20, got.Position)

	// Delete non-builtin succeeds.
	require.NoError(t, s.DeleteRoleTemplate(ctx, w.ID, rt.ID))
	_, err = s.GetRoleTemplate(ctx, w.ID, rt.ID)
	require.Error(t, err)

	// Delete builtin fails — seed defaults first.
	require.NoError(t, s.SeedDefaultRoleTemplates(ctx, w.ID))
	templates, err := s.ListRoleTemplates(ctx, w.ID)
	require.NoError(t, err)
	var builtinID string
	for _, tmpl := range templates {
		if tmpl.IsBuiltin {
			builtinID = tmpl.ID
			break
		}
	}
	require.NotEmpty(t, builtinID, "should have a builtin template")
	err = s.DeleteRoleTemplate(ctx, w.ID, builtinID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "builtin")
}

func TestSeedDefaultRoleTemplates(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	w := &platauth.Workspace{Name: "Seed WS", Slug: "seed-ws"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	require.NoError(t, s.SeedDefaultRoleTemplates(ctx, w.ID))

	templates, err := s.ListRoleTemplates(ctx, w.ID)
	require.NoError(t, err)
	require.Len(t, templates, 5, "should seed 5 default role templates")

	// Verify correct names in position order.
	expectedNames := []string{"project-admin", "developer", "translator", "reviewer", "observer"}
	for i, tmpl := range templates {
		assert.Equal(t, expectedNames[i], tmpl.Name)
		assert.True(t, tmpl.IsBuiltin)
		assert.Equal(t, w.ID, tmpl.WorkspaceID)
		assert.NotEmpty(t, tmpl.ID)
		assert.NotEmpty(t, tmpl.DisplayName)
	}

	// Verify specific permissions.
	assert.Equal(t, platauth.PermAll, templates[0].Permissions, "project-admin should have PermAll")
	assert.True(t, templates[1].Permissions.Has(platauth.PermViewContent|platauth.PermTranslate|platauth.PermManageFiles), "developer should have view+translate+manage_files")
	assert.Equal(t, platauth.PermViewContent|platauth.PermTranslate, templates[2].Permissions, "translator should have view+translate")
	assert.Equal(t, platauth.PermViewContent|platauth.PermTranslate|platauth.PermReview, templates[3].Permissions, "reviewer should have view+translate+review")
	assert.Equal(t, platauth.PermViewContent, templates[4].Permissions, "observer should have view only")
}

// ---------------------------------------------------------------------------
// Project Membership
// ---------------------------------------------------------------------------

func TestProjectMemberCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	wsID, roles := setupWorkspaceWithRoles(t, s)

	u := &platauth.User{Email: "pm-user@example.com", Name: "PM User"}
	require.NoError(t, s.CreateUser(ctx, u))

	// Find the translator role.
	var translatorRole *platauth.RoleTemplate
	for _, r := range roles {
		if r.Name == "translator" {
			translatorRole = r
			break
		}
	}
	require.NotNil(t, translatorRole)

	projectID := "proj-1"

	// Add member with role_id and languages.
	pm := &platauth.ProjectMembership{
		ProjectID:   projectID,
		UserID:      u.ID,
		RoleID:      translatorRole.ID,
		WorkspaceID: wsID,
		Languages:   []string{"fr", "de"},
	}
	require.NoError(t, s.AddProjectMember(ctx, pm))
	assert.False(t, pm.CreatedAt.IsZero())

	// Get membership returns correct fields.
	got, err := s.GetProjectMembership(ctx, projectID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, projectID, got.ProjectID)
	assert.Equal(t, u.ID, got.UserID)
	assert.Equal(t, translatorRole.ID, got.RoleID)
	assert.Equal(t, wsID, got.WorkspaceID)
	assert.Equal(t, []string{"fr", "de"}, got.Languages)

	// List returns all members.
	members, err := s.ListProjectMembers(ctx, projectID)
	require.NoError(t, err)
	require.Len(t, members, 1)
	assert.Equal(t, u.ID, members[0].UserID)

	// Update role_id and languages.
	var reviewerRole *platauth.RoleTemplate
	for _, r := range roles {
		if r.Name == "reviewer" {
			reviewerRole = r
			break
		}
	}
	require.NotNil(t, reviewerRole)

	pm.RoleID = reviewerRole.ID
	pm.Languages = []string{"es"}
	require.NoError(t, s.UpdateProjectMember(ctx, pm))

	got, err = s.GetProjectMembership(ctx, projectID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, reviewerRole.ID, got.RoleID)
	assert.Equal(t, []string{"es"}, got.Languages)

	// Remove member succeeds.
	require.NoError(t, s.RemoveProjectMember(ctx, projectID, u.ID))
	_, err = s.GetProjectMembership(ctx, projectID, u.ID)
	require.Error(t, err)

	// Remove non-existent fails.
	err = s.RemoveProjectMember(ctx, projectID, "nonexistent-user")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolveProjectPermissions(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	wsID, roles := setupWorkspaceWithRoles(t, s)

	u := &platauth.User{Email: "resolve@example.com", Name: "Resolve User"}
	require.NoError(t, s.CreateUser(ctx, u))

	// Find the translator role.
	var translatorRole *platauth.RoleTemplate
	for _, r := range roles {
		if r.Name == "translator" {
			translatorRole = r
			break
		}
	}
	require.NotNil(t, translatorRole)

	projectID := "proj-resolve"

	// Add project member with translator role and specific languages.
	pm := &platauth.ProjectMembership{
		ProjectID:   projectID,
		UserID:      u.ID,
		RoleID:      translatorRole.ID,
		WorkspaceID: wsID,
		Languages:   []string{"fr", "de"},
	}
	require.NoError(t, s.AddProjectMember(ctx, pm))

	// Resolve permissions — verify bitmask matches translator permissions.
	resolved, err := s.ResolveProjectPermissions(ctx, projectID, u.ID)
	require.NoError(t, err)
	assert.Equal(t, platauth.PermViewContent|platauth.PermTranslate, resolved.Permissions)

	// Verify languages are returned correctly.
	assert.Equal(t, []string{"fr", "de"}, resolved.Languages)

	// Verify non-member returns error.
	_, err = s.ResolveProjectPermissions(ctx, projectID, "nonexistent-user")
	assert.Error(t, err)
}

func TestProjectMemberLanguages(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	wsID, roles := setupWorkspaceWithRoles(t, s)

	u := &platauth.User{Email: "langs@example.com", Name: "Langs User"}
	require.NoError(t, s.CreateUser(ctx, u))

	var translatorRole *platauth.RoleTemplate
	for _, r := range roles {
		if r.Name == "translator" {
			translatorRole = r
			break
		}
	}
	require.NotNil(t, translatorRole)

	// Empty languages → nil on read.
	pm := &platauth.ProjectMembership{
		ProjectID:   "proj-lang-empty",
		UserID:      u.ID,
		RoleID:      translatorRole.ID,
		WorkspaceID: wsID,
		Languages:   nil,
	}
	require.NoError(t, s.AddProjectMember(ctx, pm))

	got, err := s.GetProjectMembership(ctx, "proj-lang-empty", u.ID)
	require.NoError(t, err)
	assert.Nil(t, got.Languages, "empty languages should be nil on read")

	// ["fr","de"] → correct slice on read.
	pm2 := &platauth.ProjectMembership{
		ProjectID:   "proj-lang-set",
		UserID:      u.ID,
		RoleID:      translatorRole.ID,
		WorkspaceID: wsID,
		Languages:   []string{"fr", "de"},
	}
	require.NoError(t, s.AddProjectMember(ctx, pm2))

	got2, err := s.GetProjectMembership(ctx, "proj-lang-set", u.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"fr", "de"}, got2.Languages)
}
