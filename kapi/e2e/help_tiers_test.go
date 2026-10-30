//go:build e2e

package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kapiAt runs the built kapi against the project in dir, named by KAPI_PROJECT
// with discovery left on, and returns standard output and standard error
// apart. The root takes no -p, so this is how the root help finds a project.
func kapiAt(t *testing.T, dir string, args ...string) (stdout, stderr string) {
	t.Helper()
	cmd := exec.Command(kapiBin, args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), isoEnv...), "KAPI_NO_PROJECT=", "KAPI_PROJECT="+dir)
	var out, errs bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errs
	require.NoError(t, cmd.Run(), "kapi %s failed:\n%s%s", strings.Join(args, " "), out.String(), errs.String())
	return out.String(), errs.String()
}

// TestHelpTiers_RealRoot drives the help tiers through the real kapi root, the
// one whose PersistentPreRunE runs App.Init and so points
// project.OnKeyWarnings at standard error. The recipe declares a target
// language and carries a near-miss key. Every help view reveals the Languages
// group and writes nothing to standard error; `kapi status`, which loads the
// recipe, reports the key, so the recipe does reach the hook.
func TestHelpTiers_RealRoot(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte(`version: v1
name: nearmiss
defaults:
  source_language: en
  source: en
  target_languages: [nb]
collections:
  - path: en.json
    target: "{lang}.json"
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "en.json"), []byte(`{"a":"Apple"}`), 0o644))

	for _, args := range [][]string{{"--help"}, {"help"}, {"help", "--all"}, {"help", "languages"}} {
		stdout, stderr := kapiAt(t, dir, args...)
		assert.Empty(t, stderr, "kapi %s writes nothing to standard error", strings.Join(args, " "))
		assert.Contains(t, stdout, "\nLanguages:\n", "kapi %s lists the Languages group", strings.Join(args, " "))
	}

	help, _ := kapiAt(t, dir, "--help")
	assert.NotContains(t, help, "\nAdvanced:\n", "the engine tier is only in kapi help --all")
	assert.NotContains(t, help, `Use "kapi help languages"`, "the group is on the page, so the footer does not point at it")

	_, stderr := kapiAt(t, dir, "status")
	assert.Contains(t, stderr, "defaults.source is not a known field", "a command that loads the recipe reports the near-miss key")
}

// TestHelpTiers_FreshProjectStatus: a project `kapi init` scaffolds has no
// content and no target language. The root help leaves the Languages group
// out and says where it is, and `kapi status` prints a source line naming the
// command that adds content.
func TestHelpTiers_FreshProjectStatus(t *testing.T) {
	dir := t.TempDir()
	kapi(t, "init", "--dir", dir, "--agents", "none")

	help, _ := kapiAt(t, dir, "--help")
	assert.NotContains(t, help, "\nLanguages:\n")
	assert.Contains(t, help, `Use "kapi help languages" to list the commands for other languages.`)

	status, stderr := kapiAt(t, dir, "status")
	assert.Empty(t, stderr)
	assert.Equal(t, "source      no content yet: add files with `kapi add <pattern>`\n", status)

	bare, stderr := kapiAt(t, dir)
	assert.Empty(t, stderr)
	assert.Equal(t, "source      no content yet: add files with `kapi add <pattern>`\n\nkapi --help for the commands\n", bare)
}

// TestLook_RealRoot drives `kapi look` and bare `kapi` through the real root:
// the Map names the file and its finding, bare `kapi` is the Map with the
// line naming where the commands are, and the Reader draws carets under the
// finding with a range.
func TestLook_RealRoot(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte(`version: v1
name: look-e2e
defaults:
  source_language: en
collections:
  - path: "docs/*.md"
`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "guide.md"),
		[]byte("# Guide\n\nOpen the  editor to write.\n"), 0o644))

	look, stderr := kapiAt(t, dir, "look")
	assert.Empty(t, stderr)
	assert.Contains(t, look, "look-e2e · read 1 file just now\n")
	assert.Contains(t, look, "  1 file · 2 blocks · 0 rules · 1 finding\n")
	assert.Contains(t, look, "Files\n  docs/guide.md  2 blocks · 1 finding\n")
	assert.True(t, strings.HasSuffix(look, "\nCheck with `kapi check`. See a file with `kapi look <path>`.\n"), look)

	bare, _ := kapiAt(t, dir)
	assert.Equal(t, look+"\nkapi --help for the commands\n", bare)

	reader, stderr := kapiAt(t, dir, "look", "docs/guide.md")
	assert.Empty(t, stderr)
	assert.True(t, strings.HasPrefix(reader, "docs/guide.md · 2 blocks · 1 finding\n\nh        Guide\nguide/p  Open the  editor to write.\n"), reader)
	assert.Contains(t, reader, "         Hygiene: Content contains consecutive spaces\n")
	assert.Contains(t, reader, "`kapi check docs/guide.md` for the report. `kapi context docs/guide.md` for an assistant.\n")
}
