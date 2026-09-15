package server

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A quality gate event follows the write that moved the gate. A push, an edit
// and a review decision announce the gates they change with no read in between,
// and reading a project's ship state, over the dashboard or the public feed,
// records nothing and publishes nothing.

// shipGateFailureRows counts the failures the record holds open for a project.
func shipGateFailureRows(t *testing.T, srv *Server, projectID string) int {
	t.Helper()
	pg, ok := srv.ContentStore.(*bstore.PostgresStore)
	require.True(t, ok, "the test server stores content in PostgreSQL")
	var n int
	require.NoError(t, pg.SQLDB().QueryRowContext(t.Context(),
		`SELECT count(*) FROM ship_gate_failures WHERE project_id = $1`, projectID).Scan(&n))
	return n
}

// awaitGateLines waits until the capture holds every wanted line, then drains
// it and returns what it drained. Announcements from a landed push arrive on the
// event bus, after the push request has returned.
func awaitGateLines(t *testing.T, srv *Server, c *gateCapture, want ...string) []string {
	t.Helper()
	require.Eventually(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		have := gateLines(c.events)
		for _, w := range want {
			if !slices.Contains(have, w) {
				return false
			}
		}
		return true
	}, 10*time.Second, 20*time.Millisecond, "the gate events %v are announced", want)
	return gateLines(c.drain(t, srv))
}

func TestShipGateEvents_ReadsAnnounceNothingAndRecordNothing(t *testing.T) {
	srv, token := newTestServer(t)
	events := captureGateEvents(t, srv)
	pid := seedGateProject(t, srv, "nb", "Setning 1", "")
	proj, err := srv.ContentStore.GetProject(t.Context(), pid)
	require.NoError(t, err)
	proj.Properties[ShipFeedProperty] = "true"
	require.NoError(t, srv.ContentStore.UpdateProject(t.Context(), proj))

	getDashboard(t, srv, token, pid, "")
	srv.invalidateDashboardCache("test-ws", pid)
	rec := httptest.NewRecorder()
	srv.GetEcho().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+pid+"/ship.json", nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	assert.Empty(t, gateLines(events.drain(t, srv)), "reading a withheld language announces nothing")
	assert.Zero(t, shipGateFailureRows(t, srv, pid), "reading a withheld language records no failure")
}

func TestShipGateEvents_PushEditAndReviewAnnounceWithNoRead(t *testing.T) {
	srv, token := newTestServer(t)
	events := captureGateEvents(t, srv)
	pid := createProject(t, srv, token)
	proj, err := srv.ContentStore.GetProject(t.Context(), pid)
	require.NoError(t, err)
	proj.TargetLanguages = []model.LocaleID{"nb"}
	require.NoError(t, srv.ContentStore.UpdateProject(t.Context(), proj))

	pushBlocksDrainedBy(t, srv, srv.GetEcho(), "Bearer "+token, pid, []pushBlockItem{
		{ID: "b1", Text: "Hello", ItemName: "en.json"},
		{ID: "b2", Text: "Goodbye", ItemName: "en.json"},
	}, drainPushQueueOnBus)
	assert.Equal(t, []string{"quality.gate.fail translated nb actual=0 required=2 not_checked=false"},
		awaitGateLines(t, srv, events, "quality.gate.fail translated nb actual=0 required=2 not_checked=false"),
		"a push that lands untranslated source opens the coverage gate")

	stored, err := srv.ContentStore.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: pid, Stream: "main", ItemName: "en.json"})
	require.NoError(t, err)
	ids := map[string]string{}
	for _, sb := range stored {
		ids[sb.Block.SourceText()] = sb.Block.ID
	}
	require.Len(t, ids, 2)

	for _, source := range []string{"Hello", "Goodbye"} {
		rec := writeBlockAs(t, srv.HandleUpdateBlockTarget, pid, ids[source], "", `{"target_locale":"nb","text":"Hei"}`)
		require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	}
	assert.Equal(t, []string{"quality.gate.pass translated nb actual=2 required=2 not_checked=false"}, gateLines(events.drain(t, srv)),
		"the edit that completes coverage clears the gate")

	reject := `{"target_locale":"nb","reviewed":false,"status":"draft","item_name":"en.json"}`
	rec, err := callReviewBlockBodyAs(t, srv, pid, ids["Hello"], reject, platauth.PermAll)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, []string{"quality.gate.fail rejected nb actual=1 required=0 not_checked=false"}, gateLines(events.drain(t, srv)),
		"a rejection waiting for a new draft withholds the language and announces the rejected gate")

	rec, err = callReviewBlockBodyAs(t, srv, pid, ids["Hello"], reject, platauth.PermAll)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Empty(t, gateLines(events.drain(t, srv)), "a decision that changes no gate announces nothing")
}
