package sample

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestList(t *testing.T) {
	names := List()
	assert.Equal(t, []string{"kapimart", "okapimart"}, names)
}

func TestDisplayName(t *testing.T) {
	assert.Equal(t, "KapiMart", DisplayName["kapimart"])
	assert.Equal(t, "OkapiMart", DisplayName["okapimart"])
}

func TestScaffoldKapiMart(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Scaffold("kapimart", dir))

	assertScaffoldedProject(t, dir, "KapiMart")
}

func TestScaffoldOkapiMart(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Scaffold("okapimart", dir))

	assertScaffoldedProject(t, dir, "OkapiMart")

	// OkapiMart should require the okapi-bridge plugin.
	proj, err := project.Load(filepath.Join(dir, "project.kapi"))
	require.NoError(t, err)
	assert.Contains(t, proj.Plugins, "okapi-bridge")
}

func TestScaffoldUnknown(t *testing.T) {
	err := Scaffold("unknown", t.TempDir())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown sample project")
}

func assertScaffoldedProject(t *testing.T, dir, expectedName string) {
	t.Helper()

	// project.kapi should be valid.
	proj, err := project.Load(filepath.Join(dir, "project.kapi"))
	require.NoError(t, err)
	assert.Equal(t, expectedName, proj.Name)
	assert.Equal(t, "en-US", proj.Defaults.SourceLanguage)
	assert.Equal(t, []string{"fr-FR", "de-DE", "ja-JP"}, proj.Defaults.TargetLanguages)
	assert.NotEmpty(t, proj.Flows)

	// Input files should exist.
	expectedFiles := []string{
		"store-ui.json",
		"product-catalog.yaml",
		"about-us.html",
		"error-messages.properties",
		"onboarding-video.srt",
		"release-notes.xml",
		"admin-guide.txt",
		"changelog.md",
	}
	for _, f := range expectedFiles {
		_, err := os.Stat(filepath.Join(dir, "input", f))
		assert.NoError(t, err, "missing input file: %s", f)
	}

	// Output directory should exist.
	_, err = os.Stat(filepath.Join(dir, "output"))
	assert.NoError(t, err)

	// TM should be seeded.
	tmPath := filepath.Join(dir, ".kapi", "tm.db")
	tm, err := sievepen.NewSQLiteTM(tmPath)
	require.NoError(t, err)
	defer tm.Close()
	assert.Greater(t, tm.Count(), 0, "TM should have entries")

	// Termbase should be seeded.
	tbPath := filepath.Join(dir, ".kapi", "termbase.db")
	tb, err := termbase.NewSQLiteTermBase(tbPath)
	require.NoError(t, err)
	defer tb.Close()
	assert.Greater(t, tb.Count(), 0, "termbase should have concepts")
}
