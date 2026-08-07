package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/knowledge"
)

// These tests cover the two governance rules that make a one-person workspace
// workable without pretending it has been independently reviewed: a machine
// author identity, so what the loop pushes is reviewable by the person whose
// workspace it is, and a solo-reviewer override that is permitted only while
// there is genuinely nobody else, and is recorded as such when it is used.

// ---------------------------------------------------------------------------
// Fake auth store
// ---------------------------------------------------------------------------

// fakeGovAuthStore implements the handful of auth.AuthStore methods the review
// policy consults. Everything else is left to the embedded nil interface, so a
// policy that grew a new dependency fails loudly here rather than reading a
// plausible zero value.
type fakeGovAuthStore struct {
	auth.AuthStore

	members   map[string][]*platauth.Membership
	overrides map[platauth.Role]platauth.Permission

	listErr error
}

func newFakeGovAuthStore() *fakeGovAuthStore {
	return &fakeGovAuthStore{
		members:   map[string][]*platauth.Membership{},
		overrides: map[platauth.Role]platauth.Permission{},
	}
}

func (f *fakeGovAuthStore) add(wsID, userID string, role platauth.Role) *fakeGovAuthStore {
	f.members[wsID] = append(f.members[wsID], &platauth.Membership{
		WorkspaceID: wsID, UserID: userID, Role: role,
	})
	return f
}

func (f *fakeGovAuthStore) ListMembers(_ context.Context, wsID string) ([]*platauth.Membership, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.members[wsID], nil
}

func (f *fakeGovAuthStore) GetMembership(_ context.Context, wsID, userID string) (*platauth.Membership, error) {
	for _, m := range f.members[wsID] {
		if m.UserID == userID {
			return m, nil
		}
	}
	return nil, assertNotFound
}

// Close is a no-op: the server closes its auth store on shutdown, and the
// embedded nil interface would panic.
func (f *fakeGovAuthStore) Close() error { return nil }

func (f *fakeGovAuthStore) GetWorkspaceRoleOverride(_ context.Context, _ string, role platauth.Role) (platauth.Permission, bool, error) {
	perms, ok := f.overrides[role]
	return perms, ok, nil
}

// assertNotFound is the error a missing membership returns; the policy only
// cares that it is non-nil.
var assertNotFound = errNotFoundForTest{}

type errNotFoundForTest struct{}

func (errNotFoundForTest) Error() string { return "membership not found" }

// ---------------------------------------------------------------------------
// The override rule
// ---------------------------------------------------------------------------

// TestSoloReviewOverride pins the exact shape of the exception: the owner, and
// only while there is nobody else who could review.
func TestSoloReviewOverride(t *testing.T) {
	const ws = "ws-solo"
	const author = "owner-1"
	cs := &knowledge.ChangeSet{ID: "cs1", WorkspaceID: ws, CreatedBy: author}

	tests := []struct {
		name  string
		store func() *fakeGovAuthStore
		actor string
		cs    *knowledge.ChangeSet
		want  bool
	}{
		{
			name:  "sole owner has nobody else to ask",
			store: func() *fakeGovAuthStore { return newFakeGovAuthStore().add(ws, author, platauth.RoleOwner) },
			actor: author,
			cs:    cs,
			want:  true,
		},
		{
			name: "a second admin ends the override immediately",
			store: func() *fakeGovAuthStore {
				return newFakeGovAuthStore().
					add(ws, author, platauth.RoleOwner).
					add(ws, "admin-1", platauth.RoleAdmin)
			},
			actor: author,
			cs:    cs,
			want:  false,
		},
		{
			name: "a member without review rights is not a reviewer",
			store: func() *fakeGovAuthStore {
				// RoleMember's defaults carry no manage_brand, so this
				// workspace still has exactly one person who could review.
				return newFakeGovAuthStore().
					add(ws, author, platauth.RoleOwner).
					add(ws, "member-1", platauth.RoleMember).
					add(ws, "viewer-1", platauth.RoleViewer)
			},
			actor: author,
			cs:    cs,
			want:  true,
		},
		{
			name: "a workspace override that grants members review rights ends it",
			store: func() *fakeGovAuthStore {
				f := newFakeGovAuthStore().
					add(ws, author, platauth.RoleOwner).
					add(ws, "member-1", platauth.RoleMember)
				f.overrides[platauth.RoleMember] = platauth.PermViewContent | platauth.PermManageBrand
				return f
			},
			actor: author,
			cs:    cs,
			want:  false,
		},
		{
			name: "an admin alone is not the owner",
			store: func() *fakeGovAuthStore {
				return newFakeGovAuthStore().add(ws, "admin-1", platauth.RoleAdmin)
			},
			actor: "admin-1",
			cs:    &knowledge.ChangeSet{ID: "cs2", WorkspaceID: ws, CreatedBy: "admin-1"},
			want:  false,
		},
		{
			name:  "reviewing someone else's work is not the override's business",
			store: func() *fakeGovAuthStore { return newFakeGovAuthStore().add(ws, author, platauth.RoleOwner) },
			actor: "someone-else",
			cs:    cs,
			want:  false,
		},
		{
			name: "an unreadable membership list fails closed",
			store: func() *fakeGovAuthStore {
				f := newFakeGovAuthStore().add(ws, author, platauth.RoleOwner)
				f.listErr = errNotFoundForTest{}
				return f
			},
			actor: author,
			cs:    cs,
			want:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))
			srv.AuthStore = tc.store()
			assert.Equal(t, tc.want, srv.soloReviewOverride(t.Context(), ws, tc.actor, tc.cs))
		})
	}

	t.Run("no auth store fails closed", func(t *testing.T) {
		srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))
		srv.AuthStore = nil
		assert.False(t, srv.soloReviewOverride(t.Context(), ws, author, cs))
	})
}

// ---------------------------------------------------------------------------
// The override through the handlers
// ---------------------------------------------------------------------------

const govPerms = platauth.PermViewContent | platauth.PermManageTerms | platauth.PermManageBrand

// TestSoloReviewerCanApproveOwnChangeSet proves the audited override end to end:
// the sole owner's verdict is accepted, the review row says it was a solo
// self-approval, and the merge gate — which refuses every other self-approval —
// accepts this one.
func TestSoloReviewerCanApproveOwnChangeSet(t *testing.T) {
	h := newKGHarness(t)
	ctx := context.Background()
	const owner = "owner-1"

	h.srv.AuthStore = newFakeGovAuthStore().add(kgTestWS, owner, platauth.RoleOwner)

	require.NoError(t, h.fake.CreateChangeSet(ctx, &knowledge.ChangeSet{
		ID: "cs-solo", WorkspaceID: kgTestWS, Name: "Ban legacy wording",
		Status: knowledge.ChangeSetInReview, CreatedBy: owner,
	}))

	c, rec := h.req(http.MethodPost, "/", `{"comment":"no one else to ask"}`, govPerms, "id", "cs-solo")
	withActor(c, owner)
	require.NoError(t, h.srv.HandleApproveChangeSet(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	updated, err := h.fake.GetChangeSet(ctx, kgTestWS, "cs-solo")
	require.NoError(t, err)
	assert.Equal(t, knowledge.ChangeSetApproved, updated.Status)

	reviews, err := h.fake.ListReviews(ctx, kgTestWS, "cs-solo")
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	assert.Equal(t, owner, reviews[0].Reviewer)
	assert.Equal(t, knowledge.VerdictApprove, reviews[0].Verdict)
	assert.Equal(t, knowledge.BasisSoloOwner, reviews[0].Basis,
		"a self-review that was allowed must record the rule that allowed it")

	// The merge gate reads the same marker, so the change-set is mergeable —
	// which is the point of allowing the review at all.
	assert.NoError(t, knowledge.CanMerge(*updated, true, []knowledge.ChangeSetReview{*reviews[0]}))
}

// TestSoloReviewerOverrideEndsWithASecondReviewer is the other half of the rule.
// The same owner, the same change-set, one more eligible member: refused.
func TestSoloReviewerOverrideEndsWithASecondReviewer(t *testing.T) {
	h := newKGHarness(t)
	ctx := context.Background()
	const owner = "owner-1"

	h.srv.AuthStore = newFakeGovAuthStore().
		add(kgTestWS, owner, platauth.RoleOwner).
		add(kgTestWS, "admin-1", platauth.RoleAdmin)

	require.NoError(t, h.fake.CreateChangeSet(ctx, &knowledge.ChangeSet{
		ID: "cs-not-solo", WorkspaceID: kgTestWS, Name: "Ban legacy wording",
		Status: knowledge.ChangeSetInReview, CreatedBy: owner,
	}))

	for _, verdict := range []struct {
		name   string
		handle func(echo.Context) error
	}{
		{"approve", h.srv.HandleApproveChangeSet},
		{"reject", h.srv.HandleRejectChangeSet},
	} {
		t.Run(verdict.name, func(t *testing.T) {
			c, rec := h.req(http.MethodPost, "/", "", govPerms, "id", "cs-not-solo")
			withActor(c, owner)
			require.NoError(t, verdict.handle(c))
			assert.Equal(t, http.StatusForbidden, rec.Code)
			assert.Contains(t, rec.Body.String(), "separation of duties")
		})
	}

	reviews, err := h.fake.ListReviews(ctx, kgTestWS, "cs-not-solo")
	require.NoError(t, err)
	assert.Empty(t, reviews, "a refused review must leave no record of having happened")
}

// TestHumanAuthorStillBlockedInMultiReviewerWorkspace is the case the override
// must never touch: an ordinary member wrote it, other reviewers exist, and the
// refusal is unchanged.
func TestHumanAuthorStillBlockedInMultiReviewerWorkspace(t *testing.T) {
	h := newKGHarness(t)
	ctx := context.Background()
	const author = "admin-1"

	h.srv.AuthStore = newFakeGovAuthStore().
		add(kgTestWS, "owner-1", platauth.RoleOwner).
		add(kgTestWS, author, platauth.RoleAdmin)

	require.NoError(t, h.fake.CreateChangeSet(ctx, &knowledge.ChangeSet{
		ID: "cs-human", WorkspaceID: kgTestWS, Status: knowledge.ChangeSetInReview, CreatedBy: author,
	}))

	c, rec := h.req(http.MethodPost, "/", "", govPerms, "id", "cs-human")
	withActor(c, author)
	require.NoError(t, h.srv.HandleApproveChangeSet(c))
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "separation of duties")
}

// ---------------------------------------------------------------------------
// Machine authorship
// ---------------------------------------------------------------------------

// TestMachineAuthoredChangeSetIsReviewableByTheOwner covers the one-person
// workspace: the loop pushed it, so the person whose workspace it is may review
// it — with no override, and no solo marker, because separation of duties is
// genuinely satisfied.
func TestMachineAuthoredChangeSetIsReviewableByTheOwner(t *testing.T) {
	h := newKGHarness(t)
	ctx := context.Background()
	const owner = "owner-1"

	// A one-person workspace: if the machine identity did not work, this owner
	// would have had to fall back to the audited override.
	h.srv.AuthStore = newFakeGovAuthStore().add(kgTestWS, owner, platauth.RoleOwner)

	// Draft the change-set the way a CI push does: authenticated as the owner's
	// token, but with the token's machine identity on the request.
	c, rec := h.req(http.MethodPost, "/", `{"name":"Nightly term sweep"}`, govPerms)
	withActor(c, owner)
	c.Set("author_identity", "agent/kapi-ci")
	require.NoError(t, h.srv.HandleCreateChangeSet(c))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var created knowledge.ChangeSet
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, "agent/kapi-ci", created.CreatedBy,
		"a push mediated by the loop is authored by the machine, not by the workspace's human")
	assert.NotEqual(t, owner, created.CreatedBy)

	require.NoError(t, h.fake.SetChangeSetStatus(ctx, kgTestWS, created.ID, knowledge.ChangeSetInReview))

	// The owner reviews it. No override is involved — the author is simply
	// somebody else.
	c, rec = h.req(http.MethodPost, "/", `{"comment":"read it, ship it"}`, govPerms, "id", created.ID)
	withActor(c, owner)
	require.NoError(t, h.srv.HandleApproveChangeSet(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	reviews, err := h.fake.ListReviews(ctx, kgTestWS, created.ID)
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	assert.Equal(t, owner, reviews[0].Reviewer)
	assert.Equal(t, knowledge.BasisPeer, reviews[0].Basis,
		"an ordinary review of machine-authored work is admitted on the peer basis")

	approved, err := h.fake.GetChangeSet(ctx, kgTestWS, created.ID)
	require.NoError(t, err)
	assert.NoError(t, knowledge.CanMerge(*approved, true, []knowledge.ChangeSetReview{*reviews[0]}))
}

// TestChangeSetOpsCarryTheMachineAuthor checks the op rows agree with the
// change-set header. An op attributed to the human would put the person's name
// on text they never wrote.
func TestChangeSetOpsCarryTheMachineAuthor(t *testing.T) {
	h := newKGHarness(t)
	ctx := context.Background()

	require.NoError(t, h.fake.CreateChangeSet(ctx, &knowledge.ChangeSet{
		ID: "cs-ops", WorkspaceID: kgTestWS, Status: knowledge.ChangeSetDraft, CreatedBy: "agent/kapi-ci",
	}))

	body := `{"op":"concept.create","payload":{"concept":{"id":"c1"}}}`
	c, rec := h.req(http.MethodPost, "/", body, govPerms, "id", "cs-ops")
	withActor(c, "owner-1")
	c.Set("author_identity", "agent/kapi-ci")
	require.NoError(t, h.srv.HandleAddChangeSetOp(c))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var op knowledge.ChangeSetOp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &op))
	assert.Equal(t, "agent/kapi-ci", op.CreatedBy)
}

// TestChangeSetDetailReportsTheSoloState checks the flag the review UI reads.
// Without it the UI would have to guess whether the approve button will be
// refused, and guessing is how a button that lies gets shipped.
func TestChangeSetDetailReportsTheSoloState(t *testing.T) {
	ctx := context.Background()

	seed := func(h *kgHarness) {
		require.NoError(t, h.fake.CreateChangeSet(ctx, &knowledge.ChangeSet{
			ID: "cs-detail", WorkspaceID: kgTestWS, Status: knowledge.ChangeSetInReview, CreatedBy: "owner-1",
		}))
	}

	t.Run("true for the sole owner viewing their own change-set", func(t *testing.T) {
		h := newKGHarness(t)
		h.srv.AuthStore = newFakeGovAuthStore().add(kgTestWS, "owner-1", platauth.RoleOwner)
		seed(h)

		c, rec := h.req(http.MethodGet, "/", "", govPerms, "id", "cs-detail")
		withActor(c, "owner-1")
		require.NoError(t, h.srv.HandleGetChangeSet(c))
		require.Equal(t, http.StatusOK, rec.Code)

		var resp ChangeSetDetailResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.True(t, resp.SoloReview)
	})

	t.Run("false once a second reviewer exists", func(t *testing.T) {
		h := newKGHarness(t)
		h.srv.AuthStore = newFakeGovAuthStore().
			add(kgTestWS, "owner-1", platauth.RoleOwner).
			add(kgTestWS, "admin-1", platauth.RoleAdmin)
		seed(h)

		c, rec := h.req(http.MethodGet, "/", "", govPerms, "id", "cs-detail")
		withActor(c, "owner-1")
		require.NoError(t, h.srv.HandleGetChangeSet(c))
		require.Equal(t, http.StatusOK, rec.Code)

		var resp ChangeSetDetailResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.False(t, resp.SoloReview)
	})
}

// TestAuthorIdentityFallsBackToTheUser keeps the ordinary path honest: a session
// or a personal token authors as the person.
func TestAuthorIdentityFallsBackToTheUser(t *testing.T) {
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))
	c := srv.GetEcho().NewContext(nil, nil)

	c.Set("user_id", "alice")
	assert.Equal(t, "alice", authorIdentity(c))
	assert.Empty(t, onBehalfOf(c), "a person acting for themselves is not acting on anyone's behalf")

	c.Set("author_identity", "agent/kapi-ci")
	assert.Equal(t, "agent/kapi-ci", authorIdentity(c))
	assert.Equal(t, "alice", onBehalfOf(c), "a machine's work stays traceable to whoever minted its token")
}
