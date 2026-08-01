package conformance

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPluginEnvDropsProviderKeys: the conformance harness spawns a plugin the
// same way the host does, and its whole value is that "passes conformance"
// predicts "behaves under kapi". The host stops handing plugins the user's
// model credentials (host/pluginhost.WithoutProviderKeys); a harness that still
// hands them over is testing a launch that no longer happens, and would give a
// plugin reading ANTHROPIC_API_KEY a clean bill of health.
func TestPluginEnvDropsProviderKeys(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-should-not-travel")
	t.Setenv("OPENAI_API_KEY", "sk-should-not-travel")
	t.Setenv("PATH", "/usr/bin")

	r := &runner{suite: Suite{Dir: t.TempDir()}}
	env := r.pluginEnv()

	assert.NotContains(t, envNames(env), "ANTHROPIC_API_KEY",
		"a provider key must not reach a plugin the harness spawns")
	assert.NotContains(t, envNames(env), "OPENAI_API_KEY")

	// The scrub is a denylist: a plugin is a local compute engine that needs
	// the rest of the environment, and the protocol variables besides.
	assert.Contains(t, env, "PATH=/usr/bin", "ordinary environment still travels")
	require.Contains(t, env, "KAPI_PLUGIN_DIR="+r.suite.Dir)
}

// envNames reduces an environment to its variable names, so a failing assertion
// prints the handful of names at issue rather than the whole environment (and
// the values in it).
func envNames(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		out = append(out, name)
	}
	return out
}

// TestPluginEnvKeepsTheSuitesOwnAdditions: Suite.Env is the caller's explicit
// grant — a plugin under test may legitimately need a key its manifest declares
// — so it is added after the scrub and still wins on a duplicate name.
func TestPluginEnvKeepsTheSuitesOwnAdditions(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-from-the-host")

	r := &runner{suite: Suite{Dir: t.TempDir(), Env: []string{"ANTHROPIC_API_KEY=sk-on-purpose"}}}
	env := r.pluginEnv()

	assert.Equal(t, "ANTHROPIC_API_KEY=sk-on-purpose", env[len(env)-1],
		"what the caller adds by name is a decision, not an inherited leak")
	assert.Equal(t, len(env)-1, slices.Index(envNames(env), "ANTHROPIC_API_KEY"),
		"the inherited value is gone, so the caller's is the only one left")
}
