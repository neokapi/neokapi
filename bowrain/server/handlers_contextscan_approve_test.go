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
	wireContextScan(t, srv)

	job := &jobs.ContextScanJob{
		ID:            id.New(),
		WorkspaceID:   approveWSID,
		WorkspaceSlug: approveWSSlug,
		Status:        jobs.ContextScanStatusCompleted,
		Request:       json.RawMessage(`{"paste_text":"We write plainly."}`),
	}
	require.NoError(t, srv.ContextScanStore.CreateContextScanJob(t.Context(), job))
	require.NoError(t, srv.ContextScanStore.UpdateContextScanJobStatus(
		t.Context(), job.ID, jobs.ContextScanStatusCompleted, ""))
	return srv, job.ID
}

// approveScan posts body to HandleApproveContextScan for scanID.
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
	return rec, srv.HandleApproveContextScan(c)
}

const approvePayload = `{
	"profile": {"name": "Acme Voice", "description": "How Acme sounds."},
	"locale": "en-US",
	"terms": [
		{"term": "sign in", "domain": "auth", "definition": "start a session"},
		{"term": "cart", "domain": "commerce"}
	]
}`

// TestContextScanApprove_AppliesProfileAndTermsInOneCall is the whole point:
// one request, and both halves land.
func TestContextScanApprove_AppliesProfileAndTermsInOneCall(t *testing.T) {
	srv, scanID := setupScanApproval(t)

	rec, err := approveScan(t, srv, scanID, approvePayload)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var got ContextScanApproveResponse
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

// TestContextScanApprove_RetryIsIdempotent: replaying the same approval must
// converge, not double the vocabulary.
func TestContextScanApprove_RetryIsIdempotent(t *testing.T) {
	srv, scanID := setupScanApproval(t)

	_, err := approveScan(t, srv, scanID, approvePayload)
	require.NoError(t, err)

	rec, err := approveScan(t, srv, scanID, approvePayload)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var got ContextScanApproveResponse
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

// TestContextScanApprove_ValidatesBeforeWriting: a malformed term must not leave
// a profile stored without its vocabulary.
func TestContextScanApprove_ValidatesBeforeWriting(t *testing.T) {
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

func TestContextScanApprove_RequiresANameAndACompletedScan(t *testing.T) {
	srv, scanID := setupScanApproval(t)

	rec, err := approveScan(t, srv, scanID, `{"profile":{"name":""}}`)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	queued := &jobs.ContextScanJob{
		ID: id.New(), WorkspaceID: approveWSID, WorkspaceSlug: approveWSSlug,
		Status: jobs.ContextScanStatusQueued, Request: json.RawMessage(`{"paste_text":"x"}`),
	}
	require.NoError(t, srv.ContextScanStore.CreateContextScanJob(t.Context(), queued))
	rec, err = approveScan(t, srv, queued.ID, approvePayload)
	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

// TestContextScanApprove_OtherWorkspacesScanIsNotFound keeps the approval on the
// same anti-enumeration footing as the scan read.
func TestContextScanApprove_OtherWorkspacesScanIsNotFound(t *testing.T) {
	srv, _ := setupScanApproval(t)

	foreign := &jobs.ContextScanJob{
		ID: id.New(), WorkspaceID: "ws-other", WorkspaceSlug: "other",
		Status: jobs.ContextScanStatusCompleted, Request: json.RawMessage(`{"paste_text":"x"}`),
	}
	require.NoError(t, srv.ContextScanStore.CreateContextScanJob(t.Context(), foreign))

	rec, err := approveScan(t, srv, foreign.ID, approvePayload)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestContextScanApprove_RequiresManageBrand(t *testing.T) {
	srv, scanID := setupScanApproval(t)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(approvePayload))
	req.Header.Set("Content-Type", echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := srv.GetEcho().NewContext(req, rec)
	c.Set("project_permissions", platauth.PermViewContent)
	c.Set("workspace_id", approveWSID)
	c.SetParamNames("ws", "id")
	c.SetParamValues(approveWSSlug, scanID)

	assert.Error(t, srv.HandleApproveContextScan(c))
}

// An artefact binds at a point, and a point is only real once a recipe declares
// it and a push carries it. Approving at an axis nobody has declared would put a
// coordinate into the graph that no content sits at — so the server refuses,
// rather than the UI merely greying the row, because the CLI client and any API
// caller reach the same endpoint.
func TestContextScanApprove_RefusesAnUndeclaredAxis(t *testing.T) {
	srv, scanID := setupScanApproval(t)

	rec, err := approveScan(t, srv, scanID,
		`{"at":{"product_line":"cloud"},"profile":{"name":"Acme Cloud"}}`)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), "product_line",
		"the error must name the axis that is missing")
	assert.Contains(t, rec.Body.String(), "product_line=cloud",
		"and the point the caller asked for")
}

// The default point is where a scan that found no structure proposes, and it is
// bindable before any push has happened — otherwise onboarding would depend on
// content that does not exist yet.
func TestContextScanApprove_AcceptsTheDefaultPoint(t *testing.T) {
	srv, scanID := setupScanApproval(t)

	// No `at` at all.
	rec, err := approveScan(t, srv, scanID, approvePayload)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	// An explicitly empty one means the same thing.
	_, scanID2 := setupScanApproval(t)
	rec, err = approveScan(t, srv, scanID2, `{"at":{},"profile":{"name":"Acme"}}`)
	require.NoError(t, err)
	assert.NotEqual(t, http.StatusConflict, rec.Code,
		"an empty point is the default point, never an undeclared one")
}
