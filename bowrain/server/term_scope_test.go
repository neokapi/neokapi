package server

import (
	"net/http"
	"path/filepath"
	"slices"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/bowrain/termcandidate"
	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/termscope"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
)

// scopeFixture is a workspace with two projects sharing one terms store.
type scopeFixture struct {
	e    *echo.Echo
	srv  *Server
	tb   terms.Store
	a, b string
}

// scopeServer serves the context deck and concept routes of workspace "ws"
// signed in with perms.
func (f scopeFixture) scopeServer(perms platauth.Permission) *echo.Echo {
	e := echo.New()
	g := e.Group("/api/v1/:ws", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Set("user_id", "member-a")
			c.Set("workspace_id", "ws")
			c.Set("project_permissions", perms)
			return next(c)
		}
	})
	f.srv.registerContextObjectRoutes(g)
	f.srv.registerRulesDecideRoutes(g)
	f.srv.registerContextDeckRoutes(g)
	f.srv.registerConceptRoutes(g)
	return e
}

// newScopeFixture is a workspace over a SQLite content store and the terms
// store open returns, with two projects. The store holds one concept written
// with no project limit, as every concept was before the limit existed.
func newScopeFixture(t *testing.T, open func(t *testing.T) terms.Store) scopeFixture {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	tb := open(t)
	srv := &Server{
		ContentStore: cs, wsStores: newWorkspaceStores(), KnowledgeStore: newFakeKnowledgeStore(),
	}
	srv.wsStores.termsFactory = func() terms.Store { return tb }
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID: "c-legacy", Definition: "The store of source and target pairs.",
		Terms: []terms.Term{
			{Text: "content memory", Locale: "en", Status: model.TermPreferred},
			{Text: "mémoire de contenu", Locale: "fr", Status: model.TermPreferred},
		},
	}))
	f := scopeFixture{srv: srv, tb: tb}
	f.e = f.scopeServer(decideAll)
	f.a = seedDecideParity(t, f.e, cs)
	f.b = seedDecideParity(t, f.e, cs)
	return f
}

// byWorkspaceID runs fn on a server that can name the workspace's slug from
// its id, as the review plan and the ship pass do. The routes run without
// an auth store, which reads members the fixture does not have.
func (f scopeFixture) byWorkspaceID(fn func()) {
	f.srv.AuthStore = governingAuthStore{slug: "ws"}
	defer func() { f.srv.AuthStore = nil }()
	fn()
}

// scopeBackends are the terms stores the platform runs on.
var scopeBackends = []struct {
	name string
	open func(t *testing.T) terms.Store
}{
	{"sqlite", func(t *testing.T) terms.Store {
		tb, err := terms.NewSQLiteStore(filepath.Join(t.TempDir(), "terms.db"))
		require.NoError(t, err)
		t.Cleanup(func() { _ = tb.Close() })
		return tb
	}},
	{"postgres", func(t *testing.T) terms.Store {
		tb, err := sqlterms.NewPostgresStoreFromDB(pgtest.NewTestDB(t), "ws-scope-"+id.New())
		require.NoError(t, err)
		return tb
	}},
}

// keepInProject files a candidate in project pid the way the scan does and
// keeps it from the project's context deck.
func (f scopeFixture) keepInProject(t *testing.T, pid, term string) string {
	t.Helper()
	cid := fileCandidate(t, f.tb, pid, term, termcandidate.Reach{Blocks: 3}, nil)
	card := deckCardWhere(t, readContextDeck(t, f.e, pid, ""), candidateCard(cid))
	var out contextDeckDecideResponse
	require.Equal(t, http.StatusOK, decideRoute(t, f.e, http.MethodPost, pid, "/context/deck/decide",
		contextDeckDecideRequest{Item: card.Item, Revision: card.Revision, Action: "keep"}, &out))
	return cid
}

// governs reports, surface by surface, whether a concept governs project
// pid: the context deck, the checks at a point, the translate prompt's term
// rules, the term gate of the ship pass, and the concept list a client
// syncing the project reads.
func (f scopeFixture) governs(t *testing.T, pid, cid, sourceTerm string) map[string]bool {
	t.Helper()
	ctx := t.Context()
	out := map[string]bool{}

	// A kept term is attested and leaves the deck it was kept in, so the deck
	// is asked through the cards it deals for every concept.
	proj, err := f.srv.ContentStore.GetProject(ctx, pid)
	require.NoError(t, err)
	cards, _, err := f.srv.conceptCards(ctx, deckSource{project: proj, wsID: "ws", wsSlug: "ws"}, map[string]string{})
	require.NoError(t, err)
	out["deck"] = slices.ContainsFunc(cards, func(c contextDeckCard) bool { return c.Item.ID == cid })

	checks, err := f.srv.checksAtPoint(ctx, pid, "main", "", "ws", "ws", "fr")
	require.NoError(t, err)
	_, held, err := checks.Terms.GetConcept(ctx, cid)
	require.NoError(t, err)
	out["checks"] = held

	rules, err := jobs.TermRulesFromConcepts(ctx, editorTerms(ctx, f.srv.editorVoiceContext(), "ws", pid), pid, "en", "fr")
	require.NoError(t, err)
	out["translate"] = slices.ContainsFunc(rules, func(r coreprofile.TermRule) bool { return r.Term == sourceTerm })

	var gate *termGate
	f.byWorkspaceID(func() { gate = f.srv.resolveTermGate(ctx, proj, "main", "ws") })
	require.NotNil(t, gate)
	_, held, err = gate.tb.GetConcept(ctx, cid)
	require.NoError(t, err)
	out["gate"] = held

	var list TermSearchResponse
	require.Equal(t, http.StatusOK, decideRoute(t, f.e, http.MethodGet, "concepts", "?project="+pid, nil, &list))
	out["sync"] = slices.ContainsFunc(list.Concepts, func(c ConceptInfoResponse) bool { return c.ID == cid })
	return out
}

// counted is what the review plan and the term scan count for project pid:
// the concepts with a term in the source language.
func (f scopeFixture) counted(t *testing.T, pid string) (plan, scan int) {
	t.Helper()
	ctx := t.Context()
	proj, err := f.srv.ContentStore.GetProject(ctx, pid)
	require.NoError(t, err)
	f.byWorkspaceID(func() {
		counts, err := f.srv.planContextCounts(ctx, &platauth.Workspace{ID: "ws", Slug: "ws"}, proj, "", 0, nil)
		require.NoError(t, err)
		plan = counts.Concepts
		scan, err = f.srv.sourceConcepts(ctx, "ws", pid, "en")
		require.NoError(t, err)
	})
	return plan, scan
}

var (
	everywhere = map[string]bool{"deck": true, "checks": true, "translate": true, "gate": true, "sync": true}
	nowhere    = map[string]bool{"deck": false, "checks": false, "translate": false, "gate": false, "sync": false}
)

// TestTermScope_KeptTermAppliesInItsProject: a term kept in project A
// governs A and not B on every surface; widening it makes it govern both. A
// concept with no limit governs both throughout.
func TestTermScope_KeptTermAppliesInItsProject(t *testing.T) {
	for _, backend := range scopeBackends {
		t.Run(backend.name, func(t *testing.T) {
			f := newScopeFixture(t, backend.open)
			ctx := t.Context()
			cid := f.keepInProject(t, f.a, "Flowgraph")
			cp, ok, err := f.tb.GetConcept(ctx, cid)
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, []string{f.a}, termscope.Projects(cp), "keeping limits the term to its project")
			// A rendering, so the translate prompt has a rule to carry.
			cp.Terms = append(cp.Terms, terms.Term{Text: "graphe de flux", Locale: "fr", Status: model.TermPreferred})
			require.NoError(t, f.tb.AddConcept(ctx, cp))

			assert.Equal(t, everywhere, f.governs(t, f.a, cid, "Flowgraph"), "the term governs the project it was kept in")
			assert.Equal(t, nowhere, f.governs(t, f.b, cid, "Flowgraph"), "and no other")
			assert.Equal(t, everywhere, f.governs(t, f.a, "c-legacy", "content memory"), "a concept with no limit governs A")
			assert.Equal(t, everywhere, f.governs(t, f.b, "c-legacy", "content memory"), "and B")
			plan, scan := f.counted(t, f.a)
			assert.Equal(t, [2]int{2, 2}, [2]int{plan, scan}, "A counts both concepts")
			plan, scan = f.counted(t, f.b)
			assert.Equal(t, [2]int{1, 1}, [2]int{plan, scan}, "B counts the one of the whole workspace")

			var widened conceptScopeResponse
			require.Equal(t, http.StatusOK, decideRoute(t, f.e, http.MethodPost, "concepts/"+cid, "/widen", nil, &widened))
			assert.Empty(t, widened.Projects)
			cp, _, err = f.tb.GetConcept(ctx, cid)
			require.NoError(t, err)
			assert.True(t, termscope.Workspace(cp))
			assert.Equal(t, everywhere, f.governs(t, f.a, cid, "Flowgraph"), "widened, the term governs A")
			assert.Equal(t, everywhere, f.governs(t, f.b, cid, "Flowgraph"), "and B")
			plan, scan = f.counted(t, f.b)
			assert.Equal(t, [2]int{2, 2}, [2]int{plan, scan})
		})
	}
}

// TestTermScope_KeepLimitsUnscopedCandidate: a proposed concept with no
// limit, kept in a project, is limited to that project.
func TestTermScope_KeepLimitsUnscopedCandidate(t *testing.T) {
	f := newScopeFixture(t, scopeBackends[0].open)
	ctx := t.Context()
	require.NoError(t, f.tb.AddConcept(ctx, terms.Concept{
		ID: "c-open", Terms: []terms.Term{{Text: "Dashpanel", Locale: "en", Status: model.TermProposed}},
		Properties: map[string]string{termcandidate.PropProposedBy: termcandidate.ScanIdentity},
	}))
	card := deckCardWhere(t, readContextDeck(t, f.e, f.b, ""), candidateCard("c-open"))
	var out contextDeckDecideResponse
	require.Equal(t, http.StatusOK, decideRoute(t, f.e, http.MethodPost, f.b, "/context/deck/decide",
		contextDeckDecideRequest{Item: card.Item, Revision: card.Revision, Action: "keep"}, &out))
	cp, ok, err := f.tb.GetConcept(ctx, "c-open")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, []string{f.b}, termscope.Projects(cp))
	assert.Equal(t, model.TermPreferred, cp.Terms[0].Status)
}

// TestTermScope_WidenNeedsManageTerms: widening a term is a terms decision.
func TestTermScope_WidenNeedsManageTerms(t *testing.T) {
	f := newScopeFixture(t, scopeBackends[0].open)
	cid := f.keepInProject(t, f.a, "Flowgraph")
	viewer := f.scopeServer(platauth.PermViewContent)
	assert.Equal(t, http.StatusForbidden, decideRoute(t, viewer, http.MethodPost, "concepts/"+cid, "/widen", nil, nil))
	cp, _, err := f.tb.GetConcept(t.Context(), cid)
	require.NoError(t, err)
	assert.Equal(t, []string{f.a}, termscope.Projects(cp))
}
