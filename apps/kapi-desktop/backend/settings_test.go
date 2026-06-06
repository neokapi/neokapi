package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettingsDefaults(t *testing.T) {
	s := &settingsStore{
		filePath: filepath.Join(t.TempDir(), "settings.json"),
		settings: AppSettings{Theme: "dark"},
	}
	assert.Equal(t, "dark", s.settings.Theme)
}

func TestSettingsPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	s1 := &settingsStore{
		filePath: path,
		settings: AppSettings{Theme: "light"},
	}
	s1.save()

	s2 := &settingsStore{
		filePath: path,
		settings: AppSettings{Theme: "dark"},
	}
	s2.load()
	assert.Equal(t, "light", s2.settings.Theme)
}

func TestAppGetSettings(t *testing.T) {
	app := NewApp()
	settings := app.GetSettings()
	// Theme may have been changed by prior runs; just verify it's non-empty.
	assert.NotEmpty(t, settings.Theme)
}

func TestAppSetTheme(t *testing.T) {
	app := NewApp()
	app.SetTheme("light")
	assert.Equal(t, "light", app.GetTheme())
}

func TestAppSaveSettings(t *testing.T) {
	app := NewApp()
	app.SaveSettings(AppSettings{Theme: "light"})
	settings := app.GetSettings()
	assert.Equal(t, "light", settings.Theme)
}

// isolatedSettingsApp returns an App whose settings store writes to a temp dir
// so mode/session tests never touch the developer's real config.
func isolatedSettingsApp(t *testing.T) *App {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	return &App{settings: &settingsStore{filePath: path, settings: AppSettings{Theme: "system"}}}
}

func TestGetAppModeDefaultsToProjects(t *testing.T) {
	app := isolatedSettingsApp(t)
	// First-ever launch: empty Mode → project-first default.
	assert.Equal(t, AppModeProjects, app.GetAppMode())
}

func TestSetAppModePersists(t *testing.T) {
	app := isolatedSettingsApp(t)

	app.SetAppMode(AppModeAdhoc)
	assert.Equal(t, AppModeAdhoc, app.GetAppMode())

	// Reload from disk to confirm persistence.
	reloaded := &settingsStore{filePath: app.settings.filePath}
	reloaded.load()
	assert.Equal(t, AppModeAdhoc, reloaded.settings.Mode)

	// Unknown values fall back to projects.
	app.SetAppMode("garbage")
	assert.Equal(t, AppModeProjects, app.GetAppMode())
}

func TestSessionStateRoundTrip(t *testing.T) {
	app := isolatedSettingsApp(t)

	// Default (nothing saved): projects mode, no projects → frontend falls
	// back to home screen.
	got := app.GetSessionState()
	assert.Equal(t, AppModeProjects, got.Mode)
	assert.Empty(t, got.LastOpenProjects)
	assert.Empty(t, got.ActiveProject)

	app.SaveSessionState(SessionState{
		Mode:             AppModeProjects,
		LastOpenProjects: []string{"/a/project.kapi", "/b/project.kapi", "/a/project.kapi", ""},
		ActiveProject:    "/b/project.kapi",
	})

	got = app.GetSessionState()
	assert.Equal(t, AppModeProjects, got.Mode)
	// Deduped and empty-filtered, order preserved.
	assert.Equal(t, []string{"/a/project.kapi", "/b/project.kapi"}, got.LastOpenProjects)
	assert.Equal(t, "/b/project.kapi", got.ActiveProject)

	// Persists across a fresh store load.
	reloaded := &settingsStore{filePath: app.settings.filePath}
	reloaded.load()
	app2 := &App{settings: reloaded}
	got2 := app2.GetSessionState()
	assert.Equal(t, got.LastOpenProjects, got2.LastOpenProjects)
	assert.Equal(t, got.ActiveProject, got2.ActiveProject)
}

func TestSettingsBackwardCompatibility(t *testing.T) {
	// A legacy settings.json without mode/session fields must still load,
	// keep its existing fields, and default mode to projects.
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	legacy := `{"theme":"dark","ui_language":"qps"}`
	require.NoError(t, os.WriteFile(path, []byte(legacy), 0o644))

	s := &settingsStore{filePath: path}
	s.load()
	app := &App{settings: s}

	assert.Equal(t, "dark", app.GetTheme())
	assert.Equal(t, "qps", app.GetUILanguage())
	assert.Equal(t, AppModeProjects, app.GetAppMode())

	// Saving session state must not clobber the legacy fields.
	app.SaveSessionState(SessionState{Mode: AppModeAdhoc, LastOpenProjects: []string{"/x.kapi"}})
	reloaded := &settingsStore{filePath: path}
	reloaded.load()
	assert.Equal(t, "dark", reloaded.settings.Theme)
	assert.Equal(t, "qps", reloaded.settings.UILanguage)
	assert.Equal(t, AppModeAdhoc, reloaded.settings.Mode)
}
