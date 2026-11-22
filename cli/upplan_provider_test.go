package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/config"
	"github.com/spf13/cobra"
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

// planProvider runs `kapi up --plan --json` with args, stored as the stored
// AI provider, and returns the provider the plan names.
func planProvider(t *testing.T, stored, recipe string, args ...string) string {
	t.Helper()
	out, _, err := upWith(t, recipe, stored, append([]string{"--plan", "--json"}, args...)...)
	require.NoError(t, err, out)
	var plan UpPlanOutput
	require.NoError(t, json.Unmarshal([]byte(out), &plan), out)
	return plan.Provider
}

// The plan names the provider each step will call, by the rule the run
// applies: a step's own config, the run flag, the recipe's presets, the stored
// default. A plan with nothing configured says so.
func TestUpPlan_ProviderFollowsThePrecedenceRule(t *testing.T) {
	isolate(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	pinned, _ := planFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})

	assert.Equal(t, "demo", planProvider(t, "anthropic", pinned, "--provider", "openai"),
		"a step's own provider wins over the flag and the stored default")
	assert.Equal(t, "demo", planProvider(t, "anthropic", bareTranslateFixture(t, "openai"), "--provider", "demo"),
		"the flag wins over the preset and the stored default")
	assert.Equal(t, "openai", planProvider(t, "anthropic", bareTranslateFixture(t, "openai")),
		"the preset wins over the stored default")
	assert.Equal(t, "ollama", planProvider(t, "ollama", bareTranslateFixture(t, "")),
		"the stored default applies when nothing else names a provider")

	t.Setenv("ANTHROPIC_API_KEY", "")
	assert.Empty(t, planProvider(t, "", bareTranslateFixture(t, "")), "nothing names a provider")
	out, _, err := upWith(t, bareTranslateFixture(t, ""), "", "--plan", "--provider", "demo")
	require.NoError(t, err, out)
	assert.Contains(t, out, "AI provider: demo")
	assert.NotContains(t, out, "none configured", "the plan gives no setup advice the run does not need")
}

// upWith runs `kapi up` on recipe as the binary does (the root's
// initialization, then the command) with stored as the stored AI provider,
// recording the config the translate step is constructed with.
func upWith(t *testing.T, recipe, stored string, args ...string) (string, *recorder, error) {
	t.Helper()
	a := processOnlyApp(t)
	rec := &recorder{}
	cmd := NewUpCmd(a)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(t.Context())
	cmd.SetArgs(append([]string{"--project", recipe}, args...))
	cmd.PersistentPreRunE = func(*cobra.Command, []string) error {
		if err := a.Init(); err != nil {
			return err
		}
		if stored != "" {
			a.Config.Set(config.KeyAIProvider, stored)
		}
		rec.install(a)
		return nil
	}
	err := cmd.Execute()
	return out.String(), rec, err
}

// The plan and the run agree for the same arguments, on every rung: the
// plan names the provider the run's translate step is constructed with, and
// says none is configured exactly when the run stops for want of one.
func TestUpPlan_ProviderMatchesTheRun(t *testing.T) {
	for _, tc := range []struct {
		name, preset, stored string
		env                  map[string]string
		args                 []string
		want                 string
	}{
		{name: "a provider key in the environment", env: map[string]string{"ANTHROPIC_API_KEY": "sk-test"}, want: "anthropic"},
		{name: "the run flag", args: []string{"--provider", "demo"}, want: "demo"},
		{name: "the recipe preset", preset: "openai", env: map[string]string{"OPENAI_API_KEY": "sk-test"}, want: "openai"},
		{name: "the stored default", stored: "ollama", want: "ollama"},
		{name: "nothing configured"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			recipe := bareTranslateFixture(t, tc.preset)

			out, _, err := upWith(t, recipe, tc.stored, append([]string{"--plan", "--json"}, tc.args...)...)
			require.NoError(t, err, out)
			var plan UpPlanOutput
			require.NoError(t, json.Unmarshal([]byte(out), &plan), out)
			assert.Equal(t, tc.want, plan.Provider, "the plan names the run's provider")

			out, rec, err := upWith(t, recipe, tc.stored, tc.args...)
			if tc.want == "" {
				require.Error(t, err, "the run stops asking for a provider:\n%s", out)
				return
			}
			require.NoError(t, err, out)
			built := rec.constructed()
			require.NotEmpty(t, built, out)
			for _, c := range built {
				assert.Equal(t, tc.want, c["provider"], "the run's translate step calls the provider the plan names")
			}
		})
	}
}
