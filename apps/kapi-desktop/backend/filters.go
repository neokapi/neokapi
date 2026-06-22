package backend

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/project"
)

// ProjectFilter is a saved "Active Filter" — a named narrowing of the project to
// a subset of collections (optionally further by a glob over file paths) and a
// subset of target languages. It scopes every project view and flow run. Shared
// filters live in the committed .kapi/filters.json; personal ones in the
// gitignored .kapi/filters.local.json.
type ProjectFilter struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Collections []string `json:"collections,omitempty"`
	Glob        string   `json:"glob,omitempty"`
	Languages   []string `json:"languages,omitempty"`
	// Shared marks the filter as committed to the project (vs personal/local).
	Shared bool `json:"shared,omitempty"`
}

// ProjectFilters is the merged view handed to the frontend: every saved filter
// (shared then personal) plus the id of the active one (a personal preference).
type ProjectFilters struct {
	Active  string          `json:"active"`
	Filters []ProjectFilter `json:"filters"`
}

// filtersFile is the on-disk shape of each filters file. Active is only
// meaningful in the local file.
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
	shared := readFiltersFile(layout.FiltersPath())
	local := readFiltersFile(layout.LocalFiltersPath())

	out := ProjectFilters{Active: local.Active}
	for _, f := range shared.Filters {
		f.Shared = true
		out.Filters = append(out.Filters, f)
	}
	for _, f := range local.Filters {
		f.Shared = false
		out.Filters = append(out.Filters, f)
	}
	return out
}

// SaveProjectFilter creates or updates a filter, writing it to the shared
// (committed) or local (gitignored) file per f.Shared. A filter that changes
// scope is moved between files. Returns the saved filter (with its assigned id).
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
	// Drop any existing copy from both files first (handles update + scope move).
	removeFilterFromFile(layout.FiltersPath(), f.ID)
	removeFilterFromFile(layout.LocalFiltersPath(), f.ID)

	target := layout.LocalFiltersPath()
	if f.Shared {
		target = layout.FiltersPath()
	} else if err := ensureLocalFiltersGitignored(layout); err != nil {
		return nil, err
	}
	if err := appendFilterToFile(target, f); err != nil {
		return nil, err
	}
	return &f, nil
}

// DeleteProjectFilter removes a filter from whichever file holds it and clears
// the active selection if it pointed at the deleted filter.
func (a *App) DeleteProjectFilter(tabID, filterID string) error {
	layout, ok := a.layoutForTab(tabID)
	if !ok {
		return errors.New("no project for tab")
	}
	removeFilterFromFile(layout.FiltersPath(), filterID)
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

// ─── file helpers ───────────────────────────────────────────────────────────

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

// ensureLocalFiltersGitignored makes sure .kapi/.gitignore excludes the personal
// filters file, matching the convention that the cache/ subdir is ignored.
func ensureLocalFiltersGitignored(layout project.Layout) error {
	path := filepath.Join(layout.StateDir, ".gitignore")
	existing, _ := os.ReadFile(path)
	content := string(existing)
	if strings.Contains(content, project.LocalFiltersFilename) {
		return nil
	}
	if content == "" {
		content = "cache/\n" // seed with the standard ignore
	} else if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += project.LocalFiltersFilename + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}
