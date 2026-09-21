package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/project"
)

// Per-call project scope for the MCP surface.
//
// One `kapi mcp` process serves an assistant that moves between projects, so
// the project a call acts on is an argument of the call. Every project-scoped
// tool takes an optional `project`, and the `context://` resource takes a
// `?project=` parameter. Resolution runs through the seam the CLI uses
// (ResolveProjectPath → project.ResolveRecipePath → project.ResolveLayout), so
// an MCP call and a `kapi -p …` invocation reach the same recipe.
//
// The project resolved when the server started remains the default, which is
// what a client configured with one working directory keeps getting.
//
// Handlers thread the resolved recipe through the Command they build, the way
// every embedded surface does. Nothing about a call is written back onto the
// App, so two calls for two projects can run at once: the store handles they
// reach are memoized per project root by ProjectDB and released together by
// Shutdown.

// MCPProjectError reports that an MCP call named a project that could not be
// resolved. Path is what the call asked for, so a client is told which of its
// arguments to correct.
type MCPProjectError struct {
	// Path is the `project` argument as the call spelled it.
	Path string
	// Reason says what the resolution found, in the terms of the walk.
	Reason string
	// Err is the underlying resolution error, when there was one.
	Err error
}

func (e *MCPProjectError) Error() string {
	if e.Path == "" {
		return "no kapi project: " + e.Reason
	}
	return fmt.Sprintf("project %q: %s", e.Path, e.Reason)
}

func (e *MCPProjectError) Unwrap() error { return e.Err }

// MCPRecipePath reports the recipe `kapi mcp` resolved when it started, or an
// empty string when it started outside a project. It is the default for every
// call that names none.
func (a *App) MCPRecipePath() string { return a.mcpRecipePath }

// ResolveMCPCallProject resolves the recipe one MCP call acts on:
//
//  1. explicit, the call's `project` argument. It may name the recipe file, the
//     project root, or any path inside the project; a path that holds no
//     project is refused with an *MCPProjectError naming it.
//  2. the recipe `kapi mcp` resolved at start.
//  3. ordinary discovery: KAPI_PROJECT, then the git-style upward walk from the
//     working directory, with KAPI_NO_PROJECT honoured.
//
// An empty path and a nil error mean no project is in scope, which the
// ad-hoc tools (check over a snippet, a profile named by file) run in.
func (a *App) ResolveMCPCallProject(explicit string) (string, error) {
	if explicit != "" {
		return resolveNamedMCPProject(explicit)
	}
	if a.mcpRecipePath != "" {
		return a.mcpRecipePath, nil
	}
	return ResolveProjectPath(nil)
}

// RequireMCPCallProject is ResolveMCPCallProject for a tool that has no
// ad-hoc mode, so "no project" is refused rather than returned.
func (a *App) RequireMCPCallProject(explicit string) (string, error) {
	path, err := a.ResolveMCPCallProject(explicit)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", &MCPProjectError{Reason: "pass `project` (the recipe, the project root, or a path inside it), or start the MCP server inside a kapi project"}
	}
	return path, nil
}

// resolveNamedMCPProject resolves the path a call named. A directory or a
// recipe file resolves directly; anything else inside a project resolves by the
// same upward walk `kapi` runs from its working directory, so an assistant can
// pass the file it is editing.
func resolveNamedMCPProject(named string) (string, error) {
	info, err := os.Stat(named)
	if err != nil {
		return "", &MCPProjectError{Path: named, Reason: "names no file or directory", Err: err}
	}
	if info.IsDir() {
		if recipe := filepath.Join(named, project.RecipeFileName); fileExists(recipe) {
			return absOrAsGiven(recipe), nil
		}
		return walkToMCPProject(named)
	}
	if filepath.Base(named) == project.RecipeFileName {
		return absOrAsGiven(named), nil
	}
	if recipe, werr := walkToMCPProject(named); werr == nil {
		return recipe, nil
	}
	// A recipe under another name, named directly, outside a tree the walk can
	// resolve: the spelling `kapi -p ./review.yaml` accepts.
	if ext := strings.ToLower(filepath.Ext(named)); ext == ".yaml" || ext == ".yml" {
		return absOrAsGiven(named), nil
	}
	return "", &MCPProjectError{Path: named, Reason: "sits in no kapi project (no kapi.yaml with a .kapi/ beside it, here or above)"}
}

// walkToMCPProject runs the git-style upward walk from a path inside a project.
func walkToMCPProject(from string) (string, error) {
	layout, err := project.ResolveLayout(from)
	if err != nil {
		reason := "sits in no kapi project (no kapi.yaml with a .kapi/ beside it, here or above)"
		if !errors.Is(err, project.ErrNoProject) {
			reason = err.Error()
		}
		return "", &MCPProjectError{Path: from, Reason: reason, Err: err}
	}
	return layout.RecipePath, nil
}

// absOrAsGiven makes a resolved recipe absolute, leaving it as written when the
// working directory cannot be read. Callers take filepath.Dir of the result as
// the project root, so a relative path would root the project at ".".
func absOrAsGiven(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

// mcpCallCommand builds the Command an MCP handler threads its call's project
// through, and returns the recipe it resolved ("" when no project is in scope).
//
// The project flag is registered on every command this builds, set or not, so
// the resolvers downstream read one answer: with a recipe, the call's project;
// without one, the empty flag that makes ResolveProjectPath fall through to
// discovery.
func (a *App) mcpCallCommand(ctx context.Context, name, explicit string) (*EnvCommand, string, error) {
	recipe, err := a.ResolveMCPCallProject(explicit)
	if err != nil {
		return nil, "", err
	}
	cmd := NewEnvCommand(ctx, name)
	cmd.Flags().String(projectFlagName, recipe, "")
	return cmd, recipe, nil
}

// mcpProjectContext loads the project context for one call's recipe, so a tool
// can read what that project declares without any of it reaching the App.
// A call with no project in scope gets (nil, nil).
func (a *App) mcpProjectContext(explicit string) (*project.ProjectContext, error) {
	recipe, err := a.ResolveMCPCallProject(explicit)
	if err != nil || recipe == "" {
		return nil, err
	}
	proj, lerr := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	if lerr != nil {
		return nil, fmt.Errorf("load project %s: %w", DisplayName(recipe), lerr)
	}
	return project.NewProjectContext(proj, recipe), nil
}

// mcpCallSourceLocale is the source language one MCP call reads content in: the
// `defaults.source_language` of the recipe it resolved, with a source language
// named when the server started still winning, the way an explicit
// --source-lang wins on the command line.
//
// The App's own SourceLocale answers for the recipe the server started in and
// for a call that resolved no project at all. It cannot answer for the others:
// one server serves several projects, two calls run at once, and a field on the
// App would hold whichever project answered last.
//
// A recipe that will not load falls back to the App's language. The call is
// about to load the same recipe for its voice, terms and formats, and fails
// there with the error a reader can act on.
func (a *App) mcpCallSourceLocale(recipe string) string {
	if recipe == "" || recipe == a.mcpRecipePath {
		return a.SourceLocale()
	}
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return a.SourceLocale()
	}
	return ResolveSourceLocale(a.mcpNamedSourceLang, proj.Defaults.SourceLanguage)
}
