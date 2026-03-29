package backend

import (
	"github.com/neokapi/neokapi/core/project"
)

// OpenProjectDialog shows a native file-open dialog for .kapi files,
// opens the selected file, and returns the project.
func (a *App) OpenProjectDialog() (*project.KapiProject, error) {
	if a.app == nil {
		return nil, nil
	}

	path, err := a.app.Dialog.OpenFile().
		SetTitle("Open Kapi Project").
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

// SaveProjectDialog shows a native file-save dialog for .kapi files.
func (a *App) SaveProjectDialog() error {
	if a.app == nil {
		return nil
	}

	a.mu.RLock()
	proj := a.project
	a.mu.RUnlock()

	if proj == nil {
		return nil
	}

	path, err := a.app.Dialog.SaveFile().
		AddFilter("Kapi Projects", "*.kapi").
		SetFilename(proj.Name + ".kapi").
		PromptForSingleSelection()
	if err != nil {
		return err
	}
	if path == "" {
		return nil // user canceled
	}

	return a.SaveProjectAs(path)
}
