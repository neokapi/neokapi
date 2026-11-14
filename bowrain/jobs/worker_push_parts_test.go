package jobs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/bowrain/storage"
	bloblocal "github.com/neokapi/neokapi/bowrain/storage/localblob"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	voicestore "github.com/neokapi/neokapi/bowrain/voice"
	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	corestorage "github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/core/workspace"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/terms"
)

// A push carries its content, its terminology edits and its context ledger
// objects in one manifest, and the worker applies them on one PostgreSQL
// transaction: the stores are wired exactly as bowrain-worker wires them
// (NewPostgresPushTransition over one pool). A part that fails leaves the
// project as the push found it, content included.

const (
	partsProject   = "proj-parts"
	partsWorkspace = "ws-parts"
	partsSlug      = "acme"
)

type partsEnv struct {
	deps *WorkerDeps
	db   *storage.PgDB
	cs   *bstore.PostgresStore
	ks   *knowledge.PostgresKnowledgeStore
}

func newPartsEnv(t *testing.T) *partsEnv {
	t.Helper()
	db := pgtest.NewTestDB(t)
	js, err := NewJobStore(db)
	require.NoError(t, err)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	voice, err := voicestore.NewPostgresVoiceStore(db)
	require.NoError(t, err)
	ks, err := knowledge.NewPostgresKnowledgeStore(db)
	require.NoError(t, err)
	bs, err := bloblocal.New(t.TempDir())
	require.NoError(t, err)
	deps := &WorkerDeps{JobStore: js, ContentStore: cs, VoiceStore: voice, BlobStore: bs}
	deps.PushTransition = NewPostgresPushTransition(db, cs, voice, ks)
	require.NoError(t, cs.CreateProject(t.Context(), &store.Project{
		ID: partsProject, Name: "Parts", WorkspaceID: partsWorkspace,
	}))
	return &partsEnv{deps: deps, db: db, cs: cs, ks: ks}
}

// termsStore is the workspace's terms store, read through the pool.
func (e *partsEnv) termsStore(t *testing.T) *sqlterms.PostgresStore {
	t.Helper()
	tb, err := sqlterms.NewPostgresStoreFromDB(e.db, partsSlug)
	require.NoError(t, err)
	return tb
}

// push writes a two-block chunk and a manifest carrying parts beside it, and
// runs the push job.
func (e *partsEnv) push(t *testing.T, jobID string, parts map[string]any) error {
	t.Helper()
	ctx := t.Context()
	chunk := &pb.SyncChunk{
		ContentType: "blocks",
		RecordCount: 2,
		Blocks: []*pb.SyncBlock{
			{Id: "b1", ItemName: "en.json", SourceText: "Hello", Translatable: true},
			{Id: "b2", ItemName: "en.json", SourceText: "World", Translatable: true},
		},
	}
	data, err := proto.Marshal(chunk)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	_, err = e.deps.BlobStore.Upload(ctx, data, corestorage.UploadOptions{})
	require.NoError(t, err)

	manifest := map[string]any{
		"upload_id":      "upload-" + jobID,
		"project_id":     partsProject,
		"stream":         "main",
		"actor_id":       "user-1",
		"workspace_slug": partsSlug,
		"chunks": []map[string]any{{
			"index": 0, "content_type": "blocks", "hash": hex.EncodeToString(sum[:]),
			"record_count": 2, "byte_size": len(data),
		}},
		"items": []map[string]string{{"name": "en.json", "format": "json"}},
	}
	maps.Copy(manifest, parts)
	raw, err := json.Marshal(manifest)
	require.NoError(t, err)
	ref, err := e.deps.BlobStore.Upload(ctx, raw, corestorage.UploadOptions{})
	require.NoError(t, err)
	require.NoError(t, e.deps.JobStore.CreateJob(ctx, &TranslationJob{
		ID: jobID, ProjectID: partsProject, ItemName: SyncPushItemName,
		Model: ref.Key, PushID: "push-" + jobID, Status: StatusQueued, CreatedBy: "user-1",
	}))
	return ProcessSyncPushJobForTest(ctx, e.deps, jobID)
}

func (e *partsEnv) blocks(t *testing.T) int {
	t.Helper()
	got, err := e.cs.GetBlocks(t.Context(), store.BlockQuery{ProjectID: partsProject, Stream: "main", ItemName: "en.json", Limit: 100})
	require.NoError(t, err)
	return len(got)
}

func (e *partsEnv) concepts(t *testing.T) []terms.Concept {
	t.Helper()
	got, err := e.termsStore(t).Concepts(t.Context())
	require.NoError(t, err)
	return got
}

func (e *partsEnv) contextObjects(t *testing.T) []string {
	t.Helper()
	got, err := e.cs.ListContextObjects(t.Context(), partsProject, workspace.RemoteLogDir)
	require.NoError(t, err)
	return got
}

func (e *partsEnv) contextKey(t *testing.T) string {
	t.Helper()
	key, err := e.cs.ContextKey(t.Context(), partsProject)
	require.NoError(t, err)
	return key
}

var (
	partsSegment     = workspace.SegmentName("w1", "000000000000000000000001")
	partsSegmentData = []byte("{\"id\":\"000000000000000000000001\"}\n")
)

// partsTerms is one ordinary concept create and, when proposal is set, one
// governed operation.
func partsTerms(t *testing.T, proposal bool) *apiclient.TermsEdits {
	t.Helper()
	edits := &apiclient.TermsEdits{
		Creates: []apiclient.CreateConceptParams{{
			Domain:     "ui",
			Definition: "The main call to action.",
			Terms:      []apiclient.TermInfo{{Text: "Sign up", Locale: "en", Status: "approved"}},
		}},
	}
	if proposal {
		payload, err := json.Marshal(knowledge.ConceptCreatePayload{Concept: terms.Concept{
			ID:    "c-banned",
			Terms: []terms.Term{{Text: "Click here", Locale: "en", Status: model.TermForbidden}},
		}})
		require.NoError(t, err)
		edits.Proposal = &apiclient.TermsProposal{
			Name:   "kapi push",
			Origin: "kapi-push/concepts",
			Ops:    []apiclient.ChangeSetOpInput{{Op: string(knowledge.OpConceptCreate), Payload: payload}},
		}
	}
	return edits
}

func TestPushParts_AllPartsLandTogether(t *testing.T) {
	e := newPartsEnv(t)
	ctx := t.Context()

	err := e.push(t, "job-all", map[string]any{
		"terms":              partsTerms(t, true),
		"terms_author":       "user-1",
		"context_objects":    []apiclient.ContextObjectWire{{Name: partsSegment, Data: partsSegmentData}},
		"context_key":        "prj_parts",
		"context_operations": 1,
	})
	require.NoError(t, err)

	assert.Equal(t, 2, e.blocks(t), "the content landed")
	concepts := e.concepts(t)
	require.Len(t, concepts, 1, "the ordinary create landed")
	assert.Equal(t, "The main call to action.", concepts[0].Definition)
	assert.Equal(t, []string{partsSegment}, e.contextObjects(t), "the ledger landed")
	assert.Equal(t, "prj_parts", e.contextKey(t), "and bound the project's context")

	open, err := e.ks.ListOpenChangeSetsByOrigin(ctx, partsWorkspace, "kapi-push/concepts")
	require.NoError(t, err)
	require.Len(t, open, 1, "the governed edit is one change-set")
	assert.Equal(t, knowledge.ChangeSetInReview, open[0].Status)

	raw, err := e.deps.JobStore.PushParts(ctx, "push-job-all")
	require.NoError(t, err)
	var parts venue.PushParts
	require.NoError(t, json.Unmarshal([]byte(raw), &parts))
	require.NotNil(t, parts.Terms)
	assert.Equal(t, 1, parts.Terms.ConceptsApplied)
	assert.Equal(t, 1, parts.Terms.ConceptsProposed)
	assert.Equal(t, open[0].ID, parts.Terms.ChangesetID)
	require.NotNil(t, parts.Ledger)
	assert.Equal(t, venue.PushLedgerOutcome{Objects: 1, Operations: 1, Bound: true}, *parts.Ledger)

	// The same proposal again returns the change-set already in review.
	require.NoError(t, e.push(t, "job-again", map[string]any{"terms": partsTerms(t, true), "terms_author": "user-1"}))
	raw, err = e.deps.JobStore.PushParts(ctx, "push-job-again")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(raw), &parts))
	assert.True(t, parts.Terms.ChangesetUnchanged)
	assert.Equal(t, open[0].ID, parts.Terms.ChangesetID)
}

// Each case carries content plus a part the transaction refuses. Nothing the
// push carried survives: not the blocks, not the other part.
func TestPushParts_AFailingPartRollsBackTheContent(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, e *partsEnv)
		parts func(t *testing.T) map[string]any
		want  string
	}{
		{
			name: "a terms edit of a concept the workspace does not hold",
			parts: func(t *testing.T) map[string]any {
				edits := partsTerms(t, false)
				edits.Updates = []apiclient.ConceptEdit{{ConceptID: "c-missing", Concept: apiclient.UpdateConceptParams{Definition: "gone"}}}
				return map[string]any{
					"terms":           edits,
					"context_objects": []apiclient.ContextObjectWire{{Name: partsSegment, Data: partsSegmentData}},
					"context_key":     "prj_parts",
				}
			},
			want: "no such concept",
		},
		{
			name: "a ledger object the project holds with other bytes",
			setup: func(t *testing.T, e *partsEnv) {
				require.NoError(t, e.cs.PutContextObjects(t.Context(), partsProject, "someone", []store.ContextObject{
					{Name: partsSegment, Data: []byte("other bytes\n")},
				}))
			},
			parts: func(t *testing.T) map[string]any {
				return map[string]any{
					"terms":           partsTerms(t, true),
					"context_objects": []apiclient.ContextObjectWire{{Name: partsSegment, Data: partsSegmentData}},
				}
			},
			want: "other bytes",
		},
		{
			name: "a ledger naming another project key than the bound one",
			setup: func(t *testing.T, e *partsEnv) {
				_, err := e.cs.BindContextKey(t.Context(), partsProject, "prj_first")
				require.NoError(t, err)
			},
			parts: func(t *testing.T) map[string]any {
				return map[string]any{
					"terms":           partsTerms(t, false),
					"context_objects": []apiclient.ContextObjectWire{{Name: partsSegment, Data: partsSegmentData}},
					"context_key":     "prj_second",
				}
			},
			want: "belongs to project key prj_first",
		},
		{
			name: "a proposal computed against terminology that has moved",
			setup: func(t *testing.T, e *partsEnv) {
				require.NoError(t, e.termsStore(t).AddConcept(t.Context(), terms.Concept{
					ID: "c-held", Terms: []terms.Term{{Text: "Held", Locale: "en", Status: model.TermApproved}},
				}))
			},
			parts: func(t *testing.T) map[string]any {
				edits := partsTerms(t, true)
				edits.Proposal.ExpectedTermsRef = "a-ref-from-before"
				return map[string]any{"terms": edits}
			},
			want: "terms governance moved since it was last observed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newPartsEnv(t)
			if tc.setup != nil {
				tc.setup(t, e)
			}
			conceptsBefore := len(e.concepts(t))
			objectsBefore := e.contextObjects(t)
			keyBefore := e.contextKey(t)

			err := e.push(t, "job-fail", tc.parts(t))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)

			assert.Zero(t, e.blocks(t), "the content rolled back with the part")
			assert.Len(t, e.concepts(t), conceptsBefore, "no terminology edit survived")
			assert.Equal(t, objectsBefore, e.contextObjects(t), "no ledger object survived")
			assert.Equal(t, keyBefore, e.contextKey(t), "the context binding rolled back")
			open, err := e.ks.ListOpenChangeSetsByOrigin(t.Context(), partsWorkspace, "kapi-push/concepts")
			require.NoError(t, err)
			assert.Empty(t, open, "no change-set survived")

			job, err := e.deps.JobStore.GetJob(t.Context(), "job-fail")
			require.NoError(t, err)
			assert.Equal(t, StatusFailed, job.Status, "the producer is told the push failed")
		})
	}
}
