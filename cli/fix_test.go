package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixCmdVoice carries one failing word rule with a replacement and a
// prohibited pattern with none.
const fixCmdVoice = `name: Fix
style:
  prohibited_patterns:
    - regex: '(?i)\bsimply\b'
      description: Say what to do without minimising it
terms:
  - term: utilize
    replacement: use
`

// fixCmdProject writes a one-language project with a markdown guide and a
// JSON file, each holding a word its voice replaces, and returns its
// directory.
func fixCmdProject(t *testing.T) string {
	t.Helper()
	isolateHelp(t)
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte(`version: v1
name: fixcmd
defaults:
  source_language: en
collections:
  - name: docs
    content:
      - path: "docs/*.md"
      - path: "strings/*.json"
`), 0o644))
	for rel, content := range map[string]string{
		"docs/guide.md":   "# Guide\n\nWe *utilize* the [widget](https://example.com). Simply start.\n",
		"strings/en.json": "{\n  \"cta\":  \"utilize it\"\n}\n",
	} {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	require.NoError(t, os.WriteFile(layoutVoicePath(t, dir), []byte(fixCmdVoice), 0o644))
	readProjectContext(t, dir)
	return dir
}

func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

// `kapi fix` applies the fixes across the project's content, keeps every
// other byte of each file, lists the finding with no fix and prints what a
// check finds afterwards.
func TestFixCmd_FixesTheProjectContent(t *testing.T) {
	dir := fixCmdProject(t)

	out := runRoot(t, "fix", "-p", dir)
	assert.Contains(t, out, "2 fixed in 2 files\n")
	assert.Contains(t, out, "  docs/guide.md    1 fixed: utilize → use\n")
	assert.Contains(t, out, "  strings/en.json  1 fixed: utilize → use\n")
	assert.Contains(t, out, "1 finding needs a rewrite, with no automatic fix (voice.style). Edit by hand.\n")
	assert.Contains(t, out, "kapi check: 1 failing finding remains (voice)\n")

	assert.Equal(t, "# Guide\n\nWe *use* the [widget](https://example.com). Simply start.\n", readText(t, filepath.Join(dir, "docs", "guide.md")))
	assert.Equal(t, "{\n  \"cta\":  \"use it\"\n}\n", readText(t, filepath.Join(dir, "strings", "en.json")))
}

// `kapi fix --dry-run --json` prints the result and writes nothing.
func TestFixCmd_DryRunJSON(t *testing.T) {
	dir := fixCmdProject(t)
	guide := filepath.Join(dir, "docs", "guide.md")
	before := readText(t, guide)

	out := runRoot(t, "fix", "-p", dir, "--dry-run", "--json", filepath.Join(dir, "docs", "guide.md"))
	var res struct {
		Schema string `json:"schema"`
		DryRun bool   `json:"dry_run"`
		Fixed  int    `json:"fixed"`
		Files  []struct {
			Result struct {
				Status string `json:"status"`
			} `json:"result"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res), out)
	assert.Equal(t, "kapi.fix-result/v1", res.Schema)
	assert.True(t, res.DryRun)
	assert.Equal(t, 1, res.Fixed)
	require.Len(t, res.Files, 1)
	assert.Equal(t, "previewed", res.Files[0].Result.Status)
	assert.Equal(t, before, readText(t, guide))
}

// `kapi fix` is a work command listed right after `kapi check`, and runs in
// the browser build too.
func TestFixCmd_IsAWorkCommandAfterCheck(t *testing.T) {
	isolateHelp(t)
	names := func(cmds []*cobra.Command) []string {
		var out []string
		for _, c := range cmds {
			out = append(out, c.Name())
		}
		return out
	}
	for name, cmds := range map[string][]*cobra.Command{
		"native":  KapiCommandSet(processOnlyApp(t)),
		"browser": BrowserCommandSet(processOnlyApp(t)),
	} {
		got := names(cmds)
		check, fix := slices.Index(got, "check"), slices.Index(got, "fix")
		require.GreaterOrEqual(t, check, 0, name)
		assert.Equal(t, check+1, fix, "%s: fix follows check", name)
		assert.Equal(t, "work", cmds[fix].GroupID, name)
		_, gap := BrowserUnavailableReason("fix")
		assert.False(t, gap, "fix runs in the browser")
	}
}
