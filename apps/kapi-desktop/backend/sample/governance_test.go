package sample

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
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
		filepath.Join(project.StateDirName, "voice.yaml"),
		TermsSourceRel,
		MemorySourceRel,
		filepath.Join(project.StateDirName, project.StateGitignoreFilename),
	} {
		info, err := os.Stat(filepath.Join(dir, rel))
		require.NoError(t, err, "scaffold must write %s", rel)
		assert.Positive(t, info.Size(), "%s must not be empty", rel)
	}
}

// The recipe's bindings and the files on disk are two statements of the same
// fact, and nothing else checks that they agree: a renamed source leaves the
// recipe pointing at nothing, the store compiles from an empty set, and the
// project still opens.
func TestRecipeBindsTheCommittedContext(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Scaffold("kapimart", dir))

	proj, err := project.Load(filepath.Join(dir, "kapi.yaml"))
	require.NoError(t, err)

	require.NotEmpty(t, proj.Defaults.TermsSource, "recipe must bind a terms source")
	require.NotEmpty(t, proj.Defaults.MemorySource, "recipe must bind a content-memory source")

	for _, bound := range []string{proj.Defaults.TermsSource, proj.Defaults.MemorySource} {
		_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(bound)))
		assert.NoError(t, err, "recipe binds %q, which must exist on disk", bound)
	}

	require.NotNil(t, proj.Defaults.Voice, "recipe must bind a voice profile")
	assert.NotEmpty(t, proj.Defaults.Voice.ProfileFile)
	_, err = os.Stat(filepath.Join(dir, filepath.FromSlash(proj.Defaults.Voice.ProfileFile)))
	assert.NoError(t, err, "the bound voice profile must exist on disk")
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
		assert.NoError(t, project.DeclarableAxis(axis),
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

	f, err := os.Open(filepath.Join(dir, project.StateDirName, "voice.yaml"))
	require.NoError(t, err)
	defer f.Close()

	p, err := profile.DecodeProfileStrict(f)
	require.NoError(t, err, "the committed voice profile must decode strictly")
	assert.Empty(t, profile.ValidateProfile(p), "the committed voice profile must validate")

	assert.NotEmpty(t, p.Vocabulary.ForbiddenTerms, "the profile must carry term rules to check against")
	assert.NotEmpty(t, p.Channels, "the profile must bend per channel, or the channels teach nothing")

	// A rule tied to a concept is what lets a finding lead back to the
	// definition it came from.
	var linked int
	for _, r := range p.Vocabulary.ForbiddenTerms {
		if r.ConceptID != "" {
			linked++
		}
	}
	assert.Positive(t, linked, "at least one term rule must name the concept it came from")
}
