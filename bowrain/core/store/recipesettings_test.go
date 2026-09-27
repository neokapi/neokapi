package store

import (
	"testing"

	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecipeSettingsOfResolvesDefaults(t *testing.T) {
	assert.Equal(t, venue.ProjectSettings{
		venue.SettingConvergePolicy: ConvergePolicyOnPush,
		venue.SettingTranslateAfter: "written",
		venue.SettingTermRules:      "",
	}, RecipeSettingsOf(&Project{}), "an unset project holds the defaults")

	assert.Equal(t, venue.ProjectSettings{
		venue.SettingConvergePolicy: ConvergePolicyManual,
		venue.SettingTranslateAfter: "none",
		venue.SettingTermRules:      "",
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

func TestLoosens(t *testing.T) {
	for _, tt := range []struct {
		key, from, to string
		loosens       bool
	}{
		{venue.SettingTranslateAfter, "established", "written", true},
		{venue.SettingTranslateAfter, "written", "none", true},
		{venue.SettingTranslateAfter, "established", "none", true},
		{venue.SettingTranslateAfter, "none", "written", false},
		{venue.SettingTranslateAfter, "written", "established", false},
		{venue.SettingConvergePolicy, ConvergePolicyManual, ConvergePolicyOnPush, true},
		{venue.SettingConvergePolicy, ConvergePolicyOnPush, ConvergePolicyManual, false},
		{"a_later_setting", "b", "a", false},
	} {
		assert.Equal(t, tt.loosens, Loosens(tt.key, tt.from, tt.to), "%s %s -> %s", tt.key, tt.from, tt.to)
	}
}

func TestDecideRecipeSettings(t *testing.T) {
	strict := func() *Project {
		return &Project{
			ConvergePolicy: ConvergePolicyManual,
			Properties:     map[string]string{TranslateAfterProperty: "established"},
		}
	}
	loose := venue.ProjectSettings{
		venue.SettingConvergePolicy: ConvergePolicyOnPush,
		venue.SettingTranslateAfter: "none",
	}

	t.Run("a pusher who may not manage the project tightens", func(t *testing.T) {
		apply, refused := DecideRecipeSettings(&Project{}, venue.ProjectSettings{
			venue.SettingConvergePolicy: ConvergePolicyManual,
			venue.SettingTranslateAfter: "established",
		}, SettingsPusher{Stream: "main"})
		assert.Empty(t, refused)
		assert.Equal(t, venue.ProjectSettings{
			venue.SettingConvergePolicy: ConvergePolicyManual,
			venue.SettingTranslateAfter: "established",
		}, apply)
	})

	t.Run("a pusher who may not manage the project cannot loosen", func(t *testing.T) {
		apply, refused := DecideRecipeSettings(strict(), loose, SettingsPusher{Stream: "main"})
		assert.Empty(t, apply)
		assert.Equal(t, []venue.SettingRefusal{
			{Setting: venue.SettingConvergePolicy, Requested: ConvergePolicyOnPush, InForce: ConvergePolicyManual,
				Reason: venue.SettingLoosens, Requires: "manage_project"},
			{Setting: venue.SettingTranslateAfter, Requested: "none", InForce: "established",
				Reason: venue.SettingLoosens, Requires: "manage_project"},
		}, refused)
	})

	t.Run("a pusher who may manage the project loosens", func(t *testing.T) {
		apply, refused := DecideRecipeSettings(strict(), loose, SettingsPusher{Stream: "main", MayLoosen: true})
		assert.Empty(t, refused)
		assert.Equal(t, loose, apply)
	})

	t.Run("one setting tightens while another is refused", func(t *testing.T) {
		apply, refused := DecideRecipeSettings(&Project{
			Properties: map[string]string{TranslateAfterProperty: "established"},
		}, venue.ProjectSettings{
			venue.SettingConvergePolicy: ConvergePolicyManual,
			venue.SettingTranslateAfter: "written",
		}, SettingsPusher{Stream: "main"})
		assert.Equal(t, venue.ProjectSettings{venue.SettingConvergePolicy: ConvergePolicyManual}, apply)
		require.Len(t, refused, 1)
		assert.Equal(t, venue.SettingTranslateAfter, refused[0].Setting)
	})

	t.Run("a push to another stream applies nothing", func(t *testing.T) {
		for _, mayLoosen := range []bool{false, true} {
			apply, refused := DecideRecipeSettings(&Project{DefaultStream: "main"}, venue.ProjectSettings{
				venue.SettingTranslateAfter: "established",
			}, SettingsPusher{Stream: "feature-x", MayLoosen: mayLoosen})
			assert.Empty(t, apply)
			assert.Equal(t, []venue.SettingRefusal{{
				Setting: venue.SettingTranslateAfter, Requested: "established", InForce: "written",
				Reason: venue.SettingNotDefaultStream, DefaultStream: "main",
			}}, refused)
		}
	})

	t.Run("the default stream is the project's own", func(t *testing.T) {
		apply, refused := DecideRecipeSettings(&Project{DefaultStream: "v2"}, venue.ProjectSettings{
			venue.SettingTranslateAfter: "established",
		}, SettingsPusher{Stream: "v2"})
		assert.Empty(t, refused)
		assert.Equal(t, venue.ProjectSettings{venue.SettingTranslateAfter: "established"}, apply)
	})

	t.Run("settings already held are neither applied nor refused", func(t *testing.T) {
		apply, refused := DecideRecipeSettings(strict(), RecipeSettingsOf(strict()), SettingsPusher{Stream: "feature-x"})
		assert.Empty(t, apply)
		assert.Empty(t, refused)
	})
}

func TestApplyRecipeSettings(t *testing.T) {
	p := &Project{ConvergePolicy: ConvergePolicyOnPush}

	changes := ApplyRecipeSettings(p, venue.ProjectSettings{
		venue.SettingConvergePolicy: ConvergePolicyManual,
		venue.SettingTranslateAfter: "established",
	})
	assert.Equal(t, []SettingChange{
		{Setting: venue.SettingConvergePolicy, From: ConvergePolicyOnPush, To: ConvergePolicyManual},
		{Setting: venue.SettingTranslateAfter, From: "written", To: "established"},
	}, changes)
	assert.Equal(t, ConvergePolicyManual, p.ConvergePolicy)
	assert.Equal(t, "established", string(TranslateAfterFor(p)))

	assert.Empty(t, ApplyRecipeSettings(p, RecipeSettingsOf(p)), "settings already held change nothing")
}

// encodeRules is the settings value a recipe with these rules sends.
func encodeRules(t *testing.T, all ...profile.TermRule) string {
	t.Helper()
	s, err := profile.RecipeTermRules{All: all}.Encode()
	require.NoError(t, err)
	return s
}

// The recipe's term rules are held as the push applied them, and a push that
// keeps every held rule tightens the setting while one that drops or changes a
// rule loosens it.
func TestRecipeTermRulesSetting(t *testing.T) {
	period := profile.TermRule{Term: "billing period", Replacement: "période de facturation"}
	reading := profile.TermRule{Term: "reading", Replacement: "relevé", Advisory: true}
	one, two := encodeRules(t, period), encodeRules(t, period, reading)

	require.NoError(t, ValidateRecipeSettings(venue.ProjectSettings{venue.SettingTermRules: two}))
	require.NoError(t, ValidateRecipeSettings(venue.ProjectSettings{venue.SettingTermRules: ""}))
	require.ErrorContains(t, ValidateRecipeSettings(venue.ProjectSettings{venue.SettingTermRules: "{"}), "term_rules")

	assert.False(t, Loosens(venue.SettingTermRules, "", one), "declaring rules tightens")
	assert.False(t, Loosens(venue.SettingTermRules, one, two), "adding a rule tightens")
	assert.True(t, Loosens(venue.SettingTermRules, two, one), "dropping a rule loosens")
	assert.True(t, Loosens(venue.SettingTermRules, one, ""), "dropping every rule loosens")

	p := &Project{}
	apply, refused := DecideRecipeSettings(p, venue.ProjectSettings{venue.SettingTermRules: two}, SettingsPusher{Stream: "main"})
	assert.Empty(t, refused)
	assert.Equal(t, venue.ProjectSettings{venue.SettingTermRules: two}, apply)
	changes := ApplyRecipeSettings(p, apply)
	assert.Equal(t, []SettingChange{{Setting: venue.SettingTermRules, From: "", To: two}}, changes)
	assert.Equal(t, two, RecipeSettingsOf(p)[venue.SettingTermRules])
	assert.Equal(t, []profile.TermRule{period, reading}, RecipeTermRulesOf(p).For("fr"))

	apply, refused = DecideRecipeSettings(p, venue.ProjectSettings{venue.SettingTermRules: one}, SettingsPusher{Stream: "main"})
	assert.Empty(t, apply)
	require.Len(t, refused, 1)
	assert.Equal(t, venue.SettingLoosens, refused[0].Reason)
	assert.Contains(t, refused[0].String(), "term_rules keeps the project's rules")
	assert.NotContains(t, refused[0].String(), "billing period", "the rule lists stay out of the line")

	apply, _ = DecideRecipeSettings(p, venue.ProjectSettings{venue.SettingTermRules: ""}, SettingsPusher{Stream: "main", MayLoosen: true})
	assert.Equal(t, []SettingChange{{Setting: venue.SettingTermRules, From: two, To: ""}}, ApplyRecipeSettings(p, apply))
	assert.NotContains(t, p.Properties, TermRulesProperty)
	assert.True(t, RecipeTermRulesOf(p).Empty())
}
