package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/neokapi/neokapi/bowrain/observe"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingQuotaStore fails every write and succeeds every read, standing in for
// a meter that is down while the work itself went fine.
type failingQuotaStore struct{ err error }

func (s *failingQuotaStore) CheckQuota(context.Context, string) (int64, error) { return 1 << 20, nil }
func (s *failingQuotaStore) RecordUsage(context.Context, AIUsageRecord) error  { return s.err }
func (s *failingQuotaStore) GetUsageSummary(context.Context, string) (*UsageSummary, error) {
	return &UsageSummary{}, nil
}

// TestRecordContextScanUsage_MeterFailureIsObserved covers the policy at a real
// call site: a metering failure neither fails nor panics the caller, and it is
// not silent — the discard is counted, with the quantity that went unrecorded.
func TestRecordContextScanUsage_MeterFailureIsObserved(t *testing.T) {
	before := testutil.ToFloat64(observe.MeteringDiscardedTotal.WithLabelValues(observe.MeterAITokens))
	beforeTokens := testutil.ToFloat64(
		observe.MeteringUnrecordedTotal.WithLabelValues(observe.MeterAITokens, "tokens"))

	deps := &ContextScanWorkerDeps{QuotaStore: &failingQuotaStore{err: errors.New("meter unreachable")}}
	job := &ContextScanJob{ID: "scan-1", WorkspaceSlug: "acme", WorkspaceID: "ws-1"}

	require.NotPanics(t, func() {
		recordContextScanUsage(t.Context(), deps, job, "test-model",
			aiprovider.TokenUsage{InputTokens: 700, OutputTokens: 300}, 1000)
	}, "a meter that is down must not take the scan down with it")

	assert.Equal(t, before+1,
		testutil.ToFloat64(observe.MeteringDiscardedTotal.WithLabelValues(observe.MeterAITokens)),
		"the discarded meter write is counted")
	assert.InDelta(t, beforeTokens+1000,
		testutil.ToFloat64(observe.MeteringUnrecordedTotal.WithLabelValues(observe.MeterAITokens, "tokens")),
		0.0001, "the unbilled tokens are counted, so 'how much' is answerable")
}

// TestRecordContextScanUsage_NoQuotaStoreIsNotAFailure keeps the nil-store case
// (self-hosted, unmetered) distinct from a meter that failed: nothing is
// recorded and nothing is counted as lost.
func TestRecordContextScanUsage_NoQuotaStoreIsNotAFailure(t *testing.T) {
	before := testutil.ToFloat64(observe.MeteringDiscardedTotal.WithLabelValues(observe.MeterAITokens))

	recordContextScanUsage(t.Context(), &ContextScanWorkerDeps{}, &ContextScanJob{ID: "scan-2"},
		"test-model", aiprovider.TokenUsage{InputTokens: 10}, 10)

	assert.Equal(t, before,
		testutil.ToFloat64(observe.MeteringDiscardedTotal.WithLabelValues(observe.MeterAITokens)))
}
