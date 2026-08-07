package jobs

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDrainableJobContext_SurvivesTheSignal: the job body must not see the
// shutdown signal. Deriving it from the signal context is what made every
// deploy fail the work in flight with context.Canceled.
func TestDrainableJobContext_SurvivesTheSignal(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, stop := drainableJobContext(parent, time.Hour)
	defer stop()

	cancelParent()
	time.Sleep(50 * time.Millisecond)
	assert.NoError(t, ctx.Err(), "the job keeps running through the signal")
}

// TestDrainableJobContext_EndsWhenTheGraceDoes: the grace is a bound, not a
// reprieve. Past it the job is cancelled deliberately, so it can be parked
// before the orchestrator kills the task.
func TestDrainableJobContext_EndsWhenTheGraceDoes(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, stop := drainableJobContext(parent, 50*time.Millisecond)
	defer stop()

	cancelParent()
	select {
	case <-ctx.Done():
		require.ErrorIs(t, ctx.Err(), context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("the grace never expired")
	}
}

// TestDrainableJobContext_StopReleasesTheWatchdog keeps shutdown (and the
// worker's goleak check) clean: stop blocks until the watchdog has exited.
func TestDrainableJobContext_StopReleasesTheWatchdog(t *testing.T) {
	ctx, stop := drainableJobContext(context.Background(), time.Hour)
	stop()
	assert.ErrorIs(t, ctx.Err(), context.Canceled)
}

// cancellingLeaseStore stands in for the drain deadline expiring mid-run: the
// nth lease read is where the grace runs out.
type cancellingLeaseStore struct {
	JobStore
	mu     sync.Mutex
	cancel context.CancelFunc
	at     int
	calls  int
}

func (s *cancellingLeaseStore) RenewLease(ctx context.Context, id string, epoch int64) (bool, error) {
	s.mu.Lock()
	s.calls++
	fire := s.calls == s.at
	s.mu.Unlock()
	owner, err := s.JobStore.RenewLease(ctx, id, epoch)
	if fire {
		s.cancel()
	}
	return owner, err
}

// TestWorkerShutdown_ParksTheJobInsteadOfFailingIt is the deploy-window
// property. SIGTERM used to fail the running job with context.Canceled —
// deliberately non-transient, so not retried — while the queue message was
// deleted anyway, leaving the row for the fifteen-minute stale sweeper on
// another instance at attempts + 1. It must go back to 'queued' instead, with
// its retry budget intact.
func TestWorkerShutdown_ParksTheJobInsteadOfFailingIt(t *testing.T) {
	f := newBillingWorkerFixture(t)
	ctx := t.Context()
	f.seedChunkedItem(t, "drain.json", 3*translationProgressChunk)
	require.NoError(t, f.billing.GrantCredits(ctx, "ws-drain", 5_000_000, billing.SourcePlan))

	job := f.chunkedJob("job-drain", "ws-drain", "drain.json")
	require.NoError(t, f.deps.JobStore.CreateJob(ctx, job))

	// The grace expires during the second chunk, exactly as it would mid-deploy.
	drained, cancel := context.WithCancel(ctx)
	defer cancel()
	f.deps.JobStore = &cancellingLeaseStore{JobStore: f.deps.JobStore, cancel: cancel, at: 2}

	err := processJobWithDeps(drained, f.deps, job.ID)
	var de *deferredError
	require.ErrorAs(t, err, &de, "an interrupted job is parked, not failed")
	assert.Equal(t, "worker-shutdown", de.dependency)

	got, err := f.deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusQueued, got.Status, "the row goes back for the next worker")
	assert.Equal(t, translationProgressChunk, got.DoneBlocks,
		"the chunk that finished is recorded, so the next worker resumes there")

	attempts, deferrals := f.budget(t, job.ID)
	assert.Equal(t, 0, attempts, "a shutdown is not the job's fault")
	assert.Equal(t, 1, deferrals)
}

// TestWorkerShutdown_ExhaustedDeferralsStillFail keeps the park bounded: a job
// that keeps being interrupted — a crash-looping deploy — must terminate rather
// than cycle through the queue forever.
func TestWorkerShutdown_ExhaustedDeferralsStillFail(t *testing.T) {
	f := newBillingWorkerFixture(t)
	ctx := t.Context()
	f.deps.MaxJobDeferrals = 1
	f.seedChunkedItem(t, "drain-loop.json", 2*translationProgressChunk)
	require.NoError(t, f.billing.GrantCredits(ctx, "ws-drain-loop", 5_000_000, billing.SourcePlan))

	job := f.chunkedJob("job-drain-loop", "ws-drain-loop", "drain-loop.json")
	require.NoError(t, f.deps.JobStore.CreateJob(ctx, job))

	drained, cancel := context.WithCancel(ctx)
	defer cancel()
	f.deps.JobStore = &cancellingLeaseStore{JobStore: f.deps.JobStore, cancel: cancel, at: 1}

	err := processJobWithDeps(drained, f.deps, job.ID)
	require.Error(t, err)
	var de *deferredError
	assert.NotErrorAs(t, err, &de, "an exhausted deferral budget is terminal")

	got, err := f.deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, got.Status)
}

// TestJobStore_CompleteJobIsLeaseGuarded: the terminal success write is the
// last one that was not under the lease, and it is the one that wrote
// 'completed' back over a cancellation.
func TestJobStore_CompleteJobIsLeaseGuarded(t *testing.T) {
	f := newBillingWorkerFixture(t)
	ctx := t.Context()

	job := f.chunkedJob("job-complete", "ws-complete", "en.json")
	require.NoError(t, f.deps.JobStore.CreateJob(ctx, job))
	_, epoch, err := f.deps.JobStore.ClaimJob(ctx, job.ID)
	require.NoError(t, err)

	owner, err := f.deps.JobStore.CompleteJob(ctx, job.ID, epoch+1)
	require.NoError(t, err)
	assert.False(t, owner, "a stale epoch owns nothing")

	owner, err = f.deps.JobStore.CompleteJob(ctx, job.ID, epoch)
	require.NoError(t, err)
	assert.True(t, owner)

	got, err := f.deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, got.Status)
}
