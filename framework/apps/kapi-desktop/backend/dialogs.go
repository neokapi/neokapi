package backend

import (
	"path/filepath"
	"strings"
)

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
		SetFilename(op.Project.Name + ".kapi")

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

	return &TabInfo{ID: tabID, Name: op.Project.Name, Path: path}, nil
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
