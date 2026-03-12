package jobs

import (
	"context"
	"encoding/json"

	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// ReviewQueueStoreAdapter adapts bstore.ReviewQueueStore to the ReviewQueueCreator interface.
type ReviewQueueStoreAdapter struct {
	Store *bstore.ReviewQueueStore
}

func (a *ReviewQueueStoreAdapter) CreateReviewItem(ctx context.Context, item *ReviewQueueItem) error {
	ri := &bstore.ReviewItem{
		ProjectID:  item.ProjectID,
		Type:       bstore.ReviewItemType(item.Type),
		PushID:     item.PushID,
		Data:       item.Data,
		Confidence: item.Confidence,
		Locale:     item.Locale,
	}

	// Parse occurrences from JSON.
	if len(item.Occurrences) > 0 {
		var occs []bstore.Occurrence
		if err := json.Unmarshal(item.Occurrences, &occs); err == nil {
			ri.Occurrences = occs
		}
	}

	return a.Store.CreateItem(ctx, ri)
}

func (a *ReviewQueueStoreAdapter) IsTermRejected(ctx context.Context, projectID, text, locale string) (bool, error) {
	return a.Store.IsRejected(ctx, projectID, text, locale)
}
