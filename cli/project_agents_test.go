package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `kapi init` and the coding agents that work in the project it just made.
//
// The files written here are loaded as configuration by a program that runs
// what they say, so the command names every one of them, writes nothing
// outside the project, and puts nothing in them but a command.

// runInit executes `kapi init` over dir and returns stdout and stderr.
func runInit(t *testing.T, dir string, args ...string) (string, string) {
	t.Helper()
	cmd := NewInitCmd(newAppForTest(t))
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append([]string{"--dir", dir}, args...))
	require.NoError(t, cmd.Execute())
	return out.String(), errOut.String()
}

func TestInitCmd_wiresClaudeCodeByDefault(t *testing.T) {
	dir := t.TempDir()
	out, errOut := runInit(t, dir)
	assert.Empty(t, errOut, "no warning on the happy path")

	raw, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	require.NoError(t, err, "a project Claude Code has never seen still gets its MCP entry")
	var doc struct {
		Servers map[string]struct {
			Type    string   `json:"type"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
			Env     any      `json:"env"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	entry, ok := doc.Servers["kapi"]
	require.True(t, ok)
	assert.Equal(t, "stdio", entry.Type)
	assert.Equal(t, "kapi", entry.Command)
	assert.Equal(t, []string{"mcp", "--project", "kapi.yaml"}, entry.Args,
		"the entry names this project rather than leaving the server to discover one")
	assert.Nil(t, entry.Env, "nothing but a command travels in a committed file")

	_, err = os.Stat(filepath.Join(dir, ".claude/skills/kapi/SKILL.md"))
	require.NoError(t, err, "the skill this binary carries lands where the host reads project skills")

	assert.Contains(t, out, "mcp:    .mcp.json (created")
	assert.Contains(t, out, "skill:  .claude/skills/kapi (created")
}

func TestInitCmd_agentsNoneWritesNothing(t *testing.T) {
	dir := t.TempDir()
	out, errOut := runInit(t, dir, "--agents", "none")
	assert.Empty(t, errOut)

	for _, rel := range []string{".mcp.json", ".claude", ".cursor", ".vscode", ".agents"} {
		_, err := os.Stat(filepath.Join(dir, rel))
		assert.True(t, os.IsNotExist(err), "--agents none leaves %s alone", rel)
	}
	assert.NotContains(t, out, "mcp:")
	assert.NotContains(t, out, "skill:")
}

func TestInitCmd_agentsFollowsWhatTheProjectAlreadyKeeps(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".cursor"), 0o755))
	runInit(t, dir)

	_, err := os.Stat(filepath.Join(dir, ".cursor/mcp.json"))
	require.NoError(t, err, "a project that already uses Cursor is wired for it")
	_, err = os.Stat(filepath.Join(dir, ".vscode/mcp.json"))
	assert.True(t, os.IsNotExist(err), "a host the project does not use gets no directory of its own")
}

func TestInitCmd_agentsRefusesAHostItDoesNotWire(t *testing.T) {
	dir := t.TempDir()
	cmd := NewInitCmd(newAppForTest(t))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--dir", dir, "--agents", "emacs"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "emacs")

	_, statErr := os.Stat(filepath.Join(dir, "kapi.yaml"))
	assert.True(t, os.IsNotExist(statErr),
		"the flag is read before anything is written, so a bad name leaves no half-made project")
}

// TestInitCmd_rerunWiresAnExistingProject: an existing project gains the same
// wiring by running init again, which is what makes the flag the whole story
// rather than a verb of its own.
func TestInitCmd_rerunWiresAnExistingProject(t *testing.T) {
	dir := t.TempDir()
	runInit(t, dir, "--agents", "none")
	_, err := os.Stat(filepath.Join(dir, ".mcp.json"))
	require.True(t, os.IsNotExist(err))

	out, _ := runInit(t, dir)
	assert.Contains(t, out, "already initialized")
	_, err = os.Stat(filepath.Join(dir, ".mcp.json"))
	require.NoError(t, err, "running init on a project that has a recipe wires it")
}

// A project that declares target languages gets the translation tools in its
// MCP entry beside the writing ones, and the skill is one file.
func TestInitCmd_translationProjectNamesTheToolSets(t *testing.T) {
	dir := t.TempDir()
	runInit(t, dir, "--target-locale", "fr")

	raw, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	require.NoError(t, err)
	var doc struct {
		Servers map[string]struct {
			Args []string `json:"args"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	assert.Equal(t, []string{"mcp", "--project", "kapi.yaml", "--tools", "writing,translation"}, doc.Servers["kapi"].Args)

	entries, err := os.ReadDir(filepath.Join(dir, ".claude/skills/kapi"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "SKILL.md", entries[0].Name())
}
