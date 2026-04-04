package jobs

import (
	"testing"

	"github.com/neokapi/neokapi/core/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaimJob_OnlyOneWins(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := t.Context()

	job := &TranslationJob{
		ID:            id.New(),
		WorkspaceSlug: "ws",
		ProjectID:     "proj",
		ItemName:      "en.json",
		TargetLocale:  "fr",
		Status:        StatusQueued,
	}
	require.NoError(t, store.CreateJob(ctx, job))

	// First claim succeeds.
	ok1, err := store.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	assert.True(t, ok1, "first claim should succeed")

	// Second claim fails (already processing).
	ok2, err := store.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	assert.False(t, ok2, "second claim should fail")

	// Verify status is processing.
	got, err := store.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusProcessing, got.Status)
}

func TestClaimJob_SkipsNonQueued(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := t.Context()

	job := &TranslationJob{
		ID:            id.New(),
		WorkspaceSlug: "ws",
		ProjectID:     "proj",
		ItemName:      "en.json",
		TargetLocale:  "fr",
		Status:        StatusQueued,
	}
	require.NoError(t, store.CreateJob(ctx, job))

	// Mark as completed.
	require.NoError(t, store.UpdateJobStatus(ctx, job.ID, StatusCompleted, ""))

	// Claim should fail.
	ok, err := store.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	assert.False(t, ok, "should not claim completed job")
}

func TestClaimExtractionJob_OnlyOneWins(t *testing.T) {
	store := newTestExtractionStore(t)
	ctx := t.Context()

	job := &ExtractionJob{
		ID:            id.New(),
		WorkspaceSlug: "ws",
		ProjectID:     "proj",
		ItemName:      "en.json",
		Status:        ExtractionStatusQueued,
	}
	require.NoError(t, store.CreateExtractionJob(ctx, job))

	ok1, err := store.ClaimExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	assert.True(t, ok1)

	ok2, err := store.ClaimExtractionJob(ctx, job.ID)
	require.NoError(t, err)
	assert.False(t, ok2)
}
