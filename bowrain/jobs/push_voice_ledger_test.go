package jobs

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/voicewrite"
	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
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

// TestPushVoiceLedger_RecordsEachWriteOnce: a push writes the voice profile
// and its ledger record as one change. The operation the client recorded is
// the record when the push carries it or the project already holds it, with
// the client's note and agent host; otherwise the server records the write
// as the pusher's. No case leaves two records of one write.
func TestPushVoiceLedger_RecordsEachWriteOnce(t *testing.T) {
	tests := []struct {
		name string
		// carried says the push carries the client's segment; held says the
		// project holds it from an earlier push.
		carried, held bool
		// op is the entry's voice_op: "client" names the client's operation.
		op         string
		wantWriter string
		wantNote   string
	}{
		{name: "client operation carried in the push", carried: true, op: "client",
			wantWriter: "laptop", wantNote: "saved in the voice editor (recorded as a person in a claude-code shell)"},
		{name: "client operation held from an earlier push", held: true, op: "client",
			wantWriter: "laptop", wantNote: "saved in the voice editor (recorded as a person in a claude-code shell)"},
		{name: "no client operation", op: "",
			wantWriter: voicewrite.Writer, wantNote: "pushed with `kapi push`"},
		{name: "an operation the ledger does not hold", op: "0000000000aaaaaaaaaaaaaaaa",
			wantWriter: voicewrite.Writer, wantNote: "pushed with `kapi push`"},
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

			profile := &coreprofile.VoiceProfile{
				ID: "acme", Name: "Acme Voice",
				Style: coreprofile.StyleRules{ProhibitedPatterns: []coreprofile.Pattern{{Regex: `\bleverage\b`}}},
			}
			segment, clientOp := clientVoiceSegment(t, profile)
			if tt.held {
				require.NoError(t, objects.PutContextObjects(ctx, "p1", "user-1", []store.ContextObject{segment}))
			}
			ledger := pushLedger{}
			if tt.carried {
				ledger.Objects = []apiclient.ContextObjectWire{{Name: segment.Name, Data: segment.Data}}
			}
			op := tt.op
			if op == "client" {
				op = clientOp
			}
			authored, err := json.Marshal(profile)
			require.NoError(t, err)
			entry := contextEntry("docs", map[string]string{"product": "kapi"}, "", "Acme Voice", authored)
			entry.VoiceOp = op

			err = deps.PushTransition(ctx, func(st PushStores) error {
				if _, rerr := reconcileContext(ctx, st.Content, st.Voice, "p1", "main", "ws1", "user-1",
					newPushVoiceLedger(ctx, deps, st.Content, "p1", ledger), []*pb.SyncContextEntry{entry}); rerr != nil {
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
			require.Len(t, edits, 1, "one record of the write")
			assert.Equal(t, tt.wantWriter, writers[edits[0].ID])
			assert.Equal(t, tt.wantNote, edits[0].Note)
			assert.Equal(t, contextop.ActorPerson, edits[0].Actor.Kind)
			if tt.wantWriter == voicewrite.Writer {
				assert.Equal(t, profiles[0].ID, edits[0].Voice.Profile, "the server records the profile it stored")
				assert.Empty(t, edits[0].Voice.Previous, "the push created the profile")
			} else {
				assert.Equal(t, clientOp, edits[0].ID)
			}
		})
	}
}
