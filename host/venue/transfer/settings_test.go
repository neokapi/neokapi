package transfer

import (
	"testing"

	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/host"
	bproject "github.com/neokapi/neokapi/host/venue/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildPushContext_CarriesRecipeSettings pins that every push route carries
// the recipe's project settings, resolved: an unset key travels as its default,
// so a recipe that drops a setting returns the venue to the default.
func TestBuildPushContext_CarriesRecipeSettings(t *testing.T) {
	build := func(t *testing.T, defaults coreproj.Defaults, server *bproject.ServerSpec) venue.ProjectSettings {
		t.Helper()
		defaults.SourceLanguage = "en"
		proj, err := bproject.InitProject(t.TempDir(), &bproject.Recipe{
			Defaults: defaults,
			Server:   server,
		})
		require.NoError(t, err)
		pushCtx, _, err := BuildPushContext(t.Context(), &host.App{}, proj, false)
		require.NoError(t, err)
		require.NotNil(t, pushCtx)
		return pushCtx.Settings
	}

	t.Run("declared", func(t *testing.T) {
		got := build(t,
			coreproj.Defaults{TranslateAfter: "established"},
			&bproject.ServerSpec{URL: "https://bowrain.example.com/team/proj", Converge: "manual"})
		assert.Equal(t, venue.ProjectSettings{
			venue.SettingConvergePolicy: "manual",
			venue.SettingTranslateAfter: "established",
		}, got)
	})

	t.Run("unset travels as the default", func(t *testing.T) {
		got := build(t, coreproj.Defaults{},
			&bproject.ServerSpec{URL: "https://bowrain.example.com/team/proj"})
		assert.Equal(t, venue.ProjectSettings{
			venue.SettingConvergePolicy: "on-push",
			venue.SettingTranslateAfter: "written",
		}, got)
	})

	t.Run("no venue binding carries none", func(t *testing.T) {
		assert.Nil(t, build(t, coreproj.Defaults{TranslateAfter: "none"}, nil))
	})
}
