package cli

import (
	"bytes"
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
	"github.com/neokapi/neokapi/host/credentials"
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
	assert.Equal(t, "anthropic", planProvider(t, "", bareTranslateFixture(t, "")),
		"nothing names a provider: the tool's default")
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
	return commandWith(t, NewUpCmd, recipe, stored, args...)
}

// commandWith is upWith for the command newCmd builds.
func commandWith(t *testing.T, newCmd func(*App) *cobra.Command, recipe, stored string, args ...string) (string, *recorder, error) {
	t.Helper()
	a := processOnlyApp(t)
	rec := &recorder{}
	cmd := newCmd(a)
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

// withStepProvider names provider in the translate step's own config.
func withStepProvider(t *testing.T, recipe, provider string) string {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	steps := proj.Flows["converge"].Steps
	for i := range steps {
		if steps[i].Tool == "translate" {
			steps[i].Config["provider"] = provider
		}
	}
	require.NoError(t, project.Save(recipe, proj))
	return recipe
}

// The plan and the run agree for the same arguments, on every rung: the
// plan names the provider the run's translate step is constructed with. When
// the run stops for want of that provider's credential, the plan names the
// provider and says what the run says.
func TestUpPlan_ProviderMatchesTheRun(t *testing.T) {
	for _, tc := range []struct {
		name, preset, stored, step string
		env                        map[string]string
		args                       []string
		want                       string
		stops                      bool
		advice                     string // what each command's error must name
	}{
		{name: "a provider key in the environment", env: map[string]string{"ANTHROPIC_API_KEY": "sk-test"}, want: "anthropic"},
		{name: "the run flag", args: []string{"--provider", "demo"}, want: "demo"},
		{name: "the recipe preset", preset: "openai", env: map[string]string{"OPENAI_API_KEY": "sk-test"}, want: "openai"},
		{name: "the stored default", stored: "ollama", want: "ollama"},
		{name: "nothing configured", want: "anthropic", stops: true, advice: "ANTHROPIC_API_KEY"},
		{name: "a step naming a provider with no key", step: "anthropic", want: "anthropic", stops: true},
		{name: "a preset with no key over a stored default", preset: "openai", stored: "ollama", want: "openai", stops: true},
		{name: "a credential that is not saved", env: map[string]string{"ANTHROPIC_API_KEY": "sk-test"},
			args: []string{"--credential", "nosuch"}, want: "anthropic", stops: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			recipe := bareTranslateFixture(t, tc.preset)
			if tc.step != "" {
				withStepProvider(t, recipe, tc.step)
			}

			out, _, err := upWith(t, recipe, tc.stored, append([]string{"--plan", "--json"}, tc.args...)...)
			require.NoError(t, err, out)
			var plan UpPlanOutput
			require.NoError(t, json.Unmarshal([]byte(out), &plan), out)
			assert.Equal(t, tc.want, plan.Provider, "the plan names the run's provider")

			text, _, err := upWith(t, recipe, tc.stored, append([]string{"--plan"}, tc.args...)...)
			require.NoError(t, err, text)
			assert.Contains(t, text, "AI provider: "+tc.want)
			assert.NotContains(t, text, "none configured", "no advice that fixes nothing")

			out, rec, err := upWith(t, recipe, tc.stored, tc.args...)
			if tc.stops {
				require.Error(t, err, "the run stops for want of a credential:\n%s", out)
				require.NotEmpty(t, plan.Credential, "the plan says the run would stop")
				// The person sees the error with the --api-key advice kapi up's
				// exit adds (credentials.WithAPIKeyFlag); the plan prints that.
				assert.Contains(t, credentials.WithAPIKeyFlag(err).Error(), plan.Credential, "the plan says what the run says")
				if tc.advice != "" {
					assert.Contains(t, plan.Credential, `provider "`+tc.want+`"`, "the advice names the provider the plan names")
					assert.Contains(t, err.Error(), tc.advice)
				}
				assert.Contains(t, text, plan.Credential)

				// kapi run (on the fixture recipe's own converge flow) and kapi
				// translate refuse the same project, for the same provider,
				// before they write anything. kapi translate runs its own flow,
				// not the recipe's, so a provider the recipe's step names does
				// not reach it.
				commands := map[string]func() (string, *recorder, error){
					"run": func() (string, *recorder, error) {
						return commandWith(t, NewRunCmd, recipe, tc.stored, append([]string{"converge"}, tc.args...)...)
					},
					"translate": func() (string, *recorder, error) {
						return commandWith(t, NewTranslateCmd, recipe, tc.stored, append([]string{filepath.Join(filepath.Dir(recipe), "src/locales/en/a.json"), "--target-lang", "nb-NO"}, tc.args...)...)
					},
				}
				if tc.step != "" {
					delete(commands, "translate")
				}
				for name, run := range commands {
					out, _, err := run()
					require.Error(t, err, "kapi %s stops too:\n%s", name, out)
					assert.Contains(t, err.Error(), credentialCause(plan.Credential), "kapi %s gives the same cause", name)
					assert.Contains(t, err.Error(), tc.advice, "kapi %s names what fixes it", name)
				}
				_, statErr := os.Stat(filepath.Join(filepath.Dir(recipe), "src/locales/nb-NO"))
				assert.True(t, os.IsNotExist(statErr), "nothing was written")
				return
			}
			assert.Empty(t, plan.Credential, "the credential is available")
			require.NoError(t, err, out)
			built := rec.constructed()
			require.NotEmpty(t, built, out)
			for _, c := range built {
				assert.Equal(t, tc.want, c["provider"], "the run's translate step calls the provider the plan names")
			}
		})
	}
}

// credentialCause is the part of a missing-credential message every command
// shares: the advice after it names --api-key only for a command that reads
// it (credentials.WithAPIKeyFlag).
func credentialCause(msg string) string {
	cause, _, _ := strings.Cut(msg, "; ")
	if i := strings.Index(cause, "no saved credentials"); i >= 0 {
		return cause[i:]
	}
	if i := strings.Index(cause, "credential \""); i >= 0 {
		return cause[i:]
	}
	return cause
}

// A per-language preset changes what a language uses, and the plan says so
// per language: one line per group of languages that agree.
func TestUpPlan_ProviderPerLanguage(t *testing.T) {
	isolate(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO", "de-DE"}, gate.Gate{"translated": gate.Threshold{Pct: 100}})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Flow = "converge"
	proj.Flows["converge"] = &flow.StepsSpec{Steps: []flow.FlowStep{
		{Tool: "recycle"},
		{Tool: "translate", Config: map[string]any{"skip_matched": true}},
	}}
	proj.Defaults.Locales = map[string]project.LocaleDefaults{"de-DE": {Tools: map[string]map[string]any{"translate": {"provider": "openai"}}}}
	require.NoError(t, project.Save(recipe, proj))

	out, _, err := upWith(t, recipe, "", "--plan", "--json")
	require.NoError(t, err, out)
	var plan UpPlanOutput
	require.NoError(t, json.Unmarshal([]byte(out), &plan), out)
	require.Len(t, plan.Providers, 2, out)
	byLocale := map[string]UpPlanProviderGroup{}
	for _, g := range plan.Providers {
		for _, l := range g.Locales {
			byLocale[l] = g
		}
	}
	assert.Equal(t, "anthropic", byLocale["nb-NO"].Provider)
	assert.Empty(t, byLocale["nb-NO"].Credential)
	assert.Equal(t, "openai", byLocale["de-DE"].Provider)
	assert.Contains(t, byLocale["de-DE"].Credential, `provider "openai"`)

	text, _, err := upWith(t, recipe, "", "--plan")
	require.NoError(t, err, text)
	assert.Contains(t, text, "AI provider by language:")
	assert.Contains(t, text, "nb-NO: anthropic\n")
	assert.Contains(t, text, "de-DE: openai. The run would stop before drafting:")

	_, _, err = upWith(t, recipe, "")
	require.Error(t, err, "German stops the run")
	assert.Contains(t, credentials.WithAPIKeyFlag(err).Error(), byLocale["de-DE"].Credential)
}
