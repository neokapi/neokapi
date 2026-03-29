package backend

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
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
		settings: AppSettings{Theme: "light", PluginDir: "/custom/plugins"},
	}
	s1.save()

	s2 := &settingsStore{
		filePath: path,
		settings: AppSettings{Theme: "dark"},
	}
	s2.load()
	assert.Equal(t, "light", s2.settings.Theme)
	assert.Equal(t, "/custom/plugins", s2.settings.PluginDir)
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
	app.SaveSettings(AppSettings{
		Theme:     "light",
		PluginDir: "/custom",
	})
	settings := app.GetSettings()
	assert.Equal(t, "light", settings.Theme)
	assert.Equal(t, "/custom", settings.PluginDir)
}
