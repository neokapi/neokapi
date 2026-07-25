package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/resilience"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func deferralsOf(t *testing.T, db *storage.PgDB, id string) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT deferrals FROM translation_jobs WHERE id = $1`, id).Scan(&n))
	return n
}

// TestDeferJob_DoesNotSpendRetryBudget is the core of the graceful-degradation
// contract. An AI provider being down is not the job's fault: the call was never
// made, so waiting must cost the job nothing it will need once the provider
// returns.
func TestDeferJob_DoesNotSpendRetryBudget(t *testing.T) {
	store, db := newPgJobStore(t)
	ctx := context.Background()

	job := newQueuedJob(t, store)
	ok, epoch, err := store.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, ok)

	deferred, err := store.DeferJob(ctx, job.ID, epoch, 10, "deferred: ai:bedrock unavailable")
	require.NoError(t, err)
	assert.True(t, deferred)

	got, err := store.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusQueued, got.Status, "a deferred job goes back on the queue")
	assert.Equal(t, 1, deferralsOf(t, db, job.ID))
	assert.Zero(t, attemptsOf(t, db, job.ID),
		"a deferral must not consume a retry attempt — nothing was attempted")

	// The job must be re-claimable so a later delivery can run it.
	ok, _, err = store.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	assert.True(t, ok)
}

// TestDeferJob_SurvivesLongOutageThenRetriesInFull proves the two budgets stay
// independent: a job can wait out a long outage and still arrive at the provider
// with its whole retry budget intact.
func TestDeferJob_SurvivesLongOutageThenRetriesInFull(t *testing.T) {
	store, db := newPgJobStore(t)
	ctx := context.Background()

	job := newQueuedJob(t, store)

	// 30 deferral cycles — far more than any retry budget would allow.
	for i := range 30 {
		ok, epoch, err := store.ClaimJob(ctx, job.ID)
		require.NoError(t, err)
		require.True(t, ok, "cycle %d: a deferred job must remain claimable", i)

		deferred, err := store.DeferJob(ctx, job.ID, epoch, 100, "deferred: ai:bedrock unavailable")
		require.NoError(t, err)
		require.True(t, deferred, "cycle %d: still within the deferral budget", i)
	}
	assert.Equal(t, 30, deferralsOf(t, db, job.ID))
	assert.Zero(t, attemptsOf(t, db, job.ID))

	got, err := store.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusQueued, got.Status,
		"an outage costs waiting, not a dead job")

	// The provider is back and genuinely failing now: the full retry budget is
	// still available.
	ok, epoch, err := store.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, ok)
	retry, err := store.RetryOrFail(ctx, job.ID, epoch, 3, "boom")
	require.NoError(t, err)
	assert.True(t, retry, "the retry budget was never touched by the deferrals")
}

// TestDeferJob_BudgetExhaustedFails covers the other half: an upstream that
// never comes back must terminate the job rather than cycle it forever.
func TestDeferJob_BudgetExhaustedFails(t *testing.T) {
	store, db := newPgJobStore(t)
	ctx := context.Background()

	job := newQueuedJob(t, store)

	ok, epoch, err := store.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, ok)
	deferred, err := store.DeferJob(ctx, job.ID, epoch, 2, "deferred: ai:bedrock unavailable")
	require.NoError(t, err)
	require.True(t, deferred)

	ok, epoch, err = store.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, ok)
	deferred, err = store.DeferJob(ctx, job.ID, epoch, 2, "ai:bedrock did not recover")
	require.NoError(t, err)
	assert.False(t, deferred, "1 + 1 >= 2: the deferral budget is spent")

	got, err := store.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, got.Status)
	assert.Equal(t, "ai:bedrock did not recover", got.Error)
	assert.Equal(t, 2, deferralsOf(t, db, job.ID))
	assert.Zero(t, attemptsOf(t, db, job.ID))
}

// TestDeferJob_StaleEpochIsNoop mirrors the guard RetryOrFail carries: an
// abandoned worker must not requeue the fresh owner's in-flight job.
func TestDeferJob_StaleEpochIsNoop(t *testing.T) {
	store, db := newPgJobStore(t)
	ctx := context.Background()

	job := newQueuedJob(t, store)
	ok, epoch, err := store.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, ok)

	deferred, err := store.DeferJob(ctx, job.ID, epoch-1, 10, "stale")
	require.NoError(t, err)
	assert.False(t, deferred)
	assert.Zero(t, deferralsOf(t, db, job.ID))

	got, err := store.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusProcessing, got.Status,
		"the fresh owner's run must be left alone")
}

func TestDeferJob_NonProcessingIsNoop(t *testing.T) {
	store, _ := newPgJobStore(t)
	ctx := context.Background()

	job := newQueuedJob(t, store) // never claimed → still 'queued'
	deferred, err := store.DeferJob(ctx, job.ID, 0, 10, "nope")
	require.NoError(t, err)
	assert.False(t, deferred)
}

// --- deferral classification -------------------------------------------------

func TestDeferralForRecognisesOpenCircuitOnly(t *testing.T) {
	open := &resilience.UnavailableError{
		Dependency: "ai:bedrock",
		Kind:       resilience.KindAI,
		RetryAfter: 25 * time.Second,
	}
	d := deferralFor(open)
	require.NotNil(t, d)
	assert.Equal(t, "ai:bedrock", d.dependency)
	assert.Equal(t, 25*time.Second, d.retryAfter)
	assert.ErrorIs(t, d, resilience.ErrUnavailable)

	// A wrapped rejection still reads as a deferral — the error crosses several
	// layers between the provider guard and the worker.
	require.NotNil(t, deferralFor(wrap(open)))
	// A mere string resembling one does not: the classification is typed.
	assert.Nil(t, deferralFor(errors.New("x: "+open.Error())))

	// Everything else goes down the ordinary retry/fail path.
	assert.Nil(t, deferralFor(nil))
	assert.Nil(t, deferralFor(errors.New("openai: API error 503: unavailable")))
	assert.Nil(t, deferralFor(errors.New("project not found")))
}

func wrap(err error) error { return &wrapped{err} }

type wrapped struct{ err error }

func (w *wrapped) Error() string { return "translate blocks: " + w.err.Error() }
func (w *wrapped) Unwrap() error { return w.err }

// TestIsTransientErrorExcludesOpenCircuit is the guard against the double-count
// bug: if an open-circuit rejection also classified as transient, a deferred job
// would ALSO burn a retry attempt and the degradation would be pointless.
func TestIsTransientErrorExcludesOpenCircuit(t *testing.T) {
	open := &resilience.UnavailableError{Dependency: "ai:bedrock", Kind: resilience.KindAI}
	assert.False(t, isTransientError(open))
	assert.False(t, isTransientError(wrap(open)))

	// Real upstream trouble still retries.
	assert.True(t, isTransientError(errors.New("anthropic: API error 529: overloaded")))
	assert.True(t, isTransientError(errors.New("dial tcp: connection refused")))
	// Permanent faults still fail fast.
	assert.False(t, isTransientError(errors.New("openai: API error 401: bad key")))
}

// --- delayed re-enqueue ------------------------------------------------------

// plainQueue is a Queue with no delay support, to exercise the fallback.
type plainQueue struct {
	mu       sync.Mutex
	enqueued []string
}

func (q *plainQueue) Enqueue(_ context.Context, id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.enqueued = append(q.enqueued, id)
	return nil
}

func (q *plainQueue) Dequeue(context.Context) (string, func(), func(), error) {
	return "", nil, nil, errors.New("not used")
}
func (q *plainQueue) Healthy() bool { return true }
func (q *plainQueue) Close() error  { return nil }

// delayedQueue records the delay it was asked for.
type delayedQueue struct {
	plainQueue
	delays []time.Duration
}

func (q *delayedQueue) EnqueueAfter(_ context.Context, id string, d time.Duration) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.enqueued = append(q.enqueued, id)
	q.delays = append(q.delays, d)
	return nil
}

func TestEnqueueAfterUsesBrokerDelayWhenAvailable(t *testing.T) {
	q := &delayedQueue{}
	require.NoError(t, enqueueAfter(t.Context(), q, "job-1", 30*time.Second))
	assert.Equal(t, []string{"job-1"}, q.enqueued)
	assert.Equal(t, []time.Duration{30 * time.Second}, q.delays,
		"the broker should do the waiting, not the worker")
}

func TestEnqueueAfterFallsBackToImmediate(t *testing.T) {
	q := &plainQueue{}
	require.NoError(t, enqueueAfter(t.Context(), q, "job-1", 30*time.Second))
	assert.Equal(t, []string{"job-1"}, q.enqueued,
		"a queue with no delay support must still redeliver, just sooner")
}

func TestEnqueueAfterZeroDelayIsImmediate(t *testing.T) {
	q := &delayedQueue{}
	require.NoError(t, enqueueAfter(t.Context(), q, "job-1", 0))
	assert.Equal(t, []string{"job-1"}, q.enqueued)
	assert.Empty(t, q.delays, "a non-positive delay must not go through the delay path")
}

func TestChannelQueueEnqueueAfterDelivers(t *testing.T) {
	q := NewChannelQueue(4)
	defer q.Close()

	require.NoError(t, q.EnqueueAfter(t.Context(), "job-1", 20*time.Millisecond))

	// Nothing yet: the point of a deferral is that it is not immediately visible.
	select {
	case got := <-q.ch:
		t.Fatalf("job %q delivered before its delay elapsed", got)
	case <-time.After(5 * time.Millisecond):
	}

	select {
	case got := <-q.ch:
		assert.Equal(t, "job-1", got)
	case <-time.After(2 * time.Second):
		t.Fatal("deferred job was never delivered")
	}
}

func TestChannelQueueEnqueueAfterOnClosedQueue(t *testing.T) {
	q := NewChannelQueue(1)
	require.NoError(t, q.Close())
	assert.Error(t, q.EnqueueAfter(t.Context(), "job-1", time.Millisecond))
}

func TestChannelQueueEnqueueAfterHonoursContext(t *testing.T) {
	q := NewChannelQueue(1)
	defer q.Close()

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, q.EnqueueAfter(ctx, "job-1", time.Hour))
	cancel() // a shutting-down worker must not leave the timer behind

	select {
	case got := <-q.ch:
		t.Fatalf("cancelled deferral delivered %q", got)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestMaxJobDeferralsDefaultsAndOverride(t *testing.T) {
	assert.Equal(t, defaultMaxJobDeferrals, (&WorkerDeps{}).maxJobDeferrals())
	assert.Equal(t, 7, (&WorkerDeps{MaxJobDeferrals: 7}).maxJobDeferrals())

	// The two budgets must be independent, and waiting must be the cheaper one.
	assert.Greater(t, defaultMaxJobDeferrals, defaultMaxJobAttempts,
		"a job should out-wait an outage far longer than it retries a failure")
}

func TestQueueLabelDefaults(t *testing.T) {
	assert.Equal(t, "translation", (&WorkerDeps{}).queueLabel())
	assert.Equal(t, "extraction", (&WorkerDeps{QueueName: "extraction"}).queueLabel())
}
