package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	appconfig "github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/credentials"
)

// setupProject writes a one-file project whose `draft` flow runs translate
// with the given step config, and returns it with its recipe path.
func setupProject(t *testing.T, stepConfig map[string]any) (*project.KapiProject, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"), []byte(`{"a":"Hello"}`), 0o644))
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "setup",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
			Flow:            "draft",
		},
		Collections: []project.Collection{{Name: "docs", Path: "src/en.json", Target: "src/{lang}.json"}},
		Flows: map[string]*flow.StepsSpec{
			"draft": {Steps: []flow.FlowStep{
				{Tool: "recycle"},
				{Tool: "translate", Config: stepConfig},
			}},
		},
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	return proj, recipe
}

// clearProviderEnv (contextdeeper_test.go) removes every provider key and AI
// default from the environment, so the developer's own ANTHROPIC_API_KEY or
// KAPI_AI_PROVIDER cannot decide these tests.
func TestProjectFlowAISetup_ByProviderCredentials(t *testing.T) {
	clearProviderEnv(t)

	withCredential := func(t *testing.T, providerType string) *credentials.Store {
		s := credentials.NewStore(filepath.Join(t.TempDir(), "credentials.json"))
		_, err := s.Upsert(credentials.ProviderConfig{Name: providerType + " key", ProviderType: providerType})
		require.NoError(t, err)
		return s
	}
	empty := func(t *testing.T) *credentials.Store {
		return credentials.NewStore(filepath.Join(t.TempDir(), "credentials.json"))
	}
	openaiKey := func(t *testing.T) *credentials.Store { return withCredential(t, "openai") }
	anthropicKey := func(t *testing.T) *credentials.Store { return withCredential(t, "anthropic") }

	const (
		none    = AISetupNone
		model   = AISetupChooseModel
		missing = AISetupMissingCredential
	)
	tests := []struct {
		name   string
		step   map[string]any
		store  func(*testing.T) *credentials.Store
		config *appconfig.AppConfig
		env    map[string]string
		want   AISetupKind
	}{
		{name: "credential-free provider on the step", step: map[string]any{"provider": "demo"}, store: empty, want: none},
		{name: "credential-free provider with no store at all", step: map[string]any{"provider": "ollama"}, want: none},
		{name: "provider named with its credential missing", step: map[string]any{"provider": "anthropic"}, store: empty, want: missing},
		{name: "provider named with a credential for another provider", step: map[string]any{"provider": "anthropic"}, store: openaiKey, want: missing},
		// The reviewer's scenario: a stored default for another provider
		// cannot help, because it never overrides the named one.
		{name: "provider named, no key, stored default for another provider", step: map[string]any{"provider": "anthropic"}, store: empty, config: cfgWith("ollama", "llama3.2:3b"), want: missing},
		{name: "saved credential named but not saved", step: map[string]any{"credential": "work"}, store: empty, want: missing},
		{name: "provider named with its credential saved", step: map[string]any{"provider": "anthropic"}, store: anthropicKey, want: none},
		{name: "provider named with its key in the environment", step: map[string]any{"provider": "anthropic"}, store: empty, env: map[string]string{"ANTHROPIC_API_KEY": "sk-test"}, want: none},
		{name: "no provider and nothing configured", step: nil, store: empty, want: model},
		{name: "no provider and one saved credential", step: nil, store: openaiKey, want: none},
		{name: "no provider and a credential-free stored default", step: nil, store: empty, config: cfgWith("demo", ""), want: none},
		{name: "no provider and a stored default whose credential is missing", step: nil, store: empty, config: cfgWith("openai", ""), want: model},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			proj, recipe := setupProject(t, tt.step)
			a := &App{Config: tt.config}
			if tt.store != nil {
				a.Credentials = tt.store(t)
			}
			got, err := a.ProjectFlowAISetup(proj, recipe, "", nil)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Kind)

			named, err := a.ProjectFlowAISetup(proj, recipe, "draft", nil)
			require.NoError(t, err)
			assert.Equal(t, tt.want, named.Kind, "running the flow by name answers as convergence does")

			if tt.want == missing {
				assert.Equal(t, "draft", got.Flow)
				assert.Equal(t, "translate", got.Tool)
				assert.NotEmpty(t, got.Message)
				require.Error(t, got.Err())
			} else {
				assert.NoError(t, got.Err())
			}
		})
	}
}

// A missing credential names the provider and carries the error the run
// would fail with, so the CLI and the desktop can show it before the run.
func TestProjectFlowAISetup_MissingCredentialNamesTheProvider(t *testing.T) {
	clearProviderEnv(t)
	proj, recipe := setupProject(t, map[string]any{"provider": "anthropic"})
	a := &App{
		Config:      cfgWith("ollama", "llama3.2:3b"),
		Credentials: credentials.NewStore(filepath.Join(t.TempDir(), "credentials.json")),
	}
	need, err := a.ProjectFlowAISetup(proj, recipe, "", nil)
	require.NoError(t, err)
	assert.Equal(t, AISetupNeed{
		Kind:     AISetupMissingCredential,
		Flow:     "draft",
		Tool:     "translate",
		Provider: "anthropic",
		Message:  `no saved credentials found for provider "anthropic"; save one with 'kapi credentials add' or set ANTHROPIC_API_KEY`,
		onCall:   true,
	}, need)
	assert.EqualError(t, need.Err(), `flow "draft": tool "translate": no saved credentials found for provider "anthropic"; save one with 'kapi credentials add' or set ANTHROPIC_API_KEY`)
}

// The decision reads the recipe's step config and must leave it as it was:
// the stored default it applies belongs to this one question.
func TestProjectFlowAISetup_LeavesTheRecipeUntouched(t *testing.T) {
	clearProviderEnv(t)
	proj, recipe := setupProject(t, map[string]any{"skip_matched": true})
	a := &App{Config: cfgWith("demo", "")}
	got, err := a.ProjectFlowAISetup(proj, recipe, "draft", nil)
	require.NoError(t, err)
	assert.Equal(t, AISetupNone, got.Kind)
	assert.Equal(t, map[string]any{"skip_matched": true}, proj.Flows["draft"].Steps[1].Config)
}

// The run configuration (`kapi up --provider`, a desktop run started with an
// explicit provider) applies by the rule every run applies: a step's own
// config, then the run's, then the presets, then the stored default.
func TestProjectFlowAISetup_RunConfig(t *testing.T) {
	clearProviderEnv(t)
	empty := func(t *testing.T) *credentials.Store {
		return credentials.NewStore(filepath.Join(t.TempDir(), "credentials.json"))
	}
	demo := map[string]any{"provider": "demo"}
	tests := []struct {
		name   string
		step   map[string]any
		preset map[string]any
		run    map[string]any
		want   AISetupKind
	}{
		{name: "preset names a provider with no key", preset: map[string]any{"provider": "anthropic"}, want: AISetupMissingCredential},
		{name: "--provider demo over a preset with no key", preset: map[string]any{"provider": "anthropic"}, run: demo, want: AISetupNone},
		{name: "--provider demo with nothing configured", run: demo, want: AISetupNone},
		{name: "--provider with no key over a keyless preset", preset: demo, run: map[string]any{"provider": "anthropic"}, want: AISetupMissingCredential},
		{name: "a step's own provider outranks --provider", step: map[string]any{"provider": "anthropic"}, run: demo, want: AISetupMissingCredential},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proj, recipe := setupProject(t, tt.step)
			if tt.preset != nil {
				proj.Defaults.Tools = map[string]map[string]any{"translate": tt.preset}
			}
			a := &App{Credentials: empty(t)}
			for _, flowName := range []string{"", "draft"} {
				got, err := a.ProjectFlowAISetup(proj, recipe, flowName, tt.run)
				require.NoError(t, err)
				assert.Equal(t, tt.want, got.Kind, "flow %q", flowName)
			}
		})
	}
}
