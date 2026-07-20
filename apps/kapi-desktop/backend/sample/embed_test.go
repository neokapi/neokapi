package sample

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
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
	assert.Equal(t, model.LocaleID("en-US"), proj.Defaults.SourceLanguage)
	assert.Equal(t, []model.LocaleID{"de-DE", "fr-FR", "ja-JP", "nb-NO", "ar-SA"}, proj.Defaults.TargetLanguages)

	// 4 named content collections.
	require.Len(t, proj.Content, 4)
	assert.Equal(t, "Website", proj.Content[0].Name)
	assert.Equal(t, "Online Store", proj.Content[1].Name)
	assert.Equal(t, "Contracts", proj.Content[2].Name)
	assert.Equal(t, "Templates", proj.Content[3].Name)

	// 3 flows.
	assert.NotEmpty(t, proj.Flows)

	// Source file counts per area (natural layout: <area>/en-US/…).
	assertDirCount(t, filepath.Join(dir, "web", "en-US"), 7)
	assertDirCount(t, filepath.Join(dir, "src", "en-US"), 5)
	assertDirCount(t, filepath.Join(dir, "legal", "en-US"), 2)
	assertDirCount(t, filepath.Join(dir, "marketing", "en-US"), 2)

	// No separate output/ tree — localized files land beside source in locale dirs.
	_, err = os.Stat(filepath.Join(dir, "output"))
	require.True(t, os.IsNotExist(err), "KapiMart must not scaffold an output/ dir")

	// TM should have 200+ entries. Under the multilingual model each TU
	// becomes a single entry with N variants instead of N entries per TU,
	// so the total is roughly ~1/5 of the old count.
	tm, err := sievepen.NewSQLiteTM(filepath.Join(dir, ".kapi", "tm.db"))
	require.NoError(t, err)
	defer tm.Close()
	tmCount, err := tm.Count(t.Context())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, tmCount, 200, "TM should have at least 200 multilingual entries")

	// Termbase should have 100+ concepts.
	tb, err := termbase.NewSQLiteTermBase(filepath.Join(dir, ".kapi", "termbase.db"))
	require.NoError(t, err)
	defer tb.Close()
	tbCount, err := tb.Count(t.Context())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, tbCount, 100, "termbase should have at least 100 concepts")
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
