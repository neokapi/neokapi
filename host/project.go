package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/neokapi/neokapi/core/project"
)

// projectEnvVar and noProjectEnvVar are the environment variables
// ResolveProjectPath reads; project.ResolveRecipePath documents both.
const (
	projectEnvVar   = project.ProjectEnvVar
	noProjectEnvVar = project.NoProjectEnvVar
)

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
//  4. project.ResolveLayout(cwd), git-style upward walk.
//
// Returns an empty path and nil error when nothing is found, so callers can
// fall through to one-shot mode (commands that support it). Callers that
// require a project should check for "" and return a clear error.
//
// A resolved path is absolute and names the recipe file, whichever branch
// produced it. Callers take filepath.Dir of it as the project root. See
// project.RecipeIn for why that matters. The source-connector routes a plugin
// daemon runs resolve through the same project.ResolveRecipePath.
//
// A -p or KAPI_PROJECT that holds no recipe fails here, with an error that
// names the path the person gave (project.NotAProjectError), before any
// command acts on it.
func ResolveProjectPath(cmd Command) (string, error) {
	return resolveProjectPath(cmd, project.ResolveRecipePath)
}

// ResolveNewProjectPath is ResolveProjectPath for a command that writes a new
// recipe where -p points (kapi share, kapi connect): the path is returned
// whether or not a recipe is there yet, and the command judges it.
func ResolveNewProjectPath(cmd Command) (string, error) {
	return resolveProjectPath(cmd, project.ResolveNewRecipePath)
}

func resolveProjectPath(cmd Command, resolve func(string) (string, error)) (string, error) {
	var flag string
	if cmd != nil {
		if f := cmd.Flags().Lookup(detachedFlagName); f != nil && f.Value.String() == "true" {
			return "", nil
		}
		flag, _ = cmd.Flags().GetString(projectFlagName)
	}
	return resolve(flag)
}

// detachedFlagName marks a command built to resolve no project at all: neither
// the environment nor discovery from the working directory binds one to it. A
// service that edits outside a project hands one to the hooks that resolve a
// project from a command. No command a person runs declares it.
const detachedFlagName = "kapi-detached"

// detachedCommand is a command named name that resolves no project.
func detachedCommand(ctx context.Context, name string) Command {
	cmd := NewEnvCommand(ctx, name)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.Flags().Bool(detachedFlagName, true, "")
	return cmd
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
		if os.Getenv(noProjectEnvVar) != "" {
			return "", fmt.Errorf("no kapi project: %s is set and no -p was given. Pass -p <path to kapi.yaml>", noProjectEnvVar)
		}
		return "", errors.New("no kapi project found. Pass -p <path to kapi.yaml> or run from inside a kapi project directory")
	}
	return path, nil
}
