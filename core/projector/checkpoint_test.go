package projector_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/workspace"
)

// historyPass writes one pass's block history straight into the context
// store: docs documents of transitions hash-only changes each, the shape a
// flow records over a collection.
func historyPass(t *testing.T, db *projectdb.DB, pass, docs, transitions int) {
	t.Helper()
	rows := make([]history.Row, 0, docs*transitions)
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC).Add(time.Duration(pass) * time.Hour)
	for d := range docs {
		op := workspace.NewOpID(at.Add(time.Duration(d)*time.Millisecond), "")
		for b := range transitions {
			rows = append(rows, history.Row{
				Op: op, Address: fmt.Sprintf("edit:%s:%064x", key, pass*docs+d), Doc: fmt.Sprintf("d-%016x", d),
				Block: fmt.Sprintf("doc/p#%d", b), Key: fmt.Sprintf("k1_%012x", b), Edition: "nb",
				Before:      fmt.Sprintf("r:%016x", pass*transitions+b),
				After:       fmt.Sprintf("r:%016x", (pass+1)*transitions+b),
				Basis:       fmt.Sprintf("r:%016x", b),
				ContentHash: fmt.Sprintf("%016x", b), ContextHash: fmt.Sprintf("%016x", d),
				Actor: "tool", ActorName: "translate", Origin: "flow:up", At: at,
			})
		}
	}
	require.NoError(t, db.History().Put(t.Context(), rows))
}

// TestACheckpointOfALongHistoryIsMadeOfBlobsThatFit: two full passes at the
// size the edit model budgets for (400 documents of 188 transitions, 75,200
// a pass) are a block history of 150,400 rows, whose rows alone run past the
// size one blob may have. The checkpoint keeps them in parts, every file it
// is made of fits, and a rebuild from it writes every row back.
func TestACheckpointOfALongHistoryIsMadeOfBlobsThatFit(t *testing.T) {
	if testing.Short() {
		t.Skip("writes 150,400 rows")
	}
	p, ws, db := open(t)
	ctx := t.Context()
	_, err := p.RecordEdit(ctx, flowEdit("d-a", 1, 0))
	require.NoError(t, err)
	for pass := range 2 {
		historyPass(t, db, pass, 400, 188)
	}
	rows, before := historyDigest(t, db)
	require.Equal(t, 2*400*188+1, rows)

	cp, err := p.Checkpoint(ctx)
	require.NoError(t, err, "the checkpoint is stored")
	assert.Less(t, cp.Bytes, 4<<20, "the checkpoint file carries the small tables only")
	assert.Greater(t, cp.Parts, 4, "the block history is kept in parts")

	ops, err := ws.Select(ctx, workspace.OpQuery{KindPrefix: projector.KindCheckpoint})
	require.NoError(t, err)
	require.Len(t, ops, 1)
	refs := workspace.BlobRefs(ops[0])
	require.Len(t, refs, cp.Parts+1, "the operation names the file and every part")
	total := 0
	for _, ref := range refs {
		data, err := ws.Blob(ctx, ref)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(data), workspace.MaxBlobSize/4)
		total += len(data)
	}
	assert.Greater(t, total, workspace.MaxBlobSize, "the history alone is larger than one blob may be")

	report, err := p.Rebuild(ctx)
	require.NoError(t, err)
	assert.Empty(t, report.Failed)
	assert.Equal(t, cp.Through, report.Checkpoint)
	_, after := historyDigest(t, db)
	assert.Equal(t, before, after, "every row comes back from the parts")
}

// historyDigest counts the block history's rows and digests them, every
// column, in key order: a comparison of 150,000 rows that costs a scan.
func historyDigest(t *testing.T, db *projectdb.DB) (int, string) {
	t.Helper()
	rows, err := db.Raw().QueryContext(t.Context(), `SELECT op || '|' || address || '|' || doc || '|' || block || '|' ||
    key || '|' || edition || '|' || before || '|' || after || '|' || basis || '|' || content_hash || '|' ||
    context_hash || '|' || actor || '|' || actor_name || '|' || session || '|' || origin || '|' || at
  FROM block_history ORDER BY doc, block, edition, address`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	h := sha256.New()
	n := 0
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		h.Write([]byte(line))
		h.Write([]byte{'\n'})
		n++
	}
	require.NoError(t, rows.Err())
	return n, hex.EncodeToString(h.Sum(nil))
}

// TestACheckpointMissingAPartCostsAReplay: a rebuild that cannot read a part
// of the checkpoint replays the whole log instead, and says so.
func TestACheckpointMissingAPartCostsAReplay(t *testing.T) {
	projector.SetCheckpointPartSizes(t, 4<<10, 16<<10)
	p, ws, db := open(t)
	ctx := t.Context()
	writeEditLog(t, p, db)
	cp, err := p.Checkpoint(ctx)
	require.NoError(t, err)
	require.Greater(t, cp.Parts, 1)
	before := snapshot(t, ws, db)

	ops, err := ws.Select(ctx, workspace.OpQuery{KindPrefix: projector.KindCheckpoint})
	require.NoError(t, err)
	require.Len(t, ops, 1)
	refs := workspace.BlobRefs(ops[0])
	_, err = ws.Registry().ExecContext(ctx, `DELETE FROM workspace_blobs WHERE digest = ?`, refs[len(refs)-1])
	require.NoError(t, err)

	report, err := p.Rebuild(ctx)
	require.NoError(t, err)
	assert.Empty(t, report.Checkpoint, "the rebuild replayed the log")
	require.Len(t, report.Failed, 1)
	assert.Contains(t, report.Failed[0], projector.KindCheckpoint)
	assert.Equal(t, before, snapshot(t, ws, db))
}

// TestAFirstPullStartsFromACheckpointInParts: a push writes the parts of its
// checkpoint to the remote before the checkpoint, and a first pull fetches
// them and starts from it.
func TestAFirstPullStartsFromACheckpointInParts(t *testing.T) {
	projector.SetCheckpointPartSizes(t, 4<<10, 16<<10)
	ctx := t.Context()
	dir := t.TempDir()
	remote := workspace.NewFileRemote(dir)
	from, fromWS, fromDB := open(t)
	writeEditLog(t, from, fromDB)
	opts := workspace.SyncOptions{LocalKinds: projector.LocalKinds, CheckpointEvery: 1}
	pushed, err := fromWS.NewSync(remote, key, from.Syncer(), opts).Push(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, pushed.Checkpoint)
	want := snapshot(t, fromWS, fromDB)["block_history"]

	cps, err := remote.List(ctx, workspace.RemoteCheckpointsDir)
	require.NoError(t, err)
	require.Len(t, cps, 1)
	data, err := remote.Get(ctx, cps[0])
	require.NoError(t, err)
	_, _, parts, err := from.Syncer().CheckpointMark(data)
	require.NoError(t, err)
	require.Greater(t, len(parts), 1, "the block history went in parts")
	for _, ref := range parts {
		_, err := remote.Get(ctx, workspace.BlobObjectName(ref))
		require.NoError(t, err, "part %s is on the remote", ref)
	}

	to, toWS, toDB := open(t)
	pulled, err := toWS.NewSync(remote, key, to.Syncer(), opts).Pull(ctx)
	require.NoError(t, err)
	assert.Equal(t, pushed.Checkpoint, pulled.Checkpoint, "the pull started from the checkpoint")
	assert.Equal(t, want, snapshot(t, toWS, toDB)["block_history"])
	report, err := to.Rebuild(ctx)
	require.NoError(t, err)
	assert.Empty(t, report.Failed)
	assert.Equal(t, pushed.Checkpoint, report.Checkpoint, "the installed checkpoint loads with its parts")
	assert.Equal(t, want, snapshot(t, toWS, toDB)["block_history"])

	// A remote that lost a part costs the next first pull a replay.
	require.NoError(t, os.Remove(filepath.Join(dir, filepath.FromSlash(workspace.BlobObjectName(parts[0])))))
	again, againWS, againDB := open(t)
	pulled, err = againWS.NewSync(remote, key, again.Syncer(), opts).Pull(ctx)
	require.NoError(t, err)
	assert.Empty(t, pulled.Checkpoint)
	assert.Equal(t, want, snapshot(t, againWS, againDB)["block_history"])
}

func TestRebuildFromACheckpointEqualsTheIncrementalState(t *testing.T) {
	p, ws, db := open(t)
	ctx := t.Context()
	writeMixedLog(t, p, 500)
	cp, err := p.Checkpoint(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, cp.Through)

	// Writes after the checkpoint, of the kinds that can follow one.
	require.NoError(t, p.Terms().AddConcept(ctx, concept("c-after", "After", model.TermPreferred)))
	require.NoError(t, p.Terms().DeleteConcept(ctx, "imp-03"))
	require.NoError(t, p.Memory().Add(ctx, entry("m-after", "Later", "Seinere")))
	require.NoError(t, p.Memory().Delete(ctx, "m-1"))
	require.NoError(t, p.Rules().NarrowRule(ctx, "prj_docs\x00a"))
	before := snapshot(t, ws, db)

	report, err := p.Rebuild(ctx)
	require.NoError(t, err)
	assert.Empty(t, report.Failed)
	assert.Equal(t, cp.Through, report.Checkpoint, "the rebuild starts from the checkpoint")
	assert.Equal(t, 5, report.Total(), "and replays only what came after it")
	assert.Equal(t, before, snapshot(t, ws, db))

	// An operation merged in after the checkpoint with an earlier id is one
	// the checkpoint does not include, so the rebuild replays the whole log.
	held, err := ws.Select(ctx, workspace.OpQuery{KindPrefix: projector.KindTerms, Limit: 1})
	require.NoError(t, err)
	merged := held[0]
	merged.ID, merged.Seq, merged.Address = workspace.NewOpID(merged.At.Add(-time.Millisecond), ""), 0, ""
	_, err = ws.Record(ctx, merged)
	require.NoError(t, err)
	report, err = p.Rebuild(ctx)
	require.NoError(t, err)
	assert.Empty(t, report.Checkpoint, "a checkpoint an earlier operation arrived after no longer stands")
	assert.Empty(t, report.Failed)
}
