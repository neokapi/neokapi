package schema

import (
	"testing"

	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// The schema package's init() is the registration under test. Tests
// here verify that:
//   - the bowrain group is registered
//   - each (scope, key) decoder accepts well-formed YAML
//   - each decoder rejects malformed YAML with a useful error
//   - integration via coreproj.KapiProject.Validate() surfaces decoder
//     errors with scope-aware paths
//
// These tests deliberately do not call projecttest.ResetExtensions — the
// init() registration is the system under test.

func TestSchemaPackage_RegistersBowrainGroup(t *testing.T) {
	assert.True(t, coreproj.HasExtensionGroup(Group), "bowrain group must be registered after import")
}

func decode(t *testing.T, src string) yaml.Node {
	t.Helper()
	var n yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(src), &n))
	require.NotEmpty(t, n.Content, "expected non-empty document")
	return *n.Content[0]
}

func TestServerDecoder_Valid(t *testing.T) {
	n := decode(t, `
url: https://bowrain.example.com/team/proj
stream: $auto
`)
	assert.NoError(t, serverDecoder.Decode(n))
}

func TestServerDecoder_RejectsBadURL(t *testing.T) {
	n := decode(t, `
url: "not a url"
`)
	err := serverDecoder.Decode(n)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "url")
}

func TestServerDecoder_ConvergePolicy(t *testing.T) {
	for _, policy := range []string{"on-push", "manual"} {
		n := decode(t, `
url: https://bowrain.example.com/team/proj
converge: `+policy+`
`)
		assert.NoError(t, serverDecoder.Decode(n), "policy %q must be accepted", policy)
	}
}

func TestServerDecoder_RejectsBadConverge(t *testing.T) {
	n := decode(t, `
url: https://bowrain.example.com/team/proj
converge: sometimes
`)
	err := serverDecoder.Decode(n)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "converge")
}

func TestServerSpec_ResolvedConverge(t *testing.T) {
	assert.Equal(t, ConvergeOnPush, (&ServerSpec{}).ResolvedConverge(), "empty defaults to on-push")
	assert.Equal(t, ConvergeManual, (&ServerSpec{Converge: ConvergeManual}).ResolvedConverge())
	assert.Equal(t, ConvergeOnPush, (*ServerSpec)(nil).ResolvedConverge(), "nil is safe")
}

// A retired top-level key fails the recipe with the replacement named,
// instead of being preserved as an unknown extension that nothing reads.
func TestRetiredProjectKeys_RejectTheRecipe(t *testing.T) {
	require.Contains(t, RetiredProjectKeys, "hooks")
	for key, replacement := range RetiredProjectKeys {
		t.Run(key, func(t *testing.T) {
			group, ok := coreproj.ExtensionRegistered(coreproj.ScopeProject, key)
			require.True(t, ok, "a retired key is registered so the loader refuses it")
			assert.Equal(t, Group, group)

			var p coreproj.KapiProject
			require.NoError(t, yaml.Unmarshal([]byte("version: v1\nname: t\n"+key+":\n  pre-push: [qa]\n"), &p))
			err := p.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), key+": is no longer a recipe key")
			assert.Contains(t, err.Error(), replacement)
		})
	}
}

// CheckRetiredProjectKeys gives a typed loader the same refusal from the raw
// document, and stays silent on a recipe without a retired key or one that
// does not parse.
func TestCheckRetiredProjectKeys(t *testing.T) {
	err := CheckRetiredProjectKeys([]byte("version: v1\nhooks:\n  pre-push: [qa]\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hooks: is no longer a recipe key. Use automations:")

	assert.NoError(t, CheckRetiredProjectKeys([]byte("version: v1\nautomations: []\n")))
	assert.NoError(t, CheckRetiredProjectKeys([]byte("")))
	assert.NoError(t, CheckRetiredProjectKeys([]byte("- not: a mapping\n")))
}

func TestAutomationsDecoder_Valid(t *testing.T) {
	n := decode(t, `
- name: auto-translate
  trigger: post-push
  actions:
    - type: wait_translate
    - type: pull
`)
	assert.NoError(t, automationsDecoder.Decode(n))
}

func TestAutomationsDecoder_RejectsUnknownActionType(t *testing.T) {
	n := decode(t, `
- name: bad-auto
  trigger: post-push
  actions:
    - type: nuke_everything
`)
	err := automationsDecoder.Decode(n)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nuke_everything")
}

func TestAutomationsDecoder_RejectsMissingName(t *testing.T) {
	n := decode(t, `
- trigger: post-push
  actions:
    - type: pull
`)
	err := automationsDecoder.Decode(n)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestAutomationsDecoder_RejectsUnknownTrigger(t *testing.T) {
	n := decode(t, `
- name: my-auto
  trigger: hourly
  actions:
    - type: pull
`)
	err := automationsDecoder.Decode(n)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trigger")
}

func TestAssetsDecoder_Valid(t *testing.T) {
	n := decode(t, `
enabled: true
max_size: 100MB
exclude:
  - "**/*.tmp"
`)
	assert.NoError(t, assetsDecoder.Decode(n))
}

func TestVoiceDecoder_Valid(t *testing.T) {
	n := decode(t, `
profile: company-profile
channel: marketing
collections:
  ui:
    profile: technical
`)
	assert.NoError(t, voiceDecoder.Decode(n))
}

func TestStringDecoder_AcceptsScalarString(t *testing.T) {
	n := decode(t, `"some-value"`)
	assert.NoError(t, stringDecoder.Decode(n))
}

func TestStringDecoder_RejectsSequence(t *testing.T) {
	n := decode(t, `[a, b, c]`)
	err := stringDecoder.Decode(n)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected string")
}

func TestBoolDecoder_RejectsString(t *testing.T) {
	n := decode(t, `"not-a-bool"`)
	err := boolDecoder.Decode(n)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected bool")
}

// ── End-to-end integration with KapiProject.Validate() ────────────────────

func TestRecipeValidate_SurfacesServerDecoderError(t *testing.T) {
	src := `
version: v1
bowrain:
  url: "not a url"
`
	var p coreproj.KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(src), &p))

	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), VenueKey+":")
	assert.Contains(t, err.Error(), "url")
}

func TestRecipeValidate_SurfacesPerItemDecoderError(t *testing.T) {
	src := `
version: v1
collections:
  - name: ui
    content:
      - path: src/foo.json
        asset_max_size: [bad, sequence]
`
	var p coreproj.KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(src), &p))

	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collections[0].content[0].asset_max_size")
}

func TestRecipeValidate_AcceptsValidBowrainRecipe(t *testing.T) {
	src := `
version: v1
name: my-app
defaults:
  source_language: en-US
  target_languages: [fr-FR, de-DE]
  collection: ui-strings
profiles:
  my-app:
    channels: [app]
collections:
  - name: app-strings
    channel: my-app/app
    base: src
    content:
      - path: locales/**/*.json
        format: json
        collection: app-strings
plugins:
  okapi-bridge: "^1.47.0"
bowrain:
  url: https://bowrain.example.com/my-team/abc123
  stream: $auto
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
  profile: company-voice
`
	var p coreproj.KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(src), &p))
	assert.NoError(t, p.Validate())
}

func TestRecipeValidate_RequiresBowrainGroupPasses(t *testing.T) {
	// The group is registered by this package's init(), so a recipe
	// declaring `requires: { bowrain: "*" }` should validate.
	src := `
version: v1
requires:
  bowrain: "*"
`
	var p coreproj.KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(src), &p))
	assert.NoError(t, p.Validate())
}

func TestRecipeValidate_BareListRejected(t *testing.T) {
	src := `
version: v1
requires: [bowrain]
`
	var p coreproj.KapiProject
	err := yaml.Unmarshal([]byte(src), &p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bare-list form is no longer supported")
}
