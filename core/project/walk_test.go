package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalkProjectDir(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src", "sub"), 0o755))
	require.NoError(t, os.MkdirAll(project.LayoutAt(root).CacheDir(), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "node_modules", "pkg"), 0o755))
	write := func(rel string) {
		require.NoError(t, os.WriteFile(filepath.Join(root, rel), []byte("x"), 0o644))
	}
	write("kapi.yaml")
	write(filepath.Join("src", "en.json"))
	write(filepath.Join("src", "sub", "deep.md"))
	write(filepath.Join("src", ".hidden"))
	write(filepath.Join(".kapi", "store.db"))
	write(filepath.Join("node_modules", "pkg", "index.js"))
	write("ignored.tmp")
	require.NoError(t, os.WriteFile(filepath.Join(root, ".kapiignore"),
		[]byte("*.tmp\nnode_modules/\n"), 0o644))

	var got []string
	var dirs []string
	require.NoError(t, project.WalkProjectDir(root, func(rel string, info os.FileInfo) error {
		if info.IsDir() {
			dirs = append(dirs, rel)
			return nil
		}
		got = append(got, rel)
		return nil
	}))

	assert.ElementsMatch(t, []string{
		filepath.Join("src", "en.json"),
		filepath.Join("src", "sub", "deep.md"),
	}, got, "hidden, dot-dir, default-ignored (kapi.yaml + .kapi/), and .kapiignore-matched entries are excluded")
	assert.ElementsMatch(t, []string{"src", filepath.Join("src", "sub")}, dirs,
		"visible directories are visited; hidden/ignored ones are pruned")
}

// TestWalkProjectDir_RelativeRoot: a root of "." is the caller's own working
// directory, not a hidden entry. Pruning it returned an empty walk and no
// error, so every listing built on this walk reported a project with no files.
func TestWalkProjectDir_RelativeRoot(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "page.md"), []byte("x"), 0o644))
	t.Chdir(root)

	var got []string
	require.NoError(t, project.WalkProjectDir(".", func(rel string, info os.FileInfo) error {
		if !info.IsDir() {
			got = append(got, rel)
		}
		return nil
	}))
	assert.Equal(t, []string{"page.md"}, got)
}

func TestWalkProjectDir_VisitErrorStops(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644))
	}
	seen := 0
	err := project.WalkProjectDir(root, func(string, os.FileInfo) error {
		seen++
		return filepath.SkipAll
	})
	require.NoError(t, err, "SkipAll is a clean stop, not an error")
	assert.Equal(t, 1, seen)
}
