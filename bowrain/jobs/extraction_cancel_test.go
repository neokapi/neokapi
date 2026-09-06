package jobs

import (
	"context"
	"fmt"
	"testing"

	bstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cancellingExtractionStore stands in for the real store while a run is
// cancelled underneath the worker: it hands out one claim, then reports the
// lease lost from the renewal the cancel would have invalidated onwards, and
// counts the chunks the worker got through.
type cancellingExtractionStore struct {
	ExtractionJobStore
	job *ExtractionJob
	// cancelAfter is the number of renewals that succeed before the cancel
	// lands.
	cancelAfter int
	renewals    int
	completed   int
}

func (s *cancellingExtractionStore) GetExtractionJob(context.Context, string) (*ExtractionJob, error) {
	return s.job, nil
}

func (s *cancellingExtractionStore) ClaimExtractionJob(context.Context, string) (bool, int64, error) {
	return true, 1, nil
}

func (s *cancellingExtractionStore) RenewLease(context.Context, string, int64) (bool, error) {
	s.renewals++
	return s.renewals <= s.cancelAfter, nil
}

func (s *cancellingExtractionStore) UpdateExtractionJobProgress(context.Context, string, int, int, int) error {
	return nil
}

func (s *cancellingExtractionStore) UpdateExtractionJobStatus(context.Context, string, ExtractionJobStatus, string) error {
	return nil
}

func (s *cancellingExtractionStore) CompleteExtractionJob(context.Context, string, int64) (bool, error) {
	s.completed++
	return true, nil
}

// newExtractionCancelFixture is a content store holding one item of blocks,
// enough for the chunk loop to run more than one pass over it.
func newExtractionCancelFixture(t *testing.T, blocks int) (bstore.ContentStore, string) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(t.TempDir() + "/content.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	projectID := "extract-" + id.New()
	ctx := context.Background()
	require.NoError(t, cs.CreateProject(ctx, &bstore.Project{
		ID: projectID, Name: "Extract", DefaultSourceLanguage: "en",
	}))
	bs := make([]*model.Block, 0, blocks)
	for i := range blocks {
		b := &model.Block{ID: fmt.Sprintf("b%03d", i), Translatable: true}
		b.SetSourceText(fmt.Sprintf("Sentence number %d.", i))
		bs = append(bs, b)
	}
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "en.json", bs))
	return cs, projectID
}

// A cancel reaches a worker already extracting: the chunk boundary is the
// checkpoint, so the run stops there instead of extracting to the end and
// creating the items it was stopped for.
func TestExtractionWorker_CancelStopsTheRunAtTheNextChunk(t *testing.T) {
	// 120 blocks over a progress chunk of 50: three chunks, three checkpoints.
	cs, projectID := newExtractionCancelFixture(t, 120)
	store := &cancellingExtractionStore{
		job: &ExtractionJob{
			ID: id.New(), WorkspaceSlug: "ws", ProjectID: projectID,
			ItemName: "en.json", Status: ExtractionStatusProcessing,
		},
		cancelAfter: 1,
	}
	deps := &ExtractionWorkerDeps{
		ExtractionJobStore: store,
		ContentStore:       cs,
		Platform:           &PlatformProviderConfig{Provider: "demo"},
	}

	err := executeExtraction(context.Background(), deps, store.job, 1)
	require.ErrorIs(t, err, errLeaseLost)
	assert.Equal(t, 2, store.renewals, "the worker ran past the checkpoint that told it to stop")
}

// The worker that lost its lease abandons quietly: it neither fails the job,
// which would overwrite the reason the cancel wrote, nor completes it.
func TestExtractionWorker_ALostLeaseWritesNothing(t *testing.T) {
	cs, projectID := newExtractionCancelFixture(t, 120)
	store := &cancellingExtractionStore{
		job: &ExtractionJob{
			ID: id.New(), WorkspaceSlug: "ws", ProjectID: projectID,
			ItemName: "en.json", Status: ExtractionStatusQueued,
		},
		cancelAfter: 1,
	}
	deps := &ExtractionWorkerDeps{
		ExtractionJobStore: store,
		ContentStore:       cs,
		Platform:           &PlatformProviderConfig{Provider: "demo"},
	}

	require.NoError(t, processExtractionJob(context.Background(), deps, store.job.ID))
	assert.Zero(t, store.completed, "a cancelled run reported itself completed")
}

// A run nobody cancelled reaches the end and completes under its own lease.
func TestExtractionWorker_AnUncancelledRunCompletes(t *testing.T) {
	cs, projectID := newExtractionCancelFixture(t, 60)
	store := &cancellingExtractionStore{
		job: &ExtractionJob{
			ID: id.New(), WorkspaceSlug: "ws", ProjectID: projectID,
			ItemName: "en.json", Status: ExtractionStatusQueued,
		},
		cancelAfter: 100,
	}
	deps := &ExtractionWorkerDeps{
		ExtractionJobStore: store,
		ContentStore:       cs,
		Platform:           &PlatformProviderConfig{Provider: "demo"},
	}

	require.NoError(t, processExtractionJob(context.Background(), deps, store.job.ID))
	assert.Equal(t, 1, store.completed)
}
