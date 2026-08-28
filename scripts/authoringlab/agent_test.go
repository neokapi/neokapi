package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAgentEnvDropsTheDevelopersSecrets.
//
// The agent runs with bypassPermissions over a third-party source tree that
// fetch-lab-repo.sh cloned from an overridable URL. Prose in someone else's
// README is untrusted input to a model with a shell, and passing os.Environ()
// gave that shell every credential on the machine.
//
// This is the narrowing, not a sandbox: the agent still has a shell and a
// network, which is what issue #2243 is about.
func TestAgentEnvDropsTheDevelopersSecrets(t *testing.T) {
	for _, secret := range []string{
		"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY",
		"GITHUB_TOKEN", "GH_TOKEN",
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
		"CDN_BUCKET", "NPM_TOKEN",
	} {
		t.Setenv(secret, "leaked-"+secret)
	}
	env := strings.Join(agentEnv(), "\n")
	assert.NotContains(t, env, "leaked-", "no credential from the environment reaches the agent")
}

// TestAgentEnvKeepsWhatTheCLINeeds: HOME especially, because the CLI
// authenticates from ~/.claude and a run without it fails on every cell rather
// than on one.
func TestAgentEnvKeepsWhatTheCLINeeds(t *testing.T) {
	t.Setenv("HOME", "/home/someone")
	t.Setenv("PATH", "/usr/bin")
	t.Setenv("LANG", "en_US.UTF-8")

	got := map[string]string{}
	for _, kv := range agentEnv() {
		k, v, ok := strings.Cut(kv, "=")
		if ok {
			got[k] = v
		}
	}
	assert.Equal(t, "/home/someone", got["HOME"])
	assert.Equal(t, "/usr/bin", got["PATH"])
	assert.Equal(t, "en_US.UTF-8", got["LANG"])
}

// TestIsolationEnvHonoursTheContract: the in-repo contract, so an agent driven
// here cannot bind to the dogfood project or read the developer's plugins.
// Copied in shape from the skill eval's own test, because the contract is the
// repository's rather than either harness's.
func TestIsolationEnvHonoursTheContract(t *testing.T) {
	env := strings.Join(isolationEnv("/tmp/home"), "\n")
	for _, want := range []string{
		"KAPI_NO_PROJECT=1",
		"KAPI_PLUGINS_DIR_ONLY=1",
		"KAPI_CONFIG_DIR=/tmp/home/",
		"XDG_DATA_HOME=/tmp/home/",
		"XDG_CACHE_HOME=/tmp/home/",
	} {
		assert.Contains(t, env, want)
	}
}
