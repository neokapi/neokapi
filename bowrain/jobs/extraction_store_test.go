package jobs

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestExtractionStore(t *testing.T) ExtractionJobStore {
	t.Helper()
	dbURL := os.Getenv("BOWRAIN_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("BOWRAIN_TEST_DATABASE_URL not set")
	}
	db, err := storage.OpenPostgres(dbURL)
	require.NoError(t, err)
	t.Cleanup(func() {
		// Clean up test data.
		_, _ = db.ExecContext(t.Context(), "DELETE FROM extraction_jobs")
		db.Close()
	})
	store, err := NewExtractionJobStore(db)
	require.NoError(t, err)
	return store
}

// extractionStoreTests runs the full test suite against any ExtractionJobStore.
func extractionStoreTests(t *testing.T, store ExtractionJobStore) {
	t.Run("CreateAndGet", func(t *testing.T) {
		ctx := t.Context()
		job := &ExtractionJob{
			ID:            uuid.NewString(),
			WorkspaceSlug: "test-ws",
			ProjectID:     "proj-1",
			ItemName:      "en.json",
			Locale:        "en-US",
			PushID:        "push-1",
			StepID:        "step-1",
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
		assert.Equal(t, "push-1", got.PushID)
		assert.Equal(t, "step-1", got.StepID)
		assert.Equal(t, ExtractionStatusQueued, got.Status)
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		ctx := t.Context()
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
	})

	t.Run("ClaimJob", func(t *testing.T) {
		ctx := t.Context()
		job := &ExtractionJob{
			ID:            uuid.NewString(),
			WorkspaceSlug: "ws",
			ProjectID:     "p1",
			ItemName:      "file.json",
			Status:        ExtractionStatusQueued,
		}
		require.NoError(t, store.CreateExtractionJob(ctx, job))

		claimed, err := store.ClaimExtractionJob(ctx, job.ID)
		require.NoError(t, err)
		assert.True(t, claimed)

		// Second claim should fail.
		claimed, err = store.ClaimExtractionJob(ctx, job.ID)
		require.NoError(t, err)
		assert.False(t, claimed)
	})

	t.Run("ListByPushID", func(t *testing.T) {
		ctx := t.Context()
		pushA := "push-" + uuid.NewString()[:8]
		pushB := "push-" + uuid.NewString()[:8]

		for _, pid := range []string{pushA, pushA, pushB} {
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

		got, err := store.ListByPushID(ctx, pushA)
		require.NoError(t, err)
		assert.Len(t, got, 2)
		for _, j := range got {
			assert.Equal(t, pushA, j.PushID)
		}

		got, err = store.ListByPushID(ctx, pushB)
		require.NoError(t, err)
		assert.Len(t, got, 1)

		got, err = store.ListByPushID(ctx, "nonexistent")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("UpdateProgress", func(t *testing.T) {
		ctx := t.Context()
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
	})
}

func TestExtractionJobStore(t *testing.T) {
	store := newTestExtractionStore(t)
	extractionStoreTests(t, store)
}
