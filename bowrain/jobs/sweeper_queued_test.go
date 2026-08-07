package jobs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSweep_RecoversAStrandedQueuedRow: a job whose enqueue failed after the
// row was written is stranded — the broker has no message and, until now, the
// sweep only looked at 'processing'. Nothing else scans for it.
func TestSweep_RecoversAStrandedQueuedRow(t *testing.T) {
	f := newBillingWorkerFixture(t)
	ctx := t.Context()

	job := f.chunkedJob("job-orphan", "ws-orphan", "en.json")
	require.NoError(t, f.deps.JobStore.CreateJob(ctx, job))

	// It has sat in 'queued' with nothing delivering it.
	requeued, failed, err := f.deps.JobStore.SweepStaleProcessing(ctx, -time.Second, 3)
	require.NoError(t, err)
	assert.Equal(t, []string{job.ID}, requeued, "the orphan is picked up")
	assert.Zero(t, failed)

	got, err := f.deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusQueued, got.Status)
}

// TestSweep_LeavesAFreshQueuedRowAlone keeps the widened predicate from
// re-enqueueing everything: a job queued a moment ago is waiting, not stranded.
func TestSweep_LeavesAFreshQueuedRowAlone(t *testing.T) {
	f := newBillingWorkerFixture(t)
	ctx := t.Context()

	job := f.chunkedJob("job-waiting", "ws-orphan", "en.json")
	require.NoError(t, f.deps.JobStore.CreateJob(ctx, job))

	requeued, failed, err := f.deps.JobStore.SweepStaleProcessing(ctx, time.Hour, 3)
	require.NoError(t, err)
	assert.Empty(t, requeued)
	assert.Zero(t, failed)
}
