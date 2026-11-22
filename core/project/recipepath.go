package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
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
// whichever branch produced it; see RecipeIn. A project the person named, by
// -p or KAPI_PROJECT, that holds no recipe is an error naming the path they
// gave (a *NotAProjectError), so no command goes on to act on a project that
// is not there.
func ResolveRecipePath(explicit string) (string, error) {
	return resolveRecipe(explicit, namedRecipe)
}

// ResolveNewRecipePath is ResolveRecipePath for a command that writes a new
// recipe where the person points it (kapi share, kapi connect). A location
// the person named is returned as RecipeIn resolves it, whether or not a
// recipe is there yet, and the command judges it.
func ResolveNewRecipePath(explicit string) (string, error) {
	return resolveRecipe(explicit, func(given, _ string) (string, error) { return RecipeIn(given), nil })
}

func resolveRecipe(explicit string, named func(given, source string) (string, error)) (string, error) {
	if explicit != "" {
		return named(explicit, "-p")
	}
	if os.Getenv(NoProjectEnvVar) != "" {
		return "", nil
	}
	if env := os.Getenv(ProjectEnvVar); env != "" {
		return named(env, ProjectEnvVar)
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

// NotAProjectError is a project location a person named, by -p or
// KAPI_PROJECT, that holds no recipe: the path does not exist, or it is a
// directory with no kapi.yaml in it.
type NotAProjectError struct {
	// Given is the path as the person wrote it.
	Given string
	// Source names where it came from: "-p" or "KAPI_PROJECT".
	Source string
	// Dir reports that Given is a directory, which holds no kapi.yaml.
	Dir bool
	// NotRecipe reports that Given is a file that is not a recipe: its
	// content is not a YAML mapping.
	NotRecipe bool
}

func (e *NotAProjectError) Error() string {
	if e.NotRecipe {
		return fmt.Sprintf("%s %s is not a kapi recipe. Pass the path to a %s recipe or the directory that holds it",
			e.Source, e.Given, RecipeFileName)
	}
	if e.Dir {
		return fmt.Sprintf("%s %s is not a kapi project: the directory holds no %s. Pass the directory that holds the recipe, or run kapi init there",
			e.Source, e.Given, RecipeFileName)
	}
	return fmt.Sprintf("%s %s does not exist. Pass the path to a %s recipe or the directory that holds it",
		e.Source, e.Given, RecipeFileName)
}

// namedRecipe is RecipeIn for a location a person named, which must hold a
// recipe.
func namedRecipe(given, source string) (string, error) {
	recipe := RecipeIn(given)
	if info, err := os.Stat(recipe); err == nil && !info.IsDir() {
		if !isRecipe(recipe) {
			return "", &NotAProjectError{Given: given, Source: source, NotRecipe: true}
		}
		return recipe, nil
	}
	info, err := os.Stat(given)
	return "", &NotAProjectError{Given: given, Source: source, Dir: err == nil && info.IsDir()}
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
// A recipe that is a symbolic link resolves to the file it names, so the
// project roots at the directory that holds the recipe itself, not at the
// link's directory.
//
// A path that does not resolve is returned as given. ResolveRecipePath then
// reports it, naming the path the person wrote.
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
	return followRecipeLink(abs)
}

// followRecipeLink returns the file a recipe that is a symbolic link names,
// following each link in turn. Only the recipe file itself is followed; the
// directories above it keep the spelling the person used.
func followRecipeLink(path string) string {
	for range 40 {
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			return path
		}
		target, err := os.Readlink(path)
		if err != nil {
			return path
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		path = filepath.Clean(target)
	}
	return path
}

// isRecipe reports whether the file at path can be a recipe: YAML whose
// document is a mapping. It reports false only for YAML that parses to
// something else (a list, a scalar). A file that cannot be read, or that
// fails to parse, is left for the load to report with the parser's line and
// column, since a recipe with a YAML error is still the recipe.
func isRecipe(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
		return true
	}
	return doc.Content[0].Kind == yaml.MappingNode
}
