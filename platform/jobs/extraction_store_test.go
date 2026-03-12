package jobs

import (
	"context"
	"testing"

	"github.com/gokapi/gokapi/bowrain/storage"
	"github.com/google/uuid"
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
