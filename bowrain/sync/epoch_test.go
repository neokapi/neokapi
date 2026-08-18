package sync

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedStream(t *testing.T, cs platstore.ContentStore, projectID, name string) {
	t.Helper()
	require.NoError(t, cs.CreateStream(t.Context(), &platstore.Stream{
		ProjectID: projectID,
		Name:      name,
	}))
}

// A stream that has never been told a generation refuses nobody: there is
// nothing stored to downgrade.
func TestContentModelEpoch_AnUnmarkedStreamAcceptsAnyone(t *testing.T) {
	engine, cs := newTestDiffEngine(t)
	seedProject(t, cs, "proj-epoch")
	seedStream(t, cs, "proj-epoch", "main")

	assert.NoError(t, engine.CheckContentModelEpoch(t.Context(), "proj-epoch", "main", 0))
	assert.NoError(t, engine.CheckContentModelEpoch(t.Context(), "proj-epoch", "main", 3))
}

// The refusal, which is the point: a producer whose model is older than the
// content it would overwrite is told so, and told what to do about it.
func TestContentModelEpoch_AnOlderProducerIsRefused(t *testing.T) {
	engine, cs := newTestDiffEngine(t)
	seedProject(t, cs, "proj-epoch")
	seedStream(t, cs, "proj-epoch", "main")
	ctx := t.Context()

	require.NoError(t, engine.RecordContentModelEpoch(ctx, "proj-epoch", "main", 2))

	err := engine.CheckContentModelEpoch(ctx, "proj-epoch", "main", 1)
	require.Error(t, err)
	var conflict *ContentModelConflict
	require.ErrorAs(t, err, &conflict)
	assert.Equal(t, 1, conflict.Stated)
	assert.Equal(t, 2, conflict.Recorded)
	assert.Contains(t, conflict.Error(), "push --force",
		"a refusal that does not say how to proceed deliberately is a dead end")
}

// A producer that states nothing predates the mechanism, so it cannot be
// assumed to write what the stream holds.
func TestContentModelEpoch_ANonStatingProducerReadsAsTheFirstGeneration(t *testing.T) {
	engine, cs := newTestDiffEngine(t)
	seedProject(t, cs, "proj-epoch")
	seedStream(t, cs, "proj-epoch", "main")
	ctx := t.Context()

	require.NoError(t, engine.RecordContentModelEpoch(ctx, "proj-epoch", "main", venue.ContentModelEpoch))
	assert.Error(t, engine.CheckContentModelEpoch(ctx, "proj-epoch", "main", 0))
}

// The same generation is not a downgrade — the ordinary case must not be
// refused.
func TestContentModelEpoch_TheSameGenerationPasses(t *testing.T) {
	engine, cs := newTestDiffEngine(t)
	seedProject(t, cs, "proj-epoch")
	seedStream(t, cs, "proj-epoch", "main")
	ctx := t.Context()

	require.NoError(t, engine.RecordContentModelEpoch(ctx, "proj-epoch", "main", 2))
	assert.NoError(t, engine.CheckContentModelEpoch(ctx, "proj-epoch", "main", 2))
	assert.NoError(t, engine.CheckContentModelEpoch(ctx, "proj-epoch", "main", 5))
}

// Recording never lowers the mark. A forced downgrade writes what it writes,
// but the stream still holds what the richer producer wrote for everything that
// push did not touch — so the guard stays in force for the next one.
func TestRecordContentModelEpoch_NeverLowersTheMark(t *testing.T) {
	engine, cs := newTestDiffEngine(t)
	seedProject(t, cs, "proj-epoch")
	seedStream(t, cs, "proj-epoch", "main")
	ctx := t.Context()

	require.NoError(t, engine.RecordContentModelEpoch(ctx, "proj-epoch", "main", 3))
	require.NoError(t, engine.RecordContentModelEpoch(ctx, "proj-epoch", "main", 1))

	assert.Error(t, engine.CheckContentModelEpoch(ctx, "proj-epoch", "main", 2))
}

// A stream the store does not hold yet holds no content either, so recording is
// a no-op rather than an error that would fail a committed push.
func TestRecordContentModelEpoch_AnUnknownStreamIsNotAnError(t *testing.T) {
	engine, cs := newTestDiffEngine(t)
	seedProject(t, cs, "proj-epoch")

	assert.NoError(t, engine.RecordContentModelEpoch(t.Context(), "proj-epoch", "nope", 2))
	assert.NoError(t, engine.CheckContentModelEpoch(t.Context(), "proj-epoch", "nope", 1))
}
