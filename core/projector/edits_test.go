package projector_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/workspace"
)

func runsOf(text string) []model.Run { return []model.Run{{Text: &model.TextRun{Text: text}}} }

// agentEdit is an agent's edit of one source paragraph and its French
// translation, keeping the runs around each change and the change set.
func agentEdit(doc, block, before, after string) projector.Edit {
	en, _ := model.ParseEditionKey("en")
	fr, _ := model.ParseEditionKey("fr")
	return projector.Edit{
		Doc:         projector.EditDoc{Key: doc, Path: "docs/guide.md"},
		Home:        "file",
		Actor:       change.Actor{Kind: change.ActorAgent, Name: "claude", Session: "s_01"},
		Origin:      projector.Origin{By: "apply"},
		Fingerprint: "gov_4b2",
		Note:        "Point the guide link at the handbook",
		DocBefore:   "sha256:aa", DocAfter: "sha256:bb",
		Transitions: []projector.EditTransition{
			{
				Block: block, Edition: "en",
				Before: model.RunsRevision(en, runsOf(before)), After: model.RunsRevision(en, runsOf(after)),
				ContentHash: model.ComputeContentHash(after), ContextHash: "ctx-" + block,
				BeforeRuns: runsOf(before), AfterRuns: runsOf(after),
			},
			{
				Block: block, Edition: "fr",
				Before: model.AbsentRevision, After: model.RunsRevision(fr, runsOf("Bonjour")),
				Basis:       model.RunsRevision(en, runsOf(after)),
				ContentHash: model.ComputeContentHash(after), ContextHash: "ctx-" + block,
				AfterRuns: runsOf("Bonjour"),
			},
		},
		Overridden: []change.Finding{{Rule: "terms.vocabulary", Message: "avoid utilize", Fails: true}},
		SetJSON:    []byte(`{"schema":"kapi.change/v1","ops":[]}`),
	}
}

// flowEdit is a flow's hash-only edit of n blocks of one document.
func flowEdit(doc string, n int, round int) projector.Edit {
	nb, _ := model.ParseEditionKey("nb")
	e := projector.Edit{
		Doc:    projector.EditDoc{Key: doc, Path: "docs/" + doc + ".md"},
		Home:   "file",
		Actor:  change.Actor{Kind: change.ActorTool, Name: "translate"},
		Origin: projector.Origin{By: "flow:up"},
	}
	for i := range n {
		e.Transitions = append(e.Transitions, projector.EditTransition{
			Block: fmt.Sprintf("p#%d", i), Edition: "nb",
			Before:      model.RunsRevision(nb, runsOf(fmt.Sprintf("draft %d.%d", round, i))),
			After:       model.RunsRevision(nb, runsOf(fmt.Sprintf("draft %d.%d", round+1, i))),
			Basis:       fmt.Sprintf("r:%016d", i),
			ContentHash: fmt.Sprintf("h%d", i), ContextHash: fmt.Sprintf("c%d", i),
		})
	}
	return e
}

func TestRecordEditWritesTheOperationAndTheHistory(t *testing.T) {
	p, ws, db := open(t)
	ctx := t.Context()
	e := agentEdit("d-guide", "install/p", "See the guide.", "See the handbook.")
	id, err := p.RecordEdit(ctx, e)
	require.NoError(t, err)
	require.NotEmpty(t, id)

	ops, err := ws.Select(ctx, workspace.OpQuery{KindPrefix: projector.KindEdit})
	require.NoError(t, err)
	require.Len(t, ops, 1)
	op := ops[0]
	assert.Equal(t, id, op.ID)
	assert.True(t, strings.HasPrefix(op.Address, "edit:"+string(key)+":"), "addressed by content: %s", op.Address)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(op.Payload, &payload))
	assert.Equal(t, map[string]any{"key": "d-guide", "path": "docs/guide.md"}, payload["doc"])
	assert.Equal(t, "file", payload["home"])
	assert.Equal(t, map[string]any{"kind": "agent", "name": "claude", "session": "s_01"}, payload["actor"])
	assert.Equal(t, map[string]any{"by": "apply"}, payload["origin"])
	assert.Equal(t, "gov_4b2", payload["fingerprint"])
	assert.Equal(t, "sha256:aa", payload["doc_before"])
	assert.NotEmpty(t, payload["overridden"])

	var got projector.Edit
	require.NoError(t, json.Unmarshal(op.Payload, &got))
	require.Len(t, got.Transitions, 2)
	en := got.Transitions[0]
	for name, ref := range map[string]string{"runs_before": en.RunsBefore, "runs_after": en.RunsAfter, "change_set": got.ChangeSet} {
		address, ok := projector.BlobAddress(ref)
		require.True(t, ok, "%s names a blob: %q", name, ref)
		_, err := ws.Blob(ctx, address)
		require.NoError(t, err, "%s is stored", name)
	}
	address, _ := projector.BlobAddress(en.RunsAfter)
	blob, err := ws.Blob(ctx, address)
	require.NoError(t, err)
	assert.Equal(t, string(model.CanonicalRunsJSON(runsOf("See the handbook."))), string(blob),
		"the runs are kept in the form the revision is computed over")
	assert.Empty(t, got.Transitions[1].RunsBefore, "an edition the edit created has nothing before it")

	last, found, err := db.History().LastWrite(ctx, "d-guide", "install/p", "fr")
	require.NoError(t, err)
	require.True(t, found, "the edit is in the block history")
	assert.Equal(t, id, last.Op)
	assert.Equal(t, "agent", last.Actor)
	assert.Equal(t, "claude", last.ActorName)
	assert.Equal(t, "s_01", last.Session)
	assert.Equal(t, "apply", last.Origin)
	assert.Equal(t, e.Transitions[1].Basis, last.Basis)
	assert.Equal(t, op.At, last.At)
}

func TestAFlowEditKeepsHashesOnly(t *testing.T) {
	p, ws, _ := open(t)
	ctx := t.Context()
	_, err := p.RecordEdit(ctx, flowEdit("d-a", 3, 0))
	require.NoError(t, err)
	ops, err := ws.Select(ctx, workspace.OpQuery{KindPrefix: projector.KindEdit})
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.NotContains(t, string(ops[0].Payload), "runs_")
	assert.NotContains(t, string(ops[0].Payload), "change_set")
}

func TestRecordingOneEditTwiceIsOneOperation(t *testing.T) {
	p, ws, db := open(t)
	ctx := t.Context()
	e := agentEdit("d-guide", "install/p", "A", "B")
	first, err := p.RecordEdit(ctx, e)
	require.NoError(t, err)
	again, err := p.RecordEdit(ctx, e)
	require.NoError(t, err)
	assert.Equal(t, first, again, "a re-record is the same operation")

	// The same change made again after it was undone extends a different
	// head, so it is a new operation and the history shows all three.
	back := agentEdit("d-guide", "install/p", "B", "A")
	_, err = p.RecordEdit(ctx, back)
	require.NoError(t, err)
	third, err := p.RecordEdit(ctx, e)
	require.NoError(t, err)
	assert.NotEqual(t, first, third)

	ops, err := ws.Select(ctx, workspace.OpQuery{KindPrefix: projector.KindEdit})
	require.NoError(t, err)
	assert.Len(t, ops, 3)
	rows, err := db.History().Edition(ctx, "d-guide", "install/p", "en")
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, third, rows[0].Op, "the most recent write answers who last wrote it")
}

func TestALargeEditMovesToABlob(t *testing.T) {
	p, ws, db := open(t)
	ctx := t.Context()
	_, err := p.RecordEdit(ctx, flowEdit("d-big", 400, 0))
	require.NoError(t, err)
	ops, err := ws.Select(ctx, workspace.OpQuery{KindPrefix: projector.KindEdit})
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Contains(t, string(ops[0].Payload), `"blob"`)
	rows, err := db.History().Document(ctx, "d-big")
	require.NoError(t, err)
	assert.Len(t, rows, 400)
}

func TestEditsDecisionsAndAdoptionsAreDistinctKinds(t *testing.T) {
	p, ws, db := open(t)
	ctx := t.Context()
	work := db.Work()
	work.SetJournal(p.Decisions())
	require.NoError(t, work.Put(ctx, state.UnitState{
		Scope: "d-guide", Unit: "install/p", Variant: model.Variant("fr"), Status: model.TargetStatusEstablished,
		ContentHash: "h", TargetHash: "t", Decision: state.Decision{ReviewState: "approved", By: "reviewer"},
	}))
	keys, err := work.AdoptDocuments(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, keys)
	_, err = p.RecordEdit(ctx, flowEdit("d-guide", 1, 0))
	require.NoError(t, err)

	ops, err := ws.Select(ctx, workspace.OpQuery{Project: key})
	require.NoError(t, err)
	kinds := map[string]int{}
	for _, op := range ops {
		kinds[op.Kind]++
		if op.Kind == projector.KindDecision {
			assert.True(t, strings.HasPrefix(op.Address, "decision:"+string(key)+":"), op.Address)
		}
	}
	assert.Equal(t, 1, kinds[projector.KindDecision])
	assert.Equal(t, 1, kinds[projector.KindEdit])
	assert.Zero(t, kinds["unit.record"])
}

// writeEditLog makes the record-side writes a project accumulates: agent and
// flow edits across several documents, decisions through the ledger's
// journal, and document adoptions as a checkout resolves its reads.
func writeEditLog(t *testing.T, p *projector.Projector, db *projectdb.DB) {
	t.Helper()
	ctx := t.Context()
	work := db.Work()
	work.SetJournal(p.Decisions())
	for round := range 3 {
		for d := range 4 {
			_, err := p.RecordEdit(ctx, flowEdit(fmt.Sprintf("d-%d", d), 50, round))
			require.NoError(t, err)
		}
		_, err := p.RecordEdit(ctx, agentEdit("d-guide", "install/p", fmt.Sprintf("v%d", round), fmt.Sprintf("v%d", round+1)))
		require.NoError(t, err)
	}
	require.NoError(t, work.Put(ctx, state.UnitState{
		Scope: "d-guide", Unit: "install/p", Variant: model.Variant("fr"), Status: model.TargetStatusEstablished,
		ContentHash: "h", TargetHash: "t", Decision: state.Decision{ReviewState: "approved", By: "reviewer"},
	}))
	_, err := work.AdoptDocuments(ctx, []reconcile.DocUnit{
		{Path: "docs/a.md", Content: []string{"h1", "h2"}},
		{Path: "docs/b.md", Content: []string{"h3"}},
	})
	require.NoError(t, err)
}

func TestRebuildReproducesTheBlockHistory(t *testing.T) {
	p, ws, db := open(t)
	writeMixedLog(t, p, 200)
	writeEditLog(t, p, db)
	before := snapshot(t, ws, db)
	require.Len(t, before["block_history"], 3*(4*50+2))
	require.Len(t, before["document_adoption"], 2)
	require.NotEmpty(t, before["unit_decision"])

	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second)))
	report, err := p.Rebuild(t.Context())
	require.NoError(t, err)
	assert.Empty(t, report.Failed)
	assert.Equal(t, 15, report.Operations[projector.KindEdit])
	assert.Equal(t, 2, report.Operations[projector.KindAdopt])
	assert.Equal(t, before, snapshot(t, ws, db), "a rebuild writes the block history, the adoptions and the ledger the writes left")
}

func TestRebuildFromACheckpointKeepsTheBlockHistory(t *testing.T) {
	p, ws, db := open(t)
	ctx := t.Context()
	writeEditLog(t, p, db)
	cp, err := p.Checkpoint(ctx)
	require.NoError(t, err)
	_, err = p.RecordEdit(ctx, flowEdit("d-after", 5, 0))
	require.NoError(t, err)
	before := snapshot(t, ws, db)

	report, err := p.Rebuild(ctx)
	require.NoError(t, err)
	assert.Empty(t, report.Failed)
	assert.Equal(t, cp.Through, report.Checkpoint)
	assert.Equal(t, 1, report.Total(), "only the edit after the checkpoint is replayed")
	assert.Equal(t, before, snapshot(t, ws, db))
}

func TestAnEditTravelsWithItsBlobs(t *testing.T) {
	ctx := t.Context()
	remote := workspace.NewFileRemote(t.TempDir())
	from, fromWS, _ := open(t)
	to, toWS, toDB := open(t)

	small := agentEdit("d-guide", "install/p", "A", "B")
	_, err := from.RecordEdit(ctx, small)
	require.NoError(t, err)
	large := flowEdit("d-big", 400, 0)
	large.Actor = change.Actor{Kind: change.ActorPerson, Name: "asgeir"}
	for i := range large.Transitions {
		large.Transitions[i].AfterRuns = runsOf(fmt.Sprintf("row %d", i))
	}
	_, err = from.RecordEdit(ctx, large)
	require.NoError(t, err)

	opts := workspace.SyncOptions{LocalKinds: projector.LocalKinds}
	_, err = fromWS.NewSync(remote, key, from.Syncer(), opts).Push(ctx)
	require.NoError(t, err)
	_, err = toWS.NewSync(remote, key, to.Syncer(), opts).Pull(ctx)
	require.NoError(t, err)

	ops, err := toWS.Select(ctx, workspace.OpQuery{KindPrefix: projector.KindEdit})
	require.NoError(t, err)
	require.Len(t, ops, 2)
	for _, op := range ops {
		refs := workspace.BlobRefs(op)
		require.NotEmpty(t, refs, "an edit that keeps runs names its blobs")
		for _, ref := range refs {
			_, err := toWS.Blob(ctx, ref)
			require.NoError(t, err, "%s arrived with the operation", ref)
		}
	}
	rows, err := toDB.History().Document(ctx, "d-big")
	require.NoError(t, err)
	assert.Len(t, rows, 400, "the pulled edits are in the block history")
}

func TestEmbeddedLayoutRecordsAnEditWithoutALog(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, project.StateDirName), 0o755))
	db, err := projectdb.Open(t.Context(), project.LayoutAt(root))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	p, err := projector.ForProject(nil, "", db)
	require.NoError(t, err)
	id, err := p.RecordEdit(t.Context(), agentEdit("d-guide", "install/p", "A", "B"))
	require.NoError(t, err)
	require.True(t, workspace.ValidOpID(id))
	last, found, err := db.History().LastWrite(t.Context(), "d-guide", "install/p", "en")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, id, last.Op)
}
