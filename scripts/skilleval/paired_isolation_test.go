package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPairedEnvironmentExcludesAPIRoutes(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "private-key")
	t.Setenv("OPENAI_API_KEY", "private-key")
	t.Setenv("ANTHROPIC_BASE_URL", "https://example.invalid")
	env := strings.Join(pairedEnvironment(PairedLaunch{Workspace: t.TempDir(), StateDir: t.TempDir()}), "\n")
	assert.NotContains(t, env, "private-key")
	assert.NotContains(t, env, "example.invalid")
	assert.Contains(t, env, "KAPI_NO_PROJECT=1")
	assert.Contains(t, env, "KAPI_PLUGINS_DIR_ONLY=1")
}

func TestPairedClaudeConfiguration(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "private-token")
	for _, condition := range []string{"baseline", "skill-cli", "mcp"} {
		t.Run(condition, func(t *testing.T) {
			state := t.TempDir()
			workspace := t.TempDir()
			prepared := PairedPrepared{Args: []string{}, Env: []string{}, Blockers: []string{}, IsolationNotes: []string{}, Launch: PairedLaunch{Agent: PairedAgentSpec{Host: "claude", Model: "claude-sonnet-5", Effort: "high"}, Condition: condition, StateDir: state, Workspace: workspace, KapiBin: "/test/kapi", MaxTurns: 40}}
			require.NoError(t, preparePairedClaude(context.Background(), &prepared))
			assert.Empty(t, prepared.Blockers)
			assert.NotContains(t, strings.Join(prepared.Args, " "), "private-token")
			data, err := os.ReadFile(filepath.Join(state, "claude-mcp.json"))
			require.NoError(t, err)
			config := map[string]any{}
			require.NoError(t, json.Unmarshal(data, &config))
			servers := pairedObject(config, "mcpServers")
			if condition == "mcp" {
				assert.Len(t, servers, 1)
			} else {
				assert.Empty(t, servers)
			}
			assert.Equal(t, condition != "skill-cli", strings.Contains(strings.Join(prepared.Args, " "), "--disable-slash-commands"))
			settings, err := os.ReadFile(filepath.Join(state, "claude-settings.json"))
			require.NoError(t, err)
			assert.Contains(t, string(settings), `"allowUnsandboxedCommands": false`)
			assert.NotContains(t, string(settings), "private-token")
		})
	}
}

func TestPairedToolPathHasOnlyAssignedCLI(t *testing.T) {
	for _, condition := range []string{"baseline", "skill-cli", "mcp"} {
		t.Run(condition, func(t *testing.T) {
			state := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(state, "bin"), 0o700))
			require.NoError(t, pairedToolPath(PairedLaunch{StateDir: state, Condition: condition, KapiBin: "/test/kapi"}))
			_, err := os.Lstat(filepath.Join(state, "bin", "kapi"))
			if condition == "skill-cli" {
				require.NoError(t, err)
			} else {
				assert.True(t, os.IsNotExist(err))
			}
			_, err = os.Lstat(filepath.Join(state, "bin", "cat"))
			require.NoError(t, err)
		})
	}
}
