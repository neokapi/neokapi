package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/terms"
)

// failCompletionOnce is the job store with the first completion write failing,
// the moment after the push's transaction committed: the job is redelivered
// and run whole again.
type failCompletionOnce struct {
	JobStore
	failed bool
}

func (s *failCompletionOnce) UpdateJobStatus(ctx context.Context, id string, status JobStatus, errMsg string) error {
	if status == StatusCompleted && !s.failed {
		s.failed = true
		return errors.New("the job row could not be written")
	}
	return s.JobStore.UpdateJobStatus(ctx, id, status, errMsg)
}

// A push job redelivered after its transaction committed applies nothing
// again: one set of concepts, relations and change-sets, and the redelivered
// run completes the job and records the same report.
func TestPushParts_ARedeliveredJobAppliesOnce(t *testing.T) {
	e := newPartsEnv(t)
	ctx := t.Context()
	real := e.deps.JobStore
	e.deps.JobStore = &failCompletionOnce{JobStore: real}

	// Two concepts the push relates, already in the workspace.
	tb := e.termsStore(t)
	for _, id := range []string{"c-a", "c-b"} {
		require.NoError(t, tb.AddConcept(ctx, terms.Concept{ID: id, Terms: []terms.Term{{Text: id, Locale: "en", Status: model.TermApproved}}}))
	}
	edits := partsTerms(t, true)
	edits.RelationAdds = []apiclient.RelationEdit{{SourceID: "c-a", Relation: apiclient.AddRelationParams{TargetID: "c-b", RelationType: "RELATED"}}}
	parts := map[string]any{"terms": edits, "terms_author": "user-1"}

	err := e.push(t, "job-replay", parts)
	require.ErrorContains(t, err, "mark sync push job completed", "the first run committed and could not record completion")

	// The queue redelivers the job.
	require.NoError(t, real.UpdateJobStatus(ctx, "job-replay", StatusQueued, ""))
	require.NoError(t, ProcessSyncPushJobForTest(ctx, e.deps, "job-replay"))

	assert.Equal(t, 2, e.blocks(t))
	assert.Len(t, e.concepts(t), 3, "the create applied once beside the two held concepts")
	rels, err := tb.ListRelations(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, rels, 1, "the relation applied once")
	open, err := e.ks.ListOpenChangeSetsByOrigin(ctx, partsWorkspace, "kapi-push/concepts")
	require.NoError(t, err)
	assert.Len(t, open, 1, "the change-set was proposed once")

	job, err := real.GetJob(ctx, "job-replay")
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, job.Status, "the redelivered run finishes the job")
	raw, err := real.PushParts(ctx, "push-job-replay")
	require.NoError(t, err)
	var got venue.PushParts
	require.NoError(t, json.Unmarshal([]byte(raw), &got))
	require.NotNil(t, got.Terms)
	assert.Equal(t, 1, got.Terms.ConceptsApplied)
	assert.Equal(t, 1, got.Terms.RelationsApplied)
}
