package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/platform/auth"
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
	ctx := context.Background()

	u := &platauth.User{Email: "bob@example.com", Name: "Bob"}
	require.NoError(t, s.CreateUser(ctx, u))

	got, err := s.GetUserByEmail(ctx, "bob@example.com")
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
}

func TestUpdateUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

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
	ctx := context.Background()

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
	ctx := context.Background()

	w := &platauth.Workspace{Name: "Acme Corp", Slug: "acme-corp"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	got, err := s.GetWorkspaceBySlug(ctx, "acme-corp")
	require.NoError(t, err)
	assert.Equal(t, w.ID, got.ID)
}

func TestUpdateWorkspace(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

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
	ctx := context.Background()

	w := &platauth.Workspace{Name: "Ephemeral", Slug: "ephemeral"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	require.NoError(t, s.DeleteWorkspace(ctx, w.ID))

	_, err := s.GetWorkspace(ctx, w.ID)
	assert.Error(t, err)
}

func TestMembership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

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
	ctx := context.Background()

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
	ctx := context.Background()

	u := &platauth.User{Email: "test@example.com", Name: "Test"}
	require.NoError(t, s.CreateUser(ctx, u))
	w := &platauth.Workspace{Name: "WS", Slug: "ws"}
	require.NoError(t, s.CreateWorkspace(ctx, w))

	err := s.AddMember(ctx, w.ID, u.ID, platauth.Role("superadmin"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid role")
}

func TestDeleteWorkspaceCascadesMemberships(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

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
	ctx := context.Background()

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
	ctx := context.Background()

	u := &platauth.User{Email: "expired@example.com", Name: "Expired"}
	require.NoError(t, s.CreateUser(ctx, u))

	tokenHash := "expiredhash"
	expiresAt := time.Now().Add(-1 * time.Hour) // already expired

	_, err := s.StoreRefreshToken(ctx, u.ID, tokenHash, expiresAt)
	require.NoError(t, err)

	_, err = s.ValidateRefreshTokenByHash(ctx, tokenHash)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestRevokeRefreshToken(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

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
	ctx := context.Background()

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
	assert.Error(t, err)
	_, err = s.ValidateRefreshTokenByHash(ctx, "hash2")
	assert.Error(t, err)
}

func TestDeleteUserCascadesRefreshTokens(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

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
	ctx := context.Background()

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
	ctx := context.Background()

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
	ctx := context.Background()

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
	ctx := context.Background()

	err := s.DeleteAPIToken(ctx, "nonexistent-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestUpdateAPITokenLastUsed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

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
	ctx := context.Background()

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
	ctx := context.Background()

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
	ctx := context.Background()

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
