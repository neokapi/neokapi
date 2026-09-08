package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/telemetry"
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

	// Only a recipe that is on disk is restored, so the round trip needs real
	// files behind the remembered paths.
	a := createTempKapi(t, "a")
	b := createTempKapi(t, "b")

	app.SaveSessionState(SessionState{
		Mode:             AppModeProjects,
		LastOpenProjects: []string{a, b, a, ""},
		ActiveProject:    b,
	})

	got = app.GetSessionState()
	assert.Equal(t, AppModeProjects, got.Mode)
	// Deduped and empty-filtered, order preserved.
	assert.Equal(t, []string{a, b}, got.LastOpenProjects)
	assert.Equal(t, b, got.ActiveProject)

	// Persists across a fresh store load.
	reloaded := &settingsStore{filePath: app.settings.filePath}
	reloaded.load()
	app2 := &App{settings: reloaded}
	got2 := app2.GetSessionState()
	assert.Equal(t, got.LastOpenProjects, got2.LastOpenProjects)
	assert.Equal(t, got.ActiveProject, got2.ActiveProject)
}

// TestSessionStateDropsGoneProjects covers #2560: a remembered project whose
// recipe is no longer on disk is left out of the restore set, so the frontend
// never opens a tab for it.
func TestSessionStateDropsGoneProjects(t *testing.T) {
	kept := createTempKapi(t, "kept")
	deleted := createTempKapi(t, "deleted")
	moved := createTempKapi(t, "moved")

	require.NoError(t, os.Remove(deleted))                // folder stays, recipe gone
	require.NoError(t, os.RemoveAll(filepath.Dir(moved))) // whole folder gone

	app := isolatedSettingsApp(t)
	app.SaveSessionState(SessionState{
		Mode:             AppModeProjects,
		LastOpenProjects: []string{deleted, kept, moved},
		ActiveProject:    deleted,
	})

	got := app.GetSessionState()
	assert.Equal(t, []string{kept}, got.LastOpenProjects)
	assert.Empty(t, got.ActiveProject, "an active project that is gone must not be restored")

	// The persisted list is untouched, so the recent list can still report
	// where those projects were until the user removes them.
	reloaded := &settingsStore{filePath: app.settings.filePath}
	reloaded.load()
	assert.Equal(t, []string{deleted, kept, moved}, reloaded.settings.LastOpenProjects)
}

// TestSessionStateKeepsActiveWhenOnlyOthersAreGone guards the narrower case
// where the focused project survives and its neighbours do not.
func TestSessionStateKeepsActiveWhenOnlyOthersAreGone(t *testing.T) {
	kept := createTempKapi(t, "kept")
	gone := createTempKapi(t, "gone")
	require.NoError(t, os.RemoveAll(filepath.Dir(gone)))

	app := isolatedSettingsApp(t)
	app.SaveSessionState(SessionState{
		Mode:             AppModeProjects,
		LastOpenProjects: []string{gone, kept},
		ActiveProject:    kept,
	})

	got := app.GetSessionState()
	assert.Equal(t, []string{kept}, got.LastOpenProjects)
	assert.Equal(t, kept, got.ActiveProject)
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

// TestTelemetryProjectionFollowsSharedConfig is finding 4: the desktop
// telemetry state is a projection of the shared kapi config, so `kapi
// telemetry off` disables desktop analytics and the machine reports one
// identity — not a second, settings.json-local opt-out and a separate id.
func TestTelemetryProjectionFollowsSharedConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", dir)

	app := isolatedSettingsApp(t)

	// `kapi telemetry off` writes telemetry.enabled=false to the shared config;
	// the desktop projection must reflect it.
	require.NoError(t, app.SetTelemetryEnabled(false))
	cfg := config.NewAppConfig()
	require.NoError(t, cfg.Load())
	assert.Equal(t, "false", cfg.GetString(telemetry.KeyEnabled))
	assert.True(t, app.GetSettings().TelemetryDisabled, "desktop must honor the CLI's telemetry off")

	// The toggle round-trips through the same shared key the CLI writes.
	require.NoError(t, app.SetTelemetryEnabled(true))
	cfg = config.NewAppConfig()
	require.NoError(t, cfg.Load())
	assert.Equal(t, "true", cfg.GetString(telemetry.KeyEnabled))

	// One machine identity: the desktop reports exactly the shared machine id.
	got := app.GetSettings()
	assert.NotEmpty(t, got.MachineID)
	assert.Equal(t, telemetry.MachineID(cfg), got.MachineID)
	assert.Equal(t, got.MachineID, app.GetSettings().MachineID, "machine id is stable")

	// The notice-shown flag lives in the shared config too.
	require.NoError(t, app.MarkTelemetryNoticeShown())
	assert.True(t, app.GetSettings().TelemetryNoticeShown)
}

// TestSaveSettingsDoesNotPersistTelemetry confirms the telemetry projection is
// never written into settings.json — it belongs to the shared kapi config.
func TestSaveSettingsDoesNotPersistTelemetry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KAPI_CONFIG_DIR", dir)
	app := isolatedSettingsApp(t)

	app.SaveSettings(AppSettings{Theme: "light", TelemetryDisabled: true, MachineID: "should-not-persist"})

	reloaded := &settingsStore{filePath: app.settings.filePath}
	reloaded.load()
	assert.Equal(t, "light", reloaded.settings.Theme)
	assert.False(t, reloaded.settings.TelemetryDisabled, "telemetry opt-out must not land in settings.json")
	assert.Empty(t, reloaded.settings.MachineID, "machine id must not land in settings.json")
}
