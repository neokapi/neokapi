package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskTypeIsVolume(t *testing.T) {
	volume := []TaskType{TaskTranslate, TaskReview, TaskSourceReview}
	escalation := []TaskType{
		TaskReviewTerms, TaskFixQuality, TaskFixBrandVoice, TaskFixTerminology,
		TaskConnectorSetup, TaskCustom,
	}

	for _, tt := range volume {
		assert.True(t, tt.IsVolume(), "%s grows with content pushed", tt)
		assert.Equal(t, TaskClassVolume, ClassOf(tt))
	}
	for _, tt := range escalation {
		assert.False(t, tt.IsVolume(), "%s grows with anomalies, not content", tt)
		assert.Equal(t, TaskClassEscalation, ClassOf(tt))
	}
}

func TestCreateStampsTheClass(t *testing.T) {
	store := newTestTaskStore(t)
	ctx := t.Context()

	t.Run("volume", func(t *testing.T) {
		task := &Task{
			WorkspaceID: "ws-1", ProjectID: "p-1", Type: TaskReview,
			Title: "Review fr-FR", CreatedBy: "system",
		}
		require.NoError(t, store.Create(ctx, task))
		got, err := store.Get(ctx, task.ID)
		require.NoError(t, err)
		assert.Equal(t, string(TaskClassVolume), got.Data[TaskDataClass])
	})

	t.Run("escalation", func(t *testing.T) {
		task := &Task{
			WorkspaceID: "ws-1", Type: TaskReviewTerms,
			Title: "Review proposed terms", CreatedBy: "system",
		}
		require.NoError(t, store.Create(ctx, task))
		got, err := store.Get(ctx, task.ID)
		require.NoError(t, err)
		assert.Equal(t, string(TaskClassEscalation), got.Data[TaskDataClass])
	})

	t.Run("a caller cannot disagree with the type", func(t *testing.T) {
		// The class is derived, so the ratio between the queues cannot be
		// falsified by a call site that stamps its own.
		task := &Task{
			WorkspaceID: "ws-1", ProjectID: "p-1", Type: TaskReview,
			Title: "Review de-DE", CreatedBy: "system",
			Data: map[string]string{TaskDataClass: string(TaskClassEscalation)},
		}
		require.NoError(t, store.Create(ctx, task))
		got, err := store.Get(ctx, task.ID)
		require.NoError(t, err)
		assert.Equal(t, string(TaskClassVolume), got.Data[TaskDataClass])
	})
}
