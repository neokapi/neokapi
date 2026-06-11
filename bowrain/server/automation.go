package server

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/bowrain/jobs"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	coreblockstore "github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/id"
)

// registerDefaultAutomations registers the built-in automation rules.
func (s *Server) registerDefaultAutomations() {
	if s.AutomationEngine == nil {
		return
	}

	// Rule 1: Auto-translate on push.
	s.AutomationEngine.AddRule(event.AutomationRule{
		Name:      "auto-translate-on-push",
		EventType: platev.EventPushCompleted,
		Actions: []event.AutomationAction{
			{Type: "auto_translate"},
		},
	})

	// Rule 2: Auto-extract entities and terms on push.
	s.AutomationEngine.AddRule(event.AutomationRule{
		Name:      "auto-extract-on-push",
		EventType: platev.EventPushCompleted,
		Actions: []event.AutomationAction{
			{Type: "auto_extract"},
		},
	})

	// Rule 3: Auto-translate when new locales are added.
	s.AutomationEngine.AddRule(event.AutomationRule{
		Name:      "auto-translate-new-locale",
		EventType: platev.EventProjectUpdated,
		Conditions: []event.AutomationCondition{
			{Field: "new_locales", Operator: "exists"},
		},
		Actions: []event.AutomationAction{
			{Type: "auto_translate_new_locale"},
		},
	})

	// Rule 4: Create review tasks when automations complete (Bowrain AD-014).
	// This rule is registered but only fires for projects that have it enabled
	// via stored rules. The built-in version serves as a template.
	s.AutomationEngine.AddRule(event.AutomationRule{
		Name:      "create-review-tasks-on-automation-complete",
		EventType: platev.EventPushAutomationsCompleted,
		Actions: []event.AutomationAction{
			{Type: "create_review_tasks", Config: map[string]string{"mode": "review"}},
		},
	})

	// Rule 5: Fan out review tasks after source review (Bowrain AD-014).
	s.AutomationEngine.AddRule(event.AutomationRule{
		Name:      "fan-out-after-source-review",
		EventType: platev.EventSourceReviewCompleted,
		Actions: []event.AutomationAction{
			{Type: "create_review_tasks", Config: map[string]string{"mode": "review"}},
		},
	})

	// Load user-defined rules from database.
	if s.AutomationRuleStore != nil {
		s.loadStoredAutomationRules()
	}
}

// loadStoredAutomationRules loads all enabled user-defined rules from the database
// and registers them with the automation engine.
func (s *Server) loadStoredAutomationRules() {
	if s.AutomationRuleStore == nil || s.ContentStore == nil {
		return
	}

	ctx := context.Background()
	projects, err := s.ContentStore.ListProjects(ctx)
	if err != nil {
		slog.Warn("failed to list projects for automation rule loading", "error", err)
		return
	}

	loaded := 0
	for _, proj := range projects {
		rules, err := s.AutomationRuleStore.ListRules(ctx, proj.ID)
		if err != nil {
			slog.Warn("failed to load automation rules for project", "id", proj.ID, "error", err)
			continue
		}
		for _, r := range rules {
			if !r.Enabled {
				continue
			}
			s.AutomationEngine.AddRule(event.AutomationRule{
				Name:       r.Name,
				EventType:  r.Trigger,
				Conditions: r.Conditions,
				Actions:    r.Actions,
			})
			loaded++
		}
	}
	if loaded > 0 {
		slog.Info("loaded user-defined automation rules", "count", loaded)
	}
}

// executeAutomationAction is the callback for the automation engine (via RunManager).
func (s *Server) executeAutomationAction(action event.AutomationAction, ev platev.Event, stepID string) error {
	startedAt := ev.Timestamp
	err := s.doExecuteAction(action, ev, stepID)

	// Record execution in history.
	if s.AutomationRuleStore != nil {
		status := "success"
		errMsg := ""
		if err != nil {
			status = "failed"
			errMsg = err.Error()
		}
		_ = s.AutomationRuleStore.RecordExecution(context.Background(), &event.HistoryEntry{
			ID:        id.New(),
			ProjectID: ev.ProjectID,
			EventID:   ev.ID,
			Status:    status,
			Error:     errMsg,
			StartedAt: startedAt,
			EndedAt:   ev.Timestamp,
		})
	}

	return err
}

func (s *Server) doExecuteAction(action event.AutomationAction, ev platev.Event, stepID string) error {
	// Automation actions run in background goroutines and must not inherit
	// the triggering event's cancellation. Use a fresh context with a timeout
	// so actions are bounded but survive request/event lifecycle.
	actionCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

	switch action.Type {
	case "auto_translate":
		items := ev.Data["items"]
		pushID := ev.Data["push_id"]
		wsSlug := ev.Data["workspace_slug"]
		if items == "" || pushID == "" {
			cancel()
			return nil
		}
		itemNames := strings.Split(items, ",")
		go func() {
			defer cancel()
			s.triggerAutoTranslate(actionCtx, ev.ProjectID, itemNames, nil, pushID, wsSlug, stepID)
		}()

	case "auto_extract":
		items := ev.Data["items"]
		pushID := ev.Data["push_id"]
		wsSlug := ev.Data["workspace_slug"]
		if items == "" || pushID == "" {
			cancel()
			return nil
		}
		itemNames := strings.Split(items, ",")
		go func() {
			defer cancel()
			s.triggerAutoExtract(actionCtx, ev.ProjectID, itemNames, pushID, wsSlug, stepID)
		}()

	case "notify":
		cancel()
		s.executeNotifyAction(action, ev)

	case "auto_translate_new_locale":
		newLocales := ev.Data["new_locales"]
		wsSlug := ev.Data["workspace_slug"]
		if newLocales == "" {
			cancel()
			return nil
		}
		locales := strings.Split(newLocales, ",")
		go func() {
			defer cancel()
			s.triggerAutoTranslateNewLocales(actionCtx, ev.ProjectID, locales, wsSlug)
		}()

	case "create_review_tasks":
		go func() {
			defer cancel()
			s.createReviewTasks(actionCtx, action, ev, stepID)
		}()

	case "create_source_review":
		go func() {
			defer cancel()
			s.createSourceReviewTask(actionCtx, action, ev, stepID)
		}()

	case "write_overlay":
		go func() {
			defer cancel()
			s.executeWriteOverlay(actionCtx, action, ev, stepID)
		}()

	default:
		cancel()
	}
	return nil
}

// executeWriteOverlay persists an overlay (targets / annotations / plugin
// kinds) against one or more blocks through the in-process blockstore
// adapter (#385 foundation). Config keys:
//
//	kind      — required, e.g. "annotations/qa" or "targets/fr"
//	payload   — required, JSON object written verbatim to the overlay
//	stream    — optional, defaults to "main"
//	block     — optional explicit block id; falls back to ev.Data["block_id"]
//
// This is the reference automation action that exercises the adapter
// end-to-end: no HTTP round-trip, AutomationRun log entries match the
// CLI flow-run UI shape.
func (s *Server) executeWriteOverlay(ctx context.Context, action event.AutomationAction, ev platev.Event, stepID string) {
	kind := action.Config["kind"]
	payload := action.Config["payload"]
	if kind == "" || payload == "" {
		s.appendAutomationLog(ctx, stepID, "error", "write_overlay: missing kind or payload", nil)
		return
	}
	stream := action.Config["stream"]
	if stream == "" {
		stream = "main"
	}
	blockID := action.Config["block"]
	if blockID == "" {
		blockID = ev.Data["block_id"]
	}
	if blockID == "" {
		s.appendAutomationLog(ctx, stepID, "error", "write_overlay: no block id (check action config or event data)", nil)
		return
	}

	bs, err := s.OpenBlockstore(ev.ProjectID, stream)
	if err != nil {
		s.appendAutomationLog(ctx, stepID, "error", "write_overlay: open blockstore: "+err.Error(), nil)
		return
	}
	defer func() { _ = bs.Close() }()

	sess, err := bs.Begin(ctx)
	if err != nil {
		s.appendAutomationLog(ctx, stepID, "error", "write_overlay: begin session: "+err.Error(), nil)
		return
	}
	defer func() { _ = sess.Close() }()

	if err := sess.PutOverlay(coreblockstore.Overlay{
		Kind:      kind,
		BlockHash: blockID,
		Payload:   []byte(payload),
	}); err != nil {
		s.appendAutomationLog(ctx, stepID, "error", "write_overlay: put overlay: "+err.Error(), nil)
		_ = sess.Rollback()
		return
	}
	if err := sess.Commit(); err != nil {
		s.appendAutomationLog(ctx, stepID, "error", "write_overlay: commit: "+err.Error(), nil)
		return
	}
	s.appendAutomationLog(ctx, stepID, "info", "write_overlay: wrote "+kind+" on "+blockID, map[string]string{
		"kind": kind, "block": blockID, "stream": stream,
	})
}

// appendAutomationLog records one AutomationLog entry against a step
// when the AutomationRunStore is wired up. Safe no-op otherwise.
func (s *Server) appendAutomationLog(ctx context.Context, stepID, level, message string, data map[string]string) {
	if s.AutomationRunStore == nil || stepID == "" {
		return
	}
	step, err := s.AutomationRunStore.GetStep(ctx, stepID)
	runID := ""
	if err == nil && step != nil {
		runID = step.RunID
	}
	_ = s.AutomationRunStore.AppendLogs(ctx, []bstore.AutomationLog{{
		ID:        id.New(),
		StepID:    stepID,
		RunID:     runID,
		Level:     level,
		Message:   message,
		Data:      data,
		Timestamp: time.Now().UTC(),
	}})
}

// triggerAutoTranslate creates translation jobs for each (item, locale) pair.
func (s *Server) triggerAutoTranslate(ctx context.Context, projectID string, itemNames, locales []string, pushID, wsSlug, stepID string) {
	if s.JobStore == nil || s.JobQueue == nil || s.ContentStore == nil {
		return
	}

	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil {
		slog.Info("auto-translate: failed to load project", "id", projectID, "error", err)
		return
	}

	// Check opt-out.
	if proj.Properties != nil && proj.Properties["auto_translate"] == "false" {
		return
	}

	if len(locales) == 0 {
		for _, l := range proj.TargetLanguages {
			locales = append(locales, string(l))
		}
	}
	if len(locales) == 0 {
		return
	}

	if wsSlug == "" {
		wsSlug = "_anon"
	}

	// Determine model from project properties or default.
	model := "gpt-4o-mini"
	if proj.Properties != nil && proj.Properties["ai_model"] != "" {
		model = proj.Properties["ai_model"]
	}

	var jobIDs []string
	for _, itemName := range itemNames {
		for _, locale := range locales {
			job := &jobs.TranslationJob{
				ID:               id.New(),
				WorkspaceSlug:    wsSlug,
				ProjectID:        projectID,
				ItemName:         itemName,
				TargetLocale:     locale,
				ProviderConfigID: "platform",
				Model:            model,
				PushID:           pushID,
				StepID:           stepID,
				Status:           jobs.StatusQueued,
			}

			if err := s.JobStore.CreateJob(ctx, job); err != nil {
				slog.Info("auto-translate: failed to create job for", "name", itemName, "locale", locale, "error", err)
				continue
			}

			if err := s.JobQueue.Enqueue(ctx, job.ID); err != nil {
				slog.Info("auto-translate: failed to enqueue job", "id", job.ID, "error", err)
				_ = s.JobStore.DeleteJob(ctx, job.ID)
			} else {
				jobIDs = append(jobIDs, job.ID)
			}
		}
	}

	// Register spawned jobs on the automation step for visibility tracking.
	if stepID != "" && s.AutomationRunStore != nil && len(jobIDs) > 0 {
		_ = s.AutomationRunStore.RegisterStepJobs(ctx, stepID, jobIDs)
		if s.stepCompletionTracker != nil {
			s.stepCompletionTracker.TrackStep(stepID, "", false)
		}
	}
}

// executeNotifyAction sends a notification to specified users.
func (s *Server) executeNotifyAction(action event.AutomationAction, ev platev.Event) {
	if s.NotificationStore == nil {
		return
	}

	userID := action.Config["user_id"]
	if userID == "" {
		userID = ev.Data["user_id"]
	}
	if userID == "" {
		return
	}

	title := action.Config["title"]
	if title == "" {
		title = "Automation notification"
	}
	body := action.Config["body"]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	n := &bstore.Notification{
		UserID:    userID,
		Type:      bstore.NotificationType(action.Config["notification_type"]),
		Title:     title,
		Body:      body,
		ProjectID: ev.ProjectID,
	}
	if err := s.NotificationStore.Create(ctx, n); err == nil {
		s.NotifyUser(userID, n)
	}
}

// triggerAutoExtract creates extraction jobs for each item pushed.
func (s *Server) triggerAutoExtract(ctx context.Context, projectID string, itemNames []string, pushID, wsSlug, stepID string) {
	if s.ExtractionJobStore == nil || s.ExtractionQueue == nil || s.ContentStore == nil {
		return
	}

	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil {
		slog.Info("auto-extract: failed to load project", "id", projectID, "error", err)
		return
	}

	// Check opt-out.
	if proj.Properties != nil && proj.Properties["auto_extract"] == "false" {
		return
	}

	locale := string(proj.DefaultSourceLanguage)
	model := "gpt-4o-mini"
	if proj.Properties != nil && proj.Properties["extraction_model"] != "" {
		model = proj.Properties["extraction_model"]
	}

	if wsSlug == "" {
		wsSlug = "_anon"
	}

	var jobIDs []string
	for _, itemName := range itemNames {
		job := &jobs.ExtractionJob{
			ID:            id.New(),
			WorkspaceSlug: wsSlug,
			ProjectID:     projectID,
			ItemName:      itemName,
			Locale:        locale,
			PushID:        pushID,
			StepID:        stepID,
			Model:         model,
			Status:        jobs.ExtractionStatusQueued,
		}

		if err := s.ExtractionJobStore.CreateExtractionJob(ctx, job); err != nil {
			slog.Info("auto-extract: failed to create job for", "id", itemName, "error", err)
			continue
		}

		if err := s.ExtractionQueue.Enqueue(ctx, job.ID); err != nil {
			slog.Info("auto-extract: failed to enqueue job", "id", job.ID, "error", err)
			_ = s.ExtractionJobStore.UpdateExtractionJobStatus(ctx, job.ID, jobs.ExtractionStatusFailed, "enqueue failed")
		} else {
			jobIDs = append(jobIDs, job.ID)
		}
	}

	if stepID != "" && s.AutomationRunStore != nil && len(jobIDs) > 0 {
		_ = s.AutomationRunStore.RegisterStepJobs(ctx, stepID, jobIDs)
		if s.stepCompletionTracker != nil {
			s.stepCompletionTracker.TrackStep(stepID, "", true)
		}
	}
}

// triggerAutoTranslateNewLocales creates translation jobs for all existing items in the new locales.
func (s *Server) triggerAutoTranslateNewLocales(ctx context.Context, projectID string, locales []string, wsSlug string) {
	if s.ContentStore == nil {
		return
	}

	items, err := s.ContentStore.ListItems(ctx, projectID, "main")
	if err != nil {
		slog.Info("auto-translate-new-locale: failed to list items for", "id", projectID, "error", err)
		return
	}
	if len(items) == 0 {
		return
	}

	pushID := id.New()
	var itemNames []string
	for _, item := range items {
		itemNames = append(itemNames, item.Name)
	}

	s.triggerAutoTranslate(ctx, projectID, itemNames, locales, pushID, wsSlug, "")
}

// createReviewTasks creates per-locale review or translate tasks for project members (Bowrain AD-014).
func (s *Server) createReviewTasks(ctx context.Context, action event.AutomationAction, ev platev.Event, stepID string) {
	if s.ContentStore == nil || s.TaskStore == nil || s.AuthStore == nil {
		return
	}

	proj, err := s.ContentStore.GetProject(ctx, ev.ProjectID)
	if err != nil {
		slog.Info("create-review-tasks: failed to load project", "id", ev.ProjectID, "error", err)
		return
	}

	// Check opt-in: only create tasks if the project has workflow_enabled=true.
	if proj.Properties == nil || proj.Properties["workflow_enabled"] != "true" {
		return
	}

	mode := action.Config["mode"]
	if mode == "" {
		mode = "review"
	}
	taskType := bstore.TaskReview
	if mode == "translate" {
		taskType = bstore.TaskTranslate
	}

	priority := bstore.TaskPriority(action.Config["priority"])
	if priority == "" {
		priority = bstore.TaskPriorityNormal
	}

	members, err := s.AuthStore.ListProjectMembers(ctx, proj.ID)
	if err != nil {
		slog.Info("create-review-tasks: failed to list members for", "id", proj.ID, "error", err)
		return
	}

	pushID := ev.Data["push_id"]
	items := ev.Data["items"]

	// Load existing open tasks for deduplication.
	existingLocales := s.existingOpenTaskLocales(ctx, proj.WorkspaceID, proj.ID, string(taskType))

	var taskIDs []string
	for _, locale := range proj.TargetLanguages {
		localeStr := string(locale)

		// Skip if an open/in-progress task already exists for this locale.
		if existingLocales[localeStr] {
			continue
		}

		assignees := s.findMembersForLocale(ctx, members, localeStr, mode)

		for _, m := range assignees {
			task := &bstore.Task{
				WorkspaceID: proj.WorkspaceID,
				ProjectID:   proj.ID,
				Stream:      "main",
				Type:        taskType,
				Status:      bstore.TaskStatusOpen,
				Priority:    priority,
				Title:       fmt.Sprintf("Review %s translations", localeStr),
				AssigneeID:  m.UserID,
				CreatedBy:   "system",
				Data: map[string]string{
					"push_id": pushID,
					"locale":  localeStr,
					"items":   items,
					"mode":    mode,
				},
			}
			if err := s.TaskStore.Create(ctx, task); err != nil {
				slog.Info("create-review-tasks: failed to create task for", "name", localeStr, "locale", m.UserID, "error", err)
				continue
			}
			taskIDs = append(taskIDs, task.ID)
			if s.NotificationDispatcher != nil {
				s.NotificationDispatcher.DispatchTaskNotification(
					ctx, task, bstore.NotificationTaskAssigned,
					fmt.Sprintf("New %s task: %s", mode, localeStr),
					fmt.Sprintf("Content is ready for %s in %s.", mode, localeStr),
				)
			}
		}

		// If no members for this locale, create unassigned task.
		if len(assignees) == 0 {
			task := &bstore.Task{
				WorkspaceID: proj.WorkspaceID,
				ProjectID:   proj.ID,
				Stream:      "main",
				Type:        taskType,
				Status:      bstore.TaskStatusOpen,
				Priority:    priority,
				Title:       fmt.Sprintf("Review %s translations (unassigned)", localeStr),
				CreatedBy:   "system",
				Data: map[string]string{
					"push_id": pushID,
					"locale":  localeStr,
					"items":   items,
					"mode":    mode,
				},
			}
			if err := s.TaskStore.Create(ctx, task); err == nil {
				taskIDs = append(taskIDs, task.ID)
			}
		}
	}

	// Register created tasks on the automation step.
	if stepID != "" && s.AutomationRunStore != nil && len(taskIDs) > 0 {
		_ = s.AutomationRunStore.RegisterStepTasks(ctx, stepID, taskIDs)
	}
}

// createSourceReviewTask creates a source review task before language fan-out (Bowrain AD-014).
func (s *Server) createSourceReviewTask(ctx context.Context, action event.AutomationAction, ev platev.Event, stepID string) {
	if s.ContentStore == nil || s.TaskStore == nil {
		return
	}

	proj, err := s.ContentStore.GetProject(ctx, ev.ProjectID)
	if err != nil {
		slog.Info("create-source-review: failed to load project", "id", ev.ProjectID, "error", err)
		return
	}

	if proj.Properties == nil || proj.Properties["workflow_enabled"] != "true" {
		return
	}

	reviewer := action.Config["reviewer"]

	// Fall back to first project member with PermEditSource.
	if reviewer == "" && s.AuthStore != nil {
		members, err := s.AuthStore.ListProjectMembers(ctx, proj.ID)
		if err == nil {
			for _, m := range members {
				rt, err := s.AuthStore.GetRoleTemplate(ctx, proj.WorkspaceID, m.RoleID)
				if err == nil && rt.Permissions.Has(platauth.PermEditSource) {
					reviewer = m.UserID
					break
				}
			}
		}
	}

	task := &bstore.Task{
		WorkspaceID: proj.WorkspaceID,
		ProjectID:   proj.ID,
		Stream:      "main",
		Type:        bstore.TaskSourceReview,
		Status:      bstore.TaskStatusOpen,
		Priority:    bstore.TaskPriorityNormal,
		Title:       "Review source content before translation",
		AssigneeID:  reviewer,
		CreatedBy:   "system",
		Data:        ev.Data, // carries push_id, items, workspace_slug
	}

	if err := s.TaskStore.Create(ctx, task); err != nil {
		slog.Info("create-source-review: failed to create task", "error", err)
		return
	}

	if reviewer != "" && s.NotificationDispatcher != nil {
		s.NotificationDispatcher.DispatchTaskNotification(
			ctx, task, bstore.NotificationTaskAssigned,
			"Source review needed",
			"New content needs source review before translation fan-out.",
		)
	}

	if stepID != "" && s.AutomationRunStore != nil {
		_ = s.AutomationRunStore.RegisterStepTasks(ctx, stepID, []string{task.ID})
	}
}

// findMembersForLocale returns project members whose language scope includes the locale
// and whose role has the required permission for the given mode.
func (s *Server) findMembersForLocale(ctx context.Context, members []*platauth.ProjectMembership, locale, mode string) []*platauth.ProjectMembership {
	requiredPerm := platauth.PermReview
	if mode == "translate" {
		requiredPerm = platauth.PermTranslate
	}

	var result []*platauth.ProjectMembership
	for _, m := range members {
		// Check language scope: empty = all languages.
		if len(m.Languages) > 0 {
			found := slices.Contains(m.Languages, locale)
			if !found {
				continue
			}
		}

		// Check permission via role template.
		if s.AuthStore == nil {
			continue
		}
		rt, err := s.AuthStore.GetRoleTemplate(ctx, m.WorkspaceID, m.RoleID)
		if err != nil {
			continue
		}
		if !rt.Permissions.Has(requiredPerm) {
			continue
		}

		result = append(result, m)
	}
	return result
}

// existingOpenTaskLocales returns a set of locales that already have open or in-progress tasks
// for the given project and task type. Used for deduplication.
func (s *Server) existingOpenTaskLocales(ctx context.Context, wsID, projectID, taskType string) map[string]bool {
	result := map[string]bool{}
	if s.TaskStore == nil {
		return result
	}
	res, err := s.TaskStore.List(ctx, bstore.TaskQuery{
		WorkspaceID: wsID,
		ProjectID:   projectID,
		Type:        taskType,
		Statuses:    []string{string(bstore.TaskStatusOpen), string(bstore.TaskStatusInProgress)},
	})
	if err != nil {
		return result
	}
	for _, t := range res.Tasks {
		if locale := t.Data["locale"]; locale != "" {
			result[locale] = true
		}
	}
	return result
}
