package sample

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The sample opens as a governed project: the recipe declares who governs what,
// and every artifact it binds is committed beside it. A scaffold that lost one
// of these still opens, and every governance surface in the app reads empty.
func TestScaffoldShipsCommittedContext(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Scaffold("kapimart", dir))

	for _, rel := range []string{
		ContextVoiceRel,
		ContextTermsRel,
		ContextMemoryRel,
		filepath.Join(project.StateDirName, project.StateGitignoreFilename),
	} {
		info, err := os.Stat(filepath.Join(dir, rel))
		require.NoError(t, err, "scaffold must write %s", rel)
		assert.Positive(t, info.Size(), "%s must not be empty", rel)
	}
}

// The recipe binds the voice by name, and the name is the one the scaffold's
// import stores the committed profile under. Nothing else checks that the two
// agree: a renamed profile leaves the recipe naming a voice the store does not
// hold, and the project still opens, ungoverned.
func TestRecipeBindsTheImportedVoice(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Scaffold("kapimart", dir))

	proj, err := project.Load(filepath.Join(dir, "kapi.yaml"))
	require.NoError(t, err)
	require.NotNil(t, proj.Defaults.Voice, "recipe must bind a voice profile")
	require.NotEmpty(t, proj.Defaults.Voice.Profile, "recipe binds the voice by name")

	app := &host.App{}
	app.InitRegistries()
	defer app.Shutdown()
	store, release, err := app.ProjectVoiceStore(t.Context(), dir)
	require.NoError(t, err)
	defer release()
	p, _, found, err := app.ResolveVoiceProfile(t.Context(), proj, dir, host.VoiceResolveOptions{Store: store})
	require.NoError(t, err, "the scaffold's store must hold the voice the recipe names")
	require.True(t, found)
	assert.Equal(t, "KapiMart", p.Name)
}

// A project sitting at exactly one point teaches nothing about coordinates, and
// the point map it produces is a single row. The sample declares a profile with
// several channels so the map has something to show and the resolver has
// something to resolve.
func TestSampleResolvesSeveralGovernedPoints(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Scaffold("kapimart", dir))

	proj, err := project.Load(filepath.Join(dir, "kapi.yaml"))
	require.NoError(t, err)
	require.NoError(t, proj.Validate())

	require.NotEmpty(t, proj.Profiles, "the sample must declare profiles")
	assert.NotEmpty(t, proj.Defaults.Coordinates, "the sample must declare a coordinate axis")

	// A declared axis only: the structural axes are derived from a collection's
	// channel, and the setter refuses them here.
	for axis := range proj.Defaults.Coordinates {
		require.NoError(t, project.DeclarableAxis(axis),
			"defaults.coordinates must not name a derived axis")
	}

	channels := map[string]bool{}
	for _, c := range proj.Collections {
		rc, rerr := proj.ResolveGovernance(c.Name)
		require.NoError(t, rerr, "collection %q must resolve", c.Name)
		require.NotNil(t, rc.Voice, "collection %q must be governed by a voice profile", c.Name)
		assert.NotEmpty(t, rc.Profile, "collection %q must sit in a profile", c.Name)
		channels[rc.Channel] = true
	}
	assert.GreaterOrEqual(t, len(channels), 2,
		"collections must sit at more than one channel, or the point map is one row")
}

// A per-item channel is how one file inside a collection sits somewhere else.
// It resolves through a different ladder rung than the collection does, so the
// two are asserted separately.
func TestSamplePlacesTheReferenceSurfaceApart(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Scaffold("kapimart", dir))

	proj, err := project.Load(filepath.Join(dir, "kapi.yaml"))
	require.NoError(t, err)

	ref, err := proj.ResolveGovernanceForPath("web/en/api-reference.md")
	require.NoError(t, err)
	guide, err := proj.ResolveGovernanceForPath("web/en/getting-started.md")
	require.NoError(t, err)

	assert.NotEqual(t, guide.Channel, ref.Channel,
		"the API reference and a walkthrough must not share a channel")
}

// The voice profile governs every check the app runs, so it has to survive the
// strict loader `kapi voice validate` uses, not merely the lenient one.
func TestCommittedVoiceProfileValidates(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Scaffold("kapimart", dir))

	f, err := os.Open(filepath.Join(dir, ContextDirName, "voice.yaml"))
	require.NoError(t, err)
	defer f.Close()

	p, err := profile.DecodeProfileStrict(f)
	require.NoError(t, err, "the committed voice profile must decode strictly")
	assert.Empty(t, profile.ValidateProfile(p), "the committed voice profile must validate")

	words := p.CarriedTerms().Rules
	assert.True(t, profile.HasWordRules(p), "the voice file must carry term rules to check against")
	assert.NotEmpty(t, p.Channels, "the profile must bend per channel, or the channels teach nothing")

	// A rule tied to a concept is what lets a finding lead back to the
	// definition it came from.
	var linked int
	for _, r := range words {
		if r.Term != "" && r.ConceptID != "" {
			linked++
		}
	}
	assert.Positive(t, linked, "at least one term rule must name the concept it came from")
}
