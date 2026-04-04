package jobs

import (
	"testing"

	"github.com/google/uuid"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSQLiteStore(t *testing.T) *SQLiteJobStore {
	t.Helper()
	db, err := storage.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	store, err := NewSQLiteJobStore(db)
	require.NoError(t, err)
	return store
}

func TestSQLiteJobStore_CreateAndGet(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := t.Context()

	job := &TranslationJob{
		ID:               uuid.NewString(),
		WorkspaceSlug:    "my-ws",
		ProjectID:        "proj-1",
		ItemName:         "messages.json",
		TargetLocale:     "fr-FR",
		ProviderConfigID: "cfg-1",
	}
	require.NoError(t, s.CreateJob(ctx, job))

	got, err := s.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, job.ID, got.ID)
	assert.Equal(t, "my-ws", got.WorkspaceSlug)
	assert.Equal(t, StatusQueued, got.Status)
	assert.Equal(t, "messages.json", got.ItemName)
	assert.Equal(t, "fr-FR", got.TargetLocale)
}

func TestSQLiteJobStore_ListJobs(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := t.Context()

	for i := 0; i < 3; i++ {
		require.NoError(t, s.CreateJob(ctx, &TranslationJob{
			ID:            uuid.NewString(),
			WorkspaceSlug: "ws-1",
			ProjectID:     "p",
			ItemName:      "f",
			TargetLocale:  "de",
		}))
	}
	// Different workspace.
	require.NoError(t, s.CreateJob(ctx, &TranslationJob{
		ID:            uuid.NewString(),
		WorkspaceSlug: "ws-2",
		ProjectID:     "p",
		ItemName:      "f",
		TargetLocale:  "de",
	}))

	list, err := s.ListJobs(ctx, "ws-1", 50)
	require.NoError(t, err)
	assert.Len(t, list, 3)
}

func TestSQLiteJobStore_UpdateProgress(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := t.Context()

	job := &TranslationJob{
		ID:            uuid.NewString(),
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		ItemName:      "f",
		TargetLocale:  "ja",
	}
	require.NoError(t, s.CreateJob(ctx, job))

	require.NoError(t, s.UpdateJobProgress(ctx, job.ID, 5, 10))

	got, err := s.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, 5, got.DoneBlocks)
	assert.Equal(t, 10, got.TotalBlocks)
	assert.Equal(t, 50, got.Progress)
}

func TestSQLiteJobStore_UpdateStatus(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := t.Context()

	job := &TranslationJob{
		ID:            uuid.NewString(),
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		ItemName:      "f",
		TargetLocale:  "es",
	}
	require.NoError(t, s.CreateJob(ctx, job))

	require.NoError(t, s.UpdateJobStatus(ctx, job.ID, StatusFailed, "timeout"))
	got, err := s.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusFailed, got.Status)
	assert.Equal(t, "timeout", got.Error)
}

func TestSQLiteJobStore_Delete(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := t.Context()

	job := &TranslationJob{
		ID:            uuid.NewString(),
		WorkspaceSlug: "ws",
		ProjectID:     "p",
		ItemName:      "f",
		TargetLocale:  "ko",
	}
	require.NoError(t, s.CreateJob(ctx, job))
	require.NoError(t, s.DeleteJob(ctx, job.ID))

	_, err := s.GetJob(ctx, job.ID)
	assert.Error(t, err)
}
