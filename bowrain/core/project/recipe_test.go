package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/neokapi/neokapi/bowrain/plugin/schema" // ensure decoders are registered

	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRecipe_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := `version: v1
name: my-app
defaults:
  source_language: en-US
  target_languages:
    - fr-FR
    - de-DE
content:
  - path: src/locales/**/*.json
    format: json
server:
  url: https://bowrain.example.com/team/proj
  stream: $auto
hooks:
  pre-push:
    - qa
automations:
  - name: auto-translate
    trigger: post-push
    actions:
      - type: wait_translate
      - type: pull
assets:
  enabled: true
  max_size: 100MB
brand_voice:
  profile: company
`
	path := filepath.Join(dir, "test.kapi")
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))

	r, err := LoadRecipe(path)
	require.NoError(t, err)

	// Embedded framework fields decode normally.
	assert.Equal(t, "v1", r.Version)
	assert.Equal(t, "my-app", r.Name)
	assert.Equal(t, "en-US", string(r.Defaults.SourceLanguage))
	require.Len(t, r.Content, 1)
	assert.Equal(t, "src/locales/**/*.json", r.Content[0].Path)

	// Bowrain extension fields decode into typed slots.
	require.NotNil(t, r.Server)
	assert.Equal(t, "https://bowrain.example.com/team/proj", r.Server.URL)
	assert.Equal(t, "$auto", r.Server.Stream)
	assert.Equal(t, []string{"qa"}, r.Hooks["pre-push"])
	require.Len(t, r.Automations, 1)
	assert.Equal(t, "auto-translate", r.Automations[0].Name)
	require.Len(t, r.Automations[0].Actions, 2)
	assert.Equal(t, "wait_translate", r.Automations[0].Actions[0].Type)
	require.NotNil(t, r.Assets)
	assert.True(t, r.Assets.IsEnabled())
	require.NotNil(t, r.BrandVoice)
	assert.Equal(t, "company", r.BrandVoice.Profile)
}

func TestSaveRecipe_RoundTripPreservesBowrainFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.kapi")

	r := &Recipe{
		Server: &ServerSpec{
			URL:    "https://bowrain.example.com/team/proj",
			Stream: "main",
		},
		Hooks: HooksSpec{
			"pre-push":  []string{"qa"},
			"post-pull": []string{"update-stats"},
		},
		Automations: []AutomationSpec{
			{Name: "auto", Trigger: "post-push", Actions: []ActionConfig{{Type: "pull"}}},
		},
	}
	r.Version = coreproj.CurrentVersion
	r.Name = "my-app"

	require.NoError(t, SaveRecipe(path, r))

	// Reload and verify everything came back.
	r2, err := LoadRecipe(path)
	require.NoError(t, err)
	assert.Equal(t, "my-app", r2.Name)
	require.NotNil(t, r2.Server)
	assert.Equal(t, "https://bowrain.example.com/team/proj", r2.Server.URL)
	assert.Equal(t, "main", r2.Server.Stream)
	assert.Equal(t, []string{"qa"}, r2.Hooks["pre-push"])
	assert.Equal(t, []string{"update-stats"}, r2.Hooks["post-pull"])
	require.Len(t, r2.Automations, 1)
}

func TestLoadRecipe_RejectsInvalidServerURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.kapi")
	require.NoError(t, os.WriteFile(path, []byte(`version: v1
server:
  url: "not a url"
`), 0o644))

	_, err := LoadRecipe(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server")
	assert.Contains(t, err.Error(), "url")
}

func TestRecipe_HasServer(t *testing.T) {
	r := &Recipe{}
	assert.False(t, r.HasServer())

	r.Server = &ServerSpec{}
	assert.False(t, r.HasServer(), "server with empty URL should be HasServer=false")

	r.Server.URL = "https://example.com/team/proj"
	assert.True(t, r.HasServer())
}

func TestRecipe_AssetsEnabled_DefaultsTrue(t *testing.T) {
	r := &Recipe{}
	assert.True(t, r.AssetsEnabled(), "no assets block → enabled by default")

	yes := true
	no := false
	r.Assets = &AssetsSpec{Enabled: &yes}
	assert.True(t, r.AssetsEnabled())
	r.Assets.Enabled = &no
	assert.False(t, r.AssetsEnabled())
}

func TestRecipe_DefaultCollection_FromExtras(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.kapi")
	require.NoError(t, os.WriteFile(path, []byte(`version: v1
defaults:
  source_language: en-US
  collection: ui-strings
`), 0o644))

	r, err := LoadRecipe(path)
	require.NoError(t, err)
	assert.Equal(t, "ui-strings", r.DefaultCollection())
}

func TestRecipe_SetDefaultCollection_PersistsThroughSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.kapi")
	r := &Recipe{}
	r.Version = coreproj.CurrentVersion
	r.Name = "x"
	require.NoError(t, r.SetDefaultCollection("nav-strings"))
	require.NoError(t, SaveRecipe(path, r))

	r2, err := LoadRecipe(path)
	require.NoError(t, err)
	assert.Equal(t, "nav-strings", r2.DefaultCollection())

	// Clear it.
	require.NoError(t, r2.SetDefaultCollection(""))
	require.NoError(t, SaveRecipe(path, r2))

	r3, err := LoadRecipe(path)
	require.NoError(t, err)
	assert.Empty(t, r3.DefaultCollection())
}

func TestFindRecipe_WalksUpward(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "deep", "nested"), 0o755))
	recipePath := filepath.Join(root, "myapp.kapi")
	require.NoError(t, os.WriteFile(recipePath, []byte(`version: v1
name: myapp
`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))

	// Walking up from a nested directory must find the recipe.
	r, layout, err := FindRecipe(filepath.Join(root, "deep", "nested"))
	require.NoError(t, err)
	assert.Equal(t, "myapp", r.Name)
	assert.Equal(t, recipePath, layout.RecipePath)
}

func TestRecipe_TypeAliasesMatchSchema(t *testing.T) {
	// The bowrain/core/project package re-exports schema types as
	// type aliases for backward compatibility. These checks ensure the
	// aliases haven't drifted into separate types.
	var s ServerSpec
	s.URL = "https://x.example.com/t/p"
	assert.Equal(t, "https://x.example.com", s.ServerURL())
	assert.Equal(t, "p", s.ProjectID())
	assert.Equal(t, "t", s.Workspace())

	// Constants alias.
	assert.Equal(t, "pre-push", HookPrePush)
	assert.Equal(t, "$auto", StreamAuto)
	assert.Equal(t, "main", StreamMain)
	assert.Equal(t, "wait_translate", ActionWaitTranslate)
}

func TestFrameworkLoad_PreservesUnknownExtras(t *testing.T) {
	// Forward-compat guarantee: a recipe loaded via the framework's
	// coreproj.Load (without the bowrain Recipe wrapper) must preserve
	// unknown top-level keys via KapiProject.Extras and round-trip them
	// through coreproj.Save unchanged. This is the contract that lets a
	// kapi binary without the bowrain plugin load and re-save a bowrain
	// recipe without dropping bowrain-specific blocks.
	//
	// (When loaded via the bowrain Recipe wrapper, yaml.v3's inline-
	// catchall propagation through embedded structs is limited — typed
	// fields on Recipe consume their keys but unknowns aren't promoted
	// into KapiProject.Extras. That's acceptable because the bowrain
	// loader is the authoritative path for bowrain recipes; round-trip
	// of arbitrary future-tomorrow keys is the framework loader's job.)
	dir := t.TempDir()
	path := filepath.Join(dir, "test.kapi")
	src := `version: v1
name: my-app
server:
  url: https://bowrain.example.com/team/proj
some_future_thing:
  alpha: 1
  beta: two
`
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))

	p, err := coreproj.Load(path)
	require.NoError(t, err)
	require.NotNil(t, p.Extras)
	_, present := p.Extras["some_future_thing"]
	assert.True(t, present, "unknown key must be captured in framework Extras")
	_, hasServer := p.Extras["server"]
	assert.True(t, hasServer, "bowrain extension keys captured in framework Extras")

	// Save and reload — the unknown key must survive.
	out := filepath.Join(dir, "out.kapi")
	require.NoError(t, coreproj.Save(out, p))

	raw, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(raw), "some_future_thing"), "saved YAML must contain unknown key")
	assert.True(t, strings.Contains(string(raw), "alpha"), "nested fields preserved")
	assert.True(t, strings.Contains(string(raw), "server"), "bowrain extension preserved")
}
