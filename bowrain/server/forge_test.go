package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/bowrain/forge"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/service"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubForgeConnector stands in for the live forge connector: Fetch returns one
// item (or fetchErr when set), Publish records what it was handed. statusErr
// mirrors the real connector when the repository is unreachable: Status
// re-runs the clone/list, so the same failure that broke the ingest also
// breaks every status probe.
type stubForgeConnector struct {
	id        string
	fetched   int
	fetchErr  error
	statusErr error

	// mu guards published/pubOpts: the forge-delivery subscriber writes them
	// from its own goroutine (subscribeForgeDelivery fires delivery async), so
	// tests that observe delivery must read through the accessors below.
	mu        sync.Mutex
	published []*platconn.ContentItem
	pubOpts   platconn.PublishOptions
}

func (f *stubForgeConnector) ID() string                               { return f.id }
func (f *stubForgeConnector) Name() string                             { return "stub" }
func (f *stubForgeConnector) Category() platconn.Category              { return platconn.CategoryCode }
func (f *stubForgeConnector) Configure(config map[string]string) error { return nil }
func (f *stubForgeConnector) Close() error                             { return nil }
func (f *stubForgeConnector) Status(ctx context.Context) (*platconn.SyncStatus, error) {
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	return &platconn.SyncStatus{}, nil
}
func (f *stubForgeConnector) Fetch(ctx context.Context, opts platconn.FetchOptions) ([]*platconn.ContentItem, error) {
	f.fetched++
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	return []*platconn.ContentItem{{ID: "en.txt", Name: "en.txt", Path: "en.txt", Format: "plaintext",
		Blocks: []*model.Block{model.NewBlock("greeting", "Hello")}}}, nil
}
func (f *stubForgeConnector) List(ctx context.Context) ([]*platconn.ContentItem, error) {
	return nil, nil
}
func (f *stubForgeConnector) Publish(ctx context.Context, items []*platconn.ContentItem, opts platconn.PublishOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published = items
	f.pubOpts = opts
	return nil
}

// Published returns the items handed to the most recent Publish. Reads are
// guarded so a test goroutine can poll delivery while the async forge-delivery
// subscriber may still be writing; the mutex establishes the happens-before
// edge that makes the returned items safe to inspect.
func (f *stubForgeConnector) Published() []*platconn.ContentItem {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.published
}

// PublishOpts returns the options from the most recent Publish, guarded like
// Published.
func (f *stubForgeConnector) PublishOpts() platconn.PublishOptions {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pubOpts
}

// newForgeTestServer wires the minimal server the forge surface needs: SQLite
// content + connector-config stores, a channel event bus, and a live stub
// connector registered under the persisted config's id.
func newForgeTestServer(t *testing.T, cfgID, projectID, secret string) (*Server, *stubForgeConnector) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	reg := platconn.NewRegistry()
	stub := &stubForgeConnector{id: cfgID}
	reg.Register("forge", platconn.CategoryCode, func(config map[string]string) (platconn.IntegrationConnector, error) {
		// The shared stub serves the harness's pinned connector; anything else
		// (e.g. a setup-flow bind) gets its own identity, mirroring how the
		// real constructor derives ids — a pinned-id stub would collide with
		// the pre-seeded config row.
		if config["id"] == "" && config["name"] != "" {
			return &stubForgeConnector{id: "forge-" + config["name"]}, nil
		}
		return stub, nil
	})
	formatReg := registry.NewFormatRegistry()
	toolReg := registry.NewToolRegistry()

	bus := event.NewChannelEventBus()
	t.Cleanup(bus.Close)
	s := &Server{
		ConnectorReg:         reg,
		FormatRegistry:       formatReg,
		ToolRegistry:         toolReg,
		ContentStore:         cs,
		EventBus:             bus,
		ConnectorConfigStore: bstore.NewConnectorConfigStore(cs.DB(), nil),
		// The GitHub App setup surface gates on installation ownership, so the
		// harness carries the store that records it (SQLite migration 12).
		ForgeInstallationStore: bstore.NewForgeInstallationStore(cs.DB()),
		Services:               service.NewServices(cs, reg, formatReg, toolReg),
		Config:                 Config{JWTSecret: "test-secret"},
	}

	// Persist the forge config and register the matching live instance, the
	// state boot rehydration leaves behind.
	_, err = s.ConnectorConfigStore.Upsert(context.Background(), &bstore.ConnectorConfig{
		ID: cfgID, WorkspaceID: "ws1", Type: "forge", Name: "site",
		Config: map[string]string{
			"id": cfgID, "repo": "https://github.com/acme/site.git", "branch": "main",
			"token": "tok", "webhook_secret": secret, "project_id": projectID,
		},
	})
	require.NoError(t, err)
	_, err = s.Services.Connector.AddConnector("ws1", "forge", map[string]string{"id": cfgID})
	require.NoError(t, err)

	// The project the connector feeds — block storage enforces the FK. It
	// belongs to ws1, the workspace every request in these tests is scoped to:
	// binding a repository into a project checks that ownership too.
	require.NoError(t, cs.CreateProject(context.Background(), &platstore.Project{
		ID: projectID, Name: "Site", DefaultSourceLanguage: "en", WorkspaceID: "ws1",
		TargetLanguages: []model.LocaleID{"fr", "de"},
	}))
	return s, stub
}

func githubSign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func postForgeWebhook(s *Server, cfgID string, body string, headers map[string]string) *httptest.ResponseRecorder {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/forge/"+cfgID, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("configID")
	c.SetParamValues(cfgID)
	_ = s.HandleForgeWebhook(c)
	return rec
}

func TestForgeWebhook_VerifiesAndTriggersPush(t *testing.T) {
	s, stub := newForgeTestServer(t, "conn1", "proj1", "s3cret")

	pushed := make(chan platev.Event, 1)
	s.EventBus.Subscribe(platev.EventPushCompleted, func(ev platev.Event) { pushed <- ev })

	body := `{"ref":"refs/heads/main","repository":{"full_name":"acme/site"}}`

	// Wrong signature: rejected, nothing fetched.
	rec := postForgeWebhook(s, "conn1", body, map[string]string{"X-Hub-Signature-256": "sha256=deadbeef"})
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// Unknown connector id: 404, indistinguishable from wrong-type ids.
	rec = postForgeWebhook(s, "nope", body, map[string]string{"X-Hub-Signature-256": githubSign("s3cret", []byte(body))})
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// Valid GitHub-signed push to the tracked branch: accepted, re-ingested,
	// and EventPushCompleted goes out (which is what starts the on-push run).
	rec = postForgeWebhook(s, "conn1", body, map[string]string{"X-Hub-Signature-256": githubSign("s3cret", []byte(body))})
	assert.Equal(t, http.StatusAccepted, rec.Code)
	select {
	case ev := <-pushed:
		assert.Equal(t, "proj1", ev.ProjectID)
		assert.Equal(t, "forge-webhook", ev.Source)
		assert.Equal(t, "acme/site", ev.Data["repo"])
	case <-time.After(5 * time.Second):
		t.Fatal("no EventPushCompleted after a valid webhook")
	}
	assert.Equal(t, 1, stub.fetched)

	// A push to the delivery branch is acknowledged and ignored — that is the
	// loop guard for the connector's own pushes.
	other := `{"ref":"refs/heads/bowrain/translations","repository":{"full_name":"acme/site"}}`
	rec = postForgeWebhook(s, "conn1", other, map[string]string{"X-Hub-Signature-256": githubSign("s3cret", []byte(other))})
	assert.Equal(t, http.StatusAccepted, rec.Code)

	// Non-push payloads (e.g. a ping) are acknowledged and ignored.
	ping := `{"zen":"Design for failure."}`
	rec = postForgeWebhook(s, "conn1", ping, map[string]string{"X-Hub-Signature-256": githubSign("s3cret", []byte(ping))})
	assert.Equal(t, http.StatusAccepted, rec.Code)

	assert.Equal(t, 1, stub.fetched, "only the tracked-branch push fetches")
}

func TestForgeDelivery_MaterializesPerLocaleAndPublishes(t *testing.T) {
	s, stub := newForgeTestServer(t, "conn1", "proj1", "s3cret")
	ctx := context.Background()

	require.NoError(t, s.ContentStore.StoreItem(ctx, "proj1", "main", &platstore.Item{
		ID: "item1", ProjectID: "proj1", Name: "locales/en/app.txt", Format: "plaintext",
	}))
	b := model.NewBlock("greeting", "Hello")
	b.SetTargetText("fr", "Bonjour")
	// proj1 is governed (workflow default), so delivery ships only approved
	// targets (RV-A): mark fr reviewed so it materializes.
	b.Target("fr").Status = model.TargetStatusReviewed
	require.NoError(t, s.ContentStore.StoreBlocksForItem(ctx, "proj1", "main", "locales/en/app.txt", []*model.Block{b}))

	s.deliverToForges(ctx, platev.Event{
		Type: platev.EventConvergenceRunCompleted, ProjectID: "proj1",
		Data: map[string]string{"run_id": "run1", "state": bstore.ConvergenceRunConverged, "passes": "2"},
	})

	published := stub.Published()
	require.Len(t, published, 1, "only fr has translations; de must not deliver empty")
	item := published[0]
	assert.Equal(t, "locales/fr/app.txt", item.Path)
	assert.Equal(t, model.LocaleID("fr"), item.Locale)
	require.Len(t, item.Blocks, 1)
	assert.Equal(t, "Bonjour", item.Blocks[0].SourceText(), "the target is promoted to the source position for the writer")

	opts := stub.PublishOpts()
	assert.Contains(t, opts.Metadata["pr_title"], "Update translations")
	assert.Contains(t, opts.Metadata["pr_body"], "converged")
	assert.Contains(t, opts.Message, "run1")
}

func TestForgeDelivery_IgnoresFailedRuns(t *testing.T) {
	s, stub := newForgeTestServer(t, "conn1", "proj1", "s3cret")
	s.subscribeForgeDelivery()
	s.EventBus.Publish(platev.Event{
		Type: platev.EventConvergenceRunCompleted, ProjectID: "proj1",
		Data: map[string]string{"state": bstore.ConvergenceRunFailed},
	})
	time.Sleep(100 * time.Millisecond)
	assert.Nil(t, stub.Published(), "failed runs deliver nothing")
}

func TestGitHubAppWebhook_RoutesByRepo(t *testing.T) {
	s, stub := newForgeTestServer(t, "conn1", "proj1", "s3cret")

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	app, err := forge.NewGitHubApp("1", string(pemBytes), "app-secret")
	require.NoError(t, err)
	s.GitHubApp = app

	pushed := make(chan platev.Event, 1)
	s.EventBus.Subscribe(platev.EventPushCompleted, func(ev platev.Event) { pushed <- ev })

	post := func(body string, headers map[string]string) *httptest.ResponseRecorder {
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/github-app", strings.NewReader(body))
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		_ = s.HandleGitHubAppWebhook(e.NewContext(req, rec))
		return rec
	}

	body := `{"ref":"refs/heads/main","repository":{"full_name":"acme/site"}}`

	// Bad signature: rejected.
	rec := post(body, map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature-256": "sha256=deadbeef"})
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// An installation payload carrying no installation id is acknowledged
	// without action — there is nothing to record.
	rec = post(`{"action":"created"}`, map[string]string{
		"X-GitHub-Event": "installation", "X-Hub-Signature-256": githubSign("app-secret", []byte(`{"action":"created"}`))})
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Equal(t, 0, stub.fetched)

	// A push on a repository no connector tracks is not ours.
	other := `{"ref":"refs/heads/main","repository":{"full_name":"acme/other"}}`
	rec = post(other, map[string]string{
		"X-GitHub-Event": "push", "X-Hub-Signature-256": githubSign("app-secret", []byte(other))})
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.Equal(t, 0, stub.fetched)

	// A push on the tracked repository's tracked branch routes to the
	// connector: re-ingest + the push event that starts convergence.
	rec = post(body, map[string]string{
		"X-GitHub-Event": "push", "X-Hub-Signature-256": githubSign("app-secret", []byte(body))})
	assert.Equal(t, http.StatusAccepted, rec.Code)
	select {
	case ev := <-pushed:
		assert.Equal(t, "proj1", ev.ProjectID)
		assert.Equal(t, "acme/site", ev.Data["repo"])
	case <-time.After(5 * time.Second):
		t.Fatal("no EventPushCompleted after an app push webhook")
	}
	assert.Equal(t, 1, stub.fetched)
}

func TestTargetPathFor(t *testing.T) {
	cases := []struct{ src, sourceLang, lang, want string }{
		{"locales/en/app.json", "en", "fr", "locales/fr/app.json"},
		{"content/en.json", "en", "de", "content/de.json"},
		{"docs/en/guide/index.md", "en", "nb", "docs/nb/guide/index.md"},
		{"app.json", "en", "fr", "app.fr.json"},
		{"src/messages.yaml", "en", "ja", "src/messages.ja.yaml"},
		{"locales/EN/app.json", "en", "fr", "locales/fr/app.json"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, targetPathFor(tc.src, tc.sourceLang, tc.lang), "%s (%s→%s)", tc.src, tc.sourceLang, tc.lang)
	}
}

// memJobStore is a minimal in-memory jobs.JobStore for the server's durable-
// ingest enqueue path: only the methods that path touches are implemented; the
// embedded interface nil-panics on anything else (which a test would surface).
type memJobStore struct {
	jobs.JobStore
	mu   sync.Mutex
	rows map[string]*jobs.TranslationJob
}

func newMemJobStore() *memJobStore {
	return &memJobStore{rows: map[string]*jobs.TranslationJob{}}
}

func (m *memJobStore) CreateJob(_ context.Context, job *jobs.TranslationJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *job
	m.rows[job.ID] = &cp
	return nil
}

func (m *memJobStore) DeleteJob(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, id)
	return nil
}

func (m *memJobStore) GetJob(_ context.Context, id string) (*jobs.TranslationJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.rows[id]
	if !ok {
		return nil, fmt.Errorf("job %s not found", id)
	}
	cp := *j
	return &cp, nil
}

func (m *memJobStore) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.rows)
}

// TestForgeWebhook_EnqueuesDurableIngestJob pins the durable path from the
// production drill: with the job system wired, a verified push webhook is
// acked 202 only after the ingest is safely enqueued for bowrain-worker —
// never run as a fire-and-forget goroutine that dies with a draining task.
func TestForgeWebhook_EnqueuesDurableIngestJob(t *testing.T) {
	s, stub := newForgeTestServer(t, "conn1", "proj1", "s3cret")
	store := newMemJobStore()
	queue := jobs.NewChannelQueue(4)
	t.Cleanup(func() { _ = queue.Close() })
	s.JobStore = store
	s.JobQueue = queue

	pushed := make(chan platev.Event, 1)
	s.EventBus.Subscribe(platev.EventPushCompleted, func(ev platev.Event) { pushed <- ev })

	body := `{"ref":"refs/heads/main","repository":{"full_name":"acme/site"}}`
	rec := postForgeWebhook(s, "conn1", body,
		map[string]string{"X-Hub-Signature-256": githubSign("s3cret", []byte(body))})
	require.Equal(t, http.StatusAccepted, rec.Code)

	// The ack means "queued", not "done": a broker message + job row exist...
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	jobID, ack, _, err := queue.Dequeue(ctx)
	require.NoError(t, err)
	ack()

	job, err := store.GetJob(context.Background(), jobID)
	require.NoError(t, err)
	assert.True(t, job.IsForgeIngest())
	assert.Equal(t, jobs.ForgeIngestItemName, job.ItemName)
	assert.Equal(t, "conn1", job.IngestConnectorID())
	assert.Equal(t, "proj1", job.ProjectID)
	assert.Equal(t, "ws1", job.WorkspaceID)
	assert.Equal(t, "forge-webhook", job.IngestSource())
	assert.Equal(t, jobs.StatusQueued, job.Status)

	// ...and nothing ran inline: the fetch and the push event are the
	// worker's job now.
	assert.Equal(t, 0, stub.fetched, "the durable path must not fetch inline")
	select {
	case <-pushed:
		t.Fatal("EventPushCompleted must come from the worker, not the enqueue")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestForgeWebhook_FallsBackInProcessWhenEnqueueFails pins the degradation
// path: a broken broker must not drop the push — the server falls back to the
// previous in-process ingest and rolls back the orphaned job row.
func TestForgeWebhook_FallsBackInProcessWhenEnqueueFails(t *testing.T) {
	s, stub := newForgeTestServer(t, "conn1", "proj1", "s3cret")
	store := newMemJobStore()
	queue := jobs.NewChannelQueue(1)
	_ = queue.Close() // every Enqueue now fails
	s.JobStore = store
	s.JobQueue = queue

	pushed := make(chan platev.Event, 1)
	s.EventBus.Subscribe(platev.EventPushCompleted, func(ev platev.Event) { pushed <- ev })

	body := `{"ref":"refs/heads/main","repository":{"full_name":"acme/site"}}`
	rec := postForgeWebhook(s, "conn1", body,
		map[string]string{"X-Hub-Signature-256": githubSign("s3cret", []byte(body))})
	require.Equal(t, http.StatusAccepted, rec.Code)

	// The in-process fallback ingests and announces the push, as before.
	select {
	case ev := <-pushed:
		assert.Equal(t, "proj1", ev.ProjectID)
		assert.Equal(t, "acme/site", ev.Data["repo"])
	case <-time.After(5 * time.Second):
		t.Fatal("no EventPushCompleted from the in-process fallback")
	}
	assert.Equal(t, 1, stub.fetched)
	assert.Equal(t, 0, store.count(), "the dead job row is rolled back, nothing strands in 'queued'")
}

// The app-level webhook is the only authentic notice the server gets that an
// installation exists, changed scope, or is gone. It is signed by GitHub but
// names no workspace, so it RECORDS an installation and never attributes one:
// a recorded installation is reachable from nobody until a signed setup state
// claims it. An uninstall drops the record immediately, so a workspace's claim
// cannot outlive the access it was granted.
func TestGitHubAppWebhook_RecordsInstallationOwnership(t *testing.T) {
	s, _ := newForgeTestServer(t, "conn1", "proj1", "s3cret")

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	app, err := forge.NewGitHubApp("1", string(pemBytes), "app-secret")
	require.NoError(t, err)
	s.GitHubApp = app

	post := func(t *testing.T, event, body string) *httptest.ResponseRecorder {
		t.Helper()
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/github-app", strings.NewReader(body))
		req.Header.Set("X-GitHub-Event", event)
		req.Header.Set("X-Hub-Signature-256", githubSign("app-secret", []byte(body)))
		rec := httptest.NewRecorder()
		_ = s.HandleGitHubAppWebhook(e.NewContext(req, rec))
		return rec
	}

	created := `{"action":"created","installation":{"id":4242,"account":{"login":"acme"}}}`
	ctx := t.Context()

	t.Run("created records the installation, unclaimed", func(t *testing.T) {
		require.Equal(t, http.StatusAccepted, post(t, "installation", created).Code)

		inst, err := s.ForgeInstallationStore.Get(ctx, 4242)
		require.NoError(t, err)
		assert.Equal(t, int64(4242), inst.InstallationID)
		assert.Equal(t, "acme", inst.Account, "the account is carried for display")
		assert.False(t, inst.Claimed(), "a webhook names no workspace, so it cannot attribute one")

		owned, err := s.ForgeInstallationStore.OwnedBy(ctx, 4242, "ws1")
		require.NoError(t, err)
		assert.False(t, owned, "an unclaimed installation is reachable from no workspace")
	})

	t.Run("an unsigned delivery records nothing", func(t *testing.T) {
		e := echo.New()
		body := `{"action":"created","installation":{"id":5150,"account":{"login":"mallory"}}}`
		req := httptest.NewRequest(http.MethodPost, "/api/webhooks/github-app", strings.NewReader(body))
		req.Header.Set("X-GitHub-Event", "installation")
		req.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")
		rec := httptest.NewRecorder()
		_ = s.HandleGitHubAppWebhook(e.NewContext(req, rec))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)

		_, err := s.ForgeInstallationStore.Get(ctx, 5150)
		require.ErrorIs(t, err, bstore.ErrForgeInstallationNotFound)
	})

	t.Run("installation_repositories refreshes without disturbing the owner", func(t *testing.T) {
		_, err := s.ForgeInstallationStore.Claim(ctx, 4242, "ws1")
		require.NoError(t, err)

		added := `{"action":"added","installation":{"id":4242,"account":{"login":"acme-renamed"}}}`
		require.Equal(t, http.StatusAccepted, post(t, "installation_repositories", added).Code)

		inst, err := s.ForgeInstallationStore.Get(ctx, 4242)
		require.NoError(t, err)
		assert.Equal(t, "acme-renamed", inst.Account)
		assert.Equal(t, "ws1", inst.WorkspaceID, "a scope change must never unbind the installation")
	})

	t.Run("deleted forgets the installation", func(t *testing.T) {
		deleted := `{"action":"deleted","installation":{"id":4242,"account":{"login":"acme"}}}`
		require.Equal(t, http.StatusAccepted, post(t, "installation", deleted).Code)

		_, err := s.ForgeInstallationStore.Get(ctx, 4242)
		require.ErrorIs(t, err, bstore.ErrForgeInstallationNotFound,
			"an uninstall revokes the claim there and then")

		owned, err := s.ForgeInstallationStore.OwnedBy(ctx, 4242, "ws1")
		require.NoError(t, err)
		assert.False(t, owned)
	})
}
