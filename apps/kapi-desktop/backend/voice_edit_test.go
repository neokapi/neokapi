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

func TestVoiceEditTargetNamesTheStoredProfile(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)

	def := pointOf(t, res, "project default")
	assert.True(t, def.Edit.Writable)
	assert.Equal(t, "northsea", def.Edit.Profile)
	assert.True(t, def.Edit.Exists)
	assert.False(t, def.Edit.Inherited)
}

func TestVoiceEditTargetNamesAProfilesOwnStoredProfile(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)

	support := pointOf(t, res, "support")
	assert.True(t, support.Edit.Writable)
	assert.Equal(t, "northsea-support", support.Edit.Profile)
	assert.True(t, support.Edit.Exists, "the profile has a voice of its own in the store")
	assert.False(t, support.Edit.Inherited)
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
	assert.Error(t, serr, "a starter pack is read-only")
}

func TestSaveVoiceProfileWritesTheStore(t *testing.T) {
	app := NewApp()
	tab, root := newContextProject(t, app)

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)
	profile := *pointOf(t, res, "project default").Profile
	profile.Tone.Guidelines = "Lead with what changed."
	profile.Vocabulary.PreferredTerms = append(profile.Vocabulary.PreferredTerms,
		coreprofile.TermRule{Term: "utilise", Replacement: "use", Advisory: true})

	saved, err := app.SaveVoiceProfile(tab.ID, "", profile)
	require.NoError(t, err)
	assert.True(t, saved.Saved)
	assert.True(t, saved.Changed)
	assert.Equal(t, "northsea", saved.Profile)
	assert.NotEmpty(t, saved.Recorded, "the save is on the project's context record")
	assert.Empty(t, saved.Problems)
	assert.Contains(t, saved.Guide, "Lead with what changed.")

	// The store is what a resolution reads, so the next read answers with what
	// was saved and nothing has to be read in first.
	again, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)
	reread := pointOf(t, again, "project default")
	require.NotNil(t, reread.Profile)
	assert.Equal(t, "Lead with what changed.", reread.Profile.Tone.Guidelines)
	require.Len(t, reread.Profile.Vocabulary.PreferredTerms, 2)
	assert.Equal(t, "utilise", reread.Profile.Vocabulary.PreferredTerms[1].Term)

	// The file the checkout carries is the import source and stays as it was.
	body, rerr := os.ReadFile(filepath.Join(root, ".kapi", "voice.yaml"))
	require.NoError(t, rerr)
	assert.NotContains(t, string(body), "Lead with what changed.")
}

// A save the store already holds leaves the profile where it is, so a version
// is kept for each edit a person actually made.
func TestSaveVoiceProfileIsIdempotent(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)
	profile := *pointOf(t, res, "project default").Profile
	profile.Tone.Guidelines = "Lead with what changed."

	saved, err := app.SaveVoiceProfile(tab.ID, "", profile)
	require.NoError(t, err)
	require.True(t, saved.Saved)
	require.True(t, saved.Changed)

	again, err := app.SaveVoiceProfile(tab.ID, "", profile)
	require.NoError(t, err)
	assert.True(t, again.Saved)
	assert.False(t, again.Changed, "a save that says what the store says does not move it")
	assert.Empty(t, again.Recorded, "and leaves nothing on the record")
}

func TestSaveVoiceProfileRefusesABlockingProblem(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)
	before := *pointOf(t, res, "project default").Profile

	// person_pov is read by the offline check, so an unrecognised value is a
	// rule that silently does nothing.
	bad := coreprofile.VoiceProfile{Name: "Northsea"}
	bad.Style.PersonPOV = "fourth"
	saved, err := app.SaveVoiceProfile(tab.ID, "", bad)
	require.NoError(t, err)
	assert.False(t, saved.Saved)
	require.NotEmpty(t, saved.Problems)
	assert.Equal(t, "style.person_pov", saved.Problems[0].Field)

	after, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)
	assert.Equal(t, before.Tone, pointOf(t, after, "project default").Profile.Tone,
		"a refused save leaves the stored profile alone")
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

// A point reading the voice bound coarser gets one of its own when someone
// saves there, and the recipe says which profile governs it from then on.
func TestSaveVoiceProfileGivesAPointItsOwnVoice(t *testing.T) {
	app := NewApp()
	tab, root := newContextProject(t, app)

	op := app.getOpenProject(tab.ID)
	op.Project.Profiles["field"] = project.Profile{Channels: []project.Channel{{ID: "notes"}}}
	require.NoError(t, project.Save(filepath.Join(root, "kapi.yaml"), op.Project))

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)
	field := pointOf(t, res, "field")
	assert.True(t, field.Edit.Writable)
	assert.True(t, field.Edit.Inherited, "the point reads the voice bound coarser")
	assert.False(t, field.Edit.Exists)

	saved, err := app.SaveVoiceProfile(tab.ID, "field", coreprofile.VoiceProfile{
		Name: "Northsea Field",
		Tone: coreprofile.ToneProfile{Personality: []string{"energetic"}},
	})
	require.NoError(t, err)
	assert.True(t, saved.Saved)
	assert.Equal(t, "field", saved.Profile)

	// The recipe says which profile governs the point from here on.
	reloaded, err := project.Load(filepath.Join(root, "kapi.yaml"))
	require.NoError(t, err)
	require.NotNil(t, reloaded.Profiles["field"].Voice)
	assert.Equal(t, "field", reloaded.Profiles["field"].Voice.Profile)

	op.Project = reloaded
	again, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)
	row := pointOf(t, again, "field")
	require.NotNil(t, row.Profile)
	assert.Equal(t, "Northsea Field", row.Profile.Name)
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
	assert.NotContains(t, values, "severity", "a rule is advisory or not; it carries no severity")

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

// Saving a voice makes the project one with a voice, so the assistant file
// says so: the same section `kapi init` writes, created as CLAUDE.md when no
// assistant file exists and replaced in place on the next save.
func TestSaveVoiceProfileWritesTheAssistantPointer(t *testing.T) {
	app := NewApp()
	tab, root := newContextProject(t, app)

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)
	profile := *pointOf(t, res, "project default").Profile
	profile.Tone.Guidelines = "Lead with what changed."

	saved, err := app.SaveVoiceProfile(tab.ID, "", profile)
	require.NoError(t, err)
	require.True(t, saved.Saved)
	require.NotNil(t, saved.Pointer)
	assert.Equal(t, "created", saved.Pointer.Action)
	assert.Equal(t, "CLAUDE.md", saved.Pointer.File)
	assert.True(t, saved.Pointer.Created)
	assert.Empty(t, saved.Pointer.Warning)

	body, rerr := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	require.NoError(t, rerr)
	assert.Contains(t, string(body), coreprofile.VoicePointerStart)
	assert.Contains(t, string(body), "voice, "+profile.Name+", is held by kapi")
	assert.Contains(t, string(body), "`kapi voice guide <path>`",
		"a recipe that declares profiles points at the per-file form")

	again, err := app.SaveVoiceProfile(tab.ID, "", profile)
	require.NoError(t, err)
	require.NotNil(t, again.Pointer)
	assert.Equal(t, "unchanged", again.Pointer.Action)
	assert.False(t, again.Pointer.Created)
}

// An editor that sends no constraints keeps the ones the stored profile
// carries; an editor that sends an empty list clears them.
func TestSaveVoiceProfilePreservesOmittedConstraints(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)
	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)
	profile := *pointOf(t, res, "project default").Profile
	profile.Constraints = []coreprofile.Constraint{{ID: "facts", Version: 1, Source: "facts.md", Statement: "Service facts", Kind: coreprofile.ConstraintGuidance}}
	saved, err := app.SaveVoiceProfile(tab.ID, "", profile)
	require.NoError(t, err)
	require.True(t, saved.Saved)

	profile.Constraints = nil
	profile.Description = "Older editor update"
	saved, err = app.SaveVoiceProfile(tab.ID, "", profile)
	require.NoError(t, err)
	require.True(t, saved.Saved)
	res, err = app.ProjectVoice(tab.ID)
	require.NoError(t, err)
	require.Len(t, pointOf(t, res, "project default").Profile.Constraints, 1)

	// An explicit empty list is the editor saying the profile has none.
	profile.Constraints = []coreprofile.Constraint{}
	saved, err = app.SaveVoiceProfile(tab.ID, "", profile)
	require.NoError(t, err)
	require.True(t, saved.Saved)
	res, err = app.ProjectVoice(tab.ID)
	require.NoError(t, err)
	assert.Empty(t, pointOf(t, res, "project default").Profile.Constraints)
}
