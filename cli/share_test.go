package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// `kapi share` end to end through the root: the ladder in a folder kapi has
// not been run in, --via folder writing the recipe and syncing, and the next
// share syncing through it.

// shareFolder is a folder with one Markdown file and no recipe, with project
// discovery off and the working directory in it.
func shareFolder(t *testing.T) string {
	t.Helper()
	isolateHelp(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "guide.md"), []byte("# Guide\n\nWe utilise the widget.\n"), 0o644))
	t.Setenv("KAPI_NO_PROJECT", "")
	t.Setenv("KAPI_PROJECT", "")
	t.Chdir(dir)
	return dir
}

// TestShareCmd_RefusesUnderNoProject: with KAPI_NO_PROJECT set and no -p,
// share neither reads nor writes the folder it stands in, and says what
// `kapi context sync` says; -p names the folder.
func TestShareCmd_RefusesUnderNoProject(t *testing.T) {
	dir := shareFolder(t)
	t.Setenv("KAPI_NO_PROJECT", "1")
	for _, args := range [][]string{{"share"}, {"share", "--status"}, {"share", "--via", "folder", t.TempDir()}} {
		_, err := runRootErr(t, args...)
		require.Error(t, err, "%v", args)
		assert.Contains(t, err.Error(), "KAPI_NO_PROJECT is set and no -p was given")
		_, syncErr := runRootErr(t, "context", "sync")
		require.Error(t, syncErr)
		assert.Equal(t, syncErr.Error(), err.Error(), "the message context sync gives")
	}
	assert.NoFileExists(t, filepath.Join(dir, "kapi.yaml"))

	out := runRoot(t, "share", "-p", filepath.Join(dir, "kapi.yaml"))
	assert.True(t, strings.HasPrefix(out, "How far your rules reach\n"), out)
}

// runRootErr executes args on a fresh tiered root and returns what it printed
// and the error it returned.
func runRootErr(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newTieredRoot(t)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	stdout, err := captureStdout(t, root.Execute)
	return stdout + buf.String(), err
}

func TestShareCmd_FromAFreshFolder(t *testing.T) {
	dir := shareFolder(t)
	shared := filepath.Join(t.TempDir(), "team")

	out := runRoot(t, "share")
	assert.True(t, strings.HasPrefix(out, "How far your rules reach\n"), out)
	assert.Contains(t, out, "    With your team       kapi share --via git | folder <path>\n")
	assert.NoFileExists(t, filepath.Join(dir, "kapi.yaml"), "the ladder writes nothing")

	out = runRoot(t, "share", "--via", "folder", shared)
	assert.Equal(t, "kapi.yaml  created\nWith your team · through a shared folder ("+shared+")\n"+
		"  pushed none · pulled none · last sync just now\n", out)
	proj, err := project.Load(filepath.Join(dir, "kapi.yaml"))
	require.NoError(t, err)
	assert.Equal(t, &project.ContextBackend{Backend: project.ContextBackendFile, Path: shared}, proj.Context)

	out = runRoot(t, "share")
	assert.Equal(t, "With your team · through a shared folder ("+shared+")\n"+
		"  pushed none · pulled none · last sync just now\n", out)

	out = runRoot(t, "share", "--status")
	assert.Equal(t, "With your team · through a shared folder ("+shared+")\n"+
		"  nothing to push · nothing to pull · last sync just now\n", out)

	out = runRoot(t, "share", "--status", "--json")
	var got struct {
		Rung    string `json:"rung"`
		Backend struct {
			Kind     string `json:"kind"`
			Location string `json:"location"`
		} `json:"backend"`
		Status *struct {
			ToPush int `json:"to_push"`
		} `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &got), out)
	assert.Equal(t, host.ShareRungTeam, got.Rung)
	assert.Equal(t, "file", got.Backend.Kind)
	assert.Equal(t, shared, got.Backend.Location)
	require.NotNil(t, got.Status)
	assert.Equal(t, 0, got.Status.ToPush)
}

func TestShareCmd_BowrainWithoutThePlugin(t *testing.T) {
	dir := shareFolder(t)
	out, err := runRootErr(t, "share", "--via", "bowrain", "https://bowrain.example")
	require.Error(t, err, out)
	assert.Contains(t, err.Error(), "kapi plugin install bowrain")
	assert.Contains(t, err.Error(), "brew install neokapi/tap/bowrain-cli")
	assert.Equal(t, host.ExitError, host.ExitCode(nil, err), "the plugin is not installed")
	assert.NoFileExists(t, filepath.Join(dir, "kapi.yaml"), "nothing is written")
}

func TestShareCmd_FilesOnly(t *testing.T) {
	dir := shareFolder(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"),
		[]byte("version: v1\nname: share-files-only\ndefaults:\n  source_language: en\ncollections: []\n"), 0o644))
	out := runRoot(t, "share", "--files-only")
	assert.Equal(t, "AGENTS.md  created\nCLAUDE.md  created\n"+machineLine()+"\n", out)
	assert.FileExists(t, filepath.Join(dir, "AGENTS.md"))

	out = runRoot(t, "share", "--files-only")
	assert.Equal(t, "AGENTS.md  kept\nCLAUDE.md  kept\n"+machineLine()+"\n", out)
}

// machineLine is the first rung's line on the machine the test runs on.
func machineLine() string {
	if runtime.GOOS == "darwin" {
		return "On this Mac · your rules, your assistants"
	}
	return "On this machine · your rules, your assistants"
}
