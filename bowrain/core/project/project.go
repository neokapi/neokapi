// Package project provides bowrain-specific helpers around a kapi project.
//
// The recipe schema, loader, validator, and layout discovery for the
// framework portion of a recipe live in the framework's core/project
// package. This package adds:
//
//   - The bowrain Recipe type (recipe.go) that embeds the framework
//     KapiProject and layers bowrain extensions (Server, Hooks,
//     Automations, Assets, BrandVoice) on top.
//   - The bowrain workflow context (Project, this file) that bundles a
//     loaded Recipe with its on-disk Layout for use by the source
//     connector and the bowrain CLI.
//
// The source connector itself (and its tests covering NewSourceConnector,
// Push/Pull/Status, sync-cache persistence, target-path templating, and
// content iteration) lives in bowrain/plugin/connector.
package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	coreproj "github.com/neokapi/neokapi/core/project"
)

// Project is the bowrain workflow context: a parsed bowrain Recipe paired
// with the on-disk layout for its state directory and cache.
type Project struct {
	// Root is the project root directory (sibling of the .kapi/ state
	// dir and the *.kapi recipe file).
	Root string

	// Layout describes the on-disk shape: state dir, cache dir, etc.
	// Owned by the framework.
	Layout coreproj.Layout

	// Recipe is the parsed bowrain Recipe (framework KapiProject embedded
	// + bowrain extensions decoded from its YAML).
	Recipe *Recipe
}

// FindProject discovers the kapi recipe by walking up from startDir
// (defaults to the current working directory). Returns the parsed bowrain
// Recipe paired with its Layout.
func FindProject(startDir string) (*Project, error) {
	if startDir == "" {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
	}
	r, layout, err := FindRecipe(startDir)
	if err != nil {
		return nil, err
	}
	return &Project{
		Root:   layout.Root,
		Layout: layout,
		Recipe: r,
	}, nil
}

// InitProject creates a new kapi recipe at <root>/<dir-name>.kapi and
// scaffolds the .kapi/ state directory. The supplied recipe is saved
// verbatim (both framework and bowrain fields).
func InitProject(root string, recipe *Recipe) (*Project, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("absolute path: %w", err)
	}
	if err := os.MkdirAll(absRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create root: %w", err)
	}

	name := filepath.Base(absRoot)
	if name == "." || name == string(filepath.Separator) {
		return nil, errors.New("cannot derive project name from root path")
	}
	recipePath := filepath.Join(absRoot, name+coreproj.RecipeExt)
	if _, err := os.Stat(recipePath); err == nil {
		return nil, fmt.Errorf("recipe already exists at %s", recipePath)
	}

	if recipe.Version == "" {
		recipe.Version = coreproj.CurrentVersion
	}
	if recipe.Name == "" {
		recipe.Name = name
	}
	if err := SaveRecipe(recipePath, recipe); err != nil {
		return nil, fmt.Errorf("save recipe: %w", err)
	}

	layout, err := coreproj.LayoutFor(recipePath)
	if err != nil {
		return nil, fmt.Errorf("layout: %w", err)
	}
	if err := coreproj.EnsureLayout(layout); err != nil {
		return nil, fmt.Errorf("scaffold state dir: %w", err)
	}

	return &Project{
		Root:   absRoot,
		Layout: layout,
		Recipe: recipe,
	}, nil
}

// Save persists the current recipe back to its on-disk path.
func (p *Project) Save() error {
	return SaveRecipe(p.Layout.RecipePath, p.Recipe)
}

// RecipePath is the absolute path to the *.kapi recipe.
func (p *Project) RecipePath() string { return p.Layout.RecipePath }

// StateDir is the absolute path to the .kapi/ state directory.
func (p *Project) StateDir() string { return p.Layout.StateDir }

// CacheDir is the absolute path to the .kapi/cache/ directory.
func (p *Project) CacheDir() string { return p.Layout.CacheDir() }

// FlowsDirPath returns the path to .kapi/flows/, the optional file-per-flow
// store. Bowrain reads flow definitions from here in addition to inline
// definitions on the recipe.
func (p *Project) FlowsDirPath() string {
	return filepath.Join(p.Layout.StateDir, "flows")
}

// SyncCachePath is the path to the bowrain sync cache. Bowrain owns this
// path; the framework's Layout exposes only generic CacheDir / BlockStore
// / Extractions / Collections paths.
func (p *Project) SyncCachePath() string {
	return filepath.Join(p.Layout.CacheDir(), SyncCacheFilename)
}

// ResolvePath resolves a local path relative to the project root.
func (p *Project) ResolvePath(localPath string) string {
	if filepath.IsAbs(localPath) {
		return localPath
	}
	return filepath.Join(p.Root, localPath)
}

// RelativePath returns a path relative to the project root.
func (p *Project) RelativePath(absPath string) (string, error) {
	return filepath.Rel(p.Root, absPath)
}
