package server

import (
	"context"
	"log"
	"strings"

	"github.com/gokapi/gokapi/bowrain/event"
	"github.com/gokapi/gokapi/bowrain/jobs"
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

	// Rule 2: Auto-translate when new locales are added.
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
