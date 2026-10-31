package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// connectFolder is an empty folder named name, made the working directory,
// with project discovery off so the folder is the project kapi connect writes,
// and the person's Claude Code settings pointed at an empty folder.
func connectFolder(t *testing.T, name string) string {
	t.Helper()
	isolateHelp(t)
	t.Setenv("KAPI_NO_PROJECT", "1")
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	parent, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dir := filepath.Join(parent, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	t.Chdir(dir)
	return dir
}

// runConnect runs `kapi <args>` on the tiered root and returns what it
// printed and the error it returned.
func runConnect(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newTieredRoot(t)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func TestConnectCmd_AFolderWithNoRecipe(t *testing.T) {
	dir := connectFolder(t, "site")
	out, err := runConnect(t, "connect")
	require.NoError(t, err, out)

	assert.Contains(t, out, "No assistant found here; Claude Code is connected.\n")
	assert.Contains(t, out, "Claude Code   connected · rules, MCP server, skill\n")
	assert.Contains(t, out, "  .mcp.json            created\n")
	assert.Contains(t, out, "  .claude/skills/kapi  created\n")
	assert.Contains(t, out, "  Edit hook            pending: in Claude Code, /plugin marketplace add neokapi/claude-plugins, then /plugin install kapi@neokapi-plugins\n")
	assert.Contains(t, out, "  Stop hook            pending: in Claude Code, /plugin marketplace add neokapi/claude-plugins, then /plugin install kapi@neokapi-plugins\n")
	assert.Contains(t, out, "Codex         not connected: no .codex/ folder here (kapi connect codex adds it)\n")
	assert.Contains(t, out, "\nProject\n  kapi.yaml            created\n  AGENTS.md            created\n  CLAUDE.md            created\n")
	assert.True(t, strings.HasSuffix(out, "\nSee what an assistant reads for a file: kapi context <path> --explain\n"), out)

	// The same files `kapi init` writes into a folder of the same name.
	initDir := filepath.Join(t.TempDir(), "site")
	_, err = runConnect(t, "init", "--dir", initDir)
	require.NoError(t, err)
	for _, name := range []string{"AGENTS.md", "CLAUDE.md", ".mcp.json", ".claude/skills/kapi/SKILL.md"} {
		got, gerr := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, gerr, name)
		want, werr := os.ReadFile(filepath.Join(initDir, name))
		require.NoError(t, werr, name)
		assert.Equal(t, string(want), string(got), "%s is what kapi init writes", name)
	}
}

func TestConnectCmd_KeepsWhatAPersonWrote(t *testing.T) {
	dir := connectFolder(t, "site")
	mine := "# House notes\n\nRun the tests before you push.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(mine), 0o644))

	out, err := runConnect(t, "connect")
	require.NoError(t, err, out)
	assert.NotContains(t, out, "No assistant found here")
	assert.Contains(t, out, "  CLAUDE.md            updated (Writing rules section)\n")
	doc, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(doc), mine), "the person's text is kept byte for byte")
	assert.Contains(t, string(doc), "## Writing rules")
}

func TestConnectCmd_ASecondRunKeepsEverything(t *testing.T) {
	dir := connectFolder(t, "site")
	_, err := runConnect(t, "connect")
	require.NoError(t, err)
	files := []string{"kapi.yaml", "AGENTS.md", "CLAUDE.md", ".mcp.json", ".claude/skills/kapi/SKILL.md"}
	before := map[string]string{}
	for _, f := range files {
		body, rerr := os.ReadFile(filepath.Join(dir, f))
		require.NoError(t, rerr)
		info, serr := os.Stat(filepath.Join(dir, f))
		require.NoError(t, serr)
		before[f] = info.ModTime().String() + string(body)
	}

	out, err := runConnect(t, "connect")
	require.NoError(t, err, out)
	for _, line := range []string{"kapi.yaml", "AGENTS.md", "CLAUDE.md", ".mcp.json", ".claude/skills/kapi"} {
		assert.Regexp(t, `\n  `+regexp.QuoteMeta(line)+` +kept\n`, out)
	}
	assert.NotContains(t, out, "created")
	assert.NotContains(t, out, "updated")
	for _, f := range files {
		body, _ := os.ReadFile(filepath.Join(dir, f))
		info, _ := os.Stat(filepath.Join(dir, f))
		assert.Equal(t, before[f], info.ModTime().String()+string(body), "%s is not rewritten", f)
	}
}

func TestConnectCmd_DryRunWritesNothing(t *testing.T) {
	dir := connectFolder(t, "site")
	out, err := runConnect(t, "connect", "--dry-run")
	require.NoError(t, err, out)
	assert.Contains(t, out, "No assistant found here; kapi would connect Claude Code.\n")
	assert.Contains(t, out, "Claude Code   would connect · rules, MCP server, skill\n")
	assert.Contains(t, out, "  kapi.yaml            would create\n")
	assert.Contains(t, out, "  .mcp.json            would create\n")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "--dry-run writes nothing")
}

func TestConnectCmd_JSON(t *testing.T) {
	connectFolder(t, "site")
	out, err := runConnect(t, "connect", "--json")
	require.NoError(t, err, out)

	var doc struct {
		Recipe  string `json:"recipe"`
		DryRun  bool   `json:"dry_run"`
		Default bool   `json:"default"`
		Hosts   []struct {
			Host      string   `json:"host"`
			Connected bool     `json:"connected"`
			Reason    string   `json:"reason"`
			Reads     []string `json:"reads"`
			Files     []struct {
				Path   string `json:"path"`
				Kind   string `json:"kind"`
				Action string `json:"action"`
				Detail string `json:"detail"`
				Hook   string `json:"hook"`
			} `json:"files"`
		} `json:"hosts"`
		Files []struct {
			Path   string `json:"path"`
			Kind   string `json:"kind"`
			Action string `json:"action"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &doc), out)
	assert.False(t, doc.DryRun)
	assert.True(t, doc.Default)
	assert.True(t, strings.HasSuffix(doc.Recipe, "kapi.yaml"))

	hosts := map[string]bool{}
	for _, h := range doc.Hosts {
		hosts[h.Host] = h.Connected
	}
	assert.Equal(t, map[string]bool{"claude-code": true, "cursor": false, "vscode": false, "codex": false}, hosts)

	claude := doc.Hosts[0]
	kinds := map[string]string{}
	for _, f := range claude.Files {
		kinds[f.Kind] = f.Action
	}
	assert.Equal(t, map[string]string{"mcp": "created", "skill": "created", "hook": "pending"}, kinds)
	var hooks []string
	for _, f := range claude.Files {
		if f.Kind == "hook" {
			hooks = append(hooks, f.Hook+":"+f.Action)
		}
	}
	assert.Equal(t, []string{"edit:pending", "stop:pending"}, hooks, "--json carries each hook and its state")

	project := map[string]string{}
	for _, f := range doc.Files {
		project[f.Path] = f.Kind + ":" + f.Action
	}
	assert.Equal(t, map[string]string{
		"kapi.yaml": "recipe:created", "AGENTS.md": "rules:created", "CLAUDE.md": "rules:created",
	}, project)
}

func TestConnectCmd_NamedHost(t *testing.T) {
	dir := connectFolder(t, "site")
	out, err := runConnect(t, "connect", "cursor")
	require.NoError(t, err, out)
	assert.Contains(t, out, "Cursor        connected · rules, MCP server\n")
	assert.Contains(t, out, "  .cursor/mcp.json  created\n")
	assert.NotContains(t, out, "Claude Code")
	_, err = os.Stat(filepath.Join(dir, ".cursor/mcp.json"))
	require.NoError(t, err)
}

func TestConnectCmd_UnsupportedHostExitsOne(t *testing.T) {
	dir := connectFolder(t, "site")
	root := newTieredRoot(t)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"connect", "windsurf"})
	cmd, err := root.ExecuteC()
	require.Error(t, err)
	assert.Equal(t, "windsurf: not supported yet; kapi connect supports claude, cursor, vscode, codex, agents", err.Error())
	assert.Equal(t, ExitError, ExitCode(cmd, err))
	assert.Equal(t, 1, ExitError)

	entries, rerr := os.ReadDir(dir)
	require.NoError(t, rerr)
	assert.Empty(t, entries, "an unsupported name writes nothing")
}
