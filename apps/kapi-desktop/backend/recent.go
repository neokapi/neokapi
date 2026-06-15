package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const maxRecentFiles = 10

// RecentFile represents a recently opened .kapi file.
type RecentFile struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	OpenedAt string `json:"opened_at"` // RFC3339
}

// recentStore manages the recent files list.
type recentStore struct {
	mu       sync.Mutex
	filePath string
	files    []RecentFile
	// onChange, when set, is invoked after the list is mutated (add/clear/prune)
	// so the native application menu can rebuild its Recent Projects submenu.
	// It runs outside the store lock.
	onChange func()
}

func newRecentStore() *recentStore {
	path := filepath.Join(desktopConfigDir(), "recent.json")

	s := &recentStore{filePath: path}
	s.load()
	return s
}

// notifyChange fires the onChange hook if one is registered.
func (s *recentStore) notifyChange() {
	if s.onChange != nil {
		s.onChange()
	}
}

func (s *recentStore) add(path, name string) {
	s.mu.Lock()
	// Remove existing entry for this path.
	filtered := make([]RecentFile, 0, len(s.files))
	for _, f := range s.files {
		if f.Path != path {
			filtered = append(filtered, f)
		}
	}

	// Prepend new entry.
	entry := RecentFile{
		Path:     path,
		Name:     name,
		OpenedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.files = append([]RecentFile{entry}, filtered...)

	// Trim to max.
	if len(s.files) > maxRecentFiles {
		s.files = s.files[:maxRecentFiles]
	}

	s.save()
	s.mu.Unlock()
	s.notifyChange()
}

func (s *recentStore) list() []RecentFile {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []RecentFile
	changed := false
	for _, f := range s.files {
		if _, err := os.Stat(f.Path); err == nil {
			out = append(out, f)
		} else {
			changed = true
		}
	}
	// Prune deleted entries from the persisted list.
	if changed {
		s.files = out
		s.save()
	}
	return out
}

func (s *recentStore) clear() {
	s.mu.Lock()
	s.files = nil
	s.save()
	s.mu.Unlock()
	s.notifyChange()
}

func (s *recentStore) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &s.files)
}

func (s *recentStore) save() {
	data, err := json.MarshalIndent(s.files, "", "  ")
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

// ListRecentFiles returns the list of recently opened .kapi files.
func (a *App) ListRecentFiles() []RecentFile {
	return a.recent.list()
}

// ClearRecentFiles clears the recent files list.
func (a *App) ClearRecentFiles() {
	a.recent.clear()
}
