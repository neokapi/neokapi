package server

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/service"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// meterQuotaStore is the ai_usage abuse cap under test: a fixed remaining
// balance, and every usage row it was handed.
type meterQuotaStore struct {
	remaining int64
	checkErr  error
	recorded  []jobs.AIUsageRecord
	recordErr error
}

func (q *meterQuotaStore) CheckQuota(context.Context, string) (int64, error) {
	return q.remaining, q.checkErr
}

func (q *meterQuotaStore) RecordUsage(_ context.Context, usage jobs.AIUsageRecord) error {
	q.recorded = append(q.recorded, usage)
	return q.recordErr
}

func (q *meterQuotaStore) GetUsageSummary(context.Context, string) (*jobs.UsageSummary, error) {
	return &jobs.UsageSummary{}, nil
}

// meterBillingStore records the credit deductions a run settles with.
type meterBillingStore struct {
	mockBillingStore
	deductions []struct {
		workspaceID string
		credits     int64
		operation   string
		reference   string
	}
}

func (s *meterBillingStore) DeductCredits(_ context.Context, workspaceID string, credits int64, op, refID string) error {
	s.deductions = append(s.deductions, struct {
		workspaceID string
		credits     int64
		operation   string
		reference   string
	}{workspaceID, credits, op, refID})
	return nil
}

// meterAuthStore resolves the one workspace these tests meter, and closes
// cleanly so a server built around it can shut down.
type meterAuthStore struct {
	auth.AuthStore
	ws *platauth.Workspace
}

func (m *meterAuthStore) GetWorkspace(_ context.Context, id string) (*platauth.Workspace, error) {
	if m.ws != nil && m.ws.ID == id {
		return m.ws, nil
	}
	return nil, assert.AnError
}

func (m *meterAuthStore) Close() error { return nil }

// newMeteringServer is a server with the two ledgers a platform run's AI is
// accounted through, and a workspace whose slug the abuse cap keys on.
func newMeteringServer(t *testing.T, quota *meterQuotaStore, bill *meterBillingStore) *Server {
	t.Helper()
	s := &Server{
		QuotaStore:   quota,
		BillingStore: bill,
		BillingHooks: &billing.UsageHooks{Store: bill},
		AuthStore:    &meterAuthStore{ws: &platauth.Workspace{ID: "ws-1", Slug: "acme"}},
	}
	return s
}

// A settled run writes one usage row per operation and model against the
// workspace, and deducts the run's total once.
func TestFlowAIAccountantRecordsUsageAndDeductsOnce(t *testing.T) {
	quota := &meterQuotaStore{remaining: 1_000_000}
	bill := &meterBillingStore{}
	acct := flowAIAccountant{srv: newMeteringServer(t, quota, bill)}

	acct.Record(context.Background(), service.AISpend{
		WorkspaceID: "ws-1",
		ProjectID:   "p1",
		RunID:       "run-7",
		ReferenceID: "p1:run-7:abc",
		ByOperation: []service.OperationSpend{
			{Operation: "review", Model: "sonnet", Source: service.SpendPlatformKey, Usage: aiprovider.TokenUsage{InputTokens: 4, OutputTokens: 1}},
			{Operation: "translate", Model: "haiku", Source: service.SpendPlatformKey, Usage: aiprovider.TokenUsage{InputTokens: 12, OutputTokens: 8}},
		},
		Total:    aiprovider.TokenUsage{InputTokens: 16, OutputTokens: 9},
		Billable: aiprovider.TokenUsage{InputTokens: 16, OutputTokens: 9},
	})

	require.Len(t, quota.recorded, 2)
	assert.Equal(t, "acme", quota.recorded[0].WorkspaceSlug)
	assert.Equal(t, "ws-1", quota.recorded[0].WorkspaceID)
	assert.Equal(t, "p1", quota.recorded[0].ProjectID)
	assert.Equal(t, "run-7", quota.recorded[0].JobID, "the run names the usage it spent")
	assert.Equal(t, "review", quota.recorded[0].Operation)
	assert.Equal(t, "sonnet", quota.recorded[0].Model)
	assert.Equal(t, 5, quota.recorded[0].TotalTokens)
	assert.Equal(t, "translate", quota.recorded[1].Operation)
	assert.Equal(t, 20, quota.recorded[1].TotalTokens)

	require.Len(t, bill.deductions, 1, "a run settles as one deduction")
	assert.Equal(t, "ws-1", bill.deductions[0].workspaceID)
	assert.Equal(t, billingOpFlowRun, bill.deductions[0].operation)
	assert.Equal(t, "p1:run-7:abc", bill.deductions[0].reference)
	assert.Equal(t, billing.TokensToCredits(25), bill.deductions[0].credits)
}

// A meter that is down costs the run its record, never the run: the tokens are
// already spent, so the deduction still happens.
func TestFlowAIAccountantDeductsWhenTheMeterIsDown(t *testing.T) {
	quota := &meterQuotaStore{remaining: 1_000_000, recordErr: assert.AnError}
	bill := &meterBillingStore{}
	acct := flowAIAccountant{srv: newMeteringServer(t, quota, bill)}

	acct.Record(context.Background(), service.AISpend{
		WorkspaceID: "ws-1",
		ReferenceID: "ref-1",
		ByOperation: []service.OperationSpend{
			{Operation: "translate", Model: "haiku", Source: service.SpendPlatformKey, Usage: aiprovider.TokenUsage{InputTokens: 10, OutputTokens: 10}},
		},
		Total:    aiprovider.TokenUsage{InputTokens: 10, OutputTokens: 10},
		Billable: aiprovider.TokenUsage{InputTokens: 10, OutputTokens: 10},
	})
	assert.Len(t, bill.deductions, 1)
}

// A run whose steps ran on the workspace's own key records every call against
// the cap and deducts nothing: credits pay for the platform's key alone.
func TestFlowAIAccountantRecordsAnOwnKeyRunWithoutDeducting(t *testing.T) {
	quota := &meterQuotaStore{remaining: 1_000_000}
	bill := &meterBillingStore{}
	acct := flowAIAccountant{srv: newMeteringServer(t, quota, bill)}

	acct.Record(context.Background(), service.AISpend{
		WorkspaceID: "ws-1",
		ProjectID:   "p1",
		ReferenceID: "p1:run-8:abc",
		ByOperation: []service.OperationSpend{
			{Operation: "translate", Model: "own", Source: service.SpendOwnKey, Usage: aiprovider.TokenUsage{InputTokens: 10, OutputTokens: 10}},
		},
		Total: aiprovider.TokenUsage{InputTokens: 10, OutputTokens: 10},
	})

	require.Len(t, quota.recorded, 1, "the cap must see a call the platform never paid for")
	assert.Equal(t, 20, quota.recorded[0].TotalTokens)
	assert.Empty(t, bill.deductions, "a bring-your-own key burns no credits")
}

// A mixed run deducts for the platform's share alone while the cap sees all of
// it.
func TestFlowAIAccountantDeductsOnlyThePlatformsShare(t *testing.T) {
	quota := &meterQuotaStore{remaining: 1_000_000}
	bill := &meterBillingStore{}
	acct := flowAIAccountant{srv: newMeteringServer(t, quota, bill)}

	acct.Record(context.Background(), service.AISpend{
		WorkspaceID: "ws-1",
		ReferenceID: "ref-mixed",
		ByOperation: []service.OperationSpend{
			{Operation: "review", Model: "own", Source: service.SpendOwnKey, Usage: aiprovider.TokenUsage{InputTokens: 30, OutputTokens: 0}},
			{Operation: "translate", Model: "haiku", Source: service.SpendPlatformKey, Usage: aiprovider.TokenUsage{InputTokens: 10, OutputTokens: 2}},
		},
		Total:    aiprovider.TokenUsage{InputTokens: 40, OutputTokens: 2},
		Billable: aiprovider.TokenUsage{InputTokens: 10, OutputTokens: 2},
	})

	require.Len(t, quota.recorded, 2)
	require.Len(t, bill.deductions, 1)
	assert.Equal(t, billing.TokensToCredits(12), bill.deductions[0].credits,
		"the deduction covered tokens the workspace paid for itself")
}

// A run that spent nothing settles nothing.
func TestFlowAIAccountantRecordsNothingForAnEmptySpend(t *testing.T) {
	quota := &meterQuotaStore{remaining: 1_000_000}
	bill := &meterBillingStore{}
	acct := flowAIAccountant{srv: newMeteringServer(t, quota, bill)}

	acct.Record(context.Background(), service.AISpend{WorkspaceID: "ws-1", ReferenceID: "ref-1"})
	assert.Empty(t, quota.recorded)
	assert.Empty(t, bill.deductions)
}

// Admission answers from both ledgers: a zero credit balance and a spent
// monthly ceiling each refuse the run, and everything unknown allows it.
func TestFlowAIAccountantAdmit(t *testing.T) {
	tests := []struct {
		name      string
		source    service.SpendSource
		spendable int64
		remaining int64
		checkErr  error
		noQuota   bool
		noBilling bool
		wantErr   error
	}{
		{name: "credits and quota allow", spendable: 5_000, remaining: 1_000},
		{name: "zero credits refuse", spendable: 0, remaining: 1_000, wantErr: service.ErrOutOfCredits},
		{name: "spent quota refuses", spendable: 5_000, remaining: 0, wantErr: errAIQuotaExceeded},
		{name: "unreadable quota allows", spendable: 5_000, checkErr: assert.AnError},
		{name: "no quota store allows", spendable: 5_000, noQuota: true},
		{name: "no billing store allows", remaining: 1_000, noBilling: true},
		// A step on the workspace's own key burns no credits, so an empty
		// balance says nothing about it. The ceiling still applies: it bounds
		// runaway usage whoever paid for it.
		{name: "own key ignores an empty balance", source: service.SpendOwnKey, spendable: 0, remaining: 1_000},
		{name: "own key still meets the ceiling", source: service.SpendOwnKey, spendable: 5_000, remaining: 0, wantErr: errAIQuotaExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quota := &meterQuotaStore{remaining: tt.remaining, checkErr: tt.checkErr}
			bill := &meterBillingStore{}
			bill.spendable = tt.spendable
			srv := newMeteringServer(t, quota, bill)
			if tt.noQuota {
				srv.QuotaStore = nil
			}
			if tt.noBilling {
				srv.BillingStore = nil
			}

			source := tt.source
			if source == "" {
				source = service.SpendPlatformKey
			}
			err := flowAIAccountant{srv: srv}.Admit(context.Background(), "ws-1", source)
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// A workspace whose slug cannot be resolved is still admitted: the abuse cap
// is keyed on the slug, and an unreadable one degrades to allowing the way
// every other unknown does.
func TestFlowAIAccountantAdmitsAnUnresolvableWorkspace(t *testing.T) {
	quota := &meterQuotaStore{remaining: 0}
	bill := &meterBillingStore{}
	bill.spendable = 5_000
	srv := newMeteringServer(t, quota, bill)
	srv.AuthStore = nil

	require.NoError(t, flowAIAccountant{srv: srv}.Admit(context.Background(), "ws-1", service.SpendPlatformKey))
}

// The wired server meters a real run end to end: a flow whose translate step
// calls the platform backend writes a usage row against the workspace and
// deducts the tokens it burned.
func TestWiredFlowRunReachesBothLedgers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PlatformProvider = string(aiprovider.Demo)
	srv := shutdownOnCleanup(t, NewServer(cfg))

	quota := &meterQuotaStore{remaining: 1_000_000}
	bill := &meterBillingStore{}
	bill.spendable = 100_000
	srv.QuotaStore = quota
	srv.BillingStore = bill
	srv.BillingHooks = &billing.UsageHooks{Store: bill}
	srv.AuthStore = &meterAuthStore{ws: &platauth.Workspace{ID: "ws-1", Slug: "acme"}}

	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	ctx := context.Background()
	require.NoError(t, cs.CreateProject(ctx, &store.Project{
		ID: "p1", Name: "Flow", WorkspaceID: "ws-1", DefaultSourceLanguage: "en",
	}))
	block := model.NewBlock("b1", "Hello there.")
	block.Translatable = true
	require.NoError(t, cs.StoreBlocksForItem(ctx, "p1", "main", "a.json", []*model.Block{block}))

	srv.Services = service.NewServices(cs, srv.ConnectorReg, srv.FormatRegistry, srv.ToolRegistry)
	srv.wireFlowAIProvider()

	def, err := srv.flowCatalog().Get(ctx, "p1", "translate")
	require.NoError(t, err)
	_, err = srv.Services.Flow.RunFlow(ctx, service.FlowRun{
		Definition:    def,
		ProjectID:     "p1",
		TargetLocales: []string{"fr"},
		RunID:         "run-3",
		Source:        "test",
	})
	require.NoError(t, err)

	require.NotEmpty(t, quota.recorded, "the run wrote no usage row")
	assert.Equal(t, "acme", quota.recorded[0].WorkspaceSlug)
	assert.Equal(t, "run-3", quota.recorded[0].JobID)
	assert.Equal(t, "translate", quota.recorded[0].Operation)
	assert.Positive(t, quota.recorded[0].TotalTokens)
	require.Len(t, bill.deductions, 1)
	assert.Equal(t, billingOpFlowRun, bill.deductions[0].operation)
	assert.Contains(t, bill.deductions[0].reference, "run-3")
}

// A zero-credit workspace fails the run with the reason, so the step that ran
// it closes with "out of AI credits" rather than a provider error.
func TestWiredFlowRunFailsAnExhaustedWorkspace(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PlatformProvider = string(aiprovider.Demo)
	srv := shutdownOnCleanup(t, NewServer(cfg))

	quota := &meterQuotaStore{remaining: 1_000_000}
	bill := &meterBillingStore{}
	srv.QuotaStore = quota
	srv.BillingStore = bill
	srv.BillingHooks = &billing.UsageHooks{Store: bill}
	srv.AuthStore = &meterAuthStore{ws: &platauth.Workspace{ID: "ws-1", Slug: "acme"}}

	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	ctx := context.Background()
	require.NoError(t, cs.CreateProject(ctx, &store.Project{
		ID: "p1", Name: "Flow", WorkspaceID: "ws-1", DefaultSourceLanguage: "en",
	}))
	block := model.NewBlock("b1", "Hello there.")
	block.Translatable = true
	require.NoError(t, cs.StoreBlocksForItem(ctx, "p1", "main", "a.json", []*model.Block{block}))

	srv.Services = service.NewServices(cs, srv.ConnectorReg, srv.FormatRegistry, srv.ToolRegistry)
	srv.wireFlowAIProvider()

	def, err := srv.flowCatalog().Get(ctx, "p1", "translate")
	require.NoError(t, err)
	_, err = srv.Services.Flow.RunFlow(ctx, service.FlowRun{
		Definition:    def,
		ProjectID:     "p1",
		TargetLocales: []string{"fr"},
		Source:        "test",
	})
	require.ErrorIs(t, err, service.ErrOutOfCredits)
	assert.Empty(t, quota.recorded, "a refused run spent nothing")
	assert.Empty(t, bill.deductions)
}
