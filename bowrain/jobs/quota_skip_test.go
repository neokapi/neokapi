package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/neokapi/neokapi/core/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPushJob(t *testing.T, store JobStore, pushID string) *TranslationJob {
	t.Helper()
	job := &TranslationJob{
		ID:            id.New(),
		WorkspaceSlug: "ws",
		ProjectID:     "proj",
		ItemName:      "en.json",
		TargetLocale:  "fr",
		PushID:        pushID,
		Status:        StatusQueued,
	}
	require.NoError(t, store.CreateJob(context.Background(), job))
	return job
}

// TestSkipQueuedPushSiblings covers the worker's response to an exhausted
// provider quota: the push's queued jobs are failed without a provider call,
// and jobs outside the push or already running are left alone.
func TestSkipQueuedPushSiblings(t *testing.T) {
	quota := errors.New("bedrock: converse: StatusCode: 429, ThrottlingException: Too many tokens per day, please wait before trying again.")

	tests := []struct {
		name        string
		pushID      string
		cause       error
		wantSkipped int
	}{
		{name: "quota error skips queued siblings", pushID: "push-a", cause: quota, wantSkipped: 2},
		{name: "job without a push skips nothing", pushID: "", cause: quota, wantSkipped: 0},
		{name: "nil cause skips nothing", pushID: "push-a", cause: nil, wantSkipped: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, _ := newPgJobStore(t)
			ctx := context.Background()

			failing := newPushJob(t, store, "push-a")
			queued1 := newPushJob(t, store, "push-a")
			queued2 := newPushJob(t, store, "push-a")
			running := newPushJob(t, store, "push-a")
			other := newPushJob(t, store, "push-b")
			ok, _, err := store.ClaimJob(ctx, running.ID)
			require.NoError(t, err)
			require.True(t, ok)

			self := *failing
			self.PushID = tt.pushID
			got := skipQueuedPushSiblings(ctx, store, &self, tt.cause)
			assert.Equal(t, tt.wantSkipped, got)

			for _, j := range []*TranslationJob{queued1, queued2} {
				row, err := store.GetJob(ctx, j.ID)
				require.NoError(t, err)
				if tt.wantSkipped == 0 {
					assert.Equal(t, StatusQueued, row.Status)
					continue
				}
				assert.Equal(t, StatusFailed, row.Status)
				assert.Contains(t, row.Error, QuotaSkipPrefix)
				assert.Contains(t, row.Error, "Too many tokens per day",
					"the provider's message is kept so the row still reads as a quota failure")
			}

			row, err := store.GetJob(ctx, running.ID)
			require.NoError(t, err)
			assert.Equal(t, StatusProcessing, row.Status, "a running job is left to its worker")
			row, err = store.GetJob(ctx, other.ID)
			require.NoError(t, err)
			assert.Equal(t, StatusQueued, row.Status, "another push is untouched")
			row, err = store.GetJob(ctx, failing.ID)
			require.NoError(t, err)
			assert.Equal(t, StatusQueued, row.Status, "the failing job's own row is the caller's to write")
		})
	}
}
