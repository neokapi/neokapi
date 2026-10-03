package host

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
)

// An import counts the decisions it records. A shard line an earlier release
// wrote for what a pass produced is left out of the ledger, and out of the
// count the import reports.
func TestImportDecisionRecord_CountsTheDecisionsItRecords(t *testing.T) {
	dir := t.TempDir()
	committed := filepath.Join(dir, "units")
	st, err := state.OpenWork(t.Context(), filepath.Join(dir, "work.db"), committed)
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	produced := state.UnitState{Unit: "produced", Variant: model.Variant("nb"), Scope: "d-1",
		ContentHash: state.SourceHash("Hello"), TargetHash: state.TargetHash("Hei")}
	decided := state.UnitState{Unit: "decided", Variant: model.Variant("nb"), Scope: "d-1",
		Status: model.TargetStatusEstablished, Decision: state.Decision{ReviewState: "approved"},
		ContentHash: state.SourceHash("Goodbye"), TargetHash: state.TargetHash("Ha det")}
	require.NoError(t, state.WriteCommitted(committed, []state.UnitState{produced, decided}))

	n, err := importDecisionRecord(t.Context(), st, committed)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}
