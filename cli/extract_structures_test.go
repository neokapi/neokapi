package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

const structuresARB = `{
  "@@locale": "en",
  "inboxTitle": "Messages",
  "inboxCount": "{count, plural, =0{No new messages} one{{count} new message} other{{count} new messages}}"
}
`

// arbProject writes a project whose ARB catalog holds a plain message and a
// plural, translated into French, under the isolation contract.
func arbProject(t *testing.T) (recipe, root string) {
	t.Helper()
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")
	write := func(rel, body string) {
		path := filepath.Join(real, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	write("kapi.yaml", `version: v1
name: inbox
defaults:
  source_language: en
  target_languages: [fr]
collections:
  - name: app
    content:
      - path: lib/l10n/app_en.arb
        target: lib/l10n/app_{lang}.arb
        format:
          name: arb
`)
	write("lib/l10n/app_en.arb", structuresARB)
	return filepath.Join(real, project.RecipeFileName), real
}

// An XLIFF unit carries one flat string, which shows one branch of a plural.
// kapi extract leaves a message holding a plural out of the file, and kapi
// merge refuses a returned translation that would flatten one, so the target
// catalog keeps the source's plural rather than one form of it.
func TestExtractAndMergeKeepAPlural(t *testing.T) {
	a := processOnlyApp(t)
	recipe, root := arbProject(t)

	out, err := runCLI(t, NewExtractCmd(a, ExtractCmdOptions{}), "--project", recipe, "--target-lang", "fr", "--no-memory")
	require.NoError(t, err, out)
	xliffPath := filepath.Join(root, "out", "lib-l10n-app_en.en-to-fr.xliff")
	body, err := os.ReadFile(xliffPath)
	require.NoError(t, err)
	xliff := string(body)
	assert.Contains(t, xliff, "<source>Messages</source>")
	assert.NotContains(t, xliff, "new messages", "the plural message is left out of the vendor file")

	// A file an older kapi wrote carries the plural as its other branch; its
	// translation is refused, and the plain message's lands.
	xliff = strings.Replace(xliff, "<source>Messages</source>", "<source>Messages</source>\n        <target>Messages FR</target>", 1)
	xliff = strings.Replace(xliff, "  </file>", `    <unit id="tu2">
      <segment>
        <source>{count} new messages</source>
        <target>{count} nouveaux messages</target>
      </segment>
    </unit>
  </file>`, 1)
	require.NoError(t, os.WriteFile(xliffPath, []byte(xliff), 0o644))
	out, err = runCLI(t, NewMergeCmd(a, MergeCmdOptions{}), "--project", recipe, "-i", xliffPath)
	require.Error(t, err, out)
	assert.Equal(t, ExitGate, ExitCode(nil, err), "a merge that refused a unit exits 3: %s", out)

	merged, err := os.ReadFile(filepath.Join(root, "lib", "l10n", "app_fr.arb"))
	require.NoError(t, err)
	assert.Contains(t, string(merged), `"inboxTitle": "Messages FR"`)
	assert.Contains(t, string(merged), `"inboxCount": "{count, plural, =0{No new messages} one{{count} new message} other{{count} new messages}}"`,
		"the target keeps the source's plural, untranslated, rather than one form of it")
	assert.NotContains(t, string(merged), "nouveaux messages")
}
