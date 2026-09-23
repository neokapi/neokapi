package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
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

// layoutForTab resolves the .kapi layout for a tab's project, or ok=false when
// the tab has no saved-on-disk project (e.g. an unsaved/empty project).
func (a *App) layoutForTab(tabID string) (project.Layout, bool) {
	op := a.getOpenProject(tabID)
	if op == nil || op.Path == "" {
		return project.Layout{}, false
	}
	layout, err := project.LayoutFor(op.Path)
	if err != nil {
		return project.Layout{}, false
	}
	return layout, true
}

// GetProjectFilters returns the project's saved filters (shared + personal) and
// the active selection. Empty when the project isn't saved to disk yet.
func (a *App) GetProjectFilters(tabID string) ProjectFilters {
	layout, ok := a.layoutForTab(tabID)
	if !ok {
		return ProjectFilters{}
	}
	local := readFiltersFile(layout.LocalFiltersPath())

	out := ProjectFilters{Active: local.Active}
	// Rendering the filter menu must not bring a store into being, so the
	// shared set is read only from a store the project already has.
	var shared []ProjectFilter
	if db, ok := a.existingProjectStore(a.getOpenProject(tabID)); ok {
		var err error
		if shared, err = readSharedFilters(db); err != nil {
			a.logger.Printf("read shared filters: %v", err)
		}
	}
	for _, f := range shared {
		f.Shared = true
		out.Filters = append(out.Filters, f)
	}
	for _, f := range local.Filters {
		f.Shared = false
		out.Filters = append(out.Filters, f)
	}
	return out
}

// SaveProjectFilter creates or updates a filter: a shared one in the project's
// context store, a personal one in this checkout's local file, per f.Shared. A
// filter that changes scope moves between the two. Returns the saved filter
// (with its assigned id).
func (a *App) SaveProjectFilter(tabID string, f ProjectFilter) (*ProjectFilter, error) {
	layout, ok := a.layoutForTab(tabID)
	if !ok {
		return nil, errors.New("project must be saved before filters can be stored")
	}
	if err := project.EnsureLayout(layout); err != nil {
		return nil, err
	}
	if f.ID == "" {
		f.ID = id.New()
	}
	// Any existing copy is dropped from both places (an update, or a move
	// between shared and personal).
	removeFilterFromFile(layout.LocalFiltersPath(), f.ID)

	if f.Shared {
		db, err := a.projectStore(a.getOpenProject(tabID))
		if err != nil {
			return nil, fmt.Errorf("open the project store: %w", err)
		}
		shared, err := readSharedFilters(db)
		if err != nil {
			return nil, err
		}
		if err := writeSharedFilters(db, append(withoutFilter(shared, f.ID), f)); err != nil {
			return nil, err
		}
		return &f, nil
	}
	if err := a.removeSharedFilter(tabID, f.ID); err != nil {
		return nil, err
	}
	if err := ensureLocalFiltersGitignored(layout); err != nil {
		return nil, err
	}
	if err := appendFilterToFile(layout.LocalFiltersPath(), f); err != nil {
		return nil, err
	}
	return &f, nil
}

// DeleteProjectFilter removes a filter from wherever it is kept and clears the
// active selection if it pointed at the deleted filter.
func (a *App) DeleteProjectFilter(tabID, filterID string) error {
	layout, ok := a.layoutForTab(tabID)
	if !ok {
		return errors.New("no project for tab")
	}
	if err := a.removeSharedFilter(tabID, filterID); err != nil {
		return err
	}
	removeFilterFromFile(layout.LocalFiltersPath(), filterID)

	local := readFiltersFile(layout.LocalFiltersPath())
	if local.Active == filterID {
		local.Active = ""
		_ = writeFiltersFile(layout.LocalFiltersPath(), local)
	}
	return nil
}

// SetActiveFilter records the active filter id (a personal preference, stored in
// the local file). Pass "" to clear (back to "All").
func (a *App) SetActiveFilter(tabID, filterID string) error {
	layout, ok := a.layoutForTab(tabID)
	if !ok {
		return errors.New("no project for tab")
	}
	if err := project.EnsureLayout(layout); err != nil {
		return err
	}
	if err := ensureLocalFiltersGitignored(layout); err != nil {
		return err
	}
	local := readFiltersFile(layout.LocalFiltersPath())
	local.Active = filterID
	return writeFiltersFile(layout.LocalFiltersPath(), local)
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
	for i := range filters {
		filters[i].Shared = false // implied by where it is kept
	}
	data, err := json.Marshal(filters)
	if err != nil {
		return err
	}
	return db.PutSetting(context.Background(), projectdb.SettingSavedFilters, string(data))
}

// removeSharedFilter drops a filter from the shared set. A project with no
// store yet has no shared filter to remove, and none is created for it.
func (a *App) removeSharedFilter(tabID, filterID string) error {
	db, ok := a.existingProjectStore(a.getOpenProject(tabID))
	if !ok {
		return nil
	}
	shared, err := readSharedFilters(db)
	if err != nil {
		return err
	}
	kept := withoutFilter(shared, filterID)
	if len(kept) == len(shared) {
		return nil
	}
	return writeSharedFilters(db, kept)
}

// withoutFilter returns filters with the one carrying filterID left out.
func withoutFilter(filters []ProjectFilter, filterID string) []ProjectFilter {
	kept := make([]ProjectFilter, 0, len(filters))
	for _, f := range filters {
		if f.ID != filterID {
			kept = append(kept, f)
		}
	}
	return kept
}

// ─── personal filters: this checkout's local file ───────────────────────────

func readFiltersFile(path string) filtersFile {
	var f filtersFile
	data, err := os.ReadFile(path)
	if err != nil {
		return f
	}
	_ = json.Unmarshal(data, &f)
	return f
}

func writeFiltersFile(path string, f filtersFile) error {
	if f.Filters == nil {
		f.Filters = []ProjectFilter{}
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func appendFilterToFile(path string, nf ProjectFilter) error {
	ff := readFiltersFile(path)
	ff.Filters = append(ff.Filters, nf)
	return writeFiltersFile(path, ff)
}

func removeFilterFromFile(path, filterID string) {
	ff := readFiltersFile(path)
	kept := make([]ProjectFilter, 0, len(ff.Filters))
	for _, f := range ff.Filters {
		if f.ID != filterID {
			kept = append(kept, f)
		}
	}
	if len(kept) == len(ff.Filters) {
		return // not present — leave the file untouched
	}
	ff.Filters = kept
	_ = writeFiltersFile(path, ff)
}

// ensureLocalFiltersGitignored makes sure .kapi/.gitignore keeps the personal
// filters file out of version control. project.EnsureLayout writes a rule that
// ignores the whole directory; this is the repair path for a `.kapi/` that
// carries a rule of its own, or none because it holds committed files. Either
// way it only ever adds the one line naming the personal file.
func ensureLocalFiltersGitignored(layout project.Layout) error {
	path := filepath.Join(layout.StateDir, project.StateGitignoreFilename)
	existing, _ := os.ReadFile(path)
	content := string(existing)
	if project.GitignoreCovers(content, project.LocalFiltersFilename) {
		return nil
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += project.LocalFiltersFilename + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}
