package service

// Connector ingest must write the same item bookkeeping every other ingest
// venue writes (editor upload, sync push, desktop): an item row per fetched
// ContentItem plus item-scoped blocks. Blocks stored item-less were invisible
// to the item-scoped dashboard stats, the convergence derive/produce steps
// (jobs are per item), and forge delivery — the Journey-C "Up to date over
// five untranslated blocks" drill. SQLite-backed so it runs without Docker.

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// freshItemsConnector builds its ContentItems anew on every Fetch/List — the
// behavior of real file/git connectors, which re-read their source (the store
// remaps reader block IDs in place, so reusing pointers would leak the
// remapped IDs into the next fetch).
type freshItemsConnector struct {
	id   string
	make func() []*connector.ContentItem
}

func (c *freshItemsConnector) ID() string                   { return c.id }
func (c *freshItemsConnector) Name() string                 { return "test" }
func (c *freshItemsConnector) Category() connector.Category { return connector.CategoryFile }
func (c *freshItemsConnector) Fetch(_ context.Context, _ connector.FetchOptions) ([]*connector.ContentItem, error) {
	return c.make(), nil
}
func (c *freshItemsConnector) Publish(_ context.Context, _ []*connector.ContentItem, _ connector.PublishOptions) error {
	return nil
}
func (c *freshItemsConnector) List(_ context.Context) ([]*connector.ContentItem, error) {
	return c.make(), nil
}
func (c *freshItemsConnector) Status(_ context.Context) (*connector.SyncStatus, error) {
	return &connector.SyncStatus{ConnectorID: c.id}, nil
}
func (c *freshItemsConnector) Configure(_ map[string]string) error { return nil }
func (c *freshItemsConnector) Close() error                        { return nil }

func TestConnectorServiceFetch_CreatesItemBookkeeping(t *testing.T) {
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "content.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	ctx := t.Context()

	p := &store.Project{
		Name:                  "journey-c",
		DefaultSourceLanguage: model.LocaleEnglish,
		TargetLanguages:       []model.LocaleID{model.LocaleFrench},
	}
	require.NoError(t, cs.CreateProject(ctx, p))

	// Two files with a COLLIDING format-reader block ID ("greeting"): per-file
	// reader IDs are only unique within their file, so item-less storage
	// silently clobbered one file's block with the other's. Items are built
	// FRESH per Fetch, like a real connector re-reading its source.
	reg := connector.NewRegistry()
	reg.Register("test", connector.CategoryFile, func(config map[string]string) (connector.IntegrationConnector, error) {
		return &freshItemsConnector{
			id: "test-1",
			make: func() []*connector.ContentItem {
				return []*connector.ContentItem{
					{
						ID: "locales/en/app.json", Name: "app.json", Path: "locales/en/app.json", Format: "json",
						Blocks: []*model.Block{
							model.NewBlock("greeting", "Hello from the app"),
							model.NewBlock("farewell", "Goodbye from the app"),
						},
					},
					{
						ID: "locales/en/home.json", Name: "home.json", Path: "locales/en/home.json", Format: "json",
						Blocks: []*model.Block{
							model.NewBlock("greeting", "Welcome home"),
						},
					},
				}
			},
		}, nil
	})

	svc := NewConnectorService(cs, reg)
	_, err = svc.AddConnector("ws-a", "test", nil)
	require.NoError(t, err)

	fetched, err := svc.Fetch(ctx, "ws-a", "test-1", p.ID, connector.FetchOptions{})
	require.NoError(t, err)
	require.Len(t, fetched, 2)

	// Item rows exist, named by source-relative path.
	items, err := cs.ListItems(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, items, 2, "each fetched file gets an item row")
	names := []string{items[0].Name, items[1].Name}
	assert.ElementsMatch(t, []string{"locales/en/app.json", "locales/en/home.json"}, names)

	// All three blocks stored, scoped per item — no cross-file reader-ID clobber.
	appBlocks, err := cs.GetBlocks(ctx, store.BlockQuery{ProjectID: p.ID, Stream: "main", ItemName: "locales/en/app.json"})
	require.NoError(t, err)
	assert.Len(t, appBlocks, 2)
	homeBlocks, err := cs.GetBlocks(ctx, store.BlockQuery{ProjectID: p.ID, Stream: "main", ItemName: "locales/en/home.json"})
	require.NoError(t, err)
	require.Len(t, homeBlocks, 1)
	assert.Equal(t, "Welcome home", homeBlocks[0].Block.SourceText())

	// The item-scoped stats path — what the convergence derive reads — sees
	// every ingested block.
	stats, err := cs.GetBlockStats(ctx, p.ID, "main")
	require.NoError(t, err)
	assert.Len(t, stats, 3)

	// Re-fetching is idempotent: same items, same blocks, no duplicates.
	_, err = svc.Fetch(ctx, "ws-a", "test-1", p.ID, connector.FetchOptions{})
	require.NoError(t, err)
	all, err := cs.GetBlocks(ctx, store.BlockQuery{ProjectID: p.ID, Stream: "main"})
	require.NoError(t, err)
	assert.Len(t, all, 3)
	items, err = cs.ListItems(ctx, p.ID, "main")
	require.NoError(t, err)
	assert.Len(t, items, 2)
}
