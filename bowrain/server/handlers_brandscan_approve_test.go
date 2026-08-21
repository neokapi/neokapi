package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/core/id"
)

// The scan-approval endpoint replaces a client-side loop — create the profile,
// then one createConcept per approved term — that had no atomicity and no
// idempotency: a failure halfway left a profile with some of its vocabulary,
// and a retry created the rest twice.

const (
	approveWSID   = "ws-scan-approve"
	approveWSSlug = "test"
)

// setupScanApproval wires a server with a brand store, a brand-scan store, and
// one completed scan to approve.
func setupScanApproval(t *testing.T) (*Server, string) {
	t.Helper()
	srv := setupBrandLoopServer(t)
	wireBrandScan(t, srv)

	job := &jobs.BrandScanJob{
		ID:            id.New(),
		WorkspaceID:   approveWSID,
		WorkspaceSlug: approveWSSlug,
		Status:        jobs.BrandScanStatusCompleted,
		Request:       json.RawMessage(`{"paste_text":"We write plainly."}`),
	}
	require.NoError(t, srv.BrandScanStore.CreateBrandScanJob(t.Context(), job))
	require.NoError(t, srv.BrandScanStore.UpdateBrandScanJobStatus(
		t.Context(), job.ID, jobs.BrandScanStatusCompleted, ""))
	return srv, job.ID
}

// approveScan posts body to HandleApproveBrandScan for scanID.
func approveScan(t *testing.T, srv *Server, scanID, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/"+approveWSSlug+"/brand-scans/"+scanID+"/approve", strings.NewReader(body))
	req.Header.Set("Content-Type", echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := srv.GetEcho().NewContext(req, rec)
	c.Set("project_permissions", platauth.PermAll)
	c.Set("workspace_id", approveWSID)
	c.Set("user_id", "u-approve")
	c.SetParamNames("ws", "id")
	c.SetParamValues(approveWSSlug, scanID)
	return rec, srv.HandleApproveBrandScan(c)
}

const approvePayload = `{
	"profile": {"name": "Acme Voice", "description": "How Acme sounds."},
	"locale": "en-US",
	"terms": [
		{"term": "sign in", "domain": "auth", "definition": "start a session"},
		{"term": "cart", "domain": "commerce"}
	]
}`

// TestBrandScanApprove_AppliesProfileAndTermsInOneCall is the whole point:
// one request, and both halves land.
func TestBrandScanApprove_AppliesProfileAndTermsInOneCall(t *testing.T) {
	srv, scanID := setupScanApproval(t)

	rec, err := approveScan(t, srv, scanID, approvePayload)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var got BrandScanApproveResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, brandUpsertCreated, got.ProfileAction)
	require.NotNil(t, got.Profile)
	assert.Equal(t, "Acme Voice", got.Profile.Name)
	assert.Equal(t, 2, got.ConceptsCreated)
	assert.Zero(t, got.ConceptsExisting)
	require.Len(t, got.ConceptIDs, 2)

	tb, err := srv.wsStores.getTerms(approveWSSlug)
	require.NoError(t, err)
	concept, ok, err := tb.GetConcept(t.Context(), got.ConceptIDs[0])
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, concept.Terms, 1)
	assert.Equal(t, "sign in", concept.Terms[0].Text)
	// A scan never bypasses governance: a candidate enters curation proposed.
	assert.Equal(t, "proposed", string(concept.Terms[0].Status))
	assert.Equal(t, "en-US", string(concept.Terms[0].Locale))
	assert.Equal(t, "auth", concept.Domain)
}

// TestBrandScanApprove_RetryIsIdempotent: replaying the same approval must
// converge, not double the vocabulary.
func TestBrandScanApprove_RetryIsIdempotent(t *testing.T) {
	srv, scanID := setupScanApproval(t)

	_, err := approveScan(t, srv, scanID, approvePayload)
	require.NoError(t, err)

	rec, err := approveScan(t, srv, scanID, approvePayload)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var got BrandScanApproveResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, brandUpsertUnchanged, got.ProfileAction)
	assert.Zero(t, got.ConceptsCreated)
	assert.Equal(t, 2, got.ConceptsExisting)

	tb, err := srv.wsStores.getTerms(approveWSSlug)
	require.NoError(t, err)
	count, err := tb.Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, count, "a retry must not create the same concepts again")

	profiles, err := srv.VoiceStore.ListProfiles(t.Context(), approveWSID)
	require.NoError(t, err)
	assert.Len(t, profiles, 1)
}

// TestBrandScanApprove_ValidatesBeforeWriting: a malformed term must not leave
// a profile stored without its vocabulary.
func TestBrandScanApprove_ValidatesBeforeWriting(t *testing.T) {
	srv, scanID := setupScanApproval(t)

	rec, err := approveScan(t, srv, scanID,
		`{"profile":{"name":"Acme Voice"},"terms":[{"term":"cart"},{"term":"  "}]}`)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	profiles, err := srv.VoiceStore.ListProfiles(t.Context(), approveWSID)
	require.NoError(t, err)
	assert.Empty(t, profiles, "nothing is written when the request does not validate")

	tb, err := srv.wsStores.getTerms(approveWSSlug)
	require.NoError(t, err)
	count, err := tb.Count(t.Context())
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestBrandScanApprove_RequiresANameAndACompletedScan(t *testing.T) {
	srv, scanID := setupScanApproval(t)

	rec, err := approveScan(t, srv, scanID, `{"profile":{"name":""}}`)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	queued := &jobs.BrandScanJob{
		ID: id.New(), WorkspaceID: approveWSID, WorkspaceSlug: approveWSSlug,
		Status: jobs.BrandScanStatusQueued, Request: json.RawMessage(`{"paste_text":"x"}`),
	}
	require.NoError(t, srv.BrandScanStore.CreateBrandScanJob(t.Context(), queued))
	rec, err = approveScan(t, srv, queued.ID, approvePayload)
	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

// TestBrandScanApprove_OtherWorkspacesScanIsNotFound keeps the approval on the
// same anti-enumeration footing as the scan read.
func TestBrandScanApprove_OtherWorkspacesScanIsNotFound(t *testing.T) {
	srv, _ := setupScanApproval(t)

	foreign := &jobs.BrandScanJob{
		ID: id.New(), WorkspaceID: "ws-other", WorkspaceSlug: "other",
		Status: jobs.BrandScanStatusCompleted, Request: json.RawMessage(`{"paste_text":"x"}`),
	}
	require.NoError(t, srv.BrandScanStore.CreateBrandScanJob(t.Context(), foreign))

	rec, err := approveScan(t, srv, foreign.ID, approvePayload)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestBrandScanApprove_RequiresManageBrand(t *testing.T) {
	srv, scanID := setupScanApproval(t)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(approvePayload))
	req.Header.Set("Content-Type", echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := srv.GetEcho().NewContext(req, rec)
	c.Set("project_permissions", platauth.PermViewContent)
	c.Set("workspace_id", approveWSID)
	c.SetParamNames("ws", "id")
	c.SetParamValues(approveWSSlug, scanID)

	assert.Error(t, srv.HandleApproveBrandScan(c))
}
