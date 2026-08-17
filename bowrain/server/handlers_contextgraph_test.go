package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	platgraph "github.com/neokapi/neokapi/bowrain/graph"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	coreg "github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	fwterms "github.com/neokapi/neokapi/terms"
)

// The explorer's cross-project question, answered over the real stores: a
// Postgres content store, the workspace terminology, the real server writer and
// the real graph. Nothing here is a fixture written straight into the graph —
// the subgraph the handler reads is the one a push materializes.

const (
	cgWorkspace = "ws-context-graph"
	cgStream    = "main"
)

type cgHarness struct {
	srv   *Server
	graph *platgraph.SQLGraphStore
	terms fwterms.Store
}

func newContextGraphHarness(t *testing.T) *cgHarness {
	t.Helper()
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))
	db := pgtest.NewTestDB(t)
	initTestStoresOnDB(t, srv, db)
	srv.KnowledgeStore = newFakeKnowledgeStore()

	pool := db.Pool()
	require.NotNil(t, pool)
	g := platgraph.NewSQLGraphStore(pool)
	require.NoError(t, g.EnsureGraph(t.Context()))
	srv.GraphStore = g

	tb, err := srv.wsStores.getTerms(cgWorkspace)
	require.NoError(t, err)
	return &cgHarness{srv: srv, graph: g, terms: tb}
}

// req builds an echo context for the context-graph route, with the workspace
// the handler resolves both the graph scope and the terminology from.
func (h *cgHarness) req(t *testing.T, target, conceptID string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	c := h.srv.GetEcho().NewContext(req, rec)
	c.Set("project_permissions", platauth.PermViewContent)
	c.Set("workspace_id", cgWorkspace)
	c.SetParamNames("ws", "cid")
	c.SetParamValues(cgWorkspace, conceptID)
	return c, rec
}

// seedProject creates a project with one collection holding one block, exactly
// as a push leaves it.
func (h *cgHarness) seedProject(t *testing.T, ctx context.Context, name, collection, text string) *platstore.Project {
	t.Helper()
	cs := h.srv.ContentStore
	proj := &platstore.Project{
		Name:                  name,
		DefaultSourceLanguage: "en",
		WorkspaceID:           cgWorkspace,
		DefaultStream:         cgStream,
		Properties:            map[string]string{},
	}
	require.NoError(t, cs.CreateProject(ctx, proj))
	require.NoError(t, cs.CreateCollection(ctx, &platstore.Collection{
		ID:        proj.ID + "-" + collection,
		ProjectID: proj.ID,
		Name:      collection,
		Kind:      platstore.CollectionUploaded,
		Stream:    cgStream,
		Context:   map[string]string{"product": "acme", "channel": collection},
		Owner:     venue.ContextOwnerRecipe,
	})) //nolint:exhaustruct // the writer reads name, context and stream
	item := collection + ".json"
	require.NoError(t, cs.StoreItem(ctx, proj.ID, cgStream, &platstore.Item{
		ProjectID: proj.ID, Name: item, Format: "json", ItemType: "file",
		CollectionID: proj.ID + "-" + collection,
	}))
	b := &model.Block{ID: "intro", Translatable: true}
	b.SetSourceText(text)
	require.NoError(t, cs.StoreBlocksForItem(ctx, proj.ID, cgStream, item, []*model.Block{b}))
	return proj
}

// materialize runs the same writer the push event runs.
func (h *cgHarness) materialize(t *testing.T, ctx context.Context, proj *platstore.Project) {
	t.Helper()
	_, err := platgraph.MaterializeProjectContextGraph(ctx, h.graph, platgraph.ProjectSubgraph{
		Content:     h.srv.ContentStore,
		Terms:       h.terms,
		WorkspaceID: cgWorkspace,
		ProjectID:   proj.ID,
		ProjectName: proj.Name,
		Stream:      cgStream,
	})
	require.NoError(t, err)
}

func (h *cgHarness) ask(t *testing.T, target, conceptID string) ConceptProjectsResponse {
	t.Helper()
	c, rec := h.req(t, target, conceptID)
	require.NoError(t, h.srv.HandleConceptProjects(c))
	require.Equal(t, http.StatusOK, rec.Code)
	var got ConceptProjectsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	return got
}

// TestConceptProjectsAnswersFromTheGraph is the wave's acceptance at the REST
// boundary: the question the explorer asks reaches the graph the push wrote,
// and the answer names both using projects and omits the third.
func TestConceptProjectsAnswersFromTheGraph(t *testing.T) {
	h := newContextGraphHarness(t)
	ctx := t.Context()

	require.NoError(t, h.terms.AddConcept(ctx, fwterms.Concept{
		ID: "c-memory", Domain: "product",
		Terms: []fwterms.Term{{Text: "content memory", Locale: "en", Status: model.TermApproved}},
	}))

	docs := h.seedProject(t, ctx, "docs-site", "docs", "The content memory recycles approved wording.")
	app := h.seedProject(t, ctx, "app", "web", "Content memory is what the app reuses.")
	other := h.seedProject(t, ctx, "unrelated", "misc", "Nothing relevant here at all.")
	for _, p := range []*platstore.Project{docs, app, other} {
		h.materialize(t, ctx, p)
	}

	got := h.ask(t, "/", "c-memory")
	assert.Equal(t, "graph", got.Source, "the traversal answered, not a scan")
	assert.False(t, got.Partial)

	byID := map[string]ConceptProjectUseResponse{}
	for _, p := range got.Projects {
		byID[p.ProjectID] = p
	}
	require.Len(t, got.Projects, 2, "only the projects whose content uses the concept")
	assert.Contains(t, byID, docs.ID)
	assert.Contains(t, byID, app.ID)
	assert.NotContains(t, byID, other.ID)

	assert.Equal(t, "docs-site", byID[docs.ID].ProjectName, "the display name is joined onto the id")
	assert.Equal(t, 1, byID[docs.ID].Blocks)
	assert.Equal(t, 1, byID[docs.ID].Occurrences)
	assert.Equal(t, []string{"docs"}, byID[docs.ID].Collections)
	require.Len(t, byID[docs.ID].Terms, 1)
	assert.Equal(t, "content memory", byID[docs.ID].Terms[0].Term)

	require.Len(t, got.Uses, 2)
	assert.Equal(t, cgStream, got.Uses[0].Stream)
	assert.NotEmpty(t, got.Uses[0].Document)
	assert.Contains(t, strings.ToLower(got.Uses[0].Text), "content memory",
		"the wording is read for the page shown, one indexed block at a time")

	// Pinning the project narrows the same traversal rather than needing a
	// second endpoint.
	narrowed := h.ask(t, "/?project="+app.ID, "c-memory")
	require.Len(t, narrowed.Projects, 1)
	assert.Equal(t, app.ID, narrowed.Projects[0].ProjectID)
}

// TestConceptProjectsTellsNoUseFromNoGraph pins the distinction a pane cannot
// make for itself: a concept the graph holds and nobody uses is an answer, and
// a concept the graph has never seen is a different one.
func TestConceptProjectsTellsNoUseFromNoGraph(t *testing.T) {
	h := newContextGraphHarness(t)
	ctx := t.Context()

	require.NoError(t, h.terms.AddConcept(ctx, fwterms.Concept{
		ID:    "c-memory",
		Terms: []fwterms.Term{{Text: "content memory", Locale: "en", Status: model.TermApproved}},
	}))
	require.NoError(t, h.terms.AddConcept(ctx, fwterms.Concept{
		ID:    "c-unused",
		Terms: []fwterms.Term{{Text: "cold storage", Locale: "en", Status: model.TermApproved}},
	}))
	proj := h.seedProject(t, ctx, "docs-site", "docs", "The content memory recycles wording.")
	h.materialize(t, ctx, proj)

	held := h.ask(t, "/", "c-unused")
	assert.Equal(t, "graph", held.Source)
	assert.Empty(t, held.Projects)
	assert.NotEmpty(t, held.Notes, "the graph holds it and nothing uses it — that is an answer")

	unknown := h.ask(t, "/", "c-never-pushed")
	assert.Equal(t, "scan", unknown.Source,
		"a concept the graph never recorded falls back rather than reporting nothing")
	require.NotEmpty(t, unknown.Notes)
	assert.Contains(t, unknown.Notes[0], "bounded scan")

	// A concept added after the last push is in the terms store and not in the
	// graph, which is exactly the case the fallback exists for. It answers the
	// same question, so it answers it at the same narrowing: a reader who pinned
	// a project does not get the workspace back when the source changes.
	h.seedProject(t, ctx, "app", "web", "A warm cache is not the content memory.")
	require.NoError(t, h.terms.AddConcept(ctx, fwterms.Concept{
		ID:    "c-late",
		Terms: []fwterms.Term{{Text: "warm cache", Locale: "en", Status: model.TermApproved}},
	}))

	wide := h.ask(t, "/", "c-late")
	require.Equal(t, "scan", wide.Source)
	require.Len(t, wide.Projects, 1, "the walk finds the use the graph has not recorded yet")

	pinned := h.ask(t, "/?project="+proj.ID, "c-late")
	require.Equal(t, "scan", pinned.Source)
	assert.Empty(t, pinned.Projects,
		"pinning the project that does not use it walks that project alone")
}

// TestConceptProjectsDegradesToAPartial is the degradation contract at the
// surface: a fallback scan that runs out of its budget answers with a floor
// that says it is one. The pane renders a qualified answer; it never fails.
func TestConceptProjectsDegradesToAPartial(t *testing.T) {
	h := newContextGraphHarness(t)
	ctx := t.Context()
	h.srv.conceptScanBudget = time.Nanosecond

	require.NoError(t, h.terms.AddConcept(ctx, fwterms.Concept{
		ID:    "c-memory",
		Terms: []fwterms.Term{{Text: "content memory", Locale: "en", Status: model.TermApproved}},
	}))
	h.seedProject(t, ctx, "docs-site", "docs", "The content memory recycles wording.")
	// Deliberately not materialized: the concept is not in the graph, so the
	// answer comes from the walk, whose budget is already spent.

	c, rec := h.req(t, "/", "c-memory")
	require.NoError(t, h.srv.HandleConceptProjects(c))
	require.Equal(t, http.StatusOK, rec.Code, "an exhausted budget is an answer, not a 500")

	var got ConceptProjectsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "scan", got.Source)
	assert.True(t, got.Partial, "the answer says it is a floor")
	assert.NotEmpty(t, got.PartialReason)
}

// TestConceptProjectsSaysHowTheConceptStandsHere is the other half of the
// cross-project question: naming the projects is not an answer on its own, and
// the pane has to be able to say the concept is discouraged in one of them.
//
// The same concept, the same block, two instants: before the deprecation the
// project is using an ordinary word, after it the project is breaking a rule.
// A workspace-global status could not tell those apart, which is exactly what
// made "is it discouraged here" unanswerable at this reach.
func TestConceptProjectsSaysHowTheConceptStandsHere(t *testing.T) {
	h := newContextGraphHarness(t)
	ctx := t.Context()

	deprecatedFrom := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, h.terms.AddConcept(ctx, fwterms.Concept{
		ID: "c-signin", Domain: "product",
		Terms: []fwterms.Term{
			{Text: "sign in", Locale: "en", Status: model.TermPreferred},
			{
				Text: "log in", Locale: "en", Status: model.TermDeprecated,
				Validity: &coreg.Validity{ValidFrom: &deprecatedFrom},
			},
		},
	}))

	proj := h.seedProject(t, ctx, "docs-site", "docs",
		"Sign in to continue. You can also log in from the app, or log in on the web.")
	h.materialize(t, ctx, proj)

	early := h.ask(t, "/?at=2026-01-01T00:00:00Z", "c-signin")
	require.Len(t, early.Projects, 1)
	assert.False(t, early.Projects[0].Discouraged,
		"before the window opens the deprecated spelling is not in force here")
	assert.Equal(t, string(model.TermPreferred), early.Projects[0].Status)
	assert.Empty(t, early.Projects[0].Replacement)
	require.Len(t, early.Projects[0].Terms, 1)
	assert.Equal(t, "sign in", early.Projects[0].Terms[0].Term)

	late := h.ask(t, "/?at=2026-07-01T00:00:00Z", "c-signin")
	require.Len(t, late.Projects, 1)
	assert.True(t, late.Projects[0].Discouraged,
		"after it opens the same block is a block still saying the discouraged word")
	assert.Equal(t, string(model.TermDeprecated), late.Projects[0].Status)
	assert.Equal(t, "sign in", late.Projects[0].Replacement,
		"the answer says what to reach for instead")

	byTerm := map[string]ConceptTermUseResponse{}
	for _, term := range late.Projects[0].Terms {
		byTerm[term.Term] = term
	}
	require.Len(t, byTerm, 2, "two spellings of one concept are two uses, not one")
	assert.True(t, byTerm["log in"].Discouraged)
	assert.Equal(t, 2, byTerm["log in"].Occurrences)
	assert.False(t, byTerm["sign in"].Discouraged)

	discouragedUses := 0
	for _, use := range late.Uses {
		if use.Discouraged {
			discouragedUses++
		}
	}
	assert.Positive(t, discouragedUses, "a use names the standing of the term it used")
}

// TestListConceptsResolvesTheWindowAtAnInstant pins the governance half: the
// concept list answered the same however the reader stood, because it never
// applied a term's own validity. With ?at= it answers at a point.
func TestListConceptsResolvesTheWindowAtAnInstant(t *testing.T) {
	h := newKGHarness(t)
	ctx := t.Context()
	deprecatedFrom := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, h.tb(t).AddConcept(ctx, fwterms.Concept{
		ID: "c-signin",
		Terms: []fwterms.Term{
			{Text: "sign in", Locale: "en", Status: model.TermPreferred},
			{
				Text: "log in", Locale: "en", Status: model.TermDeprecated,
				Validity: &coreg.Validity{ValidFrom: &deprecatedFrom},
			},
		},
	}))

	list := func(target string) TermSearchResponse {
		t.Helper()
		c, rec := h.req(http.MethodGet, target, "", platauth.PermViewContent)
		require.NoError(t, h.srv.HandleListConcepts(c))
		require.Equal(t, http.StatusOK, rec.Code)
		var got TermSearchResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		return got
	}

	declared := list("/concepts")
	require.Len(t, declared.Concepts, 1)
	assert.Len(t, declared.Concepts[0].Terms, 2, "with no instant the answer is as declared")

	early := list("/concepts?at=2026-01-01T00:00:00Z")
	require.Len(t, early.Concepts, 1)
	require.Len(t, early.Concepts[0].Terms, 1, "a term whose window has not opened is not in force")
	assert.Equal(t, "sign in", early.Concepts[0].Terms[0].Text)

	late := list("/concepts?at=2026-07-01T00:00:00Z")
	require.Len(t, late.Concepts, 1)
	assert.Len(t, late.Concepts[0].Terms, 2)

	bad := func() *httptest.ResponseRecorder {
		c, rec := h.req(http.MethodGet, "/concepts?at=whenever", "", platauth.PermViewContent)
		require.NoError(t, h.srv.HandleListConcepts(c))
		return rec
	}()
	assert.Equal(t, http.StatusBadRequest, bad.Code, "an instant that cannot be read is refused")
}

// TestConceptProjectsRejectsAnUnreadableInstant keeps the temporal parameter
// honest: an instant that cannot be parsed is refused rather than silently
// answered at now, which would report a different point than the one asked for.
func TestConceptProjectsRejectsAnUnreadableInstant(t *testing.T) {
	h := newContextGraphHarness(t)
	c, rec := h.req(t, "/?at=last-tuesday", "c-memory")
	require.NoError(t, h.srv.HandleConceptProjects(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestContextGraphRouteRegistered proves the read surface is wired: the query
// existed and passed for a wave with no route over it.
func TestContextGraphRouteRegistered(t *testing.T) {
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))
	e := echo.New()
	g := e.Group("/:ws")
	require.NotPanics(t, func() { srv.registerContextGraphRoutes(g) })

	got := map[string]bool{}
	for _, r := range e.Routes() {
		got[r.Method+" "+r.Path] = true
	}
	assert.True(t, got["GET /:ws/context/concepts/:cid/projects"],
		"the context graph has a read route")
}
