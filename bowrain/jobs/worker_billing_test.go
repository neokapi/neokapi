package jobs

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// billingWorkerFixture wires a translation worker against real Postgres content,
// job, quota and billing stores (testcontainers) plus the offline demo provider.
type billingWorkerFixture struct {
	deps      *WorkerDeps
	billing   *billing.PgBillingStore
	quota     *QuotaStoreDB
	projectID string
}

func newBillingWorkerFixture(t *testing.T) *billingWorkerFixture {
	t.Helper()
	db := pgtest.NewTestDB(t)
	ctx := t.Context()

	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	js, err := NewJobStore(db)
	require.NoError(t, err)
	qs, err := NewQuotaStore(db)
	require.NoError(t, err)
	bs, err := billing.NewPgBillingStore(db)
	require.NoError(t, err)

	const projectID = "proj-1"
	require.NoError(t, cs.CreateProject(ctx, &store.Project{
		ID:                    projectID,
		Name:                  "Billing Test",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
	}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "en.json", []*model.Block{
		model.NewBlock("b1", "Hello world"),
		model.NewBlock("b2", "Goodbye now"),
	}))

	return &billingWorkerFixture{
		deps: &WorkerDeps{
			JobStore:      js,
			ContentStore:  cs,
			QuotaStore:    qs,
			BillingHooks:  &billing.UsageHooks{Store: bs}, // no Stripe/Notifier in tests
			Platform:      &PlatformProviderConfig{Provider: "demo"},
			ProviderStore: &fakeProviderResolver{cfg: bstore.ProviderConfig{Type: "demo"}},
		},
		billing:   bs,
		quota:     qs,
		projectID: projectID,
	}
}

func (f *billingWorkerFixture) runJob(t *testing.T, job *TranslationJob) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, f.deps.JobStore.CreateJob(ctx, job))
	// Claim the job to obtain the lease epoch the per-chunk RenewLease gate
	// checks (Epic 003 double-run protection), mirroring the real worker loop.
	claimed, epoch, err := f.deps.JobStore.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, executeTranslationWithDeps(ctx, f.deps, job, epoch))
}

// TestWorkerBilling_PlatformDeducts verifies that a platform-key job records AI
// usage (the abuse cap) AND deducts credits (Epic 004, tasks 1 + 4).
func TestWorkerBilling_PlatformDeducts(t *testing.T) {
	f := newBillingWorkerFixture(t)
	ctx := t.Context()
	const (
		wsID   = "ws-platform"
		wsSlug = "acme"
	)
	require.NoError(t, f.billing.GrantCredits(ctx, wsID, 1_000_000, billing.SourcePlan))

	f.runJob(t, &TranslationJob{
		ID:               "job-plat",
		WorkspaceSlug:    wsSlug,
		WorkspaceID:      wsID,
		ProjectID:        f.projectID,
		ItemName:         "en.json",
		TargetLocale:     "fr",
		ProviderConfigID: "platform",
		Model:            "demo",
		Status:           StatusQueued,
	})

	remaining, err := f.billing.CheckCredits(ctx, wsID)
	require.NoError(t, err)
	assert.Less(t, remaining, int64(1_000_000), "platform run must deduct credits")

	usage, err := f.quota.GetUsageSummary(ctx, wsSlug)
	require.NoError(t, err)
	assert.Positive(t, usage.UsedTokens, "platform run must record ai_usage (abuse cap)")
}

// TestWorkerBilling_PlatformDeductsThroughReload is the BLOCKER-1 regression: it
// drives billing through the REAL worker entry point — processJobWithDeps, which
// claims the job and reloads it via GetJob before translating — not
// executeTranslationWithDeps directly. If workspace_id is not persisted on the
// translation_jobs row, job.WorkspaceID is "" after the reload, the platform
// deduction guard never fires, and no credits are burned. Exercising the reload
// path is what catches that (the direct-execute tests above mask it).
func TestWorkerBilling_PlatformDeductsThroughReload(t *testing.T) {
	f := newBillingWorkerFixture(t)
	ctx := t.Context()
	const (
		wsID   = "ws-reload"
		wsSlug = "gamma"
	)
	require.NoError(t, f.billing.GrantCredits(ctx, wsID, 1_000_000, billing.SourcePlan))

	job := &TranslationJob{
		ID:               "job-reload",
		WorkspaceSlug:    wsSlug,
		WorkspaceID:      wsID,
		ProjectID:        f.projectID,
		ItemName:         "en.json",
		TargetLocale:     "fr",
		ProviderConfigID: "platform",
		Model:            "demo",
		Status:           StatusQueued,
	}
	require.NoError(t, f.deps.JobStore.CreateJob(ctx, job))

	// The billing workspace must survive the store round-trip — this is the
	// property the worker depends on to meter platform runs.
	reloaded, err := f.deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, wsID, reloaded.WorkspaceID, "workspace_id must persist across the job store round-trip")

	// Drive the real worker path: claim → reload → translate → deduct.
	require.NoError(t, processJobWithDeps(ctx, f.deps, job.ID))

	remaining, err := f.billing.CheckCredits(ctx, wsID)
	require.NoError(t, err)
	assert.Less(t, remaining, int64(1_000_000), "platform run via the reload path must deduct credits")
}

// TestWorkerBilling_BYORecordsButDoesNotDeduct verifies that a workspace
// bring-your-own-key job records AI usage but burns NO credits (Epic 004,
// task 4).
func TestWorkerBilling_BYORecordsButDoesNotDeduct(t *testing.T) {
	f := newBillingWorkerFixture(t)
	ctx := t.Context()
	const (
		wsID   = "ws-byo"
		wsSlug = "beta"
	)
	require.NoError(t, f.billing.GrantCredits(ctx, wsID, 1_000_000, billing.SourcePlan))

	f.runJob(t, &TranslationJob{
		ID:               "job-byo",
		WorkspaceSlug:    wsSlug,
		WorkspaceID:      wsID,
		ProjectID:        f.projectID,
		ItemName:         "en.json",
		TargetLocale:     "fr",
		ProviderConfigID: "cfg-1", // a saved per-workspace BYO config
		Model:            "demo",
		Status:           StatusQueued,
	})

	remaining, err := f.billing.CheckCredits(ctx, wsID)
	require.NoError(t, err)
	assert.Equal(t, int64(1_000_000), remaining, "BYO run must not burn credits")

	usage, err := f.quota.GetUsageSummary(ctx, wsSlug)
	require.NoError(t, err)
	assert.Positive(t, usage.UsedTokens, "BYO run must still record ai_usage (abuse cap)")
}
