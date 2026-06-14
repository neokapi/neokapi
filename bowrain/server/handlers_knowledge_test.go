package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
)

// These tests exercise the brand knowledge-graph REST handlers (AD-021) in-session
// — without a PostgreSQL container — by driving the echo handlers directly with a
// fake knowledge.Store and an in-memory workspace termbase. They cover the parts
// of the surface that are decidable at the handler layer: route registration,
// permission gating, DTO bind/validate, governed-edit 409s on the direct path,
// the separation-of-duties self-review refusal, and graceful 503s when the graph
// is unconfigured. Live-Postgres handler flows (op append → merge → concept read)
// live behind //go:build integration in handlers_knowledge_integration_test.go.

const kgTestWS = "ws-kg"

// kgHarness wires a server with an in-memory termbase and a fake knowledge store,
// so the concept and change-set handlers run with no database.
type kgHarness struct {
	srv  *Server
	fake *fakeKnowledgeStore
}

func newKGHarness(t *testing.T) *kgHarness {
	t.Helper()
	srv := NewServer(DefaultConfig())
	srv.wsStores.tbFactory = func() termbase.TBStore {
		return &testTermStore{termbase.NewInMemoryTermBase()}
	}
	fake := newFakeKnowledgeStore()
	srv.KnowledgeStore = fake
	return &kgHarness{srv: srv, fake: fake}
}

// req builds an echo context for a knowledge route with the given permissions and
// the standard workspace context. params are name/value pairs for the route's path
// parameters (":ws", ":cid", ":id", …). The "ws" path param is always set to
// kgTestWS so it keys the same in-memory termbase the test seeds.
func (h *kgHarness) req(method, target, body string, perms platauth.Permission, params ...string) (echo.Context, *httptest.ResponseRecorder) {
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
	c.Set("project_permissions", perms)
	c.Set("workspace_id", kgTestWS)

	names := []string{"ws"}
	values := []string{kgTestWS}
	for i := 0; i+1 < len(params); i += 2 {
		names = append(names, params[i])
		values = append(values, params[i+1])
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	return c, rec
}

// tb returns the in-memory workspace termbase the handlers resolve for kgTestWS,
// so a test can seed concepts and relations the handlers will read.
func (h *kgHarness) tb(t *testing.T) termbase.TBStore {
	t.Helper()
	tb, err := h.srv.wsStores.getTB(kgTestWS)
	require.NoError(t, err)
	return tb
}

// withActor sets the authenticated user id on a context (the change-set author /
// reviewer identity the SoD gate compares).
func withActor(c echo.Context, userID string) echo.Context {
	c.Set("user_id", userID)
	return c
}

// ---------------------------------------------------------------------------
// Route registration
// ---------------------------------------------------------------------------

// TestKnowledgeRoutesRegistered proves both halves of the knowledge-graph API
// register without panic, that every documented method+path is wired, and that
// the former /:ws/terms routes are gone (the concept API replaces them).
func TestKnowledgeRoutesRegistered(t *testing.T) {
	srv := NewServer(DefaultConfig())
	e := echo.New()
	g := e.Group("/:ws")
	require.NotPanics(t, func() {
		srv.registerConceptRoutes(g)
		srv.registerChangesetRoutes(g)
	})

	got := map[string]bool{}
	for _, r := range e.Routes() {
		got[r.Method+" "+r.Path] = true
	}

	want := []string{
		"GET /:ws/concepts",
		"POST /:ws/concepts",
		"GET /:ws/concepts/:cid",
		"PUT /:ws/concepts/:cid",
		"DELETE /:ws/concepts/:cid",
		"GET /:ws/concepts/:cid/story",
		"GET /:ws/concepts/:cid/relations",
		"POST /:ws/concepts/:cid/relations",
		"DELETE /:ws/concepts/:cid/relations/:rid",
		"GET /:ws/concepts/:cid/blast-radius",
		"GET /:ws/concepts/:cid/observations",
		"POST /:ws/concepts/:cid/observations",
		"DELETE /:ws/concepts/:cid/observations/:oid",
		"GET /:ws/concepts/:cid/comments",
		"POST /:ws/concepts/:cid/comments",
		"POST /:ws/concepts/:cid/comments/:id/resolve",
		"DELETE /:ws/concepts/:cid/comments/:id",
		"GET /:ws/markets",
		"POST /:ws/markets",
		"PUT /:ws/markets/:mid",
		"DELETE /:ws/markets/:mid",
		"GET /:ws/changesets",
		"POST /:ws/changesets",
		"GET /:ws/changesets/:id",
		"PATCH /:ws/changesets/:id",
		"POST /:ws/changesets/:id/ops",
		"DELETE /:ws/changesets/:id/ops/:seq",
		"POST /:ws/changesets/:id/submit",
		"POST /:ws/changesets/:id/approve",
		"POST /:ws/changesets/:id/reject",
		"POST /:ws/changesets/:id/merge",
		"POST /:ws/changesets/:id/abandon",
		"GET /:ws/changesets/:id/blast-radius",
		"POST /:ws/changesets/:id/pilots",
		"DELETE /:ws/changesets/:id/pilots/:project/:stream",
	}
	for _, w := range want {
		assert.True(t, got[w], "route not registered: %s", w)
	}

	// Design-from-the-start: the concept API replaces the old /:ws/terms routes.
	for _, r := range e.Routes() {
		assert.False(t, strings.HasPrefix(r.Path, "/:ws/terms"),
			"legacy /:ws/terms route should have been removed, found %s %s", r.Method, r.Path)
	}
}

// ---------------------------------------------------------------------------
// Permission gating
// ---------------------------------------------------------------------------

// TestConceptPermissionGating proves the concept and change-set handlers fail
// closed: a context lacking the required permission is rejected with 403, and the
// approve gate requires manage_brand specifically (manage_terms is insufficient).
func TestConceptPermissionGating(t *testing.T) {
	h := newKGHarness(t)

	t.Run("list concepts needs view_content", func(t *testing.T) {
		c, rec := h.req(http.MethodGet, "/", "", 0)
		err := h.srv.HandleListConcepts(c)
		require.Error(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("create concept needs manage_terms", func(t *testing.T) {
		c, rec := h.req(http.MethodPost, "/", `{"domain":"d"}`, platauth.PermViewContent)
		err := h.srv.HandleCreateConcept(c)
		require.Error(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("create change-set needs manage_terms", func(t *testing.T) {
		c, rec := h.req(http.MethodPost, "/", `{"name":"x"}`, platauth.PermViewContent)
		err := h.srv.HandleCreateChangeSet(c)
		require.Error(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("approve change-set needs manage_brand not manage_terms", func(t *testing.T) {
		c, rec := h.req(http.MethodPost, "/", "", platauth.PermViewContent|platauth.PermManageTerms, "id", "anything")
		err := h.srv.HandleApproveChangeSet(c)
		require.Error(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("create market needs manage_terms", func(t *testing.T) {
		c, rec := h.req(http.MethodPost, "/", `{"name":"dach"}`, platauth.PermViewContent)
		err := h.srv.HandleCreateMarket(c)
		require.Error(t, err)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})
}

// ---------------------------------------------------------------------------
// Governed-edit 409s on the direct path
// ---------------------------------------------------------------------------

// TestConceptGovernedConflicts proves the direct (non-change-set) path refuses
// governed transitions with 409 and a change-set hint: deleting a concept,
// creating a forbidden/preferred term, transitioning a term to forbidden, and
// adding a REPLACED_BY relation.
func TestConceptGovernedConflicts(t *testing.T) {
	h := newKGHarness(t)
	ctx := context.Background()

	assertGoverned := func(t *testing.T, rec *httptest.ResponseRecorder) {
		t.Helper()
		assert.Equal(t, http.StatusConflict, rec.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Contains(t, body, "hint")
		assert.Contains(t, body["hint"], "change-set")
	}

	t.Run("delete concept is governed", func(t *testing.T) {
		c, rec := h.req(http.MethodDelete, "/", "", platauth.PermManageTerms, "cid", "c1")
		require.NoError(t, h.srv.HandleDeleteConcept(c))
		assertGoverned(t, rec)
	})

	t.Run("creating a forbidden term is governed", func(t *testing.T) {
		body := `{"domain":"d","terms":[{"text":"oldname","locale":"en","status":"forbidden"}]}`
		c, rec := h.req(http.MethodPost, "/", body, platauth.PermManageTerms)
		require.NoError(t, h.srv.HandleCreateConcept(c))
		assertGoverned(t, rec)
	})

	t.Run("creating a preferred term is governed", func(t *testing.T) {
		body := `{"domain":"d","terms":[{"text":"newname","locale":"en","status":"preferred"}]}`
		c, rec := h.req(http.MethodPost, "/", body, platauth.PermManageTerms)
		require.NoError(t, h.srv.HandleCreateConcept(c))
		assertGoverned(t, rec)
	})

	t.Run("transitioning a term to forbidden is governed", func(t *testing.T) {
		// Seed a concept whose term is an acceptable alternative (ordinary).
		seed := termbase.Concept{
			ID:     "c-gov",
			Domain: "d",
			Terms: []termbase.Term{
				{Text: "utilize", Locale: "en", Status: model.TermAdmitted},
			},
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		require.NoError(t, h.tb(t).AddConcept(ctx, seed))

		body := `{"domain":"d","terms":[{"text":"utilize","locale":"en","status":"forbidden"}]}`
		c, rec := h.req(http.MethodPut, "/", body, platauth.PermManageTerms, "cid", "c-gov")
		require.NoError(t, h.srv.HandleUpdateConcept(c))
		assertGoverned(t, rec)
	})

	t.Run("ordinary update lands directly", func(t *testing.T) {
		seed := termbase.Concept{
			ID:     "c-ord",
			Domain: "d",
			Terms: []termbase.Term{
				{Text: "dashboard", Locale: "en", Status: model.TermAdmitted},
			},
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		require.NoError(t, h.tb(t).AddConcept(ctx, seed))

		body := `{"domain":"ui","definition":"the home view","terms":[{"text":"dashboard","locale":"en","status":"admitted"}]}`
		c, rec := h.req(http.MethodPut, "/", body, platauth.PermManageTerms, "cid", "c-ord")
		require.NoError(t, h.srv.HandleUpdateConcept(c))
		assert.Equal(t, http.StatusNoContent, rec.Code)

		got, ok, err := h.tb(t).GetConcept(ctx, "c-ord")
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "ui", got.Domain)
		assert.Equal(t, "the home view", got.Definition)
	})

	t.Run("REPLACED_BY relation is governed", func(t *testing.T) {
		body := `{"target_id":"c2","relation_type":"REPLACED_BY"}`
		c, rec := h.req(http.MethodPost, "/", body, platauth.PermManageTerms, "cid", "c1")
		require.NoError(t, h.srv.HandleAddConceptRelation(c))
		assertGoverned(t, rec)
	})

	t.Run("ordinary relation lands directly", func(t *testing.T) {
		// A relation needs both endpoint concepts to exist.
		for _, cid := range []string{"c1", "c2"} {
			require.NoError(t, h.tb(t).AddConcept(ctx, termbase.Concept{
				ID: cid, Domain: "d",
				Terms:     []termbase.Term{{Text: cid, Locale: "en", Status: model.TermAdmitted}},
				CreatedAt: time.Now(), UpdatedAt: time.Now(),
			}))
		}
		body := `{"target_id":"c2","relation_type":"` + graph.LabelRelated + `"}`
		c, rec := h.req(http.MethodPost, "/", body, platauth.PermManageTerms, "cid", "c1")
		require.NoError(t, h.srv.HandleAddConceptRelation(c))
		assert.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	})
}

// ---------------------------------------------------------------------------
// DTO bind / validate
// ---------------------------------------------------------------------------

// TestMarketDTOValidation proves the market handlers validate their request body
// and persist a valid market.
func TestMarketDTOValidation(t *testing.T) {
	h := newKGHarness(t)

	t.Run("empty name rejected", func(t *testing.T) {
		c, rec := h.req(http.MethodPost, "/", `{"name":"   "}`, platauth.PermManageTerms)
		require.NoError(t, h.srv.HandleCreateMarket(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("valid market created", func(t *testing.T) {
		body := `{"name":"dach","description":"DE markets","locales":["de-DE","de-AT","de-CH"]}`
		c, rec := h.req(http.MethodPost, "/", body, platauth.PermManageTerms)
		require.NoError(t, h.srv.HandleCreateMarket(c))
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		var m knowledge.Market
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &m))
		assert.Equal(t, "dach", m.Name)
		assert.Equal(t, kgTestWS, m.WorkspaceID)
		assert.Len(t, m.Locales, 3)
	})
}

// TestObservationDTOValidation proves the observation handler validates the
// observation kind and requires a quote, and persists a valid observation.
func TestObservationDTOValidation(t *testing.T) {
	h := newKGHarness(t)

	t.Run("unknown kind rejected", func(t *testing.T) {
		c, rec := h.req(http.MethodPost, "/", `{"kind":"bogus","quote":"hi"}`, platauth.PermManageTerms, "cid", "c1")
		require.NoError(t, h.srv.HandleAddObservation(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("missing quote rejected", func(t *testing.T) {
		c, rec := h.req(http.MethodPost, "/", `{"kind":"competitor","quote":"   "}`, platauth.PermManageTerms, "cid", "c1")
		require.NoError(t, h.srv.HandleAddObservation(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("valid observation created", func(t *testing.T) {
		body := `{"kind":"competitor","quote":"they say 'sign in'","source":"acme.com"}`
		c, rec := h.req(http.MethodPost, "/", body, platauth.PermManageTerms, "cid", "c1")
		require.NoError(t, h.srv.HandleAddObservation(c))
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		var o knowledge.Observation
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &o))
		assert.Equal(t, knowledge.ObservationCompetitor, o.Kind)
		assert.Equal(t, "c1", o.ConceptID)
	})
}

// TestConceptCommentValidation proves the comment handler requires a body and
// persists a valid comment.
func TestConceptCommentValidation(t *testing.T) {
	h := newKGHarness(t)

	t.Run("empty body rejected", func(t *testing.T) {
		c, rec := h.req(http.MethodPost, "/", `{"body":"  "}`, platauth.PermManageTerms, "cid", "c1")
		require.NoError(t, h.srv.HandleAddConceptComment(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("valid comment created", func(t *testing.T) {
		c, rec := h.req(http.MethodPost, "/", `{"body":"should we ban this?"}`, platauth.PermManageTerms, "cid", "c1")
		withActor(c, "alice")
		require.NoError(t, h.srv.HandleAddConceptComment(c))
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		var cm knowledge.Comment
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cm))
		assert.Equal(t, "should we ban this?", cm.Body)
		assert.Equal(t, "alice", cm.Author)
	})
}

// TestChangesetDTOValidation proves the change-set collection handlers validate
// their inputs: an empty name is rejected, a valid create returns the draft, and
// an unknown status filter is rejected.
func TestChangesetDTOValidation(t *testing.T) {
	h := newKGHarness(t)

	t.Run("empty name rejected", func(t *testing.T) {
		c, rec := h.req(http.MethodPost, "/", `{"name":""}`, platauth.PermManageTerms)
		require.NoError(t, h.srv.HandleCreateChangeSet(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("valid create returns a draft", func(t *testing.T) {
		c, rec := h.req(http.MethodPost, "/", `{"name":"ban competitor terms"}`, platauth.PermManageTerms)
		withActor(c, "alice")
		require.NoError(t, h.srv.HandleCreateChangeSet(c))
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		var cs knowledge.ChangeSet
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cs))
		assert.Equal(t, knowledge.ChangeSetDraft, cs.Status)
		assert.Equal(t, "alice", cs.CreatedBy)
	})

	t.Run("unknown status filter rejected", func(t *testing.T) {
		c, rec := h.req(http.MethodGet, "/?status=bogus", "", platauth.PermViewContent)
		require.NoError(t, h.srv.HandleListChangeSets(c))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// ---------------------------------------------------------------------------
// Separation of duties
// ---------------------------------------------------------------------------

// TestChangesetSeparationOfDuties proves the approve/reject handlers enforce
// separation of duties at the handler layer: a change-set's author cannot review
// their own change-set, a different reviewer can, the wrong lifecycle state is
// rejected, and a missing change-set is a 404.
func TestChangesetSeparationOfDuties(t *testing.T) {
	h := newKGHarness(t)
	ctx := context.Background()

	const author = "alice"
	const reviewer = "bob"
	const manage = platauth.PermViewContent | platauth.PermManageTerms | platauth.PermManageBrand

	seedInReview := func(id string) {
		require.NoError(t, h.fake.CreateChangeSet(ctx, &knowledge.ChangeSet{
			ID: id, WorkspaceID: kgTestWS, Name: id, Status: knowledge.ChangeSetInReview, CreatedBy: author,
		}))
	}

	t.Run("author cannot approve own change-set", func(t *testing.T) {
		seedInReview("cs-self-approve")
		c, rec := h.req(http.MethodPost, "/", "", manage, "id", "cs-self-approve")
		withActor(c, author)
		require.NoError(t, h.srv.HandleApproveChangeSet(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Contains(t, rec.Body.String(), "separation of duties")
	})

	t.Run("author cannot reject own change-set", func(t *testing.T) {
		seedInReview("cs-self-reject")
		c, rec := h.req(http.MethodPost, "/", "", manage, "id", "cs-self-reject")
		withActor(c, author)
		require.NoError(t, h.srv.HandleRejectChangeSet(c))
		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Contains(t, rec.Body.String(), "separation of duties")
	})

	t.Run("a different reviewer can approve", func(t *testing.T) {
		seedInReview("cs-other-approve")
		c, rec := h.req(http.MethodPost, "/", `{"comment":"looks good"}`, manage, "id", "cs-other-approve")
		withActor(c, reviewer)
		require.NoError(t, h.srv.HandleApproveChangeSet(c))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		updated, err := h.fake.GetChangeSet(ctx, kgTestWS, "cs-other-approve")
		require.NoError(t, err)
		assert.Equal(t, knowledge.ChangeSetApproved, updated.Status)

		reviews, err := h.fake.ListReviews(ctx, kgTestWS, "cs-other-approve")
		require.NoError(t, err)
		require.Len(t, reviews, 1)
		assert.Equal(t, knowledge.VerdictApprove, reviews[0].Verdict)
		assert.Equal(t, reviewer, reviews[0].Reviewer)
	})

	t.Run("approving a draft (not in review) is a conflict", func(t *testing.T) {
		require.NoError(t, h.fake.CreateChangeSet(ctx, &knowledge.ChangeSet{
			ID: "cs-draft", WorkspaceID: kgTestWS, Name: "d", Status: knowledge.ChangeSetDraft, CreatedBy: author,
		}))
		c, rec := h.req(http.MethodPost, "/", "", manage, "id", "cs-draft")
		withActor(c, reviewer)
		require.NoError(t, h.srv.HandleApproveChangeSet(c))
		assert.Equal(t, http.StatusConflict, rec.Code)
	})

	t.Run("missing change-set is 404", func(t *testing.T) {
		c, rec := h.req(http.MethodPost, "/", "", manage, "id", "does-not-exist")
		withActor(c, reviewer)
		require.NoError(t, h.srv.HandleApproveChangeSet(c))
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

// ---------------------------------------------------------------------------
// Unconfigured graph
// ---------------------------------------------------------------------------

// TestKnowledgeStoreUnavailable proves the store-backed handlers degrade to 503
// when the knowledge graph is not configured (no PostgreSQL), rather than panicking.
func TestKnowledgeStoreUnavailable(t *testing.T) {
	srv := NewServer(DefaultConfig())
	srv.KnowledgeStore = nil // not configured
	e := srv.GetEcho()

	call := func(h echo.HandlerFunc, params ...string) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("project_permissions", platauth.PermAll)
		c.Set("workspace_id", kgTestWS)
		names, values := []string{"ws"}, []string{kgTestWS}
		for i := 0; i+1 < len(params); i += 2 {
			names = append(names, params[i])
			values = append(values, params[i+1])
		}
		c.SetParamNames(names...)
		c.SetParamValues(values...)
		require.NoError(t, h(c))
		return rec.Code
	}

	assert.Equal(t, http.StatusServiceUnavailable, call(srv.HandleListMarkets))
	assert.Equal(t, http.StatusServiceUnavailable, call(srv.HandleListChangeSets))
	assert.Equal(t, http.StatusServiceUnavailable, call(srv.HandleGetConceptStory, "cid", "c1"))
	assert.Equal(t, http.StatusServiceUnavailable, call(srv.HandleListObservations, "cid", "c1"))
}

// ===========================================================================
// fakeKnowledgeStore — an in-memory knowledge.Store for the in-session handler
// tests. It backs change-sets, reviews, markets, observations, and comments in
// maps and validates change-set status transitions with the real pure policy
// (knowledge.ValidateStatusTransition), so the SoD/lifecycle handler logic runs
// against faithful store behavior. Methods unused by these tests return empty
// results. A separate //go:build integration test exercises the real
// PostgresKnowledgeStore end to end.
// ===========================================================================

type fakeKnowledgeStore struct {
	changesets   map[string]*knowledge.ChangeSet
	reviews      map[string][]*knowledge.ChangeSetReview
	markets      map[string]*knowledge.Market
	observations map[string]*knowledge.Observation
	comments     map[string]*knowledge.Comment
	seq          int
}

func newFakeKnowledgeStore() *fakeKnowledgeStore {
	return &fakeKnowledgeStore{
		changesets:   map[string]*knowledge.ChangeSet{},
		reviews:      map[string][]*knowledge.ChangeSetReview{},
		markets:      map[string]*knowledge.Market{},
		observations: map[string]*knowledge.Observation{},
		comments:     map[string]*knowledge.Comment{},
	}
}

func fakeKey(ws, id string) string { return ws + "|" + id }

func (f *fakeKnowledgeStore) nextID(prefix string) string {
	f.seq++
	return prefix + "-" + strconv.Itoa(f.seq)
}

// --- Markets ---------------------------------------------------------------

func (f *fakeKnowledgeStore) CreateMarket(_ context.Context, m *knowledge.Market) error {
	if m.ID == "" {
		m.ID = f.nextID("mkt")
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	m.UpdatedAt = m.CreatedAt
	cp := *m
	f.markets[fakeKey(m.WorkspaceID, m.ID)] = &cp
	return nil
}

func (f *fakeKnowledgeStore) GetMarket(_ context.Context, ws, id string) (*knowledge.Market, error) {
	m, ok := f.markets[fakeKey(ws, id)]
	if !ok {
		return nil, fmt.Errorf("market %s not found", id)
	}
	cp := *m
	return &cp, nil
}

func (f *fakeKnowledgeStore) UpdateMarket(_ context.Context, m *knowledge.Market) error {
	if _, ok := f.markets[fakeKey(m.WorkspaceID, m.ID)]; !ok {
		return fmt.Errorf("market %s not found", m.ID)
	}
	m.UpdatedAt = time.Now().UTC()
	cp := *m
	f.markets[fakeKey(m.WorkspaceID, m.ID)] = &cp
	return nil
}

func (f *fakeKnowledgeStore) DeleteMarket(_ context.Context, ws, id string) error {
	key := fakeKey(ws, id)
	if _, ok := f.markets[key]; !ok {
		return fmt.Errorf("market %s not found", id)
	}
	delete(f.markets, key)
	return nil
}

func (f *fakeKnowledgeStore) ListMarkets(_ context.Context, ws string) ([]*knowledge.Market, error) {
	var out []*knowledge.Market
	for _, m := range f.markets {
		if m.WorkspaceID == ws {
			cp := *m
			out = append(out, &cp)
		}
	}
	return out, nil
}

// --- Observations ----------------------------------------------------------

func (f *fakeKnowledgeStore) AddObservation(_ context.Context, o *knowledge.Observation) error {
	if o.ID == "" {
		o.ID = f.nextID("obs")
	}
	if o.CreatedAt.IsZero() {
		o.CreatedAt = time.Now().UTC()
	}
	cp := *o
	f.observations[fakeKey(o.WorkspaceID, o.ID)] = &cp
	return nil
}

func (f *fakeKnowledgeStore) DeleteObservation(_ context.Context, ws, id string) error {
	key := fakeKey(ws, id)
	if _, ok := f.observations[key]; !ok {
		return fmt.Errorf("observation %s not found", id)
	}
	delete(f.observations, key)
	return nil
}

func (f *fakeKnowledgeStore) ListObservationsByConcept(_ context.Context, ws, conceptID string) ([]*knowledge.Observation, error) {
	var out []*knowledge.Observation
	for _, o := range f.observations {
		if o.WorkspaceID == ws && o.ConceptID == conceptID {
			cp := *o
			out = append(out, &cp)
		}
	}
	return out, nil
}

// --- Comments --------------------------------------------------------------

func (f *fakeKnowledgeStore) AddComment(_ context.Context, c *knowledge.Comment) error {
	if c.ID == "" {
		c.ID = f.nextID("cmt")
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	cp := *c
	f.comments[fakeKey(c.WorkspaceID, c.ID)] = &cp
	return nil
}

func (f *fakeKnowledgeStore) DeleteComment(_ context.Context, ws, id string) error {
	key := fakeKey(ws, id)
	if _, ok := f.comments[key]; !ok {
		return fmt.Errorf("comment %s not found", id)
	}
	delete(f.comments, key)
	return nil
}

func (f *fakeKnowledgeStore) ResolveComment(_ context.Context, ws, id string, resolved bool) error {
	c, ok := f.comments[fakeKey(ws, id)]
	if !ok {
		return fmt.Errorf("comment %s not found", id)
	}
	c.Resolved = resolved
	return nil
}

func (f *fakeKnowledgeStore) ListCommentsByConcept(_ context.Context, ws, conceptID string) ([]*knowledge.Comment, error) {
	var out []*knowledge.Comment
	for _, c := range f.comments {
		if c.WorkspaceID == ws && c.ConceptID == conceptID {
			cp := *c
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeKnowledgeStore) ListCommentsByChangeset(_ context.Context, ws, changesetID string) ([]*knowledge.Comment, error) {
	var out []*knowledge.Comment
	for _, c := range f.comments {
		if c.WorkspaceID == ws && c.ChangesetID == changesetID {
			cp := *c
			out = append(out, &cp)
		}
	}
	return out, nil
}

// --- Concept revisions -----------------------------------------------------

func (f *fakeKnowledgeStore) AddRevision(context.Context, *knowledge.ConceptRevision) error {
	return nil
}

func (f *fakeKnowledgeStore) ListRevisions(context.Context, string, string) ([]*knowledge.ConceptRevision, error) {
	return nil, nil
}

func (f *fakeKnowledgeStore) LatestRev(context.Context, string, string) (int64, error) { return 0, nil }

// --- Change-sets -----------------------------------------------------------

func (f *fakeKnowledgeStore) CreateChangeSet(_ context.Context, cs *knowledge.ChangeSet) error {
	if cs.ID == "" {
		cs.ID = f.nextID("cs")
	}
	if cs.Status == "" {
		cs.Status = knowledge.ChangeSetDraft
	}
	if cs.CreatedAt.IsZero() {
		cs.CreatedAt = time.Now().UTC()
	}
	cs.UpdatedAt = cs.CreatedAt
	cp := *cs
	f.changesets[fakeKey(cs.WorkspaceID, cs.ID)] = &cp
	return nil
}

func (f *fakeKnowledgeStore) GetChangeSet(_ context.Context, ws, id string) (*knowledge.ChangeSet, error) {
	cs, ok := f.changesets[fakeKey(ws, id)]
	if !ok {
		return nil, fmt.Errorf("change-set %s not found", id)
	}
	cp := *cs
	return &cp, nil
}

func (f *fakeKnowledgeStore) ListChangeSets(_ context.Context, ws string, status knowledge.ChangeSetStatus) ([]*knowledge.ChangeSet, error) {
	var out []*knowledge.ChangeSet
	for _, cs := range f.changesets {
		if cs.WorkspaceID != ws {
			continue
		}
		if status != "" && cs.Status != status {
			continue
		}
		cp := *cs
		out = append(out, &cp)
	}
	return out, nil
}

func (f *fakeKnowledgeStore) UpdateChangeSet(_ context.Context, cs *knowledge.ChangeSet) error {
	existing, ok := f.changesets[fakeKey(cs.WorkspaceID, cs.ID)]
	if !ok {
		return fmt.Errorf("change-set %s not found", cs.ID)
	}
	existing.Name = cs.Name
	existing.Description = cs.Description
	existing.UpdatedAt = time.Now().UTC()
	return nil
}

func (f *fakeKnowledgeStore) SetChangeSetStatus(_ context.Context, ws, id string, to knowledge.ChangeSetStatus) error {
	if to == knowledge.ChangeSetMerged {
		return errors.New("use SetMergeResult to merge a change-set")
	}
	cs, ok := f.changesets[fakeKey(ws, id)]
	if !ok {
		return fmt.Errorf("change-set %s not found", id)
	}
	if err := knowledge.ValidateStatusTransition(cs.Status, to); err != nil {
		return err
	}
	now := time.Now().UTC()
	if cs.Status == knowledge.ChangeSetDraft && to == knowledge.ChangeSetInReview {
		cs.SubmittedAt = &now
	}
	cs.Status = to
	cs.UpdatedAt = now
	return nil
}

func (f *fakeKnowledgeStore) SetMergeResult(_ context.Context, ws, id, mergedBy string, mergedAt time.Time) error {
	cs, ok := f.changesets[fakeKey(ws, id)]
	if !ok {
		return fmt.Errorf("change-set %s not found", id)
	}
	if err := knowledge.ValidateStatusTransition(cs.Status, knowledge.ChangeSetMerged); err != nil {
		return err
	}
	if mergedAt.IsZero() {
		mergedAt = time.Now().UTC()
	}
	cs.Status = knowledge.ChangeSetMerged
	cs.MergedBy = mergedBy
	cs.MergedAt = &mergedAt
	cs.UpdatedAt = mergedAt
	return nil
}

// --- Change-set ops --------------------------------------------------------

func (f *fakeKnowledgeStore) AppendOp(context.Context, *knowledge.ChangeSetOp) error { return nil }

func (f *fakeKnowledgeStore) RemoveOp(context.Context, string, string, int64) error { return nil }

func (f *fakeKnowledgeStore) ListOps(context.Context, string, string) ([]*knowledge.ChangeSetOp, error) {
	return nil, nil
}

// --- Reviews ---------------------------------------------------------------

func (f *fakeKnowledgeStore) AddReview(_ context.Context, r *knowledge.ChangeSetReview) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	key := fakeKey(r.WorkspaceID, r.ChangesetID)
	for i, ex := range f.reviews[key] {
		if ex.Reviewer == r.Reviewer {
			cp := *r
			f.reviews[key][i] = &cp
			return nil
		}
	}
	cp := *r
	f.reviews[key] = append(f.reviews[key], &cp)
	return nil
}

func (f *fakeKnowledgeStore) ListReviews(_ context.Context, ws, changesetID string) ([]*knowledge.ChangeSetReview, error) {
	return append([]*knowledge.ChangeSetReview(nil), f.reviews[fakeKey(ws, changesetID)]...), nil
}

// --- Pilots ----------------------------------------------------------------

func (f *fakeKnowledgeStore) AddPilot(context.Context, *knowledge.Pilot) error { return nil }

func (f *fakeKnowledgeStore) RemovePilot(context.Context, string, string, string, string) error {
	return nil
}

func (f *fakeKnowledgeStore) ListPilots(context.Context, string, string) ([]*knowledge.Pilot, error) {
	return nil, nil
}

func (f *fakeKnowledgeStore) ListPilotsForStream(context.Context, string, string, string) ([]*knowledge.Pilot, error) {
	return nil, nil
}

func (f *fakeKnowledgeStore) Close() error { return nil }

// compile-time check that the fake satisfies the Store interface.
var _ knowledge.Store = (*fakeKnowledgeStore)(nil)
