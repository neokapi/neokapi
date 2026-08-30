package backend

import (
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// axisOf returns the row for one axis, failing the test when it is absent.
func axisOf(t *testing.T, res *RecipeGovernanceDTO, axis string) RecipeAxisDTO {
	t.Helper()
	for _, a := range res.Axes {
		if a.Axis == axis {
			return a
		}
	}
	require.Failf(t, "axis missing", "no axis %q", axis)
	return RecipeAxisDTO{}
}

func TestRecipeGovernanceRefusesTheStructuralAxes(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.RecipeGovernance(tab.ID)
	require.NoError(t, err)

	for _, axis := range []string{project.ProductAxis, project.ChannelAxis} {
		row := axisOf(t, res, axis)
		assert.False(t, row.Declarable, "%s is derived from a collection's channel", axis)
		assert.Contains(t, row.Refusal, "derived from a collection's channel")
		// The refusal is the recipe's own, so an editor cannot word it
		// differently from what `kapi apply` says.
		assert.Equal(t, project.DeclarableAxis(axis).Error(), row.Refusal)
	}
}

func TestRecipeGovernanceOffersTheDeclarableAxes(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.RecipeGovernance(tab.ID)
	require.NoError(t, err)

	brand := axisOf(t, res, project.BrandAxis)
	assert.True(t, brand.Declarable)
	assert.Empty(t, brand.Refusal)
	assert.Equal(t, "northsea", brand.Used, "the axis the project already declares")

	mode := axisOf(t, res, project.ModeAxis)
	assert.True(t, mode.Declarable)
	assert.Contains(t, mode.Values, project.ModeReference)
}

func TestRecipeGovernanceListsTheDeclaredChannels(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.RecipeGovernance(tab.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"campaign/promo", "support/docs"}, res.Channels)
	assert.Equal(t, []string{"campaign", "support"}, res.Profiles)
}

func TestRecipeGovernanceOffersTheProfileFilesOnDisk(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	res, err := app.RecipeGovernance(tab.ID)
	require.NoError(t, err)
	assert.Contains(t, res.VoiceFiles, ".kapi/voice.yaml")
	assert.Contains(t, res.VoiceFiles, ".kapi/profiles/support/voice.yaml")
	assert.NotEmpty(t, res.Packs, "a binding can name a starter pack instead of a file")
}

func TestRecipeGovernanceRejectsAnUnknownTab(t *testing.T) {
	app := NewApp()

	_, err := app.RecipeGovernance("nope")
	assert.Error(t, err)
}

func TestDeclarableAxisIsTheOneRefusal(t *testing.T) {
	assert.NoError(t, project.DeclarableAxis(project.BrandAxis))
	assert.NoError(t, project.DeclarableAxis(project.ModeAxis))
	assert.Error(t, project.DeclarableAxis(""))
	assert.Error(t, project.DeclarableAxis(project.ProductAxis))
	assert.Error(t, project.DeclarableAxis(project.ChannelAxis))
}
