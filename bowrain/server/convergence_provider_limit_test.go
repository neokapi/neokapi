package server

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/neokapi/neokapi/bowrain/jobs"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bedrockDailyQuotaMsg = "bedrock: converse: operation error Bedrock Runtime: Converse, " +
	"https response error StatusCode: 429, RequestID: 0f1e, " +
	"ThrottlingException: Too many tokens per day, please wait before trying again."

func TestProviderLimitStop(t *testing.T) {
	failed := func(msg string) *jobs.TranslationJob {
		return &jobs.TranslationJob{Status: jobs.StatusFailed, Error: msg}
	}
	done := &jobs.TranslationJob{Status: jobs.StatusCompleted}
	queued := &jobs.TranslationJob{Status: jobs.StatusQueued}

	tests := []struct {
		name string
		jobs []*jobs.TranslationJob
		want *stallError
	}{
		{name: "no jobs", jobs: nil, want: nil},
		{name: "all completed", jobs: []*jobs.TranslationJob{done, done}, want: nil},
		{name: "item failure keeps going", jobs: []*jobs.TranslationJob{done, failed("store blocks: constraint violation")}, want: nil},
		{name: "outage failure keeps going", jobs: []*jobs.TranslationJob{failed("openai: API error 503: service unavailable")}, want: nil},
		{name: "daily quota stops", jobs: []*jobs.TranslationJob{done, failed(bedrockDailyQuotaMsg), queued}, want: errStallProviderQuota},
		{name: "throttling after retries stops", jobs: []*jobs.TranslationJob{failed("anthropic: API error 429: rate limit exceeded")}, want: errStallProviderThrottled},
		{
			name: "quota wins over throttling",
			jobs: []*jobs.TranslationJob{failed("anthropic: API error 429: rate limit exceeded"), failed(bedrockDailyQuotaMsg)},
			want: errStallProviderQuota,
		},
		{
			name: "a quota message on a queued row is ignored",
			jobs: []*jobs.TranslationJob{{Status: jobs.StatusQueued, Error: bedrockDailyQuotaMsg}},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Same(t, tt.want, providerLimitStop(tt.jobs))
		})
	}
}

// TestOrchestrator_DriveStopsOnProviderLimit asserts a run whose production
// hits a provider limit ends failed after the first pass, carries the reason,
// and starts no further pass, where an ordinary park would retry up to
// MaxPasses.
func TestOrchestrator_DriveStopsOnProviderLimit(t *testing.T) {
	tests := []struct {
		name       string
		stop       *stallError
		wantReason string
	}{
		{name: "quota exhausted", stop: errStallProviderQuota, wantReason: "provider_quota_exhausted"},
		{name: "throttled", stop: errStallProviderThrottled, wantReason: convergence.StallRateLimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, runStore := newOrchestratorHarness(t)
			ctx := context.Background()
			run := &bstore.ConvergenceRun{ProjectID: "proj-1", Trigger: "push", State: bstore.ConvergenceRunRunning}
			require.NoError(t, runStore.CreateRun(ctx, run))

			var produces atomic.Int32
			funcs := convergence.LoopFuncs{
				Derive: func(context.Context) (convergence.PassState, error) {
					return convergence.PassState{
						Pending:    []string{"fr-FR"},
						UnitTotals: map[string]int{"fr-FR": 10},
					}, nil
				},
				Produce: func(context.Context, string, int, *convergence.Emitter) (convergence.PassProduction, error) {
					produces.Add(1)
					return convergence.PassProduction{}, tt.stop
				},
			}
			s.convergence.driveWith(ctx, run, funcs)

			got, err := runStore.GetRun(ctx, run.ID)
			require.NoError(t, err)
			assert.Equal(t, bstore.ConvergenceRunFailed, got.State)
			assert.Equal(t, tt.wantReason, got.StallReason)
			assert.Equal(t, tt.stop.message, got.Error)
			assert.Equal(t, 1, got.Passes)
			assert.Equal(t, int32(1), produces.Load(), "no second pass after a provider limit")
		})
	}
}

// TestSkipQueuedJobs asserts a stopped run fails its push's queued jobs and
// leaves running, finished, and unrelated jobs alone.
func TestSkipQueuedJobs(t *testing.T) {
	js, err := jobs.NewJobStore(pgtest.NewTestDB(t))
	require.NoError(t, err)
	s := &Server{JobStore: js}
	s.convergence = newConvergenceOrchestrator(s)
	ctx := context.Background()

	mk := func(pushID string) *jobs.TranslationJob {
		j := &jobs.TranslationJob{
			ID: id.New(), WorkspaceSlug: "ws", ProjectID: "proj-1",
			ItemName: "en.json", TargetLocale: "fr-FR", PushID: pushID, Status: jobs.StatusQueued,
		}
		require.NoError(t, js.CreateJob(ctx, j))
		return j
	}
	queued := mk("push-1")
	running := mk("push-1")
	other := mk("push-2")
	ok, _, err := js.ClaimJob(ctx, running.ID)
	require.NoError(t, err)
	require.True(t, ok)

	s.convergence.skipQueuedJobs(ctx, "push-1", errStallProviderQuota)

	want := map[string]jobs.JobStatus{
		queued.ID:  jobs.StatusFailed,
		running.ID: jobs.StatusProcessing,
		other.ID:   jobs.StatusQueued,
	}
	for jobID, status := range want {
		row, err := js.GetJob(ctx, jobID)
		require.NoError(t, err)
		assert.Equal(t, status, row.Status, jobID)
	}
	row, err := js.GetJob(ctx, queued.ID)
	require.NoError(t, err)
	assert.Equal(t, "skipped: "+errStallProviderQuota.message, row.Error)
}
