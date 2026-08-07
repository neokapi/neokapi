package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCancelJob_LeavesTerminalJobsAlone: cancelling a job that just finished
// used to flip it to failed / "cancelled by user", and the push aggregation
// then reported the whole push as partly failed. A completed job cannot be
// cancelled after the fact.
func TestCancelJob_LeavesTerminalJobsAlone(t *testing.T) {
	f := newBillingWorkerFixture(t)
	ctx := t.Context()

	job := f.chunkedJob("job-done", "ws-cancel", "en.json")
	require.NoError(t, f.deps.JobStore.CreateJob(ctx, job))
	_, epoch, err := f.deps.JobStore.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	owner, err := f.deps.JobStore.CompleteJob(ctx, job.ID, epoch)
	require.NoError(t, err)
	require.True(t, owner)

	cancelled, err := f.deps.JobStore.CancelJob(ctx, job.ID, "cancelled by user")
	require.NoError(t, err)
	assert.False(t, cancelled, "there was nothing left to cancel")

	got, err := f.deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, got.Status, "the result stands")
	assert.Empty(t, got.Error)
}

// TestCancelJob_TakesTheLeaseFromTheWorker: cancelling a RUNNING job used to
// stop nothing. The worker checks its lease at every chunk, and cancel left the
// epoch alone — so the worker kept translating, kept billing, and then wrote
// 'completed' back over the cancellation.
func TestCancelJob_TakesTheLeaseFromTheWorker(t *testing.T) {
	f := newBillingWorkerFixture(t)
	ctx := t.Context()

	job := f.chunkedJob("job-running", "ws-cancel", "en.json")
	require.NoError(t, f.deps.JobStore.CreateJob(ctx, job))
	_, epoch, err := f.deps.JobStore.ClaimJob(ctx, job.ID)
	require.NoError(t, err)

	owner, err := f.deps.JobStore.RenewLease(ctx, job.ID, epoch)
	require.NoError(t, err)
	require.True(t, owner, "the worker holds the lease before the cancellation")

	cancelled, err := f.deps.JobStore.CancelJob(ctx, job.ID, "cancelled by user")
	require.NoError(t, err)
	assert.True(t, cancelled)

	owner, err = f.deps.JobStore.RenewLease(ctx, job.ID, epoch)
	require.NoError(t, err)
	assert.False(t, owner, "the worker abandons at its next chunk boundary")

	// And its terminal write cannot resurrect the job.
	owner, err = f.deps.JobStore.CompleteJob(ctx, job.ID, epoch)
	require.NoError(t, err)
	assert.False(t, owner)

	got, err := f.deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, got.Status)
	assert.Equal(t, "cancelled by user", got.Error)
}

// TestCancelJob_StopsAQueuedJob: the simplest case, and the one cancel was
// always right about.
func TestCancelJob_StopsAQueuedJob(t *testing.T) {
	f := newBillingWorkerFixture(t)
	ctx := t.Context()

	job := f.chunkedJob("job-queued", "ws-cancel", "en.json")
	require.NoError(t, f.deps.JobStore.CreateJob(ctx, job))

	cancelled, err := f.deps.JobStore.CancelJob(ctx, job.ID, "cancelled by user")
	require.NoError(t, err)
	assert.True(t, cancelled)

	// A worker that dequeues the message afterwards finds nothing to claim.
	claimed, _, err := f.deps.JobStore.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	assert.False(t, claimed)
}
