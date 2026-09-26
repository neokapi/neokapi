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
// content iteration) lives in host/venue/source.
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

// Load loads the project whose recipe is at path: the kapi.yaml file itself, a
// recipe under another name, or the directory holding kapi.yaml. It never walks
// up from path. Which project a command acts on is decided once, by the host's
// resolver (host.ResolveProjectPath: -p, then KAPI_NO_PROJECT, then
// KAPI_PROJECT, then the upward walk from the working directory), and the path
// it returns is what this loads. A second walk here would let a plugin act on a
// project other than the one kapi resolved.
func Load(path string) (*Project, error) {
	if path == "" {
		return nil, errors.New("no project recipe named")
	}
	layout, err := coreproj.LayoutFor(path)
	if err != nil {
		return nil, err
	}
	r, err := LoadRecipe(layout.RecipePath)
	if err != nil {
		return nil, err
	}
	return &Project{
		Root:   layout.Root,
		Layout: layout,
		Recipe: r,
	}, nil
}

// InitProject creates a new kapi recipe at <root>/kapi.yaml and the
// checkout's .kapi/ cache directory. The supplied recipe is saved
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
	recipePath := filepath.Join(absRoot, coreproj.RecipeFileName)
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

// RecipePath is the absolute path to the kapi.yaml recipe.
func (p *Project) RecipePath() string { return p.Layout.RecipePath }

// StateDir is the absolute path to the checkout's .kapi/ cache directory.
func (p *Project) StateDir() string { return p.Layout.StateDir }

// CacheDir is the absolute path to the .kapi/work/cache/ directory.
func (p *Project) CacheDir() string { return p.Layout.CacheDir() }

// FlowsDirPath returns the absolute path of the directory the recipe names
// with `flows_dir:`, one YAML file per flow, or "" when it names none. `kapi
// run` resolves a flow from here when the recipe declares none inline under
// `flows:`.
func (p *Project) FlowsDirPath() string {
	if p.Recipe == nil {
		return ""
	}
	return p.Recipe.FlowsDirIn(p.Root)
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
