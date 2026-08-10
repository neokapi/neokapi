package server

import (
	"encoding/json"
	"path/filepath"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	coresync "github.com/neokapi/neokapi/bowrain/core/sync"
	pgstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedCoordinateProject creates a project (en → fr) whose four items cover
// every shape a collection rollup can take:
//
//	web.json    → "col-web", a collection with coordinates and a channel
//	docs.json   → "col-docs", a collection declaring neither
//	orphan.json → "col-gone", a collection id with no row behind it
//	loose.json  → no collection at all (the ungrouped bucket)
func seedCoordinateProject(t *testing.T, cs platstore.ContentStore) string {
	t.Helper()
	ctx := t.Context()

	proj := &platstore.Project{
		Name:                  "coords",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
		WorkspaceID:           "ws-1",
		Properties:            map[string]string{},
	}
	require.NoError(t, cs.CreateProject(ctx, proj))

	require.NoError(t, cs.CreateCollection(ctx, &platstore.Collection{
		ID:        "col-web",
		ProjectID: proj.ID,
		Name:      "Web",
		Kind:      platstore.CollectionConnected,
		Stream:    "main",
		Context:   map[string]string{"product": "kapimart", "surface": "web"},
		Owner:     coresync.ContextOwnerRecipe,
		ConnectorConfig: map[string]string{
			coreprofile.PropertyChannel: "web",
			"type":                      "github",
		},
	}))
	require.NoError(t, cs.CreateCollection(ctx, &platstore.Collection{
		ID:        "col-docs",
		ProjectID: proj.ID,
		Name:      "Docs",
		Kind:      platstore.CollectionUploaded,
		Stream:    "main",
	}))

	seed := func(item, collectionID, text string) {
		b := &model.Block{ID: item + "-b1", Translatable: true}
		b.SetSourceText(text)
		require.NoError(t, cs.StoreItem(ctx, proj.ID, "main", &platstore.Item{
			ProjectID: proj.ID, Name: item, Format: "json", ItemType: "file",
			CollectionID: collectionID,
		}))
		require.NoError(t, cs.StoreBlocksForItem(ctx, proj.ID, "main", item, []*model.Block{b}))
	}
	seed("web.json", "col-web", "one two")
	seed("docs.json", "col-docs", "one two three")
	seed("orphan.json", "col-gone", "one")
	seed("loose.json", "", "one")

	return proj.ID
}

func collStatByID(t *testing.T, stats []platstore.CollectionTranslationStats, id string) platstore.CollectionTranslationStats {
	t.Helper()
	for _, c := range stats {
		if c.CollectionID == id {
			return c
		}
	}
	t.Fatalf("collection stat %q not found", id)
	return platstore.CollectionTranslationStats{}
}

func itemStatByName(t *testing.T, stats []platstore.ItemTranslationStats, name string) platstore.ItemTranslationStats {
	t.Helper()
	for _, it := range stats {
		if it.ItemName == name {
			return it
		}
	}
	t.Fatalf("item stat %q not found", name)
	return platstore.ItemTranslationStats{}
}

// TestDashboardCollectionCoordinates asserts the dashboard rollups project the
// channel and coordinates their collection rows persist, name the collection on
// every item row, and aggregate collection-less items under one ungrouped
// bucket — on both engines, since each scans the collection row its own way.
func TestDashboardCollectionCoordinates(t *testing.T) {
	engines := []struct {
		name string
		open func(t *testing.T) platstore.ContentStore
	}{
		{
			name: "sqlite",
			open: func(t *testing.T) platstore.ContentStore {
				cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "dash.db"))
				require.NoError(t, err)
				t.Cleanup(func() { _ = cs.Close() })
				return cs
			},
		},
		{
			name: "postgres",
			open: func(t *testing.T) platstore.ContentStore {
				db := pgtest.NewTestDB(t)
				cs, err := pgstore.NewPostgresStoreFromDB(db)
				require.NoError(t, err)
				return cs
			},
		},
	}

	for _, eng := range engines {
		t.Run(eng.name, func(t *testing.T) {
			cs := eng.open(t)
			pid := seedCoordinateProject(t, cs)

			ctx := t.Context()
			proj, err := cs.GetProject(ctx, pid)
			require.NoError(t, err)
			stats, err := editorGetDashboardStats(ctx, cs, proj, "main")
			require.NoError(t, err)

			require.Len(t, stats.CollectionStats, 4)

			web := collStatByID(t, stats.CollectionStats, "col-web")
			assert.Equal(t, "Web", web.CollectionName)
			assert.Equal(t, "web", web.Channel)
			assert.Equal(t, map[string]string{"product": "kapimart", "surface": "web"}, web.Coordinates)
			assert.False(t, web.Ungrouped)
			assert.Equal(t, 1, web.ItemCount)

			docs := collStatByID(t, stats.CollectionStats, "col-docs")
			assert.Equal(t, "Docs", docs.CollectionName)
			assert.Empty(t, docs.Channel, "a collection binding no channel carries none")
			assert.Empty(t, docs.Coordinates, "a collection at no declared point carries no coordinates")

			orphan := collStatByID(t, stats.CollectionStats, "col-gone")
			assert.Empty(t, orphan.CollectionName, "no row behind the id, so no name")
			assert.False(t, orphan.Ungrouped, "an unresolvable id is not the ungrouped bucket")

			ungrouped := collStatByID(t, stats.CollectionStats, "")
			assert.True(t, ungrouped.Ungrouped)
			assert.Empty(t, ungrouped.CollectionName, "naming the bucket is the consumer's call")
			assert.Equal(t, 1, ungrouped.ItemCount)
			assert.Equal(t, []string{"col-gone", "col-docs", "col-web", ""}, collIDs(stats.CollectionStats),
				"rollups are ordered by name (id when nameless), ungrouped last")

			assert.Equal(t, "Web", itemStatByName(t, stats.ItemStats, "web.json").CollectionName)
			assert.Equal(t, "Docs", itemStatByName(t, stats.ItemStats, "docs.json").CollectionName)
			assert.Empty(t, itemStatByName(t, stats.ItemStats, "orphan.json").CollectionName)
			assert.Empty(t, itemStatByName(t, stats.ItemStats, "loose.json").CollectionName)

			// The additive fields are omitted rather than sent empty, so a
			// consumer can tell "no channel declared" from "channel: ''".
			encoded, err := json.Marshal(stats.CollectionStats)
			require.NoError(t, err)
			body := string(encoded)
			assert.Contains(t, body, `"channel":"web"`)
			assert.Contains(t, body, `"coordinates":{"product":"kapimart","surface":"web"}`)
			assert.Contains(t, body, `"ungrouped":true`)

			docsJSON, err := json.Marshal(docs)
			require.NoError(t, err)
			assert.NotContains(t, string(docsJSON), `"channel"`)
			assert.NotContains(t, string(docsJSON), `"coordinates"`)
			assert.NotContains(t, string(docsJSON), `"ungrouped"`)
		})
	}
}

func collIDs(stats []platstore.CollectionTranslationStats) []string {
	ids := make([]string, len(stats))
	for i, c := range stats {
		ids[i] = c.CollectionID
	}
	return ids
}
