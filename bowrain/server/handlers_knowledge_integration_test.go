//go:build integration

package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	brandpg "github.com/neokapi/neokapi/bowrain/brand"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/termbase"
)

// These tests drive the brand knowledge-graph handlers against the real
// PostgreSQL-backed governance store, content store, and brand store, exercising
// the change-set lifecycle end to end. They are gated behind //go:build
// integration and skip automatically when Docker is unavailable (pgtest).

// kgIntegrationServer wires a server with the real Postgres knowledge, content,
// and brand stores on a single throwaway schema, plus an in-memory workspace
// termbase (the merge engine's writable concept store), so a change-set merge
// applies to the same termbase the concept reads resolve.
func kgIntegrationServer(t *testing.T) *Server {
	t.Helper()
	db := pgtest.NewTestDB(t)

	srv := NewServer(DefaultConfig())

	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	srv.ContentStore = cs

	ks, err := knowledge.NewPostgresKnowledgeStore(db)
	require.NoError(t, err)
	srv.KnowledgeStore = ks

	bs, err := brandpg.NewPostgresBrandStore(db)
	require.NoError(t, err)
	srv.BrandStore = bs

	srv.wsStores.tbFactory = func() termbase.TBStore {
		return &testTermStore{termbase.NewInMemoryTermBase()}
	}
	return srv
}

// kgIntegrationCtx builds an echo context for a knowledge route with full
// permissions and the given actor, workspace, and path parameters.
func kgIntegrationCtx(srv *Server, method, target, body, actor string, params ...string) (echo.Context, *httptest.ResponseRecorder) {
	const wsID = "ws-kg-int"
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, r)
	if body != "" {
		req.Header.Set("Content-Type", echo.MIMEApplicationJSON)
	}
	rec := httptest.NewRecorder()
	c := srv.GetEcho().NewContext(req, rec)
	c.Set("project_permissions", platauth.PermAll)
	c.Set("workspace_id", wsID)
	c.Set("user_id", actor)

	names, values := []string{"ws"}, []string{wsID}
	for i := 0; i+1 < len(params); i += 2 {
		names = append(names, params[i])
		values = append(values, params[i+1])
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	return c, rec
}

// TestChangesetMergeFlowIntegration exercises the ordinary-change-set happy path
// through the real handlers and Postgres store: create a draft, append an
// ordinary concept.create op, merge it directly (no review required), and read
// the resulting concept back from the workspace graph.
func TestChangesetMergeFlowIntegration(t *testing.T) {
	srv := kgIntegrationServer(t)
	const author = "alice"

	// 1. Create a draft change-set.
	c, rec := kgIntegrationCtx(srv, http.MethodPost, "/", `{"name":"rename login"}`, author)
	require.NoError(t, srv.HandleCreateChangeSet(c))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var cs knowledge.ChangeSet
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cs))
	require.NotEmpty(t, cs.ID)
	require.Equal(t, knowledge.ChangeSetDraft, cs.Status)

	// 2. Append an ordinary op: create a concept with an admitted term.
	opBody := `{"op":"concept.create","payload":{"concept":{"id":"c-int","domain":"auth","terms":[{"text":"Sign in","locale":"en","status":"admitted"}]}}}`
	c, rec = kgIntegrationCtx(srv, http.MethodPost, "/", opBody, author, "id", cs.ID)
	require.NoError(t, srv.HandleAddChangeSetOp(c))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// 3. Merge directly from draft (ordinary change-set, author may merge).
	c, rec = kgIntegrationCtx(srv, http.MethodPost, "/", "", author, "id", cs.ID)
	require.NoError(t, srv.HandleMergeChangeSet(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	merged, err := srv.KnowledgeStore.GetChangeSet(c.Request().Context(), "ws-kg-int", cs.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.ChangeSetMerged, merged.Status)

	// 4. The concept is now readable through the concept API.
	c, rec = kgIntegrationCtx(srv, http.MethodGet, "/", "", author, "cid", "c-int")
	require.NoError(t, srv.HandleGetConcept(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got ConceptInfoResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "c-int", got.ID)
	require.Len(t, got.Terms, 1)
	assert.Equal(t, "Sign in", got.Terms[0].Text)
}

// TestChangesetSubmitApproveIntegration exercises the governed-review lifecycle
// against the real store: an empty change-set cannot be submitted, a populated one
// moves draft → in_review on submit, and a reviewer other than the author can
// approve it (separation of duties is satisfied), reaching the approved state.
func TestChangesetSubmitApproveIntegration(t *testing.T) {
	srv := kgIntegrationServer(t)
	const author = "alice"
	const reviewer = "bob"

	// Create a draft.
	c, rec := kgIntegrationCtx(srv, http.MethodPost, "/", `{"name":"ban competitor term"}`, author)
	require.NoError(t, srv.HandleCreateChangeSet(c))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var cs knowledge.ChangeSet
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cs))

	// An empty change-set cannot be submitted.
	c, rec = kgIntegrationCtx(srv, http.MethodPost, "/", "", author, "id", cs.ID)
	require.NoError(t, srv.HandleSubmitChangeSet(c))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	// Add a governed op (a forbidden term-status transition).
	opBody := `{"op":"term.status","payload":{"concept_id":"c-x","locale":"en","text":"synergy","from":"admitted","to":"forbidden"}}`
	c, rec = kgIntegrationCtx(srv, http.MethodPost, "/", opBody, author, "id", cs.ID)
	require.NoError(t, srv.HandleAddChangeSetOp(c))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// Submit moves it to in_review.
	c, rec = kgIntegrationCtx(srv, http.MethodPost, "/", "", author, "id", cs.ID)
	require.NoError(t, srv.HandleSubmitChangeSet(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// The author cannot approve their own change-set.
	c, rec = kgIntegrationCtx(srv, http.MethodPost, "/", "", author, "id", cs.ID)
	require.NoError(t, srv.HandleApproveChangeSet(c))
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())

	// A different reviewer can.
	c, rec = kgIntegrationCtx(srv, http.MethodPost, "/", `{"comment":"agreed"}`, reviewer, "id", cs.ID)
	require.NoError(t, srv.HandleApproveChangeSet(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	approved, err := srv.KnowledgeStore.GetChangeSet(c.Request().Context(), "ws-kg-int", cs.ID)
	require.NoError(t, err)
	assert.Equal(t, knowledge.ChangeSetApproved, approved.Status)
	assert.NotNil(t, approved.SubmittedAt, "submitted_at recorded")
}

// TestMarketAndObservationIntegration round-trips a market and an observation
// through the real Postgres knowledge store via the handlers.
func TestMarketAndObservationIntegration(t *testing.T) {
	srv := kgIntegrationServer(t)
	const author = "alice"

	c, rec := kgIntegrationCtx(srv, http.MethodPost, "/", `{"name":"dach","locales":["de-DE","de-AT"]}`, author)
	require.NoError(t, srv.HandleCreateMarket(c))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	c, rec = kgIntegrationCtx(srv, http.MethodGet, "/", "", author)
	require.NoError(t, srv.HandleListMarkets(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var markets []*knowledge.Market
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &markets))
	require.Len(t, markets, 1)
	assert.Equal(t, "dach", markets[0].Name)

	obsBody := `{"kind":"competitor","quote":"they say 'log in'","source":"acme.com"}`
	c, rec = kgIntegrationCtx(srv, http.MethodPost, "/", obsBody, author, "cid", "c-int")
	require.NoError(t, srv.HandleAddObservation(c))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	c, rec = kgIntegrationCtx(srv, http.MethodGet, "/", "", author, "cid", "c-int")
	require.NoError(t, srv.HandleListObservations(c))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var obs []*knowledge.Observation
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &obs))
	require.Len(t, obs, 1)
	assert.Equal(t, knowledge.ObservationCompetitor, obs[0].Kind)
}
