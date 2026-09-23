package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every kapi this harness launches, and every agent host that launches one,
// runs under the whole isolation contract in CLAUDE.md. KAPI_DATA_DIR is the
// variable easiest to miss: it names the data root outright and wins over
// XDG_DATA_HOME, so an environment that sets only the latter hands a kapi the
// developer's exported KAPI_DATA_DIR, and with it their own terms, voice
// profiles, content memory and decisions.

// developerDataDir is what a developer's shell exports. No launch may pass it
// on.
const developerDataDir = "/developer/kapi-data"

// contractVars are the isolation variables every launch sets to a throwaway
// root.
var contractVars = []string{"KAPI_DATA_DIR", "KAPI_CONFIG_DIR", "XDG_DATA_HOME", "XDG_CACHE_HOME"}

// requireIsolated fails unless env sets every contract variable, each under
// root, and KAPI_PLUGINS_DIR_ONLY=1.
func requireIsolated(t *testing.T, env map[string]string, root string) {
	t.Helper()
	for _, name := range contractVars {
		value, ok := env[name]
		require.True(t, ok, "the launch sets no %s", name)
		assert.NotEqual(t, developerDataDir, value, "%s is the developer's", name)
		assert.True(t, strings.HasPrefix(value, root+string(filepath.Separator)),
			"%s=%s sits outside the launch's throwaway root %s", name, value, root)
	}
	assert.Equal(t, "1", env["KAPI_PLUGINS_DIR_ONLY"])
}

// envMap reads NAME=value pairs, the last assignment of a name winning, as it
// does for a process environment built by appending.
func envMap(pairs []string) map[string]string {
	out := map[string]string{}
	for _, pair := range pairs {
		if name, value, ok := strings.Cut(pair, "="); ok {
			out[name] = value
		}
	}
	return out
}

// fakeKapi writes an executable that records the environment it was started
// with, one NAME=value per line, to dump.
func fakeKapi(t *testing.T, dump string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake kapi is a Unix shell script")
	}
	bin := filepath.Join(t.TempDir(), "kapi")
	script := "#!/bin/sh\nenv > '" + dump + "'\nenv\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	return bin
}

func readDump(t *testing.T, dump string) map[string]string {
	t.Helper()
	body, err := os.ReadFile(dump)
	require.NoError(t, err, "the launch never started the command")
	return envMap(strings.Split(strings.TrimSpace(string(body)), "\n"))
}

// The environments the scenario, paired and evaluation runs hand their
// processes: the agent host, the kapi it starts over MCP, and the kapi a gate
// or an agent runs from a shell.
func TestLaunchEnvironmentsCarryTheIsolationContract(t *testing.T) {
	t.Setenv("KAPI_DATA_DIR", developerDataDir)

	t.Run("a scenario run and its gate", func(t *testing.T) {
		home := t.TempDir()
		requireIsolated(t, envMap(append(agentEnv(), isolationEnv(home)...)), home)
	})

	t.Run("the kapi a scenario starts over MCP", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, writeMCPConfig(dir, "/bin/kapi"))
		body, err := os.ReadFile(filepath.Join(dir, mcpConfigName))
		require.NoError(t, err)
		var cfg struct {
			MCPServers map[string]struct {
				Env map[string]string `json:"env"`
			} `json:"mcpServers"`
		}
		require.NoError(t, json.Unmarshal(body, &cfg))
		requireIsolated(t, cfg.MCPServers["kapi"].Env, dir)
	})

	t.Run("a paired study's agent and the kapi it starts", func(t *testing.T) {
		launch := PairedLaunch{Workspace: t.TempDir(), StateDir: t.TempDir()}
		requireIsolated(t, envMap(pairedEnvironment(launch)), launch.Workspace)
		requireIsolated(t, pairedKapiEnv(launch.Workspace), launch.Workspace)
	})

	t.Run("an evaluation cell", func(t *testing.T) {
		root := t.TempDir()
		requireIsolated(t, envMap(evalEnv(evalPaths(root))), root)
	})
}

// The same property asserted on processes actually started: a fake kapi
// records the environment each launch hands it.
func TestLaunchedKapiRunsIsolated(t *testing.T) {
	t.Setenv("KAPI_DATA_DIR", developerDataDir)

	t.Run("the paired study's context import", func(t *testing.T) {
		dump := filepath.Join(t.TempDir(), "env")
		workspace := t.TempDir()
		require.NoError(t, readPairedContext(context.Background(), workspace, fakeKapi(t, dump)))
		requireIsolated(t, readDump(t, dump), workspace)
	})

	t.Run("a scenario's completion gate", func(t *testing.T) {
		dump := filepath.Join(t.TempDir(), "env")
		kapi := fakeKapi(t, dump)
		dir := t.TempDir()
		res := runGate(context.Background(), dir, "kapi --version", Options{KapiBin: kapi})
		require.NotNil(t, res)
		require.Zero(t, res.ExitCode, res.Output)
		requireIsolated(t, readDump(t, dump), dir)
	})

	t.Run("the version probe a report records", func(t *testing.T) {
		dump := filepath.Join(t.TempDir(), "env")
		out := kapiProbe(context.Background(), fakeKapi(t, dump), "--version")
		require.NotEmpty(t, out)
		env := readDump(t, dump)
		require.Contains(t, env, "KAPI_DATA_DIR")
		requireIsolated(t, env, filepath.Dir(env["KAPI_DATA_DIR"]))
		assert.Equal(t, "1", env["KAPI_NO_PROJECT"], "a probe from this checkout must not bind the dogfood recipe")
	})
}
