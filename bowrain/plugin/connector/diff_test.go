package connector

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	bproject "github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDiffTestProject scaffolds a JSON-content project on disk with the given
// locale file contents and returns the loaded project plus a registry.
func newDiffTestProject(t *testing.T, jsonContent string) (*bproject.Project, *registry.FormatRegistry) {
	t.Helper()
	root := t.TempDir()

	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	recipe := &bproject.Recipe{
		KapiProject: coreproj.KapiProject{
			Defaults: coreproj.Defaults{
				SourceLanguage:  "en",
				TargetLanguages: []model.LocaleID{"fr"},
			},
			Content: []coreproj.ContentCollection{
				{Path: "locales/en.json", Format: &coreproj.FormatSpec{Name: "json"}},
			},
		},
	}
	proj, err := bproject.InitProject(root, recipe)
	require.NoError(t, err)

	abs := filepath.Join(root, "locales", "en.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(jsonContent), 0o644))

	return proj, reg
}

func TestConnectorDiff_AllAddedWhenNoCache(t *testing.T) {
	const content = `{"greeting":"Hello","farewell":"Goodbye"}`
	proj, reg := newDiffTestProject(t, content)

	conn := NewLocalConnector(proj, reg)
	defer conn.Close()

	diff, err := conn.Diff(context.Background(), nil)
	require.NoError(t, err)

	require.True(t, diff.HasChanges())
	assert.Equal(t, 2, diff.Added)
	assert.Equal(t, 0, diff.Changed)
	assert.Equal(t, 0, diff.Removed)
	require.Len(t, diff.Files, 1)

	fd := diff.Files[0]
	assert.Equal(t, "locales/en.json", fd.Path)
	assert.Equal(t, "json", fd.Format)
	assert.Equal(t, 2, fd.Added)
	require.Len(t, fd.Blocks, 2)
	for _, b := range fd.Blocks {
		assert.Equal(t, "added", b.Change)
		assert.NotEmpty(t, b.BlockID)
	}
}

func TestConnectorDiff_NoChangesWhenCacheMatches(t *testing.T) {
	const content = `{"greeting":"Hello","farewell":"Goodbye"}`
	proj, reg := newDiffTestProject(t, content)

	// Seed the cache to exactly the current scanned state.
	seedCacheFromScan(t, proj, reg, nil)

	conn := NewLocalConnector(proj, reg)
	defer conn.Close()

	diff, err := conn.Diff(context.Background(), nil)
	require.NoError(t, err)

	assert.False(t, diff.HasChanges())
	assert.Equal(t, 0, diff.Added)
	assert.Equal(t, 0, diff.Changed)
	assert.Equal(t, 0, diff.Removed)
	assert.Empty(t, diff.Files)
}

func TestConnectorDiff_ChangedAndRemoved(t *testing.T) {
	const content = `{"greeting":"Hello","farewell":"Goodbye"}`
	proj, reg := newDiffTestProject(t, content)

	// Seed the cache from the original content, then mutate it: change one
	// value (→ changed) and inject a phantom cached block (→ removed).
	seedCacheFromScan(t, proj, reg, map[string]string{"__phantom__": "deadbeef"})

	// Mutate the file so "greeting" changes content.
	abs := filepath.Join(proj.Root, "locales", "en.json")
	require.NoError(t, os.WriteFile(abs, []byte(`{"greeting":"Hi there","farewell":"Goodbye"}`), 0o644))

	conn := NewLocalConnector(proj, reg)
	defer conn.Close()

	diff, err := conn.Diff(context.Background(), nil)
	require.NoError(t, err)

	require.True(t, diff.HasChanges())
	assert.Equal(t, 0, diff.Added, "no new keys were added")
	assert.Equal(t, 1, diff.Changed, "greeting value changed")
	assert.Equal(t, 1, diff.Removed, "phantom cached block is reported removed")
	require.Len(t, diff.Files, 1)

	fd := diff.Files[0]
	var changes []string
	for _, b := range fd.Blocks {
		changes = append(changes, b.Change)
	}
	assert.Contains(t, changes, "changed")
	assert.Contains(t, changes, "removed")
}

// seedCacheFromScan scans the project's current local blocks and writes them
// to the sync cache so a subsequent Diff sees the files as already synced.
// extraBlocks, if non-nil, are injected into the first file's cached blocks to
// simulate server-side blocks no longer present locally (→ "removed").
func seedCacheFromScan(t *testing.T, proj *bproject.Project, reg *registry.FormatRegistry, extraBlocks map[string]string) {
	t.Helper()

	conn := NewLocalConnector(proj, reg)
	hashMap, _, err := conn.scanLocalBlocks(context.Background(), nil)
	require.NoError(t, err)
	require.NotEmpty(t, hashMap, "expected at least one scanned file")

	cache := bproject.LoadSyncCache(proj.Layout)
	for itemName, blocks := range hashMap {
		fc := &bproject.FileCache{Blocks: map[string]string{}}
		for id, h := range blocks {
			fc.Blocks[id] = h
		}
		cache.Files[itemName] = fc
	}
	if extraBlocks != nil {
		// Add the phantom blocks to whichever file we have.
		for itemName := range hashMap {
			for id, h := range extraBlocks {
				cache.Files[itemName].Blocks[id] = h
			}
			break
		}
	}
	require.NoError(t, cache.Save(proj.Layout))
}
