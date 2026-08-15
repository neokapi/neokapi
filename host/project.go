package host

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/project"
)

// projectEnvVar is the environment variable kapi reads to locate a
// project recipe (kapi.yaml) when the -p flag is not passed. Intended for
// CI where walking up from cwd is awkward.
const projectEnvVar = "KAPI_PROJECT"

// noProjectEnvVar disables implicit project discovery. When set to any
// non-empty value, ResolveProjectPath skips both the KAPI_PROJECT fallback
// and the git-style upward walk, behaving as if no project exists (an
// explicit -p flag still wins). Tests, scripts, and docs-scene recorders set
// this so an in-repo invocation can never silently bind to a checked-in
// recipe (e.g. a repo-root dogfood project). Note that KAPI_PROJECT="" does
// NOT disable discovery — only a non-empty KAPI_NO_PROJECT does.
const noProjectEnvVar = "KAPI_NO_PROJECT"

// projectFlagName is the long flag name for the project-recipe path. All
// project-aware kapi commands should register this flag with the short
// alias "p" using AddProjectFlag.
const projectFlagName = "project"

// AddProjectFlag registers the -p / --project flag on a command. Commands
// resolve the flag via ResolveProjectPath so every command uses the same
// semantics: explicit flag > env var > git-style upward walk.
func AddProjectFlag(cmd Command) {
	cmd.Flags().StringP(projectFlagName, "p", "", "path to a kapi.yaml project recipe or its directory (auto-discovered from cwd if omitted)")
}

// ResolveProjectPath resolves the effective project recipe path for a
// project-aware command. The resolution order is:
//
//  1. Explicit --project / -p flag.
//  2. KAPI_NO_PROJECT set → no implicit project (skip steps 3–4).
//  3. KAPI_PROJECT environment variable.
//  4. project.ResolveLayout(cwd) — git-style upward walk.
//
// Returns an empty path and nil error when nothing is found, so callers can
// fall through to one-shot mode (commands that support it). Callers that
// require a project should check for "" and return a clear error.
func ResolveProjectPath(cmd Command) (string, error) {
	if cmd != nil {
		if flag, _ := cmd.Flags().GetString(projectFlagName); flag != "" {
			return recipeIn(flag), nil
		}
	}

	// An explicit -p wins above; otherwise KAPI_NO_PROJECT opts out of all
	// implicit discovery so an in-repo invocation can't bind to a checked-in
	// recipe it didn't ask for.
	if os.Getenv(noProjectEnvVar) != "" {
		return "", nil
	}

	if env := os.Getenv(projectEnvVar); env != "" {
		return recipeIn(env), nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}
	layout, err := project.ResolveLayout(cwd)
	if err != nil {
		if errors.Is(err, project.ErrNoProject) {
			return "", nil
		}
		return "", err
	}
	return layout.RecipePath, nil
}

// recipeIn resolves a project location a user named — by flag or environment —
// to the recipe file itself. Both spellings the flag help offers are accepted:
// the recipe path, and the directory holding it. A path that is not a directory
// is returned as given, so a recipe under any name still works and a typo
// surfaces as the load error naming the path the user actually wrote.
func recipeIn(path string) string {
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return filepath.Join(path, project.RecipeFileName)
	}
	return path
}

// RequireProjectPath resolves the project path and returns an error when no
// project can be located. Use this for commands that do not have a one-shot
// fallback (e.g. kapi merge).
func RequireProjectPath(cmd Command) (string, error) {
	path, err := ResolveProjectPath(cmd)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", errors.New("no kapi project found — pass -p <path to kapi.yaml> or run from inside a kapi project directory")
	}
	return path, nil
}
