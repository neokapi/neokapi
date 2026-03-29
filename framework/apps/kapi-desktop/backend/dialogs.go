package backend

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
func (a *App) SaveProjectDialog(tabID string) error {
	if a.app == nil {
		return nil
	}

	op := a.getOpenProject(tabID)
	if op == nil {
		return nil
	}

	path, err := a.app.Dialog.SaveFile().
		AddFilter("Kapi Projects", "*.kapi").
		SetFilename(op.Project.Name + ".kapi").
		PromptForSingleSelection()
	if err != nil {
		return err
	}
	if path == "" {
		return nil // user canceled
	}

	return a.SaveProjectAs(tabID, path)
}
