package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/knowledge"
)

// These cover what the governance dashboard used to buy with one request per
// row: a change-set's detail for its op count, and a full blast-radius walk per
// pending change-set.

// seedChangeSet stores a draft with opCount concept-delete ops.
func (h *kgHarness) seedChangeSet(t *testing.T, name string, opCount int) *knowledge.ChangeSet {
	t.Helper()
	cs := &knowledge.ChangeSet{WorkspaceID: kgTestWS, Name: name, CreatedBy: "author"}
	require.NoError(t, h.fake.CreateChangeSet(t.Context(), cs))
	for i := range opCount {
		payload, err := json.Marshal(knowledge.ConceptDeletePayload{ConceptID: string(rune('a' + i))})
		require.NoError(t, err)
		require.NoError(t, h.fake.AppendOp(t.Context(), &knowledge.ChangeSetOp{
			WorkspaceID: kgTestWS, ChangesetID: cs.ID, Op: knowledge.OpConceptDelete,
			Payload: payload, CreatedBy: "author",
		}))
	}
	return cs
}

func TestListChangeSetsCarriesOpsCount(t *testing.T) {
	h := newKGHarness(t)
	empty := h.seedChangeSet(t, "empty", 0)
	full := h.seedChangeSet(t, "full", 3)

	c, rec := h.req(http.MethodGet, "/changesets", "", platauth.PermViewContent)
	require.NoError(t, h.srv.HandleListChangeSets(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var got []knowledge.ChangeSetSummary
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	counts := map[string]int{}
	for _, cs := range got {
		counts[cs.ID] = cs.OpsCount
	}
	assert.Equal(t, map[string]int{empty.ID: 0, full.ID: 3}, counts)

	// ops_count rides on the same flat object as the change-set header, so a
	// caller decoding the old shape still reads every field it had.
	assert.Contains(t, rec.Body.String(), `"ops_count":3`)
	assert.Contains(t, rec.Body.String(), `"status":"draft"`)
}

func TestChangeSetCountsBucketsByStatus(t *testing.T) {
	h := newKGHarness(t)
	h.seedChangeSet(t, "draft-one", 1)
	submitted := h.seedChangeSet(t, "submitted", 1)
	require.NoError(t, h.fake.SetChangeSetStatus(t.Context(), kgTestWS, submitted.ID, knowledge.ChangeSetInReview))

	c, rec := h.req(http.MethodGet, "/changesets/counts", "", platauth.PermViewContent)
	require.NoError(t, h.srv.HandleGetChangeSetCounts(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var got ChangeSetCountsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, 2, got.Total)
	assert.Equal(t, 1, got.ByStatus["draft"])
	assert.Equal(t, 1, got.ByStatus["in_review"])
	assert.Len(t, got.ByStatus, 6, "every lifecycle status is present, zero included")
	assert.Zero(t, got.ByStatus["merged"])
}

// TestCountsRoutesBeatTheIdParam: /changesets/counts and
// /concepts/status-counts sit next to :id and :cid segments, so the static
// route has to win or the count lands in the single-item handler.
func TestCountsRoutesBeatTheIdParam(t *testing.T) {
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))
	e := echo.New()
	g := e.Group("/:ws")
	srv.registerConceptRoutes(g)
	srv.registerChangesetRoutes(g)

	for path, want := range map[string]string{
		"/acme/changesets/counts":        "/:ws/changesets/counts",
		"/acme/concepts/status-counts":   "/:ws/concepts/status-counts",
		"/acme/concepts/locale-coverage": "/:ws/concepts/locale-coverage",
		"/acme/changesets/cs-1":          "/:ws/changesets/:id",
		"/acme/concepts/c-1":             "/:ws/concepts/:cid",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		c := e.NewContext(req, httptest.NewRecorder())
		e.Router().Find(http.MethodGet, path, c)
		assert.Equal(t, want, c.Path(), "%s must route to %s", path, want)
	}
}

// TestBlastRadiusAnswersFromTheStoredSummary is the point of storing it: the
// read is a row, not a walk, and it says when it was computed.
func TestBlastRadiusAnswersFromTheStoredSummary(t *testing.T) {
	h := newKGHarness(t)
	cs := h.seedChangeSet(t, "stored", 1)
	at := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	require.NoError(t, h.fake.SetChangeSetImpact(t.Context(), kgTestWS, cs.ID, &knowledge.ImpactSummary{
		TotalBlocks: 900, AffectedBlocks: 12, NewViolations: 7, Words: 340, Projects: 2,
		ComputedAt: at,
	}))

	c, rec := h.req(http.MethodGet, "/changesets/"+cs.ID+"/blast-radius", "",
		platauth.PermViewContent, "id", cs.ID)
	require.NoError(t, h.srv.HandleChangeSetBlastRadius(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var got knowledge.ChangeSetImpact
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.True(t, got.Stored)
	require.NotNil(t, got.ComputedAt)
	assert.Equal(t, at, got.ComputedAt.UTC())
	assert.Equal(t, 12, got.AffectedBlocks)
	assert.Empty(t, got.Projects, "the stored summary carries no breakdown")
}

// TestBlastRadiusFreshBypassesTheStoredSummary: a truthy ?fresh must reach the
// walk even when a summary is sitting there. The harness has no content store,
// so the walk is unavailable and the handler says so — which is the proof it
// went looking for the engine rather than answering from the row.
//
// Every spelling boolParam accepts is asserted because the handler used to
// compare against the literal "1": ?fresh=true asked for a recomputation and
// silently got the stored row back.
func TestBlastRadiusFreshBypassesTheStoredSummary(t *testing.T) {
	for _, query := range []string{"?fresh=1", "?fresh=true", "?fresh=t"} {
		t.Run(query, func(t *testing.T) {
			h := newKGHarness(t)
			cs := h.seedChangeSet(t, "stored", 1)
			require.NoError(t, h.fake.SetChangeSetImpact(t.Context(), kgTestWS, cs.ID, &knowledge.ImpactSummary{
				AffectedBlocks: 12, ComputedAt: time.Now().UTC(),
			}))

			c, _ := h.req(http.MethodGet, "/changesets/"+cs.ID+"/blast-radius"+query, "",
				platauth.PermViewContent, "id", cs.ID)
			assertEngineUnavailable(t, h.srv.HandleChangeSetBlastRadius(c))
		})
	}
}

// TestBlastRadiusWithoutASummaryWalks: a draft that has never been submitted
// carries no summary, so the endpoint falls back to the live scan.
func TestBlastRadiusWithoutASummaryWalks(t *testing.T) {
	h := newKGHarness(t)
	cs := h.seedChangeSet(t, "unsubmitted", 1)

	c, _ := h.req(http.MethodGet, "/changesets/"+cs.ID+"/blast-radius", "",
		platauth.PermViewContent, "id", cs.ID)
	assertEngineUnavailable(t, h.srv.HandleChangeSetBlastRadius(c))
}

// The trial endpoint sits under the pilot path, which already carries a
// :project/:stream pair the DELETE claims. This pins that the router lands on
// the findings route rather than folding the trailing segment into the stream.
func TestTrialFindingsRouteShape(t *testing.T) {
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))
	e := echo.New()
	g := e.Group("/:ws")
	srv.registerChangesetRoutes(g)

	path := "/acme/changesets/cs-1/pilots/proj-1/main/findings"
	req := httptest.NewRequest(http.MethodGet, path, nil)
	c := e.NewContext(req, httptest.NewRecorder())
	e.Router().Find(http.MethodGet, path, c)
	assert.Equal(t, "/:ws/changesets/:id/pilots/:project/:stream/findings", c.Path())
}

// A trial needs a walk, and the harness carries no content store — so a 503 is
// the proof the handler reached for the engine. It also pins that a trial does
// NOT require a pilot to exist: the refusal is the missing engine, not a missing
// pilot, because "bind it and find out" must not be the only way to find out.
func TestTrialFindingsNeedsTheEngineNotAPilot(t *testing.T) {
	h := newKGHarness(t)
	cs := h.seedChangeSet(t, "trial", 1)

	c, _ := h.req(http.MethodGet, "/changesets/"+cs.ID+"/pilots/proj-1/main/findings", "",
		platauth.PermViewContent, "id", cs.ID, "project", "proj-1", "stream", "main")
	assertEngineUnavailable(t, h.srv.HandleTrialFindings(c))
}

// assertEngineUnavailable asserts the handler refused with a 503 because the
// walk had no engine to run on — the harness carries no content store.
func assertEngineUnavailable(t *testing.T, err error) {
	t.Helper()
	var httpErr *echo.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusServiceUnavailable, httpErr.Code)
}

// TestRecordChangeSetImpactStoresNothingWithoutAnEngine: the submit path must
// leave no summary rather than a wrong one when the walk cannot run.
func TestRecordChangeSetImpactStoresNothingWithoutAnEngine(t *testing.T) {
	h := newKGHarness(t)
	cs := h.seedChangeSet(t, "no-engine", 1)

	h.srv.recordChangeSetImpact(t.Context(), kgTestWS, kgTestWS, cs.ID)

	got, err := h.fake.GetChangeSet(t.Context(), kgTestWS, cs.ID)
	require.NoError(t, err)
	assert.Nil(t, got.Impact)
}
