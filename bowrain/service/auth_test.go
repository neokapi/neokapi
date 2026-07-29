package service

import (
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

			err = svc.AcceptInvite(ctx, inv.Code, accepter.ID)

			membership, memErr := store.GetMembership(ctx, ws.ID, accepter.ID)
			if tt.wantJoined {
				require.NoError(t, err)
				require.NoError(t, memErr, "accepter should have joined the workspace")
				assert.Equal(t, platauth.RoleOwner, membership.Role)
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

	require.NoError(t, svc.AcceptInvite(ctx, inv.Code, invited.ID))
	require.NoError(t, svc.AcceptInvite(ctx, inv.Code, invited.ID))

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

	require.Error(t, svc.AcceptInvite(ctx, inv.Code, "no-such-user"))
}
