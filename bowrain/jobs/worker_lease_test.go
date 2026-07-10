package jobs

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resurrectingQuota is a QuotaStore that, on the FIRST RecordUsage, simulates
// the stale-job sweeper resurrecting the still-live job: it requeues the row
// (RetryOrFail with budget) and a "fresh worker" re-claims it, bumping
// claim_epoch past the running worker's lease. It counts RecordUsage calls so a
// test can assert the abandoned worker stops billing once it loses the lease.
type resurrectingQuota struct {
	store JobStore
	jobID string
	calls int
}

func (q *resurrectingQuota) CheckQuota(context.Context, string) (int64, error) {
	return 1_000_000, nil
}

func (q *resurrectingQuota) GetUsageSummary(context.Context, string) (*UsageSummary, error) {
	return &UsageSummary{}, nil
}

func (q *resurrectingQuota) RecordUsage(ctx context.Context, _ AIUsageRecord) error {
	q.calls++
	if q.calls == 1 {
		// Requeue the (apparently stale) processing job, then re-claim it as a
		// fresh worker — advancing claim_epoch past the running worker's lease.
		retry, err := q.store.RetryOrFail(ctx, q.jobID, defaultMaxJobAttempts, "simulated stall")
		if err != nil {
			return err
		}
		if !retry {
			return errors.New("resurrection setup: job did not requeue")
		}
		ok, _, err := q.store.ClaimJob(ctx, q.jobID)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("resurrection setup: fresh re-claim failed")
		}
	}
	return nil
}

// TestExecuteTranslation_LeaseLostStopsBilling proves finding 4's fix: when a
// live long-running job is swept and re-claimed mid-run, the abandoned worker's
// epoch guard rejects its further deductions and it does not overwrite the fresh
// owner's job. Chunk 1 bills, the resurrection happens during that billing, and
// the chunk-2 lease gate aborts before RecordUsage runs again.
func TestExecuteTranslation_LeaseLostStopsBilling(t *testing.T) {
	deps := newTestWorkerDeps(t) // real Pg JobStore + ContentStore (+ blob)
	ctx := context.Background()

	deps.Platform = &PlatformProviderConfig{Provider: "demo"} // offline, no network
	quota := &resurrectingQuota{store: deps.JobStore}
	deps.QuotaStore = quota

	projectID := "lease-proj-" + id.New() // unique: projects persist across runs
	require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{
		ID: projectID, Name: "Lease", DefaultSourceLanguage: "en",
	}))

	// 60 source blocks in one item → two progress chunks (progressChunk=50).
	const nBlocks = 60
	blocks := make([]*model.Block, 0, nBlocks)
	for i := 0; i < nBlocks; i++ {
		b := &model.Block{ID: fmt.Sprintf("b%02d", i), Translatable: true}
		b.SetSourceText(fmt.Sprintf("Hello number %d", i))
		blocks = append(blocks, b)
	}
	require.NoError(t, deps.ContentStore.StoreBlocksForItem(ctx, projectID, "main", "en.json", blocks))

	job := &TranslationJob{
		ID:            id.New(),
		WorkspaceSlug: "ws",
		WorkspaceID:   "ws-uuid",
		ProjectID:     projectID,
		ItemName:      "en.json",
		TargetLocale:  "fr",
		Status:        StatusQueued,
	}
	require.NoError(t, deps.JobStore.CreateJob(ctx, job))

	// Worker 1 claims the job (epoch e1) and translates.
	ok, e1, err := deps.JobStore.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, ok)
	quota.jobID = job.ID

	err = executeTranslationWithDeps(ctx, deps, job, e1)

	// The resurrection during chunk 1's RecordUsage cost worker 1 the lease; the
	// chunk-2 gate must abort with errLeaseLost BEFORE billing chunk 2.
	require.ErrorIs(t, err, errLeaseLost,
		"the abandoned worker must abort with errLeaseLost, not continue")
	assert.Equal(t, 1, quota.calls,
		"the abandoned worker must not record usage past the point it lost the lease (no double-billing)")

	// It must not have marked the job completed: the fresh owner (e2) still holds
	// it in 'processing'.
	got, err := deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusProcessing, got.Status,
		"the abandoned worker must not overwrite the resurrected job's status")
}
