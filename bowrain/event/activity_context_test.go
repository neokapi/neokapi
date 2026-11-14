package event

import (
	"testing"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A context.recorded event files one context_change row under the slug, with
// the counts rendered as its summary, and leaves the event's own data as it
// was published.
func TestActivityRecorder_MapsContextChanges(t *testing.T) {
	counts := platev.ContextCounts{}
	counts.Add(platev.ContextItemTermRule, platev.ContextOpKeep, 1)
	counts.Add(platev.ContextItemTermRule, platev.ContextOpDrop, 2)
	data := platev.ContextChange{
		WorkspaceSlug: "acme", ActorKind: platev.ContextActorAgent, Session: "s1",
		Source: "decide", ItemID: "op-1", Counts: counts,
	}.Data()
	ev := platev.Event{Type: platev.EventContextRecorded, ProjectID: "p1", WorkspaceID: "ws-id", Actor: "u1", Data: data}

	a := mapEvent(ev)
	require.NotNil(t, a)
	assert.Equal(t, bstore.ActivityContextChanged, a.Type)
	assert.Equal(t, "acme", a.WorkspaceID)
	assert.Equal(t, "p1", a.ProjectID)
	assert.Equal(t, "term_rule", a.EntityType)
	assert.Equal(t, "op-1", a.EntityID)
	assert.Equal(t, "kept 1 term rule · dropped 2 term rules", a.Summary)
	assert.Equal(t, bstore.FeedKindContextChange, a.Kind)
	assert.Equal(t, map[string]int{"term_rule": 3, "keep": 1, "drop": 2}, a.Counts)
	assert.Equal(t, "agent", a.ActorKind)
	assert.Equal(t, "s1", a.Session)
	assert.Equal(t, "agent", a.Data["actor_kind"])
	assert.Equal(t, "s1", a.Data["session"])
}

// A voice event names the workspace as its project; its row names none. A
// voice event without a context change stays out of the feed.
func TestActivityRecorder_VoiceContextChanges(t *testing.T) {
	counts := platev.ContextCounts{}
	counts.Add(platev.ContextItemVoiceRule, platev.ContextOpDrop, 1)
	ev := platev.Event{
		Type: platev.EventVoiceRuleRejected, ProjectID: "ws-id", WorkspaceID: "ws-id", Actor: "u1",
		Data: platev.ContextChange{WorkspaceSlug: "acme", ItemID: "utilize", Counts: counts}.Data(),
	}
	a := mapEvent(ev)
	require.NotNil(t, a)
	assert.Empty(t, a.ProjectID)
	assert.Equal(t, "voice_rule", a.EntityType)
	assert.Equal(t, "dropped 1 voice rule", a.Summary)

	assert.Nil(t, mapEvent(platev.Event{Type: platev.EventVoiceRuleRejected, Data: map[string]string{"term": "x"}}))
}

// Governed change-set rows carry the change_set feed kind. Pilot rows belong
// to no change-set event and carry none.
func TestActivityRecorder_ChangeSetFeedKind(t *testing.T) {
	for _, et := range []platev.EventType{
		knowledge.EventChangeSetSubmitted, knowledge.EventChangeSetApproved, knowledge.EventChangeSetRejected,
		knowledge.EventChangeSetMerged, knowledge.EventChangeSetCreated,
	} {
		ev := knowledgeEvent(et, map[string]string{"changeset_id": "cs-1"})
		a := mapEvent(ev)
		require.NotNil(t, a, et)
		assert.Equal(t, bstore.FeedKindChangeSet, a.Kind, et)
		assert.NotContains(t, ev.Data, "state", "the published event's data is shared and stays as it was")

	}
	a := mapEvent(knowledgeEvent(knowledge.EventPilotStarted, map[string]string{"changeset_id": "cs-1"}))
	require.NotNil(t, a)
	assert.Empty(t, a.Kind)
}
