package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/safehttp/safehttptest"
)

// fakePostHogAPI is a minimal PostHog query API for handler tests: it accepts
// one API key + project id, serves a single canned demand result set, and
// counts query calls (for cache assertions).
type fakePostHogAPI struct {
	srv *httptest.Server
	// baseURL addresses srv through the SSRF policy — see newFakePostHogAPI.
	baseURL    string
	queryCalls atomic.Int64
	status     int // non-zero forces this HTTP status on every query
}

func newFakePostHogAPI(t *testing.T) *fakePostHogAPI {
	t.Helper()
	f := &fakePostHogAPI{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/projects/{pid}/query", func(w http.ResponseWriter, r *http.Request) {
		f.queryCalls.Add(1)
		if f.status != 0 {
			w.WriteHeader(f.status)
			_, _ = w.Write([]byte(`{"detail":"forced error"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer phx_live_key_9876" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.PathValue("pid") != "12345" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var req struct {
			Query struct {
				Query string `json:"query"`
			} `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		var results [][]any
		switch {
		case strings.Contains(req.Query.Query, "SELECT 1"):
			results = [][]any{{float64(1)}}
		case strings.Contains(req.Query.Query, "toStartOfWeek"):
			results = [][]any{{"2026-06-22", "nb-NO", float64(7)}}
		case strings.Contains(req.Query.Query, "extract("):
			results = [][]any{{"nb-NO", "nb", float64(7)}, {"nb-NO", "", float64(3)}}
		default:
			results = [][]any{{"NO", "nb-NO", float64(10)}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	// A self-hosted PostHog host is dialed through the platform's SSRF policy,
	// which refuses the loopback address an httptest listener binds to. Address
	// the fixture through the policy instead of around it, so these handler
	// tests keep exercising the real client.
	f.baseURL = safehttptest.Serve(t, f.srv)
	return f
}

// createPostHogTestProject creates a project through the API and returns its id.
func createPostHogTestProject(t *testing.T, srv *Server, token string) string {
	t.Helper()
	body := `{"name":"Demand","default_source_language":"en","target_languages":["nb"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/test/projects", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)
	var project struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &project))
	return project.ID
}

func posthogJSON(t *testing.T, srv *Server, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, req)
	return rec
}

func TestPostHogConfigSaveGetAndSecretMasking(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createPostHogTestProject(t, srv, token)
	ph := newFakePostHogAPI(t)

	// Unconfigured GET reports configured=false.
	rec := posthogJSON(t, srv, token, http.MethodGet, "/api/v1/test/"+pid+"/connectors/posthog", "")
	require.Equal(t, http.StatusOK, rec.Code)
	var cfg PostHogConfigResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cfg))
	assert.False(t, cfg.Configured)

	// PUT runs the test connection ("SELECT 1") and saves.
	body := `{"host":"` + ph.baseURL + `","posthog_project_id":"12345","api_key":"phx_live_key_9876"}`
	rec = posthogJSON(t, srv, token, http.MethodPut, "/api/v1/test/"+pid+"/connectors/posthog", body)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.EqualValues(t, 1, ph.queryCalls.Load(), "save must run exactly one test query")
	assert.NotContains(t, rec.Body.String(), "phx_live_key_9876", "PUT response must not echo the key")

	// GET returns config with the key masked as •••• + last-4, never raw.
	rec = posthogJSON(t, srv, token, http.MethodGet, "/api/v1/test/"+pid+"/connectors/posthog", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cfg))
	assert.True(t, cfg.Configured)
	assert.Equal(t, "12345", cfg.PostHogProjectID)
	assert.Equal(t, "••••9876", cfg.APIKeyMasked)
	assert.NotEmpty(t, cfg.PathLocalePattern, "default pattern is persisted")
	assert.NotContains(t, rec.Body.String(), "phx_live_key_9876", "GET must never serialize the secret")

	// The generic collections API redacts the key too (shared secret pattern).
	rec = posthogJSON(t, srv, token, http.MethodGet, "/api/v1/test/"+pid+"/collections/main", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "phx_live_key_9876", "collections API must redact the key")
	assert.Contains(t, rec.Body.String(), `"api_key"`, "redacted key name is reported in connector_secret_keys")

	// Update without api_key keeps the stored secret (carry-forward rule).
	body = `{"host":"` + ph.baseURL + `","posthog_project_id":"12345"}`
	rec = posthogJSON(t, srv, token, http.MethodPut, "/api/v1/test/"+pid+"/connectors/posthog", body)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cfg))
	assert.Equal(t, "••••9876", cfg.APIKeyMasked, "omitted key is carried forward")

	// Disconnect.
	rec = posthogJSON(t, srv, token, http.MethodDelete, "/api/v1/test/"+pid+"/connectors/posthog", "")
	require.Equal(t, http.StatusNoContent, rec.Code)
	rec = posthogJSON(t, srv, token, http.MethodGet, "/api/v1/test/"+pid+"/connectors/posthog", "")
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cfg))
	assert.False(t, cfg.Configured)
}

func TestPostHogConfigTestConnectionErrors(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createPostHogTestProject(t, srv, token)
	ph := newFakePostHogAPI(t)

	put := func(body string) (int, PostHogErrorResponse) {
		rec := posthogJSON(t, srv, token, http.MethodPut, "/api/v1/test/"+pid+"/connectors/posthog", body)
		var e PostHogErrorResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		return rec.Code, e
	}

	// Wrong key → bad_key.
	code, e := put(`{"host":"` + ph.baseURL + `","posthog_project_id":"12345","api_key":"phx_wrong"}`)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "bad_key", e.Code)

	// Wrong project → bad_project.
	code, e = put(`{"host":"` + ph.baseURL + `","posthog_project_id":"777","api_key":"phx_live_key_9876"}`)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "bad_project", e.Code)

	// Unreachable host → bad_host. The address is captured while the listener
	// is still open, then the listener goes away, so the dial is refused rather
	// than the URL.
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := safehttptest.Serve(t, dead)
	dead.Close()
	code, e = put(`{"host":"` + deadURL + `","posthog_project_id":"12345","api_key":"phx_live_key_9876"}`)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "bad_host", e.Code)

	// Nothing was persisted after failed tests.
	rec := posthogJSON(t, srv, token, http.MethodGet, "/api/v1/test/"+pid+"/connectors/posthog", "")
	var cfg PostHogConfigResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cfg))
	assert.False(t, cfg.Configured, "failed test connection must not persist config")
}

func TestPostHogDemandEndpointAndCache(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createPostHogTestProject(t, srv, token)
	ph := newFakePostHogAPI(t)

	demandPath := "/api/v1/test/" + pid + "/connectors/posthog/demand"

	// Not configured → 404 with a machine code the UI can key off.
	rec := posthogJSON(t, srv, token, http.MethodGet, demandPath, "")
	require.Equal(t, http.StatusNotFound, rec.Code)
	var e PostHogErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &e))
	assert.Equal(t, "not_configured", e.Code)

	// Connect.
	body := `{"host":"` + ph.baseURL + `","posthog_project_id":"12345","api_key":"phx_live_key_9876"}`
	rec = posthogJSON(t, srv, token, http.MethodPut, "/api/v1/test/"+pid+"/connectors/posthog", body)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	callsAfterSave := ph.queryCalls.Load()

	// First demand fetch hits PostHog (3 queries) and reports provenance.
	rec = posthogJSON(t, srv, token, http.MethodGet, demandPath+"?range=7d", "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var demand PostHogDemandResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &demand))
	assert.False(t, demand.Cached)
	assert.Equal(t, "posthog", demand.Source.Kind)
	assert.Equal(t, "12345", demand.Source.PostHogProjectID)
	assert.EqualValues(t, 10, demand.TotalSessions)
	require.Len(t, demand.Countries, 1)
	assert.Equal(t, "NO", demand.Countries[0].CountryCode)
	require.Len(t, demand.Languages, 1)
	assert.Equal(t, "nb-NO", demand.Languages[0].Tag)
	require.NotNil(t, demand.ServedLocaleHitRate)
	assert.InDelta(t, 0.7, *demand.ServedLocaleHitRate, 0.0001)
	assert.Equal(t, callsAfterSave+3, ph.queryCalls.Load())

	// Second fetch is served from the 1h cache — no PostHog traffic.
	rec = posthogJSON(t, srv, token, http.MethodGet, demandPath+"?range=7d", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &demand))
	assert.True(t, demand.Cached)
	assert.Equal(t, callsAfterSave+3, ph.queryCalls.Load(), "cache hit must not query PostHog")

	// A different range is a different cache entry.
	rec = posthogJSON(t, srv, token, http.MethodGet, demandPath+"?range=90d", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, callsAfterSave+6, ph.queryCalls.Load())

	// refresh=true bypasses the cache.
	rec = posthogJSON(t, srv, token, http.MethodGet, demandPath+"?range=7d&refresh=true", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &demand))
	assert.False(t, demand.Cached)
	assert.Equal(t, callsAfterSave+9, ph.queryCalls.Load())

	// Invalid range → 400.
	rec = posthogJSON(t, srv, token, http.MethodGet, demandPath+"?range=365d", "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Upstream auth failure at read time → 502 with the classified code.
	ph.status = http.StatusUnauthorized
	rec = posthogJSON(t, srv, token, http.MethodGet, demandPath+"?range=30d", "")
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &e))
	assert.Equal(t, "bad_key", e.Code)
}
