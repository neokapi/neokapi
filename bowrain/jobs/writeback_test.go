package jobs

import (
	"context"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readThenChange is a content store on which the stored content changes right
// after a caller's first block read, the way a push applied by another worker
// lands between a job's read and its write.
type readThenChange struct {
	store.ContentStore
	once   sync.Once
	change func(ctx context.Context)
}

func (s *readThenChange) GetBlocks(ctx context.Context, q store.BlockQuery) ([]*venue.StoredBlock, error) {
	blocks, err := s.ContentStore.GetBlocks(ctx, q)
	s.once.Do(func() { s.change(ctx) })
	return blocks, err
}

// runTranslationJob drives one demo translation job for en.json into fr against
// the given store.
func runTranslationJob(t *testing.T, db *storage.PgDB, cs store.ContentStore, projectID string) {
	t.Helper()
	ctx := t.Context()
	js, err := NewJobStore(db)
	require.NoError(t, err)
	deps := &WorkerDeps{
		JobStore:      js,
		ContentStore:  cs,
		Platform:      &PlatformProviderConfig{Provider: "demo"},
		ProviderStore: &fakeProviderResolver{cfg: bstore.ProviderConfig{Type: "demo"}},
	}
	job := &TranslationJob{
		ID:               "job-" + id.New(),
		WorkspaceSlug:    "acme",
		ProjectID:        projectID,
		ItemName:         "en.json",
		TargetLocale:     "fr",
		ProviderConfigID: "platform",
		Model:            "demo",
		Status:           StatusQueued,
	}
	require.NoError(t, js.CreateJob(ctx, job))
	claimed, epoch, err := js.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, executeTranslationWithDeps(ctx, deps, job, epoch))
}

// seedTranslationProject stores one item, en.json, holding the given source
// strings, with no targets yet.
func seedTranslationProject(t *testing.T, cs *bstore.PostgresStore, projectID string, sources ...string) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, cs.CreateProject(ctx, &store.Project{
		ID: projectID, Name: projectID, DefaultSourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"},
	}))
	require.NoError(t, cs.StoreItem(ctx, projectID, "main", &store.Item{Name: "en.json", Format: "json"}))
	blocks := make([]*model.Block, 0, len(sources))
	for i, src := range sources {
		blocks = append(blocks, model.NewBlock("k"+string(rune('a'+i)), src))
	}
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "en.json", blocks))
}

// A translation job reads an item's blocks, drafts them, and writes the drafts
// back. When a push removes the item in between, the drafts have nothing left
// to land on. Writing them anyway stored each block again with no item, which
// the pull served to every client as a block no file can hold.
func TestTranslationJob_AnItemRemovedWhileTheJobRunsStaysRemoved(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	const projectID = "proj-writeback-removed"
	seedTranslationProject(t, cs, projectID, "Hello", "Goodbye")

	racing := &readThenChange{ContentStore: cs, change: func(ctx context.Context) {
		assert.NoError(t, cs.DeleteItem(ctx, projectID, "main", "en.json"))
	}}
	runTranslationJob(t, db, racing, projectID)

	all, err := cs.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID, Stream: "main"})
	require.NoError(t, err)
	for _, sb := range all {
		t.Errorf("block %s (%q) is stored again after its item was removed, under item %q",
			sb.Block.ID, sb.Block.SourceText(), sb.ItemName)
	}
	items, err := cs.ListItems(ctx, projectID, "main")
	require.NoError(t, err)
	assert.Empty(t, items, "the removed item stays removed")
}

// When a push changes a block's source while a job is drafting the old
// wording, the job's write-back must leave the new source standing. Writing the
// block as it was read restored the old source over the pushed one, and logged
// no source change, so no client ever learned of it.
func TestTranslationJob_SourceChangedWhileTheJobRunsIsNotReverted(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	const projectID = "proj-writeback-moved"
	seedTranslationProject(t, cs, projectID, "Hello")

	racing := &readThenChange{ContentStore: cs, change: func(ctx context.Context) {
		assert.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "en.json",
			[]*model.Block{model.NewBlock("ka", "Hello, world")}))
	}}
	runTranslationJob(t, db, racing, projectID)

	stored, err := cs.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID, Stream: "main", ItemName: "en.json"})
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, "Hello, world", stored[0].Block.SourceText(), "the pushed source stands")
	assert.Empty(t, stored[0].Block.TargetText("fr"),
		"a draft of the old wording is not recorded against the new wording")
}

// The extraction worker annotates an item's blocks and writes them back to the
// item. When a push removes the item in between, the write-back must not bring
// the item back.
func TestExtractionJob_AnItemRemovedWhileTheJobRunsStaysRemoved(t *testing.T) {
	ctx := context.Background()
	cs, projectID := newExtractionFixture(t, 3)
	require.NoError(t, cs.StoreItem(ctx, projectID, "main", &store.Item{Name: "en.json", Format: "json"}))

	srv, _ := newExtractionModel(t)
	jobStore := &countingExtractionStore{job: &ExtractionJob{
		ID: id.New(), WorkspaceSlug: "ws", ProjectID: projectID,
		ItemName: "en.json", Status: ExtractionStatusQueued,
	}}
	racing := &readThenChange{ContentStore: cs, change: func(ctx context.Context) {
		assert.NoError(t, cs.DeleteItem(ctx, projectID, "main", "en.json"))
	}}
	deps := &ExtractionWorkerDeps{
		ExtractionJobStore: jobStore,
		ContentStore:       racing,
		ReviewQueueCreator: &recordingReviewQueue{},
		Platform: &PlatformProviderConfig{
			Provider: "openai", APIKey: "k", Model: "test-model", BaseURL: srv.URL,
		},
	}
	require.NoError(t, processExtractionJob(ctx, deps, jobStore.job.ID))

	items, err := cs.ListItems(ctx, projectID, "main")
	require.NoError(t, err)
	assert.Empty(t, items, "the removed item stays removed")
	all, err := cs.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID, Stream: "main"})
	require.NoError(t, err)
	assert.Empty(t, all, "no annotated block is stored for the removed item")
}
