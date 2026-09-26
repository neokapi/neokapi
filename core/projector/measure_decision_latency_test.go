package projector_test

import (
	"fmt"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
)

// TestMeasureDecisionLatency records decisions from 16 concurrent writers, the
// way a review pane and agents record them, through the ledger's journal into a
// log already holding 13,000 decisions. The budget is a p99 under 50 ms.
func TestMeasureDecisionLatency(t *testing.T) {
	measuring(t)
	p, _, db := open(t)
	ctx := t.Context()
	work := db.Work()
	work.SetJournal(p.Units())
	seed := make([]state.JournalEntry, 0, 13000)
	for i := range 13000 {
		u := state.UnitState{
			Unit: fmt.Sprintf("seed-%05d", i), Variant: model.Variant("nb"), Scope: "doc",
			TargetHash: state.TargetHash(strconv.Itoa(i)), ContentHash: state.SourceHash(strconv.Itoa(i)),
		}
		id, err := state.Address(u, "", false)
		require.NoError(t, err)
		seed = append(seed, state.JournalEntry{ID: id, State: u, Origin: state.OriginImport, Recorded: time.Now().UTC()})
	}
	require.NoError(t, p.Units().RecordEntries(ctx, seed))

	const writers, each = 16, 40
	var (
		mu    sync.Mutex
		spent []time.Duration
		wg    sync.WaitGroup
	)
	for w := range writers {
		wg.Go(func() {
			for i := range each {
				u := state.UnitState{
					Unit: fmt.Sprintf("w%02d-%03d", w, i), Variant: model.Variant("nb"), Scope: "doc",
					Status:      model.TargetStatusEstablished,
					Decision:    state.Decision{ReviewState: "approved", By: "agent/claude"},
					TargetHash:  state.TargetHash(fmt.Sprint(w, i)),
					ContentHash: state.SourceHash(fmt.Sprint(i, w)),
				}
				start := time.Now()
				err := work.Put(ctx, u)
				d := time.Since(start)
				require.NoError(t, err)
				mu.Lock()
				spent = append(spent, d)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	slices.Sort(spent)
	pct := func(q float64) time.Duration { return spent[int(q*float64(len(spent)-1))] }
	t.Logf("%d decisions from %d writers: p50 %s, p90 %s, p99 %s, max %s",
		len(spent), writers, pct(0.50), pct(0.90), pct(0.99), spent[len(spent)-1])
}
