package jobs

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestContextScanStore(t *testing.T) ContextScanJobStore {
	t.Helper()
	db := pgtest.NewTestDB(t)
	store, err := NewContextScanJobStore(db)
	require.NoError(t, err)
	return store
}

func seedContextScanJob(t *testing.T, store ContextScanJobStore, slug string) *ContextScanJob {
	t.Helper()
	req, err := json.Marshal(ContextScanRequest{PasteText: "We build Bowrain."})
	require.NoError(t, err)
	job := &ContextScanJob{
		ID:            id.New(),
		WorkspaceID:   "ws-" + slug,
		WorkspaceSlug: slug,
		Status:        ContextScanStatusQueued,
		Request:       req,
	}
	require.NoError(t, store.CreateContextScanJob(t.Context(), job))
	return job
}

// TestContextScanStore_MigrationIdempotent: constructing the store twice on the
// same database (server + worker both run the namespaced migrations) is safe.
func TestContextScanStore_MigrationIdempotent(t *testing.T) {
	db := pgtest.NewTestDB(t)
	_, err := NewContextScanJobStore(db)
	require.NoError(t, err)
	_, err = NewContextScanJobStore(db)
	require.NoError(t, err, "re-running the brand-scan migrations must be a no-op")
}

// TestContextScanStore_RoundTrip: request/result JSONB and every scalar column
// survive the store round-trip.
func TestContextScanStore_RoundTrip(t *testing.T) {
	store := newTestContextScanStore(t)
	ctx := t.Context()
	job := seedContextScanJob(t, store, "acme")

	got, err := store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, job.ID, got.ID)
	assert.Equal(t, "ws-acme", got.WorkspaceID)
	assert.Equal(t, "acme", got.WorkspaceSlug)
	assert.Equal(t, ContextScanStatusQueued, got.Status)
	assert.Empty(t, got.Result, "a fresh job has no result")

	var req ContextScanRequest
	require.NoError(t, json.Unmarshal(got.Request, &req))
	assert.Equal(t, "We build Bowrain.", req.PasteText)

	// Progress + message milestones are epoch-guarded: the claim comes first.
	claimed, epoch, err := store.ClaimContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, store.UpdateContextScanJobProgress(ctx, job.ID, epoch, 40, "assembling-corpus"))
	got, err = store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, 40, got.Progress)
	assert.Equal(t, "assembling-corpus", got.Message)

	// Completion persists result + tokens only under the live lease.
	result := json.RawMessage(`{"profile":{"name":"Draft"},"terms":[]}`)
	owner, err := store.CompleteContextScanJob(ctx, job.ID, epoch, result, 1234)
	require.NoError(t, err)
	assert.True(t, owner)

	got, err = store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ContextScanStatusCompleted, got.Status)
	assert.Equal(t, 100, got.Progress)
	assert.Equal(t, "done", got.Message)
	assert.Equal(t, 1234, got.TokensUsed)
	assert.JSONEq(t, string(result), string(got.Result))
}

// TestContextScanStore_ClaimOnlyOneWins: the atomic queued→processing claim
// admits exactly one worker; terminal jobs cannot be claimed.
func TestContextScanStore_ClaimOnlyOneWins(t *testing.T) {
	store := newTestContextScanStore(t)
	ctx := t.Context()
	job := seedContextScanJob(t, store, "claims")

	ok1, epoch1, err := store.ClaimContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	assert.True(t, ok1, "first claim should succeed")
	assert.Positive(t, epoch1)

	ok2, _, err := store.ClaimContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	assert.False(t, ok2, "second claim should lose")

	require.NoError(t, store.UpdateContextScanJobStatus(ctx, job.ID, ContextScanStatusCompleted, ""))
	ok3, _, err := store.ClaimContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	assert.False(t, ok3, "terminal jobs must not be claimable")
}

// TestContextScanStore_LeaseLostAfterReclaim: a sweep + fresh claim bumps the
// epoch, invalidating the abandoned worker's lease — RenewLease reports
// !owner and CompleteContextScanJob refuses to write.
func TestContextScanStore_LeaseLostAfterReclaim(t *testing.T) {
	store := newTestContextScanStore(t)
	ctx := t.Context()
	job := seedContextScanJob(t, store, "lease")

	claimed, epoch1, err := store.ClaimContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	owner, err := store.RenewLease(ctx, job.ID, epoch1)
	require.NoError(t, err)
	assert.True(t, owner, "live lease must renew")

	// Simulate the sweeper resurrecting the stalled job, then a fresh claim.
	requeued, failed, err := store.SweepStaleProcessing(ctx, -time.Minute, 5)
	require.NoError(t, err)
	require.Equal(t, []string{job.ID}, requeued)
	assert.Zero(t, failed)

	claimed2, epoch2, err := store.ClaimContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed2)
	assert.Greater(t, epoch2, epoch1, "re-claim must advance the lease epoch")

	owner, err = store.RenewLease(ctx, job.ID, epoch1)
	require.NoError(t, err)
	assert.False(t, owner, "the abandoned worker's lease must be invalid")

	wrote, err := store.CompleteContextScanJob(ctx, job.ID, epoch1, json.RawMessage(`{}`), 1)
	require.NoError(t, err)
	assert.False(t, wrote, "a stale epoch must not persist a result")

	// Every other stale write is equally inert: terminal failure, retry
	// bookkeeping, progress, and token accumulation must not disturb the fresh
	// owner's run (a stale failure would hide a completed, already-billed
	// draft; a stale retry would spawn a duplicate run).
	wrote, err = store.FailContextScanJob(ctx, job.ID, epoch1, "stale permanent error")
	require.NoError(t, err)
	assert.False(t, wrote, "a stale epoch must not fail the fresh owner's run")

	retry, err := store.RetryOrFail(ctx, job.ID, epoch1, 5, "stale transient error")
	require.NoError(t, err)
	assert.False(t, retry, "a stale epoch must not requeue the fresh owner's run")

	require.NoError(t, store.UpdateContextScanJobProgress(ctx, job.ID, epoch1, 99, "stale-progress"))
	require.NoError(t, store.AddContextScanTokens(ctx, job.ID, epoch1, 7777))

	got, err := store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ContextScanStatusProcessing, got.Status, "the fresh owner's run stays live")
	assert.Empty(t, got.Error)
	assert.NotEqual(t, 99, got.Progress, "stale progress must not land")
	assert.Zero(t, got.TokensUsed, "stale token accumulation must not land")
}

// TestContextScanStore_TokensSurviveFailure: token usage accumulated as phases
// are billed stays on the row when the scan later fails, so a failed job
// reports the tokens the workspace actually paid for.
func TestContextScanStore_TokensSurviveFailure(t *testing.T) {
	store := newTestContextScanStore(t)
	ctx := t.Context()
	job := seedContextScanJob(t, store, "tokens")

	claimed, epoch, err := store.ClaimContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	require.NoError(t, store.AddContextScanTokens(ctx, job.ID, epoch, 100))
	require.NoError(t, store.AddContextScanTokens(ctx, job.ID, epoch, 50))

	owner, err := store.FailContextScanJob(ctx, job.ID, epoch, "provider auth error after billed phases")
	require.NoError(t, err)
	assert.True(t, owner)

	got, err := store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ContextScanStatusFailed, got.Status)
	assert.Equal(t, 150, got.TokensUsed, "billed tokens must survive the failure")
}

// TestContextScanStore_RetryOrFail: budget left → queued (retry), exhausted →
// failed with the error recorded.
func TestContextScanStore_RetryOrFail(t *testing.T) {
	store := newTestContextScanStore(t)
	ctx := t.Context()
	job := seedContextScanJob(t, store, "retry")

	claimed, epoch, err := store.ClaimContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	retry, err := store.RetryOrFail(ctx, job.ID, epoch, 2, "provider 503")
	require.NoError(t, err)
	assert.True(t, retry, "first transient failure has budget left")

	got, err := store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ContextScanStatusQueued, got.Status)
	assert.Equal(t, 1, got.Attempts)

	claimed, epoch, err = store.ClaimContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	retry, err = store.RetryOrFail(ctx, job.ID, epoch, 2, "provider 503 again")
	require.NoError(t, err)
	assert.False(t, retry, "budget exhausted")

	got, err = store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ContextScanStatusFailed, got.Status)
	assert.Equal(t, "provider 503 again", got.Error)
}

// TestContextScanStore_SweepFailsExhausted: a stalled job with no retry budget
// is failed by the sweep rather than requeued.
func TestContextScanStore_SweepFailsExhausted(t *testing.T) {
	store := newTestContextScanStore(t)
	ctx := t.Context()
	job := seedContextScanJob(t, store, "sweepfail")

	claimed, _, err := store.ClaimContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	requeued, failed, err := store.SweepStaleProcessing(ctx, -time.Minute, 1)
	require.NoError(t, err)
	assert.Empty(t, requeued)
	assert.Equal(t, 1, failed)

	got, err := store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ContextScanStatusFailed, got.Status)
}

// TestContextScanStore_SweepExpiredUploads: only terminal jobs past the
// retention cutoff whose request still carries upload_keys are selected, and
// ClearContextScanUploadKeys removes them from the next sweep while preserving
// the rest of the request.
func TestContextScanStore_SweepExpiredUploads(t *testing.T) {
	store := newTestContextScanStore(t)
	ctx := t.Context()

	// A terminal job WITH upload keys (the cleanup candidate).
	req, err := json.Marshal(ContextScanRequest{
		PasteText:  "We build Bowrain.",
		UploadKeys: []string{"blob-a", "blob-b"},
	})
	require.NoError(t, err)
	withUploads := &ContextScanJob{
		ID: id.New(), WorkspaceSlug: "uploads", Status: ContextScanStatusQueued, Request: req,
	}
	require.NoError(t, store.CreateContextScanJob(ctx, withUploads))
	require.NoError(t, store.UpdateContextScanJobStatus(ctx, withUploads.ID, ContextScanStatusFailed, "boom"))

	// A terminal job WITHOUT upload keys, and a still-queued job with keys —
	// neither must be selected.
	noUploads := seedContextScanJob(t, store, "uploads")
	require.NoError(t, store.UpdateContextScanJobStatus(ctx, noUploads.ID, ContextScanStatusCompleted, ""))
	stillQueued := &ContextScanJob{
		ID: id.New(), WorkspaceSlug: "uploads", Status: ContextScanStatusQueued, Request: req,
	}
	require.NoError(t, store.CreateContextScanJob(ctx, stillQueued))

	// A fresh terminal row is protected by the retention window…
	got, err := store.SweepExpiredContextScanUploads(ctx, time.Hour, 10)
	require.NoError(t, err)
	assert.Empty(t, got, "rows inside the retention window must not be selected")

	// …and selected once the window has passed (negative olderThan = future
	// cutoff, the same trick the stale-sweep tests use).
	got, err = store.SweepExpiredContextScanUploads(ctx, -time.Minute, 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, withUploads.ID, got[0].JobID)
	assert.Equal(t, []string{"blob-a", "blob-b"}, got[0].UploadKeys)

	// Clearing the keys removes the job from the sweep and keeps the rest of
	// the request intact.
	require.NoError(t, store.ClearContextScanUploadKeys(ctx, withUploads.ID))
	got, err = store.SweepExpiredContextScanUploads(ctx, -time.Minute, 10)
	require.NoError(t, err)
	assert.Empty(t, got, "cleared jobs must not be re-selected")

	reloaded, err := store.GetContextScanJob(ctx, withUploads.ID)
	require.NoError(t, err)
	var cleared ContextScanRequest
	require.NoError(t, json.Unmarshal(reloaded.Request, &cleared))
	assert.Empty(t, cleared.UploadKeys)
	assert.Equal(t, "We build Bowrain.", cleared.PasteText, "the rest of the request survives the clear")
}

// TestContextScanStore_RevertSweepRequeue rolls a sweep-requeued row back to
// 'processing' with a stale heartbeat so the next sweep re-selects it.
func TestContextScanStore_RevertSweepRequeue(t *testing.T) {
	store := newTestContextScanStore(t)
	ctx := t.Context()
	job := seedContextScanJob(t, store, "revert")

	claimed, _, err := store.ClaimContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	requeued, _, err := store.SweepStaleProcessing(ctx, -time.Minute, 5)
	require.NoError(t, err)
	require.Equal(t, []string{job.ID}, requeued)

	require.NoError(t, store.RevertSweepRequeue(ctx, job.ID, time.Minute))

	got, err := store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ContextScanStatusProcessing, got.Status)
	assert.Zero(t, got.Attempts, "the revert must refund the sweep's attempt")

	// The aged heartbeat makes the next sweep pick the row up again.
	requeued, _, err = store.SweepStaleProcessing(ctx, time.Minute, 5)
	require.NoError(t, err)
	assert.Equal(t, []string{job.ID}, requeued)
}
