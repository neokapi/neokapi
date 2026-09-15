package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ProjectEnvVar names a project recipe (kapi.yaml), or the directory holding
// it, for a command given no explicit project. Intended for CI, where walking
// up from the working directory is awkward.
const ProjectEnvVar = "KAPI_PROJECT"

// NoProjectEnvVar disables implicit project discovery. With any non-empty
// value, ResolveRecipePath skips both KAPI_PROJECT and the upward walk and
// resolves no project, while an explicit path still resolves. Tests, scripts
// and docs-scene recorders set it so an in-repo invocation never binds to a
// checked-in recipe it did not name. KAPI_PROJECT="" leaves discovery on.
const NoProjectEnvVar = "KAPI_NO_PROJECT"

// ResolveRecipePath resolves the project recipe a kapi command acts on, in
// this order:
//
//  1. explicit, the command's -p value, when non-empty.
//  2. KAPI_NO_PROJECT set: no project.
//  3. KAPI_PROJECT.
//  4. ResolveLayout from the working directory, a git-style upward walk.
//
// It returns an empty path and a nil error when nothing names or holds a
// project. A path that exists is returned absolute and names the recipe file,
// whichever branch produced it; see RecipeIn.
func ResolveRecipePath(explicit string) (string, error) {
	if explicit != "" {
		return RecipeIn(explicit), nil
	}
	if os.Getenv(NoProjectEnvVar) != "" {
		return "", nil
	}
	if env := os.Getenv(ProjectEnvVar); env != "" {
		return RecipeIn(env), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}
	layout, err := ResolveLayout(cwd)
	if err != nil {
		if errors.Is(err, ErrNoProject) {
			return "", nil
		}
		return "", err
	}
	return layout.RecipePath, nil
}

// RecipeIn resolves a project location a user named, by flag or environment,
// to the absolute path of the recipe file itself. Both spellings the -p help
// offers are accepted: the recipe path, and the directory holding it.
//
// Callers take filepath.Dir of this value as the project root and relativize
// the paths they write into the committed record against it. Handed
// `-p kapi.yaml`, a relative result gives the root ".", nothing relativizes,
// and `kapi commit` writes machine-specific absolute paths into `.kapi/state/`,
// a file that travels in git. Discovery already returns an absolute path, so
// only the branches a user spells need this.
//
// A path that does not resolve is returned as given, so a recipe under any name
// still works and a typo surfaces as the load error naming the path the user
// wrote.
func RecipeIn(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return path
	}
	if info.IsDir() {
		path = filepath.Join(path, RecipeFileName)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
