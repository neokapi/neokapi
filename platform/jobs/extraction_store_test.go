package jobs

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestExtractionStore(t *testing.T) *SQLiteExtractionJobStore {
	t.Helper()
	db, err := storage.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Run all job migrations (which include the extraction_jobs table).
	js, err := NewSQLiteJobStore(db)
	require.NoError(t, err)
	_ = js

	return NewSQLiteExtractionJobStore(db)
}

func TestExtractionJobStore_CreateAndGet(t *testing.T) {
	store := newTestExtractionStore(t)
	ctx := context.Background()

	job := &ExtractionJob{
		ID:            uuid.NewString(),
		WorkspaceSlug: "test-ws",
		ProjectID:     "proj-1",
		ItemName:      "en.json",
		Locale:        "en-US",
		PushID:        "push-1",
		Model:         "gpt-4o-mini",
		Status:        ExtractionStatusQueued,
	}

	err := store.CreateExtractionJob(ctx, job)
	require.NoError(t, err)
	assert.False(t, job.CreatedAt.IsZero())

	got, err := store.GetExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, job.ID, got.ID)
	assert.Equal(t, "test-ws", got.WorkspaceSlug)
	assert.Equal(t, "proj-1", got.ProjectID)
	assert.Equal(t, "en.json", got.ItemName)
	assert.Equal(t, "en-US", got.Locale)
	assert.Equal(t, ExtractionStatusQueued, got.Status)
}

func TestExtractionJobStore_UpdateStatus(t *testing.T) {
	store := newTestExtractionStore(t)
	ctx := context.Background()

	job := &ExtractionJob{
		ID:            uuid.NewString(),
		WorkspaceSlug: "ws",
		ProjectID:     "p1",
		ItemName:      "file.json",
	}
	require.NoError(t, store.CreateExtractionJob(ctx, job))

	err := store.UpdateExtractionJobStatus(ctx, job.ID, ExtractionStatusProcessing, "")
	require.NoError(t, err)

	got, err := store.GetExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ExtractionStatusProcessing, got.Status)

	err = store.UpdateExtractionJobStatus(ctx, job.ID, ExtractionStatusFailed, "some error")
	require.NoError(t, err)

	got, err = store.GetExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, ExtractionStatusFailed, got.Status)
	assert.Equal(t, "some error", got.Error)
}

func TestExtractionJobStore_ListByPushID(t *testing.T) {
	store := newTestExtractionStore(t)
	ctx := context.Background()

	// Create jobs for two different pushes.
	for _, pid := range []string{"push-A", "push-A", "push-B"} {
		job := &ExtractionJob{
			ID:            uuid.NewString(),
			WorkspaceSlug: "ws",
			ProjectID:     "p1",
			ItemName:      "file.json",
			Locale:        "en-US",
			PushID:        pid,
			Status:        ExtractionStatusQueued,
		}
		require.NoError(t, store.CreateExtractionJob(ctx, job))
	}

	got, err := store.ListByPushID(ctx, "push-A")
	require.NoError(t, err)
	assert.Len(t, got, 2)
	for _, j := range got {
		assert.Equal(t, "push-A", j.PushID)
	}

	got, err = store.ListByPushID(ctx, "push-B")
	require.NoError(t, err)
	assert.Len(t, got, 1)

	got, err = store.ListByPushID(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestExtractionJobStore_UpdateProgress(t *testing.T) {
	store := newTestExtractionStore(t)
	ctx := context.Background()

	job := &ExtractionJob{
		ID:            uuid.NewString(),
		WorkspaceSlug: "ws",
		ProjectID:     "p1",
		ItemName:      "file.json",
	}
	require.NoError(t, store.CreateExtractionJob(ctx, job))

	err := store.UpdateExtractionJobProgress(ctx, job.ID, 10, 50, 3)
	require.NoError(t, err)

	got, err := store.GetExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, 10, got.DoneBlocks)
	assert.Equal(t, 50, got.TotalBlocks)
	assert.Equal(t, 3, got.ItemsCreated)
}
