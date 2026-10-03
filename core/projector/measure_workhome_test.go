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
	"github.com/neokapi/neokapi/core/workhome"
	"github.com/neokapi/neokapi/core/workspace"
)

// TestMeasureKeptDrafts keeps what a gated convergence pass over the dogfood
// project would park: 400 documents of 188 drafts each (75,200), one
// workspace home write per document, each draft carrying its runs, basis,
// status, origin and stamp. A second pass drafts every block again, as a pass
// after a source edit on every page does, and a third keeps the same drafts,
// as a steady pass does. It reports the time, the bytes the writes carry and
// the size of each database, then times reading every kept edition, a
// rebuild, and releasing every edition as a delivery does.
func TestMeasureKeptDrafts(t *testing.T) {
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
	home := &workhome.Home{Store: db.Heads(), Log: p}

	const docs, perDoc = 400, 188
	nb, _ := model.ParseEditionKey("nb")
	docRef := func(d int) string { return fmt.Sprintf("docs/page-%03d.md", d) }
	stamp := []byte(`{"key":"docs/page.md\u0000tu1\u0000source","provider":"demo","config":"cfg_4b2c"}`)
	pass := func(round, written int) time.Duration {
		start := time.Now()
		for d := range docs {
			held, err := home.Edition(ctx, docRef(d), nb)
			require.NoError(t, err)
			pr := workhome.Produce{Doc: docRef(d), Edition: nb,
				Actor: change.Actor{Kind: change.ActorTool, Name: "up"}, Origin: "flow:up"}
			for b := range perDoc {
				block := fmt.Sprintf("section-%d/p#%d", b/10, b%10)
				before := model.AbsentRevision
				if ed, ok := held.Blocks[block]; ok {
					before = model.RunsRevision(nb, ed.Runs)
				}
				pr.Blocks = append(pr.Blocks, workhome.Produced{Block: block, Before: before, Tool: "translate",
					Edition: model.Edition{Runs: runsOf(fmt.Sprintf("Utkast %d av avsnitt %d, side %d, med en setning av vanlig lengde.", round, b, d)),
						Status: model.Status(model.TargetStatusDraft), Origin: model.Origin{Kind: model.OriginAI, Engine: "demo"}},
					Basis:       model.RunsRevision(model.EditionKey{}, runsOf(fmt.Sprintf("source %d %d", d, b))),
					Stamp:       stamp,
					ContentHash: model.ComputeContentHash(fmt.Sprintf("source %d %d", d, b)),
					ContextHash: model.ComputeContentHash(fmt.Sprintf("context %d", b)),
				})
			}
			res, err := home.Produce(ctx, pr)
			require.NoError(t, err)
			require.Equal(t, written, res.Written)
		}
		return time.Since(start)
	}

	drafts := docs * perDoc
	first := pass(0, perDoc)
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
	t.Logf("pass 1: %d drafts kept in %d writes in %s (%.3f ms a draft)", drafts, len(ops), first,
		float64(first.Microseconds())/1000/float64(drafts))
	t.Logf("pass 1: %.1f MB carried by the writes and their blobs", float64(payload)/1e6)

	second := pass(1, perDoc)
	t.Logf("pass 2, every draft made again: %s (%.3f ms a draft)", second, float64(second.Microseconds())/1000/float64(drafts))
	before, err := ws.Select(ctx, workspace.OpQuery{Project: key, KindPrefix: projector.KindEdit})
	require.NoError(t, err)
	steady := pass(1, 0)
	after, err := ws.Select(ctx, workspace.OpQuery{Project: key, KindPrefix: projector.KindEdit})
	require.NoError(t, err)
	require.Len(t, after, len(before), "a steady pass records nothing")
	t.Logf("a steady pass, every draft held already: %s, nothing recorded", steady)

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
	for d := range docs {
		held, err := home.Edition(ctx, docRef(d), nb)
		require.NoError(t, err)
		require.Len(t, held.Blocks, perDoc)
	}
	t.Logf("reading every kept edition: %s (%.2f ms a document)", time.Since(start), float64(time.Since(start).Microseconds())/1000/docs)

	start = time.Now()
	report, err := p.Rebuild(ctx)
	require.NoError(t, err)
	require.Empty(t, report.Failed)
	t.Logf("rebuild of %d writes in %s", report.Operations[projector.KindEdit], time.Since(start))

	start = time.Now()
	for d := range docs {
		held, err := home.Read(ctx, docRef(d), nb)
		require.NoError(t, err)
		_, err = home.Release(ctx, docRef(d), nb, workhome.Release{Token: held.Token,
			Actor: change.Actor{Kind: change.ActorTool, Name: "merge"}, Origin: "merge"})
		require.NoError(t, err)
	}
	t.Logf("releasing every edition, as a delivery does: %s", time.Since(start))
}
