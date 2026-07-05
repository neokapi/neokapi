package project_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectStoreDrift_MissingStore(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "blocks.db")
	d := project.DetectStoreDrift(storePath, nil)
	assert.True(t, d.StoreMissing)
	assert.True(t, d.Any())
}

func TestDetectStoreDrift_VersionStamp(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "blocks.db")
	require.NoError(t, os.WriteFile(storePath, []byte("db"), 0o644))

	// No stamp → stale.
	d := project.DetectStoreDrift(storePath, nil)
	assert.False(t, d.StoreMissing)
	assert.True(t, d.VersionStale, "missing version stamp ⇒ stale")

	// Current-version stamp → clean.
	require.NoError(t, project.StampBlockStoreVersion(storePath))
	d = project.DetectStoreDrift(storePath, nil)
	assert.False(t, d.VersionStale)
	assert.False(t, d.Any())

	// Foreign-version stamp → stale.
	require.NoError(t, os.WriteFile(project.BlockStoreVersionStampPath(storePath), []byte("v0.0.0-other"), 0o644))
	d = project.DetectStoreDrift(storePath, nil)
	assert.True(t, d.VersionStale)
}

func TestBlockStoreStale(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "blocks.db")
	assert.True(t, project.BlockStoreStale(storePath), "no stamp ⇒ stale")
	require.NoError(t, project.StampBlockStoreVersion(storePath))
	assert.False(t, project.BlockStoreStale(storePath))
	require.NoError(t, os.WriteFile(project.BlockStoreVersionStampPath(storePath), []byte("some-other-schema"), 0o644))
	assert.True(t, project.BlockStoreStale(storePath))
}

func TestDetectStoreDrift_SourceFiles(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "blocks.db")
	require.NoError(t, os.WriteFile(storePath, []byte("db"), 0o644))
	require.NoError(t, project.StampBlockStoreVersion(storePath))

	src := filepath.Join(dir, "a.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"k":"v"}`), 0o644))
	files := []project.ResolvedFile{{Path: src, Relative: "a.json", Format: "json"}}

	// Never stamped → changed.
	d := project.DetectStoreDrift(storePath, files)
	assert.Equal(t, []string{"a.json"}, d.Changed, "unstamped source reads as drifted")

	// Stamp it → clean.
	info, err := os.Stat(src)
	require.NoError(t, err)
	hash, err := project.HashFile(src)
	require.NoError(t, err)
	stamps := map[string]project.SourceStamp{
		"a.json": {Hash: hash, Size: info.Size(), MTimeNS: info.ModTime().UnixNano()},
	}
	require.NoError(t, project.SaveSourceStamps(storePath, stamps))
	d = project.DetectStoreDrift(storePath, files)
	assert.False(t, d.Any(), "stamped, unchanged source ⇒ no drift")

	// Touch without changing bytes → hash confirm keeps it clean.
	future := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(src, future, future))
	d = project.DetectStoreDrift(storePath, files)
	assert.Empty(t, d.Changed, "byte-identical touch is not drift (hash confirm)")

	// Edit the bytes → changed.
	require.NoError(t, os.WriteFile(src, []byte(`{"k":"edited"}`), 0o644))
	d = project.DetectStoreDrift(storePath, files)
	assert.Equal(t, []string{"a.json"}, d.Changed)

	// A stamped file that no longer resolves → removed.
	d = project.DetectStoreDrift(storePath, nil)
	assert.Equal(t, []string{"a.json"}, d.Removed)
	assert.True(t, d.Any())
}

func TestExtractToBlockStore_RoundTripAndStamps(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	recipe := filepath.Join(real, "app.kapi")
	proj := &project.KapiProject{
		Version:  "v1",
		Name:     "StoreSync",
		Defaults: project.Defaults{SourceLanguage: "en-US"},
		Content: []project.ContentCollection{{
			Name:   "ui",
			Path:   "src/*.json",
			Format: &project.FormatSpec{Name: "json"},
		}},
	}
	require.NoError(t, project.Save(recipe, proj))
	srcDir := filepath.Join(real, "src")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a.json"), []byte(`{"greeting":"Hello","farewell":"Bye"}`), 0o644))

	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	pctx := project.NewProjectContext(proj, recipe)
	files, err := pctx.ResolveContent(reg)
	require.NoError(t, err)
	require.Len(t, files, 1)

	storePath := filepath.Join(real, ".kapi", "cache", "blocks.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(storePath), 0o755))
	store := blockstore.NewMemoryStore()
	defer store.Close()

	stats, err := project.ExtractToBlockStore(context.Background(), reg, pctx, store, storePath, files)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Files)
	assert.Equal(t, 2, stats.Blocks)
	assert.Empty(t, stats.Skipped)

	// Blocks are keyed by the shared store hash under the collection label.
	sess, err := store.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()
	hashes := map[string]bool{}
	for b, berr := range sess.Blocks(blockstore.BlockFilter{Collection: "ui"}) {
		require.NoError(t, berr)
		require.NotEmpty(t, b.ID)
		assert.Equal(t, project.BlockStoreHash("src/a.json", b.ID, ""), b.Hash,
			"blocks are keyed by the shared source-file-namespaced store hash")
		hashes[b.Hash] = true
	}
	assert.Len(t, hashes, 2)

	// Version + source stamps are written; a re-detect reports no drift for
	// the extracted source even though the memory store has no file (only the
	// sidecars matter to the file-level checks, so give the store a file).
	require.NoError(t, os.WriteFile(storePath, []byte("db"), 0o644))
	d := project.DetectStoreDrift(storePath, files)
	assert.False(t, d.Any(), "freshly extracted ⇒ no drift: %+v", d)
}

func TestExtractToBlockStore_SkipsUnreadable(t *testing.T) {
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	recipe := filepath.Join(real, "app.kapi")
	proj := &project.KapiProject{
		Version:  "v1",
		Defaults: project.Defaults{SourceLanguage: "en"},
		Content:  []project.ContentCollection{{Path: "src/*.json", Format: &project.FormatSpec{Name: "json"}}},
	}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(real, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(real, "src", "ok.json"), []byte(`{"k":"v"}`), 0o644))

	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	pctx := project.NewProjectContext(proj, recipe)
	files, err := pctx.ResolveContent(reg)
	require.NoError(t, err)
	// Add a file with no detectable format: skipped, not fatal.
	files = append(files, project.ResolvedFile{Path: filepath.Join(real, "src", "mystery.bin"), Relative: "src/mystery.bin"})

	storePath := filepath.Join(real, "blocks.db")
	store := blockstore.NewMemoryStore()
	defer store.Close()
	stats, err := project.ExtractToBlockStore(context.Background(), reg, pctx, store, storePath, files)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Files)
	require.Len(t, stats.Skipped, 1)
	assert.Equal(t, "src/mystery.bin", stats.Skipped[0].Path)
}

func TestCollectionLabel(t *testing.T) {
	assert.Equal(t, "(unnamed)", project.CollectionLabel(""))
	assert.Equal(t, "docs", project.CollectionLabel("docs"))
}

func TestMaterializePolicy(t *testing.T) {
	var d project.Defaults
	assert.Equal(t, project.MaterializeManual, d.ResolvedMaterialize(), "manual is the default")
	d.Materialize = project.MaterializeOnConverge
	assert.Equal(t, project.MaterializeOnConverge, d.ResolvedMaterialize())

	// Validation rejects unknown values.
	p := &project.KapiProject{Version: "v1", Defaults: project.Defaults{Materialize: "sometimes"}}
	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "defaults.materialize")

	p.Defaults.Materialize = project.MaterializeOnConverge
	assert.NoError(t, p.Validate())
}
