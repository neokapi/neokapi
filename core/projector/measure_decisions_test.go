package projector_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
)

// TestMeasureDecisionRebuild times the dogfood project's decision record: 13,000
// ledger entries, read in the way an import reads a record, then a rebuild
// from the log, a checkpoint, and a rebuild from that checkpoint.
func TestMeasureDecisionRebuild(t *testing.T) {
	measuring(t)
	p, _, _ := open(t)
	ctx := t.Context()
	const n = 13000
	entries := make([]state.JournalEntry, 0, n)
	now := time.Now().UTC()
	for i := range n {
		u := state.UnitState{
			Unit: fmt.Sprintf("u-%05d", i), Variant: model.Variant("nb"), Scope: fmt.Sprintf("doc-%03d", i%720),
			Status:      model.TargetStatusReviewed,
			Decision:    state.Decision{ReviewState: "approved", By: "reviewer"},
			TargetHash:  state.TargetHash(fmt.Sprintf("Mål %d", i)),
			ContentHash: state.SourceHash(fmt.Sprintf("Source %d", i)),
		}
		id, err := state.Address(u, "reviewer", false)
		require.NoError(t, err)
		entries = append(entries, state.JournalEntry{ID: id, State: u, Actor: "reviewer", Origin: state.OriginImport, Recorded: now})
	}
	start := time.Now()
	require.NoError(t, p.Units().RecordEntries(ctx, entries))
	t.Logf("recorded %d decisions as one import in %s", n, time.Since(start))

	start = time.Now()
	report, err := p.Rebuild(ctx)
	require.NoError(t, err)
	require.Empty(t, report.Failed)
	t.Logf("rebuilt %d operations from the log in %s", report.Total(), time.Since(start))

	start = time.Now()
	cp, err := p.Checkpoint(ctx)
	require.NoError(t, err)
	t.Logf("wrote a checkpoint of %d operations (%d bytes) in %s", cp.Operations, cp.Bytes, time.Since(start))

	start = time.Now()
	report, err = p.Rebuild(ctx)
	require.NoError(t, err)
	require.Equal(t, cp.Through, report.Checkpoint)
	t.Logf("rebuilt from the checkpoint in %s", time.Since(start))
}
