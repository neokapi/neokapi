package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/bowrain/service"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubForgeConnector stands in for the live forge connector: Fetch returns one
// item, Publish records what it was handed.
type stubForgeConnector struct {
	id        string
	fetched   int
	published []*platconn.ContentItem
	pubOpts   platconn.PublishOptions
}

func (f *stubForgeConnector) ID() string                                  { return f.id }
func (f *stubForgeConnector) Name() string                                { return "stub" }
func (f *stubForgeConnector) Category() platconn.Category                 { return platconn.CategoryCode }
func (f *stubForgeConnector) Configure(config map[string]string) error    { return nil }
func (f *stubForgeConnector) Close() error                                { return nil }
func (f *stubForgeConnector) Status(ctx context.Context) (*platconn.SyncStatus, error) {
	return &platconn.SyncStatus{}, nil
}
func (f *stubForgeConnector) Fetch(ctx context.Context, opts platconn.FetchOptions) ([]*platconn.ContentItem, error) {
	f.fetched++
	return []*platconn.ContentItem{{ID: "en.txt", Name: "en.txt", Path: "en.txt", Format: "plaintext",
		Blocks: []*model.Block{model.NewBlock("greeting", "Hello")}}}, nil
}
func (f *stubForgeConnector) List(ctx context.Context) ([]*platconn.ContentItem, error) {
	return nil, nil
}
func (f *stubForgeConnector) Publish(ctx context.Context, items []*platconn.ContentItem, opts platconn.PublishOptions) error {
	f.published = items
	f.pubOpts = opts
	return nil
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
		Services:             service.NewServices(cs, reg, formatReg, toolReg),
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

	// The project the connector feeds — block storage enforces the FK.
	require.NoError(t, cs.CreateProject(context.Background(), &platstore.Project{
		ID: projectID, Name: "Site", DefaultSourceLanguage: "en",
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
	require.NoError(t, s.ContentStore.StoreBlocksForItem(ctx, "proj1", "main", "locales/en/app.txt", []*model.Block{b}))

	s.deliverToForges(ctx, platev.Event{
		Type: platev.EventConvergenceRunCompleted, ProjectID: "proj1",
		Data: map[string]string{"run_id": "run1", "state": bstore.ConvergenceRunConverged, "passes": "2"},
	})

	require.Len(t, stub.published, 1, "only fr has translations; de must not deliver empty")
	item := stub.published[0]
	assert.Equal(t, "locales/fr/app.txt", item.Path)
	assert.Equal(t, model.LocaleID("fr"), item.Locale)
	require.Len(t, item.Blocks, 1)
	assert.Equal(t, "Bonjour", item.Blocks[0].SourceText(), "the target is promoted to the source position for the writer")

	assert.Contains(t, stub.pubOpts.Metadata["pr_title"], "Update translations")
	assert.Contains(t, stub.pubOpts.Metadata["pr_body"], "converged")
	assert.Contains(t, stub.pubOpts.Message, "run1")
}

func TestForgeDelivery_IgnoresFailedRuns(t *testing.T) {
	s, stub := newForgeTestServer(t, "conn1", "proj1", "s3cret")
	s.subscribeForgeDelivery()
	s.EventBus.Publish(platev.Event{
		Type: platev.EventConvergenceRunCompleted, ProjectID: "proj1",
		Data: map[string]string{"state": bstore.ConvergenceRunFailed},
	})
	time.Sleep(100 * time.Millisecond)
	assert.Nil(t, stub.published, "failed runs deliver nothing")
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
