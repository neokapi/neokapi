package event

import (
	"testing"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The mapping is a pure function of the event, so it is exercised directly
// rather than through a bus and a database.
func mapEvent(ev platev.Event) *bstore.Activity {
	return (&ActivityRecorder{}).mapEventToActivity(ev)
}

func knowledgeEvent(t platev.EventType, data map[string]string) platev.Event {
	if data == nil {
		data = map[string]string{}
	}
	data["workspace_slug"] = "acme"
	return platev.Event{
		Type: t,
		// The knowledge path sets the workspace *id* here and carries the slug
		// in Data — the shape server/knowledge_wiring.go publishes.
		WorkspaceID: "9f1c0b3e-0000-4000-8000-000000000001",
		Actor:       "user-1",
		Data:        data,
	}
}

func TestActivityRecorder_KnowledgeEventsFileUnderTheSlug(t *testing.T) {
	// The feed lists by slug, so an event carrying both must file under the
	// slug or the row exists and no feed can ever show it.
	a := mapEvent(knowledgeEvent(knowledge.EventChangeSetSubmitted, map[string]string{
		"changeset_id": "cs-1",
	}))
	require.NotNil(t, a)
	assert.Equal(t, "acme", a.WorkspaceID)
}

func TestActivityRecorder_WorkspaceKeyFallbacks(t *testing.T) {
	t.Run("the event's own field when no slug is carried", func(t *testing.T) {
		a := mapEvent(platev.Event{
			Type:        platev.EventSyncCompleted,
			WorkspaceID: "acme",
			Data:        map[string]string{},
		})
		require.NotNil(t, a)
		assert.Equal(t, "acme", a.WorkspaceID)
	})

	t.Run("the data key when neither is set", func(t *testing.T) {
		a := mapEvent(platev.Event{
			Type: platev.EventSyncCompleted,
			Data: map[string]string{"workspace_id": "acme"},
		})
		require.NotNil(t, a)
		assert.Equal(t, "acme", a.WorkspaceID)
	})
}

func TestActivityRecorder_MapsKnowledgeEvents(t *testing.T) {
	tests := []struct {
		name       string
		event      platev.EventType
		data       map[string]string
		wantType   bstore.ActivityType
		wantEntity string
		wantID     string
		wantStream string
		wantIn     string
	}{
		{
			name:       "concept created",
			event:      knowledge.EventConceptCreated,
			data:       map[string]string{"concept_id": "c-1"},
			wantType:   bstore.ActivityConceptCreated,
			wantEntity: "concept",
			wantID:     "c-1",
			wantIn:     "created a concept",
		},
		{
			name:       "concept updated",
			event:      knowledge.EventConceptUpdated,
			data:       map[string]string{"concept_id": "c-1"},
			wantType:   bstore.ActivityConceptUpdated,
			wantEntity: "concept",
			wantID:     "c-1",
			wantIn:     "updated a concept",
		},
		{
			name:       "concept deleted",
			event:      knowledge.EventConceptDeleted,
			data:       map[string]string{"concept_id": "c-1"},
			wantType:   bstore.ActivityConceptDeleted,
			wantEntity: "concept",
			wantID:     "c-1",
		},
		{
			name:       "term status changed",
			event:      knowledge.EventConceptTermStatusChanged,
			data:       map[string]string{"concept_id": "c-1"},
			wantType:   bstore.ActivityConceptTermStatus,
			wantEntity: "concept",
			wantID:     "c-1",
		},
		{
			name:       "relation added",
			event:      knowledge.EventConceptRelationAdded,
			data:       map[string]string{"concept_id": "c-1"},
			wantType:   bstore.ActivityConceptRelationAdded,
			wantEntity: "concept",
			wantID:     "c-1",
		},
		{
			name:       "relation removed",
			event:      knowledge.EventConceptRelationRemoved,
			data:       map[string]string{"concept_id": "c-1"},
			wantType:   bstore.ActivityConceptRelationRemoved,
			wantEntity: "concept",
			wantID:     "c-1",
		},
		{
			name:       "observation added",
			event:      knowledge.EventObservationAdded,
			data:       map[string]string{"concept_id": "c-1"},
			wantType:   bstore.ActivityObservationAdded,
			wantEntity: "concept",
			wantID:     "c-1",
			wantIn:     "observation",
		},
		{
			name:       "comment added",
			event:      knowledge.EventConceptCommentAdded,
			data:       map[string]string{"concept_id": "c-1"},
			wantType:   bstore.ActivityCommentAdded,
			wantEntity: "concept",
			wantID:     "c-1",
			wantIn:     "commented",
		},
		{
			name:       "change-set opened",
			event:      knowledge.EventChangeSetCreated,
			data:       map[string]string{"changeset_id": "cs-1"},
			wantType:   bstore.ActivityChangeSetOpened,
			wantEntity: "changeset",
			wantID:     "cs-1",
			wantIn:     "opened an experiment",
		},
		{
			name:       "change-set abandoned",
			event:      knowledge.EventChangeSetAbandoned,
			data:       map[string]string{"changeset_id": "cs-1"},
			wantType:   bstore.ActivityChangeSetAbandoned,
			wantEntity: "changeset",
			wantID:     "cs-1",
		},
		{
			name:       "change-set superseded",
			event:      knowledge.EventChangeSetSuperseded,
			data:       map[string]string{"changeset_id": "cs-1"},
			wantType:   bstore.ActivityChangeSetSuperseded,
			wantEntity: "changeset",
			wantID:     "cs-1",
		},
		{
			name:       "pilot started names where it runs",
			event:      knowledge.EventPilotStarted,
			data:       map[string]string{"changeset_id": "cs-1", "project_id": "p-web", "stream": "main"},
			wantType:   bstore.ActivityPilotStarted,
			wantEntity: "changeset",
			wantID:     "cs-1",
			wantStream: "main",
			wantIn:     "p-web · main",
		},
		{
			name:       "pilot stopped",
			event:      knowledge.EventPilotStopped,
			data:       map[string]string{"changeset_id": "cs-1", "project_id": "p-web", "stream": "main"},
			wantType:   bstore.ActivityPilotStopped,
			wantEntity: "changeset",
			wantID:     "cs-1",
			wantStream: "main",
			wantIn:     "stopped piloting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := mapEvent(knowledgeEvent(tt.event, tt.data))
			require.NotNil(t, a, "event must map to an activity")
			assert.Equal(t, tt.wantType, a.Type)
			assert.Equal(t, tt.wantEntity, a.EntityType)
			assert.Equal(t, tt.wantID, a.EntityID)
			assert.Equal(t, tt.wantStream, a.Stream)
			assert.Equal(t, "acme", a.WorkspaceID)
			assert.NotEmpty(t, a.Summary)
			if tt.wantIn != "" {
				assert.Contains(t, a.Summary, tt.wantIn)
			}
		})
	}
}

// The families are the feed's filter vocabulary: `?types=` matches a prefix, so
// a UI asking for "concept" must get every concept.* entry and nothing else.
func TestActivityRecorder_KnowledgeTypeFamilies(t *testing.T) {
	families := map[string][]platev.EventType{
		"concept.": {
			knowledge.EventConceptCreated,
			knowledge.EventConceptUpdated,
			knowledge.EventConceptDeleted,
			knowledge.EventConceptTermStatusChanged,
			knowledge.EventConceptRelationAdded,
			knowledge.EventConceptRelationRemoved,
		},
		"changeset.": {
			knowledge.EventChangeSetCreated,
			knowledge.EventChangeSetAbandoned,
			knowledge.EventChangeSetSuperseded,
		},
		"pilot.":       {knowledge.EventPilotStarted, knowledge.EventPilotStopped},
		"observation.": {knowledge.EventObservationAdded},
		"comment.":     {knowledge.EventConceptCommentAdded},
		"review.": {
			knowledge.EventChangeSetSubmitted,
			knowledge.EventChangeSetApproved,
			knowledge.EventChangeSetRejected,
			knowledge.EventChangeSetMerged,
		},
	}

	for prefix, events := range families {
		for _, ev := range events {
			a := mapEvent(knowledgeEvent(ev, nil))
			require.NotNil(t, a, "%s must map", ev)
			assert.Truef(t, len(string(a.Type)) > len(prefix) && string(a.Type)[:len(prefix)] == prefix,
				"%s mapped to %q, which is not in the %q family", ev, a.Type, prefix)
		}
	}
}

func TestPilotSummary(t *testing.T) {
	assert.Equal(t, "started piloting a change on p-web · main",
		pilotSummary("started piloting a change", map[string]string{"project_id": "p-web", "stream": "main"}))
	assert.Equal(t, "started piloting a change on p-web",
		pilotSummary("started piloting a change", map[string]string{"project_id": "p-web"}))
	assert.Equal(t, "started piloting a change",
		pilotSummary("started piloting a change", map[string]string{}))
}
