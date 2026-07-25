package sample

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestList(t *testing.T) {
	names := List()
	assert.Equal(t, []string{"kapimart"}, names)
}

func TestDisplayName(t *testing.T) {
	assert.Equal(t, "KapiMart", DisplayName["kapimart"])
}

func TestScaffoldKapiMart(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Scaffold("kapimart", dir))

	// Validate project file.
	proj, err := project.Load(filepath.Join(dir, "kapi.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "KapiMart", proj.Name)
	assert.Equal(t, model.LocaleID("en"), proj.Defaults.SourceLanguage)
	assert.Equal(t, []model.LocaleID{"de", "fr", "ja", "nb", "ar"}, proj.Defaults.TargetLanguages)

	// 4 named content collections.
	require.Len(t, proj.Content, 4)
	assert.Equal(t, "Website", proj.Content[0].Name)
	assert.Equal(t, "Online Store", proj.Content[1].Name)
	assert.Equal(t, "Contracts", proj.Content[2].Name)
	assert.Equal(t, "Templates", proj.Content[3].Name)

	// 3 flows.
	assert.NotEmpty(t, proj.Flows)

	// Source file counts per area (natural layout: <area>/en/…).
	assertDirCount(t, filepath.Join(dir, "web", "en"), 7)
	assertDirCount(t, filepath.Join(dir, "src", "en"), 5)
	assertDirCount(t, filepath.Join(dir, "legal", "en"), 2)
	assertDirCount(t, filepath.Join(dir, "marketing", "en"), 2)

	// No separate output/ tree — localized files land beside source in locale dirs.
	_, err = os.Stat(filepath.Join(dir, "output"))
	require.True(t, os.IsNotExist(err), "KapiMart must not scaffold an output/ dir")

	// content memory should have 200+ entries. Under the multilingual model each TU
	// becomes a single entry with N variants instead of N entries per TU,
	// so the total is roughly ~1/5 of the old count.
	tm, err := memory.NewSQLiteStore(filepath.Join(dir, ".kapi", "tm.db"))
	require.NoError(t, err)
	defer tm.Close()
	memoryCount, err := tm.Count(t.Context())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, memoryCount, 200, "content memory should have at least 200 multilingual entries")

	// Terms should have 100+ concepts.
	tb, err := terms.NewSQLiteStore(filepath.Join(dir, ".kapi", "termbase.db"))
	require.NoError(t, err)
	defer tb.Close()
	tbCount, err := tb.Count(t.Context())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, tbCount, 100, "terms should have at least 100 concepts")
}

func TestScaffoldUnknown(t *testing.T) {
	err := Scaffold("unknown", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown sample project")
}

func assertDirCount(t *testing.T, dir string, expectedCount int) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "directory should exist: %s", dir)
	assert.Len(t, entries, expectedCount, "file count in %s", dir)
}
