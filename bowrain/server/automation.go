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
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/bowrain/jobs"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	coreblockstore "github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
)

// registerDefaultAutomations installs the engine's rule set at startup: the
// built-in rules plus every enabled stored rule.
func (s *Server) registerDefaultAutomations() {
	if s.AutomationEngine == nil {
		return
	}
	s.reloadAutomationRules()
}

// builtInAutomationRules are the platform's own rules. They carry no project,
// so they fire for every project's events.
func builtInAutomationRules() []event.AutomationRule {
	return []event.AutomationRule{
		// Auto-extract entities and terms on push. This stays an automation:
		// entity and term extraction is not convergence. Translation on push is
		// the server convergence engine's (an on-push project starts a
		// convergence run; see subscribeConvergeOnPush), whose produce and park
		// steps cover what the former auto-translate-on-push,
		// auto-translate-new-locale and create-review-tasks-on-automation-complete
		// rules did.
		{
			Name:      "auto-extract-on-push",
			EventType: platev.EventPushCompleted,
			Actions: []event.AutomationAction{
				{Type: "auto_extract"},
			},
		},
		// Fan out review tasks after source review (Bowrain AD-014). Source
		// review is an independent workflow beside push convergence.
		{
			Name:      "fan-out-after-source-review",
			EventType: platev.EventSourceReviewCompleted,
			Actions: []event.AutomationAction{
				{Type: "create_review_tasks", Config: map[string]string{"mode": "review"}},
			},
		},
	}
}

// reloadAutomationRules replaces the engine's rule set with the built-in rules
// plus every enabled stored rule, each scoped to its project. It runs at
// startup and after a rule is created, updated, toggled or deleted, so a rule
// saved from the app fires without a restart.
func (s *Server) reloadAutomationRules() {
	if s.AutomationEngine == nil {
		return
	}
	rules := builtInAutomationRules()
	rules = append(rules, s.storedAutomationRules()...)
	s.AutomationEngine.ReplaceRules(rules)
}

// storedAutomationRules loads every enabled user-defined rule from the
// database as an engine rule scoped to the project it was authored on.
func (s *Server) storedAutomationRules() []event.AutomationRule {
	if s.AutomationRuleStore == nil || s.ContentStore == nil {
		return nil
	}

	// Startup wiring and rule mutations alike run outside any one request's
	// lifetime, so a fresh background context is correct here.
	ctx := context.Background()
	projects, err := s.ContentStore.ListProjects(ctx)
	if err != nil {
		slog.Warn("failed to list projects for automation rule loading", "error", err)
		return nil
	}

	var rules []event.AutomationRule
	for _, proj := range projects {
		stored, err := s.AutomationRuleStore.ListRules(ctx, proj.ID)
		if err != nil {
			slog.Warn("failed to load automation rules for project", "id", proj.ID, "error", err)
			continue
		}
		for _, r := range stored {
			if !r.Enabled {
				continue
			}
			rules = append(rules, event.AutomationRule{
				Name:       r.Name,
				EventType:  r.Trigger,
				ProjectID:  proj.ID,
				Conditions: r.Conditions,
				Actions:    r.Actions,
			})
		}
	}
	if len(rules) > 0 {
		slog.Info("loaded user-defined automation rules", "count", len(rules))
	}
	return rules
}

// newRunManager builds the run manager that records a run and its steps
// around every action executeAutomationAction dispatches, reporting each
// transition to the SSE hub so a subscriber of the run sees it as it happens.
func (s *Server) newRunManager() *event.AutomationRunManager {
	rm := event.NewAutomationRunManager(s.AutomationRunStore, s.executeAutomationAction)
	if s.runHub != nil {
		rm.SetRunNotifier(s.runHub)
	}
	return rm
}

// executeAutomationAction is the callback for the automation engine (via RunManager).
func (s *Server) executeAutomationAction(action event.AutomationAction, ev platev.Event, stepID string) error {
	startedAt := ev.Timestamp
	err := s.doExecuteAction(action, ev, stepID)
	if err == nil && actionReportsOwnOutcome(action.Type) {
		// The action writes its history entry when it finishes.
		return nil
	}
	// Automation-engine callback: event-driven background work with no
	// request context in scope.
	s.recordAutomationHistory(context.Background(), ev, startedAt, ev.Timestamp, err)
	return err
}

// recordAutomationHistory writes one execution history entry for an action.
// The record must always be written, so callers hand it a context that is
// not about to be cancelled.
func (s *Server) recordAutomationHistory(ctx context.Context, ev platev.Event, startedAt, endedAt time.Time, err error) {
	if s.AutomationRuleStore == nil {
		return
	}
	status := "success"
	errMsg := ""
	if err != nil {
		status = "failed"
		errMsg = err.Error()
	}
	_ = s.AutomationRuleStore.RecordExecution(ctx, &event.HistoryEntry{
		ID:        id.New(),
		ProjectID: ev.ProjectID,
		EventID:   ev.ID,
		Status:    status,
		Error:     errMsg,
		StartedAt: startedAt,
		EndedAt:   endedAt,
	})
}

// runAction starts one automation action in the background, under the server's
// wait group and behind a recover.
//
// Nothing above these goroutines can recover for them: they are started from an
// event-bus callback, so a panic in an action — a nil store, a malformed
// payload — takes the whole server down, not the automation. And they must be
// waited on: an action that outlives Shutdown writes into stores that are
// closing under it.
func (s *Server) runAction(name string, cancel context.CancelFunc, fn func()) {
	s.actions.Go(func() {
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("recovered panic in automation action", "action", name, "panic", r)
			}
		}()
		fn()
	})
}

// awaitActions blocks until every running automation action has finished, or
// until ctx says the shutdown budget is spent. Actions that outrun the budget
// are named, so a slow one is diagnosable rather than merely late.
func (s *Server) awaitActions(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.actions.Wait()
	}()
	select {
	case <-done:
	case <-ctx.Done():
		slog.Warn("shutdown: automation actions still running when the budget ran out")
	}
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
		s.runAction(action.Type, cancel, func() {
			s.triggerAutoTranslate(actionCtx, ev.ProjectID, itemNames, nil, pushID, wsSlug, stepID)
		})

	case "auto_extract":
		items := ev.Data["items"]
		pushID := ev.Data["push_id"]
		wsSlug := ev.Data["workspace_slug"]
		if items == "" || pushID == "" {
			cancel()
			return nil
		}
		itemNames := strings.Split(items, ",")
		s.runAction(action.Type, cancel, func() {
			s.triggerAutoExtract(actionCtx, ev.ProjectID, itemNames, pushID, wsSlug, stepID)
		})

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
		s.runAction(action.Type, cancel, func() {
			s.triggerAutoTranslateNewLocales(actionCtx, ev.ProjectID, locales, wsSlug)
		})

	case "create_review_tasks":
		s.runAction(action.Type, cancel, func() {
			s.createReviewTasks(actionCtx, action, ev, stepID)
		})

	case "create_source_review":
		s.runAction(action.Type, cancel, func() {
			s.createSourceReviewTask(actionCtx, action, ev, stepID)
		})

	case "write_overlay":
		s.runAction(action.Type, cancel, func() {
			s.executeWriteOverlay(actionCtx, action, ev, stepID)
		})

	case "run_flow":
		s.runAction(action.Type, cancel, func() {
			s.executeRunFlow(actionCtx, action, ev, stepID)
		})

	default:
		cancel()
		return fmt.Errorf("unsupported action type %q", action.Type)
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

	// The automation path treats a credit refusal as a silent no-op (it has no
	// run row to label); the convergence orchestrator inspects the returned
	// error instead. See createTranslationJobs.
	// The automation trigger predates stream scoping and fires for main-line
	// pushes; jobs it spawns keep writing to main.
	jobIDs, _ := s.createTranslationJobs(ctx, proj, "main", itemNames, locales, pushID, wsSlug, stepID)

	// Register spawned jobs on the automation step for visibility tracking.
	//
	// Tracking a step whose registration failed is worse than not tracking it:
	// the tracker reads TotalJobs == 0 as "not registered yet", waits thirty
	// minutes, then marks the step failed and bills the wait as container time —
	// for jobs that ran perfectly well. So a failed registration means no
	// tracking, and the step stays untracked rather than becoming a false
	// failure with an invoice attached.
	if stepID != "" && s.AutomationRunStore != nil && len(jobIDs) > 0 {
		if err := s.AutomationRunStore.RegisterStepJobs(ctx, stepID, jobIDs); err != nil {
			slog.ErrorContext(ctx, "automation: failed to register step jobs; leaving the step untracked",
				"step", stepID, "jobs", len(jobIDs), "error", err)
		} else if s.stepCompletionTracker != nil {
			s.stepCompletionTracker.TrackStep(stepID, "", false)
		}
	}
}

// createTranslationJobs creates and enqueues one platform translation job per
// (item, locale), returning the enqueued job IDs. It is the shared production
// primitive behind both the auto-extract/translate automation path and the
// convergence orchestrator's Produce step; the model resolution and provider
// binding ("platform" + BOWRAIN_PLATFORM_PROVIDER) are identical for both.
//
// On a zero-credit workspace it refuses to spawn jobs and returns a typed
// errStallNeedsCredits sentinel (NOT an empty list silently) so the convergence
// orchestrator can label the run's stall_reason instead of parking with no
// reason (strategy 2026-07-dogfood doc 06, theme C). The automation caller
// discards the error — a refusal there is a legitimate no-op.
func (s *Server) createTranslationJobs(ctx context.Context, proj *store.Project, stream string, itemNames, locales []string, pushID, wsSlug, stepID string) ([]string, error) {
	if s.JobStore == nil || s.JobQueue == nil {
		return nil, nil
	}
	// Credit pre-check (Epic 004): these are platform-key jobs. Refuse to spawn
	// them for a zero-credit workspace so automation (and the convergence
	// orchestrator) can't drive the ledger deeply negative. Self-hosted/unbilled
	// deployments are never blocked.
	if s.insufficientPlatformCredits(ctx, proj.WorkspaceID, "platform") {
		slog.Warn("translation jobs: skipped, workspace out of credits",
			"workspace_id", proj.WorkspaceID, "project", proj.ID)
		return nil, errStallNeedsCredits
	}
	model := "gpt-4o-mini"
	if proj.Properties != nil && proj.Properties["ai_model"] != "" {
		model = proj.Properties["ai_model"]
	}
	if wsSlug == "" {
		wsSlug = "_anon"
	}
	var jobIDs []string
	for _, itemName := range itemNames {
		for _, locale := range locales {
			job := &jobs.TranslationJob{
				ID:               id.New(),
				WorkspaceSlug:    wsSlug,
				WorkspaceID:      proj.WorkspaceID,
				ProjectID:        proj.ID,
				ItemName:         itemName,
				Stream:           stream,
				TargetLocale:     locale,
				ProviderConfigID: "platform",
				Model:            model,
				PushID:           pushID,
				StepID:           stepID,
				Status:           jobs.StatusQueued,
			}
			if err := s.JobStore.CreateJob(ctx, job); err != nil {
				slog.Info("translation jobs: failed to create job", "name", itemName, "locale", locale, "error", err)
				continue
			}
			if err := s.JobQueue.Enqueue(ctx, job.ID); err != nil {
				slog.Info("translation jobs: failed to enqueue job", "id", job.ID, "error", err)
				_ = s.JobStore.DeleteJob(ctx, job.ID)
				continue
			}
			jobIDs = append(jobIDs, job.ID)
		}
	}
	return jobIDs, nil
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

	// Automation action: event-driven background work with no request context
	// in scope; bounded by its own timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	n := &bstore.Notification{
		UserID:    userID,
		Type:      bstore.NotificationType(action.Config["notification_type"]),
		Title:     title,
		Body:      body,
		ProjectID: ev.ProjectID,
	}
	if created, err := s.NotificationStore.Create(ctx, n); err == nil && created {
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
		// See triggerAutoTranslate: tracking an unregistered step turns into a
		// billed thirty-minute false failure.
		if err := s.AutomationRunStore.RegisterStepJobs(ctx, stepID, jobIDs); err != nil {
			slog.ErrorContext(ctx, "auto-extract: failed to register step jobs; leaving the step untracked",
				"step", stepID, "jobs", len(jobIDs), "error", err)
		} else if s.stepCompletionTracker != nil {
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

// workflowReviewEnabled reports whether human review fan-out is enabled for a
// project. Governed review is the default: a missing or empty workflow_enabled
// property means enabled; only an explicit "false" opts a project out.
func workflowReviewEnabled(proj *store.Project) bool {
	return proj.Properties["workflow_enabled"] != "false"
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

	if !workflowReviewEnabled(proj) {
		return
	}

	s.createReviewTasksForLocales(ctx, proj, proj.TargetLanguages, action, ev, stepID)
}

// createReviewTasksForLocales creates review/translate tasks for a specific set
// of `locales` of an already-loaded governed project (the caller owns the
// workflowReviewEnabled gate). createReviewTasks fans out to every configured
// target language; RV-E's re-check (recheckProjectTargets) reuses this to
// re-queue only the locales whose approved targets it demoted. The per-locale
// dedup against open tasks and the owner-reach fallback are shared, so both
// callers route work to a person the same way.
func (s *Server) createReviewTasksForLocales(ctx context.Context, proj *store.Project, locales []model.LocaleID, action event.AutomationAction, ev platev.Event, stepID string) {
	if s.TaskStore == nil || s.AuthStore == nil {
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
	runID := ev.Data["run_id"] // set when a convergence run fans out review tasks

	// The points this push touched, so the work reaches whoever holds them
	// rather than everyone who happens to speak the language. Empty when the
	// push named no items or none resolve to a collection, which routes exactly
	// as it did before regions existed.
	points := s.pushPoints(ctx, proj.ID, items)

	// taskData builds the per-locale linkage carried on each task, preserving
	// run_id when present so a convergence-run task points back at its run.
	taskData := func(localeStr string) map[string]string {
		d := map[string]string{"push_id": pushID, "locale": localeStr, "items": items, "mode": mode}
		if runID != "" {
			d["run_id"] = runID
		}
		return d
	}

	// Load existing open tasks for deduplication.
	existingLocales := s.existingOpenTaskLocales(ctx, proj.WorkspaceID, proj.ID, string(taskType))

	var taskIDs []string
	for _, locale := range locales {
		localeStr := string(locale)

		// Skip if an open/in-progress task already exists for this locale.
		if existingLocales[localeStr] {
			continue
		}

		var assigneeIDs []string
		for _, m := range s.findMembersForLocale(ctx, members, localeStr, mode, points) {
			assigneeIDs = append(assigneeIDs, m.UserID)
		}
		// No project member covers this locale — the common case for a project
		// created without explicit members (the onboarding gap that made governed
		// review invisible: an unassigned task never surfaces on the assignee=me
		// dashboard). Route the review to the workspace owner / review-capable
		// workspace members so the work always reaches a person.
		if len(assigneeIDs) == 0 && taskType == bstore.TaskReview {
			assigneeIDs = s.fallbackReviewAssignees(ctx, proj, mode)
		}

		if len(assigneeIDs) == 0 {
			// Truly nobody can take it. Record an unassigned task so the work is
			// not lost, and WARN — a governed run must never silently produce a
			// review task no one can act on.
			slog.Warn("create-review-tasks: no eligible assignee; creating unassigned task",
				"project", proj.ID, "locale", localeStr, "mode", mode)
			task := &bstore.Task{
				WorkspaceID: proj.WorkspaceID,
				ProjectID:   proj.ID,
				Stream:      "main",
				Type:        taskType,
				Status:      bstore.TaskStatusOpen,
				Priority:    priority,
				Title:       fmt.Sprintf("Review %s translations (unassigned)", localeStr),
				CreatedBy:   "system",
				Data:        taskData(localeStr),
			}
			if err := s.TaskStore.Create(ctx, task); err == nil {
				taskIDs = append(taskIDs, task.ID)
			}
			continue
		}

		for _, uid := range assigneeIDs {
			task := &bstore.Task{
				WorkspaceID: proj.WorkspaceID,
				ProjectID:   proj.ID,
				Stream:      "main",
				Type:        taskType,
				Status:      bstore.TaskStatusOpen,
				Priority:    priority,
				Title:       fmt.Sprintf("Review %s translations", localeStr),
				AssigneeID:  uid,
				CreatedBy:   "system",
				Data:        taskData(localeStr),
			}
			if err := s.TaskStore.Create(ctx, task); err != nil {
				slog.Info("create-review-tasks: failed to create task for", "name", localeStr, "locale", uid, "error", err)
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

	if !workflowReviewEnabled(proj) {
		return
	}

	// Who the summons reaches. A rule that names a reviewer routes to exactly
	// that person; otherwise it reaches every member entitled to edit the
	// source (sourceOwners), with the first of them carrying the assignment.
	// A run held below the source gate is blocked until somebody settles the
	// source, so telling one arbitrary member and nobody else made the hold
	// depend on that member being available.
	reviewer := action.Config["reviewer"]
	owners := []string{reviewer}
	if reviewer == "" {
		owners = s.sourceOwners(ctx, proj)
		if len(owners) > 0 {
			reviewer = owners[0]
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

	s.notifySourceOwners(ctx, task, owners,
		"Source review needed",
		"New content needs source review before translation fan-out.",
	)

	if stepID != "" && s.AutomationRunStore != nil {
		_ = s.AutomationRunStore.RegisterStepTasks(ctx, stepID, []string{task.ID})
	}
}

// fallbackReviewAssignees resolves who should receive a locale's review task
// when no project member covers it (a project created without explicit members).
// It routes to the workspace owner(s) — owner-first so a solo founder's governed
// review always reaches them — and, failing an owner, any workspace member whose
// role carries the mode's required permission (review needs PermReview). This is
// what makes governed review never invisible: the assignee=me dashboard has
// someone to surface the task to. Returns user IDs.
func (s *Server) fallbackReviewAssignees(ctx context.Context, proj *store.Project, mode string) []string {
	if s.AuthStore == nil || proj.WorkspaceID == "" {
		return nil
	}
	requiredPerm := platauth.PermReview
	if mode == "translate" {
		requiredPerm = platauth.PermTranslate
	}
	members, err := s.AuthStore.ListMembers(ctx, proj.WorkspaceID)
	if err != nil {
		slog.Info("create-review-tasks: workspace member lookup failed", "workspace", proj.WorkspaceID, "error", err)
		return nil
	}
	var owners, others []string
	for _, m := range members {
		// Workspace-role default permissions (owner/admin = full, member =
		// translate-capable) decide review capability when no project membership
		// narrows it — the same fallback the permission layer resolves with.
		if !platauth.DefaultPermissionsForRole(m.Role).Permissions.Has(requiredPerm) {
			continue
		}
		if m.Role == platauth.RoleOwner {
			owners = append(owners, m.UserID)
		} else {
			others = append(others, m.UserID)
		}
	}
	if len(owners) > 0 {
		return owners
	}
	return others
}

// findMembersForLocale returns project members whose language scope includes the locale
// and whose role has the required permission for the given mode.
// It applies two rules beyond language and permission, both of them consequences
// of what a membership now carries:
//
//   - A member bounded to a region only receives work for a push that touched
//     that region. An unbounded member receives everything, as before.
//   - A custodian is never the first choice for volume work. The seat is priced
//     against the review function it replaces, so a custodian sitting on a queue
//     that grows with content pushed is the failure that would invalidate the
//     price. A reviewer bounded to one brand is not a custodian — see
//     auth.CustodialPermissions.
//
// Neither rule can starve the queue: when nothing survives them the caller falls
// back to the workspace owners, which is what keeps governed review visible.
func (s *Server) findMembersForLocale(ctx context.Context, members []*platauth.ProjectMembership, locale, mode string, points []map[string]string) []*platauth.ProjectMembership {
	requiredPerm := platauth.PermReview
	if mode == "translate" {
		requiredPerm = platauth.PermTranslate
	}
	taskType := bstore.TaskReview
	if mode == "translate" {
		taskType = bstore.TaskTranslate
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

		reach := platauth.CoordinateReach{}.Add(m.Coordinates)
		if !reachesAnyPoint(reach, points) {
			continue
		}
		if taskType.IsVolume() && platauth.IsCustodian(rt.Permissions, reach) {
			continue
		}

		result = append(result, m)
	}
	return result
}

// reachesAnyPoint reports whether a member's custody covers at least one of the
// points a push touched. No points means the push told us nothing about where
// its content sits, which routes to everyone as it did before regions existed —
// silence about the region must widen the audience, never narrow it, or a push
// that failed to resolve its collections would quietly reach nobody.
func reachesAnyPoint(reach platauth.CoordinateReach, points []map[string]string) bool {
	if len(points) == 0 || reach.Unconstrained() {
		return true
	}
	return slices.ContainsFunc(points, reach.Reaches)
}

// pushPoints resolves the distinct points a push touched, by walking its items
// to the collections that hold them and reading the coordinates each collection
// recorded. Returns nil when nothing resolves, which routes as before.
func (s *Server) pushPoints(ctx context.Context, projectID, items string) []map[string]string {
	if s.ContentStore == nil || items == "" {
		return nil
	}
	wanted := map[string]bool{}
	for name := range strings.SplitSeq(items, ",") {
		if name = strings.TrimSpace(name); name != "" {
			wanted[name] = true
		}
	}
	if len(wanted) == 0 {
		return nil
	}

	all, err := s.ContentStore.ListItems(ctx, projectID, "main")
	if err != nil {
		slog.InfoContext(ctx, "push points: cannot list items; routing to everyone",
			"project", projectID, "error", err)
		return nil
	}
	collectionIDs := map[string]bool{}
	for _, it := range all {
		if it != nil && wanted[it.Name] && it.CollectionID != "" {
			collectionIDs[it.CollectionID] = true
		}
	}
	if len(collectionIDs) == 0 {
		return nil
	}

	collections, err := s.ContentStore.ListCollections(ctx, projectID, "main")
	if err != nil {
		slog.InfoContext(ctx, "push points: cannot list collections; routing to everyone",
			"project", projectID, "error", err)
		return nil
	}
	seen := map[string]bool{}
	var points []map[string]string
	for _, col := range collections {
		if col == nil || !collectionIDs[col.ID] {
			continue
		}
		// The default point — a collection that declared no coordinates — is a
		// real place content sits, so it is carried as an empty point rather
		// than skipped. Only a bounded custodian of some other region misses it.
		key := platauth.CoordinateFilter(col.Context).String()
		if seen[key] {
			continue
		}
		seen[key] = true
		points = append(points, col.Context)
	}
	return points
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
