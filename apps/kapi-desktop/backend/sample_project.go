package backend

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/kapi-desktop/backend/sample"
)

// CreateSampleProject scaffolds a sample project and opens it as a tab.
// name must be "kapimart" or "okapimart".
// If the project already exists on disk, it is opened without re-scaffolding.
func (a *App) CreateSampleProject(name string) (*TabInfo, error) {
	displayName, ok := sample.DisplayName[name]
	if !ok {
		return nil, fmt.Errorf("unknown sample project %q", name)
	}

	home, err := userHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}

	targetDir := filepath.Join(home, "KapiProjects", displayName)
	kapiPath := filepath.Join(targetDir, "project.kapi")

	// Idempotent: if already scaffolded, just open it.
	if _, err := os.Stat(kapiPath); err == nil {
		return a.OpenProject(kapiPath)
	}

	if err := sample.Scaffold(name, targetDir); err != nil {
		return nil, fmt.Errorf("scaffold sample project: %w", err)
	}

	return a.OpenProject(kapiPath)
}
