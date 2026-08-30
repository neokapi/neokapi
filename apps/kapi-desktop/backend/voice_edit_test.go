package backend

import (
	"os"
	"path/filepath"
	"testing"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVoiceEditTargetNamesTheBoundFile(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)

	def := pointOf(t, res, "project default")
	assert.True(t, def.Edit.Writable)
	assert.Equal(t, ".kapi/voice.yaml", def.Edit.Target)
	assert.True(t, def.Edit.Exists)
	assert.False(t, def.Edit.Inherited)
}

func TestVoiceEditTargetNamesAProfilesConventionalFile(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)

	support := pointOf(t, res, "support")
	assert.True(t, support.Edit.Writable)
	assert.Equal(t, ".kapi/profiles/support/voice.yaml", support.Edit.Target)
	assert.True(t, support.Edit.Exists, "the profile keeps its voice at the conventional path")
}

func TestVoiceEditRefusesAPackBinding(t *testing.T) {
	app := NewApp()
	tab, root := newContextProject(t, app)

	op := app.getOpenProject(tab.ID)
	op.Project.Defaults.Voice = &project.VoiceBinding{Pack: "technical-docs"}
	require.NoError(t, project.Save(filepath.Join(root, "kapi.yaml"), op.Project))

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)
	def := pointOf(t, res, "project default")
	assert.False(t, def.Edit.Writable)
	assert.Contains(t, def.Edit.Reason, "starter pack")

	_, serr := app.SaveVoiceProfile(tab.ID, "", coreprofile.VoiceProfile{Name: "Nope"})
	assert.Error(t, serr, "a pack is not a file this surface can write over")
}

func TestSaveVoiceProfileWritesTheBoundFile(t *testing.T) {
	app := NewApp()
	tab, root := newContextProject(t, app)

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)
	profile := *pointOf(t, res, "project default").Profile
	profile.Tone.Guidelines = "Lead with what changed."
	profile.Vocabulary.PreferredTerms = append(profile.Vocabulary.PreferredTerms,
		coreprofile.TermRule{Term: "utilise", Replacement: "use", Severity: "minor"})

	saved, err := app.SaveVoiceProfile(tab.ID, "", profile)
	require.NoError(t, err)
	assert.True(t, saved.Saved)
	assert.True(t, saved.Changed)
	assert.Empty(t, saved.Problems)
	assert.Contains(t, saved.Guide, "Lead with what changed.")

	// The loop reads the change back through the same ladder that resolved it.
	again, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)
	reread := pointOf(t, again, "project default")
	require.NotNil(t, reread.Profile)
	assert.Equal(t, "Lead with what changed.", reread.Profile.Tone.Guidelines)
	require.Len(t, reread.Profile.Vocabulary.PreferredTerms, 2)
	assert.Equal(t, "utilise", reread.Profile.Vocabulary.PreferredTerms[1].Term)

	body, rerr := os.ReadFile(filepath.Join(root, ".kapi", "voice.yaml"))
	require.NoError(t, rerr)
	assert.Contains(t, string(body), "Lead with what changed.")
}

func TestSaveVoiceProfileIsByteStable(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)
	profile := *pointOf(t, res, "project default").Profile

	saved, err := app.SaveVoiceProfile(tab.ID, "", profile)
	require.NoError(t, err)
	assert.True(t, saved.Saved)
	assert.False(t, saved.Changed, "a save that says what the file says does not touch it")
}

func TestSaveVoiceProfileRefusesABlockingProblem(t *testing.T) {
	app := NewApp()
	tab, root := newContextProject(t, app)

	before, rerr := os.ReadFile(filepath.Join(root, ".kapi", "voice.yaml"))
	require.NoError(t, rerr)

	// person_pov is read by the offline check, so an unrecognised value is a
	// rule that silently does nothing.
	bad := coreprofile.VoiceProfile{Name: "Northsea"}
	bad.Style.PersonPOV = "fourth"
	saved, err := app.SaveVoiceProfile(tab.ID, "", bad)
	require.NoError(t, err)
	assert.False(t, saved.Saved)
	require.NotEmpty(t, saved.Problems)
	assert.Equal(t, "style.person_pov", saved.Problems[0].Field)

	after, rerr := os.ReadFile(filepath.Join(root, ".kapi", "voice.yaml"))
	require.NoError(t, rerr)
	assert.Equal(t, before, after, "a refused save leaves the file alone")
}

func TestSaveVoiceProfileKeepsAToneOutsideTheUsualValues(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)
	profile := *pointOf(t, res, "project default").Profile
	profile.Tone.Formality = "calm and matter-of-fact"

	saved, err := app.SaveVoiceProfile(tab.ID, "", profile)
	require.NoError(t, err)
	assert.True(t, saved.Saved, "tone is described, not enumerated")
	require.NotEmpty(t, saved.Problems)
	assert.True(t, saved.Problems[0].Warning, "an unusual register is noted, not refused")
	assert.Contains(t, saved.Guide, "calm and matter-of-fact")
}

func TestSaveVoiceProfileCreatesAProfilesOwnVoice(t *testing.T) {
	app := NewApp()
	tab, root := newContextProject(t, app)

	target := filepath.Join(root, ".kapi", "profiles", "campaign", "voice.yaml")
	require.NoError(t, os.Remove(target))

	saved, err := app.SaveVoiceProfile(tab.ID, "campaign", coreprofile.VoiceProfile{
		Name: "Northsea Campaign",
		Tone: coreprofile.ToneProfile{Personality: []string{"energetic"}},
	})
	require.NoError(t, err)
	assert.True(t, saved.Saved)
	assert.Equal(t, ".kapi/profiles/campaign/voice.yaml", saved.Target)
	assert.FileExists(t, target)
}

func TestValidateVoiceProfileMatchesTheCommand(t *testing.T) {
	app := NewApp()

	probs, err := app.ValidateVoiceProfile(coreprofile.VoiceProfile{})
	require.NoError(t, err)
	require.NotEmpty(t, probs)
	assert.Equal(t, "name", probs[0].Field, "only name is required")

	sound, err := app.ValidateVoiceProfile(coreprofile.VoiceProfile{Name: "Northsea"})
	require.NoError(t, err)
	assert.Empty(t, sound)

	bad := coreprofile.VoiceProfile{Name: "Northsea"}
	bad.Style.ProhibitedPatterns = []coreprofile.Pattern{{Regex: "("}}
	probs, err = app.ValidateVoiceProfile(bad)
	require.NoError(t, err)
	require.NotEmpty(t, probs)
	assert.Contains(t, probs[0].Message, "invalid regex")
}

func TestVoiceFieldValuesMatchWhatValidationApplies(t *testing.T) {
	app := NewApp()
	values := app.VoiceFieldValues()

	assert.True(t, values["tone.formality"].Open, "a register outside the list is kept")
	assert.False(t, values["style.person_pov"].Open, "style enums are read by code")
	assert.Contains(t, values["style.person_pov"].Values, "second")
	assert.Contains(t, values["severity"].Values, "critical")

	// Every closed set the editor offers must actually validate.
	for _, pov := range values["style.person_pov"].Values {
		p := coreprofile.VoiceProfile{Name: "Northsea"}
		p.Style.PersonPOV = pov
		probs, err := app.ValidateVoiceProfile(p)
		require.NoError(t, err)
		assert.Empty(t, probs, "the editor offers %q, so validation must accept it", pov)
	}
}

func TestVoiceStarterPacksLoad(t *testing.T) {
	app := NewApp()

	names, err := app.VoiceStarterPacks()
	require.NoError(t, err)
	require.NotEmpty(t, names)

	pack, err := app.VoiceStarterPack(names[0])
	require.NoError(t, err)
	require.NotNil(t, pack)
	assert.NotEmpty(t, pack.Name)

	probs, err := app.ValidateVoiceProfile(*pack)
	require.NoError(t, err)
	assert.Empty(t, coreprofile.Blocking(probs), "a starter pack is a valid starting point")
}
