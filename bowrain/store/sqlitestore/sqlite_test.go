package sqlitestore

import (
	"path/filepath"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "store.db")
	s, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func createTestProject(t *testing.T, s *SQLiteStore) *platstore.Project {
	t.Helper()
	p := &platstore.Project{
		Name:                  "Test Project",
		DefaultSourceLanguage: model.LocaleEnglish,
		TargetLanguages:       []model.LocaleID{model.LocaleFrench, model.LocaleGerman},
		Properties:            map[string]string{"client": "acme"},
	}
	require.NoError(t, s.CreateProject(t.Context(), p))
	return p
}

// TestStoreBlocks_SourceAddedThenModified exercises the StoreBlocks hash-diff
// path: a freshly stored block must classify as "source_added", and a
// content change on the same block must classify as "source_modified". This
// guards the existingHashes diff (including its rows.Err() check) against
// regressions that would mis-classify added vs modified entries.
func TestStoreBlocks_SourceAddedThenModified(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// First write → source_added.
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{model.NewBlock("b1", "Hello")}))

	cs, err := s.GetChanges(ctx, p.ID, "", 0, nil, 100)
	require.NoError(t, err)
	require.Len(t, cs.Changes, 1)
	assert.Equal(t, "b1", cs.Changes[0].BlockID)
	assert.Equal(t, "source_added", cs.Changes[0].ChangeType)
	assert.NotEmpty(t, cs.Changes[0].ContentHash)

	// Second write with changed content → source_modified (diffed against the
	// existingHashes map built from the prior write).
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{model.NewBlock("b1", "Hello World")}))

	cs, err = s.GetChanges(ctx, p.ID, "", 0, nil, 100)
	require.NoError(t, err)
	require.Len(t, cs.Changes, 2)
	assert.Equal(t, "source_added", cs.Changes[0].ChangeType)
	assert.Equal(t, "source_modified", cs.Changes[1].ChangeType)
}

// TestStoreBlocks_UnchangedSourceNotLogged ensures that re-storing identical
// content produces no spurious "source_modified" entry — the content_hash read
// back through the existingHashes diff must match exactly.
func TestStoreBlocks_UnchangedSourceNotLogged(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	b := model.NewBlock("b1", "Hello")
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{b}))
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{b}))

	cs, err := s.GetChanges(ctx, p.ID, "", 0, nil, 100)
	require.NoError(t, err)
	assert.Len(t, cs.Changes, 1) // Only the initial source_added.
}

// TestStoreBlocks_MultipleBlocksMixedClassification stores a batch where one
// block is new and another is modified in the same call, verifying the
// existingHashes map correctly distinguishes added from modified across rows.
func TestStoreBlocks_MultipleBlocksMixedClassification(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// Seed b1.
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{model.NewBlock("b1", "One")}))

	// Batch: b1 modified + b2 new.
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{
		model.NewBlock("b1", "One changed"),
		model.NewBlock("b2", "Two"),
	}))

	cs, err := s.GetChanges(ctx, p.ID, "", 0, nil, 100)
	require.NoError(t, err)
	require.Len(t, cs.Changes, 3)

	byBlock := map[string]string{}
	for _, c := range cs.Changes {
		// Last write wins per block for this assertion set; both b1 entries are
		// distinct change types, so collect them all.
		byBlock[c.BlockID+":"+c.ChangeType] = c.ChangeType
	}
	assert.Contains(t, byBlock, "b1:source_added")
	assert.Contains(t, byBlock, "b1:source_modified")
	assert.Contains(t, byBlock, "b2:source_added")
}
