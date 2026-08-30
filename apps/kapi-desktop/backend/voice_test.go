package backend

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pointOf returns the row for one point, failing the test when it is absent.
func pointOf(t *testing.T, res *ProjectVoiceResult, label string) VoicePointDTO {
	t.Helper()
	for _, p := range res.Points {
		if p.Label == label {
			return p
		}
	}
	require.Failf(t, "point missing", "no point labelled %q", label)
	return VoicePointDTO{}
}

func TestProjectVoiceListsEveryDeclaredPoint(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)
	require.NotEmpty(t, res.At, "the resolution instant is reported, not implied")

	labels := make([]string, 0, len(res.Points))
	for _, p := range res.Points {
		labels = append(labels, p.Label)
	}
	assert.Equal(t, []string{"project default", "campaign", "support"}, labels,
		"the project's own point leads, then each declared profile in name order")
}

func TestProjectVoiceResolvesTheProfileAtItsPoint(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)

	def := pointOf(t, res, "project default")
	require.NotNil(t, def.Profile)
	assert.Equal(t, "Northsea", def.Profile.Name)
	assert.True(t, def.Point.Default)
	assert.Equal(t, project.DefaultVoiceField, def.Field)
	// Promo's own profile expired, so its collection resolves here too.
	assert.Equal(t, []string{"App", "Promo"}, def.Collections)

	// A profile that binds no voice of its own is answered by its conventional
	// file, which is a different profile from the project's.
	support := pointOf(t, res, "support")
	require.NotNil(t, support.Profile)
	assert.Equal(t, "Northsea Support", support.Profile.Name)
	assert.Equal(t, []string{"docs"}, support.Channels)
	assert.Equal(t, []string{"Docs"}, support.Collections)
	assert.False(t, support.Point.Default)
}

func TestProjectVoiceCarriesTheWholeProfile(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)
	def := pointOf(t, res, "project default")
	require.NotNil(t, def.Profile)

	assert.Equal(t, []string{"clear", "calm"}, def.Profile.Tone.Personality)
	assert.Equal(t, "neutral", def.Profile.Tone.Formality)
	assert.True(t, def.Profile.Style.ActiveVoice)
	require.Len(t, def.Profile.Style.ProhibitedPatterns, 1)
	assert.Equal(t, "minor", def.Profile.Style.ProhibitedPatterns[0].Severity)

	// A rule's severity decides whether a violation fails or only reports, so it
	// travels with the rule rather than being inferred at the surface.
	require.Len(t, def.Profile.Vocabulary.PreferredTerms, 1)
	assert.Equal(t, "log in", def.Profile.Vocabulary.PreferredTerms[0].Term)
	assert.Equal(t, "sign in", def.Profile.Vocabulary.PreferredTerms[0].Replacement)
	assert.Equal(t, "major", def.Profile.Vocabulary.PreferredTerms[0].Severity)
	require.Len(t, def.Profile.Vocabulary.ForbiddenTerms, 1)
	assert.Equal(t, "critical", def.Profile.Vocabulary.ForbiddenTerms[0].Severity)

	require.Len(t, def.Profile.Examples, 1)
	assert.Equal(t, "Use the portal.", def.Profile.Examples[0].After)

	// The overrides arrive unapplied, so each audience reads as itself.
	require.Contains(t, def.Profile.Locales, model.LocaleID("nb-NO"))
	assert.Equal(t, "informal", def.Profile.Locales[model.LocaleID("nb-NO")].Formality)
	require.Contains(t, def.Profile.Channels, "docs")
	require.Contains(t, def.Profile.Personas, "support-agent")

	assert.Contains(t, def.Guide, "Northsea", "the rendered guide travels with the profile")
}

func TestProjectVoiceNamesTheBindingAndItsSource(t *testing.T) {
	app := NewApp()
	tab, root := newContextProject(t, app)

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)

	def := pointOf(t, res, "project default")
	require.NotNil(t, def.Binding)
	assert.Equal(t, "profile_file", def.Binding.Kind)
	assert.Equal(t, project.RelStatePath("voice.yaml"), def.Binding.Value)
	assert.Contains(t, def.Source, root, "the source names the file the profile came from")
}

func TestProjectVoiceReportsAClosedWindowAndWhatGovernsInstead(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)

	campaign := pointOf(t, res, "campaign")
	require.NotNil(t, campaign.Validity, "the declared window is reported at its own point")
	assert.Equal(t, "expired", campaign.Validity.State)
	assert.NotEmpty(t, campaign.Validity.To)

	require.NotNil(t, campaign.Fallback, "a skipped binding is named, not quietly replaced")
	assert.Equal(t, "campaign", campaign.Fallback.Profile)
	assert.True(t, campaign.Fallback.Expired)
	assert.Empty(t, campaign.Fallback.Governing, "the project default governs in its place")
	assert.Contains(t, campaign.Fallback.Message, "expired")

	// Falling through means the project's voice governs there, not the
	// campaign's own file.
	require.NotNil(t, campaign.Profile)
	assert.Equal(t, "Northsea", campaign.Profile.Name)
	assert.Equal(t, project.DefaultVoiceField, campaign.Field)

	// A collection whose profile expired resolves to the default point rather
	// than being left ungoverned.
	assert.Contains(t, pointOf(t, res, "project default").Collections, "Promo")
	assert.Empty(t, campaign.Collections)
}

func TestProjectVoiceCarriesTheDeclaredCoordinates(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.ProjectVoice(tab.ID)
	require.NoError(t, err)

	def := pointOf(t, res, "project default")
	assert.Equal(t, map[string]string{project.BrandAxis: "northsea"}, def.Coordinates,
		"a declared axis is inherited by every point")

	support := pointOf(t, res, "support")
	assert.Equal(t, map[string]string{
		project.BrandAxis:   "northsea",
		project.ProductAxis: "support",
	}, support.Coordinates, "the structural axis is derived, the declared one inherited")
}

func TestProjectVoiceRejectsAnUnknownTab(t *testing.T) {
	app := NewApp()

	_, err := app.ProjectVoice("nope")
	assert.Error(t, err)
}
