package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func TestPairedCLIWrapperPreservesArgumentsAndBindsFixture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("wrapper uses a Unix shell")
	}
	state := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "fixture with 'quoted' name")
	binary := filepath.Join(state, "fake-kapi")
	require.NoError(t, os.Mkdir(filepath.Join(state, "bin"), 0o700))
	script := "#!/bin/sh\nprintf '%s\\n' \"$KAPI_PROJECT\" \"$KAPI_NO_PROJECT\" \"$@\"\n"
	require.NoError(t, os.WriteFile(binary, []byte(script), 0o700))
	require.NoError(t, pairedToolPath(PairedLaunch{
		StateDir: state, Workspace: workspace, Condition: "skill-cli", KapiBin: binary,
	}))
	cmd := exec.Command(filepath.Join(state, "bin", "kapi"), "inspect", "page with spaces.json", "--project", "html")
	cmd.Env = []string{"KAPI_NO_PROJECT=1", "KAPI_PROJECT=/unrelated/kapi.yaml"}
	output, err := cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(workspace, "kapi.yaml")+"\n\ninspect\npage with spaces.json\n--project\nhtml\n", string(output))
}

func TestPairedCLIWrapperWithBuiltKapi(t *testing.T) {
	binary := os.Getenv("PAIRED_TEST_KAPI")
	if binary == "" {
		t.Skip("set PAIRED_TEST_KAPI to a built kapi binary for the offline integration check")
	}
	binary, err := filepath.Abs(binary)
	require.NoError(t, err)
	task, err := findPairedTask("audience-child")
	require.NoError(t, err)
	launch := PairedLaunch{
		Workspace: t.TempDir(), StateDir: t.TempDir(), Condition: "skill-cli", KapiBin: binary,
	}
	require.NoError(t, materializePairedTask(launch.Workspace, task))
	require.NoError(t, os.Mkdir(filepath.Join(launch.StateDir, "bin"), 0o700))
	require.NoError(t, pairedToolPath(launch))
	run := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command(filepath.Join(launch.StateDir, "bin", "kapi"), args...)
		cmd.Dir, cmd.Env = launch.Workspace, pairedEnvironment(launch)
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
		return output
	}
	guide := run("context", "content/en/page.json")
	assert.Contains(t, string(guide), "harbor-help/child")
	assert.Contains(t, string(guide), "trusted adult")
	blocks := run("inspect", "content/en/page.json", "--jsonl")
	lines := strings.Split(strings.TrimSpace(string(blocks)), "\n")
	require.Len(t, lines, 4)
	var changes strings.Builder
	for _, line := range lines {
		var block map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &block))
		assert.NotEmpty(t, block["content_hash"])
		block["kind"] = "content"
		change, err := json.Marshal(block)
		require.NoError(t, err)
		changes.Write(change)
		changes.WriteByte('\n')
	}
	// A no-op apply checks that format-aware read/write commands accept the same
	// environment binding as context retrieval, without altering content.
	edits := filepath.Join(launch.Workspace, "edits.jsonl")
	require.NoError(t, os.WriteFile(edits, []byte(changes.String()), 0o600))
	run("apply", edits)
	run("version")
	page, err := os.ReadFile(filepath.Join(launch.Workspace, "content/en/page.json"))
	require.NoError(t, err)
	files, err := pairedTaskFiles(task)
	require.NoError(t, err)
	assert.JSONEq(t, string(files["content/en/page.json"]), string(page))
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
			for i, arg := range prepared.Args {
				if arg == "--setting-sources" {
					want := ""
					if condition == "skill-cli" {
						want = "project"
					}
					assert.Equal(t, want, prepared.Args[i+1])
				}
			}
			settings, err := os.ReadFile(filepath.Join(state, "claude-settings.json"))
			require.NoError(t, err)
			assert.Contains(t, string(settings), `"allowUnsandboxedCommands": false`)
			assert.NotContains(t, string(settings), "private-token")
		})
	}
}

func TestPairedCodexMCPApprovalAppliesOnlyToFixtureServer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture executable uses a Unix shell")
	}
	authDir := t.TempDir()
	t.Setenv("CODEX_HOME", authDir)
	require.NoError(t, os.WriteFile(filepath.Join(authDir, "auth.json"), []byte("{}"), 0o600))
	probe := filepath.Join(t.TempDir(), "subscription-probe")
	require.NoError(t, os.WriteFile(probe, []byte("#!/bin/sh\nprintf 'Logged in using ChatGPT\\n'\n"), 0o700))
	for _, condition := range []string{"baseline", "skill-cli", "mcp"} {
		state := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(state, "codex"), 0o700))
		p := PairedPrepared{
			Executable: probe, Env: []string{}, Blockers: []string{}, IsolationNotes: []string{},
			Launch: PairedLaunch{
				Agent:     PairedAgentSpec{Host: "codex", Model: "fixture", Effort: "medium"},
				Condition: condition, StateDir: state, Workspace: t.TempDir(), KapiBin: "/test/kapi",
			},
		}
		require.NoError(t, preparePairedCodex(t.Context(), &p))
		data, err := os.ReadFile(filepath.Join(state, "codex", "config.toml"))
		require.NoError(t, err)
		assert.Contains(t, string(data), "approval_policy = \"never\"")
		if condition == "mcp" {
			assert.Contains(t, string(data), "[mcp_servers.kapi]\ndefault_tools_approval_mode = \"approve\"")
		} else {
			assert.NotContains(t, string(data), "default_tools_approval_mode")
		}
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
