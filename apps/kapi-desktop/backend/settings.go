package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// App modes. The desktop is project-first: "projects" is the default for a
// first-ever launch. "adhoc" is the secondary loose-file "quick tools" path.
const (
	AppModeProjects = "projects"
	AppModeAdhoc    = "adhoc"
)

// AppSettings holds persisted user preferences.
type AppSettings struct {
	Theme            string         `json:"theme"`                       // "system", "dark", or "light"
	UILanguage       string         `json:"ui_language,omitempty"`       // BCP-47 UI language code, e.g. "en" or "qps"
	SamplesDismissed bool           `json:"samples_dismissed,omitempty"` // true after user dismisses sample project cards
	HiddenLocales    []string       `json:"hidden_locales,omitempty"`    // locale codes to hide from selectors
	CustomLocales    []CustomLocale `json:"custom_locales,omitempty"`    // additional locales not in the well-known list

	// Mode persists the app's project-first vs ad-hoc preference across
	// launches. Empty (legacy settings) is treated as AppModeProjects.
	Mode string `json:"mode,omitempty"`

	// LastOpenProjects is the ordered list of project recipe paths that
	// were open when the app last persisted its session. Most-recently
	// activated first. The frontend restores these tabs at startup.
	LastOpenProjects []string `json:"last_open_projects,omitempty"`

	// ActiveProject is the recipe path of the project tab that was active
	// (focused) in the last session. Empty when no project was open.
	ActiveProject string `json:"active_project,omitempty"`

	// DefaultCredentialID is the credential the desktop falls back to when a
	// flow step needs an AI provider but pins none of its own. It holds a
	// credential ID (which carries the provider it links to). Empty means no
	// default — a run then auto-detects, which only succeeds unambiguously
	// with a single saved credential.
	DefaultCredentialID string `json:"default_credential_id,omitempty"`
}

// CustomLocale is a user-defined locale with code and display name.
type CustomLocale struct {
	Code        string `json:"code"`
	DisplayName string `json:"display_name,omitempty"`
}

// SessionState is the persisted project-first session the frontend restores
// at startup: which mode to open in and which projects to reopen.
type SessionState struct {
	Mode             string   `json:"mode"`
	LastOpenProjects []string `json:"lastOpenProjects"`
	ActiveProject    string   `json:"activeProject"`
}

// settingsStore manages user preferences.
type settingsStore struct {
	mu       sync.Mutex
	filePath string
	settings AppSettings
}

func newSettingsStore() *settingsStore {
	path := filepath.Join(desktopConfigDir(), "settings.json")

	s := &settingsStore{
		filePath: path,
		settings: AppSettings{Theme: "system", UILanguage: "en"},
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

// GetUILanguage returns the persisted UI language code (e.g. "en", "qps").
func (a *App) GetUILanguage() string {
	a.settings.mu.Lock()
	defer a.settings.mu.Unlock()
	if a.settings.settings.UILanguage == "" {
		return "en"
	}
	return a.settings.settings.UILanguage
}

// SetUILanguage updates the UI language preference and rebuilds the
// metadata Translator so the very next ListTools/ListFormats/etc. call
// returns strings in the new locale — no desktop restart needed.
// Frontend updates are single-threaded (one user toggling settings);
// the SetLocale call re-resolves catalogs, which is cheap (cached MO
// reads).
func (a *App) SetUILanguage(lang string) {
	a.settings.mu.Lock()
	a.settings.settings.UILanguage = lang
	a.settings.save()
	a.settings.mu.Unlock()
	a.SetLocale(lang)
}

// GetDefaultCredential returns the ID of the credential the desktop uses when
// a flow step needs an AI provider but pins none. Empty when no default is set.
func (a *App) GetDefaultCredential() string {
	a.settings.mu.Lock()
	defer a.settings.mu.Unlock()
	return a.settings.settings.DefaultCredentialID
}

// SetDefaultCredential persists the fallback credential id (pass "" to clear).
func (a *App) SetDefaultCredential(id string) {
	a.settings.mu.Lock()
	defer a.settings.mu.Unlock()
	a.settings.settings.DefaultCredentialID = id
	a.settings.save()
}

// DismissSamples marks the sample project cards as dismissed.
func (a *App) DismissSamples() {
	a.settings.mu.Lock()
	defer a.settings.mu.Unlock()
	a.settings.settings.SamplesDismissed = true
	a.settings.save()
}

// normalizeMode coerces a persisted/requested mode into one of the two valid
// values, defaulting to project-first for empty or unknown input.
func normalizeMode(mode string) string {
	if mode == AppModeAdhoc {
		return AppModeAdhoc
	}
	return AppModeProjects
}

// GetAppMode returns the persisted app mode ("projects" or "adhoc").
// Defaults to "projects" (project-first) for a first-ever launch or when the
// persisted value is empty/unknown. The frontend calls this at startup.
func (a *App) GetAppMode() string {
	a.settings.mu.Lock()
	defer a.settings.mu.Unlock()
	return normalizeMode(a.settings.settings.Mode)
}

// SetAppMode persists the app mode. Unknown values fall back to "projects".
func (a *App) SetAppMode(mode string) {
	a.settings.mu.Lock()
	defer a.settings.mu.Unlock()
	a.settings.settings.Mode = normalizeMode(mode)
	a.settings.save()
}

// GetSessionState returns the persisted project-first session: the mode plus
// the projects to restore and which one was active. The frontend reads this
// at startup to reopen the previous session; if LastOpenProjects is empty it
// falls back to the home screen.
func (a *App) GetSessionState() SessionState {
	a.settings.mu.Lock()
	defer a.settings.mu.Unlock()
	s := a.settings.settings
	paths := make([]string, 0, len(s.LastOpenProjects))
	for _, p := range s.LastOpenProjects {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return SessionState{
		Mode:             normalizeMode(s.Mode),
		LastOpenProjects: paths,
		ActiveProject:    s.ActiveProject,
	}
}

// SaveSessionState persists the project-first session so the next launch can
// restore the open tabs. The frontend calls this whenever the open-project set
// or the active tab changes (and at shutdown). Mode is normalized; empty or
// unknown values become "projects".
func (a *App) SaveSessionState(state SessionState) {
	a.settings.mu.Lock()
	defer a.settings.mu.Unlock()
	paths := make([]string, 0, len(state.LastOpenProjects))
	seen := make(map[string]bool, len(state.LastOpenProjects))
	for _, p := range state.LastOpenProjects {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}
	a.settings.settings.Mode = normalizeMode(state.Mode)
	a.settings.settings.LastOpenProjects = paths
	a.settings.settings.ActiveProject = state.ActiveProject
	a.settings.save()
}
