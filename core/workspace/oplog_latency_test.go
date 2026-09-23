package workspace_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/workspace"
)

// TestOperationLogWriteLatency measures what an agent pays to record one
// operation while fifteen others do the same, against a log that already holds
// a dogfood-sized history. The budget is a p99 under 50 ms.
//
// Timing depends on the machine, so the test only runs when asked:
//
//	KAPI_MEASURE_OPLOG=1 go test -tags fts5 ./core/workspace -run TestOperationLogWriteLatency -v
func TestOperationLogWriteLatency(t *testing.T) {
	if os.Getenv("KAPI_MEASURE_OPLOG") == "" {
		t.Skip("set KAPI_MEASURE_OPLOG=1 to measure the operation log's write latency")
	}
	const (
		seeded  = 20_000
		writers = 16
		each    = 100
		budget  = 50 * time.Millisecond
	)
	ctx := t.Context()
	b := workspace.Local(filepath.Join(t.TempDir(), "workspaces", "default"))
	t.Cleanup(func() { _ = b.Close() })

	batch := make([]workspace.Op, 0, 1000)
	for i := range seeded {
		batch = append(batch, workspace.Op{
			Project: "prj_docs", Kind: "context.observe", Address: fmt.Sprintf("seed-%d", i),
			Payload: []byte(`{"actor":{"kind":"agent"},"subject":{"kind":"note","text":"seeded"}}`),
		})
		if len(batch) == cap(batch) {
			_, err := b.Record(ctx, batch...)
			require.NoError(t, err)
			batch = batch[:0]
		}
	}

	var (
		mu        sync.Mutex
		latencies []time.Duration
		wg        sync.WaitGroup
	)
	for w := range writers {
		wg.Go(func() {
			for i := range each {
				start := time.Now()
				_, err := b.Record(ctx, workspace.Op{
					Project: "prj_docs", Kind: "context.observe",
					Payload: fmt.Appendf(nil, `{"actor":{"kind":"agent","session":"s%d"},"subject":{"kind":"note","text":"%d"}}`, w, i),
				})
				took := time.Since(start)
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				latencies = append(latencies, took)
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	slices.Sort(latencies)
	p := func(q float64) time.Duration { return latencies[int(q*float64(len(latencies)-1))] }
	t.Logf("%d writes by %d writers over a %d-operation log: p50 %s, p99 %s, max %s",
		len(latencies), writers, seeded, p(0.50), p(0.99), latencies[len(latencies)-1])
	require.Less(t, p(0.99), budget, "p99 write latency is inside the budget")
}
