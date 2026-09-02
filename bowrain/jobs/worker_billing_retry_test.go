package jobs

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flakyLeaseStore fails one RenewLease call, which is how a translation dies
// between two chunks: the chunks before it are billed and persisted, the ones
// after it were never reached.
type flakyLeaseStore struct {
	JobStore
	mu     sync.Mutex
	failAt int
	calls  int
}

func (s *flakyLeaseStore) RenewLease(ctx context.Context, id string, epoch int64) (bool, error) {
	s.mu.Lock()
	s.calls++
	n := s.calls
	s.mu.Unlock()
	if s.failAt > 0 && n == s.failAt {
		return false, errors.New("connection reset by peer")
	}
	return s.JobStore.RenewLease(ctx, id, epoch)
}

// renewals counts lease reads, one per chunk the run reached.
func (s *flakyLeaseStore) renewals() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// seedChunkedItem writes enough blocks to span several progress chunks, so the
// job bills more than once and a mid-run failure has something to leave behind.
func (f *billingWorkerFixture) seedChunkedItem(t *testing.T, item string, blocks int) {
	t.Helper()
	bs := make([]*model.Block, 0, blocks)
	for i := range blocks {
		bs = append(bs, model.NewBlock(fmt.Sprintf("b%d", i), fmt.Sprintf("Sentence number %d.", i)))
	}
	require.NoError(t, f.deps.ContentStore.StoreBlocksForItem(t.Context(), f.projectID, "main", item, bs))
}

func (f *billingWorkerFixture) chunkedJob(id, wsID, item string) *TranslationJob {
	return &TranslationJob{
		ID:               id,
		WorkspaceSlug:    "acme",
		WorkspaceID:      wsID,
		ProjectID:        f.projectID,
		ItemName:         item,
		TargetLocale:     "fr",
		ProviderConfigID: "platform",
		Model:            "demo",
		Status:           StatusQueued,
	}
}

// spent reports how many credits a workspace has been charged.
func (f *billingWorkerFixture) spent(t *testing.T, wsID string, granted int64) int64 {
	t.Helper()
	remaining, err := f.billing.CheckCredits(t.Context(), wsID)
	require.NoError(t, err)
	return granted - remaining
}

// TestWorkerBilling_RetryAfterPartialRunChargesOnce is the money property of a
// retried translation. A job that dies after billing some of its chunks goes
// back to 'queued' and restarts from chunk zero, so every chunk it already paid
// for arrives at the ledger a second time. It must cost what one run costs.
func TestWorkerBilling_RetryAfterPartialRunChargesOnce(t *testing.T) {
	f := newBillingWorkerFixture(t)
	ctx := t.Context()
	const (
		grant    = int64(5_000_000)
		retried  = "ws-retried"
		straight = "ws-straight"
	)
	// Two items of the same shape, one per job. A run records the source each
	// target it wrote was translated from, so a second job over the SAME item
	// would find every unit settled and translate nothing: the control needs
	// content of its own to price.
	f.seedChunkedItem(t, "chunked.json", 3*translationProgressChunk)
	f.seedChunkedItem(t, "chunked-control.json", 3*translationProgressChunk)
	require.NoError(t, f.billing.GrantCredits(ctx, retried, grant, billing.SourcePlan))
	require.NoError(t, f.billing.GrantCredits(ctx, straight, grant, billing.SourcePlan))

	// The control: the same content, translated once, with nothing going wrong.
	control := f.chunkedJob("job-straight", straight, "chunked-control.json")
	require.NoError(t, f.deps.JobStore.CreateJob(ctx, control))
	claimed, epoch, err := f.deps.JobStore.ClaimJob(ctx, control.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, executeTranslationWithDeps(ctx, f.deps, control, epoch))
	oneRun := f.spent(t, straight, grant)
	require.Positive(t, oneRun)

	// The retried job: the lease read after the second chunk fails, so chunk one
	// is billed and the rest is not.
	flaky := &flakyLeaseStore{JobStore: f.deps.JobStore, failAt: 2}
	f.deps.JobStore = flaky
	job := f.chunkedJob("job-retried", retried, "chunked.json")
	require.NoError(t, f.deps.JobStore.CreateJob(ctx, job))
	claimed, epoch, err = f.deps.JobStore.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	require.Error(t, executeTranslationWithDeps(ctx, f.deps, job, epoch),
		"a failed lease read must abort the run")

	partial := f.spent(t, retried, grant)
	require.Positive(t, partial, "the chunks before the failure were billed")
	require.Less(t, partial, oneRun, "the chunks after it were not")

	// The retry: requeued, re-claimed, and restarted from chunk zero.
	requeued, err := f.deps.JobStore.RetryOrFail(ctx, job.ID, epoch, 3, "connection reset by peer")
	require.NoError(t, err)
	require.True(t, requeued)
	flaky.failAt = 0
	retry, err := f.deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, err)
	claimed, epoch, err = f.deps.JobStore.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, executeTranslationWithDeps(ctx, f.deps, retry, epoch))

	assert.Equal(t, oneRun, f.spent(t, retried, grant),
		"a job that failed late and retried costs one job")
}

// TestWorkerTranslate_RetryResumesWhereItStopped is the work half of the same
// story. The ledger guard makes a redone chunk free; resuming makes it undone.
// The finished chunks are already in the overlay table, so the retry has only
// the remainder to translate — and the provider is asked for it once.
func TestWorkerTranslate_RetryResumesWhereItStopped(t *testing.T) {
	f := newBillingWorkerFixture(t)
	ctx := t.Context()
	const (
		grant   = int64(5_000_000)
		wsID    = "ws-resume"
		chunks  = 3
		nBlocks = chunks * translationProgressChunk
	)
	f.seedChunkedItem(t, "resume.json", nBlocks)
	require.NoError(t, f.billing.GrantCredits(ctx, wsID, grant, billing.SourcePlan))

	flaky := &flakyLeaseStore{JobStore: f.deps.JobStore, failAt: 2}
	f.deps.JobStore = flaky
	job := f.chunkedJob("job-resume", wsID, "resume.json")
	require.NoError(t, f.deps.JobStore.CreateJob(ctx, job))
	claimed, epoch, err := f.deps.JobStore.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	require.Error(t, executeTranslationWithDeps(ctx, f.deps, job, epoch))

	stopped, err := f.deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, translationProgressChunk, stopped.DoneBlocks,
		"the chunk that completed is recorded as done")
	require.Equal(t, translationProgressChunk, countTranslated(t, f, "fr"),
		"and its translations are already persisted")

	requeued, err := f.deps.JobStore.RetryOrFail(ctx, job.ID, epoch, 3, "connection reset by peer")
	require.NoError(t, err)
	require.True(t, requeued)
	flaky.failAt = 0
	retry, err := f.deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, err)
	claimed, epoch, err = f.deps.JobStore.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	before := flaky.renewals()
	require.NoError(t, executeTranslationWithDeps(ctx, f.deps, retry, epoch))

	assert.Equal(t, chunks-1, flaky.renewals()-before,
		"the retry works the remaining chunks and no more")
	assert.Equal(t, nBlocks, countTranslated(t, f, "fr"), "every block ends up translated")
}

// countTranslated counts the project's blocks carrying a target for a locale.
func countTranslated(t *testing.T, f *billingWorkerFixture, locale model.LocaleID) int {
	t.Helper()
	blocks, err := f.deps.ContentStore.GetBlocks(t.Context(), store.BlockQuery{
		ProjectID: f.projectID,
		Stream:    "main",
		ItemName:  "resume.json",
	})
	require.NoError(t, err)
	n := 0
	for _, b := range blocks {
		if b.Block.Target(locale) != nil {
			n++
		}
	}
	return n
}

// TestWorkerBilling_RedeliveredJobChargesOnce covers the other at-least-once
// door: the queue redelivers a message whose job already ran to completion.
func TestWorkerBilling_RedeliveredJobChargesOnce(t *testing.T) {
	f := newBillingWorkerFixture(t)
	ctx := t.Context()
	const (
		grant = int64(5_000_000)
		wsID  = "ws-redelivered"
	)
	f.seedChunkedItem(t, "redelivered.json", 2*translationProgressChunk)
	require.NoError(t, f.billing.GrantCredits(ctx, wsID, grant, billing.SourcePlan))

	job := f.chunkedJob("job-redelivered", wsID, "redelivered.json")
	require.NoError(t, f.deps.JobStore.CreateJob(ctx, job))

	var first int64
	for pass := range 3 {
		require.NoError(t, f.deps.JobStore.UpdateJobStatus(ctx, job.ID, StatusQueued, ""))
		claimed, epoch, err := f.deps.JobStore.ClaimJob(ctx, job.ID)
		require.NoError(t, err)
		require.True(t, claimed)
		reloaded, err := f.deps.JobStore.GetJob(ctx, job.ID)
		require.NoError(t, err)
		require.NoError(t, executeTranslationWithDeps(ctx, f.deps, reloaded, epoch))

		spent := f.spent(t, wsID, grant)
		if pass == 0 {
			first = spent
			require.Positive(t, first)
			continue
		}
		assert.Equal(t, first, spent, "delivery %d must be free", pass+1)
	}
}

// TestContextScanWorker_RetryChargesOncePerPhase is the brand-scan half of the
// same property: its two phases carry per-phase reference ids, and a re-run
// re-derives both.
func TestContextScanWorker_RetryChargesOncePerPhase(t *testing.T) {
	f := newContextScanFixture(t)
	bs, err := billing.NewPgBillingStore(f.db)
	require.NoError(t, err)
	f.deps.BillingHooks = &billing.UsageHooks{Store: bs}
	ctx := t.Context()

	const (
		grant = int64(5_000_000)
		wsID  = "ws-scan-retried"
	)
	require.NoError(t, bs.GrantCredits(ctx, wsID, grant, billing.SourcePlan))

	job := f.seedJob(t, wsID, ContextScanRequest{PasteText: demoPasteText})
	require.NoError(t, processContextScanJob(ctx, f.deps, job.ID))

	remaining, err := bs.CheckCredits(ctx, wsID)
	require.NoError(t, err)
	oneScan := grant - remaining
	require.Positive(t, oneScan)

	// Requeue and run it again, exactly as a redelivered queue message would.
	f.requeueJob(t, job.ID)
	require.NoError(t, processContextScanJob(ctx, f.deps, job.ID))

	remaining, err = bs.CheckCredits(ctx, wsID)
	require.NoError(t, err)
	assert.Equal(t, oneScan, grant-remaining, "a re-run scan costs one scan")

	var rows int
	require.NoError(t, f.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM credit_ledger WHERE workspace_id = $1 AND operation = 'context_scan'`,
		wsID).Scan(&rows))
	assert.Equal(t, 3, rows, "one ledger entry per phase (infer, terms, axes), however many times the scan runs")
}
