package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// planWriteAs is planWrite for a named user.
func planWriteAs(t *testing.T, s *Server, h echo.HandlerFunc, wsID, projID, body, user string, perms platauth.Permission) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	r.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(r, rec)
	c.SetPathValues(echo.PathValues{{Name: "ws", Value: "rc"}, {Name: "id", Value: projID}})
	c.Set("workspace_id", wsID)
	c.Set("user_id", user)
	c.Set("project_permissions", perms)
	if err := h(c); err != nil {
		require.ErrorIs(t, err, errAccessDenied)
	}
	return rec
}

// readPlanAs reads the plan for the owner, who sees every language.
func readPlanAs(t *testing.T, s *Server, wsID, projID string) reviewPlanResponse {
	t.Helper()
	rec := deckCall(t, s, s.HandleGetReviewPlan, wsID, projID, "", "owner-rc", platauth.PermAll, nil, url.Values{})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decodeJSON[reviewPlanResponse](t, rec)
}

// addMember adds a workspace member with a role.
func addMember(t *testing.T, s *Server, wsID, id string, role platauth.Role) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, s.AuthStore.CreateUser(ctx, &platauth.User{ID: id, Email: id + "@rc.test", Name: "Member " + id}))
	require.NoError(t, s.AuthStore.AddMember(ctx, wsID, id, role))
}

// runCompleted publishes a run completion for the project and waits for the
// settle-after-run group to handle it.
func runCompleted(t *testing.T, s *Server, projID string, passes int) {
	t.Helper()
	ev := platev.Event{
		ID: "ev-" + projID + "-" + strconv.FormatInt(time.Now().UnixNano(), 10), Type: platev.EventConvergenceRunCompleted, ProjectID: projID,
		Data: map[string]string{"state": bstore.ConvergenceRunConverged, "passes": strconv.Itoa(passes), "stream": "main"},
	}
	bus, ok := s.EventBus.(*handledBus)
	require.True(t, ok)
	done := bus.done(settleAfterRunGroup, ev.ID)
	s.EventBus.Publish(ev)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("the settle-after-run group never handled the run completion")
	}
}

func TestSettleAfterRun_SettlesWhenThePolicyAllows(t *testing.T) {
	s, wsID, ownerID := newRecheckHarness(t)
	s.subscribeSettleAfterRun()
	projID, _ := containerProject(t, s, wsID)
	require.Equal(t, http.StatusNoContent,
		planWriteAs(t, s, s.HandlePutReviewInvolvement, wsID, projID, `{"involvement":"context"}`, ownerID, platauth.PermAll).Code)

	var mu sync.Mutex
	var bulk []platev.Event
	sub := s.EventBus.Subscribe(platev.EventReviewBulkApproved, func(ev platev.Event) {
		mu.Lock()
		defer mu.Unlock()
		bulk = append(bulk, ev)
	})
	t.Cleanup(func() { s.EventBus.Unsubscribe(sub) })

	before := readPlanAs(t, s, wsID, projID)
	assert.Equal(t, settlePolicy, before.Policy.Settle)
	require.NotNil(t, before.Policy.Auto)
	assert.Equal(t, ownerID, before.Policy.Auto.By)
	assert.Equal(t, autoSettleByInvolvement, before.Policy.Auto.Source)
	assert.Empty(t, before.Policy.Auto.Skip)
	assert.Nil(t, before.Policy.Last)

	runCompleted(t, s, projID, 1)

	plan := readPlanAs(t, s, wsID, projID)
	require.NotNil(t, plan.Policy.Last, "the pass is recorded on the project")
	assert.True(t, plan.Policy.Last.Automatic)
	assert.Equal(t, ownerID, plan.Policy.Last.By)
	assert.Equal(t, "Owner", plan.Policy.Last.ByName)
	assert.Positive(t, plan.Policy.Last.Approved)
	for _, l := range plan.Languages {
		assert.Zero(t, l.Settleable, "every translation that cleared the bars is established")
	}

	// The feed reads the same record a pass by hand files, credited to the
	// person who set the involvement.
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(bulk) > 0
	}, 5*time.Second, 20*time.Millisecond)
	mu.Lock()
	assert.Equal(t, ownerID, bulk[0].Actor)
	assert.Equal(t, "fr", bulk[0].Data["locale"])
	mu.Unlock()
}

func TestSettleAfterRun_SkipsARunThatDraftedNothing(t *testing.T) {
	s, wsID, ownerID := newRecheckHarness(t)
	s.subscribeSettleAfterRun()
	projID, _ := containerProject(t, s, wsID)
	require.NoError(t, s.updatePlanProperties(t.Context(), projID, map[string]string{propInvolvementSetBy: ownerID}))

	runCompleted(t, s, projID, 0)
	assert.Nil(t, readPlanAs(t, s, wsID, projID).Policy.Last, "a run with no production pass is not followed by a pass")
}

func TestSettleAfterRun_SkippedUnderEvery(t *testing.T) {
	s, wsID, ownerID := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	require.Equal(t, http.StatusNoContent,
		planWriteAs(t, s, s.HandlePutReviewInvolvement, wsID, projID, `{"involvement":"every"}`, ownerID, platauth.PermAll).Code)

	assert.Nil(t, s.settleAfterRun(t.Context(), projID, "main"))
	plan := readPlanAs(t, s, wsID, projID)
	assert.Equal(t, settleNone, plan.Policy.Settle)
	assert.Nil(t, plan.Policy.Last)
}

func TestSettleAfterRun_TakesOneFirstPassThenWaitsForTheContext(t *testing.T) {
	s, wsID, ownerID := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	openContext(t, s)
	require.NoError(t, s.updatePlanProperties(t.Context(), projID, map[string]string{propInvolvementSetBy: ownerID}))

	first := s.settleAfterRun(t.Context(), projID, "main")
	require.NotNil(t, first, "the first pass settles on the checks alone")
	assert.True(t, first.FirstPass)
	plan := readPlanAs(t, s, wsID, projID)
	assert.Equal(t, settleHeld, plan.Policy.Settle)
	assert.Equal(t, ownerID, plan.Policy.FirstPassBy)

	assert.Nil(t, s.settleAfterRun(t.Context(), projID, "main"), "a later pass waits while the context is open")
}

func TestSettleAfterRun_CreditsTheCoordinatesConfirmerWhenNobodySetTheInvolvement(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	addMember(t, s, wsID, "u-admin", platauth.RoleAdmin)
	require.Equal(t, http.StatusNoContent,
		planWriteAs(t, s, s.HandleConfirmReviewCoordinates, wsID, projID, "", "u-admin", platauth.PermAll).Code)

	plan := readPlanAs(t, s, wsID, projID)
	require.NotNil(t, plan.Policy.Auto)
	assert.Equal(t, "u-admin", plan.Policy.Auto.By)
	assert.Equal(t, autoSettleByCoordinates, plan.Policy.Auto.Source)

	require.NotNil(t, s.settleAfterRun(t.Context(), projID, "main"))
	plan = readPlanAs(t, s, wsID, projID)
	require.NotNil(t, plan.Policy.Last)
	assert.Equal(t, "u-admin", plan.Policy.Last.By)
}

func TestSettleAfterRun_SkippedWhenNoOneCanBeCredited(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)

	assert.Nil(t, s.settleAfterRun(t.Context(), projID, "main"))
	plan := readPlanAs(t, s, wsID, projID)
	require.NotNil(t, plan.Policy.Auto)
	assert.Equal(t, autoSettleNoOne, plan.Policy.Auto.Skip)
	assert.Nil(t, plan.Policy.Last)
}

func TestSettleAfterRun_SkippedWhenThePersonLostThePermission(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	addMember(t, s, wsID, "u-viewer", platauth.RoleViewer)
	// The person set the involvement and has since become a viewer.
	require.NoError(t, s.updatePlanProperties(t.Context(), projID, map[string]string{propInvolvementSetBy: "u-viewer"}))

	assert.Nil(t, s.settleAfterRun(t.Context(), projID, "main"))
	plan := readPlanAs(t, s, wsID, projID)
	require.NotNil(t, plan.Policy.Auto)
	assert.Equal(t, "u-viewer", plan.Policy.Auto.By)
	assert.Equal(t, autoSettleNoPermission, plan.Policy.Auto.Skip)
	assert.Nil(t, plan.Policy.Last)
}

func TestSettleAfterRun_NeverRunsTwoPassesForOneProject(t *testing.T) {
	s, wsID, ownerID := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	require.NoError(t, s.updatePlanProperties(t.Context(), projID, map[string]string{propInvolvementSetBy: ownerID}))

	// A pass on this instance holds the project.
	release, ok := s.beginSettle(t.Context(), projID)
	require.True(t, ok)
	_, again := s.beginSettle(t.Context(), projID)
	assert.False(t, again, "a second pass on the same instance is refused")
	assert.Nil(t, s.settleAfterRun(t.Context(), projID, "main"))
	rec := planWriteAs(t, s, s.HandleSettleReviewPlan, wsID, projID, `{}`, ownerID, platauth.PermAll)
	assert.Equal(t, http.StatusConflict, rec.Code, "the settle step waits for the running pass")
	release()

	// Another instance holds the project's advisory lock.
	db := s.advisoryDB()
	require.NotNil(t, db)
	conn, err := db.Conn(t.Context())
	require.NoError(t, err)
	defer conn.Close()
	var locked bool
	require.NoError(t, conn.QueryRowContext(t.Context(), `SELECT pg_try_advisory_lock(hashtext($1))`, settleLockKey(projID)).Scan(&locked))
	require.True(t, locked)
	assert.Nil(t, s.settleAfterRun(t.Context(), projID, "main"), "a pass held on another instance keeps this one out")
	_, err = conn.ExecContext(t.Context(), `SELECT pg_advisory_unlock(hashtext($1))`, settleLockKey(projID))
	require.NoError(t, err)

	require.NotNil(t, s.settleAfterRun(t.Context(), projID, "main"), "the pass runs once the project is free")
}

func TestReviewPlan_SettleByHandIsRecorded(t *testing.T) {
	s, wsID, ownerID := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	rec := planWriteAs(t, s, s.HandleSettleReviewPlan, wsID, projID, `{}`, ownerID, platauth.PermAll)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	last := readPlanAs(t, s, wsID, projID).Policy.Last
	require.NotNil(t, last)
	assert.False(t, last.Automatic)
	assert.Equal(t, ownerID, last.By)
}
