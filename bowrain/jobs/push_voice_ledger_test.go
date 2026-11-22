package jobs

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/voicewrite"
	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/projector"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/workspace"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

const voiceLedgerKey = "prj_pushvoiceledgeraaaa"

// memLog is a contextop.Log over a slice: Record appends with fresh ids, and
// Select returns what it holds.
type memLog struct{ ops []workspace.Op }

func (l *memLog) Record(_ context.Context, ops ...workspace.Op) ([]workspace.Op, error) {
	for i := range ops {
		if ops[i].ID == "" {
			ops[i].ID = workspace.NewOpID(ops[i].At, "")
		}
		ops[i].Seq = int64(len(l.ops) + 1)
		l.ops = append(l.ops, ops[i])
	}
	return ops, nil
}

func (l *memLog) Select(context.Context, workspace.OpQuery) ([]workspace.Op, error) {
	return l.ops, nil
}

// clientVoiceSegment is the ledger segment a kapi client pushes after writing
// profile in a claude-code shell: one edit operation, as host records it. It
// returns the segment and the operation's id.
func clientVoiceSegment(t *testing.T, profile *coreprofile.VoiceProfile) (store.ContextObject, string) {
	t.Helper()
	edit, err := contextop.NewVoiceEdit(nil, profile)
	require.NoError(t, err)
	log := &memLog{}
	rec, err := contextop.NewLedger(log, nil).Append(t.Context(), contextop.Record{
		Project: voiceLedgerKey,
		Actor:   contextop.Actor{Kind: contextop.ActorPerson},
		Kind:    contextop.KindEdit,
		Subject: contextop.Subject{Kind: contextop.SubjectNote, Text: "voice profile " + profile.ID + " saved"},
		Note:    "saved in the voice editor (recorded as a person in a claude-code shell)",
		Voice:   edit,
	})
	require.NoError(t, err)
	data, err := workspace.EncodeSegment(log.ops)
	require.NoError(t, err)
	return store.ContextObject{Name: workspace.SegmentName("laptop", rec.ID), Data: data}, rec.ID
}

// projectVoiceEdits reads the voice edit operations the project's ledger
// holds, with the writer of the segment each was read from.
func projectVoiceEdits(t *testing.T, deps *WorkerDeps, projectID string) ([]contextop.Record, map[string]string) {
	t.Helper()
	ctx := t.Context()
	objects, ok := store.ContextObjects(deps.ContentStore)
	require.True(t, ok)
	names, err := objects.ListContextObjects(ctx, projectID, workspace.RemoteLogDir)
	require.NoError(t, err)
	log := &memLog{}
	writers := map[string]string{}
	for _, name := range names {
		obj, err := objects.GetContextObject(ctx, projectID, name)
		require.NoError(t, err)
		ops, err := workspace.DecodeSegment(obj.Data, voiceLedgerKey)
		require.NoError(t, err)
		for _, op := range ops {
			writers[op.ID] = workspace.SegmentWriter(name)
			op.Seq = int64(len(log.ops) + 1)
			log.ops = append(log.ops, op)
		}
	}
	records, err := contextop.NewLedger(log, nil).Records(ctx, contextop.Filter{})
	require.NoError(t, err)
	var out []contextop.Record
	for _, r := range records {
		if r.Voice != nil {
			out = append(out, r)
		}
	}
	return out, writers
}

// serverSegments reads the ledger segments the server wrote into the
// project, each as the operations it holds.
func serverSegments(t *testing.T, deps *WorkerDeps, projectID string) [][]workspace.Op {
	t.Helper()
	ctx := t.Context()
	objects, ok := store.ContextObjects(deps.ContentStore)
	require.True(t, ok)
	names, err := objects.ListContextObjects(ctx, projectID, workspace.RemoteLogDir)
	require.NoError(t, err)
	var out [][]workspace.Op
	for _, name := range names {
		if workspace.SegmentWriter(name) != voicewrite.Writer {
			continue
		}
		obj, err := objects.GetContextObject(ctx, projectID, name)
		require.NoError(t, err)
		ops, err := workspace.DecodeSegment(obj.Data, voiceLedgerKey)
		require.NoError(t, err)
		out = append(out, ops)
	}
	return out
}

// TestPushVoiceLedger_RecordsEachWriteOnce: a push writes the voice profile
// and its ledger record as one change. The operation the client names stands
// as the record only when it describes this write: a voice edit of the same
// profile, fingerprinted as the profile now stands, pushed by the pusher.
// Any other operation is ignored, and the server records the write as the
// pusher's, so a client cannot credit its write to another member or an
// agent.
func TestPushVoiceLedger_RecordsEachWriteOnce(t *testing.T) {
	pushedProfile := func() *coreprofile.VoiceProfile {
		return &coreprofile.VoiceProfile{
			ID: "acme-voice", Name: "Acme Voice",
			Style: coreprofile.StyleRules{ProhibitedPatterns: []coreprofile.Pattern{{Regex: `\bleverage\b`}}},
		}
	}
	tests := []struct {
		name string
		// client is the profile the client's operation records, nil for none.
		client func() *coreprofile.VoiceProfile
		// carried says the push carries the client's segment; heldBy names the
		// member who pushed it earlier, empty when nobody did.
		carried bool
		heldBy  string
		// op is the entry's voice_op: "client" names the client's operation.
		op string
		// wantClient says the client's operation is the record.
		wantClient bool
	}{
		{name: "client operation carried in the push", client: pushedProfile, carried: true, op: "client", wantClient: true},
		{name: "client operation the pusher pushed earlier", client: pushedProfile, heldBy: "user-1", op: "client", wantClient: true},
		{name: "another member's operation", client: pushedProfile, heldBy: "user-2", op: "client"},
		{name: "an operation for a different profile", carried: true, op: "client",
			client: func() *coreprofile.VoiceProfile {
				p := pushedProfile()
				p.ID, p.Name = "other-voice", "Other Voice"
				return p
			}},
		{name: "a stale operation", carried: true, op: "client",
			client: func() *coreprofile.VoiceProfile {
				p := pushedProfile()
				p.Style.ProhibitedPatterns = nil
				return p
			}},
		{name: "no client operation", op: ""},
		{name: "an operation the ledger does not hold", op: "0000000000aaaaaaaaaaaaaaaa"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, _ := newTransitionDeps(t)
			ctx := t.Context()
			newContextProject(t, ctx, deps, "p1", "ws1")
			objects, ok := store.ContextObjects(deps.ContentStore)
			require.True(t, ok)
			_, err := objects.BindContextKey(ctx, "p1", voiceLedgerKey)
			require.NoError(t, err)

			ledger := pushLedger{}
			var clientOp string
			if tt.client != nil {
				var segment store.ContextObject
				segment, clientOp = clientVoiceSegment(t, tt.client())
				if tt.heldBy != "" {
					require.NoError(t, objects.PutContextObjects(ctx, "p1", store.Member(tt.heldBy), []store.ContextObject{segment}))
				}
				if tt.carried {
					ledger.Objects = []apiclient.ContextObjectWire{{Name: segment.Name, Data: segment.Data}}
				}
			}
			op := tt.op
			if op == "client" {
				op = clientOp
			}
			authored, err := json.Marshal(pushedProfile())
			require.NoError(t, err)
			entry := contextEntry("docs", map[string]string{"product": "kapi"}, "", "Acme Voice", authored)
			entry.VoiceOp = op

			err = deps.PushTransition(ctx, func(st PushStores) error {
				if _, rerr := reconcileContext(ctx, st.Content, st.Voice, "p1", "main", "ws1", "user-1",
					newPushVoiceLedger(ctx, deps, st.Content, "p1", "user-1", ledger), []*pb.SyncContextEntry{entry}); rerr != nil {
					return rerr
				}
				_, aerr := applyPushLedger(ctx, st.Content, "p1", "user-1", ledger)
				return aerr
			})
			require.NoError(t, err)

			profiles, err := deps.VoiceStore.ListProfiles(ctx, "ws1")
			require.NoError(t, err)
			require.Len(t, profiles, 1, "the push wrote the profile")

			edits, writers := projectVoiceEdits(t, deps, "p1")
			var server []contextop.Record
			for _, e := range edits {
				if writers[e.ID] == voicewrite.Writer {
					server = append(server, e)
				}
			}
			segments := serverSegments(t, deps, "p1")
			if tt.wantClient {
				require.Len(t, edits, 1, "the client's operation is the one record")
				assert.Equal(t, clientOp, edits[0].ID)
				assert.Contains(t, edits[0].Note, "claude-code shell")
				assert.Empty(t, segments, "the client carries its own store write, so none echoes back")
				return
			}
			require.Len(t, server, 1, "the server records the write as the pusher's")
			require.Len(t, segments, 1)
			require.Len(t, segments[0], 2, "the record and the store write share one segment")
			assert.Equal(t, server[0].ID, segments[0][0].ID)
			assert.Equal(t, projector.KindVoice, segments[0][1].Kind)
			assert.Contains(t, string(segments[0][1].Payload), `"id":"acme-voice"`, "stored under the id a checkout spells from the name")
			assert.Equal(t, contextop.ActorPerson, server[0].Actor.Kind)
			assert.Equal(t, "pushed with `kapi push`", server[0].Note)
			assert.Equal(t, profiles[0].ID, server[0].Voice.Profile)
		})
	}
}

// A push made with a tool token stores its segments as the tool's, and records
// its voice profile write as the tool's, issued by the member. An operation the
// member pushed in person earlier is not the record of the tool's write.
func TestPushVoiceLedger_AToolsPushIsTheTools(t *testing.T) {
	deps, _ := newTransitionDeps(t)
	ctx := platev.WithTool(t.Context(), platev.Tool{Name: "dogfood-sync", TokenID: "tok-1"})
	newContextProject(t, ctx, deps, "p1", "ws1")
	objects, ok := store.ContextObjects(deps.ContentStore)
	require.True(t, ok)
	_, err := objects.BindContextKey(ctx, "p1", voiceLedgerKey)
	require.NoError(t, err)

	profile := &coreprofile.VoiceProfile{
		ID: "acme-voice", Name: "Acme Voice",
		Style: coreprofile.StyleRules{ProhibitedPatterns: []coreprofile.Pattern{{Regex: `\bleverage\b`}}},
	}
	segment, clientOp := clientVoiceSegment(t, profile)
	require.NoError(t, objects.PutContextObjects(t.Context(), "p1", store.Member("user-1"), []store.ContextObject{segment}))
	authored, err := json.Marshal(profile)
	require.NoError(t, err)
	entry := contextEntry("docs", map[string]string{"product": "kapi"}, "", "Acme Voice", authored)
	entry.VoiceOp = clientOp

	ledger := pushLedger{}
	err = deps.PushTransition(ctx, func(st PushStores) error {
		_, rerr := reconcileContext(ctx, st.Content, st.Voice, "p1", "main", "ws1", "user-1",
			newPushVoiceLedger(ctx, deps, st.Content, "p1", "user-1", ledger), []*pb.SyncContextEntry{entry})
		return rerr
	})
	require.NoError(t, err)

	edits, writers := projectVoiceEdits(t, deps, "p1")
	var server []contextop.Record
	for _, e := range edits {
		if writers[e.ID] == voicewrite.Writer {
			server = append(server, e)
		}
	}
	require.Len(t, server, 1, "the member's own earlier operation is not the tool's record")
	assert.Equal(t, contextop.Actor{Kind: contextop.ActorTool, Name: "dogfood-sync"}, server[0].Actor)

	names, err := objects.ListContextObjects(ctx, "p1", workspace.RemoteLogDir)
	require.NoError(t, err)
	for _, name := range names {
		if workspace.SegmentWriter(name) != voicewrite.Writer {
			continue
		}
		obj, err := objects.GetContextObject(ctx, "p1", name)
		require.NoError(t, err)
		assert.Equal(t, store.Pusher{UserID: "user-1", Tool: "dogfood-sync", TokenID: "tok-1"}, obj.PushedBy)
	}
}
