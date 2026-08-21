package jobs

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/neokapi/neokapi/bowrain/storage"
	bloblocal "github.com/neokapi/neokapi/bowrain/storage/localblob"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/registry"
	corestorage "github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// contextScanFixture wires a brand-scan worker against a real Postgres store
// (testcontainers), a local blob store, and the offline demo provider.
type contextScanFixture struct {
	deps *ContextScanWorkerDeps
	logs *contextScanLogCapture
	db   *storage.PgDB
}

type contextScanLogCapture struct {
	mu      sync.Mutex
	entries []string
}

func (l *contextScanLogCapture) log(_, level, message string, _ map[string]string) {
	l.mu.Lock()
	l.entries = append(l.entries, level+": "+message)
	l.mu.Unlock()
}

func (l *contextScanLogCapture) contains(substr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.entries {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

func newContextScanFixture(t *testing.T) *contextScanFixture {
	t.Helper()
	db := pgtest.NewTestDB(t)
	store, err := NewContextScanJobStore(db)
	require.NoError(t, err)
	blobs, err := bloblocal.New(t.TempDir())
	require.NoError(t, err)

	logs := &contextScanLogCapture{}
	return &contextScanFixture{
		deps: &ContextScanWorkerDeps{
			Store:     store,
			Queue:     NewChannelQueue(8),
			BlobStore: blobs,
			Platform:  &PlatformProviderConfig{Provider: "demo"},
			LogFunc:   logs.log,
		},
		logs: logs,
		db:   db,
	}
}

// requeueJob puts a finished scan back in the queue, which is what a retry or a
// redelivered queue message leaves behind.
func (f *contextScanFixture) requeueJob(t *testing.T, jobID string) {
	t.Helper()
	_, err := f.db.ExecContext(t.Context(),
		`UPDATE context_scan_jobs SET status = 'queued' WHERE id = $1`, jobID)
	require.NoError(t, err)
}

func (f *contextScanFixture) seedJob(t *testing.T, wsID string, req ContextScanRequest) *ContextScanJob {
	t.Helper()
	raw, err := json.Marshal(req)
	require.NoError(t, err)
	job := &ContextScanJob{
		ID:            id.New(),
		WorkspaceID:   wsID,
		WorkspaceSlug: "acme",
		Status:        ContextScanStatusQueued,
		Request:       raw,
	}
	require.NoError(t, f.deps.Store.CreateContextScanJob(t.Context(), job))
	return job
}

// uploadEnvelope stores a brand-scan upload envelope blob and returns its key.
func (f *contextScanFixture) uploadEnvelope(t *testing.T, filename, contentType string, data []byte) string {
	t.Helper()
	payload, err := json.Marshal(ContextScanUploadEnvelope{
		Filename: filename, ContentType: contentType, Data: data,
	})
	require.NoError(t, err)
	ref, err := f.deps.BlobStore.Upload(t.Context(), payload, corestorage.UploadOptions{
		ContentType: "application/json", Filename: filename,
	})
	require.NoError(t, err)
	return ref.Key
}

// demoPasteText is a corpus with recurring capitalized terms (so the demo
// provider's vocabulary heuristic yields preferred-term candidates),
// contractions, and first-person-plural voice.
const demoPasteText = "We're building Bowrain for teams that care about voice. " +
	"Bowrain keeps your copy on-brand across every locale. " +
	"With Bowrain, we ship faster and we don't compromise on tone!"

// TestContextScanWorker_DemoPipeline drives the full worker pipeline (claim →
// gather → corpus → infer → terms → persist) with the deterministic demo
// provider over a paste-text source, asserting a completed job carrying a
// schema-valid draft profile, evidence, terms, and source attribution.
func TestContextScanWorker_DemoPipeline(t *testing.T) {
	f := newContextScanFixture(t)
	ctx := t.Context()
	job := f.seedJob(t, "ws-demo", ContextScanRequest{
		PasteText:   demoPasteText,
		ProfileName: "Acme Voice",
	})

	require.NoError(t, processContextScanJob(ctx, f.deps, job.ID))

	got, err := f.deps.Store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, ContextScanStatusCompleted, got.Status, "error: %s", got.Error)
	assert.Equal(t, 100, got.Progress)
	assert.Equal(t, "done", got.Message)
	assert.Positive(t, got.TokensUsed, "the scan must account provider token usage")

	var result ContextScanResult
	require.NoError(t, json.Unmarshal(got.Result, &result))

	require.NotNil(t, result.Profile, "the draft profile is the scan's core output")
	assert.Equal(t, "Acme Voice", result.Profile.Name)
	assert.NotEmpty(t, result.Profile.Tone.Personality)
	assert.NotEmpty(t, result.Profile.Tone.Formality)

	require.NotNil(t, result.Evidence)
	for _, field := range []string{"tone", "style", "vocabulary", "examples"} {
		ev, ok := result.Evidence.Fields[field]
		require.True(t, ok, "evidence must cover %s", field)
		assert.GreaterOrEqual(t, ev.Confidence, 0.0)
		assert.LessOrEqual(t, ev.Confidence, 1.0)
		assert.NotEmpty(t, ev.Source)
	}

	assert.NotEmpty(t, result.Terms, "recurring product terms must surface as glossary candidates")
	var hasBowrain bool
	for _, term := range result.Terms {
		if term.Term == "Bowrain" {
			hasBowrain = true
		}
	}
	assert.True(t, hasBowrain, "the corpus' recurring capitalized term must be a candidate")

	require.Len(t, result.Sources, 1)
	assert.Equal(t, "paste", result.Sources[0].Kind)
	assert.Positive(t, result.Sources[0].Runes)
	assert.False(t, result.Truncated)
}

// deduction records one fake credit deduction.
type deduction struct {
	workspaceID string
	amount      int64
	op          string
	refID       string
}

// fakeBillingLedger records DeductCredits calls. The embedded interface
// panics on any other method — the worker (with Stripe and Notifier unset)
// must only ever hit DeductCredits.
type fakeBillingLedger struct {
	billing.BillingStore
	mu         sync.Mutex
	deductions []deduction
}

func (f *fakeBillingLedger) DeductCredits(_ context.Context, workspaceID string, amount int64, op, refID string) error {
	f.mu.Lock()
	f.deductions = append(f.deductions, deduction{workspaceID, amount, op, refID})
	f.mu.Unlock()
	return nil
}

func (f *fakeBillingLedger) all() []deduction {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]deduction(nil), f.deductions...)
}

// TestContextScanWorker_MeteringPerPhase: a platform scan on a billed workspace
// deducts credits exactly once per phase (infer, terms) with op "context_scan"
// and per-phase reference ids.
func TestContextScanWorker_MeteringPerPhase(t *testing.T) {
	f := newContextScanFixture(t)
	ledger := &fakeBillingLedger{}
	f.deps.BillingHooks = &billing.UsageHooks{Store: ledger}
	ctx := t.Context()

	job := f.seedJob(t, "ws-billed", ContextScanRequest{PasteText: demoPasteText})
	require.NoError(t, processContextScanJob(ctx, f.deps, job.ID))

	got, err := f.deps.Store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, ContextScanStatusCompleted, got.Status, "error: %s", got.Error)

	deductions := ledger.all()
	require.Len(t, deductions, 2, "exactly one deduction per phase")
	assert.Equal(t, "context_scan", deductions[0].op)
	assert.Equal(t, "context_scan", deductions[1].op)
	assert.Equal(t, job.ID+":infer", deductions[0].refID)
	assert.Equal(t, job.ID+":terms", deductions[1].refID)
	for _, d := range deductions {
		assert.Equal(t, "ws-billed", d.workspaceID)
		assert.Positive(t, d.amount)
	}
}

// TestContextScanWorker_MeteringSkippedWhenHooksNil: a self-hosted / unbilled
// deployment (nil hooks) completes the scan without touching billing.
func TestContextScanWorker_MeteringSkippedWhenHooksNil(t *testing.T) {
	f := newContextScanFixture(t)
	f.deps.BillingHooks = nil
	ctx := t.Context()

	job := f.seedJob(t, "ws-unbilled", ContextScanRequest{PasteText: demoPasteText})
	require.NoError(t, processContextScanJob(ctx, f.deps, job.ID))

	got, err := f.deps.Store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ContextScanStatusCompleted, got.Status, "error: %s", got.Error)
	assert.Positive(t, got.TokensUsed, "tokens are still accounted on the job without billing")
}

// TestContextScanWorker_MeteringSkippedWithoutWorkspace: hooks wired but no
// billing workspace on the job (e.g. legacy row) must not deduct.
func TestContextScanWorker_MeteringSkippedWithoutWorkspace(t *testing.T) {
	f := newContextScanFixture(t)
	ledger := &fakeBillingLedger{}
	f.deps.BillingHooks = &billing.UsageHooks{Store: ledger}
	ctx := t.Context()

	job := f.seedJob(t, "", ContextScanRequest{PasteText: demoPasteText})
	require.NoError(t, processContextScanJob(ctx, f.deps, job.ID))

	got, err := f.deps.Store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ContextScanStatusCompleted, got.Status)
	assert.Empty(t, ledger.all(), "no billing workspace → no deduction")
}

// fakeQuotaStore is a QuotaStore capturing RecordUsage calls with a fixed
// CheckQuota verdict.
type fakeQuotaStore struct {
	mu        sync.Mutex
	remaining int64
	records   []AIUsageRecord
}

func (f *fakeQuotaStore) CheckQuota(context.Context, string) (int64, error) {
	return f.remaining, nil
}

func (f *fakeQuotaStore) RecordUsage(_ context.Context, usage AIUsageRecord) error {
	f.mu.Lock()
	f.records = append(f.records, usage)
	f.mu.Unlock()
	return nil
}

func (f *fakeQuotaStore) GetUsageSummary(context.Context, string) (*UsageSummary, error) {
	return &UsageSummary{}, nil
}

func (f *fakeQuotaStore) all() []AIUsageRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]AIUsageRecord(nil), f.records...)
}

// TestContextScanWorker_RecordsAbuseUsage: every scan phase lands in the
// internal abuse cap (ai_usage) via QuotaStore.RecordUsage with operation
// "context_scan" — brand scans must never be invisible to abuse detection.
func TestContextScanWorker_RecordsAbuseUsage(t *testing.T) {
	f := newContextScanFixture(t)
	quota := &fakeQuotaStore{remaining: 1_000_000}
	f.deps.QuotaStore = quota
	ctx := t.Context()

	job := f.seedJob(t, "ws-quota", ContextScanRequest{PasteText: demoPasteText})
	require.NoError(t, processContextScanJob(ctx, f.deps, job.ID))

	got, err := f.deps.Store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, ContextScanStatusCompleted, got.Status, "error: %s", got.Error)

	records := quota.all()
	require.Len(t, records, 2, "one abuse-cap record per phase (infer, terms)")
	for _, rec := range records {
		assert.Equal(t, "context_scan", rec.Operation)
		assert.Equal(t, "acme", rec.WorkspaceSlug)
		assert.Equal(t, "ws-quota", rec.WorkspaceID)
		assert.Equal(t, job.ID, rec.JobID)
		assert.Positive(t, rec.TotalTokens)
	}
}

// TestContextScanWorker_QuotaExceededFailsUpfront: a workspace over its monthly
// abuse ceiling cannot run a scan — the job fails before any provider call or
// usage recording.
func TestContextScanWorker_QuotaExceededFailsUpfront(t *testing.T) {
	f := newContextScanFixture(t)
	quota := &fakeQuotaStore{remaining: 0}
	f.deps.QuotaStore = quota
	ctx := t.Context()

	job := f.seedJob(t, "ws-capped", ContextScanRequest{PasteText: demoPasteText})
	err := processContextScanJob(ctx, f.deps, job.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quota exceeded")

	got, gerr := f.deps.Store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, gerr)
	assert.Equal(t, ContextScanStatusFailed, got.Status)
	assert.Contains(t, got.Error, "workspace AI quota exceeded")
	assert.Empty(t, quota.all(), "no usage is recorded for a scan blocked by the cap")
}

// TestContextScanWorker_UploadSourceAndUnsupportedSkip: an uploaded markdown file
// feeds the corpus while a pdf upload (no registered format) is skipped with a
// per-source reason — surfaced as a zero-rune source, never a job failure.
func TestContextScanWorker_UploadSourceAndUnsupportedSkip(t *testing.T) {
	f := newContextScanFixture(t)
	ctx := t.Context()

	mdKey := f.uploadEnvelope(t, "voice-guide.md", "text/markdown",
		[]byte("# Voice\n\nWe write plainly. We're direct and Bowrain-first. Bowrain wins."))
	pdfKey := f.uploadEnvelope(t, "brand-deck.pdf", "application/pdf", []byte("%PDF-1.4 stub"))

	job := f.seedJob(t, "ws-uploads", ContextScanRequest{UploadKeys: []string{mdKey, pdfKey}})
	require.NoError(t, processContextScanJob(ctx, f.deps, job.ID))

	got, err := f.deps.Store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, ContextScanStatusCompleted, got.Status, "error: %s", got.Error)

	var result ContextScanResult
	require.NoError(t, json.Unmarshal(got.Result, &result))

	byLabel := map[string]ContextScanSource{}
	for _, s := range result.Sources {
		byLabel[s.Label] = s
	}
	require.Contains(t, byLabel, "voice-guide.md")
	assert.Positive(t, byLabel["voice-guide.md"].Runes)
	require.Contains(t, byLabel, "brand-deck.pdf", "the skipped upload must still be attributed")
	assert.Zero(t, byLabel["brand-deck.pdf"].Runes)

	assert.True(t, f.logs.contains("unsupported file type"),
		"the skip reason (unsupported file type — pdf reads out-of-core via the pdfium plugin) must reach the step logs")
}

// TestContextScanWorker_AllSourcesFail: when every source is unreadable the job
// fails with a clear user-facing error naming the reasons.
func TestContextScanWorker_AllSourcesFail(t *testing.T) {
	f := newContextScanFixture(t)
	ctx := t.Context()

	job := f.seedJob(t, "ws-fail", ContextScanRequest{
		UploadKeys: []string{"deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"},
	})
	err := processContextScanJob(ctx, f.deps, job.ID)
	require.Error(t, err)

	got, gerr := f.deps.Store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, gerr)
	assert.Equal(t, ContextScanStatusFailed, got.Status)
	assert.Contains(t, got.Error, "no readable brand material")
}

// TestContextScanWorker_EmptyCorpusFails: sources that yield only whitespace
// produce a failed job with an actionable error, not a crash or empty draft.
func TestContextScanWorker_EmptyCorpusFails(t *testing.T) {
	f := newContextScanFixture(t)
	ctx := t.Context()

	key := f.uploadEnvelope(t, "empty.txt", "text/plain", []byte("   \n\n  "))
	job := f.seedJob(t, "ws-empty", ContextScanRequest{UploadKeys: []string{key}})

	err := processContextScanJob(ctx, f.deps, job.ID)
	require.Error(t, err)

	got, gerr := f.deps.Store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, gerr)
	assert.Equal(t, ContextScanStatusFailed, got.Status)
	assert.Contains(t, got.Error, "no analyzable text")
}

// TestFetchContextScanRepo_URLPolicy: the repository source accepts only public
// https URLs — non-https transports (including the scp-like SSH form) and
// hosts in forbidden address ranges are rejected before any git subprocess
// runs (no network, no DNS: all cases are literals).
func TestFetchContextScanRepo_URLPolicy(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	cases := []struct {
		name string
		url  string
		want string
	}{
		{"git protocol to private host", "git://10.0.0.5:9418/x", "only public https"},
		{"ssh protocol", "ssh://user@10.1.2.3/x", "only public https"},
		{"scp-like ssh", "git@github.com:org/repo.git", "repository URL"},
		{"plain http", "http://example.com/repo.git", "only public https"},
		{"loopback", "https://127.0.0.1/repo.git", "loopback"},
		{"private rfc1918", "https://10.0.0.5/repo.git", "private"},
		{"cloud metadata endpoint", "https://169.254.169.254/latest", "link-local"},
		{"cgnat", "https://100.64.1.2/repo.git", "carrier-grade NAT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, err := fetchContextScanRepo(t.Context(), reg, tc.url)
			require.Error(t, err)
			assert.Empty(t, text)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// TestGatherContextScanSources_BudgetCap: once earlier sources fill the gather
// budget, later sources are skipped up front (never fetched) with a
// user-facing reason.
func TestGatherContextScanSources_BudgetCap(t *testing.T) {
	logs := &contextScanLogCapture{}
	deps := &ContextScanWorkerDeps{LogFunc: logs.log}
	req := &ContextScanRequest{
		PasteText: strings.Repeat("a", contextScanSourceTextCapBytes+1),
		URLs:      []string{"https://127.0.0.1/never-fetched"},
		RepoURL:   "https://127.0.0.1/never-cloned.git",
	}

	sources, skipped := gatherContextScanSources(t.Context(), deps, "job-budget", req)

	require.Len(t, sources, 1, "only the paste source is gathered")
	require.Len(t, skipped, 2, "url and repo sources are skipped over budget")
	for _, sk := range skipped {
		assert.Equal(t, contextScanBudgetSkipReason, sk.reason)
	}
}

// TestGatherContextScanSources_SanitizedReasons: source-fetch failures surface
// generic user-facing reasons; resolver output and address-policy detail
// (which would turn the scan into an internal-network oracle via job.Error
// and the step logs) stay out of the skip reasons.
func TestGatherContextScanSources_SanitizedReasons(t *testing.T) {
	logs := &contextScanLogCapture{}
	blobs, err := bloblocal.New(t.TempDir())
	require.NoError(t, err)
	deps := &ContextScanWorkerDeps{LogFunc: logs.log, BlobStore: blobs}
	req := &ContextScanRequest{
		URLs:       []string{"https://10.0.0.5/internal"},
		UploadKeys: []string{"no-such-key"},
		RepoURL:    "https://169.254.169.254/repo.git",
	}

	sources, skipped := gatherContextScanSources(t.Context(), deps, "job-sanitize", req)

	assert.Empty(t, sources)
	require.Len(t, skipped, 3)
	for _, sk := range skipped {
		for _, leak := range []string{"10.0.0.5", "169.254.169.254", "private", "link-local", "resolves to"} {
			assert.NotContains(t, sk.reason, leak, "skip reason for %q must not leak %q", sk.label, leak)
		}
	}
	// The upload failure (caller-controlled key against the blob store) is
	// generic too: blob-backend detail stays in server logs.
	assert.Equal(t, "the uploaded file could not be read", skipped[1].reason)
}

// TestContextScanWorker_DoubleClaimIsNoOp: a second delivery of an already
// claimed job returns without error and without disturbing the first run.
func TestContextScanWorker_DoubleClaimIsNoOp(t *testing.T) {
	f := newContextScanFixture(t)
	ctx := t.Context()
	job := f.seedJob(t, "ws-dup", ContextScanRequest{PasteText: demoPasteText})

	claimed, _, err := f.deps.Store.ClaimContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	// The duplicate delivery must be a clean no-op (nil error → ACK).
	require.NoError(t, processContextScanJob(ctx, f.deps, job.ID))

	got, err := f.deps.Store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ContextScanStatusProcessing, got.Status, "the first claim's run must stay live")
}

// TestContextScanWorker_NoPlatformProviderFails: without a platform provider a
// scan fails with the configuration hint rather than hanging.
func TestContextScanWorker_NoPlatformProviderFails(t *testing.T) {
	f := newContextScanFixture(t)
	f.deps.Platform = nil
	ctx := t.Context()

	job := f.seedJob(t, "ws-noprov", ContextScanRequest{PasteText: demoPasteText})
	err := processContextScanJob(ctx, f.deps, job.ID)
	require.Error(t, err)

	got, gerr := f.deps.Store.GetContextScanJob(ctx, job.ID)
	require.NoError(t, gerr)
	assert.Equal(t, ContextScanStatusFailed, got.Status)
	assert.Contains(t, got.Error, "platform provider not configured")
}

// TestContextScanWorker_RunLoopProcessesQueuedJob drives RunContextScanWorker
// through the queue end-to-end (enqueue → dequeue → process → ack).
func TestContextScanWorker_RunLoopProcessesQueuedJob(t *testing.T) {
	f := newContextScanFixture(t)
	job := f.seedJob(t, "ws-loop", ContextScanRequest{PasteText: demoPasteText})
	require.NoError(t, f.deps.Queue.Enqueue(t.Context(), job.ID))

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = RunContextScanWorker(ctx, f.deps)
	}()

	require.Eventually(t, func() bool {
		got, err := f.deps.Store.GetContextScanJob(t.Context(), job.ID)
		return err == nil && got.Status == ContextScanStatusCompleted
	}, 30*time.Second, 100*time.Millisecond, "the worker loop must complete the queued scan")

	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("worker loop did not stop on context cancellation")
	}
}
