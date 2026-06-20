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
// These tests deliberately do not call ResetExtensionsForTest — the
// init() registration is the system under test.

func TestSchemaPackage_RegistersBowrainGroup(t *testing.T) {
	assert.True(t, coreproj.HasExtensionGroup(Group), "bowrain group must be registered after import")
}

func decode(t *testing.T, src string) yaml.Node {
	t.Helper()
	var n yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(src), &n))
	require.Greater(t, len(n.Content), 0, "expected non-empty document")
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

func TestHooksDecoder_Valid(t *testing.T) {
	n := decode(t, `
pre-push: [qa]
post-pull: [update-stats]
`)
	assert.NoError(t, hooksDecoder.Decode(n))
}

func TestHooksDecoder_RejectsUnknownTrigger(t *testing.T) {
	n := decode(t, `
weird-trigger: [some-flow]
`)
	err := hooksDecoder.Decode(n)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "weird-trigger")
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

func TestBrandVoiceDecoder_Valid(t *testing.T) {
	n := decode(t, `
profile: company-profile
channel: marketing
collections:
  ui:
    profile: technical
`)
	assert.NoError(t, brandVoiceDecoder.Decode(n))
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
server:
  url: "not a url"
`
	var p coreproj.KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(src), &p))

	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server:")
	assert.Contains(t, err.Error(), "url")
}

func TestRecipeValidate_SurfacesPerItemDecoderError(t *testing.T) {
	src := `
version: v1
content:
  - name: ui
    items:
      - path: src/foo.json
        asset_max_size: [bad, sequence]
`
	var p coreproj.KapiProject
	require.NoError(t, yaml.Unmarshal([]byte(src), &p))

	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content[0].items[0].asset_max_size")
}

func TestRecipeValidate_AcceptsValidBowrainRecipe(t *testing.T) {
	src := `
version: v1
name: my-app
defaults:
  source_language: en-US
  target_languages: [fr-FR, de-DE]
  collection: ui-strings
content:
  - path: src/locales/**/*.json
    format: json
    base: src/
    collection: app-strings
plugins:
  okapi-bridge: "^1.47.0"
server:
  url: https://bowrain.example.com/my-team/abc123
  stream: $auto
hooks:
  pre-push: [qa]
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
