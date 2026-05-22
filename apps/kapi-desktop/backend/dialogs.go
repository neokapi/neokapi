package backend

import (
	"os"
	"path/filepath"
	"strings"
)

// BrowsePathFilter mirrors the schema-form host's SchemaFormFileFilter.
// Extensions is a space-delimited glob list, e.g. "*.tmx" or "*.html *.htm".
type BrowsePathFilter struct {
	Name       string `json:"name"`
	Extensions string `json:"extensions"`
}

// BrowsePathRequest mirrors the schema-form host's SchemaFormBrowseRequest.
// It is the generic browse request the schema-form PathPicker widget hands to
// the host so file/folder path fields get native dialogs in kapi-desktop.
type BrowsePathRequest struct {
	// Kind is "file" for a file dialog or "directory" for a folder dialog.
	Kind string `json:"kind"`
	// Field is the schema property name driving the request (for logging/routing).
	Field string `json:"field"`
	// CurrentValue seeds the dialog with the field's current path.
	CurrentValue string `json:"currentValue,omitempty"`
	// Title is an optional dialog title from the schema.
	Title string `json:"title,omitempty"`
	// ForSaveAs requests a save dialog instead of an open dialog (file kind only).
	ForSaveAs bool `json:"forSaveAs,omitempty"`
	// Filters are named file-extension filters (file dialogs only).
	Filters []BrowsePathFilter `json:"filters,omitempty"`
	// Accepts are bare extension hints (e.g. ["html", "txt"]) from x-path.accepts.
	Accepts []string `json:"accepts,omitempty"`
}

// BrowsePath shows a native file/folder dialog for a schema-form path field and
// returns the chosen path (empty string when the user cancels). It is the
// host-side implementation of the shared schema-form PathPicker widget's
// `onBrowse` capability: kapi-desktop maps the generic browse request onto the
// same Wails dialog API that powers OpenProjectDialog / AddFilesDialog.
func (a *App) BrowsePath(req BrowsePathRequest) (string, error) {
	if a.app == nil {
		return "", nil
	}

	// Directory picker: an open-file dialog scoped to directories.
	if req.Kind == "directory" {
		dlg := a.app.Dialog.OpenFile().
			CanChooseDirectories(true).
			CanChooseFiles(false).
			CanCreateDirectories(true)
		if req.Title != "" {
			dlg.SetTitle(req.Title)
		}
		if dir := seedDirectory(req.CurrentValue); dir != "" {
			dlg.SetDirectory(dir)
		}
		path, err := dlg.PromptForSingleSelection()
		if err != nil {
			return "", err
		}
		return path, nil
	}

	// Save-as file dialog.
	if req.ForSaveAs {
		dlg := a.app.Dialog.SaveFile().CanCreateDirectories(true)
		if req.Title != "" {
			dlg.SetMessage(req.Title)
		}
		for _, f := range browseFilters(req) {
			dlg.AddFilter(f.name, f.pattern)
		}
		if dir, name := seedDirAndName(req.CurrentValue); dir != "" || name != "" {
			if dir != "" {
				dlg.SetDirectory(dir)
			}
			if name != "" {
				dlg.SetFilename(name)
			}
		}
		path, err := dlg.PromptForSingleSelection()
		if err != nil {
			return "", err
		}
		return path, nil
	}

	// Open file dialog.
	dlg := a.app.Dialog.OpenFile().
		CanChooseFiles(true).
		CanChooseDirectories(false)
	if req.Title != "" {
		dlg.SetTitle(req.Title)
	}
	for _, f := range browseFilters(req) {
		dlg.AddFilter(f.name, f.pattern)
	}
	if dir := seedDirectory(req.CurrentValue); dir != "" {
		dlg.SetDirectory(dir)
	}
	path, err := dlg.PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	return path, nil
}

// browseFilters converts the schema-form filters/accepts into Wails dialog
// filters. Wails expects a semicolon-separated glob list ("*.html;*.htm"),
// while the schema-form filters carry a space-delimited list ("*.html *.htm").
// Bare `accepts` hints (e.g. "html") are folded into a single filter. An
// "All Files" catch-all is always appended so users can override the filter.
func browseFilters(req BrowsePathRequest) []struct{ name, pattern string } {
	var out []struct{ name, pattern string }
	for _, f := range req.Filters {
		pattern := toWailsPattern(strings.Fields(f.Extensions))
		if pattern == "" {
			continue
		}
		name := f.Name
		if name == "" {
			name = "Files"
		}
		out = append(out, struct{ name, pattern string }{name, pattern})
	}
	if len(req.Accepts) > 0 {
		globs := make([]string, 0, len(req.Accepts))
		for _, ext := range req.Accepts {
			ext = strings.TrimSpace(ext)
			if ext == "" {
				continue
			}
			ext = strings.TrimPrefix(ext, ".")
			globs = append(globs, "*."+ext)
		}
		if pattern := toWailsPattern(globs); pattern != "" {
			out = append(out, struct{ name, pattern string }{"Supported Files", pattern})
		}
	}
	out = append(out, struct{ name, pattern string }{"All Files", "*"})
	return out
}

// toWailsPattern joins globs into the semicolon-separated form Wails expects.
func toWailsPattern(globs []string) string {
	cleaned := make([]string, 0, len(globs))
	for _, g := range globs {
		g = strings.TrimSpace(g)
		if g != "" {
			cleaned = append(cleaned, g)
		}
	}
	return strings.Join(cleaned, ";")
}

// seedDirectory returns a directory to seed a dialog with, derived from the
// field's current value (its parent directory, or the value itself if it is an
// existing directory). Returns "" when nothing useful can be derived.
func seedDirectory(current string) string {
	if current == "" {
		return ""
	}
	if info, err := os.Stat(current); err == nil && info.IsDir() {
		return current
	}
	dir := filepath.Dir(current)
	if dir == "." || dir == "" {
		return ""
	}
	return dir
}

// seedDirAndName splits a current path into the directory + filename used to
// seed a save dialog.
func seedDirAndName(current string) (dir, name string) {
	if current == "" {
		return "", ""
	}
	return filepath.Dir(current), filepath.Base(current)
}

// OpenProjectDialog shows a native file-open dialog for Kapi project files,
// opens the selected file, and returns the tab info.
func (a *App) OpenProjectDialog() (*TabInfo, error) {
	if a.app == nil {
		return nil, nil
	}

	path, err := a.app.Dialog.OpenFile().
		AddFilter("Kapi Projects", "*.kapi").
		AddFilter("All Files", "*").
		PromptForSingleSelection()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil // user canceled
	}

	return a.OpenProject(path)
}

// SaveProjectDialog shows a native file-save dialog for a project tab.
// Ensures the .kapi extension is appended and updates the project name
// to match the filename.
func (a *App) SaveProjectDialog(tabID string) (*TabInfo, error) {
	if a.app == nil {
		return nil, nil
	}

	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, nil
	}

	dlg := a.app.Dialog.SaveFile().
		AddFilter("Kapi Projects", "*.kapi").
		SetFilename(projectDisplayName(op.Project, op.Path) + ".kapi")

	// Default to base path directory for the save dialog.
	if dir := a.GetBasePath(tabID); dir != "" {
		dlg.SetDirectory(dir)
	}

	path, err := dlg.PromptForSingleSelection()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil // user canceled
	}

	// Ensure .kapi extension.
	if !strings.HasSuffix(strings.ToLower(path), ".kapi") {
		path += ".kapi"
	}

	// Update the project name to match the filename (without extension).
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	if name != "" {
		op.Project.Name = name
	}

	if err := a.SaveProjectAs(tabID, path); err != nil {
		return nil, err
	}

	return &TabInfo{ID: tabID, Name: projectDisplayName(op.Project, path), Path: path}, nil
}

// BrowseProjectLocation shows a native directory picker and returns the selected path.
// Used during "New Project" to choose where to save the project.
func (a *App) BrowseProjectLocation() string {
	if a.app == nil {
		return ""
	}

	path, err := a.app.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		PromptForSingleSelection()
	if err != nil || path == "" {
		return ""
	}
	return path
}
