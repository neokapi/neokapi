package host

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
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

// An import stamps the decision record by a digest over its shards, the way
// it stamps a file by its bytes. A second run of the same import reads
// nothing, counts the record with the sources already in the store, and says
// so; a record that moved is read again.
func TestImportProjectContext_StampsTheDecisionRecord(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	ctx := context.Background()
	recordDir := project.LayoutAt(root).Export().UnitStateDir()
	decided := state.UnitState{Unit: "greeting", Variant: model.Variant("nb"), Scope: "d-1",
		Status: model.TargetStatusEstablished, Decision: state.Decision{ReviewState: "approved", By: "reviewer"},
		ContentHash: state.SourceHash("Hello"), TargetHash: state.TargetHash("Hei")}
	require.NoError(t, state.WriteCommitted(recordDir, []state.UnitState{decided}))

	first, err := a.ImportProjectContext(ctx, recipe, ContextImportRequest{})
	require.NoError(t, err)
	assert.Equal(t, 1, first.Decisions)

	second, err := a.ImportProjectContext(ctx, recipe, ContextImportRequest{})
	require.NoError(t, err)
	assert.Equal(t, 0, second.Decisions, "a record at bytes this checkout has read is skipped")
	assert.False(t, second.Read())
	assert.Positive(t, second.Unchanged, "the record counts among the sources already read")
	var out bytes.Buffer
	require.NoError(t, second.FormatText(&out))
	assert.Contains(t, out.String(), "Nothing to read:", out.String())

	forced, err := a.ImportProjectContext(ctx, recipe, ContextImportRequest{Force: true})
	require.NoError(t, err)
	assert.Equal(t, 1, forced.Decisions, "--force reads the record again")

	another := state.UnitState{Unit: "farewell", Variant: model.Variant("nb"), Scope: "d-1",
		Status: model.TargetStatusEstablished, Decision: state.Decision{ReviewState: "approved", By: "reviewer"},
		ContentHash: state.SourceHash("Goodbye"), TargetHash: state.TargetHash("Ha det")}
	require.NoError(t, state.WriteCommitted(recordDir, []state.UnitState{decided, another}))
	third, err := a.ImportProjectContext(ctx, recipe, ContextImportRequest{})
	require.NoError(t, err)
	assert.Equal(t, 2, third.Decisions, "a record whose shards moved is read again")
}
