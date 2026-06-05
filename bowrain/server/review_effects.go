package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
)

// processDecisionSideEffects handles the downstream effects of a review decision.
// For approved term candidates → creates a termbase concept.
// For rejected term candidates → adds to rejected_terms list.
// For approved entities with DNT → adds to dnt_entries list.
func (s *Server) processDecisionSideEffects(ctx context.Context, item *bstore.ReviewItem, wsSlug string) {
	if item == nil {
		return
	}

	switch item.Status {
	case bstore.ReviewItemApproved:
		s.processApproval(ctx, item, wsSlug)
	case bstore.ReviewItemRejected:
		s.processRejection(ctx, item)
	}
}

func (s *Server) processApproval(ctx context.Context, item *bstore.ReviewItem, wsSlug string) {
	switch item.Type {
	case bstore.ReviewItemTermCandidate:
		s.approveTermCandidate(ctx, item, wsSlug)
	case bstore.ReviewItemEntityReview:
		s.approveEntity(ctx, item)
	}
}

func (s *Server) processRejection(ctx context.Context, item *bstore.ReviewItem) {
	if item.Type != bstore.ReviewItemTermCandidate || s.ReviewQueueStore == nil {
		return
	}

	// Extract term text from data and add to rejected_terms.
	var data struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(item.Data, &data); err != nil || data.Text == "" {
		return
	}

	if err := s.ReviewQueueStore.AddRejectedTerm(ctx, item.ProjectID, data.Text, item.Locale); err != nil {
		slog.Info("review-effects: failed to add rejected term", "id", data.Text, "error", err)
	}
}

// approveTermCandidate creates a termbase concept from an approved term candidate.
func (s *Server) approveTermCandidate(ctx context.Context, item *bstore.ReviewItem, wsSlug string) {
	if s.wsStores == nil {
		return
	}

	var candidate model.TermCandidateAnnotation
	if err := json.Unmarshal(item.Data, &candidate); err != nil {
		slog.Info("review-effects: failed to unmarshal term candidate", "error", err)
		return
	}

	// Apply any user edits.
	if len(item.Edits) > 0 {
		var edits struct {
			Definition string `json:"definition"`
			Category   string `json:"category"`
		}
		if err := json.Unmarshal(item.Edits, &edits); err == nil {
			if edits.Definition != "" {
				candidate.Definition = edits.Definition
			}
			if edits.Category != "" {
				candidate.Category = model.TermCategory(edits.Category)
			}
		}
	}

	// Determine term status based on translatability.
	termStatus := model.TermApproved
	if candidate.Translatability == model.TranslatabilityDNT {
		termStatus = model.TermPreferred
	}

	concept := termbase.Concept{
		ID:         id.New(),
		Domain:     string(candidate.Category),
		Definition: candidate.Definition,
		Terms: []termbase.Term{
			{
				Text:   candidate.Text,
				Locale: candidate.Locale,
				Status: termStatus,
			},
		},
		Properties: map[string]string{
			"translatability": string(candidate.Translatability),
			"source":          string(candidate.Source),
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	tb, tbErr := s.wsStores.getTB(wsSlug)
	if tbErr != nil {
		slog.Error("review-effects: failed to init termbase", "workspace", wsSlug, "error", tbErr)
		return
	}
	if err := tb.AddConcept(ctx, concept); err != nil {
		slog.Info("review-effects: failed to add concept for", "id", candidate.Text, "error", err)
		return
	}

	// If translatability is DNT, also add to dnt_entries.
	if candidate.Translatability == model.TranslatabilityDNT && s.ReviewQueueStore != nil {
		_ = s.ReviewQueueStore.AddDNTEntry(ctx, item.ProjectID, candidate.Text, "", item.Locale, "review")
	}
}

// approveEntity processes an approved entity — if DNT, adds to dnt_entries.
func (s *Server) approveEntity(ctx context.Context, item *bstore.ReviewItem) {
	if s.ReviewQueueStore == nil {
		return
	}

	var entity model.EntityAnnotation
	if err := json.Unmarshal(item.Data, &entity); err != nil {
		slog.Info("review-effects: failed to unmarshal entity", "error", err)
		return
	}

	if entity.DNT {
		_ = s.ReviewQueueStore.AddDNTEntry(ctx, item.ProjectID, entity.Text, string(entity.Type), item.Locale, "review")
	}
}
