package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/knowledge"
)

// A change-set a push proposed is submitted by the push worker after the
// request is over. When its changeset.submitted event arrives, the server
// summons the reviewers exactly as the submit route does: one review task
// assigned to an eligible reviewer who is not the author, and a mail. When a
// later push supersedes it, its review task closes.
func TestPushedChangeSetSummonsReviewers(t *testing.T) {
	h := newSummonsHarness(t)
	id := h.draftWithGovernedOps(t, "kapi push 2026-11-14", 3)
	require.NoError(t, h.fake.SetChangeSetStatus(context.Background(), summonsWSID, id, knowledge.ChangeSetInReview))

	event := func(t knowledge.EventType, source string) platev.Event {
		return knowledge.PlatformEvent(knowledge.MergeEvent{
			Type: t, WorkspaceID: summonsWSID, ChangesetID: id, Actor: "author",
		}, source, summonsWSSlug, "author", nil, time.Now())
	}
	h.srv.handlePushedChangeSet(event(knowledge.EventChangeSetSubmitted, "knowledge"))
	assert.Empty(t, h.openTasks(t), "a submit through the route summons in the request, not here")

	h.srv.handlePushedChangeSet(event(knowledge.EventChangeSetSubmitted, pushWorkerSource))
	tasks := h.openTasks(t)
	require.Len(t, tasks, 1, "the pushed change-set opens one review task")
	assert.Equal(t, "owner", tasks[0].AssigneeID, "assigned to a reviewer, never the author")
	assert.Equal(t, id, tasks[0].Data["changeset_id"])
	require.Len(t, h.sends.messages(), 1)
	assert.Equal(t, "owner@acme.test", h.sends.messages()[0].to)

	h.srv.handlePushedChangeSet(event(knowledge.EventChangeSetSuperseded, pushWorkerSource))
	assert.Empty(t, h.openTasks(t), "a superseded change-set's review task closes")
}
