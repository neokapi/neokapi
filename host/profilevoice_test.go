package host

import (
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A profile's own voice is selected by name, out of the store: the recipe's
// `profiles.<name>.voice`, or for a profile that binds none, the stored profile
// named after it. The path an import read the file from is bookkeeping kept on
// the importing machine, and a checkout that received the store any other way
// never sees it, so it must never be what selects a voice.

// profileVoiceProject writes a recipe with a project voice and one profile,
// promo, that binds no voice of its own, plus the two voice files an import
// reads. promoVoice is the body of `.kapi/profiles/promo/voice.yaml`.
func profileVoiceProject(t *testing.T, promoVoice string) (a *App, root, recipe string) {
	t.Helper()
	a, root, recipe = newSeedProject(t)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Profiles = map[string]project.Profile{
		"promo": {Channels: []project.Channel{{ID: "web"}}},
	}
	proj.Collections = []project.Collection{{
		Name:    "web",
		Channel: "promo/web",
		Content: []project.ContentItem{{Path: "src/*.json"}},
	}}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.WriteFile(layoutVoicePath(t, root), []byte("id: house\nname: House Style\n"), 0o644))
	require.NoError(t, os.WriteFile(layoutVoicePath(t, root, "promo"), []byte(promoVoice), 0o644))
	return a, root, recipe
}

// forgetImportTies drops the path-to-id bookkeeping an import keeps, leaving the
// store as a checkout that pulled it, or read it from a transfer file, holds it.
func forgetImportTies(t *testing.T, a *App, root string) {
	t.Helper()
	db, err := a.ProjectDB(t.Context(), root)
	require.NoError(t, err)
	require.NoError(t, db.PutContextMeta(t.Context(), MetaVoiceBindings, "{}"))
}

// voiceAtWeb resolves the voice governing the web collection.
func voiceAtWeb(t *testing.T, a *App, root, recipe string) string {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	store, release, err := a.ProjectVoiceStore(t.Context(), root)
	require.NoError(t, err)
	defer release()
	p, _, _, found, err := a.LoadCollectionVoice(t.Context(), proj, root, VoiceResolveOptions{
		Point: project.GovernancePoint{Collection: "web"},
		Store: store,
	})
	require.NoError(t, err)
	require.True(t, found)
	return p.Name
}

func TestImport_BindsAProfileVoiceItsNameWouldNotSelect(t *testing.T) {
	// No `id:`, so the profile is stored under a slug of its name, promo-voice,
	// which the profile name promo does not select.
	a, root, recipe := profileVoiceProject(t, "name: Promo Voice\n")

	res, err := a.ImportProjectContext(t.Context(), recipe, ContextImportRequest{})
	require.NoError(t, err)
	require.Len(t, res.ProfileVoices, 1)
	assert.Equal(t, "promo", res.ProfileVoices[0].Profile)
	assert.Equal(t, "promo-voice", res.ProfileVoices[0].Bound.ID)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	require.NotNil(t, proj.Profiles["promo"].Voice, "the binding is written where it travels: the recipe")
	assert.Equal(t, "promo-voice", proj.Profiles["promo"].Voice.Profile)

	forgetImportTies(t, a, root)
	assert.Equal(t, "Promo Voice", voiceAtWeb(t, a, root, recipe),
		"a checkout without the import's bookkeeping resolves the same voice")

	var out strings.Builder
	require.NoError(t, res.FormatText(&out))
	assert.Contains(t, out.String(), "Bound voice Promo Voice for profile promo in kapi.yaml (profiles.promo.voice.profile: promo-voice).")

	// A second import finds the binding in place and writes nothing.
	again, err := a.ImportProjectContext(t.Context(), recipe, ContextImportRequest{})
	require.NoError(t, err)
	assert.Empty(t, again.ProfileVoices)
}

func TestImport_LeavesAProfileVoiceItsNameSelects(t *testing.T) {
	a, root, recipe := profileVoiceProject(t, "id: promo\nname: Promo Voice\n")
	res, err := a.ImportProjectContext(t.Context(), recipe, ContextImportRequest{})
	require.NoError(t, err)
	assert.Empty(t, res.ProfileVoices, "the stored id is the profile's name, so the name selects it")

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	assert.Nil(t, proj.Profiles["promo"].Voice, "the recipe is left as it was")

	forgetImportTies(t, a, root)
	assert.Equal(t, "Promo Voice", voiceAtWeb(t, a, root, recipe))
}

func TestResolve_TheImportPathSelectsNoVoice(t *testing.T) {
	a, root, recipe := profileVoiceProject(t, "name: Promo Voice\n")
	_, err := a.ImportProjectContext(t.Context(), recipe, ContextImportRequest{})
	require.NoError(t, err)

	// Take the binding back out of the recipe, keeping the import's record of
	// which file promo-voice was read from.
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	promo := proj.Profiles["promo"]
	promo.Voice = nil
	proj.Profiles["promo"] = promo
	require.NoError(t, project.Save(recipe, proj))

	assert.Equal(t, "House Style", voiceAtWeb(t, a, root, recipe),
		"with no binding and no stored profile named promo, defaults.voice governs")

	target, err := a.VoiceProfileTargetAt(t.Context(), root, project.GovernancePoint{Profile: "promo"})
	require.NoError(t, err)
	assert.Equal(t, "promo", target.ID, "a save opens a profile named after the point")
	assert.True(t, target.Inherited)
	assert.False(t, target.Exists)
}
