package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/knowledge"
)

// These tests cover the machine author identity: what the loop pushes is
// authored by the machine, so the person whose workspace it is remains an
// eligible reviewer of it.

const govPerms = platauth.PermViewContent | platauth.PermManageTerms | platauth.PermManageBrand

// TestMachineAuthoredChangeSetIsReviewableByTheOwner is the founder's case: the
// loop pushed it, so the person whose workspace it is may review it — with
// separation of duties genuinely satisfied rather than waived.
func TestMachineAuthoredChangeSetIsReviewableByTheOwner(t *testing.T) {
	h := newKGHarness(t)
	ctx := context.Background()
	const owner = "owner-1"

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

	// The owner reviews it. The separation-of-duties gate is untouched — the
	// author is simply somebody else.
	c, rec = h.req(http.MethodPost, "/", `{"comment":"read it, ship it"}`, govPerms, "id", created.ID)
	withActor(c, owner)
	require.NoError(t, h.srv.HandleApproveChangeSet(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	reviews, err := h.fake.ListReviews(ctx, kgTestWS, created.ID)
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	assert.Equal(t, owner, reviews[0].Reviewer)

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

// TestAuthorIdentityFallsBackToTheUser keeps the ordinary path honest: a session
// or a personal token authors as the person, exactly as before.
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
