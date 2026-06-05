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
	srv := NewServer(cfg)
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
