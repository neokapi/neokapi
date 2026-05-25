package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/project"
	"github.com/spf13/cobra"
)

// ProjectEnvVar is the environment variable kapi reads to locate a .kapi
// project recipe when the -p flag is not passed. Intended for CI where
// walking up from cwd is awkward.
const ProjectEnvVar = "KAPI_PROJECT"

// NoProjectEnvVar disables implicit project discovery. When set to any
// non-empty value, ResolveProjectPath skips both the KAPI_PROJECT fallback
// and the git-style upward walk, behaving as if no project exists (an
// explicit -p flag still wins). Tests, scripts, and docs-scene recorders set
// this so an in-repo invocation can never silently bind to a checked-in
// recipe (e.g. a repo-root dogfood project). Note that KAPI_PROJECT="" does
// NOT disable discovery — only a non-empty KAPI_NO_PROJECT does.
const NoProjectEnvVar = "KAPI_NO_PROJECT"

// ProjectFlagName is the long flag name for the project-recipe path. All
// project-aware kapi commands should register this flag with the short
// alias "p" using AddProjectFlag.
const ProjectFlagName = "project"

// AddProjectFlag registers the -p / --project flag on a command. Commands
// resolve the flag via ResolveProjectPath so every command uses the same
// semantics: explicit flag > env var > git-style upward walk.
func AddProjectFlag(cmd *cobra.Command) {
	cmd.Flags().StringP(ProjectFlagName, "p", "", "path to a .kapi project recipe (auto-discovered from cwd if omitted)")
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
//
// project.ErrAmbiguousLayout is wrapped with guidance to pass -p explicitly.
func ResolveProjectPath(cmd *cobra.Command) (string, error) {
	if cmd != nil {
		if flag, _ := cmd.Flags().GetString(ProjectFlagName); flag != "" {
			return flag, nil
		}
	}

	// An explicit -p wins above; otherwise KAPI_NO_PROJECT opts out of all
	// implicit discovery so an in-repo invocation can't bind to a checked-in
	// recipe it didn't ask for.
	if os.Getenv(NoProjectEnvVar) != "" {
		return "", nil
	}

	if env := os.Getenv(ProjectEnvVar); env != "" {
		return env, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}
	layout, err := project.ResolveLayout(cwd)
	if err != nil {
		switch {
		case errors.Is(err, project.ErrNoProject):
			return "", nil
		case errors.Is(err, project.ErrAmbiguousLayout):
			return "", fmt.Errorf("%w — pass -p <recipe> to disambiguate", err)
		default:
			return "", err
		}
	}
	return layout.RecipePath, nil
}

// RequireProjectPath resolves the project path and returns an error when no
// project can be located. Use this for commands that do not have a one-shot
// fallback (e.g. kapi merge).
func RequireProjectPath(cmd *cobra.Command) (string, error) {
	path, err := ResolveProjectPath(cmd)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", errors.New("no .kapi project found — pass -p <recipe> or run from inside a kapi project directory")
	}
	return path, nil
}
