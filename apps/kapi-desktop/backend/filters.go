package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/atomicfile"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/storage/filelock"
)

// ProjectFilter is a saved "Active Filter" — a named narrowing of the project to
// a subset of collections (optionally further by a glob over file paths) and a
// subset of target languages. It scopes every project view and flow run.
//
// A shared filter is a team setting about the project, so it is kept in the
// project's context store (projectdb.SettingSavedFilters), which every checkout
// of the project reads. A personal filter, and the choice of active filter,
// belong to this checkout and sit in .kapi/filters.local.json.
type ProjectFilter struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Collections []string `json:"collections,omitempty"`
	Glob        string   `json:"glob,omitempty"`
	Languages   []string `json:"languages,omitempty"`
	// Shared marks the filter as the team's, kept in the project's context
	// store, rather than personal to this checkout.
	Shared bool `json:"shared,omitempty"`
}

// ProjectFilters is the merged view handed to the frontend: every saved filter
// (shared then personal) plus the id of the active one (a personal preference).
type ProjectFilters struct {
	Active  string          `json:"active"`
	Filters []ProjectFilter `json:"filters"`
}

// filtersFile is the shape of the personal filters file.
type filtersFile struct {
	Active  string          `json:"active,omitempty"`
	Filters []ProjectFilter `json:"filters"`
}

type filterSettings struct {
	local  filtersFile
	shared []ProjectFilter
}

// GetProjectFilters reads the shared filters and this checkout’s personal filters and selection.
func (a *App) GetProjectFilters(tabID string) (ProjectFilters, error) {
	op := a.getOpenProject(tabID)
	if op == nil || op.Path == "" {
		return ProjectFilters{}, nil
	}
	op.filtersMu.RLock()
	defer op.filtersMu.RUnlock()
	layout, err := project.LayoutFor(op.Path)
	if err != nil {
		return ProjectFilters{}, err
	}
	db, err := a.projectStoreIfPresent(op)
	if err != nil {
		return ProjectFilters{}, err
	}
	settings, err := readFilterSettings(layout, db)
	if err != nil {
		return ProjectFilters{}, err
	}
	out := ProjectFilters{Active: settings.local.Active}
	for _, f := range settings.shared {
		f.Shared = true
		out.Filters = append(out.Filters, f)
	}
	for _, f := range settings.local.Filters {
		f.Shared = false
		out.Filters = append(out.Filters, f)
	}
	return out, nil
}

// SaveProjectFilter saves a filter in its requested scope and returns its ID.
func (a *App) SaveProjectFilter(tabID string, f ProjectFilter) (*ProjectFilter, error) {
	if f.ID == "" {
		f.ID = id.New()
	}
	err := a.editFilters(tabID, f.Shared, func(next *filterSettings) {
		next.remove(f.ID)
		saved := f
		saved.Shared = false
		if f.Shared {
			next.shared = append(slices.Clone(next.shared), saved)
		} else {
			next.local.Filters = append(slices.Clone(next.local.Filters), saved)
		}
	})
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// DeleteProjectFilter deletes the filter and clears its active selection.
func (a *App) DeleteProjectFilter(tabID, filterID string) error {
	return a.editFilters(tabID, false, func(next *filterSettings) {
		next.remove(filterID)
		if next.local.Active == filterID {
			next.local.Active = ""
		}
	})
}

// SetActiveFilter records the selection for this checkout. An empty ID clears it.
func (a *App) SetActiveFilter(tabID, filterID string) error {
	return a.editFilters(tabID, false, func(next *filterSettings) {
		next.local.Active = filterID
	})
}

func (s *filterSettings) remove(id string) {
	s.local.Filters = withoutFilter(s.local.Filters, id)
	s.shared = withoutFilter(s.shared, id)
}

func (a *App) editFilters(tabID string, createShared bool, edit func(*filterSettings)) error {
	op := a.getOpenProject(tabID)
	if op == nil || op.Path == "" {
		return errors.New("project must be saved before filters can be stored")
	}
	op.filtersMu.Lock()
	defer op.filtersMu.Unlock()
	layout, err := project.LayoutFor(op.Path)
	if err != nil {
		return err
	}
	if err := project.EnsureLayout(layout); err != nil {
		return err
	}
	unlock, err := lockFilters(filepath.Join(layout.CacheDir(), "filters.lock"))
	if err != nil {
		return err
	}
	defer unlock()

	var db *projectdb.DB
	if createShared {
		db, err = a.projectStore(op)
	} else {
		db, err = a.projectStoreIfPresent(op)
	}
	if err != nil {
		return err
	}
	if db != nil {
		unlock, err := lockFilters(db.ContextPath() + ".filters.lock")
		if err != nil {
			return err
		}
		defer unlock()
	}
	previous, err := readFilterSettings(layout, db)
	if err != nil {
		return err
	}
	next := previous
	edit(&next)
	return next.persist(layout, db, previous, createShared)
}

func lockFilters(path string) (func(), error) {
	lock, err := filelock.Open(path)
	if err != nil {
		return nil, err
	}
	if err := lock.Lock(context.Background()); err != nil {
		return nil, errors.Join(err, lock.Close())
	}
	return func() {
		lock.Unlock()
		_ = lock.Close()
	}, nil
}

func readFilterSettings(layout project.Layout, db *projectdb.DB) (filterSettings, error) {
	var s filterSettings
	data, err := os.ReadFile(layout.LocalFiltersPath())
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &s.local); err != nil {
			return s, fmt.Errorf("decode personal filters: %w", err)
		}
	case !errors.Is(err, os.ErrNotExist):
		return s, err
	}
	if db != nil {
		s.shared, err = readSharedFilters(db)
		return s, err
	}
	return s, nil
}

// A scope change writes the destination before removing the source. Both locks stay held through rollback.
func (s filterSettings) persist(layout project.Layout, db *projectdb.DB, previous filterSettings, sharedFirst bool) error {
	sharedChanged := !reflect.DeepEqual(s.shared, previous.shared)
	if reflect.DeepEqual(s.local, previous.local) {
		if sharedChanged {
			return writeSharedFilters(db, s.shared)
		}
		return nil
	}
	if err := ensureLocalFiltersGitignored(layout); err != nil {
		return err
	}
	path := layout.LocalFiltersPath()
	if !sharedChanged {
		return writeFiltersFile(path, s.local)
	}
	if sharedFirst {
		if err := writeSharedFilters(db, s.shared); err != nil {
			return err
		}
		if err := writeFiltersFile(path, s.local); err != nil {
			return errors.Join(err, writeSharedFilters(db, previous.shared))
		}
		return nil
	}
	if err := writeFiltersFile(path, s.local); err != nil {
		return err
	}
	if err := writeSharedFilters(db, s.shared); err != nil {
		return errors.Join(err, writeFiltersFile(path, previous.local))
	}
	return nil
}

func writeFiltersFile(path string, local filtersFile) error {
	if local.Filters == nil {
		local.Filters = []ProjectFilter{}
	}
	_, err := atomicfile.Replace(path, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(local)
	})
	return err
}

// ─── filter application ──────────────────────────────────────────────────────

// FilesNarrowed reports whether the filter constrains files (collections/glob);
// the languages dimension is applied separately by each caller.
func (f ProjectFilter) FilesNarrowed() bool {
	return len(f.Collections) > 0 || strings.TrimSpace(f.Glob) != ""
}

// MatchesFile reports whether a resolved file (by its collection and
// project-relative path) passes the filter's collection + glob narrowing.
func (f ProjectFilter) MatchesFile(collection, relative string) bool {
	if len(f.Collections) > 0 && collection != "" {
		found := slices.Contains(f.Collections, collection)
		if !found {
			return false
		}
	}
	if g := strings.TrimSpace(f.Glob); g != "" && !matchGlobPath(g, relative) {
		return false
	}
	return true
}

// matchGlobPath matches a project-relative path against a simple glob, mirroring
// the frontend (lib/filter.ts): `*` within a segment, `**` across segments, `?`
// one non-separator char; a glob with no `/` matches anywhere in the tree.
func matchGlobPath(glob, path string) bool {
	g := strings.TrimSpace(glob)
	if g == "" {
		return true
	}
	if !strings.Contains(g, "/") {
		g = "**/" + g
	}
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(g); i++ {
		c := g[i]
		switch {
		case c == '*':
			if i+1 < len(g) && g[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case c == '?':
			b.WriteString("[^/]")
		case strings.IndexByte(`\^$.|+()[]{}`, c) >= 0:
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return true // a malformed glob shouldn't hide files
	}
	return re.MatchString(path)
}

// ─── shared filters: the project's context store ────────────────────────────

// readSharedFilters reads the shared filters from the project's context store.
func readSharedFilters(db *projectdb.DB) ([]ProjectFilter, error) {
	raw, ok, err := db.Setting(context.Background(), projectdb.SettingSavedFilters)
	if err != nil || !ok {
		return nil, err
	}
	var filters []ProjectFilter
	if err := json.Unmarshal([]byte(raw), &filters); err != nil {
		return nil, fmt.Errorf("decode the shared filters: %w", err)
	}
	return filters, nil
}

// writeSharedFilters replaces the shared filters in the project's context store.
func writeSharedFilters(db *projectdb.DB, filters []ProjectFilter) error {
	if filters == nil {
		filters = []ProjectFilter{}
	}
	data, err := json.Marshal(filters)
	if err != nil {
		return err
	}
	return db.PutSetting(context.Background(), projectdb.SettingSavedFilters, string(data))
}

func withoutFilter(filters []ProjectFilter, filterID string) []ProjectFilter {
	matches := func(f ProjectFilter) bool { return f.ID == filterID }
	if !slices.ContainsFunc(filters, matches) {
		return filters
	}
	return slices.DeleteFunc(slices.Clone(filters), matches)
}

// ensureLocalFiltersGitignored makes sure .kapi/.gitignore keeps the personal
// filters file out of version control. project.EnsureLayout writes a rule that
// ignores the whole directory; this is the repair path for a `.kapi/` that
// carries a rule of its own, or none because it holds committed files. Either
// way it only ever adds the one line naming the personal file.
func ensureLocalFiltersGitignored(layout project.Layout) error {
	path := filepath.Join(layout.StateDir, project.StateGitignoreFilename)
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	content := string(existing)
	if project.GitignoreCovers(content, project.LocalFiltersFilename) {
		return nil
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += project.LocalFiltersFilename + "\n"
	_, err = atomicfile.Replace(path, func(w io.Writer) error {
		_, err := io.WriteString(w, content)
		return err
	})
	return err
}
