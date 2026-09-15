package server

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readThenChange is a content store on which the stored content changes right
// after a caller's first block read, the way a push applied by the worker lands
// between a server run's read and its write.
type readThenChange struct {
	platstore.ContentStore
	once   sync.Once
	change func(ctx context.Context)
}

func (s *readThenChange) GetBlocks(ctx context.Context, q platstore.BlockQuery) ([]*venue.StoredBlock, error) {
	blocks, err := s.ContentStore.GetBlocks(ctx, q)
	s.once.Do(func() { s.change(ctx) })
	return blocks, err
}

// seedWriteBackItem creates a project with one item, en.json, holding the given
// source strings.
func seedWriteBackItem(t *testing.T, cs platstore.ContentStore, projectID string, sources ...string) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{
		ID: projectID, Name: projectID, DefaultSourceLanguage: model.LocaleEnglish,
		TargetLanguages: []model.LocaleID{model.LocaleFrench},
	}))
	require.NoError(t, cs.StoreItem(ctx, projectID, "main", &platstore.Item{ProjectID: projectID, Name: "en.json", Format: "json"}))
	blocks := make([]*model.Block, 0, len(sources))
	for i, src := range sources {
		b := model.NewBlock("k"+string(rune('a'+i)), src)
		b.Translatable = true
		blocks = append(blocks, b)
	}
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "en.json", blocks))
}

// requireNothingStored fails for every block the project still holds.
func requireNothingStored(t *testing.T, cs platstore.ContentStore, projectID string) {
	t.Helper()
	all, err := cs.GetBlocks(t.Context(), platstore.BlockQuery{ProjectID: projectID, Stream: "main"})
	require.NoError(t, err)
	for _, sb := range all {
		t.Errorf("block %s (%q) is stored again after its item was removed, under item %q",
			sb.Block.ID, sb.Block.SourceText(), sb.ItemName)
	}
}

// A server run's source settlement reads blocks a batch at a time and writes
// back the ones whose status moved. When a push removes an item between the
// read and the write, the write-back has nothing to land on and must store
// nothing: storing the blocks anyway left rows with no item, which the pull
// served as blocks no file can hold.
func TestSettleSource_AnItemRemovedDuringSettlementStaysRemoved(t *testing.T) {
	cs, err := bstore.NewPostgresStoreFromDB(pgtest.NewTestDB(t))
	require.NoError(t, err)
	const projectID = "settle-writeback-removed"
	seedWriteBackItem(t, cs, projectID, "A well-formed sentence.", "Another well-formed sentence.")

	racing := &readThenChange{ContentStore: cs, change: func(ctx context.Context) {
		assert.NoError(t, cs.DeleteItem(ctx, projectID, "main", "en.json"))
	}}
	s := &Server{ContentStore: racing}
	s.convergence = newConvergenceOrchestrator(s)

	_, err = s.convergence.settleSource(t.Context(), projectID)
	require.NoError(t, err)
	requireNothingStored(t, cs, projectID)
}

// Pseudo-translation reads an item's blocks and writes them back with targets.
// When the item is removed in between, nothing is stored.
func TestPseudoTranslate_AnItemRemovedDuringTheRunStaysRemoved(t *testing.T) {
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "writeback.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	const projectID = "pseudo-writeback-removed"
	seedWriteBackItem(t, cs, projectID, "Hello", "Goodbye")

	racing := &readThenChange{ContentStore: cs, change: func(ctx context.Context) {
		assert.NoError(t, cs.DeleteItem(ctx, projectID, "main", "en.json"))
	}}
	_, err = editorPseudoTranslate(t.Context(), racing, projectID, "main", "en.json", "fr")
	require.NoError(t, err)
	requireNothingStored(t, cs, projectID)
}
