package projector_test

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
)

// These measure what the log costs at the size the dogfood project reaches.
// They take tens of seconds, so they run only when asked:
//
//	KAPI_MEASURE_OPLOG=1 go test -tags fts5 ./core/projector -run Measure -v
func measuring(t *testing.T) {
	t.Helper()
	if os.Getenv("KAPI_MEASURE_OPLOG") == "" {
		t.Skip("set KAPI_MEASURE_OPLOG=1 to measure")
	}
}

// TestMeasureRebuild times a rebuild at the dogfood project's size: 13,000
// content-memory entries written as single writes collected in a batch, 2,000
// more written one operation each, and 1,000 concepts written one operation
// each.
func TestMeasureRebuild(t *testing.T) {
	measuring(t)
	p, _, _ := open(t)
	ctx := t.Context()
	const entries, concepts = 13000, 1000
	start := time.Now()
	batch := p.Batch()
	for i := range entries {
		require.NoError(t, batch.Memory().Add(ctx, entry(fmt.Sprintf("m-%05d", i), fmt.Sprintf("Sentence %d", i), fmt.Sprintf("Setning %d", i))))
	}
	require.NoError(t, batch.Commit(ctx))
	for i := range concepts {
		require.NoError(t, p.Terms().AddConcept(ctx, concept(fmt.Sprintf("c-%04d", i), fmt.Sprintf("Term %d", i), model.TermPreferred)))
	}
	for i := range 2000 {
		require.NoError(t, p.Memory().Add(ctx, entry(fmt.Sprintf("s-%05d", i), fmt.Sprintf("Single %d", i), "x")))
	}
	t.Logf("wrote %d entries in one batch, %d concepts and 2000 entries one at a time in %s", entries, concepts, time.Since(start))

	start = time.Now()
	report, err := p.Rebuild(ctx)
	require.NoError(t, err)
	require.Empty(t, report.Failed)
	t.Logf("rebuilt %d operations (%v) in %s", report.Total(), report.Operations, time.Since(start))
}

// TestMeasureWriteLatency records single writes from 16 concurrent writers
// into a log already holding 13,000 operations, and reports the percentiles.
// The budget is a p99 under 50 ms.
func TestMeasureWriteLatency(t *testing.T) {
	measuring(t)
	p, _, db := open(t)
	ctx := t.Context()
	direct := os.Getenv("KAPI_MEASURE_DIRECT") != ""
	seeds := make([]memory.Entry, 0, 13000)
	for i := range 13000 {
		seeds = append(seeds, entry(fmt.Sprintf("seed-%05d", i), fmt.Sprintf("Seed %d", i), "x"))
	}
	require.NoError(t, p.Memory().BulkAddWithStream(ctx, seeds, ""))
	require.NoError(t, p.Memory().RebuildSearchIndex(ctx))
	require.NoError(t, p.Memory().RebuildFuzzyIndex(ctx))
	for i := range 200 {
		require.NoError(t, p.Terms().AddConcept(ctx, concept(fmt.Sprintf("seed-%04d", i), fmt.Sprintf("Seed %d", i), model.TermPreferred)))
	}

	const writers, each = 16, 40
	var (
		mu    sync.Mutex
		spent []time.Duration
		wg    sync.WaitGroup
	)
	for w := range writers {
		wg.Go(func() {
			for i := range each {
				start := time.Now()
				var err error
				switch {
				case direct && i%2 == 0:
					err = db.Terms().AddConcept(ctx, concept(fmt.Sprintf("w%02d-%03d", w, i), fmt.Sprintf("Word %d %d", w, i), model.TermPreferred))
				case direct:
					err = db.Memory().Add(ctx, memory.Entry{
						ID: fmt.Sprintf("w%02d-%03d", w, i), HintSrcLang: "en",
						Variants: entry("x", fmt.Sprintf("Line %d %d", w, i), "y").Variants,
					})
				case i%2 == 0:
					err = p.Terms().AddConcept(ctx, concept(fmt.Sprintf("w%02d-%03d", w, i), fmt.Sprintf("Word %d %d", w, i), model.TermPreferred))
				default:
					err = p.Memory().Add(ctx, memory.Entry{
						ID: fmt.Sprintf("w%02d-%03d", w, i), HintSrcLang: "en",
						Variants: entry("x", fmt.Sprintf("Line %d %d", w, i), "y").Variants,
					})
				}
				d := time.Since(start)
				require.NoError(t, err)
				mu.Lock()
				spent = append(spent, d)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	sort.Slice(spent, func(i, j int) bool { return spent[i] < spent[j] })
	pct := func(q float64) time.Duration { return spent[int(q*float64(len(spent)-1))] }
	t.Logf("%d writes from %d writers: p50 %s, p90 %s, p99 %s, max %s",
		len(spent), writers, pct(0.50), pct(0.90), pct(0.99), spent[len(spent)-1])
}
