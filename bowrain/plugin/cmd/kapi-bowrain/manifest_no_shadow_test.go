package main

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The no-shadowing rule (strategy: 2026-07-product-surface, Option A):
// installing this plugin must never change what an existing kapi verb means.
// A manifest command may collide with a built-in only when that is a
// deliberate group-scoped surface (reachable as `kapi bowrain <verb>`, with
// the built-in keeping the top-level verb), and dispatch plumbing consumed
// by built-ins must be hidden. This test makes any new collision a conscious
// decision instead of a silent override.
func TestManifestNeverShadowsBuiltins(t *testing.T) {
	data, err := os.ReadFile("manifest.json")
	require.NoError(t, err)
	m, err := manifest.Parse(data)
	require.NoError(t, err)

	builtins := map[string]bool{}
	for _, c := range cli.KapiCommandSet(app) {
		builtins[c.Name()] = true
	}
	require.True(t, builtins["up"], "sanity: the built-in command set should be populated")

	// Verbs deliberately group-scoped: the built-in keeps the top level and
	// the plugin's version lives under `kapi bowrain <verb>`. Grow this list
	// only with a matching docs update.
	groupScoped := map[string]bool{
		"config": true, // built-in: app config (+ positional recipe keys); plugin: --global bowrain.yaml
	}

	for _, c := range m.Capabilities.Commands {
		if c.Hidden {
			assert.False(t, builtins[c.Name],
				"hidden plumbing %q must not reuse a built-in verb — pick a distinct name (server-*)", c.Name)
			continue
		}
		if builtins[c.Name] {
			assert.True(t, groupScoped[c.Name],
				"command %q collides with a built-in kapi verb without being on the acknowledged group-scoped list — plugins extend kapi, they never override it", c.Name)
		}
	}

	// The plumbing consumed by built-ins must stay declared and hidden:
	// kapi status shells server-status, kapi up dispatches server-up.
	plumbing := map[string]bool{}
	for _, c := range m.Capabilities.Commands {
		if c.Hidden {
			plumbing[c.Name] = true
		}
	}
	assert.True(t, plumbing["server-status"], "kapi status depends on the hidden server-status plumbing")
	assert.True(t, plumbing["server-up"], "kapi up's server venue depends on the hidden server-up plumbing")
	assert.True(t, plumbing["server-ls"], "kapi ls's sync column depends on the hidden server-ls plumbing")
}

// kapi carries a compiled-in hint table (host.PluginHint) naming the verbs this
// plugin provides, so that typing one of them without the plugin installed
// yields an install offer instead of cobra's bare "unknown command". kapi
// cannot read this manifest when the plugin is absent — that is the whole
// point — so the table is hand-written, and this test is what keeps it honest.
//
// The table must hold exactly the verbs that would surface as unknown: visible
// commands that no built-in already owns. Hidden plumbing never gets typed, and
// a group-scoped verb (config) resolves to the built-in, so neither belongs.
func TestPluginHintMatchesManifestVerbs(t *testing.T) {
	data, err := os.ReadFile("manifest.json")
	require.NoError(t, err)
	m, err := manifest.Parse(data)
	require.NoError(t, err)

	builtins := map[string]bool{}
	for _, c := range cli.KapiCommandSet(app) {
		builtins[c.Name()] = true
	}
	require.True(t, builtins["up"], "sanity: the built-in command set should be populated")

	var want []string
	for _, c := range m.Capabilities.Commands {
		if c.Hidden || builtins[c.Name] {
			continue
		}
		want = append(want, c.Name)
	}
	require.NotEmpty(t, want, "sanity: the plugin should contribute at least one top-level verb")

	hint, ok := host.PluginHintFor("bowrain")
	require.True(t, ok, "kapi must carry a plugin hint for bowrain")
	assert.ElementsMatch(t, want, hint.Verbs,
		"host.pluginHints for bowrain has drifted from manifest.json — update the table in host/missingplugin.go")
	assert.Equal(t, "server", hint.RecipeKey,
		"the hint's recipe key must be the top-level block this plugin's schema extension owns")

	// Every declared schema extension group belongs to this plugin, so the
	// hinted key must really be one of them.
	var groups []string
	for _, ext := range m.Capabilities.SchemaExtensions {
		groups = append(groups, ext.Name)
	}
	assert.Contains(t, groups, hint.RecipeKey)
}
