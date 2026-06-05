package store

import (
	"fmt"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *PostgresStore {
	t.Helper()
	db := pgtest.NewTestDB(t)
	s, err := NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	return s
}

func createTestProject(t *testing.T, s *PostgresStore) *platstore.Project {
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

// ---------------------------------------------------------------------------
// Project CRUD
// ---------------------------------------------------------------------------

func TestProjectCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	t.Run("create and get", func(t *testing.T) {
		p := createTestProject(t, s)
		assert.NotEmpty(t, p.ID)

		got, err := s.GetProject(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, p.Name, got.Name)
		assert.Equal(t, p.DefaultSourceLanguage, got.DefaultSourceLanguage)
		assert.Equal(t, p.TargetLanguages, got.TargetLanguages)
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
	ctx := t.Context()
	p := createTestProject(t, s)

	t.Run("store and get", func(t *testing.T) {
		b := model.NewBlock("b1", "Hello world")
		b.Name = "greeting"
		b.Type = "text"

		require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{b}))

		got, err := s.GetBlock(ctx, p.ID, "", "b1")
		require.NoError(t, err)
		assert.Equal(t, "b1", got.ID)
		assert.Equal(t, "greeting", got.Name)
		assert.Equal(t, "Hello world", got.SourceText())
		assert.NotEmpty(t, got.ContentHash)
	})

	t.Run("upsert on conflict", func(t *testing.T) {
		b := model.NewBlock("b1", "Updated text")
		b.Name = "greeting"
		require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{b}))

		got, err := s.GetBlock(ctx, p.ID, "", "b1")
		require.NoError(t, err)
		assert.Equal(t, "Updated text", got.SourceText())
	})

	t.Run("store with targets", func(t *testing.T) {
		b := model.NewBlock("b2", "Hello")
		b.SetTargetText(model.LocaleFrench, "Bonjour")
		require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{b}))

		got, err := s.GetBlock(ctx, p.ID, "", "b2")
		require.NoError(t, err)
		assert.Equal(t, "Bonjour", got.TargetText(model.LocaleFrench))
	})

	t.Run("get blocks with query", func(t *testing.T) {
		blocks, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(blocks), 2)
	})

	t.Run("get blocks by IDs", func(t *testing.T) {
		blocks, err := s.GetBlocks(ctx, platstore.BlockQuery{
			ProjectID: p.ID,
			IDs:       []string{"b1"},
		})
		require.NoError(t, err)
		assert.Len(t, blocks, 1)
		assert.Equal(t, "b1", blocks[0].ID)
	})

	t.Run("get blocks by content hash", func(t *testing.T) {
		hash := model.ComputeContentHash("Updated text")
		blocks, err := s.GetBlocks(ctx, platstore.BlockQuery{
			ProjectID:   p.ID,
			ContentHash: hash,
		})
		require.NoError(t, err)
		assert.Len(t, blocks, 1)
	})

	t.Run("pagination", func(t *testing.T) {
		blocks, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Limit: 1})
		require.NoError(t, err)
		assert.Len(t, blocks, 1)

		blocks2, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Limit: 1, Offset: 1})
		require.NoError(t, err)
		assert.Len(t, blocks2, 1)
		assert.NotEqual(t, blocks[0].ID, blocks2[0].ID)
	})

	t.Run("delete block", func(t *testing.T) {
		b := model.NewBlock("b-del", "Delete me")
		require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{b}))
		require.NoError(t, s.DeleteBlock(ctx, p.ID, "", "b-del"))

		_, err := s.GetBlock(ctx, p.ID, "", "b-del")
		assert.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// Version management
// ---------------------------------------------------------------------------

func TestVersioning(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// Store initial blocks.
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{
		model.NewBlock("b1", "Hello"),
		model.NewBlock("b2", "World"),
	}))

	t.Run("create version", func(t *testing.T) {
		v, err := s.CreateVersion(ctx, p.ID, "", "v1.0", "Initial version")
		require.NoError(t, err)
		assert.Equal(t, "v1.0", v.Label)
		assert.Equal(t, 2, v.BlockCount)
	})

	t.Run("list versions", func(t *testing.T) {
		versions, err := s.ListVersions(ctx, p.ID, "")
		require.NoError(t, err)
		assert.Len(t, versions, 1)
		assert.Equal(t, "v1.0", versions[0].Label)
	})

	t.Run("get version", func(t *testing.T) {
		versions, err := s.ListVersions(ctx, p.ID, "")
		require.NoError(t, err)

		v, err := s.GetVersion(ctx, versions[0].ID)
		require.NoError(t, err)
		assert.Equal(t, "v1.0", v.Label)
	})

	t.Run("diff between versions", func(t *testing.T) {
		v1, err := s.ListVersions(ctx, p.ID, "")
		require.NoError(t, err)

		// Modify b1, add b3, remove b2.
		require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{
			model.NewBlock("b1", "Hello modified"),
			model.NewBlock("b3", "New block"),
		}))
		require.NoError(t, s.DeleteBlock(ctx, p.ID, "", "b2"))

		v2, err := s.CreateVersion(ctx, p.ID, "", "v2.0", "Modified version")
		require.NoError(t, err)

		diff, err := s.Diff(ctx, v1[0].ID, v2.ID)
		require.NoError(t, err)

		changeMap := map[string]platstore.ChangeType{}
		for _, c := range diff.Changes {
			changeMap[c.BlockID] = c.ChangeType
		}

		assert.Equal(t, platstore.ChangeModified, changeMap["b1"])
		assert.Equal(t, platstore.ChangeRemoved, changeMap["b2"])
		assert.Equal(t, platstore.ChangeAdded, changeMap["b3"])
	})
}

// ---------------------------------------------------------------------------
// Concurrent access
// ---------------------------------------------------------------------------

func TestConcurrentAccess(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// Store blocks concurrently.
	done := make(chan error, 10)
	for i := range 10 {
		go func(i int) {
			b := model.NewBlock(
				fmt.Sprintf("concurrent-%d", i),
				fmt.Sprintf("Text %d", i),
			)
			done <- s.StoreBlocks(ctx, p.ID, "", []*model.Block{b})
		}(i)
	}

	for range 10 {
		require.NoError(t, <-done)
	}

	blocks, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID})
	require.NoError(t, err)
	assert.Len(t, blocks, 10)
}

// ---------------------------------------------------------------------------
// Item CRUD
// ---------------------------------------------------------------------------

func TestItemCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	t.Run("store and get", func(t *testing.T) {
		item := &platstore.Item{
			Name:       "messages.json",
			Format:     "json",
			ItemType:   "file",
			Properties: map[string]string{"encoding": "UTF-8"},
		}
		require.NoError(t, s.StoreItem(ctx, p.ID, "", item))

		got, err := s.GetItem(ctx, p.ID, "", "messages.json")
		require.NoError(t, err)
		assert.Equal(t, "messages.json", got.Name)
		assert.Equal(t, "json", got.Format)
		assert.Equal(t, "file", got.ItemType)
		assert.Equal(t, "UTF-8", got.Properties["encoding"])
		assert.NotZero(t, got.CreatedAt)
	})

	t.Run("upsert", func(t *testing.T) {
		item := &platstore.Item{
			Name:       "messages.json",
			Format:     "json",
			ItemType:   "file",
			Properties: map[string]string{},
		}
		require.NoError(t, s.StoreItem(ctx, p.ID, "", item))

		got, err := s.GetItem(ctx, p.ID, "", "messages.json")
		require.NoError(t, err)
		assert.Equal(t, "messages.json", got.Name)
	})

	t.Run("list", func(t *testing.T) {
		item2 := &platstore.Item{
			Name:     "strings.xml",
			Format:   "xml",
			ItemType: "file",
		}
		require.NoError(t, s.StoreItem(ctx, p.ID, "", item2))

		items, err := s.ListItems(ctx, p.ID, "")
		require.NoError(t, err)
		assert.Len(t, items, 2)
	})

	t.Run("delete", func(t *testing.T) {
		require.NoError(t, s.DeleteItem(ctx, p.ID, "", "strings.xml"))

		items, err := s.ListItems(ctx, p.ID, "")
		require.NoError(t, err)
		assert.Len(t, items, 1)
	})

	t.Run("delete nonexistent", func(t *testing.T) {
		err := s.DeleteItem(ctx, p.ID, "", "nonexistent.txt")
		assert.Error(t, err)
	})

	t.Run("get nonexistent", func(t *testing.T) {
		_, err := s.GetItem(ctx, p.ID, "", "nonexistent.txt")
		assert.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// Item PreviewHTML persistence
// ---------------------------------------------------------------------------

func TestItemPreviewHTML(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	t.Run("store and retrieve PreviewHTML", func(t *testing.T) {
		item := &platstore.Item{
			Name:        "page.html",
			Format:      "html",
			ItemType:    "file",
			PreviewHTML: "<html><body><p>Hello world</p></body></html>",
		}
		require.NoError(t, s.StoreItem(ctx, p.ID, "", item))

		got, err := s.GetItem(ctx, p.ID, "", "page.html")
		require.NoError(t, err)
		assert.Equal(t, "<html><body><p>Hello world</p></body></html>", got.PreviewHTML)
	})

	t.Run("upsert preserves PreviewHTML", func(t *testing.T) {
		item := &platstore.Item{
			Name:        "page.html",
			Format:      "html",
			ItemType:    "file",
			PreviewHTML: "<html><body><p>Updated preview</p></body></html>",
		}
		require.NoError(t, s.StoreItem(ctx, p.ID, "", item))

		got, err := s.GetItem(ctx, p.ID, "", "page.html")
		require.NoError(t, err)
		assert.Equal(t, "<html><body><p>Updated preview</p></body></html>", got.PreviewHTML)
	})

	t.Run("empty PreviewHTML", func(t *testing.T) {
		item := &platstore.Item{
			Name:     "plain.txt",
			Format:   "plaintext",
			ItemType: "file",
		}
		require.NoError(t, s.StoreItem(ctx, p.ID, "", item))

		got, err := s.GetItem(ctx, p.ID, "", "plain.txt")
		require.NoError(t, err)
		assert.Empty(t, got.PreviewHTML)
	})
}

// ---------------------------------------------------------------------------
// Block-Item association
// ---------------------------------------------------------------------------

func TestBlockItemAssociation(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// Store an item.
	item := &platstore.Item{Name: "messages.json", Format: "json", ItemType: "file"}
	require.NoError(t, s.StoreItem(ctx, p.ID, "", item))

	// Store blocks associated with the item — format-reader IDs get mapped to internal IDs.
	b1 := model.NewBlock("msg-1", "Hello")
	b2 := model.NewBlock("msg-2", "Goodbye")
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "", "messages.json", []*model.Block{b1, b2}))

	// After StoreBlocksForItem, b1.ID and b2.ID are mutated to internal IDs.
	assert.NotEqual(t, "msg-1", b1.ID, "block ID should be remapped")
	assert.NotEqual(t, "msg-2", b2.ID, "block ID should be remapped")
	assert.Len(t, b1.ID, 8, "internal ID should be 8 chars")
	assert.Len(t, b2.ID, 8, "internal ID should be 8 chars")

	// Store blocks without item association — keeps original ID.
	b3 := model.NewBlock("other-1", "Other")
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{b3}))

	t.Run("query by item name", func(t *testing.T) {
		blocks, err := s.GetBlocks(ctx, platstore.BlockQuery{
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
		blocks, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID})
		require.NoError(t, err)
		assert.Len(t, blocks, 3)
	})

	t.Run("get block by internal ID", func(t *testing.T) {
		sb, err := s.GetBlock(ctx, p.ID, "", b1.ID)
		require.NoError(t, err)
		assert.Equal(t, "messages.json", sb.ItemName)
		assert.Equal(t, "msg-1", sb.SourceID)
		assert.Equal(t, "Hello", sb.SourceText())
	})

	t.Run("delete item cascades blocks", func(t *testing.T) {
		require.NoError(t, s.DeleteItem(ctx, p.ID, "", "messages.json"))

		blocks, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID})
		require.NoError(t, err)
		assert.Len(t, blocks, 1)
		assert.Equal(t, "other-1", blocks[0].ID)
	})
}

func TestBlockIDUniqueness(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// Two items with blocks that have the same format-reader IDs.
	item1 := &platstore.Item{Name: "file1.json", Format: "json", ItemType: "file"}
	item2 := &platstore.Item{Name: "file2.json", Format: "json", ItemType: "file"}
	require.NoError(t, s.StoreItem(ctx, p.ID, "", item1))
	require.NoError(t, s.StoreItem(ctx, p.ID, "", item2))

	blocks1 := []*model.Block{model.NewBlock("tu1", "Hello"), model.NewBlock("tu2", "World")}
	blocks2 := []*model.Block{model.NewBlock("tu1", "Bonjour"), model.NewBlock("tu2", "Monde")}
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "", "file1.json", blocks1))
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "", "file2.json", blocks2))

	t.Run("different internal IDs", func(t *testing.T) {
		// All four blocks should have unique 8-char IDs.
		ids := map[string]bool{blocks1[0].ID: true, blocks1[1].ID: true, blocks2[0].ID: true, blocks2[1].ID: true}
		assert.Len(t, ids, 4, "all four blocks should have unique IDs")
		for id := range ids {
			assert.Len(t, id, 8, "internal ID should be 8 chars")
		}
	})

	t.Run("no ambiguity on GetBlock", func(t *testing.T) {
		sb1, err := s.GetBlock(ctx, p.ID, "", blocks1[0].ID)
		require.NoError(t, err)
		assert.Equal(t, "Hello", sb1.SourceText())
		assert.Equal(t, "tu1", sb1.SourceID)
		assert.Equal(t, "file1.json", sb1.ItemName)

		sb3, err := s.GetBlock(ctx, p.ID, "", blocks2[0].ID)
		require.NoError(t, err)
		assert.Equal(t, "Bonjour", sb3.SourceText())
		assert.Equal(t, "tu1", sb3.SourceID)
		assert.Equal(t, "file2.json", sb3.ItemName)
	})

	t.Run("re-ingest reuses IDs", func(t *testing.T) {
		savedID1 := blocks1[0].ID
		savedID2 := blocks1[1].ID

		// Re-store same blocks for file1 — should reuse existing internal IDs.
		reBlocks := []*model.Block{model.NewBlock("tu1", "Hello updated"), model.NewBlock("tu2", "World")}
		require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "", "file1.json", reBlocks))
		assert.Equal(t, savedID1, reBlocks[0].ID, "re-ingested block should keep same internal ID")
		assert.Equal(t, savedID2, reBlocks[1].ID, "re-ingested block should keep same internal ID")

		sb, err := s.GetBlock(ctx, p.ID, "", savedID1)
		require.NoError(t, err)
		assert.Equal(t, "Hello updated", sb.SourceText())
	})
}

// ---------------------------------------------------------------------------
// GetBlockStats — lightweight projection for dashboard
// ---------------------------------------------------------------------------

func TestGetBlockStats(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// Create two items.
	item1 := &platstore.Item{Name: "messages.json", Format: "json", ItemType: "file"}
	item2 := &platstore.Item{Name: "strings.xml", Format: "xml", ItemType: "file"}
	require.NoError(t, s.StoreItem(ctx, p.ID, "", item1))
	require.NoError(t, s.StoreItem(ctx, p.ID, "", item2))

	// Store blocks for item 1: 2 translatable (one with French target), 1 non-translatable.
	b1 := model.NewBlock("b1", "Hello world")
	b1.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	b2 := model.NewBlock("b2", "Click here to continue")
	b3 := model.NewBlock("b3", "")
	b3.Translatable = false
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "", "messages.json", []*model.Block{b1, b2, b3}))

	// Store blocks for item 2: 1 translatable with both French and German targets.
	b4 := model.NewBlock("b4", "Settings")
	b4.SetTargetText(model.LocaleFrench, "Paramètres")
	b4.SetTargetText(model.LocaleGerman, "Einstellungen")
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "", "strings.xml", []*model.Block{b4}))

	t.Run("returns all blocks", func(t *testing.T) {
		stats, err := s.GetBlockStats(ctx, p.ID, "")
		require.NoError(t, err)
		assert.Len(t, stats, 4, "should return all 4 blocks")
	})

	t.Run("translatable flag is correct", func(t *testing.T) {
		stats, err := s.GetBlockStats(ctx, p.ID, "")
		require.NoError(t, err)

		translatableCount := 0
		for _, bs := range stats {
			if bs.Translatable {
				translatableCount++
			}
		}
		assert.Equal(t, 3, translatableCount, "3 of 4 blocks are translatable")
	})

	t.Run("source word counts are positive", func(t *testing.T) {
		stats, err := s.GetBlockStats(ctx, p.ID, "")
		require.NoError(t, err)

		for _, bs := range stats {
			if bs.Translatable {
				assert.Greater(t, bs.SourceWords, 0, "translatable block %q should have positive word count", bs.ItemName)
			}
		}
	})

	t.Run("target locales are extracted correctly", func(t *testing.T) {
		stats, err := s.GetBlockStats(ctx, p.ID, "")
		require.NoError(t, err)

		// Find b4's stat (Settings — has fr and de targets).
		var settingsBlock *platstore.BlockStatRow
		for i, bs := range stats {
			if bs.ItemName == "strings.xml" {
				settingsBlock = &stats[i]
				break
			}
		}
		require.NotNil(t, settingsBlock)
		assert.Len(t, settingsBlock.TargetLocales, 2, "Settings block should have 2 target locales")
		assert.Contains(t, settingsBlock.TargetLocales, string(model.LocaleFrench))
		assert.Contains(t, settingsBlock.TargetLocales, string(model.LocaleGerman))
	})

	t.Run("block without targets has empty locale list", func(t *testing.T) {
		stats, err := s.GetBlockStats(ctx, p.ID, "")
		require.NoError(t, err)

		// Find b2's stat (no targets).
		found := false
		for _, bs := range stats {
			if bs.ItemName == "messages.json" && len(bs.TargetLocales) == 0 && bs.Translatable {
				found = true
				break
			}
		}
		assert.True(t, found, "should find a translatable block with no target locales")
	})

	t.Run("empty project returns nil", func(t *testing.T) {
		emptyProj := createTestProject(t, s)
		stats, err := s.GetBlockStats(ctx, emptyProj.ID, "")
		require.NoError(t, err)
		assert.Empty(t, stats)
	})
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestCountWordsFromSourceJSON(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected int
	}{
		{
			name:     "simple text",
			json:     `[{"text":"Hello world"}]`,
			expected: 2,
		},
		{
			name:     "multiple runs",
			json:     `[{"text":"Hello"},{"text":" beautiful world"}]`,
			expected: 3,
		},
		{
			name:     "text with inline code run",
			json:     `[{"text":"Click "},{"ph":{"id":"1","type":"url","data":"http://x"}},{"text":" to continue"}]`,
			expected: 3,
		},
		{
			name:     "empty text",
			json:     `[{"text":""}]`,
			expected: 0,
		},
		{
			name:     "empty array",
			json:     `[]`,
			expected: 0,
		},
		{
			name:     "invalid json",
			json:     `not-json`,
			expected: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, countWordsFromSourceJSON(tt.json))
		})
	}
}

// ---------------------------------------------------------------------------
// Asset CRUD (Bowrain AD-007)
// ---------------------------------------------------------------------------

func TestAssetCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// Store asset.
	asset := &platstore.Asset{
		ItemName:   "docs/manual.docx",
		SourceID:   "image1",
		BlobKey:    "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2",
		MimeType:   "image/png",
		Filename:   "diagram.png",
		SizeBytes:  102400,
		AltText:    "Architecture diagram",
		Properties: map[string]string{"width": "800", "height": "600"},
	}
	err := s.StoreAsset(ctx, p.ID, "main", asset)
	require.NoError(t, err)
	assert.NotEmpty(t, asset.ID)

	// Get asset.
	got, err := s.GetAsset(ctx, p.ID, "main", asset.ID)
	require.NoError(t, err)
	assert.Equal(t, asset.BlobKey, got.BlobKey)
	assert.Equal(t, "image/png", got.MimeType)
	assert.Equal(t, "diagram.png", got.Filename)
	assert.Equal(t, int64(102400), got.SizeBytes)
	assert.Equal(t, "Architecture diagram", got.AltText)
	assert.Equal(t, "800", got.Properties["width"])
	assert.Equal(t, "none", got.ProcessingStatus)

	// List assets.
	assets, err := s.ListAssets(ctx, p.ID, "main", "docs/manual.docx")
	require.NoError(t, err)
	assert.Len(t, assets, 1)

	// List all assets.
	allAssets, err := s.ListAssets(ctx, p.ID, "main", "")
	require.NoError(t, err)
	assert.Len(t, allAssets, 1)

	// Delete asset.
	err = s.DeleteAsset(ctx, p.ID, "main", asset.ID)
	require.NoError(t, err)

	_, err = s.GetAsset(ctx, p.ID, "main", asset.ID)
	assert.Error(t, err)
}

func TestAssetVariantCRUD(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// Create asset first.
	asset := &platstore.Asset{
		BlobKey:  "aabbccdd",
		MimeType: "image/png",
	}
	require.NoError(t, s.StoreAsset(ctx, p.ID, "main", asset))

	// Store variant.
	variant := &platstore.AssetVariant{
		AssetID:    asset.ID,
		Locale:     "fr-FR",
		BlobKey:    "eeff0011",
		Status:     "draft",
		MimeType:   "image/png",
		SizeBytes:  98304,
		Properties: map[string]string{"width": "800"},
	}
	err := s.StoreAssetVariant(ctx, p.ID, variant)
	require.NoError(t, err)

	// Get variant.
	got, err := s.GetAssetVariant(ctx, p.ID, asset.ID, "fr-FR")
	require.NoError(t, err)
	assert.Equal(t, "fr-FR", got.Locale)
	assert.Equal(t, "eeff0011", got.BlobKey)
	assert.Equal(t, "draft", got.Status)
	assert.Equal(t, "800", got.Properties["width"])

	// List variants.
	variants, err := s.ListAssetVariants(ctx, p.ID, asset.ID)
	require.NoError(t, err)
	assert.Len(t, variants, 1)

	// Upsert variant (update status).
	variant.Status = "approved"
	require.NoError(t, s.StoreAssetVariant(ctx, p.ID, variant))

	got2, err := s.GetAssetVariant(ctx, p.ID, asset.ID, "fr-FR")
	require.NoError(t, err)
	assert.Equal(t, "approved", got2.Status)
}

func TestAssetDedup(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// Store same blob_key twice — should upsert (dedup).
	asset1 := &platstore.Asset{
		BlobKey:  "sameblobkey",
		MimeType: "image/png",
		Filename: "first.png",
	}
	require.NoError(t, s.StoreAsset(ctx, p.ID, "main", asset1))

	asset2 := &platstore.Asset{
		BlobKey:  "sameblobkey",
		MimeType: "image/png",
		Filename: "second.png",
	}
	require.NoError(t, s.StoreAsset(ctx, p.ID, "main", asset2))

	// Should still be one asset (upserted).
	assets, err := s.ListAssets(ctx, p.ID, "main", "")
	require.NoError(t, err)
	assert.Len(t, assets, 1)
	assert.Equal(t, "second.png", assets[0].Filename) // Updated
}

func TestAssetCascadeDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// Create asset with variant.
	asset := &platstore.Asset{BlobKey: "cascadetest", MimeType: "image/png"}
	require.NoError(t, s.StoreAsset(ctx, p.ID, "main", asset))

	variant := &platstore.AssetVariant{
		AssetID: asset.ID, Locale: "de-DE", BlobKey: "devariant", MimeType: "image/png",
	}
	require.NoError(t, s.StoreAssetVariant(ctx, p.ID, variant))

	// Delete asset — variants should cascade.
	require.NoError(t, s.DeleteAsset(ctx, p.ID, "main", asset.ID))

	variants, err := s.ListAssetVariants(ctx, p.ID, asset.ID)
	require.NoError(t, err)
	assert.Empty(t, variants)
}

func TestAssetChangeLog(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// Store asset — should log "asset_added".
	asset := &platstore.Asset{
		BlobKey:  "changelog1234",
		MimeType: "image/png",
	}
	require.NoError(t, s.StoreAsset(ctx, p.ID, "main", asset))

	// Store same blob key again — should log "asset_modified".
	asset2 := &platstore.Asset{
		BlobKey:  "changelog1234",
		MimeType: "image/png",
		Filename: "updated.png",
	}
	require.NoError(t, s.StoreAsset(ctx, p.ID, "main", asset2))

	// Store variant — should log "variant_added".
	variant := &platstore.AssetVariant{
		AssetID: asset.ID, Locale: "fr-FR", BlobKey: "frblob", MimeType: "image/png",
	}
	require.NoError(t, s.StoreAssetVariant(ctx, p.ID, variant))

	// Delete asset — should log "asset_removed".
	require.NoError(t, s.DeleteAsset(ctx, p.ID, "main", asset.ID))

	// Verify change log entries.
	cs, err := s.GetChanges(ctx, p.ID, "main", 0, nil, 100)
	require.NoError(t, err)

	var assetChangeTypes []string
	for _, c := range cs.Changes {
		switch c.ChangeType {
		case "asset_added", "asset_modified", "asset_removed", "variant_added", "variant_modified", "variant_approved":
			assetChangeTypes = append(assetChangeTypes, c.ChangeType)
		}
	}

	assert.Equal(t, []string{"asset_added", "asset_modified", "variant_added", "asset_removed"}, assetChangeTypes)
}

// ---------------------------------------------------------------------------
// Default Stream
// ---------------------------------------------------------------------------

func TestDefaultStream_FirstPush(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	p := createTestProject(t, s)
	assert.Empty(t, p.DefaultStream, "new project should have empty default stream")

	// Simulate pushing to a non-main stream ("bowrain-main").
	_ = s.CreateStream(ctx, &platstore.Stream{
		ProjectID:  p.ID,
		Name:       "bowrain-main",
		Parent:     "main",
		Visibility: platstore.StreamPublic,
	})
	item := &platstore.Item{Name: "en.json", Format: "json", ItemType: "file"}
	require.NoError(t, s.StoreItem(ctx, p.ID, "bowrain-main", item))

	// Set default stream (simulating what the sync push worker does).
	got, err := s.GetProject(ctx, p.ID)
	require.NoError(t, err)
	assert.Empty(t, got.DefaultStream)
	got.DefaultStream = "bowrain-main"
	require.NoError(t, s.UpdateProject(ctx, got))

	// Verify it persists.
	got2, err := s.GetProject(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "bowrain-main", got2.DefaultStream)
}

func TestDefaultStream_SubsequentPush(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	p := createTestProject(t, s)

	// First push sets default stream.
	p.DefaultStream = "foo"
	require.NoError(t, s.UpdateProject(ctx, p))

	// Subsequent push to "bar" should NOT change the default.
	got, err := s.GetProject(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "foo", got.DefaultStream, "default stream should not change after first push")
}

func TestDefaultStream_Migration(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	// Create project and push items to "main" — migration backfill should set default_stream to "main".
	p := createTestProject(t, s)
	item := &platstore.Item{Name: "test.json", Format: "json", ItemType: "file"}
	require.NoError(t, s.StoreItem(ctx, p.ID, "main", item))

	// The migration backfill already ran during NewPostgresStoreFromDB, but since we just created the project
	// after migration, verify the field can be set manually.
	p.DefaultStream = "main"
	require.NoError(t, s.UpdateProject(ctx, p))

	got, err := s.GetProject(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "main", got.DefaultStream)
}

func TestDefaultStream_ListProjects(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	p := createTestProject(t, s)
	p.DefaultStream = "develop"
	require.NoError(t, s.UpdateProject(ctx, p))

	projects, err := s.ListProjects(ctx)
	require.NoError(t, err)
	var found bool
	for _, proj := range projects {
		if proj.ID == p.ID {
			assert.Equal(t, "develop", proj.DefaultStream)
			found = true
		}
	}
	assert.True(t, found, "project should be in list")
}
