package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const maxRecentFiles = 10

// Reasons a remembered project can no longer be opened. These are the values
// of RecentFile.Unavailable and the frontend narrows on them.
const (
	// recentReasonMoved: the project folder itself is gone from that location.
	recentReasonMoved = "moved"
	// recentReasonDeleted: the folder is still there, the recipe inside it is not.
	recentReasonDeleted = "deleted"
)

// RecentFile represents a recently opened project recipe.
type RecentFile struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	OpenedAt string `json:"opened_at"` // RFC3339
	// Available reports whether the recipe is still readable at Path. The
	// list() call recomputes it on every read, so a persisted value is only
	// ever a record of what was true when the entry was written.
	Available bool `json:"available"`
	// Unavailable names the shape of the loss when Available is false:
	// "moved" when the project folder is gone, "deleted" when the folder is
	// still there without a recipe in it.
	Unavailable string `json:"unavailable,omitempty"`
}

// recipeStatus reports whether a recipe path still resolves to a file on disk,
// and when it does not, which shape of loss it is. The two cases read very
// differently to a person: a folder that vanished was moved or removed
// wholesale, a folder that survived without its recipe lost one file.
func recipeStatus(path string) (available bool, reason string) {
	if path == "" {
		return false, recentReasonMoved
	}
	if _, err := os.Stat(path); err == nil {
		return true, ""
	}
	if fi, err := os.Stat(filepath.Dir(path)); err == nil && fi.IsDir() {
		return false, recentReasonDeleted
	}
	return false, recentReasonMoved
}

// recipeGoneError describes a recipe that cannot be opened, in the terms of
// whichever loss recipeStatus found.
func recipeGoneError(path, reason string) error {
	dir := filepath.Dir(path)
	if reason == recentReasonDeleted {
		return fmt.Errorf("%s is missing from %s", filepath.Base(path), dir)
	}
	return fmt.Errorf("the project folder %s no longer exists", dir)
}

// recentStore manages the recent files list.
type recentStore struct {
	mu       sync.Mutex
	filePath string
	files    []RecentFile
	// onChange, when set, is invoked after the list is mutated (add/remove/clear)
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
		Path:      path,
		Name:      name,
		OpenedAt:  time.Now().UTC().Format(time.RFC3339),
		Available: true,
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

// list returns every remembered project, each stamped with whether its recipe
// is still on disk. An entry whose recipe is gone stays in the list, marked
// unavailable, so the user sees where the project went and removes it when
// they are ready. Dropping it silently loses the only trace of it.
func (s *recentStore) list() []RecentFile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RecentFile, 0, len(s.files))
	for _, f := range s.files {
		f.Available, f.Unavailable = recipeStatus(f.Path)
		out = append(out, f)
	}
	return out
}

// remove drops one project from the recent list by recipe path.
func (s *recentStore) remove(path string) {
	s.mu.Lock()
	kept := make([]RecentFile, 0, len(s.files))
	for _, f := range s.files {
		if f.Path != path {
			kept = append(kept, f)
		}
	}
	changed := len(kept) != len(s.files)
	if changed {
		s.files = kept
		s.save()
	}
	s.mu.Unlock()
	if changed {
		s.notifyChange()
	}
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

// ListRecentFiles returns the recently opened projects, each stamped with
// whether its recipe is still on disk.
func (a *App) ListRecentFiles() []RecentFile {
	return a.recent.list()
}

// RemoveRecentFile drops one project from the recent list. The frontend calls
// it from the remove action on an unavailable row; nothing on disk is touched.
func (a *App) RemoveRecentFile(path string) {
	a.recent.remove(path)
}

// ClearRecentFiles clears the recent files list.
func (a *App) ClearRecentFiles() {
	a.recent.clear()
}
