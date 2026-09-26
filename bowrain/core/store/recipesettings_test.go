package store

import (
	"testing"

	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecipeSettingsOfResolvesDefaults(t *testing.T) {
	assert.Equal(t, venue.ProjectSettings{
		venue.SettingConvergePolicy: ConvergePolicyOnPush,
		venue.SettingTranslateAfter: "written",
	}, RecipeSettingsOf(&Project{}), "an unset project holds the defaults")

	assert.Equal(t, venue.ProjectSettings{
		venue.SettingConvergePolicy: ConvergePolicyManual,
		venue.SettingTranslateAfter: "none",
	}, RecipeSettingsOf(&Project{
		ConvergePolicy: ConvergePolicyManual,
		Properties:     map[string]string{TranslateAfterProperty: "none"},
	}))
}

func TestValidateRecipeSettings(t *testing.T) {
	require.NoError(t, ValidateRecipeSettings(nil))
	require.NoError(t, ValidateRecipeSettings(venue.ProjectSettings{
		venue.SettingConvergePolicy: "manual",
		venue.SettingTranslateAfter: "established",
		"a_later_setting":           "anything",
	}), "a key the server does not know is left alone")

	require.ErrorContains(t, ValidateRecipeSettings(venue.ProjectSettings{
		venue.SettingConvergePolicy: "sometimes",
	}), "converge_policy")
	require.ErrorContains(t, ValidateRecipeSettings(venue.ProjectSettings{
		venue.SettingTranslateAfter: "approved",
	}), "translate_after")
}

func TestApplyRecipeSettings(t *testing.T) {
	p := &Project{ConvergePolicy: ConvergePolicyOnPush}

	changed := ApplyRecipeSettings(p, venue.ProjectSettings{
		venue.SettingConvergePolicy: ConvergePolicyManual,
		venue.SettingTranslateAfter: "established",
	})
	assert.True(t, changed)
	assert.Equal(t, ConvergePolicyManual, p.ConvergePolicy)
	assert.Equal(t, "established", p.Properties[TranslateAfterProperty])
	assert.Equal(t, "established", string(TranslateAfterFor(p)))

	assert.False(t, ApplyRecipeSettings(p, RecipeSettingsOf(p)),
		"settings already held change nothing")

	// A project that has never stored a level holds the default, and a push
	// carrying the default writes it, so the stored value is explicit from
	// then on.
	fresh := &Project{}
	assert.True(t, ApplyRecipeSettings(fresh, venue.ProjectSettings{venue.SettingTranslateAfter: "written"}))
	assert.Equal(t, "written", fresh.Properties[TranslateAfterProperty])
}
