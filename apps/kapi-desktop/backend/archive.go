package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/project"
)

// OpenPath dispatches on file extension: a `.kapi` recipe goes
// straight into OpenProject; a `.klz` archive is extracted into a
// sibling directory and its contained recipe is opened.
//
// Called from the macOS file-association / double-click hook and
// (eventually) from an "Open…" file chooser that permits either
// extension.
func (a *App) OpenPath(path string) (*TabInfo, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case project.RecipeExt:
		return a.OpenProject(path)
	case ".klz":
		recipe, err := a.OpenArchive(path, "")
		if err != nil {
			return nil, err
		}
		return a.OpenProject(recipe)
	default:
		return nil, fmt.Errorf("unsupported file extension %q — expected .kapi or .klz", ext)
	}
}

// OpenArchive extracts a `.klz` archive into a working project
// directory and returns the path to the recipe. `targetDir`, when
// empty, defaults to a sibling directory named after the archive
// filename stem (e.g. `my-app.klz` → `my-app/` next to it).
//
// The target directory is created on demand. If it exists and is
// non-empty, returns an error to avoid overwriting user files.
func (a *App) OpenArchive(archivePath, targetDir string) (string, error) {
	info, err := os.Stat(archivePath)
	if err != nil {
		return "", fmt.Errorf("stat archive: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory, not a .klz file", archivePath)
	}

	if targetDir == "" {
		base := filepath.Base(archivePath)
		stem := base
		if ext := filepath.Ext(base); ext != "" {
			stem = base[:len(base)-len(ext)]
		}
		targetDir = filepath.Join(filepath.Dir(archivePath), stem)
	}
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return "", fmt.Errorf("resolve target: %w", err)
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	layout, err := project.Open(f, info.Size(), absTarget)
	if err != nil {
		return "", fmt.Errorf("extract archive: %w", err)
	}

	// Track the freshly-opened project in recents so subsequent
	// launches show it in the File menu.
	a.recent.add(layout.RecipePath, filepath.Base(layout.RecipePath))

	return layout.RecipePath, nil
}
