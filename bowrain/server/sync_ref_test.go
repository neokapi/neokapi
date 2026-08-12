package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/apierror"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	coresync "github.com/neokapi/neokapi/bowrain/core/sync"
	"github.com/neokapi/neokapi/bowrain/service"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/ref"
)

// The freshness ref, end to end over a real content store: what the server
// publishes, and what its compare-and-swap refuses.
//
// A real SQLite store rather than a fake, because every component is DERIVED
// from stored rows and a fake would be answering with the very value under
// test. Two "clients" are two requests carrying different assertions — which is
// exactly what two clients are to this server.

const refTestStream = "main"

type refHarness struct {
	srv     *Server
	store   *sqlitestore.SQLiteStore
	project *platstore.Project
}

func newRefHarness(t *testing.T) *refHarness {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	ctx := t.Context()
	p := &platstore.Project{
		Name:                  "refs",
		DefaultSourceLanguage: model.LocaleEnglish,
		TargetLanguages:       []model.LocaleID{model.LocaleFrench},
	}
	require.NoError(t, cs.CreateProject(ctx, p))
	require.NoError(t, EnsureMainStream(ctx, cs, p.ID))

	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))
	srv.ContentStore = cs
	srv.Services = service.NewServices(cs, nil, nil, nil)
	return &refHarness{srv: srv, store: cs, project: p}
}

// ctx builds a request context on a sync route, authorized for content work.
func (h *refHarness) ctx(t *testing.T, method, target, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, r)
	if body != "" {
		req.Header.Set("Content-Type", echo.MIMEApplicationJSON)
	}
	rec := httptest.NewRecorder()
	c := h.srv.GetEcho().NewContext(req, rec)
	c.Set("project_permissions", platauth.PermAll)
	c.Set("user_id", "tester")
	c.SetParamNames("id", "ref")
	c.SetParamValues(h.project.ID, refTestStream)
	return c, rec
}

// currentRef reads the ref the server publishes, through the handler.
func (h *refHarness) currentRef(t *testing.T) ref.Ref {
	t.Helper()
	c, rec := h.ctx(t, http.MethodGet, "/ref", "")
	require.NoError(t, h.srv.HandleSyncRef(c))
	require.Equal(t, http.StatusOK, rec.Code)
	var out ref.Ref
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out
}

// declareCollection writes a recipe-owned collection the way the push worker
// does, so the context component moves exactly as a real reconcile moves it.
func (h *refHarness) declareCollection(t *testing.T, name, contextHash string) {
	t.Helper()
	require.NoError(t, h.store.CreateCollection(t.Context(), &platstore.Collection{
		ProjectID:   h.project.ID,
		Name:        name,
		Kind:        platstore.CollectionConnected,
		Stream:      refTestStream,
		Owner:       coresync.ContextOwnerRecipe,
		ContextHash: contextHash,
	}))
}

// decide writes one decision into the ledger.
func (h *refHarness) decide(t *testing.T, unit, reviewState string) {
	t.Helper()
	_, err := h.store.UpsertUnitDecisions(t.Context(), h.project.ID, refTestStream,
		[]platstore.UnitDecision{{
			ItemName: "docs/intro.md", Unit: unit, Variant: "fr",
			Status: "reviewed", ReviewState: reviewState, DecidedBy: "ana",
			Updated: "2026-08-01T00:00:00Z",
		}})
	require.NoError(t, err)
}

// storeBlocks pushes source content straight into the store, which is what
// moves the position component and nothing else.
func (h *refHarness) storeBlocks(t *testing.T, n int) {
	t.Helper()
	blocks := make([]*model.Block, 0, n)
	for i := range n {
		blocks = append(blocks, model.NewBlock(fmt.Sprintf("msg.%d", i), fmt.Sprintf("Message %d", i)))
	}
	require.NoError(t, h.store.StoreBlocks(t.Context(), h.project.ID, refTestStream, blocks))
}

// TestSyncRef_PublishesAStreamsStanding is the publication contract: a fresh
// project claims nothing, and every component moves with the state it names.
func TestSyncRef_PublishesAStreamsStanding(t *testing.T) {
	h := newRefHarness(t)

	fresh := h.currentRef(t)
	assert.Equal(t, int64(0), fresh.Content)
	assert.Empty(t, fresh.Context, "a project with no declared collections makes no claim about context")
	assert.Empty(t, fresh.Decisions)
	assert.Empty(t, fresh.Terms, "no workspace terminology is reachable, so none is claimed")

	h.declareCollection(t, "docs", "hash-docs")
	withContext := h.currentRef(t)
	assert.NotEmpty(t, withContext.Context)
	assert.Empty(t, withContext.Decisions, "declaring a collection is not a decision")

	h.decide(t, "u1", "approved")
	withDecisions := h.currentRef(t)
	assert.Equal(t, withContext.Context, withDecisions.Context, "a decision is not a context change")
	assert.NotEmpty(t, withDecisions.Decisions)

	h.storeBlocks(t, 3)
	withContent := h.currentRef(t)
	assert.Greater(t, withContent.Content, int64(0), "stored content advances the position")
	assert.Equal(t, withDecisions.Context, withContent.Context)
	assert.Equal(t, withDecisions.Decisions, withContent.Decisions)
}

// TestSyncRef_ContextComponentIgnoresWorkspaceOwnedCollections pins the same
// restriction the push negotiation makes: a collection the recipe never
// declared cannot make a current recipe read as diverged.
func TestSyncRef_ContextComponentIgnoresWorkspaceOwnedCollections(t *testing.T) {
	h := newRefHarness(t)
	h.declareCollection(t, "docs", "hash-docs")
	declared := h.currentRef(t).Context

	require.NoError(t, h.store.CreateCollection(t.Context(), &platstore.Collection{
		ProjectID: h.project.ID, Name: "hub-only", Kind: platstore.CollectionConnected,
		Stream: refTestStream, Owner: coresync.ContextOwnerWorkspace, ContextHash: "hash-hub",
	}))
	assert.Equal(t, declared, h.currentRef(t).Context)
}

// TestSyncRef_DeletingTheLocalCacheCostsOneRoundTrip: a client holding nothing
// asks once and knows everything a cached ref would have told it. The check is
// that the published value is a function of stored state alone — no client
// input, no prior contact.
func TestSyncRef_DeletingTheLocalCacheCostsOneRoundTrip(t *testing.T) {
	h := newRefHarness(t)
	h.declareCollection(t, "docs", "hash-docs")
	h.decide(t, "u1", "approved")
	h.storeBlocks(t, 2)

	first := h.currentRef(t)
	second := h.currentRef(t)
	assert.Equal(t, first, second, "the ref is derived, so asking twice answers twice the same")
	assert.NotEmpty(t, first.Context)
	assert.NotEmpty(t, first.Decisions)
	assert.Greater(t, first.Content, int64(0))
}

// TestSyncPull_CarriesTheRef proves the field is really on the wire: a pull is
// a client's cheapest contact with the server, and the ref rides on it so an
// ordinary loop never has to ask for one.
func TestSyncPull_CarriesTheRef(t *testing.T) {
	h := newRefHarness(t)
	h.declareCollection(t, "docs", "hash-docs")
	h.decide(t, "u1", "approved")
	h.storeBlocks(t, 2)

	c, rec := h.ctx(t, http.MethodGet, "/pull?cursor=0", "")
	require.NoError(t, h.srv.HandleSyncPull(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		HasMore bool     `json:"has_more"`
		Ref     *ref.Ref `json:"ref"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.False(t, body.HasMore)
	require.NotNil(t, body.Ref, "a final page carries the ref")
	assert.Equal(t, h.currentRef(t), *body.Ref)
}

// commit posts a push commit manifest carrying the given assertion.
func (h *refHarness) commit(t *testing.T, contexts, decisions string, expected ref.Ref) *httptest.ResponseRecorder {
	t.Helper()
	assertion, err := json.Marshal(expected)
	require.NoError(t, err)
	body := fmt.Sprintf(`{"upload_id":"u1","contexts":%s,"decisions":%s,"expected_ref":%s}`,
		orNull(contexts), orNull(decisions), assertion)
	c, rec := h.ctx(t, http.MethodPost, "/push/commit", body)
	err = h.srv.HandleSyncPushCommit(c)
	if err != nil {
		// A handler that returns rather than writes is the central error
		// handler's business; surface it so a test failure names the cause.
		require.NoError(t, err)
	}
	return rec
}

func orNull(s string) string {
	if s == "" {
		return "null"
	}
	return s
}

const oneContextEntry = `[{"name":"docs","owner":"recipe","content_hash":"hash-docs"}]`
const oneDecision = `[{"item":"docs/intro.md","unit":"u2","variant":"fr","status":"reviewed","reviewState":"approved","by":"ben","updated":"2026-08-02T00:00:00Z"}]`

// TestSyncPushCommit_StaleContextComponentIsRefused is the two-client
// compare-and-swap: both read the ref, one lands a context change, and the
// other's write — computed against the ref it read — is refused.
func TestSyncPushCommit_StaleContextComponentIsRefused(t *testing.T) {
	h := newRefHarness(t)
	h.declareCollection(t, "docs", "hash-docs")

	// Both clients observe the same ref.
	clientA := h.currentRef(t)
	clientB := clientA

	// A's context write lands (the worker's reconcile, reproduced by the store
	// write the reconcile makes).
	h.declareCollection(t, "marketing", "hash-marketing")
	require.NotEqual(t, clientA.Context, h.currentRef(t).Context)

	// B commits against what it read.
	rec := h.commit(t, oneContextEntry, "", clientB)
	require.Equal(t, http.StatusConflict, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, apierror.CodeGovernanceMoved, body["error"])
	assert.Equal(t, string(ref.ComponentContext), body["component"])
	assert.Contains(t, body["message"], "Pull first")
}

// TestSyncPushCommit_StaleDecisionsComponentIsRefused is the same property for
// the ledger: an approval that landed on the server while this client was
// deciding must not be overwritten by a record that never saw it.
func TestSyncPushCommit_StaleDecisionsComponentIsRefused(t *testing.T) {
	h := newRefHarness(t)
	h.decide(t, "u1", "approved")

	clientA := h.currentRef(t)
	clientB := clientA

	h.decide(t, "u9", "approved") // another client's approval
	require.NotEqual(t, clientA.Decisions, h.currentRef(t).Decisions)

	rec := h.commit(t, "", oneDecision, clientB)
	require.Equal(t, http.StatusConflict, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, string(ref.ComponentDecisions), body["component"])
}

// TestSyncPushCommit_CurrentAssertionsPass: the same writes with the ref the
// server actually stands at go through, so the guard refuses staleness and not
// governance writes as such.
func TestSyncPushCommit_CurrentAssertionsPass(t *testing.T) {
	h := newRefHarness(t)
	h.declareCollection(t, "docs", "hash-docs")
	h.decide(t, "u1", "approved")

	rec := h.commit(t, oneContextEntry, oneDecision, h.currentRef(t))
	assert.NotEqual(t, http.StatusConflict, rec.Code)
}

// TestSyncPushCommit_UnrelatedBlockTrafficNeverConflicts is the composite ref's
// central promise. A push carrying only blocks writes no governance, so it is
// accepted however far the governance has moved beneath it — and it moves no
// governance component of its own.
func TestSyncPushCommit_UnrelatedBlockTrafficNeverConflicts(t *testing.T) {
	h := newRefHarness(t)
	h.declareCollection(t, "docs", "hash-docs")
	h.decide(t, "u1", "approved")

	stale := h.currentRef(t)

	// Governance moves twice over, in both components.
	h.declareCollection(t, "marketing", "hash-marketing")
	h.decide(t, "u9", "approved")
	moved := h.currentRef(t)
	require.NotEqual(t, stale.Context, moved.Context)
	require.NotEqual(t, stale.Decisions, moved.Decisions)

	// A content-only commit asserting the stale ref is accepted regardless.
	rec := h.commit(t, "", "", stale)
	assert.NotEqual(t, http.StatusConflict, rec.Code,
		"a push that writes no governance must never be refused because governance moved")

	// And storing blocks moves the position without disturbing either identity.
	h.storeBlocks(t, 4)
	after := h.currentRef(t)
	assert.Greater(t, after.Content, moved.Content)
	assert.Equal(t, moved.Context, after.Context)
	assert.Equal(t, moved.Decisions, after.Decisions)
}

// TestSyncPushCommit_AnUnassertingClientIsAccepted is the backward-tolerance
// contract: a client that predates the ref sends no assertion and writes
// exactly as it always did.
func TestSyncPushCommit_AnUnassertingClientIsAccepted(t *testing.T) {
	h := newRefHarness(t)
	h.declareCollection(t, "docs", "hash-docs")
	h.declareCollection(t, "marketing", "hash-marketing")

	rec := h.commit(t, oneContextEntry, oneDecision, ref.Ref{})
	assert.NotEqual(t, http.StatusConflict, rec.Code)
}
