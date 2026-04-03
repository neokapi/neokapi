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
// When connected to a server, returns server workspaces; otherwise returns
// a single "Personal" local workspace.
func (a *App) ListWorkspaces() []WorkspaceInfo {
	if a.isConnected() {
		ws, err := a.remote.ListWorkspaces()
		if err == nil && len(ws) > 0 {
			return ws
		}
		// Fall through to local on error.
	}
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

// GetCurrentWorkspace returns the active workspace.
func (a *App) GetCurrentWorkspace() WorkspaceInfo {
	if a.isConnected() {
		a.mu.RLock()
		slug := a.activeWS
		a.mu.RUnlock()
		if slug != "" {
			ws, err := a.remote.ListWorkspaces()
			if err == nil {
				for _, w := range ws {
					if w.Slug == slug {
						return w
					}
				}
			}
		}
	}
	return WorkspaceInfo{
		ID:   "personal",
		Name: "Personal",
		Slug: "personal",
		Role: "owner",
	}
}
