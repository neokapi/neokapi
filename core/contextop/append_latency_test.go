package contextop_test

import (
	"fmt"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/workspace"
)

// TestAppendLatency measures what an agent pays to record one observation
// while fifteen others do the same, in a workspace whose log already holds a
// dogfood-sized history: thousands of operations of other kinds and a few
// hundred context operations. The budget is a p99 under 50 ms.
//
//	KAPI_MEASURE_OPLOG=1 go test -tags fts5 ./core/contextop -run TestAppendLatency -v
func TestAppendLatency(t *testing.T) {
	if os.Getenv("KAPI_MEASURE_OPLOG") == "" {
		t.Skip("set KAPI_MEASURE_OPLOG=1 to measure the latency of recording a context operation")
	}
	const (
		otherOps   = 13_000
		contextOps = 500
		writers    = 16
		each       = 20
		budget     = 50 * time.Millisecond
	)
	ctx := t.Context()
	ws := openWorkspace(t)

	batch := make([]workspace.Op, 0, 1000)
	flush := func() {
		if len(batch) > 0 {
			_, err := ws.Record(ctx, batch...)
			require.NoError(t, err)
			batch = batch[:0]
		}
	}
	for i := range otherOps {
		batch = append(batch, workspace.Op{
			Project: "prj_docs", Kind: "unit.decide", Address: fmt.Sprintf("unit-%d", i),
			Payload: []byte(`{"unit":"u","variant":"nb","status":"established"}`),
		})
		if len(batch) == cap(batch) {
			flush()
		}
	}
	flush()
	ledger := contextop.NewLedger(ws, contextop.Allow)
	for i := range contextOps {
		_, err := ledger.Append(ctx, contextop.Record{
			Project: "prj_docs", Actor: agent("claude", "seed"),
			Kind: contextop.KindObserve, Subject: termRule(fmt.Sprintf("word%d", i), fmt.Sprintf("form%d", i), false),
		})
		require.NoError(t, err)
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
				_, err := ledger.Append(ctx, contextop.Record{
					Project: "prj_docs", Actor: agent("claude", fmt.Sprintf("s%d", w)),
					Kind:    contextop.KindObserve,
					Subject: contextop.Subject{Kind: contextop.SubjectNote, Text: fmt.Sprintf("note %d/%d", w, i)},
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
	t.Logf("%d appends by %d writers over %d other and %d context operations: p50 %s, p99 %s, max %s",
		len(latencies), writers, otherOps, contextOps, p(0.50), p(0.99), latencies[len(latencies)-1])
	require.Less(t, p(0.99), budget, "p99 append latency is inside the budget")
}
