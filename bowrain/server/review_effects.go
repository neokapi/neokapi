package server

import (
	"context"
	"encoding/json"
	"log/slog"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
)

// processDecisionSideEffects handles the downstream effects of a review
// decision. An approved entity marked do-not-translate is added to the
// project's do-not-translate list; every other decision has none.
func (s *Server) processDecisionSideEffects(ctx context.Context, item *bstore.ReviewItem, _ string) {
	if item == nil || item.Status != bstore.ReviewItemApproved {
		return
	}
	if item.Type == bstore.ReviewItemEntityReview {
		s.approveEntity(ctx, item)
	}
}

// approveEntity adds an approved entity marked do-not-translate to the
// project's do-not-translate list.
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
		if err := s.ReviewQueueStore.AddDNTEntry(ctx, item.ProjectID, entity.Text, string(entity.Type), item.Locale, "review"); err != nil {
			slog.ErrorContext(ctx, "review-effects: failed to record do-not-translate entry",
				"project", item.ProjectID, "entity", entity.Text, "error", err)
		}
	}
}
