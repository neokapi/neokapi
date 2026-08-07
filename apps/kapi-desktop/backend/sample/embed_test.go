package sample

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
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
	require.Len(t, proj.Collections, 4)
	assert.Equal(t, "Website", proj.Collections[0].Name)
	assert.Equal(t, "Online Store", proj.Collections[1].Name)
	assert.Equal(t, "Contracts", proj.Collections[2].Name)
	assert.Equal(t, "Templates", proj.Collections[3].Name)

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

	// The seed landed in the project's one store; both subsystems come off the
	// same handle, and closing it releases both.
	db, err := projectdb.Open(t.Context(), project.Layout{
		Root: dir, StateDir: filepath.Join(dir, project.StateDirName),
	})
	require.NoError(t, err)
	defer db.Close()

	// The content memory should have 200+ entries. Under the multilingual model
	// each TU becomes a single entry with N variants instead of N entries per TU,
	// so the total is roughly ~1/5 of the old count.
	memoryCount, err := db.Memory().Count(t.Context())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, memoryCount, 200, "content memory should have at least 200 multilingual entries")

	// Terms should have 100+ concepts.
	tbCount, err := db.Terms().Count(t.Context())
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
