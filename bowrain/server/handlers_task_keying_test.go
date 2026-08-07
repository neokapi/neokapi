package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
)

// TestListTasksIsKeyedByWorkspaceID pins the keying of the workspace task queue.
//
// Every server-side task writer keys on the workspace **id** — createSourceProposalTask
// and the automation review fan-out both pass proj.WorkspaceID. A queue handler
// reading c.Param("ws"), the **slug**, matches none of them, and a workspace
// with open, system-created review work answers /:ws/tasks with an empty list.
func TestListTasksIsKeyedByWorkspaceID(t *testing.T) {
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "tasks.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	srv := &Server{TaskStore: bstore.NewTaskStore(cs.DB())}
	const (
		wsID   = "ws-01HXYZ"
		wsSlug = "bowrain-hq"
	)
	// A task written the way the server writes them: keyed by workspace id.
	require.NoError(t, srv.TaskStore.Create(context.Background(), &bstore.Task{
		WorkspaceID: wsID,
		Type:        bstore.TaskReviewTerms,
		Status:      bstore.TaskStatusOpen,
		Title:       "Review a governed change",
		CreatedBy:   "system",
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/"+wsSlug+"/tasks", nil)
	rec := httptest.NewRecorder()
	c := srv.GetEcho().NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues(wsSlug)
	c.Set("workspace_id", wsID)

	require.NoError(t, srv.HandleListTasks(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var result bstore.TaskResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	require.Len(t, result.Tasks, 1, "a task written by the server must be visible in its workspace queue")
	assert.Equal(t, "Review a governed change", result.Tasks[0].Title)
}

// TestCreateTaskWritesWorkspaceID pins the other half: a task created through the
// API lands under the same key the queue reads, so the two cannot drift apart
// again.
func TestCreateTaskWritesWorkspaceID(t *testing.T) {
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "tasks.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	srv := &Server{TaskStore: bstore.NewTaskStore(cs.DB())}
	const (
		wsID   = "ws-01HXYZ"
		wsSlug = "bowrain-hq"
	)

	body := `{"project_id":"proj-1","type":"review","title":"Read this"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/"+wsSlug+"/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := srv.GetEcho().NewContext(req, rec)
	c.SetParamNames("ws")
	c.SetParamValues(wsSlug)
	c.Set("workspace_id", wsID)
	c.Set("user_id", "user-1")

	require.NoError(t, srv.HandleCreateTask(c))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var task bstore.Task
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &task))
	assert.Equal(t, wsID, task.WorkspaceID, "the queue is keyed by id, so the API writes the id")
}
