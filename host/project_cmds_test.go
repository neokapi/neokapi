package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/preset"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInitProject_NeokapiI18nCleanLayout: `kapi init --framework
// neokapi-i18n` writes the preset's clean nested layout (source under
// i18n/src/, per-locale targets under i18n/{lang}/) before any catalog exists,
// and the recipe loads through the real loader.
func TestInitProject_NeokapiI18nCleanLayout(t *testing.T) {
	res, err := InitProject(t.TempDir(), InitOptions{
		Name:          "MyApp",
		TargetLocales: []string{"de", "fr", "nb"},
		Framework:     preset.NeokapiI18nPresetName,
	})
	require.NoError(t, err)

	proj, err := project.Load(res.RecipePath)
	require.NoError(t, err)
	require.Len(t, proj.Collections, 1)
	assert.Equal(t, "i18n/src/**/*.kbf.json", proj.Collections[0].Path)
	assert.Equal(t, "i18n/{lang}/{path}.kbf.json", proj.Collections[0].Target)
	assert.Nil(t, proj.Defaults.Voice, "a voice is bound by name once the store holds one")
}

// TestInitProject: the whole `kapi init` composition (propose collections,
// write the recipe, create the cache directory) is one host call. It is
// idempotent, and it writes no manifest: `.kapi/` is this checkout's cache.
func TestInitProject(t *testing.T) {
	dir := t.TempDir()

	res, err := InitProject(dir, InitOptions{Name: "MyApp", TargetLocales: []string{"fr"}})
	require.NoError(t, err)
	assert.False(t, res.AlreadyInitialized)
	assert.Equal(t, "MyApp", res.Name)
	assert.Equal(t, []string{"fr"}, res.TargetLanguages)

	proj, err := project.Load(res.RecipePath)
	require.NoError(t, err)
	assert.Equal(t, "MyApp", proj.Name)
	stateDir := filepath.Join(dir, project.StateDirName)
	assert.DirExists(t, stateDir)
	assert.NoFileExists(t, filepath.Join(stateDir, "manifest.yaml"))

	// Idempotent: a second run adopts the existing recipe and leaves it be,
	// and reads its languages back from it.
	before, err := os.ReadFile(res.RecipePath)
	require.NoError(t, err)
	res2, err := InitProject(dir, InitOptions{Name: "Renamed"})
	require.NoError(t, err)
	assert.True(t, res2.AlreadyInitialized)
	assert.Equal(t, []string{"fr"}, res2.TargetLanguages)
	after, err := os.ReadFile(res.RecipePath)
	require.NoError(t, err)
	assert.Equal(t, before, after, "an existing recipe is left untouched")
}

// Every project kapi scaffolds is born with a stable id, and a recipe that
// already exists is adopted exactly as it stands: no id appears in it and
// nothing about the file moves.
func TestInitProject_MintsAnIDForANewProject(t *testing.T) {
	tests := []struct {
		name string
		opts InitOptions
	}{
		{name: "content scaffold", opts: InitOptions{Name: "MyApp"}},
		{name: "translation scaffold", opts: InitOptions{Name: "MyApp", TargetLocales: []string{"fr"}}},
		{name: "framework scaffold", opts: InitOptions{Name: "MyApp", Framework: preset.NeokapiI18nPresetName}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := InitProject(t.TempDir(), tt.opts)
			require.NoError(t, err)
			require.True(t, res.IDMinted)
			require.NoError(t, project.ValidateID(res.ID))

			proj, err := project.Load(res.RecipePath)
			require.NoError(t, err)
			assert.Equal(t, res.ID, proj.ID)
			assert.Equal(t, res.ID, proj.Identity(), "identity is the id, not the name")
		})
	}
}

// An existing recipe with no id keeps working, unchanged and unannotated, until
// someone asks for one.
func TestInitProject_AdoptsARecipeWithNoID(t *testing.T) {
	dir := t.TempDir()
	recipePath := filepath.Join(dir, project.RecipeFileName)
	const recipe = "version: v1\nname: legacy\n"
	require.NoError(t, os.WriteFile(recipePath, []byte(recipe), 0o644))

	res, err := InitProject(dir, InitOptions{})
	require.NoError(t, err)
	assert.True(t, res.AlreadyInitialized)
	assert.False(t, res.IDMinted)
	assert.Empty(t, res.ID)

	after, err := os.ReadFile(recipePath)
	require.NoError(t, err)
	assert.Equal(t, recipe, string(after), "adoption writes nothing")

	proj, err := project.Load(recipePath)
	require.NoError(t, err)
	assert.Empty(t, proj.ID)
	assert.Equal(t, "legacy", proj.Identity(), "identity falls back to the name")
}

// The recipe a commented tutorial explains is the one people keep, so minting
// an id into it must leave every comment, every blank line and the order of
// every key exactly as they were.
func TestMintProjectID_PreservesTheDocument(t *testing.T) {
	const recipe = `version: v1
# The label people read. Rename it freely.
name: legacy

defaults:
  # English is what we author in.
  source_language: en
  target_languages:
    - fr # the first market
    - de

# Everything below is content we govern.
collections:
  - path: "docs/**/*.md"
    format: markdown
`
	dir := t.TempDir()
	recipePath := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipePath, []byte(recipe), 0o644))

	id, minted, err := MintProjectID(recipePath)
	require.NoError(t, err)
	require.True(t, minted)
	require.NoError(t, project.ValidateID(id))

	after, err := os.ReadFile(recipePath)
	require.NoError(t, err)
	got := string(after)

	for line := range strings.SplitSeq(recipe, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		assert.Containsf(t, got, line, "the recipe keeps %q", line)
	}
	assert.Equal(t, strings.Count(recipe, "\n\n"), strings.Count(got, "\n\n"),
		"the blank lines between sections are where they were")
	assert.Contains(t, got, "id: "+id)

	proj, err := project.Load(recipePath)
	require.NoError(t, err)
	assert.Equal(t, id, proj.ID)
	assert.Equal(t, "legacy", proj.Name)
	require.Len(t, proj.Collections, 1)
	assert.Equal(t, "docs/**/*.md", proj.Collections[0].Path)

	// Asking again reports the id the recipe carries and writes nothing.
	again, mintedAgain, err := MintProjectID(recipePath)
	require.NoError(t, err)
	assert.Equal(t, id, again)
	assert.False(t, mintedAgain)
	unchanged, err := os.ReadFile(recipePath)
	require.NoError(t, err)
	assert.Equal(t, got, string(unchanged))
}

// InitProject routes --mint-id at an adopted recipe and reports what it did.
func TestInitProject_MintIDOnAnExistingRecipe(t *testing.T) {
	dir := t.TempDir()
	recipePath := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipePath, []byte("version: v1\nname: legacy\n"), 0o644))

	res, err := InitProject(dir, InitOptions{MintID: true})
	require.NoError(t, err)
	require.True(t, res.AlreadyInitialized)
	require.True(t, res.IDMinted)
	require.NoError(t, project.ValidateID(res.ID))

	// A second run finds the id already there and leaves it.
	res2, err := InitProject(dir, InitOptions{MintID: true})
	require.NoError(t, err)
	assert.False(t, res2.IDMinted)
	assert.Equal(t, res.ID, res2.ID)
}

// TestInitProject_ContentScaffold: with no target locales and no framework, the
// on-brand content scaffold is written (not the translation one).
func TestInitProject_ContentScaffold(t *testing.T) {
	dir := t.TempDir()
	res, err := InitProject(dir, InitOptions{})
	require.NoError(t, err)
	assert.Equal(t, filepath.Base(dir), res.Name, "name defaults to the dir basename")

	proj, err := project.Load(res.RecipePath)
	require.NoError(t, err)
	assert.Empty(t, proj.Defaults.TargetLanguages, "content scaffold declares no targets")
}

// TestScaffoldRecipe_EmptyShowsTheShape: a tree with nothing kapi recognises
// gets an empty list under a commented example, and the example loads when a
// person uncomments it.
func TestScaffoldRecipe_EmptyShowsTheShape(t *testing.T) {
	yaml := string(ScaffoldRecipe("MyApp", project.NewID(), "en", []string{"fr"}, nil))
	require.Contains(t, yaml, "collections: []")
	assert.NotContains(t, yaml, "flows:", "kapi check runs with no flow declared")

	var example []string
	for line := range strings.SplitSeq(yaml, "\n") {
		body, ok := strings.CutPrefix(line, "#   ")
		if ok {
			body, _, _ = strings.Cut(body, "   #")
			example = append(example, "  "+body)
		}
	}
	require.NotEmpty(t, example)

	dir := t.TempDir()
	recipePath := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipePath,
		[]byte("version: v1\nname: MyApp\ndefaults:\n  source_language: en\n  target_languages: [fr]\ncollections:\n"+strings.Join(example, "\n")+"\n"), 0o644))
	proj, err := project.Load(recipePath)
	require.NoError(t, err, "the scaffold's own example must load:\n%s", strings.Join(example, "\n"))
	require.Len(t, proj.Collections, 1)
	assert.Equal(t, "docs/**/*.md", proj.Collections[0].Path)
}
