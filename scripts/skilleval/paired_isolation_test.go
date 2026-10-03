package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

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
	cmd := exec.Command(filepath.Join(state, "bin", "kapi"), "inspect", "page with spaces.json", "--render", "html")
	cmd.Env = []string{"KAPI_NO_PROJECT=1", "KAPI_PROJECT=/unrelated/kapi.yaml"}
	output, err := cmd.CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(workspace, "kapi.yaml")+"\n\ninspect\npage with spaces.json\n--render\nhtml\n", string(output))
}

func TestPairedCLIWrapperWithBuiltKapi(t *testing.T) {
	binary := os.Getenv("PAIRED_TEST_KAPI")
	if binary == "" {
		t.Skip("set PAIRED_TEST_KAPI to a built kapi binary for the offline integration check")
	}
	binary, err := filepath.Abs(binary)
	require.NoError(t, err)
	task, err := findPairedTask("add-json-key")
	require.NoError(t, err)
	launch := PairedLaunch{
		Workspace: t.TempDir(), StateDir: t.TempDir(), Condition: "skill-cli", KapiBin: binary,
	}
	require.NoError(t, materializePairedTask(launch.Workspace, task))
	require.NoError(t, readPairedContext(t.Context(), launch.Workspace, binary))
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
	guide := run("context", "i18n/de/messages.json")
	assert.Contains(t, string(guide), "Harbor Help")
	blocks := run("inspect", "i18n/de/messages.json", "--jsonl")
	lines := strings.Split(strings.TrimSpace(string(blocks)), "\n")
	require.Len(t, lines, 9)
	var changes strings.Builder
	for _, line := range lines {
		var block map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &block))
		assert.NotEmpty(t, block["ref"])
		assert.NotEmpty(t, block["rev"])
		// Each block's own text, guarded by the revision the read printed:
		// an operation that changes nothing.
		change, err := json.Marshal(map[string]any{"op": "set_content", "at": block["ref"], "if_match": block["rev"], "text": block["text"]})
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
	page, err := os.ReadFile(filepath.Join(launch.Workspace, "i18n/de/messages.json"))
	require.NoError(t, err)
	files, err := pairedTaskFiles(task)
	require.NoError(t, err)
	assert.Equal(t, string(files["i18n/de/messages.json"]), string(page))
}

func TestPairedClaudeConfiguration(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "private-token")
	for _, condition := range pairedConditions {
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
			arm, err := pairedArmFor(condition)
			require.NoError(t, err)
			assert.Equal(t, arm.Skill == "", strings.Contains(strings.Join(prepared.Args, " "), "--disable-slash-commands"))
			for i, arg := range prepared.Args {
				if arg == "--setting-sources" {
					assert.Equal(t, "project", prepared.Args[i+1], "only the workspace's own settings are read")
				}
			}
			assert.Contains(t, prepared.Args, "--strict-mcp-config")
			settings, err := os.ReadFile(filepath.Join(state, "claude-settings.json"))
			require.NoError(t, err)
			assert.Contains(t, string(settings), `"allowUnsandboxedCommands": false`)
			assert.NotContains(t, string(settings), "private-token")
			// Claude Code's own skills are off in every arm.
			project, err := os.ReadFile(filepath.Join(workspace, ".claude", "settings.json"))
			require.NoError(t, err)
			for _, skill := range pairedClaudeHostSkills {
				assert.Contains(t, string(project), `"`+skill+`": "off"`)
			}
		})
	}
}

// Each cell has a temporary directory of its own, short enough for Claude
// Code to keep its sockets in, which both hosts and the agent's tools use.
// The shared /tmp/claude and the developer's /tmp/claude-<uid> stay out of
// reach of Claude's shell.
func TestPairedCellHasItsOwnTemporaryDirectory(t *testing.T) {
	tmp, err := makePairedCellTmp()
	require.NoError(t, err)
	defer os.RemoveAll(tmp)
	assert.LessOrEqual(t, len(filepath.Join(pairedResolve(tmp), "claude-"+strconv.Itoa(os.Getuid()))), pairedCellTmpLimit)
	info, err := os.Stat(tmp)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())

	state, workspace := t.TempDir(), t.TempDir()
	launch := PairedLaunch{Agent: PairedAgentSpec{Host: "claude", Model: "m", Effort: "high"}, Condition: "baseline",
		StateDir: state, Workspace: workspace, TmpDir: tmp}
	env := pairedEnvironment(launch)
	assert.Contains(t, env, "TMPDIR="+tmp)
	assert.Contains(t, env, "CLAUDE_CODE_TMPDIR="+tmp)
	assert.Contains(t, env, "CLAUDE_CODE_DISABLE_BUNDLED_SKILLS=1")

	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "private-token")
	prepared := PairedPrepared{Args: []string{}, Env: env, Launch: launch}
	require.NoError(t, preparePairedClaude(context.Background(), &prepared))
	data, err := os.ReadFile(filepath.Join(state, "claude-settings.json"))
	require.NoError(t, err)
	settings := map[string]any{}
	require.NoError(t, json.Unmarshal(data, &settings))
	filesystem := pairedObject(pairedObject(settings, "sandbox"), "filesystem")
	assert.Equal(t, []string{tmp}, pairedStringList(filesystem["allowWrite"]))
	denied := pairedStringList(filesystem["denyRead"])
	for _, shared := range pairedSharedTemp() {
		assert.Contains(t, denied, shared)
	}

	// When the session ends, what the agent left there moves into the cell.
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "edits.json"), []byte("{}"), 0o600))
	kept := settlePairedCellTmp(launch)
	assert.Equal(t, filepath.Join(state, "tmp"), kept)
	_, err = os.Stat(filepath.Join(kept, "edits.json"))
	require.NoError(t, err)
	_, err = os.Stat(tmp)
	assert.True(t, os.IsNotExist(err))
}

// Codex's sandbox writes only to the workspace and the cell's own TMPDIR:
// its workspace-write mode would otherwise add /tmp, which every session
// shares.
func TestPairedCodexSandboxExcludesSharedTmp(t *testing.T) {
	state, workspace := t.TempDir(), t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(state, "codex"), 0o700))
	prepared := PairedPrepared{Executable: "/nonexistent/codex", Launch: PairedLaunch{
		Agent: PairedAgentSpec{Host: "codex", Model: "m", Effort: "medium"}, Condition: "baseline",
		StateDir: state, Workspace: workspace, TmpDir: "/tmp/kpe-test"}}
	t.Setenv("CODEX_HOME", t.TempDir())
	require.NoError(t, preparePairedCodex(context.Background(), &prepared))
	config, err := os.ReadFile(filepath.Join(state, "codex", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(config), "[sandbox_workspace_write]\nexclude_slash_tmp = true\nexclude_tmpdir_env_var = false\n")
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
	for _, condition := range pairedConditions {
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
	for _, condition := range pairedConditions {
		t.Run(condition, func(t *testing.T) {
			state := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(state, "bin"), 0o700))
			require.NoError(t, pairedToolPath(PairedLaunch{StateDir: state, Condition: condition, KapiBin: "/test/kapi"}))
			arm, err := pairedArmFor(condition)
			require.NoError(t, err)
			assert.Equal(t, arm.Executables, pairedCellExecutables(filepath.Join(state, "bin")))
			if condition == "project-free" {
				// The alias is the binary itself under another name.
				target, err := os.Readlink(filepath.Join(state, "bin", pairedFilesAlias))
				require.NoError(t, err)
				assert.Equal(t, "/test/kapi", target)
			}
			_, err = os.Lstat(filepath.Join(state, "bin", "cat"))
			require.NoError(t, err)
		})
	}
}

func TestPairedClaudeCredentialRejectsKnownExpiredToken(t *testing.T) {
	now := time.Date(2026, time.September, 10, 21, 7, 0, 0, time.UTC)
	need := 10*time.Minute + pairedTokenMargin
	for _, expiry := range []int64{now.Add(-time.Hour).UnixMilli(), now.UnixMilli()} {
		raw, err := json.Marshal(map[string]any{"claudeAiOauth": map[string]any{"accessToken": "private-test-token", "expiresAt": expiry}})
		require.NoError(t, err)
		token, err := pairedClaudeCredentialToken(raw, now, need)
		require.ErrorContains(t, err, "has expired")
		assert.Empty(t, token)
		assert.NotContains(t, err.Error(), "private-test-token")
	}
	for _, expiry := range []any{now.Add(time.Hour).UnixMilli(), nil, "unknown"} {
		raw, err := json.Marshal(map[string]any{"claudeAiOauth": map[string]any{"accessToken": "private-test-token", "expiresAt": expiry}})
		require.NoError(t, err)
		token, err := pairedClaudeCredentialToken(raw, now, need)
		require.NoError(t, err)
		assert.Equal(t, "private-test-token", token)
	}
}

// A token that would expire during a session is refused before the session
// starts: the cell gets it as a fixed value it cannot refresh, so it would
// fail partway, and that failure would read as the agent's.
func TestPairedClaudeCredentialNeedsTheSessionAndAMargin(t *testing.T) {
	now := time.Date(2026, time.October, 3, 12, 0, 0, 0, time.UTC)
	need := 10*time.Minute + pairedTokenMargin
	short, err := json.Marshal(map[string]any{"claudeAiOauth": map[string]any{
		"accessToken": "private-test-token", "expiresAt": now.Add(20 * time.Minute).UnixMilli()}})
	require.NoError(t, err)
	_, err = pairedClaudeCredentialToken(short, now, need)
	require.ErrorContains(t, err, "expires in 20m0s, and a session needs 25m0s")
	assert.Contains(t, err.Error(), "claude setup-token")
	long, err := json.Marshal(map[string]any{"claudeAiOauth": map[string]any{
		"accessToken": "private-test-token", "expiresAt": now.Add(26 * time.Minute).UnixMilli()}})
	require.NoError(t, err)
	token, err := pairedClaudeCredentialToken(long, now, need)
	require.NoError(t, err)
	assert.Equal(t, "private-test-token", token)
}

func TestPairedClaudeOpaqueEnvironmentTokenHasNoInferredExpiry(t *testing.T) {
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "opaque-test-token")
	token, err := pairedClaudeSubscriptionToken(t.Context(), time.Hour)
	require.NoError(t, err)
	assert.Equal(t, "opaque-test-token", token)
}
