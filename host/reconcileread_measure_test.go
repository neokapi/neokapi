package host

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/reconcile"
	"github.com/neokapi/neokapi/core/storage"
)

// TestMeasureReconcileOnRead measures what reconciling a read against the
// block history would cost beside the read itself, on the repository's own
// documentation (web/docs, read with the mdx reader the dogfood recipe uses).
// docs/internals/evals.md records the result and the decision taken on it.
//
// The history is the worst case: every block of every document has a recorded
// change, so every read fetches a prior for every block it returns. Each pass
// reads every document and then, timed apart:
//
//   - fetches the document's priors from block_history (history.Priors);
//   - runs reconcile.Blocks against them;
//   - reads the document's history head (history.DocumentHead), which is all a
//     cache keyed by document revision and history head would still pay on a
//     hit.
//
// Run with:
//
//	KAPI_MEASURE_RECONCILE=1 go test -tags fts5 ./host -run TestMeasureReconcileOnRead -v
func TestMeasureReconcileOnRead(t *testing.T) {
	if os.Getenv("KAPI_MEASURE_RECONCILE") == "" {
		t.Skip("set KAPI_MEASURE_RECONCILE=1 to measure")
	}
	ctx := t.Context()
	root, err := filepath.Abs(filepath.Join("..", "web", "docs"))
	require.NoError(t, err)
	var paths []string
	require.NoError(t, filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && (strings.HasSuffix(p, ".md") || strings.HasSuffix(p, ".mdx")) {
			paths = append(paths, p)
		}
		return nil
	}))
	sort.Strings(paths)
	require.NotEmpty(t, paths)
	reg := appWithFormats().FormatReg

	read := func(p string) []*model.Block {
		blocks, _, err := project.ReadSourceBlocks(ctx, reg, "mdx", p, "en", "", nil)
		require.NoError(t, err, p)
		return blocks
	}

	db, err := storage.OpenWith(filepath.Join(t.TempDir(), "context.db"), storage.ProjectOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	hist, err := history.Open(db)
	require.NoError(t, err)

	// Every block of every document gets one recorded change.
	blocksTotal := 0
	var rows []history.Row
	for i, p := range paths {
		doc := reconcile.DocumentKeyFor(p)
		for _, b := range read(p) {
			id := model.ComputeIdentity(b)
			rows = append(rows, history.Row{
				Op: fmt.Sprintf("op%06d", i), Address: fmt.Sprintf("edit:%06d", i), Doc: doc, Block: b.ID, Edition: "en",
				ContentHash: id.ContentHash, ContextHash: id.ContextHash, At: time.Now(),
			})
			blocksTotal++
		}
	}
	require.NoError(t, hist.Put(ctx, rows))

	const passes = 5
	var readTime, priorTime, reconcileTime, headTime time.Duration
	for range passes {
		for _, p := range paths {
			doc := reconcile.DocumentKeyFor(p)
			t0 := time.Now()
			blocks := read(p)
			t1 := time.Now()
			priors, err := hist.Priors(ctx, doc)
			require.NoError(t, err)
			t2 := time.Now()
			results := reconcile.Blocks(doc, blocks, priors)
			t3 := time.Now()
			_, err = hist.DocumentHead(ctx, doc)
			require.NoError(t, err)
			t4 := time.Now()
			require.Len(t, results, len(blocks))
			readTime += t1.Sub(t0)
			priorTime += t2.Sub(t1)
			reconcileTime += t3.Sub(t2)
			headTime += t4.Sub(t3)
		}
	}
	per := func(d time.Duration) time.Duration { return d / passes }
	share := func(d time.Duration) float64 { return float64(d) / float64(readTime) * 100 }
	t.Logf("documents %d, blocks %d, passes %d (times per pass)", len(paths), blocksTotal, passes)
	t.Logf("read (mdx reader, whole corpus)  %v", per(readTime))
	t.Logf("priors from block_history        %v  %5.1f%%", per(priorTime), share(priorTime))
	t.Logf("reconcile.Blocks                 %v  %5.1f%%", per(reconcileTime), share(reconcileTime))
	t.Logf("history head (a cache hit)       %v  %5.1f%%", per(headTime), share(headTime))
	t.Logf("a read at a new revision: %.1f%% of read time", share(priorTime+reconcileTime))
}
