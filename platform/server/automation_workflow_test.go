package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
	platauth "github.com/neokapi/neokapi/platform/auth"
	platev "github.com/neokapi/neokapi/platform/event"
	platstore "github.com/neokapi/neokapi/platform/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newWorkflowTestServer creates a server with auth, event bus, task store,
// and a project with members assigned to specific locales.
func newWorkflowTestServer(t *testing.T) (*Server, string, string) {
	t.Helper()
	cfg := DefaultServerConfig()
	cfg.JWTSecret = "test-workflow"
	srv := NewServer(cfg)
	initTestStores(t, srv)

	ctx := context.Background()

	// Create event bus + task store.
	bus := event.NewChannelEventBus()
	t.Cleanup(func() { bus.Close() })
	srv.EventBus = bus

	sqliteStore := srv.ContentStore.(*bstore.SQLiteStore)
	srv.TaskStore = bstore.NewTaskStore(sqliteStore.DB())

	// Create user and workspace.
	user := &platauth.User{ID: "admin-1", Email: "admin@test.com", Name: "Admin"}
	require.NoError(t, srv.AuthStore.CreateUser(ctx, user))

	ws := &platauth.Workspace{ID: "ws-1", Name: "Test WS", Slug: "test-ws"}
	require.NoError(t, srv.AuthStore.CreateWorkspace(ctx, ws))
	require.NoError(t, srv.AuthStore.AddMember(ctx, ws.ID, user.ID, platauth.RoleOwner))
	require.NoError(t, srv.AuthStore.SeedDefaultRoleTemplates(ctx, ws.ID))

	token, err := platauth.GenerateToken(user, cfg.JWTSecret, 1*time.Hour)
	require.NoError(t, err)

	return srv, token, ws.ID
}

// createWorkflowProject creates a project with workflow_enabled and target languages.
func createWorkflowProject(t *testing.T, srv *Server, wsID string, props map[string]string) string {
	t.Helper()
	ctx := context.Background()

	proj := &platstore.Project{
		Name:                  "Workflow Test",
		WorkspaceID:           wsID,
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr-FR", "de-DE"},
		Properties:            props,
	}
	require.NoError(t, srv.ContentStore.CreateProject(ctx, proj))
	return proj.ID
}

// addProjectMember adds a user as a project member with a given role and languages.
func addProjectMember(t *testing.T, srv *Server, wsID, projectID, roleName string, languages []string) string {
	t.Helper()
	ctx := context.Background()

	// Create user.
	langSuffix := "all"
	if len(languages) > 0 {
		langSuffix = languages[0]
	}
	userID := "user-" + roleName + "-" + langSuffix
	user := &platauth.User{ID: userID, Email: userID + "@test.com", Name: roleName}
	require.NoError(t, srv.AuthStore.CreateUser(ctx, user))
	require.NoError(t, srv.AuthStore.AddMember(ctx, wsID, userID, platauth.RoleMember))

	// Find role template.
	templates, err := srv.AuthStore.ListRoleTemplates(ctx, wsID)
	require.NoError(t, err)
	var roleID string
	for _, rt := range templates {
		if rt.Name == roleName {
			roleID = rt.ID
			break
		}
	}
	require.NotEmpty(t, roleID, "role template %q not found", roleName)

	pm := &platauth.ProjectMembership{
		ProjectID:   projectID,
		UserID:      userID,
		RoleID:      roleID,
		WorkspaceID: wsID,
		Languages:   languages,
	}
	require.NoError(t, srv.AuthStore.AddProjectMember(ctx, pm))
	return userID
}

func TestCreateReviewTasks_FanOutPerLocale(t *testing.T) {
	srv, _, wsID := newWorkflowTestServer(t)

	projID := createWorkflowProject(t, srv, wsID, map[string]string{
		"workflow_enabled": "true",
	})

	frUser := addProjectMember(t, srv, wsID, projID, "reviewer", []string{"fr-FR"})
	deUser := addProjectMember(t, srv, wsID, projID, "reviewer", []string{"de-DE"})

	// Execute the action.
	action := event.AutomationAction{
		Type:   "create_review_tasks",
		Config: map[string]string{"mode": "review"},
	}
	ev := platev.Event{
		ProjectID: projID,
		Data: map[string]string{
			"push_id": "push-test",
			"items":   "en.json",
		},
	}
	srv.createReviewTasks(context.Background(), action, ev, "")

	// Verify tasks were created.
	ctx := context.Background()
	res, err := srv.TaskStore.List(ctx, bstore.TaskQuery{
		WorkspaceID: wsID,
		ProjectID:   projID,
	})
	require.NoError(t, err)
	tasks := res.Tasks
	require.Len(t, tasks, 2, "expected one task per locale")

	// Check locale assignment.
	localeAssignees := map[string]string{}
	for _, task := range tasks {
		assert.Equal(t, bstore.TaskReview, task.Type)
		assert.Equal(t, bstore.TaskStatusOpen, task.Status)
		assert.Equal(t, "push-test", task.Data["push_id"])
		localeAssignees[task.Data["locale"]] = task.AssigneeID
	}
	assert.Equal(t, frUser, localeAssignees["fr-FR"])
	assert.Equal(t, deUser, localeAssignees["de-DE"])
}

func TestCreateReviewTasks_TranslateMode(t *testing.T) {
	srv, _, wsID := newWorkflowTestServer(t)

	projID := createWorkflowProject(t, srv, wsID, map[string]string{
		"workflow_enabled": "true",
	})
	addProjectMember(t, srv, wsID, projID, "translator", []string{"fr-FR"})

	action := event.AutomationAction{
		Type:   "create_review_tasks",
		Config: map[string]string{"mode": "translate"},
	}
	ev := platev.Event{
		ProjectID: projID,
		Data:      map[string]string{"push_id": "p1", "items": "en.json"},
	}
	srv.createReviewTasks(context.Background(), action, ev, "")

	res, err := srv.TaskStore.List(context.Background(), bstore.TaskQuery{
		WorkspaceID: wsID,
		ProjectID:   projID,
	})
	require.NoError(t, err)

	// fr-FR gets a translate task (translator has PermTranslate).
	// de-DE gets an unassigned task (no member).
	frTasks := filterTasksByLocale(res.Tasks, "fr-FR")
	require.Len(t, frTasks, 1)
	assert.Equal(t, bstore.TaskTranslate, frTasks[0].Type)
	assert.NotEmpty(t, frTasks[0].AssigneeID)

	deTasks := filterTasksByLocale(res.Tasks, "de-DE")
	require.Len(t, deTasks, 1)
	assert.Empty(t, deTasks[0].AssigneeID, "de-DE should be unassigned")
}

func TestCreateReviewTasks_SkipsWhenWorkflowDisabled(t *testing.T) {
	srv, _, wsID := newWorkflowTestServer(t)

	projID := createWorkflowProject(t, srv, wsID, nil) // no workflow_enabled
	addProjectMember(t, srv, wsID, projID, "reviewer", []string{"fr-FR"})

	action := event.AutomationAction{Type: "create_review_tasks"}
	ev := platev.Event{
		ProjectID: projID,
		Data:      map[string]string{"push_id": "p1", "items": "en.json"},
	}
	srv.createReviewTasks(context.Background(), action, ev, "")

	res, err := srv.TaskStore.List(context.Background(), bstore.TaskQuery{
		WorkspaceID: wsID,
		ProjectID:   projID,
	})
	require.NoError(t, err)
	assert.Empty(t, res.Tasks, "no tasks should be created when workflow is disabled")
}

func TestCreateSourceReviewTask(t *testing.T) {
	srv, _, wsID := newWorkflowTestServer(t)

	projID := createWorkflowProject(t, srv, wsID, map[string]string{
		"workflow_enabled": "true",
	})
	reviewerID := addProjectMember(t, srv, wsID, projID, "developer", []string{})

	action := event.AutomationAction{
		Type:   "create_source_review",
		Config: map[string]string{"reviewer": reviewerID},
	}
	ev := platev.Event{
		ProjectID: projID,
		Data: map[string]string{
			"push_id":        "push-src",
			"items":          "en.json",
			"workspace_slug": "test-ws",
		},
	}
	srv.createSourceReviewTask(context.Background(), action, ev, "")

	res, err := srv.TaskStore.List(context.Background(), bstore.TaskQuery{
		WorkspaceID: wsID,
		ProjectID:   projID,
	})
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)

	task := res.Tasks[0]
	assert.Equal(t, bstore.TaskSourceReview, task.Type)
	assert.Equal(t, reviewerID, task.AssigneeID)
	assert.Equal(t, "push-src", task.Data["push_id"])
}

func TestSourceReviewCompletionEmitsEvent(t *testing.T) {
	srv, token, wsID := newWorkflowTestServer(t)

	projID := createWorkflowProject(t, srv, wsID, map[string]string{
		"workflow_enabled": "true",
	})

	// Create a source review task.
	ctx := context.Background()
	task := &bstore.Task{
		WorkspaceID: wsID,
		ProjectID:   projID,
		Type:        bstore.TaskSourceReview,
		Status:      bstore.TaskStatusOpen,
		Priority:    bstore.TaskPriorityNormal,
		Title:       "Review source",
		AssigneeID:  "admin-1",
		CreatedBy:   "system",
		Data:        map[string]string{"push_id": "push-src", "items": "en.json"},
	}
	require.NoError(t, srv.TaskStore.Create(ctx, task))

	// Subscribe to the event.
	var received []platev.Event
	srv.EventBus.Subscribe(platev.EventSourceReviewCompleted, func(ev platev.Event) {
		received = append(received, ev)
	})

	// Complete the task via API.
	e := srv.GetEcho()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/workspaces/test-ws/tasks/"+task.ID+"/complete", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Wait for async event delivery.
	time.Sleep(200 * time.Millisecond)

	require.Len(t, received, 1, "should emit source.review.completed")
	assert.Equal(t, projID, received[0].ProjectID)
	assert.Equal(t, "push-src", received[0].Data["push_id"])
}

func TestFindMembersForLocale(t *testing.T) {
	srv, _, wsID := newWorkflowTestServer(t)
	ctx := context.Background()

	projID := createWorkflowProject(t, srv, wsID, map[string]string{
		"workflow_enabled": "true",
	})

	frReviewer := addProjectMember(t, srv, wsID, projID, "reviewer", []string{"fr-FR"})
	addProjectMember(t, srv, wsID, projID, "translator", []string{"de-DE"})
	allLangsReviewer := addProjectMember(t, srv, wsID, projID, "reviewer", []string{}) // all languages

	members, err := srv.AuthStore.ListProjectMembers(ctx, projID)
	require.NoError(t, err)

	// Review mode: find reviewers for fr-FR.
	result := srv.findMembersForLocale(ctx, members, "fr-FR", "review")
	userIDs := extractUserIDs(result)
	assert.Contains(t, userIDs, frReviewer)
	assert.Contains(t, userIDs, allLangsReviewer)
	assert.Len(t, result, 2)

	// Translate mode: find translators for de-DE.
	// de-DE translator + all-langs reviewer (reviewer role has PermTranslate too).
	result = srv.findMembersForLocale(ctx, members, "de-DE", "translate")
	assert.Len(t, result, 2)

	// Review mode for ja-JP: only all-langs reviewer matches.
	result = srv.findMembersForLocale(ctx, members, "ja-JP", "review")
	userIDs = extractUserIDs(result)
	assert.Contains(t, userIDs, allLangsReviewer)
	assert.Len(t, result, 1)
}

func filterTasksByLocale(tasks []bstore.Task, locale string) []bstore.Task {
	var result []bstore.Task
	for _, t := range tasks {
		if t.Data["locale"] == locale {
			result = append(result, t)
		}
	}
	return result
}

func extractUserIDs(members []*platauth.ProjectMembership) []string {
	ids := make([]string, len(members))
	for i, m := range members {
		ids[i] = m.UserID
	}
	return ids
}

// TestWorkflowEndToEnd tests the full chain:
// EventPushAutomationsCompleted → automation engine → create_review_tasks → tasks created
func TestWorkflowEndToEnd(t *testing.T) {
	srv, _, wsID := newWorkflowTestServer(t)
	ctx := context.Background()

	projID := createWorkflowProject(t, srv, wsID, map[string]string{
		"workflow_enabled": "true",
	})
	frUser := addProjectMember(t, srv, wsID, projID, "reviewer", []string{"fr-FR"})
	deUser := addProjectMember(t, srv, wsID, projID, "reviewer", []string{"de-DE"})

	// Wire up the automation engine via RunManager (AD-035).
	rm := event.NewAutomationRunManager(nil, srv.executeAutomationAction)
	engine := event.NewAutomationEngine(srv.EventBus, rm.Execute)
	defer engine.Close()

	// Register the built-in rule: push.automations.completed → create_review_tasks
	engine.AddRule(event.AutomationRule{
		Name:      "create-review-tasks",
		EventType: platev.EventPushAutomationsCompleted,
		Actions: []event.AutomationAction{
			{Type: "create_review_tasks", Config: map[string]string{"mode": "review"}},
		},
	})

	// Simulate: push automations completed (normally emitted by PushCompletionTracker).
	srv.EventBus.Publish(platev.Event{
		Type:      platev.EventPushAutomationsCompleted,
		Source:    "push_completion_tracker",
		ProjectID: projID,
		Actor:     "system",
		Data: map[string]string{
			"push_id":            "push-e2e",
			"items":              "en.json",
			"workspace_slug":     "test-ws",
			"translation_status": "all_completed",
			"extraction_status":  "none",
		},
	})

	// Wait for async automation execution.
	require.Eventually(t, func() bool {
		res, err := srv.TaskStore.List(ctx, bstore.TaskQuery{
			WorkspaceID: wsID,
			ProjectID:   projID,
		})
		return err == nil && len(res.Tasks) >= 2
	}, 3*time.Second, 100*time.Millisecond, "tasks should be created by automation")

	// Verify tasks.
	res, err := srv.TaskStore.List(ctx, bstore.TaskQuery{
		WorkspaceID: wsID,
		ProjectID:   projID,
	})
	require.NoError(t, err)

	localeAssignees := map[string]string{}
	for _, task := range res.Tasks {
		assert.Equal(t, bstore.TaskReview, task.Type)
		assert.Equal(t, "push-e2e", task.Data["push_id"])
		localeAssignees[task.Data["locale"]] = task.AssigneeID
	}
	assert.Equal(t, frUser, localeAssignees["fr-FR"])
	assert.Equal(t, deUser, localeAssignees["de-DE"])
}

// TestWorkflowEndToEnd_SourceReviewGate tests the full chain with source review:
// EventPushAutomationsCompleted → create_source_review → complete task →
// EventSourceReviewCompleted → create_review_tasks → tasks created
func TestWorkflowEndToEnd_SourceReviewGate(t *testing.T) {
	srv, token, wsID := newWorkflowTestServer(t)
	ctx := context.Background()

	projID := createWorkflowProject(t, srv, wsID, map[string]string{
		"workflow_enabled": "true",
	})
	addProjectMember(t, srv, wsID, projID, "reviewer", []string{"fr-FR"})
	addProjectMember(t, srv, wsID, projID, "reviewer", []string{"de-DE"})

	// Wire up the automation engine via RunManager.
	rm := event.NewAutomationRunManager(nil, srv.executeAutomationAction)
	engine := event.NewAutomationEngine(srv.EventBus, rm.Execute)
	defer engine.Close()

	// Rule 1: push.automations.completed → create_source_review
	engine.AddRule(event.AutomationRule{
		Name:      "source-review-gate",
		EventType: platev.EventPushAutomationsCompleted,
		Actions: []event.AutomationAction{
			{Type: "create_source_review", Config: map[string]string{"reviewer": "admin-1"}},
		},
	})

	// Rule 2: source.review.completed → create_review_tasks
	engine.AddRule(event.AutomationRule{
		Name:      "fan-out-after-review",
		EventType: platev.EventSourceReviewCompleted,
		Actions: []event.AutomationAction{
			{Type: "create_review_tasks", Config: map[string]string{"mode": "review"}},
		},
	})

	// Simulate: push automations completed.
	srv.EventBus.Publish(platev.Event{
		Type:      platev.EventPushAutomationsCompleted,
		Source:    "push_completion_tracker",
		ProjectID: projID,
		Data: map[string]string{
			"push_id":            "push-gate",
			"items":              "en.json",
			"workspace_slug":     "test-ws",
			"translation_status": "all_completed",
			"extraction_status":  "none",
		},
	})

	// Wait for source review task to be created.
	var sourceReviewTaskID string
	require.Eventually(t, func() bool {
		res, err := srv.TaskStore.List(ctx, bstore.TaskQuery{
			WorkspaceID: wsID,
			ProjectID:   projID,
			Type:        "source_review",
		})
		if err != nil || len(res.Tasks) == 0 {
			return false
		}
		sourceReviewTaskID = res.Tasks[0].ID
		return true
	}, 3*time.Second, 100*time.Millisecond, "source review task should be created")

	// At this point, no review tasks should exist yet (only source review).
	res, err := srv.TaskStore.List(ctx, bstore.TaskQuery{
		WorkspaceID: wsID,
		ProjectID:   projID,
		Type:        "review",
	})
	require.NoError(t, err)
	assert.Empty(t, res.Tasks, "no review tasks before source review completed")

	// Complete the source review task via API (triggers EventSourceReviewCompleted).
	e := srv.GetEcho()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/workspaces/test-ws/tasks/"+sourceReviewTaskID+"/complete", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Wait for review tasks to be created by the fan-out rule.
	require.Eventually(t, func() bool {
		res, err := srv.TaskStore.List(ctx, bstore.TaskQuery{
			WorkspaceID: wsID,
			ProjectID:   projID,
			Type:        "review",
		})
		return err == nil && len(res.Tasks) >= 2
	}, 3*time.Second, 100*time.Millisecond, "review tasks should be created after source review")

	// Verify review tasks were created for both locales.
	res, err = srv.TaskStore.List(ctx, bstore.TaskQuery{
		WorkspaceID: wsID,
		ProjectID:   projID,
		Type:        "review",
	})
	require.NoError(t, err)
	assert.Len(t, res.Tasks, 2)

	locales := map[string]bool{}
	for _, task := range res.Tasks {
		locales[task.Data["locale"]] = true
	}
	assert.True(t, locales["fr-FR"])
	assert.True(t, locales["de-DE"])
}

// TestWorkflowDeduplication verifies that duplicate tasks are not created.
func TestWorkflowDeduplication(t *testing.T) {
	srv, _, wsID := newWorkflowTestServer(t)
	ctx := context.Background()

	projID := createWorkflowProject(t, srv, wsID, map[string]string{
		"workflow_enabled": "true",
	})
	addProjectMember(t, srv, wsID, projID, "reviewer", []string{"fr-FR"})

	action := event.AutomationAction{
		Type:   "create_review_tasks",
		Config: map[string]string{"mode": "review"},
	}
	ev := platev.Event{
		ProjectID: projID,
		Data:      map[string]string{"push_id": "p1", "items": "en.json"},
	}

	// Create tasks twice.
	srv.createReviewTasks(ctx, action, ev, "")
	srv.createReviewTasks(ctx, action, ev, "")

	res, err := srv.TaskStore.List(ctx, bstore.TaskQuery{
		WorkspaceID: wsID,
		ProjectID:   projID,
		Type:        "review",
	})
	require.NoError(t, err)

	// fr-FR should have 1 task (dedup), de-DE should have 1 unassigned task (dedup).
	frTasks := filterTasksByLocale(res.Tasks, "fr-FR")
	assert.Len(t, frTasks, 1, "duplicate fr-FR tasks should be prevented")

	deTasks := filterTasksByLocale(res.Tasks, "de-DE")
	assert.Len(t, deTasks, 1, "duplicate de-DE tasks should be prevented")
}
