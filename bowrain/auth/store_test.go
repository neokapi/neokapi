package auth

import (
	"context"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPostgresAuthStore_ImplementsInterface is a compile-time check that
// PostgresAuthStore satisfies the AuthStore interface.
func TestPostgresAuthStore_ImplementsInterface(t *testing.T) {
	var _ AuthStore = (*PostgresAuthStore)(nil)
}

// newTestStore returns a migrated AuthStore backed by an isolated PostgreSQL
// schema. Skips if Docker (testcontainers) is unavailable.
func newTestStore(t *testing.T) *PostgresAuthStore {
	t.Helper()
	db := pgtest.NewTestDB(t)
	s, err := NewAuthStoreFromDB(db)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// seedWorkspace inserts a minimal workspace so slug-uniqueness queries that
// UNION against the workspaces table have rows to find, and so FK-bearing
// child tables (slug reservations, sod settings, role overrides) have a parent.
func seedWorkspace(t *testing.T, s *PostgresAuthStore, id, slug string) {
	t.Helper()
	err := s.CreateWorkspace(context.Background(), &platauth.Workspace{
		ID:   id,
		Name: "WS " + slug,
		Slug: slug,
	})
	require.NoError(t, err)
}

// seedUser inserts a minimal user so FK-bearing child tables (email-change
// requests) have a parent row.
func seedUser(t *testing.T, s *PostgresAuthStore, id, email string) {
	t.Helper()
	err := s.CreateUser(context.Background(), &platauth.User{
		ID:    id,
		Email: email,
		Name:  "User " + id,
	})
	require.NoError(t, err)
}

func TestSlugReservation_ReserveAndCheck(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedWorkspace(t, s, "ws-1", "ws-1-slug")
	until := time.Now().Add(SlugReservationWindow)
	require.NoError(t, s.ReserveSlug(ctx, "ws-1", "old-slug", until))

	wsID, reservedUntil, ok, err := s.IsSlugReserved(ctx, "old-slug")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "ws-1", wsID)
	assert.WithinDuration(t, until, reservedUntil, time.Second)
}

func TestSlugReservation_NotReservedWhenAbsent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, _, ok, err := s.IsSlugReserved(ctx, "never-reserved")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestSlugReservation_ExpiredIsNotReserved(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Reserve in the past — the WHERE reserved_until > NOW() clause must treat
	// it as not reserved.
	seedWorkspace(t, s, "ws-1", "ws-1-slug")
	require.NoError(t, s.ReserveSlug(ctx, "ws-1", "stale", time.Now().Add(-time.Hour)))

	_, _, ok, err := s.IsSlugReserved(ctx, "stale")
	require.NoError(t, err)
	assert.False(t, ok, "expired reservation must not count as reserved")
}

func TestSlugReservation_Validation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.Error(t, s.ReserveSlug(ctx, "ws-1", "", time.Now().Add(time.Hour)))
	require.Error(t, s.ReserveSlug(ctx, "", "slug", time.Now().Add(time.Hour)))
}

func TestSlugReservation_OverwriteMostRecentWins(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedWorkspace(t, s, "ws-1", "ws-1-slug")
	seedWorkspace(t, s, "ws-2", "ws-2-slug")
	until := time.Now().Add(time.Hour)
	require.NoError(t, s.ReserveSlug(ctx, "ws-1", "shared", until))
	// Same slug, different workspace — ON CONFLICT DO UPDATE replaces owner.
	require.NoError(t, s.ReserveSlug(ctx, "ws-2", "shared", until))

	wsID, _, ok, err := s.IsSlugReserved(ctx, "shared")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "ws-2", wsID, "most recent reservation must win")
}

func TestSlugReservation_Purge(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedWorkspace(t, s, "ws-1", "ws-1-slug")
	seedWorkspace(t, s, "ws-2", "ws-2-slug")
	seedWorkspace(t, s, "ws-3", "ws-3-slug")
	require.NoError(t, s.ReserveSlug(ctx, "ws-1", "active", time.Now().Add(time.Hour)))
	require.NoError(t, s.ReserveSlug(ctx, "ws-2", "expired1", time.Now().Add(-time.Hour)))
	require.NoError(t, s.ReserveSlug(ctx, "ws-3", "expired2", time.Now().Add(-2*time.Hour)))

	n, err := s.PurgeExpiredSlugReservations(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	// The active one survives.
	_, _, ok, err := s.IsSlugReserved(ctx, "active")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestSlugReservation_List(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedWorkspace(t, s, "ws-1", "ws-1-slug")
	seedWorkspace(t, s, "ws-2", "ws-2-slug")
	seedWorkspace(t, s, "ws-3", "ws-3-slug")
	require.NoError(t, s.ReserveSlug(ctx, "ws-1", "a", time.Now().Add(time.Hour)))
	require.NoError(t, s.ReserveSlug(ctx, "ws-2", "b", time.Now().Add(2*time.Hour)))
	require.NoError(t, s.ReserveSlug(ctx, "ws-3", "expired", time.Now().Add(-time.Hour)))

	list, err := s.ListSlugReservations(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2, "expired reservations must be excluded")

	slugs := map[string]bool{}
	for _, r := range list {
		slugs[r.Slug] = true
	}
	assert.True(t, slugs["a"])
	assert.True(t, slugs["b"])
	assert.False(t, slugs["expired"])
}

func TestSlugReservation_Release(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedWorkspace(t, s, "ws-1", "ws-1-slug")
	require.NoError(t, s.ReserveSlug(ctx, "ws-1", "temp", time.Now().Add(time.Hour)))
	require.NoError(t, s.ReleaseSlugReservation(ctx, "temp"))

	_, _, ok, err := s.IsSlugReserved(ctx, "temp")
	require.NoError(t, err)
	assert.False(t, ok)

	// Releasing a non-existent reservation is an error (admin override expects a row).
	err = s.ReleaseSlugReservation(ctx, "temp")
	require.Error(t, err)
}

func TestListTakenSlugs_EmptyInput(t *testing.T) {
	s := newTestStore(t)
	taken, err := s.ListTakenSlugs(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, taken)
}

func TestListTakenSlugs_ActiveWorkspaceSlug(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedWorkspace(t, s, "ws-1", "acme")

	taken, err := s.ListTakenSlugs(ctx, []string{"acme", "free-one", "free-two"})
	require.NoError(t, err)
	assert.True(t, taken["acme"], "an active workspace slug is taken")
	assert.False(t, taken["free-one"])
	assert.False(t, taken["free-two"])
}

func TestListTakenSlugs_ReservedSlug(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedWorkspace(t, s, "ws-1", "ws-1-slug")
	seedWorkspace(t, s, "ws-2", "ws-2-slug")
	require.NoError(t, s.ReserveSlug(ctx, "ws-1", "reserved-slug", time.Now().Add(time.Hour)))
	require.NoError(t, s.ReserveSlug(ctx, "ws-2", "expired-slug", time.Now().Add(-time.Hour)))

	taken, err := s.ListTakenSlugs(ctx, []string{"reserved-slug", "expired-slug", "free"})
	require.NoError(t, err)
	assert.True(t, taken["reserved-slug"], "a live reservation makes a slug taken")
	assert.False(t, taken["expired-slug"], "an expired reservation does not")
	assert.False(t, taken["free"])
}

func TestListTakenSlugs_UnionOfWorkspaceAndReservation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedWorkspace(t, s, "ws-1", "live-ws")
	seedWorkspace(t, s, "ws-2", "ws-2-slug")
	require.NoError(t, s.ReserveSlug(ctx, "ws-2", "held", time.Now().Add(time.Hour)))

	taken, err := s.ListTakenSlugs(ctx, []string{"live-ws", "held", "open"})
	require.NoError(t, err)
	assert.True(t, taken["live-ws"])
	assert.True(t, taken["held"])
	assert.False(t, taken["open"])
}

func TestCreateWorkspace_SlugUniqueness(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.CreateWorkspace(ctx, &platauth.Workspace{ID: "ws-1", Name: "A", Slug: "dup"}))
	// Second workspace claiming the same slug must violate the UNIQUE constraint.
	err := s.CreateWorkspace(ctx, &platauth.Workspace{ID: "ws-2", Name: "B", Slug: "dup"})
	require.Error(t, err)
}

func TestGetWorkspaceBySlug_Roundtrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedWorkspace(t, s, "ws-1", "lookup-me")
	got, err := s.GetWorkspaceBySlug(ctx, "lookup-me")
	require.NoError(t, err)
	assert.Equal(t, "ws-1", got.ID)
	assert.Equal(t, "lookup-me", got.Slug)
}

func TestEmailChangeRequest_CreateAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedUser(t, s, "user-1", "old@example.com")
	req := &platauth.EmailChangeRequest{
		UserID:   "user-1",
		NewEmail: "new@example.com",
	}
	require.NoError(t, s.CreateEmailChangeRequest(ctx, req, "hash-abc"))
	assert.NotEmpty(t, req.ID, "ID is generated when empty")
	assert.False(t, req.ExpiresAt.IsZero(), "ExpiresAt defaults to created+TTL")
	assert.WithinDuration(t, req.CreatedAt.Add(EmailChangeTokenTTL), req.ExpiresAt, time.Second)

	got, err := s.GetEmailChangeRequestByToken(ctx, "hash-abc")
	require.NoError(t, err)
	assert.Equal(t, req.ID, got.ID)
	assert.Equal(t, "user-1", got.UserID)
	assert.Equal(t, "new@example.com", got.NewEmail)
}

func TestEmailChangeRequest_Validation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.Error(t, s.CreateEmailChangeRequest(ctx, &platauth.EmailChangeRequest{NewEmail: "x@y.z"}, "h"))
	require.Error(t, s.CreateEmailChangeRequest(ctx, &platauth.EmailChangeRequest{UserID: "u"}, "h"))
	require.Error(t, s.CreateEmailChangeRequest(ctx, &platauth.EmailChangeRequest{UserID: "u", NewEmail: "x@y.z"}, ""))
}

func TestEmailChangeRequest_DeleteForUser(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedUser(t, s, "user-1", "old@example.com")
	require.NoError(t, s.CreateEmailChangeRequest(ctx, &platauth.EmailChangeRequest{UserID: "user-1", NewEmail: "a@x.z"}, "h1"))
	require.NoError(t, s.CreateEmailChangeRequest(ctx, &platauth.EmailChangeRequest{UserID: "user-1", NewEmail: "b@x.z"}, "h2"))
	require.NoError(t, s.DeleteEmailChangeRequestsForUser(ctx, "user-1"))

	_, err := s.GetEmailChangeRequestByToken(ctx, "h1")
	require.Error(t, err)
	_, err = s.GetEmailChangeRequestByToken(ctx, "h2")
	require.Error(t, err)
}

func TestEmailChangeRequest_PurgeExpired(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedUser(t, s, "user-1", "u1@example.com")
	seedUser(t, s, "user-2", "u2@example.com")
	// One in the past (manually set ExpiresAt), one in the future.
	past := &platauth.EmailChangeRequest{
		UserID:    "user-1",
		NewEmail:  "a@x.z",
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	require.NoError(t, s.CreateEmailChangeRequest(ctx, past, "h-past"))
	require.NoError(t, s.CreateEmailChangeRequest(ctx, &platauth.EmailChangeRequest{UserID: "user-2", NewEmail: "b@x.z"}, "h-future"))

	n, err := s.PurgeExpiredEmailChangeRequests(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	_, err = s.GetEmailChangeRequestByToken(ctx, "h-future")
	require.NoError(t, err, "unexpired request survives purge")
}

func TestSoDMode_DefaultAndSet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedWorkspace(t, s, "ws-1", "ws-1-slug")
	// Default when unset is "warn".
	mode, err := s.GetSoDMode(ctx, "ws-1")
	require.NoError(t, err)
	assert.Equal(t, platauth.SoDWarn, mode)

	require.NoError(t, s.SetSoDMode(ctx, "ws-1", platauth.SoDBlock))
	mode, err = s.GetSoDMode(ctx, "ws-1")
	require.NoError(t, err)
	assert.Equal(t, platauth.SoDBlock, mode)

	// Upsert path: overwrite existing.
	require.NoError(t, s.SetSoDMode(ctx, "ws-1", platauth.SoDOff))
	mode, err = s.GetSoDMode(ctx, "ws-1")
	require.NoError(t, err)
	assert.Equal(t, platauth.SoDOff, mode)
}

func TestWorkspaceRoleOverride_GetSetList(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedWorkspace(t, s, "ws-1", "ws-1-slug")
	// No override configured yet.
	_, ok, err := s.GetWorkspaceRoleOverride(ctx, "ws-1", platauth.RoleMember)
	require.NoError(t, err)
	assert.False(t, ok)

	perms := platauth.PermViewContent | platauth.PermTranslate
	require.NoError(t, s.SetWorkspaceRoleOverride(ctx, "ws-1", platauth.RoleMember, perms))

	got, ok, err := s.GetWorkspaceRoleOverride(ctx, "ws-1", platauth.RoleMember)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, perms, got)

	// Upsert overwrites.
	newPerms := platauth.PermViewContent
	require.NoError(t, s.SetWorkspaceRoleOverride(ctx, "ws-1", platauth.RoleMember, newPerms))
	all, err := s.ListWorkspaceRoleOverrides(ctx, "ws-1")
	require.NoError(t, err)
	assert.Equal(t, newPerms, all[platauth.RoleMember])
}
