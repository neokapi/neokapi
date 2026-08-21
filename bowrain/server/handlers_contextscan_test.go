package server

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/jobs"
	bloblocal "github.com/neokapi/neokapi/bowrain/storage/localblob"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wireContextScan attaches a real Postgres brand-scan store, an in-memory
// queue, and a local blob store to a test server.
func wireContextScan(t *testing.T, s *Server) {
	t.Helper()
	db := pgtest.NewTestDB(t)
	store, err := jobs.NewContextScanJobStore(db)
	require.NoError(t, err)
	s.ContextScanStore = store
	s.ContextScanQueue = jobs.NewChannelQueue(8)
	if s.BlobStore == nil {
		blobs, berr := bloblocal.New(t.TempDir())
		require.NoError(t, berr)
		s.BlobStore = blobs
	}
}

// TestContextScanInfoFeature: /api/v1/info advertises the context_scan capability
// only when the job system (store + queue) is wired, so the web app can gate
// the onboarding CTAs instead of surfacing a 503 on unconfigured deployments.
func TestContextScanInfoFeature(t *testing.T) {
	s, _ := newTestServer(t)

	fetchInfo := func() InfoResponse {
		rec := httptest.NewRecorder()
		s.GetEcho().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/info", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		var resp InfoResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		return resp
	}

	assert.False(t, fetchInfo().Features["context_scan"], "unwired server must not advertise context_scan")

	wireContextScan(t, s)
	assert.True(t, fetchInfo().Features["context_scan"], "wired server must advertise context_scan")
}

// TestContextScanCreate_Authz: a non-member is rejected by workspace access
// resolution, and a plain member (no PermManageVoice) by the permission guard.
func TestContextScanCreate_Authz(t *testing.T) {
	s, _ := newTestServer(t)
	wireContextScan(t, s)
	body := `{"paste_text":"We write plainly."}`

	// Non-member: not part of the workspace at all.
	outsider := &platauth.User{ID: "outsider-1", Email: "outsider@example.com", Name: "Outsider"}
	require.NoError(t, s.AuthStore.CreateUser(t.Context(), outsider))
	outsiderToken, err := platauth.GenerateToken(outsider, "test-secret", time.Hour)
	require.NoError(t, err)
	code := do(t, s, http.MethodPost, "/api/v1/test/brand-scans", outsiderToken, body)
	assert.True(t, code == http.StatusForbidden || code == http.StatusNotFound,
		"non-member must be rejected, got %d", code)

	// Member without PermManageVoice.
	memberToken := addWorkspaceMember(t, s, "bs-member", "bs-member@example.com", platauth.RoleMember)
	code = do(t, s, http.MethodPost, "/api/v1/test/brand-scans", memberToken, body)
	assert.Equal(t, http.StatusForbidden, code, "a plain member lacks PermManageVoice")
}

// TestContextScanCreate_RequiresSource: an empty request is a 400, not a queued
// no-op job.
func TestContextScanCreate_RequiresSource(t *testing.T) {
	s, ownerToken := newTestServer(t)
	wireContextScan(t, s)

	code := do(t, s, http.MethodPost, "/api/v1/test/brand-scans", ownerToken, `{}`)
	assert.Equal(t, http.StatusBadRequest, code)

	code = do(t, s, http.MethodPost, "/api/v1/test/brand-scans", ownerToken, `{"paste_text":"   "}`)
	assert.Equal(t, http.StatusBadRequest, code, "whitespace-only paste is not a source")

	// URL cap: at most 5 links per scan.
	over := `{"urls":["https://a.example","https://b.example","https://c.example","https://d.example","https://e.example","https://f.example"]}`
	code = do(t, s, http.MethodPost, "/api/v1/test/brand-scans", ownerToken, over)
	assert.Equal(t, http.StatusBadRequest, code)
}

// zeroCreditBillingStore stubs the billing store with an exhausted balance.
// The enqueue pre-check reads CheckCredits; the workspace middlewares touch
// GetCurrentAllocation, GrantTrialCredits (the free-plan lazy trial backfill),
// and GetFeatureOverrides on every request.
type zeroCreditBillingStore struct{ billing.BillingStore }

func (zeroCreditBillingStore) CheckCredits(context.Context, string) (int64, error) { return 0, nil }

func (zeroCreditBillingStore) GetCurrentAllocation(context.Context, string) (*billing.CreditAllocation, error) {
	return &billing.CreditAllocation{CreditsTotal: 100, CreditsUsed: 100}, nil
}

func (zeroCreditBillingStore) GrantTrialCredits(context.Context, string, int64) (bool, error) {
	return false, nil // already granted (and spent) — this store is the exhausted state
}

func (zeroCreditBillingStore) GetFeatureOverrides(context.Context, string) ([]billing.FeatureOverride, error) {
	return nil, nil
}

// TestContextScanCreate_InsufficientCredits402: the handler's platform-credit
// pre-check refuses a zero-credit workspace with 402 — scans always run on
// the platform key and deduction is post-hoc, so enqueueing would drive the
// ledger negative. (On the full route, QuotaGuard middleware may refuse the
// same workspace earlier with 429 when a plan is resolved; the pre-check
// covers the paths QuotaGuard skips, e.g. no plan on the request context.)
func TestContextScanCreate_InsufficientCredits402(t *testing.T) {
	s := &Server{wsStores: newWorkspaceStores(), BillingStore: zeroCreditBillingStore{}}
	db := pgtest.NewTestDB(t)
	store, err := jobs.NewContextScanJobStore(db)
	require.NoError(t, err)
	s.ContextScanStore = store
	s.ContextScanQueue = jobs.NewChannelQueue(8)

	e := echo.New()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/test/brand-scans",
		strings.NewReader(`{"paste_text":"We write plainly."}`))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws")
	c.SetParamValues("test")
	c.Set("workspace_id", "test-ws")
	c.Set("project_permissions", platauth.PermAll)

	require.NoError(t, s.HandleCreateContextScan(c))
	assert.Equal(t, http.StatusPaymentRequired, rec.Code, rec.Body.String())

	// No job row and no queue message may exist after the refusal.
	_, _, _, qerr := s.ContextScanQueue.(*jobs.ChannelQueue).Dequeue(canceledContext(t))
	assert.Error(t, qerr, "a refused scan must not enqueue")
}

// TestContextScanCreate_ZeroCreditWorkspaceRefusedOnRoute: through the full
// route stack a zero-credit workspace is refused before any job is created
// (QuotaGuard 429 when a plan resolves, or the handler's 402 pre-check).
func TestContextScanCreate_ZeroCreditWorkspaceRefusedOnRoute(t *testing.T) {
	s, ownerToken := newTestServer(t)
	wireContextScan(t, s)
	s.BillingStore = zeroCreditBillingStore{}

	code := do(t, s, http.MethodPost, "/api/v1/test/brand-scans", ownerToken, `{"paste_text":"We write plainly."}`)
	assert.True(t, code == http.StatusPaymentRequired || code == http.StatusTooManyRequests,
		"a zero-credit workspace must be refused, got %d", code)

	// Nothing was enqueued.
	_, _, _, qerr := s.ContextScanQueue.Dequeue(canceledContext(t))
	assert.Error(t, qerr, "a refused scan must not enqueue")
}

// canceledContext returns an already-cancelled context so a queue Dequeue
// probe returns immediately instead of blocking.
func canceledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}

// TestContextScanCreate_EnqueuesJob: the happy path returns 202 with a job id,
// persists the queued row, and publishes exactly one queue message.
func TestContextScanCreate_EnqueuesJob(t *testing.T) {
	s, ownerToken := newTestServer(t)
	wireContextScan(t, s)

	body := `{"paste_text":"We write plainly.","profile_name":"Acme Voice","domain":"developer tools"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/test/brand-scans", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+ownerToken)
	rec := httptest.NewRecorder()
	s.GetEcho().ServeHTTP(rec, r)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	var resp struct {
		JobID  string `json:"job_id"`
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.JobID)
	assert.Equal(t, "queued", resp.Status)

	job, err := s.ContextScanStore.GetContextScanJob(t.Context(), resp.JobID)
	require.NoError(t, err)
	assert.Equal(t, "test", job.WorkspaceSlug)
	assert.Equal(t, "test-ws", job.WorkspaceID, "the billing workspace id must be persisted for metering")

	var req jobs.ContextScanRequest
	require.NoError(t, json.Unmarshal(job.Request, &req))
	assert.Equal(t, "Acme Voice", req.ProfileName)

	queued, _, _, err := s.ContextScanQueue.Dequeue(t.Context())
	require.NoError(t, err)
	assert.Equal(t, resp.JobID, queued)
}

// TestContextScanGet_CrossTenant404: a scan owned by another workspace is 404
// (anti-enumeration) even for an authenticated owner of a different workspace.
func TestContextScanGet_CrossTenant404(t *testing.T) {
	s, ownerToken := newTestServer(t)
	wireContextScan(t, s)
	ctx := t.Context()

	victim := &jobs.ContextScanJob{
		ID:            "victim-scan",
		WorkspaceID:   "other-ws",
		WorkspaceSlug: "other",
		Status:        jobs.ContextScanStatusCompleted,
		Request:       json.RawMessage(`{"paste_text":"secret"}`),
	}
	require.NoError(t, s.ContextScanStore.CreateContextScanJob(ctx, victim))

	code := do(t, s, http.MethodGet, "/api/v1/test/brand-scans/victim-scan", ownerToken, "")
	assert.Equal(t, http.StatusNotFound, code, "cross-tenant scan reads must 404")

	// A same-workspace job is visible, and a completed one carries the draft.
	mine := &jobs.ContextScanJob{
		ID:            "my-scan",
		WorkspaceID:   "test-ws",
		WorkspaceSlug: "test",
		Status:        jobs.ContextScanStatusQueued,
		Request:       json.RawMessage(`{"paste_text":"ours"}`),
	}
	require.NoError(t, s.ContextScanStore.CreateContextScanJob(ctx, mine))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/test/brand-scans/my-scan", nil)
	r.Header.Set("Authorization", "Bearer "+ownerToken)
	rec := httptest.NewRecorder()
	s.GetEcho().ServeHTTP(rec, r)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "my-scan", resp["id"])
	assert.Equal(t, "queued", resp["status"])
	_, hasDraft := resp["draft"]
	assert.False(t, hasDraft, "a non-completed scan carries no draft")
}

// TestContextScanGet_CompletedCarriesDraft: a completed scan returns the result
// payload under "draft".
func TestContextScanGet_CompletedCarriesDraft(t *testing.T) {
	s, ownerToken := newTestServer(t)
	wireContextScan(t, s)
	ctx := t.Context()

	job := &jobs.ContextScanJob{
		ID:            "done-scan",
		WorkspaceID:   "test-ws",
		WorkspaceSlug: "test",
		Status:        jobs.ContextScanStatusQueued,
		Request:       json.RawMessage(`{"paste_text":"ours"}`),
	}
	require.NoError(t, s.ContextScanStore.CreateContextScanJob(ctx, job))
	claimed, epoch, err := s.ContextScanStore.ClaimContextScanJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	result := json.RawMessage(`{"profile":{"name":"Draft"},"terms":[],"sources":[],"truncated":false}`)
	owner, err := s.ContextScanStore.CompleteContextScanJob(ctx, job.ID, epoch, result, 42)
	require.NoError(t, err)
	require.True(t, owner)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/test/brand-scans/done-scan", nil)
	r.Header.Set("Authorization", "Bearer "+ownerToken)
	rec := httptest.NewRecorder()
	s.GetEcho().ServeHTTP(rec, r)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Status     string          `json:"status"`
		TokensUsed int             `json:"tokens_used"`
		Draft      json.RawMessage `json:"draft"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "completed", resp.Status)
	assert.Equal(t, 42, resp.TokensUsed)
	assert.JSONEq(t, string(result), string(resp.Draft))
}

// contextScanMultipart builds a multipart body with the given name → content
// files under the "files" field.
func contextScanMultipart(t *testing.T, files map[string][]byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for name, content := range files {
		fw, err := w.CreateFormFile("files", name)
		require.NoError(t, err)
		_, err = fw.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return &buf, w.FormDataContentType()
}

func newContextScanUploadContext(t *testing.T, body *bytes.Buffer, contentType string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/acme/brand-scans/uploads", body)
	r.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws")
	c.SetParamValues("acme")
	c.Set("project_permissions", platauth.PermAll)
	return c, rec
}

type contextScanUploadResponse struct {
	Uploads []struct {
		Key      string `json:"key"`
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
	} `json:"uploads"`
	Skipped []struct {
		Name   string `json:"name"`
		Reason string `json:"reason"`
	} `json:"skipped"`
}

// TestContextScanUploads_StoresAndSkips: importable files are stored as envelope
// blobs; unsupported (pdf, unknown extension) and oversize files come back in
// "skipped" with reasons.
func TestContextScanUploads_StoresAndSkips(t *testing.T) {
	blobs, err := bloblocal.New(t.TempDir())
	require.NoError(t, err)
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	s := &Server{BlobStore: blobs, FormatRegistry: reg, wsStores: newWorkspaceStores()}

	oversize := bytes.Repeat([]byte("a"), (10<<20)+1)
	body, contentType := contextScanMultipart(t, map[string][]byte{
		"voice-guide.md": []byte("# Voice\n\nWe write plainly."),
		"notes.xyz":      []byte("not importable"),
		"brand-deck.pdf": []byte("%PDF-1.4 stub"),
		"huge.md":        oversize,
	})
	c, rec := newContextScanUploadContext(t, body, contentType)

	require.NoError(t, s.HandleContextScanUploads(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp contextScanUploadResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	require.Len(t, resp.Uploads, 1)
	assert.Equal(t, "voice-guide.md", resp.Uploads[0].Filename)
	assert.NotEmpty(t, resp.Uploads[0].Key)
	assert.Positive(t, resp.Uploads[0].Size)

	reasons := map[string]string{}
	for _, sk := range resp.Skipped {
		reasons[sk.Name] = sk.Reason
	}
	require.Len(t, reasons, 3)
	assert.Contains(t, reasons["notes.xyz"], "unsupported")
	assert.Contains(t, reasons["brand-deck.pdf"], "unsupported",
		"pdf has no in-process reader (pdfium reads out of core), so a server-side scan reports it unsupported")
	assert.Contains(t, reasons["huge.md"], "too large")

	// The stored blob is a decodable envelope carrying the original filename.
	rc, err := blobs.Download(t.Context(), resp.Uploads[0].Key)
	require.NoError(t, err)
	defer rc.Close()
	var env jobs.ContextScanUploadEnvelope
	require.NoError(t, json.NewDecoder(rc).Decode(&env))
	assert.Equal(t, "voice-guide.md", env.Filename)
	assert.Equal(t, []byte("# Voice\n\nWe write plainly."), env.Data)
}

// TestContextScanUploads_Caps: more than 10 files is rejected outright.
func TestContextScanUploads_Caps(t *testing.T) {
	blobs, err := bloblocal.New(t.TempDir())
	require.NoError(t, err)
	s := &Server{BlobStore: blobs, wsStores: newWorkspaceStores()}

	files := map[string][]byte{}
	for i := range 11 {
		files[strings.Repeat("f", i+1)+".md"] = []byte("text")
	}
	body, contentType := contextScanMultipart(t, files)
	c, rec := newContextScanUploadContext(t, body, contentType)

	require.NoError(t, s.HandleContextScanUploads(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// TestContextScanUploads_RequiresPermission: without PermManageVoice the upload
// endpoint denies (fail-closed permission context).
func TestContextScanUploads_RequiresPermission(t *testing.T) {
	blobs, err := bloblocal.New(t.TempDir())
	require.NoError(t, err)
	s := &Server{BlobStore: blobs, wsStores: newWorkspaceStores()}

	body, contentType := contextScanMultipart(t, map[string][]byte{"a.md": []byte("x")})
	c, rec := newContextScanUploadContext(t, body, contentType)
	c.Set("project_permissions", platauth.PermViewContent) // no manage-brand

	err = s.HandleContextScanUploads(c)
	require.Error(t, err, "requirePermission signals denial via error")
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestContextScanCheckDraft: the stateless tester scores sample text against an
// inline draft profile — forbidden vocabulary must be caught and scored.
func TestContextScanCheckDraft(t *testing.T) {
	s := &Server{wsStores: newWorkspaceStores()}

	body := `{
		"profile": {
			"name": "Draft",
			"vocabulary": {"forbidden_terms": [{"term": "cheap", "replacement": "affordable"}]}
		},
		"text": "Our cheap plan is great."
	}`
	e := echo.New()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/acme/brand-scans/check-draft", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws")
	c.SetParamValues("acme")

	require.NoError(t, s.HandleCheckBrandDraft(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp struct {
		Score struct {
			Overall float64 `json:"overall"`
		} `json:"score"`
		Findings []map[string]any `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Findings, 1, "the forbidden term must be flagged")
	assert.Less(t, resp.Score.Overall, 100.0)

	// Clean text scores clean.
	cleanBody := strings.Replace(body, "Our cheap plan is great.", "Our affordable plan is great.", 1)
	r = httptest.NewRequest(http.MethodPost, "/api/v1/acme/brand-scans/check-draft", strings.NewReader(cleanBody))
	r.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	c = e.NewContext(r, rec)
	require.NoError(t, s.HandleCheckBrandDraft(c))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Empty(t, resp.Findings)

	// Validation: text and profile are both required.
	for _, invalid := range []string{`{"text":"x"}`, `{"profile":{"name":"d"}}`} {
		r = httptest.NewRequest(http.MethodPost, "/api/v1/acme/brand-scans/check-draft", strings.NewReader(invalid))
		r.Header.Set("Content-Type", "application/json")
		rec = httptest.NewRecorder()
		c = e.NewContext(r, rec)
		require.NoError(t, s.HandleCheckBrandDraft(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code, invalid)
	}
}
