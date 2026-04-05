package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// AppSettings holds persisted user preferences.
type AppSettings struct {
	Theme            string   `json:"theme"`                        // "system", "dark", or "light"
	SamplesDismissed bool     `json:"samples_dismissed,omitempty"`  // true after user dismisses sample project cards
	HiddenLocales    []string `json:"hidden_locales,omitempty"`     // locale codes to hide from selectors
	CustomLocales    []string `json:"custom_locales,omitempty"`     // additional locale codes not in the well-known list
}

// settingsStore manages user preferences.
type settingsStore struct {
	mu       sync.Mutex
	filePath string
	settings AppSettings
}

func newSettingsStore() *settingsStore {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	path := filepath.Join(configDir, "kapi-desktop", "settings.json")

	s := &settingsStore{
		filePath: path,
		settings: AppSettings{Theme: "system"},
	}
	s.load()
	return s
}

func (s *settingsStore) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &s.settings)
}

func (s *settingsStore) save() {
	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return
	}
	dir := filepath.Dir(s.filePath)
	_ = os.MkdirAll(dir, 0o755)

	tmp := s.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, s.filePath)
}

// --- App methods ---

// GetSettings returns the current app settings.
func (a *App) GetSettings() AppSettings {
	a.settings.mu.Lock()
	defer a.settings.mu.Unlock()
	return a.settings.settings
}

// SaveSettings updates and persists app settings.
func (a *App) SaveSettings(s AppSettings) {
	a.settings.mu.Lock()
	defer a.settings.mu.Unlock()
	a.settings.settings = s
	a.settings.save()
}

// GetTheme returns the current theme preference.
func (a *App) GetTheme() string {
	a.settings.mu.Lock()
	defer a.settings.mu.Unlock()
	return a.settings.settings.Theme
}

// SetTheme updates the theme preference.
func (a *App) SetTheme(theme string) {
	a.settings.mu.Lock()
	defer a.settings.mu.Unlock()
	a.settings.settings.Theme = theme
	a.settings.save()
}

// DismissSamples marks the sample project cards as dismissed.
func (a *App) DismissSamples() {
	a.settings.mu.Lock()
	defer a.settings.mu.Unlock()
	a.settings.settings.SamplesDismissed = true
	a.settings.save()
}
