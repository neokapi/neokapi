package backend

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/skills"
)

// claudeDesktopAt points the Claude Desktop folder and the kapi lookup at
// places of the test's own, so no test reads or writes the person's
// configuration or depends on a kapi being installed.
func claudeDesktopAt(t *testing.T, dir string) string {
	t.Helper()
	prevDir, prevBins := claudeDesktopConfigDir, kapiBinaryCandidates
	t.Cleanup(func() { claudeDesktopConfigDir, kapiBinaryCandidates = prevDir, prevBins })
	claudeDesktopConfigDir = func() (string, bool) { return dir, true }
	t.Setenv("PATH", t.TempDir())
	bin := filepath.Join(t.TempDir(), "kapi")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))
	kapiBinaryCandidates = []string{bin}
	return bin
}

// hostRow finds one host's row.
func hostRow(t *testing.T, rows []AgentHostStatus, name string) AgentHostStatus {
	t.Helper()
	for _, r := range rows {
		if r.Host == name {
			return r
		}
	}
	require.Failf(t, "no row", "no row for %s", name)
	return AgentHostStatus{}
}

// TestConnectAgentHost_WritesWhatKapiInitWrites: connecting a project host
// writes the files `kapi init` writes for it, byte for byte, and the row then
// reads connected with those files.
func TestConnectAgentHost_WritesWhatKapiInitWrites(t *testing.T) {
	claudeDesktopAt(t, filepath.Join(t.TempDir(), "missing"))
	app := newWorkspaceApp(t)
	tab, recipe := openSharingProject(t, app, "Wiring")
	root := filepath.Dir(recipe)

	rows, err := app.AgentHosts(tab)
	require.NoError(t, err)
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.Host)
		assert.False(t, r.Wired, r.Host)
	}
	assert.Equal(t, []string{"claude-code", "cursor", "vscode", "codex", "claude-desktop"}, names)
	assert.True(t, hostRow(t, rows, "claude-code").Detected, "Claude Code counts as in use, as kapi init counts it")
	assert.False(t, hostRow(t, rows, "cursor").Detected)
	assert.False(t, hostRow(t, rows, "claude-desktop").Detected, "its configuration folder does not exist")

	for _, h := range []string{"claude-code", "cursor", "vscode", "codex"} {
		_, err := app.ConnectAgentHost(tab, h)
		require.NoError(t, err, h)
	}

	initRoot := t.TempDir()
	_, err = host.WriteAgentWiring(host.AgentWiringOptions{
		Root: initRoot, Hosts: []host.AgentHost{host.AgentHostClaudeCode, host.AgentHostCursor, host.AgentHostVSCode, host.AgentHostCodex},
		Recipe: "kapi.yaml", Skills: skills.Wiring(), Retired: skills.Retired, Tools: host.MCPToolSets(nil),
	})
	require.NoError(t, err)
	require.NoError(t, filepath.WalkDir(initRoot, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return werr
		}
		rel, _ := filepath.Rel(initRoot, p)
		want, _ := os.ReadFile(p)
		got, rerr := os.ReadFile(filepath.Join(root, rel))
		require.NoError(t, rerr, rel)
		assert.Equal(t, string(want), string(got), rel)
		return nil
	}))

	rows, err = app.AgentHosts(tab)
	require.NoError(t, err)
	assert.Equal(t, AgentHostStatus{Host: "claude-code", Label: "Claude Code", Detected: true, Wired: true,
		Files: []string{".mcp.json", ".claude/skills/kapi"}}, hostRow(t, rows, "claude-code"))
	assert.Equal(t, AgentHostStatus{Host: "cursor", Label: "Cursor", Detected: true, Wired: true,
		Files: []string{".cursor/mcp.json"}}, hostRow(t, rows, "cursor"))
	assert.True(t, hostRow(t, rows, "codex").Wired)
}

// TestConnectAgentHost_RefusesWhatItCannotConnect: an unknown host and a
// request for several are refused, and a tab with no checkout lists nothing
// and connects nothing.
func TestConnectAgentHost_RefusesWhatItCannotConnect(t *testing.T) {
	claudeDesktopAt(t, t.TempDir())
	app := newWorkspaceApp(t)
	tab, _ := openSharingProject(t, app, "Refusing")

	_, err := app.ConnectAgentHost(tab, "emacs")
	require.ErrorIs(t, err, host.ErrUnknownAgentHost)
	_, err = app.ConnectAgentHost(tab, "all")
	require.EqualError(t, err, `name one assistant to connect, not "all"`)

	home, err := app.ListWorkspaceProjects()
	require.NoError(t, err)
	ctxTab, err := app.OpenWorkspaceContext(home.Projects[0].Key)
	require.NoError(t, err)
	rows, err := app.AgentHosts(ctxTab.ID)
	require.NoError(t, err)
	assert.Empty(t, rows)
	files, err := app.RulesFiles(ctxTab.ID)
	require.NoError(t, err)
	assert.Empty(t, files)
	_, err = app.ConnectAgentHost(ctxTab.ID, "cursor")
	require.Error(t, err)
}

// TestConnectAgentHost_ClaudeDesktop: the entry names the kapi binary and the
// recipe by absolute path, keeps every other key in the person's file, reads
// back as connected, and a second connect writes nothing. An entry someone
// wrote by hand is kept.
func TestConnectAgentHost_ClaudeDesktop(t *testing.T) {
	dir := t.TempDir()
	bin := claudeDesktopAt(t, dir)
	config := filepath.Join(dir, claudeDesktopConfigFile)
	require.NoError(t, os.WriteFile(config, []byte(`{"globalShortcut": "Cmd+Space", "mcpServers": {"files": {"command": "npx", "args": ["-y", "server"]}}}`), 0o600))
	app := newWorkspaceApp(t)
	tab, recipe := openSharingProject(t, app, "Desktop")
	abs, err := filepath.Abs(recipe)
	require.NoError(t, err)

	res, err := app.ConnectAgentHost(tab, agentHostClaudeDesktop)
	require.NoError(t, err)
	require.Len(t, res.Files, 1)
	assert.Equal(t, host.AgentWiringUpdated, res.Files[0].Action)
	assert.Equal(t, config, res.Files[0].Path)

	var doc map[string]any
	data, err := os.ReadFile(config)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "Cmd+Space", doc["globalShortcut"], "every other key is kept")
	servers := doc["mcpServers"].(map[string]any)
	assert.Contains(t, servers, "files")
	assert.Equal(t, map[string]any{"command": bin, "args": []any{"mcp", "--project", abs}}, servers["kapi"])
	info, err := os.Stat(config)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o600), info.Mode().Perm(), "the file keeps its mode")

	rows, err := app.AgentHosts(tab)
	require.NoError(t, err)
	assert.Equal(t, AgentHostStatus{Host: agentHostClaudeDesktop, Label: "Claude Desktop", Detected: true, Wired: true,
		Files: []string{config}}, hostRow(t, rows, agentHostClaudeDesktop))

	again, err := app.ConnectAgentHost(tab, agentHostClaudeDesktop)
	require.NoError(t, err)
	assert.Equal(t, host.AgentWiringUnchanged, again.Files[0].Action)

	require.NoError(t, os.WriteFile(config, []byte(`{"mcpServers": {"kapi": {"command": "kapi", "args": ["mcp"], "env": {"KAPI_DATA_DIR": "/data"}}}}`), 0o600))
	kept, err := app.ConnectAgentHost(tab, agentHostClaudeDesktop)
	require.NoError(t, err)
	assert.Equal(t, host.AgentWiringKept, kept.Files[0].Action)
	after, err := os.ReadFile(config)
	require.NoError(t, err)
	assert.Contains(t, string(after), "KAPI_DATA_DIR", "a hand-written entry is left as it is")
}

// TestRulesFiles_PlanThenWrite: the files kapi writes are listed before they
// exist, written on request, and then listed as on disk.
func TestRulesFiles_PlanThenWrite(t *testing.T) {
	app := newWorkspaceApp(t)
	tab, recipe := openSharingProject(t, app, "Rules")

	files, err := app.RulesFiles(tab)
	require.NoError(t, err)
	require.NotEmpty(t, files)
	assert.Equal(t, "AGENTS.md", files[0].Path)
	for _, f := range files {
		assert.False(t, f.Exists, f.Path)
		assert.NotEmpty(t, f.Section, f.Path)
	}

	res, err := app.WriteRulesFiles(tab)
	require.NoError(t, err)
	assert.NotEmpty(t, res.Files)
	_, err = os.Stat(filepath.Join(filepath.Dir(recipe), "AGENTS.md"))
	require.NoError(t, err)

	files, err = app.RulesFiles(tab)
	require.NoError(t, err)
	for _, f := range files {
		assert.True(t, f.Exists, f.Path)
	}
}
