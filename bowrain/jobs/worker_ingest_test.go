package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeIngestFetcher stands in for the worker's connector fetcher.
type fakeIngestFetcher struct {
	mu       sync.Mutex
	calls    int
	err      error
	lastWS   string
	lastConn string
	lastProj string
}

func (f *fakeIngestFetcher) Fetch(_ context.Context, ws, conn, proj string, _ platconn.FetchOptions) ([]*platconn.ContentItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastWS, f.lastConn, f.lastProj = ws, conn, proj
	if f.err != nil {
		return nil, f.err
	}
	return []*platconn.ContentItem{{ID: "en.json"}}, nil
}

func (f *fakeIngestFetcher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// recordingBus is a minimal platev.EventBus that records published events.
// (The real channel bus lives in bowrain/event, which imports this package —
// an internal-test import would cycle.)
type recordingBus struct {
	mu     sync.Mutex
	events []platev.Event
}

func (b *recordingBus) Publish(ev platev.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
}
func (b *recordingBus) Subscribe(platev.EventType, platev.EventHandler) *platev.Subscription {
	return nil
}
func (b *recordingBus) SubscribeAll(platev.EventHandler) *platev.Subscription           { return nil }
func (b *recordingBus) SubscribeGroup(string, platev.EventHandler) *platev.Subscription { return nil }
func (b *recordingBus) Unsubscribe(*platev.Subscription)                                {}
func (b *recordingBus) Close()                                                          {}

func (b *recordingBus) published() []platev.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]platev.Event(nil), b.events...)
}

// newIngestFixture wires WorkerDeps for the durable forge-ingest path against
// real Postgres job + connector-config stores (testcontainers).
func newIngestFixture(t *testing.T, fetcher *fakeIngestFetcher) (*WorkerDeps, *bstore.ConnectorConfigStore, *recordingBus) {
	t.Helper()
	db := pgtest.NewTestDB(t)
	// The content store runs the baseline migrations that create
	// connector_configs (mirrors worker boot).
	_, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	js, err := NewJobStore(db)
	require.NoError(t, err)
	cfgs := bstore.NewConnectorConfigStore(db.DB, nil)
	bus := &recordingBus{}
	return &WorkerDeps{
		JobStore:         js,
		ConnectorFetcher: fetcher,
		ConnectorConfigs: cfgs,
		EventBus:         bus,
		MaxJobAttempts:   2,
	}, cfgs, bus
}

func seedIngestConnector(t *testing.T, cfgs *bstore.ConnectorConfigStore) {
	t.Helper()
	_, err := cfgs.Upsert(t.Context(), &bstore.ConnectorConfig{
		ID: "conn1", WorkspaceID: "ws1", Type: "forge", Name: "site",
		Config: map[string]string{
			"repo": "https://github.com/acme/site.git", "branch": "main",
			"project_id": "proj1", "token": "tok",
		},
	})
	require.NoError(t, err)
}

func TestForgeIngestJob_SuccessStampsAndPublishes(t *testing.T) {
	fetcher := &fakeIngestFetcher{}
	deps, cfgs, bus := newIngestFixture(t, fetcher)
	seedIngestConnector(t, cfgs)
	ctx := t.Context()

	job := NewForgeIngestJob("ws1", "conn1", "proj1", "forge-webhook")
	require.NoError(t, deps.JobStore.CreateJob(ctx, job))
	require.NoError(t, processJobWithDeps(ctx, deps, job.ID))

	got, err := deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, got.Status)

	require.Equal(t, 1, fetcher.callCount())
	assert.Equal(t, "ws1", fetcher.lastWS)
	assert.Equal(t, "conn1", fetcher.lastConn)
	assert.Equal(t, "proj1", fetcher.lastProj)

	// The successful ingest stamps last-sync (clearing any recorded error).
	cfg, err := cfgs.Get(ctx, "ws1", "conn1")
	require.NoError(t, err)
	assert.False(t, cfg.LastSyncAt.IsZero())
	assert.Empty(t, cfg.LastError)

	// EventPushCompleted goes out with the same payload the in-process path
	// always published — this is what starts the on-push convergence run.
	evs := bus.published()
	require.Len(t, evs, 1)
	assert.Equal(t, platev.EventPushCompleted, evs[0].Type)
	assert.Equal(t, "proj1", evs[0].ProjectID)
	assert.Equal(t, "forge-webhook", evs[0].Source)
	assert.Equal(t, "conn1", evs[0].Data["connector_id"])
	assert.Equal(t, "acme/site", evs[0].Data["repo"])
	assert.Equal(t, "main", evs[0].Data["branch"])
}

func TestForgeIngestJob_FetchFailureRetriesThenFails(t *testing.T) {
	fetcher := &fakeIngestFetcher{err: errors.New("git clone: fatal: unable to access repo: exit status 128")}
	deps, cfgs, bus := newIngestFixture(t, fetcher)
	seedIngestConnector(t, cfgs)
	ctx := t.Context()

	job := NewForgeIngestJob("ws1", "conn1", "proj1", "forge-webhook")
	require.NoError(t, deps.JobStore.CreateJob(ctx, job))

	// Attempt 1: transient — the row goes back to 'queued' for redelivery.
	err := processJobWithDeps(ctx, deps, job.ID)
	var te *transientError
	require.ErrorAs(t, err, &te, "a fetch failure with budget left must signal a retry")
	got, gerr := deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, gerr)
	assert.Equal(t, StatusQueued, got.Status)

	// The failure lands on the connector row while retries run, so the status
	// path surfaces it instead of an endless "importing".
	cfg, cerr := cfgs.Get(ctx, "ws1", "conn1")
	require.NoError(t, cerr)
	assert.Contains(t, cfg.LastError, "exit status 128")

	// Attempt 2 (budget = 2): terminal failure.
	err = processJobWithDeps(ctx, deps, job.ID)
	require.Error(t, err)
	assert.NotErrorAs(t, err, &te, "an exhausted budget is terminal, not a retry")
	got, gerr = deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, gerr)
	assert.Equal(t, StatusFailed, got.Status)
	assert.Contains(t, got.Error, "exit status 128")

	assert.Empty(t, bus.published(), "a failed ingest must not announce a push")
	assert.Equal(t, 2, fetcher.callCount())
}

func TestForgeIngestJob_ConnectorRemovedIsTerminal(t *testing.T) {
	fetcher := &fakeIngestFetcher{}
	deps, _, bus := newIngestFixture(t, fetcher)
	ctx := t.Context()

	job := NewForgeIngestJob("ws1", "gone", "proj1", "forge-webhook")
	require.NoError(t, deps.JobStore.CreateJob(ctx, job))
	require.NoError(t, processJobWithDeps(ctx, deps, job.ID),
		"a connector removed while queued is a clean drop, not an operational error")

	got, err := deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, got.Status)
	assert.Contains(t, got.Error, "no longer exists")
	assert.Zero(t, fetcher.callCount())
	assert.Empty(t, bus.published())
}

func TestForgeIngestJob_MissingWiringFailsPermanently(t *testing.T) {
	deps, cfgs, _ := newIngestFixture(t, &fakeIngestFetcher{})
	deps.ConnectorFetcher = nil
	seedIngestConnector(t, cfgs)
	ctx := t.Context()

	job := NewForgeIngestJob("ws1", "conn1", "proj1", "forge-webhook")
	require.NoError(t, deps.JobStore.CreateJob(ctx, job))
	require.Error(t, processJobWithDeps(ctx, deps, job.ID))
	got, err := deps.JobStore.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, got.Status)
}
