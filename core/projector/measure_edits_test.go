package projector_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/workspace"
)

// TestMeasureEditRecording records what a convergence pass over the dogfood
// project would: 400 documents, 188 hash-only transitions each (75,200), one
// content.edit per document. The first pass records each document on its own
// (RecordEdit), the second the whole pass in one call (RecordEdits). It
// reports the time, the bytes the operations carry and the size of each
// database, then times a rebuild.
func TestMeasureEditRecording(t *testing.T) {
	measuring(t)
	ctx := t.Context()
	dir := t.TempDir()
	ws, err := workspace.OpenLocal(ctx, filepath.Join(dir, "workspace"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ws.Close() })
	root := filepath.Join(dir, "checkout")
	require.NoError(t, os.MkdirAll(filepath.Join(root, project.StateDirName), 0o755))
	contextDB, err := ws.Context(ctx, key)
	require.NoError(t, err)
	db, err := projectdb.Open(ctx, project.LayoutAt(root), projectdb.WithWorkspace(projectdb.Stores{Context: contextDB, Graph: ws.Registry()}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	p, err := projector.ForProject(ws, key, db)
	require.NoError(t, err)

	const docs, perDoc = 400, 188
	nb, _ := model.ParseEditionKey("nb")
	pass := func(round int, batched bool) time.Duration {
		start := time.Now()
		var batch []projector.Edit
		for d := range docs {
			e := projector.Edit{
				Doc:    projector.EditDoc{Key: fmt.Sprintf("d-%016x", d), Path: fmt.Sprintf("docs/page-%03d.md", d)},
				Home:   "file",
				Actor:  change.Actor{Kind: change.ActorTool, Name: "translate"},
				Origin: projector.Origin{By: "flow:up"},
			}
			for b := range perDoc {
				e.Transitions = append(e.Transitions, projector.EditTransition{
					Block: fmt.Sprintf("section-%d/p#%d", b/10, b%10), Edition: "nb",
					Before:      model.RunsRevision(nb, runsOf(fmt.Sprintf("utkast %d %d %d", round, d, b))),
					After:       model.RunsRevision(nb, runsOf(fmt.Sprintf("utkast %d %d %d", round+1, d, b))),
					Basis:       model.RunsRevision(nb, runsOf(fmt.Sprintf("source %d %d", d, b))),
					ContentHash: model.ComputeContentHash(fmt.Sprintf("source %d %d", d, b)),
					ContextHash: model.ComputeContentHash(fmt.Sprintf("context %d", b)),
				})
			}
			if batched {
				batch = append(batch, e)
				continue
			}
			_, err := p.RecordEdit(ctx, e)
			require.NoError(t, err)
		}
		if batched {
			_, err := p.RecordEdits(ctx, batch)
			require.NoError(t, err)
		}
		return time.Since(start)
	}

	first := pass(0, false)
	ops, err := ws.Select(ctx, workspace.OpQuery{Project: key, KindPrefix: projector.KindEdit})
	require.NoError(t, err)
	payload := 0
	for _, op := range ops {
		payload += len(op.Payload)
		for _, ref := range workspace.BlobRefs(op) {
			data, err := ws.Blob(ctx, ref)
			require.NoError(t, err)
			payload += len(data)
		}
	}
	transitions := docs * perDoc
	t.Logf("pass 1, a document at a time: %d transitions in %d operations in %s (%.3f ms each)", transitions, len(ops), first,
		float64(first.Microseconds())/1000/float64(transitions))
	t.Logf("pass 1: %.1f MB carried by the operations and their blobs", float64(payload)/1e6)

	second := pass(1, true)
	t.Logf("pass 2, the whole pass in one call, over a history holding pass 1: %s (%.3f ms each)", second,
		float64(second.Microseconds())/1000/float64(transitions))

	// Each file's write-ahead log folded in, so the sizes are the databases'.
	for _, raw := range []*storage.DB{db.Raw(), ws.Registry()} {
		_, err := raw.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
		require.NoError(t, err)
	}
	size := func(path string) float64 {
		var total int64
		for _, suffix := range []string{"", "-wal"} {
			if info, err := os.Stat(path + suffix); err == nil {
				total += info.Size()
			}
		}
		return float64(total) / 1e6
	}
	t.Logf("after two passes: workspace.db %.1f MB, the project's context store %.1f MB",
		size(filepath.Join(dir, "workspace", workspace.RegistryFileName)), size(db.ContextPath()))

	start := time.Now()
	report, err := p.Rebuild(ctx)
	require.NoError(t, err)
	require.Empty(t, report.Failed)
	t.Logf("rebuild of %d edits (%d rows) in %s", report.Operations[projector.KindEdit], 2*transitions, time.Since(start))
}
