package ignore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRules(t *testing.T) {
	m := New()

	assert.True(t, m.Match("project.kapi", false), "*.kapi should be ignored by default")
	assert.True(t, m.Match("sub/other.kapi", false))
	assert.True(t, m.Match(".git", true))
	assert.True(t, m.Match(".DS_Store", false))

	assert.False(t, m.Match("README.md", false))
	assert.False(t, m.Match("src/main.go", false))
}

func TestSimpleGlobs(t *testing.T) {
	m := New()
	m.AddPattern("*.tmp")
	m.AddPattern("*.log")

	assert.True(t, m.Match("scratch.tmp", false))
	assert.True(t, m.Match("deep/nested/file.tmp", false))
	assert.True(t, m.Match("server.log", false))
	assert.False(t, m.Match("main.go", false))
}

func TestDirectoryOnlyPattern(t *testing.T) {
	m := New()
	m.AddPattern("build/")

	assert.True(t, m.Match("build", true), "directory should match")
	assert.False(t, m.Match("build", false), "file named build should not match")
}

func TestNegation(t *testing.T) {
	m := New()
	m.AddPattern("*.log")
	m.AddPattern("!important.log")

	assert.True(t, m.Match("debug.log", false))
	assert.False(t, m.Match("important.log", false), "negated pattern should un-ignore")
}

func TestPathPatterns(t *testing.T) {
	m := New()
	m.AddPattern("vendor/modules")

	assert.True(t, m.Match("vendor/modules", false))
	assert.False(t, m.Match("other/modules", false))
}

func TestDoublestarSuffix(t *testing.T) {
	m := New()
	m.AddPattern("output/**")

	assert.True(t, m.Match("output/en.json", false))
	assert.True(t, m.Match("output/fr/messages.json", false))
	assert.True(t, m.Match("output", true))
	assert.False(t, m.Match("input/en.json", false))
}

func TestDoublestarPrefix(t *testing.T) {
	m := New()
	m.AddPattern("**/*.tmp")

	assert.True(t, m.Match("file.tmp", false))
	assert.True(t, m.Match("a/b/c/file.tmp", false))
	assert.False(t, m.Match("file.json", false))
}

func TestDoublestarMiddle(t *testing.T) {
	m := New()
	m.AddPattern("docs/**/draft.*")

	assert.True(t, m.Match("docs/draft.md", false))
	assert.True(t, m.Match("docs/v2/draft.md", false))
	assert.True(t, m.Match("docs/a/b/draft.txt", false))
	assert.False(t, m.Match("src/draft.md", false))
}

func TestCommentsAndBlanks(t *testing.T) {
	m := New()
	m.AddPattern("# this is a comment")
	m.AddPattern("")
	m.AddPattern("   ")
	m.AddPattern("real-pattern")

	assert.True(t, m.Match("real-pattern", false))
	assert.Len(t, m.rules, len(defaultPatterns)+1, "should only add 1 real rule")
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	ignoreFile := filepath.Join(dir, ".kapiignore")
	err := os.WriteFile(ignoreFile, []byte("# Build output\nbuild/\n*.tmp\n"), 0o644)
	require.NoError(t, err)

	m := New()
	require.NoError(t, m.LoadFile(ignoreFile))

	assert.True(t, m.Match("build", true))
	assert.True(t, m.Match("scratch.tmp", false))
	assert.False(t, m.Match("main.go", false))
}

func TestLoadFileMissing(t *testing.T) {
	m := New()
	err := m.LoadFile("/nonexistent/.kapiignore")
	require.NoError(t, err, "missing file should not error")
}

func TestLoadEnv(t *testing.T) {
	t.Setenv("KAPI_IGNORE", "*.bak,temp/")

	m := New()
	m.LoadEnv()

	assert.True(t, m.Match("old.bak", false))
	assert.True(t, m.Match("temp", true))
	assert.False(t, m.Match("main.go", false))
}

func TestForProjectDir(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, ".kapiignore"), []byte("node_modules/\n"), 0o644)
	require.NoError(t, err)

	m := ForProjectDir(dir)

	// Defaults.
	assert.True(t, m.Match("project.kapi", false))
	assert.True(t, m.Match(".git", true))

	// From file.
	assert.True(t, m.Match("node_modules", true))

	// Not ignored.
	assert.False(t, m.Match("src/main.go", false))
}

func TestDirectoryNameMatch(t *testing.T) {
	m := New()
	m.AddPattern("node_modules")

	// Without trailing slash, matches both files and dirs with that name.
	assert.True(t, m.Match("node_modules", true))
	assert.True(t, m.Match("node_modules", false))
	assert.True(t, m.Match("packages/app/node_modules", true))
}
