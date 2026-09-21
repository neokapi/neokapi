package host_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host"
)

// testSkills is a skill tree the shape of the real one: a directory named for
// the skill, a SKILL.md, and one reference file below it.
func testSkills() fstest.MapFS {
	return fstest.MapFS{
		"kapi/SKILL.md":            {Data: []byte("---\nname: kapi\n---\n")},
		"kapi/references/edit.md":  {Data: []byte("read, edit, write, verify\n")},
		"other/SKILL.md":           {Data: []byte("---\nname: other\n---\n")},
		"kapi/references/voice.md": {Data: []byte("retrieve, score, fix\n")},
	}
}

// wire runs the wiring over a fresh project root and returns the root and the
// result.
func wire(t *testing.T, hosts []host.AgentHost) (string, *host.AgentWiringResult) {
	t.Helper()
	root := t.TempDir()
	res, err := host.WriteAgentWiring(host.AgentWiringOptions{
		Root:   root,
		Hosts:  hosts,
		Skills: testSkills(),
	})
	require.NoError(t, err)
	return root, res
}

// readJSON reads a written config file back as a generic document.
func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "read %s", path)
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func TestAgentWiringWritesClaudeCodeConfigAndSkill(t *testing.T) {
	root, res := wire(t, []host.AgentHost{host.AgentHostClaudeCode})

	doc := readJSON(t, filepath.Join(root, ".mcp.json"))
	servers, ok := doc["mcpServers"].(map[string]any)
	require.True(t, ok, "Claude Code reads its servers under mcpServers")
	entry, ok := servers["kapi"].(map[string]any)
	require.True(t, ok, "the entry is named kapi")
	assert.Equal(t, "stdio", entry["type"])
	assert.Equal(t, "kapi", entry["command"])
	assert.Equal(t, []any{"mcp", "--project", "kapi.yaml"}, entry["args"],
		"the entry names the project, so the server never answers for whichever directory it started in")
	assert.NotContains(t, entry, "env", "no environment travels in a committed configuration file")

	body, err := os.ReadFile(filepath.Join(root, ".claude/skills/kapi/SKILL.md"))
	require.NoError(t, err, "the skill lands where Claude Code reads project skills")
	assert.Contains(t, string(body), "name: kapi")
	_, err = os.Stat(filepath.Join(root, ".claude/skills/kapi/references/edit.md"))
	require.NoError(t, err, "a skill's reference files travel with it")

	assert.Equal(t, []host.AgentHost{host.AgentHostClaudeCode}, res.Hosts)
	require.Len(t, res.Files, 3, "one config file and one entry per skill")
	for _, f := range res.Files {
		assert.Equal(t, host.AgentWiringCreated, f.Action, "%s", f.Path)
		assert.False(t, filepath.IsAbs(f.Path), "a reported path is project-relative: %s", f.Path)
	}
}

func TestAgentWiringWritesEachHostsOwnSpelling(t *testing.T) {
	root, _ := wire(t, host.AgentHosts())

	// VS Code reads `servers`; Claude Code and Cursor read `mcpServers`. A key
	// the host does not read is silently inert, which is the whole reason this
	// is asserted rather than remembered.
	cursor := readJSON(t, filepath.Join(root, ".cursor/mcp.json"))
	assert.Contains(t, cursor, "mcpServers")
	assert.NotContains(t, cursor, "servers")

	code := readJSON(t, filepath.Join(root, ".vscode/mcp.json"))
	assert.Contains(t, code, "servers")
	assert.NotContains(t, code, "mcpServers")

	_, err := os.Stat(filepath.Join(root, ".agents/skills/kapi/SKILL.md"))
	require.NoError(t, err, "the cross-client skills directory gets the same skill")
}

func TestAgentWiringIsIdempotent(t *testing.T) {
	root, _ := wire(t, host.AgentHosts())
	before := treeSnapshot(t, root)

	res, err := host.WriteAgentWiring(host.AgentWiringOptions{
		Root: root, Hosts: host.AgentHosts(), Skills: testSkills(),
	})
	require.NoError(t, err)
	assert.Equal(t, before, treeSnapshot(t, root), "a second run writes nothing new")
	for _, f := range res.Files {
		assert.Contains(t, []host.AgentWiringAction{host.AgentWiringKept, host.AgentWiringUnchanged}, f.Action,
			"%s", f.Path)
	}
}

func TestAgentWiringKeepsAnEntrySomeoneElseWrote(t *testing.T) {
	root := t.TempDir()
	held := `{
  "mcpServers": {
    "kapi": {"command": "/opt/mine/kapi", "args": ["mcp", "--all-tools"]},
    "other": {"command": "other"}
  },
  "extra": true
}
`
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mcp.json"), []byte(held), 0o644))

	res, err := host.WriteAgentWiring(host.AgentWiringOptions{
		Root: root, Hosts: []host.AgentHost{host.AgentHostClaudeCode},
	})
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
	require.NoError(t, err)
	assert.Equal(t, held, string(raw), "a file that already names a kapi server is read and left alone")
	require.NotEmpty(t, res.Files)
	assert.Equal(t, host.AgentWiringKept, res.Files[0].Action)
}

func TestAgentWiringKeepsWhatElseIsInTheFile(t *testing.T) {
	root := t.TempDir()
	held := `{"mcpServers":{"other":{"command":"other","args":["serve"]}},"inputs":[{"id":"token"}]}`
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mcp.json"), []byte(held), 0o644))

	_, err := host.WriteAgentWiring(host.AgentWiringOptions{
		Root: root, Hosts: []host.AgentHost{host.AgentHostClaudeCode},
	})
	require.NoError(t, err)

	doc := readJSON(t, filepath.Join(root, ".mcp.json"))
	assert.Contains(t, doc, "inputs", "a key kapi does not know about survives")
	servers := doc["mcpServers"].(map[string]any)
	assert.Contains(t, servers, "other", "another server stays")
	assert.Contains(t, servers, "kapi", "kapi's server is added beside it")
}

func TestAgentWiringWritesNothingForNoHosts(t *testing.T) {
	root, res := wire(t, nil)
	assert.Empty(t, res.Files)
	assert.Empty(t, res.Hosts)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries, "`--agents none` leaves the project as it found it")
}

func TestAgentWiringStaysInsideTheProject(t *testing.T) {
	root, res := wire(t, host.AgentHosts())
	for _, f := range res.Files {
		abs := filepath.Join(root, filepath.FromSlash(f.Path))
		rel, err := filepath.Rel(root, abs)
		require.NoError(t, err)
		assert.NotContains(t, rel, "..", "every path written is under the project root: %s", f.Path)
	}
}

// TestAgentWiringNamesTheRecipeAndNotAPathToIt: the entry is committed and
// shared, so a directory that resolves on one machine must not reach it.
func TestAgentWiringNamesTheRecipeAndNotAPathToIt(t *testing.T) {
	root := t.TempDir()
	res, err := host.WriteAgentWiring(host.AgentWiringOptions{
		Root:   root,
		Hosts:  []host.AgentHost{host.AgentHostClaudeCode},
		Recipe: filepath.Join(root, "kapi.yaml"),
	})
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), root, "no machine path reaches a committed file")
	assert.Contains(t, string(raw), `"kapi.yaml"`)
	require.NotEmpty(t, res.Files)
	assert.Contains(t, res.Files[0].Detail, "kapi mcp --project kapi.yaml")
}

// TestAgentWiringRefreshesTheSkillItShips pins the one place the wiring
// overwrites something: the files of the skill it carries.
//
// The skill names commands and flags, so a copy that lags the binary teaches
// an assistant a kapi this binary does not have. What the binary did not put
// there is left where it is.
func TestAgentWiringRefreshesTheSkillItShips(t *testing.T) {
	root, _ := wire(t, []host.AgentHost{host.AgentHostClaudeCode})

	skill := filepath.Join(root, ".claude/skills/kapi")
	require.NoError(t, os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("stale\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(skill, "NOTES.md"), []byte("mine\n"), 0o644))

	res, err := host.WriteAgentWiring(host.AgentWiringOptions{
		Root: root, Hosts: []host.AgentHost{host.AgentHostClaudeCode}, Skills: testSkills(),
	})
	require.NoError(t, err)

	body, err := os.ReadFile(filepath.Join(skill, "SKILL.md"))
	require.NoError(t, err)
	assert.Contains(t, string(body), "name: kapi", "the shipped file is refreshed to what this binary carries")

	mine, err := os.ReadFile(filepath.Join(skill, "NOTES.md"))
	require.NoError(t, err)
	assert.Equal(t, "mine\n", string(mine), "a file the binary does not ship is left alone")

	var refreshed bool
	for _, f := range res.Files {
		if f.Path == ".claude/skills/kapi" {
			refreshed = f.Action == host.AgentWiringUpdated
		}
	}
	assert.True(t, refreshed, "the result says the skill was refreshed")
}

func TestParseAgentHosts(t *testing.T) {
	cases := []struct {
		name     string
		spec     string
		want     []host.AgentHost
		explicit bool
		wantErr  bool
	}{
		{name: "empty asks for detection", spec: "", explicit: false},
		{name: "none writes nothing", spec: "none", explicit: true},
		{name: "all is every host", spec: "all", want: host.AgentHosts(), explicit: true},
		{
			name: "a list is read in the order kapi writes them", spec: "vscode, claude-code",
			want:     []host.AgentHost{host.AgentHostClaudeCode, host.AgentHostVSCode},
			explicit: true,
		},
		{name: "a name kapi does not wire is refused", spec: "emacs", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, explicit, err := host.ParseAgentHosts(tc.spec)
			if tc.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, host.ErrUnknownAgentHost)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.explicit, explicit)
		})
	}
}

func TestDetectAgentHostsFollowsWhatTheProjectAlreadyKeeps(t *testing.T) {
	bare := t.TempDir()
	assert.Equal(t, []host.AgentHost{host.AgentHostClaudeCode}, host.DetectAgentHosts(bare),
		"a project that has met no agent is still wired for Claude Code")

	used := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(used, ".cursor"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(used, ".vscode"), 0o755))
	assert.Equal(t,
		[]host.AgentHost{host.AgentHostClaudeCode, host.AgentHostCursor, host.AgentHostVSCode},
		host.DetectAgentHosts(used))
}

// treeSnapshot reads every file under root into a path-to-content map, so two
// runs can be compared byte for byte.
func treeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		body, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		out[filepath.ToSlash(rel)] = string(body)
		return nil
	}))
	return out
}
