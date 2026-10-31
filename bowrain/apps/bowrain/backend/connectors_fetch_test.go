package backend

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/venue"
	venueconn "github.com/neokapi/neokapi/core/venue/connector"
)

// fileConnector fetches one file, read fresh on every Fetch the way an import
// reads it, so its blocks are the blocks an import of the same file produces.
type fileConnector struct {
	id, path, format string
	reg              *registry.FormatRegistry
}

func (c *fileConnector) ID() string                   { return c.id }
func (c *fileConnector) Name() string                 { return "test" }
func (c *fileConnector) Category() venueconn.Category { return venueconn.CategoryCMS }
func (c *fileConnector) Fetch(ctx context.Context, _ connector.FetchOptions) ([]*connector.ContentItem, error) {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return nil, err
	}
	reader, err := c.reg.NewReader(registry.FormatID(c.format))
	if err != nil {
		return nil, err
	}
	coreproj.WireReadSkeleton(c.reg, c.format, reader)
	name := filepath.Base(c.path)
	doc := &model.RawDocument{URI: name, SourceLocale: "en", Encoding: "UTF-8", Reader: io.NopCloser(bytes.NewReader(data))}
	result, err := editor.ParseItem(ctx, reader, doc, "en", c.format, name)
	if err != nil {
		return nil, err
	}
	return []*connector.ContentItem{{ID: name, Name: name, Format: c.format, Blocks: result.Blocks}}, nil
}
func (c *fileConnector) Publish(context.Context, []*connector.ContentItem, connector.PublishOptions) error {
	return nil
}
func (c *fileConnector) List(ctx context.Context) ([]*connector.ContentItem, error) {
	return c.Fetch(ctx, connector.FetchOptions{})
}
func (c *fileConnector) Status(context.Context) (*venueconn.SyncStatus, error) {
	return &venueconn.SyncStatus{ConnectorID: c.id}, nil
}
func (c *fileConnector) Configure(map[string]string) error { return nil }
func (c *fileConnector) Close() error                      { return nil }

// FetchContent stores what it fetched under the item it fetched it for, so an
// import of the same file afterwards resolves its blocks against the fetched
// rows: each row stays, and so does a translation on it. The import's preview
// marks each block by the key its row is stored under.
func TestFetchContent_StoresUnderTheItemAndAnImportKeepsTheRows(t *testing.T) {
	app := newTestApp(t)
	ctx := t.Context()
	proj, err := app.CreateProject("Fetch", "en", []string{"fr"})
	require.NoError(t, err)

	dir := t.TempDir()
	const name = "guide.md"
	writeTestFile(t, dir, name, "# Getting started\n\nInstall the tool.\n\nRun it on a file.\n")
	path := filepath.Join(dir, name)
	format, err := app.DetectFormat(path)
	require.NoError(t, err)

	conn := &fileConnector{id: "fetch-test", path: path, format: format, reg: app.formatReg}
	activeConnectors[conn.id] = conn
	t.Cleanup(func() { delete(activeConnectors, conn.id) })

	fetched, err := app.FetchContent(conn.id, proj.ID)
	require.NoError(t, err)
	require.Len(t, fetched, 1)
	assert.Equal(t, 3, fetched[0].BlockCount)

	rowsOf := func() map[string]*venue.StoredBlock {
		got, err := app.store.GetBlocks(ctx, store.BlockQuery{ProjectID: proj.ID, Stream: "main", ItemName: name})
		require.NoError(t, err)
		rows := make(map[string]*venue.StoredBlock, len(got))
		for _, sb := range got {
			rows[sb.Block.ID] = sb
		}
		return rows
	}

	item, err := app.store.GetItem(ctx, proj.ID, "main", name)
	require.NoError(t, err, "the fetch stored an item row")
	assert.Equal(t, format, item.Format)
	fetchedRows := rowsOf()
	require.Len(t, fetchedRows, 3, "every fetched block is a row of the item")
	all, err := app.store.GetBlocks(ctx, store.BlockQuery{ProjectID: proj.ID, Stream: "main"})
	require.NoError(t, err)
	assert.Len(t, all, 3, "no block is stored outside the item")

	var translated string
	for id := range fetchedRows {
		if translated == "" || id < translated {
			translated = id
		}
	}
	_, err = app.store.UpdateBlock(ctx, proj.ID, "main", translated, func(sb *venue.StoredBlock) error {
		sb.Block.SetTargetText(model.LocaleID("fr"), "Bonjour")
		return nil
	})
	require.NoError(t, err)

	_, err = app.addItemsLocal(ctx, proj.ID, []string{path})
	require.NoError(t, err)

	imported := rowsOf()
	assert.Len(t, imported, len(fetchedRows), "the import adds no row and removes none")
	for id := range fetchedRows {
		assert.Contains(t, imported, id, "the fetched row %q is still there", fetchedRows[id].SourceID)
	}
	if kept := imported[translated]; assert.NotNil(t, kept) {
		assert.Equal(t, "Bonjour", kept.Block.TargetText(model.LocaleID("fr")), "and so is its translation")
	}

	item, err = app.store.GetItem(ctx, proj.ID, "main", name)
	require.NoError(t, err)
	var markers, keys []string
	for _, m := range regexp.MustCompile(`<kat-block id="([^"]*)"`).FindAllStringSubmatch(item.PreviewHTML, -1) {
		markers = append(markers, m[1])
	}
	for _, sb := range imported {
		keys = append(keys, sb.SourceID)
	}
	sort.Strings(markers)
	sort.Strings(keys)
	assert.Equal(t, keys, markers, "the preview marks each block by its row's source id")
}
