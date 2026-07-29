package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingEnqueueQueue is a Queue whose Enqueue always fails, to exercise the
// sweeper's recovery when the broker is down exactly when recovery matters most.
type failingEnqueueQueue struct{}

func (failingEnqueueQueue) Enqueue(context.Context, string) error {
	return errors.New("broker unavailable")
}
func (failingEnqueueQueue) Dequeue(context.Context) (string, func(), func(), error) {
	return "", nil, nil, errors.New("not used")
}
func (failingEnqueueQueue) Healthy() bool { return false }
func (failingEnqueueQueue) Close() error  { return nil }

// newPgJobStore returns a migrated JobStore plus the underlying PgDB (for
// direct SQL that seeds attempt/updated_at state the public API can't set).
// The database is an isolated schema from pgtest, so the tests below never
// see each other's rows and never touch a neighbouring package's tables.
func newPgJobStore(t *testing.T) (JobStore, *storage.PgDB) {
	t.Helper()
	db := pgtest.NewTestDB(t)
	store, err := NewJobStore(db)
	require.NoError(t, err)
	return store, db
}

func newQueuedJob(t *testing.T, store JobStore) *TranslationJob {
	t.Helper()
	job := &TranslationJob{
		ID:            id.New(),
		WorkspaceSlug: "ws",
		ProjectID:     "proj",
		ItemName:      "en.json",
		TargetLocale:  "fr",
		Status:        StatusQueued,
	}
	require.NoError(t, store.CreateJob(context.Background(), job))
	return job
}

func attemptsOf(t *testing.T, db *storage.PgDB, id string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT attempts FROM translation_jobs WHERE id = $1`, id).Scan(&n))
	return n
}

// backdateUpdatedAt ages a job's updated_at so the stale sweeper considers it.
func backdateUpdatedAt(t *testing.T, db *storage.PgDB, id string, age time.Duration) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`UPDATE translation_jobs SET updated_at = $1 WHERE id = $2`,
		time.Now().UTC().Add(-age), id)
	require.NoError(t, err)
}

func TestRetryOrFail_RequeuesUntilBudgetExhausted(t *testing.T) {
	store, db := newPgJobStore(t)
	ctx := context.Background()

	job := newQueuedJob(t, store)
	ok, epoch, err := store.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, ok)

	// Attempt 1: budget left (0 < 3) → requeued.
	retry, err := store.RetryOrFail(ctx, job.ID, epoch, 3, "boom-1")
	require.NoError(t, err)
	assert.True(t, retry, "first transient failure should requeue")
	got, err := store.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusQueued, got.Status)
	assert.Equal(t, 1, attemptsOf(t, db, job.ID))

	// A requeued job can be re-claimed.
	ok, epoch, err = store.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, ok, "requeued job must be re-claimable")

	// A stale-epoch retry (an abandoned worker's transient error) is a no-op:
	// it must not knock the fresh claim back to 'queued' (spawning a duplicate
	// run) or eat its retry budget.
	retry, err = store.RetryOrFail(ctx, job.ID, epoch-1, 3, "stale boom")
	require.NoError(t, err)
	assert.False(t, retry, "stale epoch must not requeue")
	assert.Equal(t, 1, attemptsOf(t, db, job.ID), "stale epoch must not eat retry budget")

	// Attempt 2: budget left (1 < 3) → requeued.
	retry, err = store.RetryOrFail(ctx, job.ID, epoch, 3, "boom-2")
	require.NoError(t, err)
	assert.True(t, retry)
	assert.Equal(t, 2, attemptsOf(t, db, job.ID))

	ok, epoch, err = store.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, ok)

	// Attempt 3: budget exhausted (2 + 1 >= 3) → failed, no more retries.
	retry, err = store.RetryOrFail(ctx, job.ID, epoch, 3, "boom-final")
	require.NoError(t, err)
	assert.False(t, retry, "exhausted budget must not requeue")
	got, err = store.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, got.Status)
	assert.Equal(t, "boom-final", got.Error)
	assert.Equal(t, 3, attemptsOf(t, db, job.ID))

	// A failed job is no longer claimable.
	ok, _, err = store.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRetryOrFail_NonProcessingIsNoop(t *testing.T) {
	store, _ := newPgJobStore(t)
	ctx := context.Background()

	job := newQueuedJob(t, store) // still 'queued', never claimed
	retry, err := store.RetryOrFail(ctx, job.ID, 0, 3, "boom")
	require.NoError(t, err)
	assert.False(t, retry, "a non-processing job must not be requeued")

	got, err := store.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusQueued, got.Status, "status unchanged")
}

func TestSweepStaleProcessing_RequeuesAndFails(t *testing.T) {
	store, db := newPgJobStore(t)
	ctx := context.Background()

	// A: stale processing with budget → requeued.
	jobA := newQueuedJob(t, store)
	_, _, err := store.ClaimJob(ctx, jobA.ID)
	require.NoError(t, err)
	backdateUpdatedAt(t, db, jobA.ID, 30*time.Minute)

	// B: fresh processing (recently claimed) → untouched.
	jobB := newQueuedJob(t, store)
	_, _, err = store.ClaimJob(ctx, jobB.ID)
	require.NoError(t, err)

	// C: stale processing already at the attempt ceiling → failed.
	jobC := newQueuedJob(t, store)
	_, _, err = store.ClaimJob(ctx, jobC.ID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE translation_jobs SET attempts = 2 WHERE id = $1`, jobC.ID)
	require.NoError(t, err)
	backdateUpdatedAt(t, db, jobC.ID, 30*time.Minute)

	requeued, failed, err := store.SweepStaleProcessing(ctx, 15*time.Minute, 3)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{jobA.ID}, requeued)
	assert.Equal(t, 1, failed)

	gotA, err := store.GetJob(ctx, jobA.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusQueued, gotA.Status)
	assert.Equal(t, 1, attemptsOf(t, db, jobA.ID))

	gotB, err := store.GetJob(ctx, jobB.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusProcessing, gotB.Status, "fresh job is not swept")

	gotC, err := store.GetJob(ctx, jobC.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, gotC.Status)
	assert.Contains(t, gotC.Error, "stalled in processing")
}

// TestClaimJob_LeaseEpochInvalidatesAbandonedWorker proves the double-run guard
// primitive: ClaimJob hands out a strictly increasing claim_epoch, and
// RenewLease only succeeds for the current lease holder. A worker whose job was
// swept and re-claimed by a fresh worker (epoch advanced) can no longer renew —
// its subsequent writes are locked out, so it cannot double-bill the fresh run.
func TestClaimJob_LeaseEpochInvalidatesAbandonedWorker(t *testing.T) {
	store, db := newPgJobStore(t)
	ctx := context.Background()

	job := newQueuedJob(t, store)

	ok, e1, err := store.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Positive(t, e1, "claim must issue a positive lease epoch")

	// The current holder can renew (heartbeat) and keeps ownership.
	owner, err := store.RenewLease(ctx, job.ID, e1)
	require.NoError(t, err)
	assert.True(t, owner, "the claim holder must be able to renew its lease")

	// A second claim of a still-processing job fails (no epoch handed out).
	ok, _, err = store.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	assert.False(t, ok, "a processing job cannot be re-claimed")

	// Simulate the sweeper resurrecting the job: requeue it, then a fresh worker
	// re-claims it — which bumps the epoch past the first worker's lease.
	_, err = db.ExecContext(ctx, `UPDATE translation_jobs SET status='queued' WHERE id=$1`, job.ID)
	require.NoError(t, err)
	ok, e2, err := store.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Greater(t, e2, e1, "each claim must advance the epoch")

	// The abandoned first worker (holding e1) has lost the lease: RenewLease is a
	// no-op, signalling it to stop before double-billing.
	owner, err = store.RenewLease(ctx, job.ID, e1)
	require.NoError(t, err)
	assert.False(t, owner, "a stale-epoch holder must not be able to renew a re-claimed job")

	// The fresh worker (e2) owns it.
	owner, err = store.RenewLease(ctx, job.ID, e2)
	require.NoError(t, err)
	assert.True(t, owner, "the fresh claimant owns the lease")

	// RenewLease never resurrects a terminal job.
	require.NoError(t, store.UpdateJobStatus(ctx, job.ID, StatusCompleted, ""))
	owner, err = store.RenewLease(ctx, job.ID, e2)
	require.NoError(t, err)
	assert.False(t, owner, "a completed job cannot be renewed")
}

// TestStaleJobSweeper_RevertsOnEnqueueFailure proves a stale job the sweeper
// flips to 'queued' is NOT stranded when the follow-up Enqueue fails: it is
// rolled back to 'processing' with a stale heartbeat (and the sweep's attempts
// bump undone), so the NEXT sweep re-selects and re-enqueues it once the broker
// recovers. Without the revert the row sits 'queued' with no live message and
// nothing (the sweep scans only 'processing') ever recovers it.
func TestStaleJobSweeper_RevertsOnEnqueueFailure(t *testing.T) {
	store, db := newPgJobStore(t)
	ctx := context.Background()

	job := newQueuedJob(t, store)
	_, _, err := store.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	backdateUpdatedAt(t, db, job.ID, 30*time.Minute)
	require.Equal(t, 0, attemptsOf(t, db, job.ID))

	// First sweep: broker down → Enqueue fails → job reverted, not stranded.
	down := NewStaleJobSweeper(store, failingEnqueueQueue{}, 15*time.Minute, time.Minute, 3)
	requeued, failed, err := down.sweepOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, requeued, "a failed enqueue must not count as recovered")
	assert.Equal(t, 0, failed)

	got, err := store.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusProcessing, got.Status,
		"a job whose re-enqueue failed must be reverted to processing, not stranded in queued")
	assert.Equal(t, 0, attemptsOf(t, db, job.ID),
		"the failed sweep must not consume retry budget")

	// Second sweep with a working broker: the reverted job's stale updated_at
	// makes it re-selectable, and it is successfully re-enqueued.
	queue := NewChannelQueue(8)
	t.Cleanup(func() { _ = queue.Close() })
	up := NewStaleJobSweeper(store, queue, 15*time.Minute, time.Minute, 3)
	requeued, failed, err = up.sweepOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, requeued, "the reverted job must be recovered by the next sweep")
	assert.Equal(t, 0, failed)

	gotID, ack, _, err := queue.Dequeue(ctx)
	require.NoError(t, err)
	assert.Equal(t, job.ID, gotID)
	ack()

	after, err := store.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusQueued, after.Status, "after a successful re-enqueue the job is queued for a worker")
}

func TestStaleJobSweeper_SweepOnceReenqueues(t *testing.T) {
	store, db := newPgJobStore(t)
	ctx := context.Background()

	job := newQueuedJob(t, store)
	_, _, err := store.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	backdateUpdatedAt(t, db, job.ID, 30*time.Minute)

	queue := NewChannelQueue(8)
	t.Cleanup(func() { _ = queue.Close() })

	sweeper := NewStaleJobSweeper(store, queue, 15*time.Minute, time.Minute, 3)
	requeued, failed, err := sweeper.sweepOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, requeued)
	assert.Equal(t, 0, failed)

	// The requeued job ID must land on the queue so a worker picks it up again.
	gotID, ack, _, err := queue.Dequeue(ctx)
	require.NoError(t, err)
	assert.Equal(t, job.ID, gotID)
	ack()
}
