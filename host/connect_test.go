package host

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/agentrules"
)

// connectIn runs ConnectHosts over dir with no project resolved from the
// environment, so dir is the project.
func connectIn(t *testing.T, app *App, dir string, opts ConnectOptions) *ConnectResult {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "1")
	opts.Dir = dir
	res, err := app.ConnectHosts(t.Context(), NewEnvCommand(t.Context(), "connect"), opts)
	require.NoError(t, err)
	return res
}

// isolateClaudeSettings points the person's Claude Code settings at an empty
// folder, so a plugin installed on the machine running the tests is not read.
func isolateClaudeSettings(t *testing.T) {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())
}

// connectFolder is an empty folder, its symlinks resolved so the paths a
// result reports compare equal to it. The person's Claude Code settings are
// isolated with it.
func connectFolder(t *testing.T, name string) string {
	t.Helper()
	isolateClaudeSettings(t)
	parent, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dir := filepath.Join(parent, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	return dir
}

// actions maps each file a result reports to what happened to it, the hook
// under "hook".
func actions(res *ConnectResult) map[string]ConnectAction {
	out := map[string]ConnectAction{}
	for _, f := range res.Files {
		out[f.Path] = f.Action
	}
	for _, h := range res.Hosts {
		for _, f := range h.Files {
			key := f.Path
			if f.Kind == ConnectHook {
				key = "hook"
			}
			out[key] = f.Action
		}
	}
	return out
}

// snapshot reads every file under root, with its modification time.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		body, rerr := os.ReadFile(path)
		require.NoError(t, rerr)
		info, serr := d.Info()
		require.NoError(t, serr)
		rel, _ := filepath.Rel(root, path)
		out[filepath.ToSlash(rel)] = info.ModTime().Format(time.RFC3339Nano) + "\n" + string(body)
		return nil
	}))
	return out
}

func TestConnectHosts_AFolderWithNoRecipe(t *testing.T) {
	app, _ := contextOpsApp(t)
	dir := connectFolder(t, "site")

	res := connectIn(t, app, dir, ConnectOptions{})
	assert.True(t, res.Default, "nothing in the folder names an assistant, so Claude Code is connected by default")
	assert.Equal(t, filepath.Join(dir, "kapi.yaml"), res.Recipe)
	assert.Equal(t, map[string]ConnectAction{
		"kapi.yaml":           ConnectCreated,
		"AGENTS.md":           ConnectCreated,
		"CLAUDE.md":           ConnectCreated,
		".mcp.json":           ConnectCreated,
		".claude/skills/kapi": ConnectCreated,
		"hook":                ConnectPending,
	}, actions(res))

	require.Len(t, res.Hosts, 4, "the four assistants are listed whether or not they are connected")
	assert.Equal(t, AgentHostClaudeCode, res.Hosts[0].Host)
	assert.True(t, res.Hosts[0].Connected)
	assert.Equal(t, []string{"rules", "MCP server", "skill"}, res.Hosts[0].Reads)
	for _, h := range res.Hosts[1:] {
		assert.False(t, h.Connected, h.Label)
		assert.Contains(t, h.Reason, "folder here", h.Label)
	}

	// The recipe and the rules files are the ones kapi init writes: the same
	// draft, and the same section from the same (empty) context.
	initDir := connectFolder(t, "site")
	initRes, err := InitProject(initDir, InitOptions{})
	require.NoError(t, err)
	_, err = app.WriteRulesFiles(t.Context(), initRes.RecipePath)
	require.NoError(t, err)
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		got, gerr := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, gerr)
		want, werr := os.ReadFile(filepath.Join(initDir, name))
		require.NoError(t, werr)
		assert.Equal(t, string(want), string(got), "%s holds the section kapi init writes", name)
	}
	recipe, err := os.ReadFile(res.Recipe)
	require.NoError(t, err)
	assert.Contains(t, string(recipe), "name: site")
}

func TestConnectHosts_KeepsWhatAPersonWroteInClaudeFile(t *testing.T) {
	app, _ := contextOpsApp(t)
	dir := connectFolder(t, "site")
	mine := "# House notes\n\nRun the tests before you push.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(mine), 0o644))

	res := connectIn(t, app, dir, ConnectOptions{})
	assert.False(t, res.Default, "a CLAUDE.md says Claude Code is in use here")
	assert.Equal(t, ConnectUpdated, actions(res)["CLAUDE.md"])

	doc, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(doc, []byte(mine)), "the person's text comes first, byte for byte")
	rest, removed, err := agentrules.Remove(doc)
	require.NoError(t, err)
	require.True(t, removed, "the file holds kapi's section")
	assert.Equal(t, strings.TrimRight(mine, "\n"), strings.TrimRight(string(rest), "\n"),
		"taking the section out leaves exactly what the person wrote")
}

func TestConnectHosts_ASecondRunRewritesNothing(t *testing.T) {
	app, _ := contextOpsApp(t)
	dir := connectFolder(t, "site")
	connectIn(t, app, dir, ConnectOptions{})
	before := snapshot(t, dir)

	res := connectIn(t, app, dir, ConnectOptions{})
	for path, action := range actions(res) {
		if path == "hook" {
			assert.Equal(t, ConnectPending, action)
			continue
		}
		assert.Equal(t, ConnectKept, action, path)
	}
	assert.Equal(t, before, snapshot(t, dir), "no file is rewritten, so no byte and no modification time changes")
}

func TestConnectHosts_DryRunWritesNothing(t *testing.T) {
	app, _ := contextOpsApp(t)
	dir := connectFolder(t, "site")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Mine\n"), 0o644))
	before := snapshot(t, dir)

	res := connectIn(t, app, dir, ConnectOptions{DryRun: true})
	assert.True(t, res.DryRun)
	assert.Equal(t, map[string]ConnectAction{
		"kapi.yaml":           ConnectCreated,
		"AGENTS.md":           ConnectCreated,
		"CLAUDE.md":           ConnectUpdated,
		".mcp.json":           ConnectCreated,
		".claude/skills/kapi": ConnectCreated,
		"hook":                ConnectPending,
	}, actions(res))
	assert.Equal(t, before, snapshot(t, dir), "a dry run writes nothing")

	var text bytes.Buffer
	require.NoError(t, res.FormatText(&text))
	assert.Contains(t, text.String(), "Claude Code   would connect")
	assert.Contains(t, text.String(), "kapi.yaml            would create")
	assert.Contains(t, text.String(), "CLAUDE.md            would update (Writing rules section)")

	// The dry run over the project the real run then writes says what that
	// run does.
	real := connectIn(t, app, dir, ConnectOptions{})
	assert.Equal(t, actions(res), actions(real))
}

func TestConnectHosts_DryRunOverAProjectSaysWhatTheWriteDoes(t *testing.T) {
	app, root := spacesProject(t)
	before := snapshot(t, root)

	dry := connectIn(t, app, root, ConnectOptions{DryRun: true})
	assert.Equal(t, before, snapshot(t, root), "a dry run writes nothing")
	assert.Equal(t, ConnectKept, actions(dry)["kapi.yaml"])
	assert.Equal(t, ConnectCreated, actions(dry)["help/AGENTS.md"], "a folder whose rules differ gets files of its own")

	wrote := connectIn(t, app, root, ConnectOptions{})
	assert.Equal(t, actions(dry), actions(wrote))
}

func TestConnectHosts_NamedHost(t *testing.T) {
	app, _ := contextOpsApp(t)
	dir := connectFolder(t, "site")

	res := connectIn(t, app, dir, ConnectOptions{Hosts: []AgentHost{AgentHostCodex}})
	require.Len(t, res.Hosts, 1, "a named assistant is the only one listed")
	assert.Equal(t, AgentHostCodex, res.Hosts[0].Host)
	assert.Equal(t, []string{"rules", "MCP server"}, res.Hosts[0].Reads)
	require.Len(t, res.Hosts[0].Files, 1)
	assert.Equal(t, ".codex/config.toml", res.Hosts[0].Files[0].Path)
	assert.Equal(t, ConnectCreated, res.Hosts[0].Files[0].Action)

	_, err := os.Stat(filepath.Join(dir, ".mcp.json"))
	assert.ErrorIs(t, err, fs.ErrNotExist, "naming Codex writes no Claude Code file")
}

func TestConnectHosts_ClaudePluginEnabledInTheProject(t *testing.T) {
	app, _ := contextOpsApp(t)
	dir := connectFolder(t, "site")
	settings, err := json.Marshal(map[string]any{"enabledPlugins": map[string]bool{"kapi@neokapi-plugins": true}})
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude/settings.json"), settings, 0o644))

	res := connectIn(t, app, dir, ConnectOptions{})
	claude := res.Hosts[0]
	hook := claude.Files[len(claude.Files)-1]
	assert.Equal(t, ConnectHook, hook.Kind)
	assert.Equal(t, ConnectKept, hook.Action)
	assert.Equal(t, ".claude/settings.json", hook.Path)
	assert.Contains(t, claude.Reads, "kapi check when it finishes")
}

func TestParseConnectHost(t *testing.T) {
	for name, want := range map[string]AgentHost{
		"claude": AgentHostClaudeCode, "claude-code": AgentHostClaudeCode, "Cursor": AgentHostCursor,
		"vscode": AgentHostVSCode, "codex": AgentHostCodex, "agents": AgentHostAgents,
	} {
		got, err := ParseConnectHost(name)
		require.NoError(t, err, name)
		assert.Equal(t, want, got, name)
	}

	_, err := ParseConnectHost("windsurf")
	require.ErrorIs(t, err, ErrConnectUnsupported)
	assert.Contains(t, err.Error(), "windsurf: not supported yet")
	assert.Contains(t, err.Error(), "claude, cursor, vscode, codex, agents")

	_, err = ParseConnectHost("claude-desktop")
	require.ErrorIs(t, err, ErrConnectUnsupported)
	assert.Contains(t, err.Error(), "Kapi Desktop")
}

func TestWriteAgentWiring_DryRunWritesNothing(t *testing.T) {
	dir := connectFolder(t, "site")
	res, err := WriteAgentWiring(AgentWiringOptions{
		Root:   dir,
		Hosts:  AgentHosts(),
		DryRun: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.Files)
	for _, f := range res.Files {
		assert.Equal(t, AgentWiringCreated, f.Action, f.Path)
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "a dry run creates no file and no folder")
}

// writeSettings writes a Claude Code settings file enabling the kapi plugin.
func writeSettings(t *testing.T, path string) {
	t.Helper()
	settings, err := json.Marshal(map[string]any{"enabledPlugins": map[string]bool{"kapi@neokapi-plugins": true}})
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, settings, 0o644))
}

func TestConnectHosts_ClaudePluginEnabledForTheUser(t *testing.T) {
	app, _ := contextOpsApp(t)

	t.Run("CLAUDE_CONFIG_DIR", func(t *testing.T) {
		dir := connectFolder(t, "site")
		config := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", config)
		writeSettings(t, filepath.Join(config, "settings.json"))
		res := connectIn(t, app, dir, ConnectOptions{DryRun: true})
		hook := res.Hosts[0].Files[len(res.Hosts[0].Files)-1]
		assert.Equal(t, ConnectKept, hook.Action)
		assert.Empty(t, hook.Path, "the person's own file is named by no project path")
		var text bytes.Buffer
		require.NoError(t, res.FormatText(&text))
		assert.Contains(t, text.String(), "Stop hook            kept (enabled for your user)\n")
	})

	t.Run("home", func(t *testing.T) {
		dir := connectFolder(t, "site")
		home := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", "")
		t.Setenv("HOME", home)
		writeSettings(t, filepath.Join(home, ".claude", "settings.json"))
		res := connectIn(t, app, dir, ConnectOptions{DryRun: true})
		hook := res.Hosts[0].Files[len(res.Hosts[0].Files)-1]
		assert.Equal(t, ConnectKept, hook.Action)
		assert.Equal(t, "enabled for your user", hook.Detail)
		assert.Contains(t, res.Hosts[0].Reads, "kapi check when it finishes")
	})
}

func TestConnectHosts_DryRunWithNoRecipeReportsStaleSections(t *testing.T) {
	app, _ := contextOpsApp(t)
	dir := connectFolder(t, "site")
	section := capLineLength(renderRootSection(rulesPlan{}))
	stale := agentrules.StartLine + "\n## Writing rules for docs/\n\n- Space, not \"Workspace\"\n" + agentrules.End + "\n"
	mine := "# Guide notes\n\nKeep this.\n"
	for rel, body := range map[string]string{
		"AGENTS.md":       section,
		"docs/AGENTS.md":  stale,
		"guide/CLAUDE.md": mine + "\n" + stale,
	} {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}
	before := snapshot(t, dir)

	dry := connectIn(t, app, dir, ConnectOptions{DryRun: true})
	assert.Equal(t, before, snapshot(t, dir), "a dry run writes nothing")
	got := actions(dry)
	assert.Equal(t, ConnectKept, got["AGENTS.md"], "a root section that is already current is kept")
	assert.Equal(t, ConnectDeleted, got["docs/AGENTS.md"], "a file holding only a stale section is deleted")
	assert.Equal(t, ConnectRemoved, got["guide/CLAUDE.md"], "a stale section beside a person's text is taken out")

	real := connectIn(t, app, dir, ConnectOptions{})
	assert.Equal(t, got, actions(real), "the dry run says what the real run does")
	_, err := os.Stat(filepath.Join(dir, "docs/AGENTS.md"))
	require.ErrorIs(t, err, fs.ErrNotExist)
	guide, err := os.ReadFile(filepath.Join(dir, "guide/CLAUDE.md"))
	require.NoError(t, err)
	assert.Equal(t, mine, string(guide))
}

func TestConnectHosts_RefusesARecipeItCannotWrite(t *testing.T) {
	app, _ := contextOpsApp(t)
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir := connectFolder(t, "site")

	for _, tt := range []struct {
		name    string
		recipe  string
		message string
	}{
		{name: "another name", recipe: filepath.Join(dir, "custom.yaml"), message: "no recipe at " + filepath.Join(dir, "custom.yaml")},
		{name: "a missing folder", recipe: filepath.Join(dir, "missing", "kapi.yaml"), message: "no folder at " + filepath.Join(dir, "missing")},
	} {
		for _, dry := range []bool{true, false} {
			cmd := NewEnvCommand(t.Context(), "connect")
			cmd.Flags().String("project", tt.recipe, "")
			_, err := app.ConnectHosts(t.Context(), cmd, ConnectOptions{DryRun: dry})
			require.Error(t, err, "%s, dry run %v", tt.name, dry)
			assert.Contains(t, err.Error(), tt.message)
			entries, rerr := os.ReadDir(dir)
			require.NoError(t, rerr)
			assert.Empty(t, entries, "%s, dry run %v: nothing is written", tt.name, dry)
		}
	}
}

func TestConnectHosts_DryRunOverAConnectedProject(t *testing.T) {
	app, _ := contextOpsApp(t)
	dir := connectFolder(t, "site")
	connectIn(t, app, dir, ConnectOptions{})

	res := connectIn(t, app, dir, ConnectOptions{DryRun: true})
	var text bytes.Buffer
	require.NoError(t, res.FormatText(&text))
	assert.Contains(t, text.String(), "Claude Code   connected · nothing to write\n")
	assert.NotContains(t, text.String(), "would")
}

func TestConnectHosts_HostsOnlyReadsNoContext(t *testing.T) {
	app, _ := contextOpsApp(t)
	dir := connectFolder(t, "site")
	_, err := InitProject(dir, InitOptions{})
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".kapi")))
	before := snapshot(t, dir)

	res := connectIn(t, app, dir, ConnectOptions{Hosts: []AgentHost{AgentHostClaudeCode}, HostsOnly: true})
	assert.True(t, res.DryRun, "reading the assistants is a dry run")
	assert.Equal(t, before, snapshot(t, dir), "it writes nothing")
	assert.NoDirExists(t, filepath.Join(dir, ".kapi"), "and opens no store")
	require.Len(t, res.Files, 1, "the rules files are left out")
	assert.Equal(t, ConnectRecipe, res.Files[0].Kind)

	full := connectIn(t, app, dir, ConnectOptions{Hosts: []AgentHost{AgentHostClaudeCode}, DryRun: true})
	assert.Equal(t, full.Hosts, res.Hosts, "the assistants read as a full dry run reads them")
}

func TestClaudePluginCommands(t *testing.T) {
	assert.Equal(t, []string{
		"/plugin marketplace add neokapi/claude-plugins",
		"/plugin install kapi@neokapi-plugins",
	}, ClaudePluginCommands())
	assert.Equal(t, "in Claude Code, /plugin marketplace add neokapi/claude-plugins, then /plugin install kapi@neokapi-plugins",
		claudePluginSteps)
}
