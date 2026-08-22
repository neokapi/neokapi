package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// namedVoiceProject writes a project whose default voice is bound by NAME, so
// resolving it has to reach a store rather than a file beside the recipe.
func namedVoiceProject(t *testing.T, profileName string) (recipe, root string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs", "guides"), 0o755))

	proj := &project.KapiProject{
		Version: "v1",
		Name:    "Named Voice",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
			Voice:           &project.VoiceBinding{Profile: profileName},
		},
		Collections: []project.Collection{
			{Name: "docs", Content: []project.ContentItem{{Path: "docs/**/*.md"}}},
		},
	}
	recipe = filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	return recipe, dir
}

// TestVoiceStoreIsTheProjectsWhereverTheCommandWasTyped is the regression the
// pool binding exists for. A `voice: profile: <name>` binding used to be looked
// up in ./voice.db resolved against the WORKING DIRECTORY, so the same recipe
// resolved a profile when the command ran at the project root and reported it
// missing when the command ran one directory down — while a push resolved the
// same binding against the root either way, so the two halves of a round trip
// disagreed about what the recipe binds.
func TestVoiceStoreIsTheProjectsWhereverTheCommandWasTyped(t *testing.T) {
	recipe, root := namedVoiceProject(t, "house-style")
	a := &App{}

	// The profile exists in the project's own store — the shared pool, beside
	// the content memory, the terms store and the block cache.
	db, err := a.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	require.NotNil(t, db.Voice(), "the project pool carries a voice store")
	require.NoError(t, db.Voice().CreateProfile(t.Context(), &profile.VoiceProfile{
		ID: "house-style", Name: "House Style", Scope: LocalScope,
	}))

	// Run from a subdirectory, which is where the old resolution went wrong.
	t.Chdir(filepath.Join(root, "docs", "guides"))

	cmd := bindingsCmd(t, recipe)
	sel, err := a.ResolveVoiceStore(cmd)
	require.NoError(t, err)
	assert.Equal(t, root, sel.Root, "the project's store governs")
	assert.Empty(t, sel.Path, "and no file is resolved against the working directory")

	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	require.NoError(t, err)
	b, err := a.resolveProjectBindings(cmd, proj, recipe, project.GovernancePoint{Collection: "docs"})
	require.NoError(t, err)
	require.NotNil(t, b)
	require.NotNil(t, b.profile, "the name-bound profile resolves from a subdirectory")
	assert.Equal(t, "House Style", b.profile.Name)
}

// TestVoiceStoreFlagsStillSelectAFile keeps the standalone escape hatch: an
// explicit --file names a store outside the project, and the project's own is
// not consulted.
func TestVoiceStoreFlagsStillSelectAFile(t *testing.T) {
	recipe, root := namedVoiceProject(t, "house-style")
	a := &App{}

	elsewhere := filepath.Join(t.TempDir(), "elsewhere.db")
	cmd := bindingsCmd(t, recipe)
	AddResourceFlags(cmd)
	require.NoError(t, cmd.Flags().Set("file", elsewhere))

	sel, err := a.ResolveVoiceStore(cmd)
	require.NoError(t, err)
	assert.Equal(t, elsewhere, sel.Path, "the named file wins over the project")
	assert.Empty(t, sel.Root)
	assert.True(t, sel.Explicit, "and records that the user asked for it")
	assert.NotEqual(t, root, sel.Path)
}

// TestVoiceLookupCreatesNoStore holds the line the old path held: resolving a
// binding must not bring a voice.db into being in whatever directory the
// command was typed. Outside a project there is nowhere to look, and that is an
// answer rather than a failure.
func TestVoiceLookupCreatesNoStore(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Chdir(dir)

	a := &App{}
	cmd := NewEnvCommand(t.Context(), "voice-store-test")
	AddProjectFlag(cmd)
	AddResourceFlags(cmd)
	require.NoError(t, cmd.Flags().Set("local", "true"))

	store, release, err := a.VoiceLookupStore(cmd)
	require.NoError(t, err)
	defer release()
	assert.Nil(t, store, "no store where no file exists")

	_, statErr := os.Stat(filepath.Join(dir, "voice.db"))
	assert.True(t, os.IsNotExist(statErr), "and none was created as a side effect")
}
