package server

import (
	"context"
	"log"
	"strings"

	"github.com/gokapi/gokapi/bowrain/event"
	"github.com/gokapi/gokapi/bowrain/jobs"
	bstore "github.com/gokapi/gokapi/bowrain/store"
	platev "github.com/gokapi/gokapi/platform/event"
	"github.com/google/uuid"
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

	// Load user-defined rules from database.
	if s.AutomationRuleStore != nil {
		s.loadStoredAutomationRules()
	}
}

// loadStoredAutomationRules loads all enabled user-defined rules from the database
// and registers them with the automation engine.
func (s *Server) loadStoredAutomationRules() {
	// Rules will be loaded per-project when events occur.
	// The automation engine's event handler checks the rule's project scope
	// against the event's project ID. For now, this is a placeholder for
	// future project-scoped rule loading on startup.
}

// executeAutomationAction is the callback for the automation engine.
func (s *Server) executeAutomationAction(action event.AutomationAction, ev platev.Event) error {
	switch action.Type {
	case "auto_translate":
		items := ev.Data["items"]
		pushID := ev.Data["push_id"]
		wsSlug := ev.Data["workspace_slug"]
		if items == "" || pushID == "" {
			return nil
		}
		itemNames := strings.Split(items, ",")
		go s.triggerAutoTranslate(context.Background(), ev.ProjectID, itemNames, nil, pushID, wsSlug)

	case "auto_extract":
		items := ev.Data["items"]
		pushID := ev.Data["push_id"]
		wsSlug := ev.Data["workspace_slug"]
		if items == "" || pushID == "" {
			return nil
		}
		itemNames := strings.Split(items, ",")
		go s.triggerAutoExtract(context.Background(), ev.ProjectID, itemNames, pushID, wsSlug)

	case "notify":
		s.executeNotifyAction(action, ev)

	case "auto_translate_new_locale":
		newLocales := ev.Data["new_locales"]
		wsSlug := ev.Data["workspace_slug"]
		if newLocales == "" {
			return nil
		}
		locales := strings.Split(newLocales, ",")
		go s.triggerAutoTranslateNewLocales(context.Background(), ev.ProjectID, locales, wsSlug)
	}
	return nil
}

// triggerAutoTranslate creates translation jobs for each (item, locale) pair.
func (s *Server) triggerAutoTranslate(ctx context.Context, projectID string, itemNames, locales []string, pushID, wsSlug string) {
	if s.JobStore == nil || s.JobQueue == nil || s.ContentStore == nil {
		return
	}

	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil {
		log.Printf("auto-translate: failed to load project %s: %v", projectID, err)
		return
	}

	// Check opt-out.
	if proj.Properties != nil && proj.Properties["auto_translate"] == "false" {
		return
	}

	if len(locales) == 0 {
		for _, l := range proj.TargetLocales {
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

	for _, itemName := range itemNames {
		for _, locale := range locales {
			job := &jobs.TranslationJob{
				ID:               uuid.NewString(),
				WorkspaceSlug:    wsSlug,
				ProjectID:        projectID,
				ItemName:         itemName,
				TargetLocale:     locale,
				ProviderConfigID: "platform",
				Model:            model,
				PushID:           pushID,
				Status:           jobs.StatusQueued,
			}

			if err := s.JobStore.CreateJob(ctx, job); err != nil {
				log.Printf("auto-translate: failed to create job for %s/%s: %v", itemName, locale, err)
				continue
			}

			if err := s.JobQueue.Enqueue(ctx, job.ID); err != nil {
				log.Printf("auto-translate: failed to enqueue job %s: %v", job.ID, err)
				_ = s.JobStore.DeleteJob(ctx, job.ID)
			}
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

	ctx := context.Background()
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
func (s *Server) triggerAutoExtract(ctx context.Context, projectID string, itemNames []string, pushID, wsSlug string) {
	if s.ExtractionJobStore == nil || s.ExtractionQueue == nil || s.ContentStore == nil {
		return
	}

	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil {
		log.Printf("auto-extract: failed to load project %s: %v", projectID, err)
		return
	}

	// Check opt-out.
	if proj.Properties != nil && proj.Properties["auto_extract"] == "false" {
		return
	}

	locale := string(proj.SourceLocale)
	model := "gpt-4o-mini"
	if proj.Properties != nil && proj.Properties["extraction_model"] != "" {
		model = proj.Properties["extraction_model"]
	}

	if wsSlug == "" {
		wsSlug = "_anon"
	}

	for _, itemName := range itemNames {
		job := &jobs.ExtractionJob{
			ID:            uuid.NewString(),
			WorkspaceSlug: wsSlug,
			ProjectID:     projectID,
			ItemName:      itemName,
			Locale:        locale,
			PushID:        pushID,
			Model:         model,
			Status:        jobs.ExtractionStatusQueued,
		}

		if err := s.ExtractionJobStore.CreateExtractionJob(ctx, job); err != nil {
			log.Printf("auto-extract: failed to create job for %s: %v", itemName, err)
			continue
		}

		if err := s.ExtractionQueue.Enqueue(ctx, job.ID); err != nil {
			log.Printf("auto-extract: failed to enqueue job %s: %v", job.ID, err)
			_ = s.ExtractionJobStore.UpdateExtractionJobStatus(ctx, job.ID, jobs.ExtractionStatusFailed, "enqueue failed")
		}
	}
}

// triggerAutoTranslateNewLocales creates translation jobs for all existing items in the new locales.
func (s *Server) triggerAutoTranslateNewLocales(ctx context.Context, projectID string, locales []string, wsSlug string) {
	if s.ContentStore == nil {
		return
	}

	items, err := s.ContentStore.ListItems(ctx, projectID)
	if err != nil {
		log.Printf("auto-translate-new-locale: failed to list items for %s: %v", projectID, err)
		return
	}
	if len(items) == 0 {
		return
	}

	pushID := uuid.NewString()
	var itemNames []string
	for _, item := range items {
		itemNames = append(itemNames, item.Name)
	}

	s.triggerAutoTranslate(ctx, projectID, itemNames, locales, pushID, wsSlug)
}
