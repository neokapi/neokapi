package backend

// WorkspaceInfo represents a workspace as exposed to the Bowrain frontend.
type WorkspaceInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	LogoURL     string `json:"logo_url"`
	Role        string `json:"role"`
}

// ListWorkspaces returns the workspaces available to the current user.
// In desktop mode, this returns a single "Personal" workspace.
func (a *App) ListWorkspaces() []WorkspaceInfo {
	return []WorkspaceInfo{
		{
			ID:          "personal",
			Name:        "Personal",
			Slug:        "personal",
			Description: "Your local workspace",
			Role:        "owner",
		},
	}
}

// GetCurrentWorkspace returns the active workspace. In desktop mode,
// this is always the "Personal" workspace.
func (a *App) GetCurrentWorkspace() WorkspaceInfo {
	return WorkspaceInfo{
		ID:   "personal",
		Name: "Personal",
		Slug: "personal",
		Role: "owner",
	}
}
