package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestServerWithStores(t *testing.T) *Server {
	t.Helper()
	cfg := DefaultConfig()
	cfg.JWTSecret = "test-secret"
	srv := shutdownOnCleanup(t, NewServer(cfg))
	initTestStores(t, srv)

	// Wire up activity and task stores using the same PostgreSQL DB as ContentStore.
	pgStore := srv.ContentStore.(*bstore.PostgresStore)
	db := pgStore.SQLDB()
	srv.ActivityStore = bstore.NewActivityStore(db)
	srv.TaskStore = bstore.NewTaskStore(db)
	srv.PreferenceStore = bstore.NewPreferenceStore(db)

	return srv
}

func TestHandleListActivities_Empty(t *testing.T) {
	srv := setupTestServerWithStores(t)
	e := srv.GetEcho()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/demo/activities", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	// Set route params via echo context.
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")

	err := srv.HandleListActivities(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp bstore.ActivityResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Empty(t, resp.Activities)
}

func TestHandleListActivities_WithData(t *testing.T) {
	srv := setupTestServerWithStores(t)
	ctx := t.Context()

	// Seed an activity.
	a := &bstore.Activity{
		WorkspaceID: "demo",
		ProjectID:   "proj-1",
		ActorID:     "user-1",
		ActorName:   "Alice",
		Type:        bstore.ActivityProjectCreated,
		Summary:     "created project",
	}
	require.NoError(t, srv.ActivityStore.Create(ctx, a))

	e := srv.GetEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/demo/activities?project_id=proj-1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")

	err := srv.HandleListActivities(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp bstore.ActivityResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Activities, 1)
	assert.Equal(t, "Alice", resp.Activities[0].ActorName)
}

func TestHandleCreateTask(t *testing.T) {
	srv := setupTestServerWithStores(t)
	e := srv.GetEcho()

	body := `{"project_id":"proj-1","type":"translate","title":"Translate docs","priority":"high"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/demo/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")
	c.Set("user_id", "user-1")

	err := srv.HandleCreateTask(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, rec.Code)

	var task bstore.Task
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &task))
	assert.Equal(t, "Translate docs", task.Title)
	assert.Equal(t, bstore.TaskPriorityHigh, task.Priority)
	assert.Equal(t, bstore.TaskStatusOpen, task.Status)
	assert.NotEmpty(t, task.ID)
}

func TestHandleCreateTask_ValidationErrors(t *testing.T) {
	srv := setupTestServerWithStores(t)
	e := srv.GetEcho()

	t.Run("missing title", func(t *testing.T) {
		body := `{"project_id":"proj-1","type":"translate"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/demo/tasks", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("ws")
		c.SetParamValues("demo")

		err := srv.HandleCreateTask(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("missing project_id", func(t *testing.T) {
		body := `{"title":"Test","type":"translate"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/demo/tasks", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("ws")
		c.SetParamValues("demo")

		err := srv.HandleCreateTask(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestHandleGetTask(t *testing.T) {
	srv := setupTestServerWithStores(t)
	ctx := t.Context()

	task := &bstore.Task{
		WorkspaceID: "demo",
		ProjectID:   "proj-1",
		Type:        bstore.TaskReview,
		Title:       "Review blocks",
		CreatedBy:   "user-1",
	}
	require.NoError(t, srv.TaskStore.Create(ctx, task))

	e := srv.GetEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/demo/tasks/"+task.ID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws", "taskId")
	c.SetParamValues("demo", task.ID)

	err := srv.HandleGetTask(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var got bstore.Task
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "Review blocks", got.Title)
}

func TestHandleCompleteTask(t *testing.T) {
	srv := setupTestServerWithStores(t)
	ctx := t.Context()

	task := &bstore.Task{
		WorkspaceID: "demo",
		ProjectID:   "proj-1",
		Type:        bstore.TaskTranslate,
		Title:       "Complete me",
		CreatedBy:   "user-1",
	}
	require.NoError(t, srv.TaskStore.Create(ctx, task))

	e := srv.GetEcho()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/demo/tasks/"+task.ID+"/complete", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws", "taskId")
	c.SetParamValues("demo", task.ID)
	c.Set("user_id", "user-2")

	err := srv.HandleCompleteTask(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify task is completed.
	got, err := srv.TaskStore.Get(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.TaskStatusCompleted, got.Status)
	assert.Equal(t, "user-2", got.CompletedBy)
}

func TestHandleCancelTask(t *testing.T) {
	srv := setupTestServerWithStores(t)
	ctx := t.Context()

	task := &bstore.Task{
		WorkspaceID: "demo",
		ProjectID:   "proj-1",
		Type:        bstore.TaskCustom,
		Title:       "Cancel me",
		CreatedBy:   "user-1",
	}
	require.NoError(t, srv.TaskStore.Create(ctx, task))

	e := srv.GetEcho()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/demo/tasks/"+task.ID+"/cancel", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws", "taskId")
	c.SetParamValues("demo", task.ID)

	err := srv.HandleCancelTask(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	got, err := srv.TaskStore.Get(ctx, task.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.TaskStatusCancelled, got.Status)
}

func TestHandleNotificationPreferences_RoundTrip(t *testing.T) {
	srv := setupTestServerWithStores(t)
	e := srv.GetEcho()

	// Get defaults.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/demo/notification-preferences", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues("demo")
	c.Set("user_id", "user-1")

	err := srv.HandleGetNotificationPreferences(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var getResp struct {
		Preferences []bstore.NotificationPreference `json:"preferences"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
	assert.Len(t, getResp.Preferences, 7)

	// Update preferences.
	updateBody := `{"preferences":[{"category":"task","channels":{"web":true,"email":true,"push":false,"desktop":false}}]}`
	req2 := httptest.NewRequest(http.MethodPut, "/api/v1/demo/notification-preferences", strings.NewReader(updateBody))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("ws")
	c2.SetParamValues("demo")
	c2.Set("user_id", "user-1")

	err = srv.HandleUpdateNotificationPreferences(c2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec2.Code)
}

// TestHandleListTasks_AssigneeMe pins the "my tasks" surface: /:ws/tasks?
// assignee_id=me resolves to the authenticated user (the dedicated /my/tasks
// route was folded into this filter, Bowrain AD-011). Before the fix the
// literal "me" reached the store and matched nothing — and the web client
// still calling the retired /my/tasks path got the catch-all 404 on fresh
// workspaces (production drill follow-up).
func TestHandleListTasks_AssigneeMe(t *testing.T) {
	srv := setupTestServerWithStores(t)
	ctx := t.Context()

	mine := &bstore.Task{
		WorkspaceID: "demo", ProjectID: "proj-1", Type: bstore.TaskReview,
		Title: "Mine", CreatedBy: "user-2", AssigneeID: "user-1",
	}
	theirs := &bstore.Task{
		WorkspaceID: "demo", ProjectID: "proj-1", Type: bstore.TaskReview,
		Title: "Theirs", CreatedBy: "user-2", AssigneeID: "user-2",
	}
	require.NoError(t, srv.TaskStore.Create(ctx, mine))
	require.NoError(t, srv.TaskStore.Create(ctx, theirs))

	e := srv.GetEcho()
	list := func(query string, userID string) bstore.TaskResult {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/demo/tasks"+query, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("ws")
		c.SetParamValues("demo")
		if userID != "" {
			c.Set("user_id", userID)
		}
		require.NoError(t, srv.HandleListTasks(c))
		require.Equal(t, http.StatusOK, rec.Code)
		var resp bstore.TaskResult
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		return resp
	}

	// assignee_id=me → only the caller's tasks.
	resp := list("?assignee_id=me", "user-1")
	require.Len(t, resp.Tasks, 1)
	assert.Equal(t, "Mine", resp.Tasks[0].Title)

	// A literal user id keeps working.
	resp = list("?assignee_id=user-2", "user-1")
	require.Len(t, resp.Tasks, 1)
	assert.Equal(t, "Theirs", resp.Tasks[0].Title)

	// "me" with no authenticated user resolves to an empty list — never the
	// whole workspace.
	resp = list("?assignee_id=me", "")
	assert.Empty(t, resp.Tasks)
}

// seedReviewTasks fills a workspace with the type/status spread the dashboard
// and the review inbox filter over.
func seedReviewTasks(t *testing.T, srv *Server) {
	t.Helper()
	ctx := t.Context()
	seed := []*bstore.Task{
		{WorkspaceID: "demo", ProjectID: "proj-1", Type: bstore.TaskReview, Title: "review-open", CreatedBy: "u0"},
		{WorkspaceID: "demo", ProjectID: "proj-1", Type: bstore.TaskReviewTerms, Title: "terms-open", CreatedBy: "u0"},
		{WorkspaceID: "demo", ProjectID: "proj-1", Type: bstore.TaskSourceReview, Title: "source-progress", CreatedBy: "u0"},
		{WorkspaceID: "demo", ProjectID: "proj-1", Type: bstore.TaskTranslate, Title: "translate-open", CreatedBy: "u0"},
		{WorkspaceID: "demo", ProjectID: "proj-2", Type: bstore.TaskReview, Title: "other-project", CreatedBy: "u0"},
	}
	for _, task := range seed {
		require.NoError(t, srv.TaskStore.Create(ctx, task))
	}
	require.NoError(t, srv.TaskStore.Assign(ctx, seed[2].ID, "user-1"))
}

// The review inbox and the dashboard filter over review|review_terms|
// source_review; ?types answers that server-side instead of over a page the
// client already downloaded.
func TestHandleListTasks_TypesFilter(t *testing.T) {
	srv := setupTestServerWithStores(t)
	seedReviewTasks(t, srv)

	e := srv.GetEcho()
	list := func(query string) []string {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/demo/tasks"+query, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("ws")
		c.SetParamValues("demo")
		require.NoError(t, srv.HandleListTasks(c))
		require.Equal(t, http.StatusOK, rec.Code)
		var resp bstore.TaskResult
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		titles := make([]string, 0, len(resp.Tasks))
		for _, task := range resp.Tasks {
			titles = append(titles, task.Title)
		}
		return titles
	}

	assert.ElementsMatch(t,
		[]string{"review-open", "terms-open", "source-progress", "other-project"},
		list("?types=review,review_terms,source_review"))

	assert.ElementsMatch(t, []string{"review-open", "other-project"}, list("?types=review"))
	assert.ElementsMatch(t, []string{"review-open"}, list("?types=review&project_id=proj-1"))
	assert.ElementsMatch(t, []string{"translate-open"}, list("?type=translate"))

	// Whitespace and empty elements are tolerated rather than reaching SQL.
	assert.ElementsMatch(t, []string{"review-open", "other-project"}, list("?types=+review+,"))
}

// The kanban column headers count the whole column, not the page loaded.
func TestHandleGetTaskCounts(t *testing.T) {
	srv := setupTestServerWithStores(t)
	seedReviewTasks(t, srv)

	e := srv.GetEcho()
	counts := func(query, userID string) bstore.TaskCounts {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/demo/tasks/counts"+query, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("ws")
		c.SetParamValues("demo")
		if userID != "" {
			c.Set("user_id", userID)
		}
		require.NoError(t, srv.HandleGetTaskCounts(c))
		require.Equal(t, http.StatusOK, rec.Code)
		var resp bstore.TaskCounts
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		return resp
	}

	t.Run("whole workspace", func(t *testing.T) {
		got := counts("", "")
		assert.Equal(t, 5, got.Total)
		assert.Equal(t, 4, got.ByStatus[string(bstore.TaskStatusOpen)])
		assert.Equal(t, 1, got.ByStatus[string(bstore.TaskStatusInProgress)])
		assert.Equal(t, 0, got.ByStatus[string(bstore.TaskStatusCompleted)])
	})

	t.Run("same filter vocabulary as the list", func(t *testing.T) {
		got := counts("?types=review,review_terms,source_review&project_id=proj-1", "")
		assert.Equal(t, 3, got.Total)
		assert.Equal(t, 2, got.ByStatus[string(bstore.TaskStatusOpen)])
		assert.Equal(t, 1, got.ByStatus[string(bstore.TaskStatusInProgress)])
	})

	t.Run("assignee_id=me resolves to the caller", func(t *testing.T) {
		got := counts("?assignee_id=me", "user-1")
		assert.Equal(t, 1, got.Total)
		assert.Equal(t, 1, got.ByStatus[string(bstore.TaskStatusInProgress)])
	})

	t.Run("unauthenticated me counts nothing, never the workspace", func(t *testing.T) {
		assert.Equal(t, 0, counts("?assignee_id=me", "").Total)
	})
}

// ?types ORs one prefix per entry, so a feed can span families no single
// prefix expresses.
func TestHandleListActivities_TypesFilter(t *testing.T) {
	srv := setupTestServerWithStores(t)
	ctx := t.Context()

	for _, typ := range []bstore.ActivityType{
		bstore.ActivityReviewAssigned, bstore.ActivityReviewDecided,
		bstore.ActivityTaskCreated, bstore.ActivityBlockTranslated,
	} {
		require.NoError(t, srv.ActivityStore.Create(ctx, &bstore.Activity{
			WorkspaceID: "demo", ActorID: "user-1", Type: typ, Summary: string(typ),
		}))
	}

	e := srv.GetEcho()
	list := func(query string) []string {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/demo/activities"+query, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("ws")
		c.SetParamValues("demo")
		require.NoError(t, srv.HandleListActivities(c))
		require.Equal(t, http.StatusOK, rec.Code)
		var resp bstore.ActivityResult
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		types := make([]string, 0, len(resp.Activities))
		for _, a := range resp.Activities {
			types = append(types, string(a.Type))
		}
		return types
	}

	assert.ElementsMatch(t,
		[]string{"review.assigned", "review.decided", "task.created"},
		list("?types=review,task"))
	assert.ElementsMatch(t, []string{"review.assigned", "review.decided"}, list("?type=review"))
	assert.ElementsMatch(t, []string{"review.decided"}, list("?types=review.decided"))
	assert.Len(t, list(""), 4)
}
