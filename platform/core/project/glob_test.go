package project

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestTree creates a directory tree under root and returns root.
func createTestTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	files := []string{
		"src/main/index.html",
		"src/main/about.html",
		"src/legacy/old.html",
		"src/legacy/ancient.html",
		"src/styles/app.css",
		"docs/readme.md",
	}
	for _, f := range files {
		abs := filepath.Join(root, f)
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0755))
		require.NoError(t, os.WriteFile(abs, []byte("test"), 0644))
	}
	return root
}

func sorted(s []string) []string {
	sort.Strings(s)
	return s
}

func TestExpandGlob_BasicGlob(t *testing.T) {
	root := createTestTree(t)
	matches, err := ExpandGlob(root, "docs/*.md")
	require.NoError(t, err)
	assert.Equal(t, []string{"docs/readme.md"}, matches)
}

func TestExpandGlob_RecursiveGlob(t *testing.T) {
	root := createTestTree(t)
	matches, err := ExpandGlob(root, "src/**/*.html")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"src/legacy/ancient.html",
		"src/legacy/old.html",
		"src/main/about.html",
		"src/main/index.html",
	}, sorted(matches))
}

func TestExpandGlob_WithExclude(t *testing.T) {
	root := createTestTree(t)
	matches, err := ExpandGlob(root, "src/**/*.html", "src/legacy/*.html")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"src/main/about.html",
		"src/main/index.html",
	}, sorted(matches))
}

func TestExpandGlob_MultipleExcludes(t *testing.T) {
	root := createTestTree(t)
	matches, err := ExpandGlob(root, "src/**/*.html", "src/legacy/*.html", "src/main/about.html")
	require.NoError(t, err)
	assert.Equal(t, []string{"src/main/index.html"}, matches)
}

func TestExpandGlob_ExcludeMatchesNothing(t *testing.T) {
	root := createTestTree(t)
	matches, err := ExpandGlob(root, "src/**/*.html", "nonexistent/**")
	require.NoError(t, err)
	assert.Len(t, matches, 4, "exclude that matches nothing should not filter anything")
}

func TestExpandGlob_NoExcludes(t *testing.T) {
	root := createTestTree(t)
	withExcludes, err := ExpandGlob(root, "src/**/*.html")
	require.NoError(t, err)

	withoutExcludes, err := ExpandGlob(root, "src/**/*.html")
	require.NoError(t, err)

	assert.Equal(t, withoutExcludes, withExcludes, "no excludes should behave identically")
}

func TestExpandGlob_NoMatches(t *testing.T) {
	root := createTestTree(t)
	matches, err := ExpandGlob(root, "nonexistent/**/*.txt")
	require.NoError(t, err)
	assert.Empty(t, matches)
}
