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
func TestProjectFlowNeedsAISetup_ByProviderCredentials(t *testing.T) {
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

	tests := []struct {
		name   string
		step   map[string]any
		store  func(*testing.T) *credentials.Store
		config *appconfig.AppConfig
		env    map[string]string
		want   bool
	}{
		{name: "credential-free provider on the step", step: map[string]any{"provider": "demo"}, store: empty, want: false},
		{name: "credential-free provider with no store at all", step: map[string]any{"provider": "ollama"}, want: false},
		{name: "provider named with its credential missing", step: map[string]any{"provider": "anthropic"}, store: empty, want: true},
		{name: "provider named with a credential for another provider", step: map[string]any{"provider": "anthropic"}, store: func(t *testing.T) *credentials.Store { return withCredential(t, "openai") }, want: true},
		{name: "provider named with its credential saved", step: map[string]any{"provider": "anthropic"}, store: func(t *testing.T) *credentials.Store { return withCredential(t, "anthropic") }, want: false},
		{name: "provider named with its key in the environment", step: map[string]any{"provider": "anthropic"}, store: empty, env: map[string]string{"ANTHROPIC_API_KEY": "sk-test"}, want: false},
		{name: "no provider and nothing configured", step: nil, store: empty, want: true},
		{name: "no provider and one saved credential", step: nil, store: func(t *testing.T) *credentials.Store { return withCredential(t, "openai") }, want: false},
		{name: "no provider and a credential-free stored default", step: nil, store: empty, config: cfgWith("demo", ""), want: false},
		{name: "no provider and a stored default whose credential is missing", step: nil, store: empty, config: cfgWith("openai", ""), want: true},
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
			got, err := a.ProjectFlowNeedsAISetup(proj, recipe, "")
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)

			named, err := a.ProjectFlowNeedsAISetup(proj, recipe, "draft")
			require.NoError(t, err)
			assert.Equal(t, tt.want, named, "running the flow by name answers as convergence does")
		})
	}
}

// The decision reads the recipe's step config and must leave it as it was:
// the stored default it applies belongs to this one question.
func TestProjectFlowNeedsAISetup_LeavesTheRecipeUntouched(t *testing.T) {
	clearProviderEnv(t)
	proj, recipe := setupProject(t, map[string]any{"skip_matched": true})
	a := &App{Config: cfgWith("demo", "")}
	got, err := a.ProjectFlowNeedsAISetup(proj, recipe, "draft")
	require.NoError(t, err)
	assert.False(t, got)
	assert.Equal(t, map[string]any{"skip_matched": true}, proj.Flows["draft"].Steps[1].Config)
}
