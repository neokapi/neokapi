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

	// Idempotent: if already scaffolded and the recipe still opens, reuse it.
	// A sample scaffolded by an older app version may carry a recipe that no
	// longer parses against the current schema (e.g. legacy top-level languages
	// or list-form `plugins:`); in that case re-scaffold over it so the sample
	// opens cleanly instead of failing with a YAML unmarshal error.
	if _, err := os.Stat(kapiPath); err == nil {
		if tab, err := a.OpenProject(kapiPath); err == nil {
			return tab, nil
		}
		a.logger.Printf("sample %q recipe is stale/unparseable — re-scaffolding", name)
		// Clear the regenerable state dir first: a tm.db / termbase.db left by an
		// older app version carries an incompatible migration history, so re-seeding
		// into it fails ("apply migration N: no such table ..."). Removing .kapi lets
		// Scaffold create fresh DBs; the user's input/ and output/ are preserved.
		if err := os.RemoveAll(filepath.Join(targetDir, ".kapi")); err != nil {
			return nil, fmt.Errorf("reset stale sample state: %w", err)
		}
		if err := sample.Scaffold(name, targetDir); err != nil {
			return nil, fmt.Errorf("re-scaffold stale sample project: %w", err)
		}
		return a.OpenProject(kapiPath)
	}

	if err := sample.Scaffold(name, targetDir); err != nil {
		return nil, fmt.Errorf("scaffold sample project: %w", err)
	}

	return a.OpenProject(kapiPath)
}
