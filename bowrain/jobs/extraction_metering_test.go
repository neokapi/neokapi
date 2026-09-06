package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/neokapi/neokapi/bowrain/observe"
	"github.com/neokapi/neokapi/bowrain/service"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingQuotaStore keeps the usage rows an extraction wrote and answers the
// abuse cap with whatever remaining the case needs.
type recordingQuotaStore struct {
	mu        sync.Mutex
	remaining int64
	records   []AIUsageRecord
}

func (s *recordingQuotaStore) CheckQuota(context.Context, string) (int64, error) {
	return s.remaining, nil
}

func (s *recordingQuotaStore) RecordUsage(_ context.Context, usage AIUsageRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, usage)
	return nil
}

func (s *recordingQuotaStore) GetUsageSummary(context.Context, string) (*UsageSummary, error) {
	return &UsageSummary{}, nil
}

func (s *recordingQuotaStore) written() []AIUsageRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]AIUsageRecord(nil), s.records...)
}

// balanceLedger is the context-scan fake plus the balance an extraction's
// admission reads, which is the one method beyond DeductCredits this path
// calls.
type balanceLedger struct {
	fakeBillingLedger
	remaining int64
}

func (s *balanceLedger) CheckCredits(context.Context, string) (int64, error) {
	return s.remaining, nil
}

// meteredRun is one extraction driven against the fake model with both ledgers
// wired.
type meteredRun struct {
	err     error
	spent   *extractionModel
	quota   *recordingQuotaStore
	credits *balanceLedger
}

// runMeteredExtraction drives one extraction job with a workspace identity and
// both ledgers in place.
func runMeteredExtraction(t *testing.T, blocks int, quota *recordingQuotaStore, credits *balanceLedger) meteredRun {
	t.Helper()
	ctx := context.Background()
	cs, projectID := newExtractionFixture(t, blocks)
	srv, spent := newExtractionModel(t)
	store := &countingExtractionStore{job: &ExtractionJob{
		ID: "extract-job-1", WorkspaceSlug: "acme", WorkspaceID: "ws-1",
		ProjectID: projectID, ItemName: "en.json", Status: ExtractionStatusQueued,
	}}
	deps := &ExtractionWorkerDeps{
		ExtractionJobStore: store,
		ContentStore:       cs,
		QuotaStore:         quota,
		BillingHooks:       &billing.UsageHooks{Store: credits},
		Platform: &PlatformProviderConfig{
			Provider: "openai", APIKey: "k", Model: "test-model", BaseURL: srv.URL,
		},
	}

	return meteredRun{
		err:     executeExtraction(ctx, deps, store.job, 1),
		spent:   spent,
		quota:   quota,
		credits: credits,
	}
}

// An extraction on the platform key records what it spent and deducts for it,
// the way the translation worker and the context scan do.
func TestExtractionWorker_RecordsUsageAndDeductsOnce(t *testing.T) {
	run := runMeteredExtraction(t, 60,
		&recordingQuotaStore{remaining: 1 << 20},
		&balanceLedger{remaining: 1 << 20})
	require.NoError(t, run.err)

	rows := run.quota.written()
	require.Len(t, rows, 1, "one row per operation and model")
	assert.Equal(t, "acme", rows[0].WorkspaceSlug)
	assert.Equal(t, "ws-1", rows[0].WorkspaceID)
	assert.Equal(t, "extract-job-1", rows[0].JobID)
	assert.Equal(t, usageOpEntityExtract, rows[0].Operation)
	assert.Equal(t, "test-model", rows[0].Model)
	// The fake model reports ten prompt and five completion tokens per call,
	// over six batches of ten blocks.
	assert.Equal(t, 60, rows[0].PromptTokens)
	assert.Equal(t, 30, rows[0].OutputTokens)
	assert.Equal(t, 90, rows[0].TotalTokens)

	taken := run.credits.all()
	require.Len(t, taken, 1, "a job settles once, not once per model call")
	assert.Equal(t, "ws-1", taken[0].workspaceID)
	assert.Equal(t, billingOpExtraction, taken[0].op)
	assert.Equal(t, billing.TokensToCredits(90), taken[0].amount)
	assert.Contains(t, taken[0].refID, "extract-job-1",
		"the deduction names the job it paid for")
}

// A workspace with nothing left to spend fails before a token is burned:
// deduction is post-hoc, so a run admitted here would drive the ledger negative
// over a whole item.
func TestExtractionWorker_ExhaustedWorkspaceIsRefused(t *testing.T) {
	run := runMeteredExtraction(t, 60,
		&recordingQuotaStore{remaining: 1 << 20},
		&balanceLedger{remaining: 0})

	require.ErrorIs(t, run.err, service.ErrOutOfCredits)
	run.spent.mu.Lock()
	defer run.spent.mu.Unlock()
	assert.Zero(t, run.spent.requests, "the model was called for a workspace that cannot pay")
	assert.Empty(t, run.quota.written())
	assert.Empty(t, run.credits.all())
}

// The abuse cap is the other gate: a workspace past its monthly ceiling is
// refused whether or not it has credits.
func TestExtractionWorker_SpentQuotaCeilingIsRefused(t *testing.T) {
	run := runMeteredExtraction(t, 60,
		&recordingQuotaStore{remaining: 0},
		&balanceLedger{remaining: 1 << 20})

	require.ErrorIs(t, run.err, errExtractionQuotaExceeded)
	run.spent.mu.Lock()
	defer run.spent.mu.Unlock()
	assert.Zero(t, run.spent.requests)
}

// An out-of-credits failure is permanent: the retry classifier must not read it
// as an upstream wobble and spend the job's whole budget re-asking a workspace
// that still cannot pay.
func TestExtractionWorker_RefusalIsPermanent(t *testing.T) {
	assert.False(t, isTransientError(service.ErrOutOfCredits))
	assert.False(t, isTransientError(errExtractionQuotaExceeded))
}

// A bring-your-own key is recorded against the abuse cap, which must see every
// call, and burns no credits (Epic 004).
func TestExtractionAccountant_ByoKeyRecordsUsageAndDeductsNothing(t *testing.T) {
	quota := &recordingQuotaStore{remaining: 1 << 20}
	credits := &balanceLedger{remaining: 1 << 20}
	deps := &ExtractionWorkerDeps{QuotaStore: quota, BillingHooks: &billing.UsageHooks{Store: credits}}
	job := &ExtractionJob{ID: "job-2", WorkspaceSlug: "acme", WorkspaceID: "ws-1"}

	acct := accountantFor(deps, job)
	require.NotNil(t, acct)
	require.NoError(t, acct.Admit(t.Context(), "ws-1", service.SpendOwnKey),
		"a key the workspace pays for needs no credits")
	acct.Record(t.Context(), service.AISpend{
		WorkspaceID: "ws-1",
		ProjectID:   "proj-1",
		ReferenceID: "ref-1",
		ByOperation: []service.OperationSpend{{
			Operation: usageOpEntityExtract,
			Model:     "own-model",
			Source:    service.SpendOwnKey,
			Usage:     aiprovider.TokenUsage{InputTokens: 10, OutputTokens: 4},
		}},
		Total: aiprovider.TokenUsage{InputTokens: 10, OutputTokens: 4},
	})

	require.Len(t, quota.written(), 1)
	assert.Equal(t, "own-model", quota.written()[0].Model)
	assert.Empty(t, credits.all(), "a bring-your-own key burns no credits")
}

// failingRecordQuotaStore fails every write and answers every read, standing in
// for a meter that is down while the extraction itself went fine.
type failingRecordQuotaStore struct{ err error }

func (s *failingRecordQuotaStore) CheckQuota(context.Context, string) (int64, error) {
	return 1 << 20, nil
}
func (s *failingRecordQuotaStore) RecordUsage(context.Context, AIUsageRecord) error { return s.err }
func (s *failingRecordQuotaStore) GetUsageSummary(context.Context, string) (*UsageSummary, error) {
	return &UsageSummary{}, nil
}

// Recording is fail-open: the tokens are spent by the time it runs, so a meter
// outage must not also cost the customer the extraction. It is not fail-silent,
// though, so the discard is counted with the quantity that went unrecorded.
func TestExtractionAccountant_MeterFailureIsObserved(t *testing.T) {
	before := testutil.ToFloat64(observe.MeteringDiscardedTotal.WithLabelValues(observe.MeterAITokens))
	beforeTokens := testutil.ToFloat64(
		observe.MeteringUnrecordedTotal.WithLabelValues(observe.MeterAITokens, "tokens"))

	credits := &balanceLedger{remaining: 1 << 20}
	deps := &ExtractionWorkerDeps{
		QuotaStore:   &failingRecordQuotaStore{err: errors.New("meter unreachable")},
		BillingHooks: &billing.UsageHooks{Store: credits},
	}
	acct := accountantFor(deps, &ExtractionJob{ID: "job-3", WorkspaceSlug: "acme"})

	require.NotPanics(t, func() {
		acct.Record(t.Context(), service.AISpend{
			WorkspaceID: "ws-1",
			ReferenceID: "ref-3",
			ByOperation: []service.OperationSpend{{
				Operation: usageOpEntityExtract,
				Model:     "test-model",
				Source:    service.SpendPlatformKey,
				Usage:     aiprovider.TokenUsage{InputTokens: 700, OutputTokens: 300},
			}},
			Total:    aiprovider.TokenUsage{InputTokens: 700, OutputTokens: 300},
			Billable: aiprovider.TokenUsage{InputTokens: 700, OutputTokens: 300},
		})
	}, "a meter that is down must not take the extraction down with it")

	assert.Equal(t, before+1,
		testutil.ToFloat64(observe.MeteringDiscardedTotal.WithLabelValues(observe.MeterAITokens)),
		"the discarded meter write is counted")
	assert.InDelta(t, beforeTokens+1000,
		testutil.ToFloat64(observe.MeteringUnrecordedTotal.WithLabelValues(observe.MeterAITokens, "tokens")),
		0.0001, "the unbilled tokens are counted, so 'how much' is answerable")
	assert.Len(t, credits.all(), 1, "the credit deduction stands whatever the abuse cap did")
}

// A deployment with neither ledger meters nothing, which is what a self-hosted
// instance gets.
func TestExtractionAccountant_UnmeteredDeploymentHasNoAccountant(t *testing.T) {
	assert.Nil(t, accountantFor(&ExtractionWorkerDeps{}, &ExtractionJob{}))
}
