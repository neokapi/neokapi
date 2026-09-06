package jobs

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedExtractionJob(t *testing.T, store ExtractionJobStore) *ExtractionJob {
	t.Helper()
	job := &ExtractionJob{
		ID:            uuid.NewString(),
		WorkspaceSlug: "ws",
		ProjectID:     "proj",
		ItemName:      "en.json",
		Status:        ExtractionStatusQueued,
	}
	require.NoError(t, store.CreateExtractionJob(t.Context(), job))
	return job
}

// TestExtractionStore_RetryOrFailSpendsItsBudget: one provider 503 used to
// strand an extraction in 'processing' forever — the nack was discarded, the
// ack unconditional, and there was no attempts column to retry against.
func TestExtractionStore_RetryOrFailSpendsItsBudget(t *testing.T) {
	store := newTestExtractionStore(t)
	ctx := t.Context()
	job := seedExtractionJob(t, store)

	for attempt := 1; attempt <= 2; attempt++ {
		claimed, epoch, err := store.ClaimExtractionJob(ctx, job.ID)
		require.NoError(t, err)
		require.True(t, claimed, "attempt %d", attempt)

		retry, err := store.RetryOrFail(ctx, job.ID, epoch, 3, "provider 503")
		require.NoError(t, err)
		assert.True(t, retry, "attempt %d still has budget", attempt)

		got, err := store.GetExtractionJob(ctx, job.ID)
		require.NoError(t, err)
		assert.Equal(t, ExtractionStatusQueued, got.Status)
	}

	// Third failure exhausts the budget.
	claimed, epoch, err := store.ClaimExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	retry, err := store.RetryOrFail(ctx, job.ID, epoch, 3, "provider 503")
	require.NoError(t, err)
	assert.False(t, retry)

	got, err := store.GetExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ExtractionStatusFailed, got.Status)
	assert.Contains(t, got.Error, "provider 503")
}

// TestExtractionStore_StaleWorkerLosesItsLease: the epoch guard, without which
// a worker resurrected by the sweeper could still write over its replacement.
func TestExtractionStore_StaleWorkerLosesItsLease(t *testing.T) {
	store := newTestExtractionStore(t)
	ctx := t.Context()
	job := seedExtractionJob(t, store)

	_, stale, err := store.ClaimExtractionJob(ctx, job.ID)
	require.NoError(t, err)

	// The sweeper puts it back and a fresh worker claims it.
	requeued, failed, err := store.SweepStaleProcessing(ctx, -time.Second, 3)
	require.NoError(t, err)
	require.Equal(t, []string{job.ID}, requeued)
	assert.Zero(t, failed)

	_, fresh, err := store.ClaimExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Greater(t, fresh, stale)

	owner, err := store.RenewLease(ctx, job.ID, stale)
	require.NoError(t, err)
	assert.False(t, owner, "the abandoned worker no longer owns the job")

	owner, err = store.FailExtractionJob(ctx, job.ID, stale, "too late")
	require.NoError(t, err)
	assert.False(t, owner)

	got, err := store.GetExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ExtractionStatusProcessing, got.Status, "the fresh owner's run is untouched")
}

// TestExtractionStore_SweepFailsWhatIsOutOfBudget: the other half of the sweep,
// so a job cannot cycle through 'processing' forever.
func TestExtractionStore_SweepFailsWhatIsOutOfBudget(t *testing.T) {
	store := newTestExtractionStore(t)
	ctx := t.Context()
	job := seedExtractionJob(t, store)

	_, _, err := store.ClaimExtractionJob(ctx, job.ID)
	require.NoError(t, err)

	requeued, failed, err := store.SweepStaleProcessing(ctx, -time.Second, 1)
	require.NoError(t, err)
	assert.Empty(t, requeued)
	assert.Equal(t, 1, failed)

	got, err := store.GetExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ExtractionStatusFailed, got.Status)
	assert.Contains(t, got.Error, "exceeded max attempts")
}

// TestExtractionStore_RevertSweepRequeueRestoresTheRow: a sweep whose
// re-enqueue failed must leave the row where the NEXT sweep can find it, since
// nothing scans 'queued' orphans.
func TestExtractionStore_RevertSweepRequeueRestoresTheRow(t *testing.T) {
	store := newTestExtractionStore(t)
	ctx := t.Context()
	job := seedExtractionJob(t, store)

	_, _, err := store.ClaimExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	requeued, _, err := store.SweepStaleProcessing(ctx, -time.Second, 3)
	require.NoError(t, err)
	require.Equal(t, []string{job.ID}, requeued)

	require.NoError(t, store.RevertSweepRequeue(ctx, job.ID, time.Minute))

	got, err := store.GetExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ExtractionStatusProcessing, got.Status)

	// Stale enough for the next sweep to select it, with its budget restored.
	requeued, _, err = store.SweepStaleProcessing(ctx, time.Minute, 3)
	require.NoError(t, err)
	assert.Equal(t, []string{job.ID}, requeued)
}

// TestExtractionStore_CancelStopsAQueuedJob: a cancel on a job nobody has
// claimed marks it terminal with the reason, so a worker that picks up the
// message afterwards finds nothing to claim.
func TestExtractionStore_CancelStopsAQueuedJob(t *testing.T) {
	store := newTestExtractionStore(t)
	ctx := t.Context()
	job := seedExtractionJob(t, store)

	cancelled, err := store.CancelExtractionJob(ctx, job.ID, "cancelled by user")
	require.NoError(t, err)
	assert.True(t, cancelled)

	got, err := store.GetExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ExtractionStatusFailed, got.Status)
	assert.Contains(t, got.Error, "cancelled by user")

	claimed, _, err := store.ClaimExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	assert.False(t, claimed, "a cancelled job was claimed anyway")
}

// TestExtractionStore_CancelReachesTheRunningWorker: the claim epoch is what
// carries a cancellation to a worker already processing the job. Its next
// renewal reports !owner, which is the checkpoint it abandons at, and its
// completion write is refused.
func TestExtractionStore_CancelReachesTheRunningWorker(t *testing.T) {
	store := newTestExtractionStore(t)
	ctx := t.Context()
	job := seedExtractionJob(t, store)

	claimed, epoch, err := store.ClaimExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	owner, err := store.RenewLease(ctx, job.ID, epoch)
	require.NoError(t, err)
	require.True(t, owner, "the worker holds the lease before the cancel")

	cancelled, err := store.CancelExtractionJob(ctx, job.ID, "run cancelled")
	require.NoError(t, err)
	require.True(t, cancelled)

	owner, err = store.RenewLease(ctx, job.ID, epoch)
	require.NoError(t, err)
	assert.False(t, owner, "the cancelled worker still holds its lease")

	owner, err = store.CompleteExtractionJob(ctx, job.ID, epoch)
	require.NoError(t, err)
	assert.False(t, owner, "a cancelled worker wrote completed over the cancellation")

	got, err := store.GetExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ExtractionStatusFailed, got.Status)
	assert.Contains(t, got.Error, "run cancelled")
}

// TestExtractionStore_CancelRefusesATerminalJob: a job that already finished
// cannot be cancelled after the fact, and reporting it stopped would be a lie
// the run history then repeats.
func TestExtractionStore_CancelRefusesATerminalJob(t *testing.T) {
	store := newTestExtractionStore(t)
	ctx := t.Context()
	job := seedExtractionJob(t, store)

	claimed, epoch, err := store.ClaimExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	owner, err := store.CompleteExtractionJob(ctx, job.ID, epoch)
	require.NoError(t, err)
	require.True(t, owner)

	cancelled, err := store.CancelExtractionJob(ctx, job.ID, "too late")
	require.NoError(t, err)
	assert.False(t, cancelled)

	got, err := store.GetExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ExtractionStatusCompleted, got.Status)
	assert.Empty(t, got.Error)
}

// TestExtractionStore_CompleteRefusesAStaleWorker: the sweeper's re-claim
// takes the lease, and the worker it replaced cannot report success for a run
// somebody else now owns.
func TestExtractionStore_CompleteRefusesAStaleWorker(t *testing.T) {
	store := newTestExtractionStore(t)
	ctx := t.Context()
	job := seedExtractionJob(t, store)

	_, stale, err := store.ClaimExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	require.NoError(t, store.UpdateExtractionJobStatus(ctx, job.ID, ExtractionStatusQueued, ""))
	_, fresh, err := store.ClaimExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	require.NotEqual(t, stale, fresh)

	owner, err := store.CompleteExtractionJob(ctx, job.ID, stale)
	require.NoError(t, err)
	assert.False(t, owner)

	owner, err = store.CompleteExtractionJob(ctx, job.ID, fresh)
	require.NoError(t, err)
	assert.True(t, owner)
}
