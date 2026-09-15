package project

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// symlinkTree writes the fixture the tests below share, in the shape a pnpm
// workspace gives a package: src/app.json is real content, store/pkg holds a
// file of its own, src/node_modules/pkg is a link to that directory, and
// src/linked.json is a link to the store's file.
func symlinkTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	createFile(t, dir, "src/app.json", "{}")
	createFile(t, dir, "store/pkg/lib.json", "{}")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "node_modules"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join("..", "..", "store", "pkg"), filepath.Join(dir, "src", "node_modules", "pkg")))
	require.NoError(t, os.Symlink(filepath.Join("..", "store", "pkg", "lib.json"), filepath.Join(dir, "src", "linked.json")))
	return dir
}

// `**` reads the directories under the root and does not follow a link into
// another directory, as git ls-files does. A link that matches the pattern
// itself still resolves.
func TestExpandGlob_DoesNotFollowSymlinkedDirectories(t *testing.T) {
	dir := symlinkTree(t)

	got, err := ExpandGlob(dir, "src/**/*.json")
	require.NoError(t, err)
	sort.Strings(got)
	assert.Equal(t, []string{"src/app.json", "src/linked.json"}, got,
		"the file behind src/node_modules/pkg is reached only through a linked directory")
}

// A link to a file that matches the pattern directly is content, wherever the
// file it names sits.
func TestExpandGlob_ResolvesADirectlyMatchingSymlinkedFile(t *testing.T) {
	dir := symlinkTree(t)

	got, err := ExpandGlob(dir, "src/*.json")
	require.NoError(t, err)
	sort.Strings(got)
	assert.Equal(t, []string{"src/app.json", "src/linked.json"}, got)
}

// A pattern that names a linked directory before any wildcard reads through it:
// the recipe asked for that directory by name.
func TestExpandGlob_ReadsALinkedDirectoryNamedBeforeAnyWildcard(t *testing.T) {
	dir := symlinkTree(t)

	got, err := ExpandGlob(dir, "src/node_modules/pkg/**/*.json")
	require.NoError(t, err)
	assert.Equal(t, []string{"src/node_modules/pkg/lib.json"}, got)
}

// A link back to its own directory is a loop. Expansion finishes, and reports
// the one file the directory holds once.
func TestExpandGlob_TerminatesOnASymlinkLoop(t *testing.T) {
	dir := t.TempDir()
	createFile(t, dir, "loop/a.json", "{}")
	require.NoError(t, os.Symlink(".", filepath.Join(dir, "loop", "self")))

	type result struct {
		got []string
		err error
	}
	done := make(chan result, 1)
	go func() {
		got, err := ExpandGlob(dir, "loop/**/*.json")
		done <- result{got, err}
	}()
	select {
	case r := <-done:
		require.NoError(t, r.err)
		assert.Equal(t, []string{"loop/a.json"}, r.got)
	case <-time.After(10 * time.Second):
		t.Fatal("expanding loop/**/*.json did not finish within 10s: the link loop was followed")
	}
}

// Content resolution reads no file through a linked directory, and resolves a
// linked file that the pattern matches.
func TestResolveContent_ReadsNoFileThroughASymlinkedDirectory(t *testing.T) {
	dir := symlinkTree(t)

	reg := registry.NewFormatRegistry()
	registerBuiltIn(reg, "json", ".json")
	proj := &KapiProject{
		Version:     CurrentVersion,
		Collections: []Collection{{Name: "App", Content: []ContentItem{{Path: "src/**/*.json"}}}},
	}
	ctx := NewProjectContext(proj, filepath.Join(dir, "kapi.yaml"))

	files, err := ctx.ResolveContent(reg)
	require.NoError(t, err)
	rels := make([]string, 0, len(files))
	for _, f := range files {
		rels = append(rels, f.Relative)
	}
	sort.Strings(rels)
	assert.Equal(t, []string{"src/app.json", "src/linked.json"}, rels)
}
