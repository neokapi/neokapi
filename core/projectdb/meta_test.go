package projectdb_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
)

func TestMeta_ReadWriteReplace(t *testing.T) {
	db := openStore(t, newLayout(t))
	ctx := t.Context()

	_, ok, err := db.Meta(ctx, "absent.key")
	require.NoError(t, err)
	assert.False(t, ok, "an unwritten key reads as absent, not as empty")

	require.NoError(t, db.PutMeta(ctx, "a.key", "first"))
	v, ok, err := db.Meta(ctx, "a.key")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "first", v)

	require.NoError(t, db.PutMeta(ctx, "a.key", "second"))
	v, ok, err = db.Meta(ctx, "a.key")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "second", v, "a key holds one value")
}

func TestBlockStoreVersionStamp(t *testing.T) {
	layout := newLayout(t)
	db := openStore(t, layout)
	ctx := t.Context()

	assert.True(t, db.BlockStoreStale(ctx), "an unstamped store reads as stale — the safe direction")

	require.NoError(t, db.StampBlockStoreVersion(ctx))
	assert.False(t, db.BlockStoreStale(ctx))

	v, ok, err := db.Meta(ctx, projectdb.MetaBlocksSchemaVersion)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, project.BlockStoreSchemaVersion, v)

	// A store written under other extraction semantics reads as stale.
	require.NoError(t, db.PutMeta(ctx, projectdb.MetaBlocksSchemaVersion, "1999-01-01.1"))
	assert.True(t, db.BlockStoreStale(ctx))

	// The stamp is in the file, not in the handle.
	require.NoError(t, db.Close())
	reopened := openStore(t, layout)
	assert.True(t, reopened.BlockStoreStale(ctx))
	require.NoError(t, reopened.StampBlockStoreVersion(ctx))
	assert.False(t, reopened.BlockStoreStale(ctx))
}

func TestSourceStamps_RoundTrip(t *testing.T) {
	db := openStore(t, newLayout(t))
	ctx := t.Context()

	assert.Empty(t, db.LoadSourceStamps(ctx), "no stamps yet: every file reads as drifted")

	stamps := map[string]project.SourceStamp{
		"docs/intro.md": {Hash: "sha256:aaa", Size: 12, MTimeNS: 1234},
		"ui/en.json":    {Hash: "sha256:bbb", Size: 34, MTimeNS: 5678},
	}
	require.NoError(t, db.SaveSourceStamps(ctx, stamps))
	assert.Equal(t, stamps, db.LoadSourceStamps(ctx))

	// A malformed value degrades to "everything drifted" rather than erroring:
	// re-extracting is always correct, just not always cheap.
	require.NoError(t, db.PutMeta(ctx, projectdb.MetaBlocksSourceStamps, "not json"))
	assert.Empty(t, db.LoadSourceStamps(ctx))
}

// writeSource writes a source file and returns it as a resolved file plus its
// extract-time stamp.
func writeSource(t *testing.T, root, rel, content string) (project.ResolvedFile, project.SourceStamp) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	info, err := os.Stat(path)
	require.NoError(t, err)
	hash, err := project.HashFile(path)
	require.NoError(t, err)

	return project.ResolvedFile{Path: path, Relative: rel, Format: "markdown"},
		project.SourceStamp{Hash: hash, Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}
}

func TestDetectStoreDrift(t *testing.T) {
	layout := newLayout(t)
	db := openStore(t, layout)
	ctx := t.Context()

	intro, introStamp := writeSource(t, layout.Root, "docs/intro.md", "Hello")
	guide, guideStamp := writeSource(t, layout.Root, "docs/guide.md", "Guide")
	files := []project.ResolvedFile{intro, guide}

	// An empty block cache is what "never extracted" now means: the store file
	// exists from the first open of any subsystem, so its presence proves
	// nothing.
	drift := db.DetectStoreDrift(ctx, files)
	assert.True(t, drift.StoreMissing)

	sess, err := db.Blocks().Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, sess.PutBlock("docs", &blockstore.Block{Hash: "h1", Source: []model.Run{model.TextR("Hello")}}))
	require.NoError(t, sess.Commit())

	drift = db.DetectStoreDrift(ctx, files)
	assert.False(t, drift.StoreMissing)
	assert.True(t, drift.VersionStale, "extracted but unstamped")
	assert.ElementsMatch(t, []string{"docs/intro.md", "docs/guide.md"}, drift.Changed,
		"unstamped sources read as drifted")

	require.NoError(t, db.StampBlockStoreVersion(ctx))
	require.NoError(t, db.SaveSourceStamps(ctx, map[string]project.SourceStamp{
		"docs/intro.md": introStamp,
		"docs/guide.md": guideStamp,
	}))

	drift = db.DetectStoreDrift(ctx, files)
	assert.False(t, drift.Any(), "a freshly stamped store has no drift: %+v", drift)

	t.Run("edited source", func(t *testing.T) {
		require.NoError(t, os.WriteFile(intro.Path, []byte("Hello again"), 0o644))
		// Defeat the stat fast path's mtime granularity.
		future := time.Now().Add(2 * time.Second)
		require.NoError(t, os.Chtimes(intro.Path, future, future))

		drift := db.DetectStoreDrift(ctx, files)
		assert.Equal(t, []string{"docs/intro.md"}, drift.Changed)
		assert.Empty(t, drift.Removed)
	})

	t.Run("de-scoped source", func(t *testing.T) {
		drift := db.DetectStoreDrift(ctx, []project.ResolvedFile{intro})
		assert.Equal(t, []string{"docs/guide.md"}, drift.Removed,
			"a stamped file that no longer resolves left blocks behind")
	})
}

// The table-backed stamps and the file sidecars must read the same drift from
// the same stamps — they share the comparison, and this is what says so.
func TestDetectStoreDrift_MatchesFileBackedDetection(t *testing.T) {
	layout := newLayout(t)
	db := openStore(t, layout)
	ctx := t.Context()

	intro, introStamp := writeSource(t, layout.Root, "docs/intro.md", "Hello")
	guide, _ := writeSource(t, layout.Root, "docs/guide.md", "Guide")
	files := []project.ResolvedFile{intro, guide}
	stamps := map[string]project.SourceStamp{"docs/intro.md": introStamp}

	sess, err := db.Blocks().Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, sess.PutBlock("docs", &blockstore.Block{Hash: "h1", Source: []model.Run{model.TextR("Hello")}}))
	require.NoError(t, sess.Commit())
	require.NoError(t, db.StampBlockStoreVersion(ctx))
	require.NoError(t, db.SaveSourceStamps(ctx, stamps))

	// The predecessor layout: a standalone block store with sidecar stamps.
	legacyStore := filepath.Join(t.TempDir(), "blocks.db")
	require.NoError(t, os.WriteFile(legacyStore, []byte("not really sqlite"), 0o644))
	require.NoError(t, project.StampBlockStoreVersion(legacyStore))
	require.NoError(t, project.SaveSourceStamps(legacyStore, stamps))

	fromTable := db.DetectStoreDrift(ctx, files)
	fromFiles := project.DetectStoreDrift(legacyStore, files)
	assert.Equal(t, fromFiles.Changed, fromTable.Changed)
	assert.Equal(t, fromFiles.Removed, fromTable.Removed)
	assert.Equal(t, fromFiles.VersionStale, fromTable.VersionStale)
}
