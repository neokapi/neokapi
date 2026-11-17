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

	bevent "github.com/neokapi/neokapi/bowrain/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// feedDecisionsOf calls the roll-up's decisions route as user.
func feedDecisionsOf(t *testing.T, srv *Server, pid, entry, user string) (int, feedDecisionsResponse) {
	t.Helper()
	e := echo.New()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/acme/"+pid+"/feed/"+entry+"/decisions", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(r, rec)
	c.SetParamNames("ws", "id", "entry")
	c.SetParamValues("acme", pid, entry)
	c.Set("project_permissions", fullCaller.perms)
	c.Set("user_id", user)
	require.NoError(t, srv.HandleFeedDecisions(c))
	var out feedDecisionsResponse
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out), rec.Body.String())
	}
	return rec.Code, out
}

// A reviewer approves one translation and rejects another. The feed files
// both in one roll-up, and its decisions route lists them with the
// translations as they stand, each undoable by the reviewer and by nobody
// else. Once the approval is withdrawn on the revision the route served, it
// no longer holds and offers no undo.
func TestFeedDecisions_ListsARollUpsDecisionsWithUndo(t *testing.T) {
	srv, cs := newReviewTestServer(t)
	bus := bevent.NewChannelEventBus()
	t.Cleanup(func() { bus.Close() })
	srv.EventBus = bus
	db := cs.SQLDB()
	srv.AuditLogger = bevent.NewAuditLogger(db, bus)
	t.Cleanup(srv.AuditLogger.Close)
	srv.ActivityStore = bstore.NewActivityStore(db)
	recorder := bevent.NewActivityRecorder(srv.ActivityStore, bus)
	t.Cleanup(recorder.Close)

	b1 := &model.Block{ID: "b1", Translatable: true}
	b1.SetSourceText("Hello")
	b1.SetTargetText("fr", "Bonjour")
	b2 := &model.Block{ID: "b2", Translatable: true}
	b2.SetSourceText("Goodbye")
	b2.SetTargetText("fr", "Salut")
	pid, ids := seedReviewProject(t, cs, []*model.Block{b1, b2})
	hello, bye := ids["Hello"], ids["Goodbye"]

	for _, d := range []struct {
		bid     string
		outcome change.Outcome
	}{{hello, change.OutcomeEstablish}, {bye, change.OutcomeReject}} {
		rec, _ := sendChanges(t, srv, pid, fullCaller, change.Set{Ops: []change.Op{
			decide(at("greetings.txt", d.bid, "fr"), readTargetRevision(t, srv, pid, d.bid, "fr"), d.outcome)}})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}

	var entry string
	require.Eventually(t, func() bool {
		res, err := srv.ActivityStore.List(t.Context(), bstore.ActivityQuery{ProjectID: pid, Kinds: []bstore.FeedKind{bstore.FeedKindReviewSummary}, Limit: 10})
		if err != nil || len(res.Activities) != 1 {
			return false
		}
		a := res.Activities[0]
		entry = a.ID
		return a.Counts["approved"] == 1 && a.Counts["rejected"] == 1
	}, 5*time.Second, 20*time.Millisecond, "the decisions are rolled up into one feed entry")

	var out feedDecisionsResponse
	require.Eventually(t, func() bool {
		_, out = feedDecisionsOf(t, srv, pid, entry, fullCaller.user)
		return len(out.Decisions) == 2
	}, 5*time.Second, 20*time.Millisecond, "both decisions are listed")
	assert.Equal(t, "fr", out.Locale)
	byBlock := map[string]feedDecision{}
	for _, d := range out.Decisions {
		byBlock[d.Card.BlockID] = d
	}
	assert.Equal(t, "approved", byBlock[hello].Decision)
	assert.Equal(t, "rejected", byBlock[bye].Decision)
	assert.Equal(t, "Bonjour", byBlock[hello].Card.TargetText)
	for _, d := range out.Decisions {
		assert.True(t, d.Current, d.Card.BlockID)
		assert.True(t, d.CanUndo, d.Card.BlockID)
	}

	_, other := feedDecisionsOf(t, srv, pid, entry, translateCaller.user)
	for _, d := range other.Decisions {
		assert.False(t, d.CanUndo, "only the reviewer undoes their decision")
	}

	rec, res := sendChanges(t, srv, pid, fullCaller, change.Set{Ops: []change.Op{
		decide(at("greetings.txt", hello, "fr"), byBlock[hello].Card.TargetRevision, change.OutcomeWithdraw)}})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, change.SetApplied, res.Status)

	require.Eventually(t, func() bool {
		_, out = feedDecisionsOf(t, srv, pid, entry, fullCaller.user)
		for _, d := range out.Decisions {
			if d.Card.BlockID == hello {
				return !d.CanUndo
			}
		}
		return false
	}, 5*time.Second, 20*time.Millisecond, "a withdrawn approval offers no undo")

	code, _ := feedDecisionsOf(t, srv, pid, "rvw_missing", fullCaller.user)
	assert.Equal(t, http.StatusNotFound, code)
}
