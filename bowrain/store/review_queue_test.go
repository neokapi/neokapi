package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
)

func newTestReviewStore(t *testing.T) (*ReviewQueueStore, *SQLiteStore) {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })

	rq := NewReviewQueueStore(s.DB())
	return rq, s
}

func createReviewProject(t *testing.T, s *SQLiteStore) *platstore.Project {
	t.Helper()
	p := &platstore.Project{
		Name:                  "Test Project",
		DefaultSourceLanguage: model.LocaleEnglish,
		TargetLanguages:       []model.LocaleID{model.LocaleFrench},
		Properties:            map[string]string{},
	}
	require.NoError(t, s.CreateProject(context.Background(), p))
	return p
}

func TestReviewQueueStore_CreateAndGet(t *testing.T) {
	rq, cs := newTestReviewStore(t)
	proj := createReviewProject(t, cs)
	ctx := context.Background()

	item := &ReviewItem{
		ProjectID: proj.ID,
		Type:      ReviewItemTermCandidate,
		Data:      json.RawMessage(`{"text":"Dashboard","definition":"Main screen"}`),
		Occurrences: []Occurrence{
			{BlockID: "b1", FilePath: "en.json", Start: 5, End: 14, Context: "The Dashboard shows..."},
		},
		Confidence: 0.88,
		Locale:     "en-US",
		PushID:     "push-1",
	}

	err := rq.CreateItem(ctx, item)
	require.NoError(t, err)
	assert.NotEmpty(t, item.ID)
	assert.False(t, item.CreatedAt.IsZero())

	got, err := rq.GetItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, item.ID, got.ID)
	assert.Equal(t, proj.ID, got.ProjectID)
	assert.Equal(t, ReviewItemTermCandidate, got.Type)
	assert.Equal(t, ReviewItemPending, got.Status)
	assert.Equal(t, 0.88, got.Confidence)
	assert.Equal(t, "en-US", got.Locale)
	assert.Len(t, got.Occurrences, 1)
	assert.Equal(t, "b1", got.Occurrences[0].BlockID)
}

func TestReviewQueueStore_ListWithFilters(t *testing.T) {
	rq, cs := newTestReviewStore(t)
	proj := createReviewProject(t, cs)
	ctx := context.Background()

	// Create mixed items.
	items := []ReviewItem{
		{ProjectID: proj.ID, Type: ReviewItemTermCandidate, Data: json.RawMessage(`{"text":"Dashboard"}`), Confidence: 0.9, Locale: "en-US"},
		{ProjectID: proj.ID, Type: ReviewItemEntityReview, Data: json.RawMessage(`{"text":"John"}`), Confidence: 0.95, Locale: "en-US"},
		{ProjectID: proj.ID, Type: ReviewItemTermCandidate, Data: json.RawMessage(`{"text":"Workflow"}`), Confidence: 0.3, Locale: "fr-FR"},
	}
	for i := range items {
		require.NoError(t, rq.CreateItem(ctx, &items[i]))
	}

	t.Run("all items", func(t *testing.T) {
		result, err := rq.ListItems(ctx, ReviewQueueQuery{ProjectID: proj.ID})
		require.NoError(t, err)
		assert.Equal(t, 3, result.Total)
		assert.Len(t, result.Items, 3)
		assert.Equal(t, 3, result.Remaining)
	})

	t.Run("filter by type", func(t *testing.T) {
		result, err := rq.ListItems(ctx, ReviewQueueQuery{ProjectID: proj.ID, Type: ReviewItemTermCandidate})
		require.NoError(t, err)
		assert.Equal(t, 2, result.Total)
	})

	t.Run("filter by locale", func(t *testing.T) {
		result, err := rq.ListItems(ctx, ReviewQueueQuery{ProjectID: proj.ID, Locale: "fr-FR"})
		require.NoError(t, err)
		assert.Equal(t, 1, result.Total)
		assert.Equal(t, "fr-FR", result.Items[0].Locale)
	})

	t.Run("limit", func(t *testing.T) {
		result, err := rq.ListItems(ctx, ReviewQueueQuery{ProjectID: proj.ID, Limit: 2})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		assert.NotEmpty(t, result.NextCursor)
	})
}

func TestReviewQueueStore_Decide(t *testing.T) {
	rq, cs := newTestReviewStore(t)
	proj := createReviewProject(t, cs)
	ctx := context.Background()

	item := &ReviewItem{
		ProjectID: proj.ID, Type: ReviewItemTermCandidate,
		Data: json.RawMessage(`{"text":"Dashboard"}`), Confidence: 0.9, Locale: "en-US",
	}
	require.NoError(t, rq.CreateItem(ctx, item))

	t.Run("approve", func(t *testing.T) {
		err := rq.Decide(ctx, item.ID, DecideRequest{
			Decision: "approve",
			Comment:  "Looks good",
			UserID:   "user-1",
			Edits:    json.RawMessage(`{"definition":"Updated definition"}`),
		})
		require.NoError(t, err)

		got, err := rq.GetItem(ctx, item.ID)
		require.NoError(t, err)
		assert.Equal(t, ReviewItemApproved, got.Status)
		assert.Equal(t, "user-1", got.DecidedBy)
		assert.NotNil(t, got.DecidedAt)
		assert.Equal(t, "Looks good", got.Comment)
	})

	t.Run("already decided fails", func(t *testing.T) {
		err := rq.Decide(ctx, item.ID, DecideRequest{
			Decision: "reject", UserID: "user-2",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already decided")
	})
}

func TestReviewQueueStore_BatchDecide(t *testing.T) {
	rq, cs := newTestReviewStore(t)
	proj := createReviewProject(t, cs)
	ctx := context.Background()

	var ids []string
	for i := 0; i < 5; i++ {
		item := &ReviewItem{
			ProjectID: proj.ID, Type: ReviewItemEntityReview,
			Data: json.RawMessage(`{"text":"entity"}`), Confidence: 0.9, Locale: "en-US",
		}
		require.NoError(t, rq.CreateItem(ctx, item))
		ids = append(ids, item.ID)
	}

	decided, err := rq.BatchDecide(ctx, ids[:3], DecideRequest{
		Decision: "approve", UserID: "user-1",
	})
	require.NoError(t, err)
	assert.Equal(t, 3, decided)

	// Remaining should be 2.
	result, err := rq.ListItems(ctx, ReviewQueueQuery{ProjectID: proj.ID, Status: ReviewItemPending})
	require.NoError(t, err)
	assert.Equal(t, 2, result.Total)
}

func TestReviewQueueStore_Assign(t *testing.T) {
	rq, cs := newTestReviewStore(t)
	proj := createReviewProject(t, cs)
	ctx := context.Background()

	item := &ReviewItem{
		ProjectID: proj.ID, Type: ReviewItemTermCandidate,
		Data: json.RawMessage(`{"text":"API"}`), Confidence: 0.85, Locale: "en-US",
	}
	require.NoError(t, rq.CreateItem(ctx, item))

	err := rq.Assign(ctx, item.ID, "reviewer-1")
	require.NoError(t, err)

	got, err := rq.GetItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, ReviewItemAssigned, got.Status)
	assert.Equal(t, "reviewer-1", got.AssignedTo)
}

func TestReviewQueueStore_SplitItem(t *testing.T) {
	rq, cs := newTestReviewStore(t)
	proj := createReviewProject(t, cs)
	ctx := context.Background()

	item := &ReviewItem{
		ProjectID: proj.ID, Type: ReviewItemTermCandidate,
		Data: json.RawMessage(`{"text":"bank"}`),
		Occurrences: []Occurrence{
			{BlockID: "b1", Context: "river bank"},
			{BlockID: "b2", Context: "bank account"},
			{BlockID: "b3", Context: "blood bank"},
		},
		Confidence: 0.7, Locale: "en-US",
	}
	require.NoError(t, rq.CreateItem(ctx, item))

	// Split b2 into a separate item.
	newItem, err := rq.SplitItem(ctx, item.ID, []string{"b2"})
	require.NoError(t, err)
	assert.NotEqual(t, item.ID, newItem.ID)
	assert.Len(t, newItem.Occurrences, 1)
	assert.Equal(t, "b2", newItem.Occurrences[0].BlockID)

	// Original should have 2 remaining.
	original, err := rq.GetItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Len(t, original.Occurrences, 2)
}

func TestReviewQueueStore_SplitItem_InvalidSplit(t *testing.T) {
	rq, cs := newTestReviewStore(t)
	proj := createReviewProject(t, cs)
	ctx := context.Background()

	item := &ReviewItem{
		ProjectID: proj.ID, Type: ReviewItemTermCandidate,
		Data:        json.RawMessage(`{"text":"term"}`),
		Occurrences: []Occurrence{{BlockID: "b1"}},
		Confidence:  0.8, Locale: "en-US",
	}
	require.NoError(t, rq.CreateItem(ctx, item))

	// Can't split all occurrences (would leave original empty).
	_, err := rq.SplitItem(ctx, item.ID, []string{"b1"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "both items")
}

func TestReviewQueueStore_RejectedTerms(t *testing.T) {
	rq, cs := newTestReviewStore(t)
	proj := createReviewProject(t, cs)
	ctx := context.Background()

	rejected, err := rq.IsRejected(ctx, proj.ID, "Dashboard", "en-US")
	require.NoError(t, err)
	assert.False(t, rejected)

	require.NoError(t, rq.AddRejectedTerm(ctx, proj.ID, "Dashboard", "en-US"))

	rejected, err = rq.IsRejected(ctx, proj.ID, "Dashboard", "en-US")
	require.NoError(t, err)
	assert.True(t, rejected)

	// Idempotent.
	require.NoError(t, rq.AddRejectedTerm(ctx, proj.ID, "Dashboard", "en-US"))
}

func TestReviewQueueStore_DNTEntries(t *testing.T) {
	rq, cs := newTestReviewStore(t)
	proj := createReviewProject(t, cs)
	ctx := context.Background()

	require.NoError(t, rq.AddDNTEntry(ctx, proj.ID, "Acme Corp", "entity:organization", "en-US", "llm"))
	require.NoError(t, rq.AddDNTEntry(ctx, proj.ID, "March 15", "entity:date", "en-US", "ner"))

	entries, err := rq.ListDNTEntries(ctx, proj.ID)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
	assert.Equal(t, "Acme Corp", entries[0].Text)
	assert.Equal(t, "March 15", entries[1].Text)
}
