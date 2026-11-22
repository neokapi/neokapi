package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bareTranslateFixture is planFixture with a translate step that names no
// provider, so the provider comes from the run flags, the presets or the
// stored default. preset, when set, is defaults.tools.translate.provider.
func bareTranslateFixture(t *testing.T, preset string) string {
	t.Helper()
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Flow = "converge"
	proj.Flows["converge"] = &flow.StepsSpec{Steps: []flow.FlowStep{
		{Tool: "recycle"},
		{Tool: "translate", Config: map[string]any{"skip_matched": true}},
	}}
	if preset != "" {
		proj.Defaults.Tools = map[string]map[string]any{"translate": {"provider": preset}}
	}
	require.NoError(t, project.Save(recipe, proj))
	return recipe
}

// planProvider runs `kapi up --plan --json` with args and returns the
// provider the plan names.
func planProvider(t *testing.T, a *App, recipe string, args ...string) string {
	t.Helper()
	out, err := runUp(t, a, recipe, append([]string{"--plan", "--json"}, args...)...)
	require.NoError(t, err, out)
	var plan UpPlanOutput
	require.NoError(t, json.Unmarshal([]byte(out), &plan), out)
	return plan.Provider
}

// The plan names the provider each step will call, by the rule the run
// applies: a step's own config, the run flag, the recipe's presets, the stored
// default. A plan with nothing configured says so.
func TestUpPlan_ProviderFollowsThePrecedenceRule(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	t.Setenv("KAPI_AI_PROVIDER", "")
	stored := func(provider string) *App {
		a := processOnlyApp(t)
		a.Config = config.NewAppConfig()
		if provider != "" {
			a.Config.Set(config.KeyAIProvider, provider)
		}
		return a
	}
	pinned, _ := planFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})

	assert.Equal(t, "demo", planProvider(t, stored("anthropic"), pinned, "--provider", "openai"),
		"a step's own provider wins over the flag and the stored default")
	assert.Equal(t, "demo", planProvider(t, stored("anthropic"), bareTranslateFixture(t, "openai"), "--provider", "demo"),
		"the flag wins over the preset and the stored default")
	assert.Equal(t, "openai", planProvider(t, stored("anthropic"), bareTranslateFixture(t, "openai")),
		"the preset wins over the stored default")
	assert.Equal(t, "ollama", planProvider(t, stored("ollama"), bareTranslateFixture(t, "")),
		"the stored default applies when nothing else names a provider")
	assert.Empty(t, planProvider(t, stored(""), bareTranslateFixture(t, "")),
		"nothing names a provider")

	out, err := runUp(t, stored(""), bareTranslateFixture(t, ""), "--plan", "--provider", "demo")
	require.NoError(t, err, out)
	assert.Contains(t, out, "AI provider: demo")
	assert.NotContains(t, out, "none configured", "the plan gives no setup advice the run does not need")
}

// The plan and the run agree: the run given the same arguments drafts with
// the provider the plan names.
func TestUpPlan_ProviderMatchesTheRun(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	t.Setenv("KAPI_AI_PROVIDER", "")
	a := processOnlyApp(t)
	a.Config = config.NewAppConfig()
	recipe := bareTranslateFixture(t, "")
	require.Equal(t, "demo", planProvider(t, a, recipe, "--provider", "demo"))

	run := processOnlyApp(t)
	run.Config = config.NewAppConfig()
	out, err := runUp(t, run, recipe, "--provider", "demo")
	require.NoError(t, err, out)
	got, err := os.ReadFile(filepath.Join(filepath.Dir(recipe), "src", "locales", "nb-NO", "a.json"))
	require.NoError(t, err, out)
	assert.True(t, strings.Contains(string(got), "⟦nb"), "the demo provider drafted the target: %s", got)
}
