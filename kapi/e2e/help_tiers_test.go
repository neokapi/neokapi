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
}
