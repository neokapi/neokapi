package projector_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/workspace"
)

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
