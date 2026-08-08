package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/knowledge"
)

// These tests pin the one-open-proposal rule: an origin-carrying change-set is
// an automated proposal, and repeated pushes must converge to a single
// reviewable change-set — never a pile of identical drafts.

// draftWithOrigin creates a draft change-set carrying an origin and one
// governed deletion per named concept.
func (h *summonsHarness) draftWithOrigin(t *testing.T, name, origin string, conceptIDs ...string) string {
	t.Helper()
	ctx := context.Background()
	cs := &knowledge.ChangeSet{
		WorkspaceID: summonsWSID,
		Name:        name,
		Status:      knowledge.ChangeSetDraft,
		Origin:      origin,
		CreatedBy:   "author",
	}
	require.NoError(t, h.fake.CreateChangeSet(ctx, cs))
	for _, id := range conceptIDs {
		payload, err := json.Marshal(knowledge.ConceptDeletePayload{ConceptID: id})
		require.NoError(t, err)
		require.NoError(t, h.fake.AppendOp(ctx, &knowledge.ChangeSetOp{
			WorkspaceID: summonsWSID,
			ChangesetID: cs.ID,
			Op:          knowledge.OpConceptDelete,
			Payload:     payload,
			CreatedBy:   "author",
		}))
	}
	return cs.ID
}

func TestSubmitSupersedesOpenChangeSetFromSameOrigin(t *testing.T) {
	h := newSummonsHarness(t)
	ctx := context.Background()

	first := h.draftWithOrigin(t, "push one", "kapi-push/concepts", "ca", "cb")
	c, _ := h.ctx(http.MethodPost, "/api/v1/acme/changesets/"+first+"/submit", "", "author", "id", first)
	require.NoError(t, h.srv.HandleSubmitChangeSet(c))

	second := h.draftWithOrigin(t, "push two", "kapi-push/concepts", "ca", "cb", "cc")
	c, _ = h.ctx(http.MethodPost, "/api/v1/acme/changesets/"+second+"/submit", "", "author", "id", second)
	require.NoError(t, h.srv.HandleSubmitChangeSet(c))

	prior, err := h.fake.GetChangeSet(ctx, summonsWSID, first)
	require.NoError(t, err)
	assert.Equal(t, knowledge.ChangeSetSuperseded, prior.Status)
	assert.Equal(t, second, prior.SupersededBy)

	successor, err := h.fake.GetChangeSet(ctx, summonsWSID, second)
	require.NoError(t, err)
	assert.Equal(t, knowledge.ChangeSetInReview, successor.Status)

	// The superseded review is over: only the successor holds an open task.
	tasks := h.openTasks(t)
	require.Len(t, tasks, 1)
	assert.Equal(t, second, tasks[0].Data["changeset_id"])
}

func TestSubmitIdenticalOpsIsIdempotent(t *testing.T) {
	h := newSummonsHarness(t)
	ctx := context.Background()

	first := h.draftWithOrigin(t, "push one", "kapi-push/concepts", "ca", "cb")
	c, _ := h.ctx(http.MethodPost, "/api/v1/acme/changesets/"+first+"/submit", "", "author", "id", first)
	require.NoError(t, h.srv.HandleSubmitChangeSet(c))

	second := h.draftWithOrigin(t, "push two", "kapi-push/concepts", "ca", "cb")
	c, rec := h.ctx(http.MethodPost, "/api/v1/acme/changesets/"+second+"/submit", "", "author", "id", second)
	require.NoError(t, h.srv.HandleSubmitChangeSet(c))

	// The response is the change-set already in review, not the new draft.
	var got knowledge.ChangeSet
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, first, got.ID)
	assert.Equal(t, knowledge.ChangeSetInReview, got.Status)

	abandoned, err := h.fake.GetChangeSet(ctx, summonsWSID, second)
	require.NoError(t, err)
	assert.Equal(t, knowledge.ChangeSetAbandoned, abandoned.Status)

	// No second summons: the open review task is still the first one's.
	tasks := h.openTasks(t)
	require.Len(t, tasks, 1)
	assert.Equal(t, first, tasks[0].Data["changeset_id"])
}

func TestHandDraftedChangeSetsNeverSupersede(t *testing.T) {
	h := newSummonsHarness(t)
	ctx := context.Background()

	first := h.draftWithOrigin(t, "manual one", "", "ca")
	c, _ := h.ctx(http.MethodPost, "/api/v1/acme/changesets/"+first+"/submit", "", "author", "id", first)
	require.NoError(t, h.srv.HandleSubmitChangeSet(c))

	second := h.draftWithOrigin(t, "manual two", "", "ca")
	c, _ = h.ctx(http.MethodPost, "/api/v1/acme/changesets/"+second+"/submit", "", "author", "id", second)
	require.NoError(t, h.srv.HandleSubmitChangeSet(c))

	for _, id := range []string{first, second} {
		cs, err := h.fake.GetChangeSet(ctx, summonsWSID, id)
		require.NoError(t, err)
		assert.Equal(t, knowledge.ChangeSetInReview, cs.Status, "hand-drafted change-sets review independently")
	}
	assert.Len(t, h.openTasks(t), 2)
}
