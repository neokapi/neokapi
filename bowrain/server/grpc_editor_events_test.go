package server

import (
	"testing"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	pb "github.com/neokapi/neokapi/bowrain/proto/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBusEventToProjectEvent_Variants verifies the broadened mapping from
// platform events to the gRPC ProjectEvent oneof so the desktop refreshes the
// right view on each external change. Wire-compat for the two original
// variants (block + presence) is asserted alongside the new ones.
func TestBusEventToProjectEvent_Variants(t *testing.T) {
	tests := []struct {
		name   string
		ev     platev.Event
		assert func(t *testing.T, pe *pb.ProjectEvent)
	}{
		{
			name: "block update",
			ev: platev.Event{Type: platev.EventBlockUpdated, ProjectID: "p1", Data: map[string]string{
				"block_id": "b1", "item_name": "home.json", "change_type": "updated", "changed_by": "alice", "stream": "main",
			}},
			assert: func(t *testing.T, pe *pb.ProjectEvent) {
				bc := pe.GetBlockChange()
				require.NotNil(t, bc)
				assert.Equal(t, "b1", bc.BlockId)
				assert.Equal(t, "home.json", bc.ItemName)
				assert.Equal(t, "main", bc.Stream)
				assert.Equal(t, "alice", bc.ChangedBy)
			},
		},
		{
			name: "editor block update prefix",
			ev:   platev.Event{Type: "editor.block.updated", ProjectID: "p1", Data: map[string]string{"block_id": "b2"}},
			assert: func(t *testing.T, pe *pb.ProjectEvent) {
				require.NotNil(t, pe.GetBlockChange())
				assert.Equal(t, "b2", pe.GetBlockChange().BlockId)
			},
		},
		{
			name: "presence joined",
			ev: platev.Event{Type: "editor.presence.joined", ProjectID: "p1", Data: map[string]string{
				"event_kind": "presence", "user_id": "u1", "user_name": "Bob",
			}},
			assert: func(t *testing.T, pe *pb.ProjectEvent) {
				pc := pe.GetPresenceChange()
				require.NotNil(t, pc)
				assert.Equal(t, "joined", pc.ChangeType)
				assert.Equal(t, "Bob", pc.User.UserName)
			},
		},
		{
			name: "item created",
			ev:   platev.Event{Type: platev.EventItemCreated, ProjectID: "p1", Data: map[string]string{"item_name": "about.json", "stream": "main"}},
			assert: func(t *testing.T, pe *pb.ProjectEvent) {
				ic := pe.GetItemChange()
				require.NotNil(t, ic)
				assert.Equal(t, "item.created", ic.EventType)
				assert.Equal(t, "about.json", ic.ItemName)
			},
		},
		{
			name: "connector sync",
			ev:   platev.Event{Type: platev.EventSyncCompleted, ProjectID: "p1", Actor: "system", Data: map[string]string{}},
			assert: func(t *testing.T, pe *pb.ProjectEvent) {
				cs := pe.GetConnectorSync()
				require.NotNil(t, cs)
				assert.Equal(t, "connector.sync.completed", cs.EventType)
				assert.Equal(t, "system", cs.Actor)
			},
		},
		{
			name: "flow completed",
			ev:   platev.Event{Type: platev.EventFlowCompleted, ProjectID: "p1", Data: map[string]string{}},
			assert: func(t *testing.T, pe *pb.ProjectEvent) {
				fe := pe.GetFlowEvent()
				require.NotNil(t, fe)
				assert.Equal(t, "flow.completed", fe.EventType)
			},
		},
		{
			name: "brand profile updated (workspace-global, no project)",
			ev:   platev.Event{Type: platev.EventBrandProfileUpdated, ProjectID: "", Data: map[string]string{}},
			assert: func(t *testing.T, pe *pb.ProjectEvent) {
				bv := pe.GetBrandVoice()
				require.NotNil(t, bv)
				assert.Equal(t, "brand.profile.updated", bv.EventType)
			},
		},
		{
			name: "stream merged",
			ev:   platev.Event{Type: platev.EventStreamMerged, ProjectID: "p1", Data: map[string]string{"stream": "feature-x"}},
			assert: func(t *testing.T, pe *pb.ProjectEvent) {
				se := pe.GetStream()
				require.NotNil(t, se)
				assert.Equal(t, "stream.merged", se.EventType)
				assert.Equal(t, "feature-x", se.Stream)
			},
		},
		{
			name: "termbase change via event_kind",
			ev:   platev.Event{Type: "concept.updated", ProjectID: "p1", Data: map[string]string{"event_kind": "termbase"}},
			assert: func(t *testing.T, pe *pb.ProjectEvent) {
				tb := pe.GetTermbase()
				require.NotNil(t, tb)
				assert.Equal(t, "concept.updated", tb.EventType)
			},
		},
		{
			name: "membership change via task event",
			ev:   platev.Event{Type: "task.assigned", ProjectID: "p1", Actor: "alice", Data: map[string]string{}},
			assert: func(t *testing.T, pe *pb.ProjectEvent) {
				mc := pe.GetMembershipChange()
				require.NotNil(t, mc)
				assert.Equal(t, "task.assigned", mc.EventType)
			},
		},
		{
			name: "project updated → generic ProjectChange",
			ev:   platev.Event{Type: platev.EventProjectUpdated, ProjectID: "p1", Actor: "alice", Data: map[string]string{"change_type": "renamed"}},
			assert: func(t *testing.T, pe *pb.ProjectEvent) {
				pc := pe.GetProjectChange()
				require.NotNil(t, pc)
				assert.Equal(t, "project.updated", pc.EventType)
				assert.Equal(t, "renamed", pc.ChangeType)
				assert.Equal(t, "alice", pc.Actor)
			},
		},
		{
			name: "extraction completed → generic ProjectChange",
			ev:   platev.Event{Type: platev.EventExtractionCompleted, ProjectID: "p1", Data: map[string]string{}},
			assert: func(t *testing.T, pe *pb.ProjectEvent) {
				require.NotNil(t, pe.GetProjectChange())
				assert.Equal(t, "extraction.completed", pe.GetProjectChange().EventType)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pe := busEventToProjectEvent(tc.ev)
			require.NotNil(t, pe, "event should map to a ProjectEvent")
			tc.assert(t, pe)
		})
	}
}

// TestBusEventToProjectEvent_Dropped verifies events that should NOT reach
// project watchers map to nil.
func TestBusEventToProjectEvent_Dropped(t *testing.T) {
	dropped := []platev.Event{
		{Type: platev.EventAgentMessageSent, ProjectID: "p1", Data: map[string]string{}},
		{Type: "agent.tool.executed", ProjectID: "p1", Data: map[string]string{}},
		{Type: "", ProjectID: "p1", Data: map[string]string{}},
	}
	for _, ev := range dropped {
		assert.Nil(t, busEventToProjectEvent(ev), "event %q must be dropped", ev.Type)
	}
}
