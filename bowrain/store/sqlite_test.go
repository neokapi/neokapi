package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/gokapi/gokapi/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func createTestProject(t *testing.T, s *SQLiteStore) *Project {
	t.Helper()
	p := &Project{
		Name:          "Test Project",
		SourceLocale:  model.LocaleEnglish,
		TargetLocales: []model.LocaleID{model.LocaleFrench, model.LocaleGerman},
		Properties:    map[string]string{"client": "acme"},
	}
	require.NoError(t, s.CreateProject(context.Background(), p))
	return p
}

// ---------------------------------------------------------------------------
// Project CRUD
// ---------------------------------------------------------------------------

func TestProjectCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	t.Run("create and get", func(t *testing.T) {
		p := createTestProject(t, s)
		assert.NotEmpty(t, p.ID)

		got, err := s.GetProject(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, p.Name, got.Name)
		assert.Equal(t, p.SourceLocale, got.SourceLocale)
		assert.Equal(t, p.TargetLocales, got.TargetLocales)
		assert.Equal(t, "acme", got.Properties["client"])
	})

	t.Run("list", func(t *testing.T) {
		projects, err := s.ListProjects(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(projects), 1)
	})

	t.Run("update", func(t *testing.T) {
		p := createTestProject(t, s)
		p.Name = "Updated Name"
		require.NoError(t, s.UpdateProject(ctx, p))

		got, err := s.GetProject(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated Name", got.Name)
	})

	t.Run("delete", func(t *testing.T) {
		p := createTestProject(t, s)
		require.NoError(t, s.DeleteProject(ctx, p.ID))

		_, err := s.GetProject(ctx, p.ID)
		assert.Error(t, err)
	})

	t.Run("delete nonexistent", func(t *testing.T) {
		err := s.DeleteProject(ctx, "nonexistent")
		assert.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// Block storage
// ---------------------------------------------------------------------------

func TestBlockStorage(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	t.Run("store and get", func(t *testing.T) {
		b := model.NewBlock("b1", "Hello world")
		b.Name = "greeting"
		b.Type = "text"

		require.NoError(t, s.StoreBlocks(ctx, p.ID, []*model.Block{b}))

		got, err := s.GetBlock(ctx, p.ID, "b1")
		require.NoError(t, err)
		assert.Equal(t, "b1", got.ID)
		assert.Equal(t, "greeting", got.Name)
		assert.Equal(t, "Hello world", got.SourceText())
		assert.NotEmpty(t, got.ContentHash)
	})

	t.Run("upsert on conflict", func(t *testing.T) {
		b := model.NewBlock("b1", "Updated text")
		b.Name = "greeting"
		require.NoError(t, s.StoreBlocks(ctx, p.ID, []*model.Block{b}))

		got, err := s.GetBlock(ctx, p.ID, "b1")
		require.NoError(t, err)
		assert.Equal(t, "Updated text", got.SourceText())
	})

	t.Run("store with targets", func(t *testing.T) {
		b := model.NewBlock("b2", "Hello")
		b.SetTargetText(model.LocaleFrench, "Bonjour")
		require.NoError(t, s.StoreBlocks(ctx, p.ID, []*model.Block{b}))

		got, err := s.GetBlock(ctx, p.ID, "b2")
		require.NoError(t, err)
		assert.Equal(t, "Bonjour", got.TargetText(model.LocaleFrench))
	})

	t.Run("get blocks with query", func(t *testing.T) {
		blocks, err := s.GetBlocks(ctx, BlockQuery{ProjectID: p.ID})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(blocks), 2)
	})

	t.Run("get blocks by IDs", func(t *testing.T) {
		blocks, err := s.GetBlocks(ctx, BlockQuery{
			ProjectID: p.ID,
			IDs:       []string{"b1"},
		})
		require.NoError(t, err)
		assert.Len(t, blocks, 1)
		assert.Equal(t, "b1", blocks[0].ID)
	})

	t.Run("get blocks by content hash", func(t *testing.T) {
		hash := model.ComputeContentHash("Updated text")
		blocks, err := s.GetBlocks(ctx, BlockQuery{
			ProjectID:   p.ID,
			ContentHash: hash,
		})
		require.NoError(t, err)
		assert.Len(t, blocks, 1)
	})

	t.Run("pagination", func(t *testing.T) {
		blocks, err := s.GetBlocks(ctx, BlockQuery{ProjectID: p.ID, Limit: 1})
		require.NoError(t, err)
		assert.Len(t, blocks, 1)

		blocks2, err := s.GetBlocks(ctx, BlockQuery{ProjectID: p.ID, Limit: 1, Offset: 1})
		require.NoError(t, err)
		assert.Len(t, blocks2, 1)
		assert.NotEqual(t, blocks[0].ID, blocks2[0].ID)
	})

	t.Run("delete block", func(t *testing.T) {
		b := model.NewBlock("b-del", "Delete me")
		require.NoError(t, s.StoreBlocks(ctx, p.ID, []*model.Block{b}))
		require.NoError(t, s.DeleteBlock(ctx, p.ID, "b-del"))

		_, err := s.GetBlock(ctx, p.ID, "b-del")
		assert.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// Version management
// ---------------------------------------------------------------------------

func TestVersioning(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	// Store initial blocks.
	require.NoError(t, s.StoreBlocks(ctx, p.ID, []*model.Block{
		model.NewBlock("b1", "Hello"),
		model.NewBlock("b2", "World"),
	}))

	t.Run("create version", func(t *testing.T) {
		v, err := s.CreateVersion(ctx, p.ID, "v1.0", "Initial version")
		require.NoError(t, err)
		assert.Equal(t, "v1.0", v.Label)
		assert.Equal(t, 2, v.BlockCount)
	})

	t.Run("list versions", func(t *testing.T) {
		versions, err := s.ListVersions(ctx, p.ID)
		require.NoError(t, err)
		assert.Len(t, versions, 1)
		assert.Equal(t, "v1.0", versions[0].Label)
	})

	t.Run("get version", func(t *testing.T) {
		versions, err := s.ListVersions(ctx, p.ID)
		require.NoError(t, err)

		v, err := s.GetVersion(ctx, versions[0].ID)
		require.NoError(t, err)
		assert.Equal(t, "v1.0", v.Label)
	})

	t.Run("diff between versions", func(t *testing.T) {
		v1, err := s.ListVersions(ctx, p.ID)
		require.NoError(t, err)

		// Modify b1, add b3, remove b2.
		require.NoError(t, s.StoreBlocks(ctx, p.ID, []*model.Block{
			model.NewBlock("b1", "Hello modified"),
			model.NewBlock("b3", "New block"),
		}))
		require.NoError(t, s.DeleteBlock(ctx, p.ID, "b2"))

		v2, err := s.CreateVersion(ctx, p.ID, "v2.0", "Modified version")
		require.NoError(t, err)

		diff, err := s.Diff(ctx, v1[0].ID, v2.ID)
		require.NoError(t, err)

		changeMap := map[string]ChangeType{}
		for _, c := range diff.Changes {
			changeMap[c.BlockID] = c.ChangeType
		}

		assert.Equal(t, ChangeModified, changeMap["b1"])
		assert.Equal(t, ChangeRemoved, changeMap["b2"])
		assert.Equal(t, ChangeAdded, changeMap["b3"])
	})
}

// ---------------------------------------------------------------------------
// Concurrent access
// ---------------------------------------------------------------------------

func TestConcurrentAccess(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	// Store blocks concurrently.
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			b := model.NewBlock(
				fmt.Sprintf("concurrent-%d", i),
				fmt.Sprintf("Text %d", i),
			)
			done <- s.StoreBlocks(ctx, p.ID, []*model.Block{b})
		}(i)
	}

	for i := 0; i < 10; i++ {
		assert.NoError(t, <-done)
	}

	blocks, err := s.GetBlocks(ctx, BlockQuery{ProjectID: p.ID})
	require.NoError(t, err)
	assert.Len(t, blocks, 10)
}

// ---------------------------------------------------------------------------
// Item CRUD
// ---------------------------------------------------------------------------

func TestItemCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	t.Run("store and get", func(t *testing.T) {
		item := &Item{
			Name:        "messages.json",
			Format:      "json",
			ItemType:    "file",
			SourceBytes: []byte(`{"hello":"world"}`),
			Properties:  map[string]string{"encoding": "UTF-8"},
		}
		require.NoError(t, s.StoreItem(ctx, p.ID, item))

		got, err := s.GetItem(ctx, p.ID, "messages.json")
		require.NoError(t, err)
		assert.Equal(t, "messages.json", got.Name)
		assert.Equal(t, "json", got.Format)
		assert.Equal(t, "file", got.ItemType)
		assert.Equal(t, []byte(`{"hello":"world"}`), got.SourceBytes)
		assert.Equal(t, "UTF-8", got.Properties["encoding"])
		assert.NotZero(t, got.CreatedAt)
	})

	t.Run("upsert", func(t *testing.T) {
		item := &Item{
			Name:        "messages.json",
			Format:      "json",
			ItemType:    "file",
			SourceBytes: []byte(`{"hello":"updated"}`),
			Properties:  map[string]string{},
		}
		require.NoError(t, s.StoreItem(ctx, p.ID, item))

		got, err := s.GetItem(ctx, p.ID, "messages.json")
		require.NoError(t, err)
		assert.Equal(t, []byte(`{"hello":"updated"}`), got.SourceBytes)
	})

	t.Run("list", func(t *testing.T) {
		item2 := &Item{
			Name:     "strings.xml",
			Format:   "xml",
			ItemType: "file",
		}
		require.NoError(t, s.StoreItem(ctx, p.ID, item2))

		items, err := s.ListItems(ctx, p.ID)
		require.NoError(t, err)
		assert.Len(t, items, 2)
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, s.DeleteItem(ctx, p.ID, "strings.xml"))

		items, err := s.ListItems(ctx, p.ID)
		require.NoError(t, err)
		assert.Len(t, items, 1)
	})

	t.Run("delete nonexistent", func(t *testing.T) {
		err := s.DeleteItem(ctx, p.ID, "nonexistent.txt")
		assert.Error(t, err)
	})

	t.Run("get nonexistent", func(t *testing.T) {
		_, err := s.GetItem(ctx, p.ID, "nonexistent.txt")
		assert.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// Block-Item association
// ---------------------------------------------------------------------------

func TestBlockItemAssociation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := createTestProject(t, s)

	// Store an item.
	item := &Item{Name: "messages.json", Format: "json", ItemType: "file"}
	require.NoError(t, s.StoreItem(ctx, p.ID, item))

	// Store blocks associated with the item.
	b1 := model.NewBlock("msg-1", "Hello")
	b2 := model.NewBlock("msg-2", "Goodbye")
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "messages.json", []*model.Block{b1, b2}))

	// Store blocks without item association.
	b3 := model.NewBlock("other-1", "Other")
	require.NoError(t, s.StoreBlocks(ctx, p.ID, []*model.Block{b3}))

	t.Run("query by item name", func(t *testing.T) {
		blocks, err := s.GetBlocks(ctx, BlockQuery{
			ProjectID: p.ID,
			ItemName:  "messages.json",
		})
		require.NoError(t, err)
		assert.Len(t, blocks, 2)
		for _, sb := range blocks {
			assert.Equal(t, "messages.json", sb.ItemName)
		}
	})

	t.Run("query all blocks", func(t *testing.T) {
		blocks, err := s.GetBlocks(ctx, BlockQuery{ProjectID: p.ID})
		require.NoError(t, err)
		assert.Len(t, blocks, 3)
	})

	t.Run("get block has item_name", func(t *testing.T) {
		sb, err := s.GetBlock(ctx, p.ID, "msg-1")
		require.NoError(t, err)
		assert.Equal(t, "messages.json", sb.ItemName)
	})

	t.Run("delete item cascades blocks", func(t *testing.T) {
		require.NoError(t, s.DeleteItem(ctx, p.ID, "messages.json"))

		blocks, err := s.GetBlocks(ctx, BlockQuery{ProjectID: p.ID})
		require.NoError(t, err)
		assert.Len(t, blocks, 1)
		assert.Equal(t, "other-1", blocks[0].ID)
	})
}
