package service

import (
	"context"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAuthStore(t *testing.T) *auth.PostgresAuthStore {
	t.Helper()
	db := pgtest.NewTestDB(t)
	s, err := auth.NewAuthStoreFromDB(db)
	require.NoError(t, err)
	return s
}

// TestAcceptInviteEmailBinding pins the rule that decides what a held invite
// code is worth: an invite carrying an address admits only that address, while
// an invite with no address stays the deliberate bearer-style link invite.
func TestAcceptInviteEmailBinding(t *testing.T) {
	tests := []struct {
		name string
		// inviteEmail is the address stamped on the invite ("" = link invite).
		inviteEmail string
		// accepterEmail is the account email of the user redeeming the code.
		accepterEmail string
		wantJoined    bool
	}{
		{
			name:          "bound invite admits the invited address",
			inviteEmail:   "invited@example.com",
			accepterEmail: "invited@example.com",
			wantJoined:    true,
		},
		{
			name:          "bound invite admits the invited address case-insensitively",
			inviteEmail:   "Invited@Example.com",
			accepterEmail: "invited@example.com",
			wantJoined:    true,
		},
		{
			name:          "bound invite refuses a different address",
			inviteEmail:   "invited@example.com",
			accepterEmail: "someone-else@example.com",
			wantJoined:    false,
		},
		{
			name:          "bound invite refuses a lookalike address",
			inviteEmail:   "invited@example.com",
			accepterEmail: "invited@example.com.attacker.test",
			wantJoined:    false,
		},
		{
			name:          "link invite admits any authenticated user",
			inviteEmail:   "",
			accepterEmail: "anyone@example.com",
			wantJoined:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTestAuthStore(t)
			svc := NewAuthService(store, "test-secret")
			ctx := t.Context()

			owner, err := svc.GetOrCreateUser(ctx, "owner@example.com", "Owner", "", "sub-owner", "en")
			require.NoError(t, err)

			ws := &platauth.Workspace{Name: "Acme", Slug: "acme"}
			require.NoError(t, store.CreateWorkspace(ctx, ws))
			require.NoError(t, store.AddMember(ctx, ws.ID, owner.ID, platauth.RoleOwner))

			accepter, err := svc.GetOrCreateUser(ctx, tt.accepterEmail, "Accepter", "", "sub-accepter", "en")
			require.NoError(t, err)

			// The escalation that matters: the invite hands out ownership.
			inv, err := svc.CreateInvite(ctx, ws.ID, owner.ID, platauth.RoleOwner, tt.inviteEmail, 1, time.Hour)
			require.NoError(t, err)

			accepted, err := svc.AcceptInvite(ctx, inv.Code, accepter.ID)

			membership, memErr := store.GetMembership(ctx, ws.ID, accepter.ID)
			if tt.wantJoined {
				require.NoError(t, err)
				require.NoError(t, memErr, "accepter should have joined the workspace")
				assert.Equal(t, platauth.RoleOwner, membership.Role)
				// The acceptance names what was joined: the client confirms the
				// join and switches workspace from this alone.
				require.NotNil(t, accepted)
				assert.Equal(t, ws.ID, accepted.WorkspaceID)
				assert.Equal(t, "acme", accepted.WorkspaceSlug)
				assert.Equal(t, "Acme", accepted.WorkspaceName)
				assert.Equal(t, platauth.RoleOwner, accepted.Role)
				return
			}

			require.Error(t, err)
			require.Error(t, memErr, "accepter must not have joined the workspace")
			// The refusal must not disclose who the invite was addressed to —
			// the caller has only proved they hold the code.
			assert.NotContains(t, err.Error(), tt.inviteEmail)

			// A refused acceptance must not burn a use, or a bound invite could
			// be exhausted by anyone holding the link.
			after, err := store.GetInviteByCode(ctx, inv.Code)
			require.NoError(t, err)
			assert.Zero(t, after.UseCount)
		})
	}
}

// TestAcceptInviteReportsTheRoleActuallyHeld covers the idempotent path when
// the member's role has moved on from the invite's: the confirmation must say
// what they are, not what the invite once offered.
func TestAcceptInviteReportsTheRoleActuallyHeld(t *testing.T) {
	store := newTestAuthStore(t)
	svc := NewAuthService(store, "test-secret")
	ctx := t.Context()

	owner, err := svc.GetOrCreateUser(ctx, "owner@example.com", "Owner", "", "sub-owner", "en")
	require.NoError(t, err)

	ws := &platauth.Workspace{Name: "Acme", Slug: "acme"}
	require.NoError(t, store.CreateWorkspace(ctx, ws))
	require.NoError(t, store.AddMember(ctx, ws.ID, owner.ID, platauth.RoleOwner))

	invited, err := svc.GetOrCreateUser(ctx, "invited@example.com", "Invited", "", "sub-invited", "en")
	require.NoError(t, err)
	require.NoError(t, store.AddMember(ctx, ws.ID, invited.ID, platauth.RoleAdmin))

	inv, err := svc.CreateInvite(ctx, ws.ID, owner.ID, platauth.RoleMember, "invited@example.com", 1, time.Hour)
	require.NoError(t, err)

	accepted, err := svc.AcceptInvite(ctx, inv.Code, invited.ID)
	require.NoError(t, err)
	assert.Equal(t, platauth.RoleAdmin, accepted.Role)
}

// TestAcceptInviteIsIdempotentForTheInvitedUser guards the pre-existing
// "already a member" shortcut against the new identity gate: the invited user
// redeeming twice still succeeds without a second membership write.
func TestAcceptInviteIsIdempotentForTheInvitedUser(t *testing.T) {
	store := newTestAuthStore(t)
	svc := NewAuthService(store, "test-secret")
	ctx := t.Context()

	owner, err := svc.GetOrCreateUser(ctx, "owner@example.com", "Owner", "", "sub-owner", "en")
	require.NoError(t, err)

	ws := &platauth.Workspace{Name: "Acme", Slug: "acme"}
	require.NoError(t, store.CreateWorkspace(ctx, ws))
	require.NoError(t, store.AddMember(ctx, ws.ID, owner.ID, platauth.RoleOwner))

	invited, err := svc.GetOrCreateUser(ctx, "invited@example.com", "Invited", "", "sub-invited", "en")
	require.NoError(t, err)

	inv, err := svc.CreateInvite(ctx, ws.ID, owner.ID, platauth.RoleMember, "invited@example.com", 2, time.Hour)
	require.NoError(t, err)

	first, err := svc.AcceptInvite(ctx, inv.Code, invited.ID)
	require.NoError(t, err)
	second, err := svc.AcceptInvite(ctx, inv.Code, invited.ID)
	require.NoError(t, err)

	// The repeat still describes the workspace, so a re-opened invite link
	// confirms the membership rather than reporting an empty one.
	require.NotNil(t, second)
	assert.Equal(t, first.WorkspaceID, second.WorkspaceID)
	assert.Equal(t, "Acme", second.WorkspaceName)
	assert.Equal(t, platauth.RoleMember, second.Role)

	after, err := store.GetInviteByCode(ctx, inv.Code)
	require.NoError(t, err)
	assert.Equal(t, 1, after.UseCount, "the second acceptance is a no-op, not a second use")
}

// TestAcceptInviteRejectsUnknownUser covers the case where the caller's user
// record cannot be loaded: a bound invite must fail closed rather than fall
// through to the bearer path.
func TestAcceptInviteRejectsUnknownUser(t *testing.T) {
	store := newTestAuthStore(t)
	svc := NewAuthService(store, "test-secret")
	ctx := t.Context()

	owner, err := svc.GetOrCreateUser(ctx, "owner@example.com", "Owner", "", "sub-owner", "en")
	require.NoError(t, err)

	ws := &platauth.Workspace{Name: "Acme", Slug: "acme"}
	require.NoError(t, store.CreateWorkspace(ctx, ws))

	inv, err := svc.CreateInvite(ctx, ws.ID, owner.ID, platauth.RoleOwner, "invited@example.com", 1, time.Hour)
	require.NoError(t, err)

	_, err = svc.AcceptInvite(ctx, inv.Code, "no-such-user")
	require.Error(t, err)
}

// TestCompleteOnboardingReportsTheFirstTime pins the signal a once-per-account
// side effect hangs off: true exactly when this call turned an account into a
// workspace. Everything else — a repeat, an account that already had one — is
// false, and the marker is stamped either way.
func TestCompleteOnboardingReportsTheFirstTime(t *testing.T) {
	t.Run("first onboarding wins", func(t *testing.T) {
		store := newTestAuthStore(t)
		svc := NewAuthService(store, "test-secret")
		ctx := t.Context()

		u, err := svc.GetOrCreateUser(ctx, "dana@example.com", "Dana", "", "sub-dana", "en")
		require.NoError(t, err)

		ws, first, err := svc.CompleteOnboarding(ctx, u.ID, "dana", "Dana")
		require.NoError(t, err)
		assert.True(t, first, "the call that provisioned the workspace is the first")
		assert.Equal(t, platauth.WorkspaceTypePersonal, ws.Type)

		again, first, err := svc.CompleteOnboarding(ctx, u.ID, "dana", "Dana")
		require.NoError(t, err)
		assert.False(t, first, "onboarding is idempotent, and so is what hangs off it")
		assert.Equal(t, ws.ID, again.ID)
	})

	t.Run("an account that predates the marker is not first", func(t *testing.T) {
		store := newTestAuthStore(t)
		svc := NewAuthService(store, "test-secret")
		ctx := t.Context()

		u, err := svc.GetOrCreateUser(ctx, "otto@example.com", "Otto", "", "sub-otto", "en")
		require.NoError(t, err)
		// A personal workspace written straight to the store, as the accounts
		// created before onboarding existed have: onboarded_at still NULL.
		ws := &platauth.Workspace{Name: "Otto", Slug: "otto", Type: platauth.WorkspaceTypePersonal}
		require.NoError(t, store.CreateWorkspace(ctx, ws))
		require.NoError(t, store.AddMember(ctx, ws.ID, u.ID, platauth.RoleOwner))
		require.Nil(t, u.OnboardedAt)

		got, first, err := svc.CompleteOnboarding(ctx, u.ID, "otto-2", "Otto")
		require.NoError(t, err)
		assert.False(t, first, "the workspace was already there; the account is not new")
		assert.Equal(t, ws.ID, got.ID)

		after, err := store.GetUser(ctx, u.ID)
		require.NoError(t, err)
		assert.NotNil(t, after.OnboardedAt, "the marker is stamped regardless, so /welcome stops asking")
	})
}

// panickingTokenStore validates a token fine and then panics on the
// fire-and-forget bookkeeping write that follows it. Only the two methods
// ValidateAPIToken reaches are implemented; the embedded nil interface turns
// any other call into a visible failure rather than a silent one.
type panickingTokenStore struct {
	auth.AuthStore
	token  *platauth.APIToken
	called chan struct{}
}

func (s *panickingTokenStore) GetAPITokenByHash(context.Context, string) (*platauth.APIToken, error) {
	return s.token, nil
}

func (s *panickingTokenStore) UpdateAPITokenLastUsed(context.Context, string) error {
	close(s.called)
	panic("api token store is unreachable")
}

// The last-used write runs on a goroutine nothing above it can recover for, so
// a panicking store would take the whole server down over bookkeeping that has
// no bearing on the token it just validated.
func TestValidateAPITokenSurvivesPanickingLastUsedWrite(t *testing.T) {
	called := make(chan struct{})
	svc := &AuthService{store: &panickingTokenStore{
		token:  &platauth.APIToken{ID: "tok-1"},
		called: called,
	}}

	got, err := svc.ValidateAPIToken(t.Context(), "some-plaintext-token")
	require.NoError(t, err)
	assert.Equal(t, "tok-1", got.ID)

	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("last-used update never ran")
	}

	// Let the deferred recover run. Without it the process is already gone and
	// this test never reports a result at all.
	time.Sleep(100 * time.Millisecond)
}
