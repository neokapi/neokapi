//go:build e2e

package e2e

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const markupRecipe = `version: v1
name: markup
defaults:
  source_language: en
  target_languages: [nb]
collections:
  - name: docs
    content:
      - path: docs/en/*.md
        format:
          name: markdown
        target: docs/{lang}/{name}.md
`

const markupPage = `# Welcome

Harbor Help connects you by **video appointment** in the [Harbor app](https://harbor.example/app).
`

var xliffSource = regexp.MustCompile(`(?s)<source>(.*?)</source>`)

// extractMarkupProject writes a Markdown project, extracts it to XLIFF for
// nb, and returns the project directory and the extracted file.
func extractMarkupProject(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte(markupRecipe), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs", "en"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "en", "welcome.md"), []byte(markupPage), 0o600))
	kapiIn(t, dir, "extract", "-p", "kapi.yaml", "--target-lang", "nb")
	xliffs, err := filepath.Glob(filepath.Join(dir, "out", "*.xliff"))
	require.NoError(t, err)
	require.Len(t, xliffs, 1)
	return dir, xliffs[0]
}

// fillTargets gives each unit of an extracted XLIFF 2 file a target: its
// source as fill turns it.
func fillTargets(t *testing.T, path string, fill func(string) string) {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	out := xliffSource.ReplaceAllStringFunc(string(body), func(m string) string {
		src := xliffSource.FindStringSubmatch(m)[1]
		return m + "<target>" + fill(src) + "</target>"
	})
	require.NoError(t, os.WriteFile(path, []byte(out), 0o600))
}

// A Markdown page extracted to XLIFF carries its inline markup as XLIFF codes,
// so a translation that keeps them merges back with the markup in place.
func TestXLIFFRoundTripKeepsMarkdownMarkup(t *testing.T) {
	dir, xliff := extractMarkupProject(t)
	body := readFile(t, xliff)
	assert.Contains(t, body, `<pc id="1"`, "the bold span is an XLIFF code")
	assert.NotContains(t, body, "**video", "the markup is not text to translate")

	r := strings.NewReplacer("Harbor Help connects you by", "Harbor Help kobler deg via", " in the ", " i ", "Welcome", "Velkommen")
	fillTargets(t, xliff, r.Replace)
	kapiIn(t, dir, "merge", "-p", "kapi.yaml", "-i", xliff)
	nb := readFile(t, filepath.Join(dir, "docs", "nb", "welcome.md"))
	assert.Contains(t, nb, "Harbor Help kobler deg via **video appointment** i [Harbor app](https://harbor.example/app).")
}

// A translation that drops a code is refused naming the block by its key and
// edition and the codes as a read shows them, and the merge exits 3, so a
// command chained after it does not go on with a file half in English.
func TestXLIFFMergeRefusesADroppedCodeAndExitsThree(t *testing.T) {
	dir, xliff := extractMarkupProject(t)
	bold := regexp.MustCompile(`<pc id="1"[^>]*>(.*?)</pc>`)
	fillTargets(t, xliff, func(src string) string { return bold.ReplaceAllString(src, "$1") })
	cmd := exec.Command(kapiBin, "merge", "-p", "kapi.yaml", "-i", xliff)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), isoEnv...)
	out, err := cmd.CombinedOutput()
	exit, ok := errors.AsType[*exec.ExitError](err)
	require.True(t, ok, "the merge exits non-zero when a unit is refused:\n%s", out)
	assert.Equal(t, 3, exit.ExitCode(), string(out))
	assert.Contains(t, string(out), `welcome/p@nb not merged: guard (codes_changed): expected <x id="1"/> <x id="/1"/>, found none`)
	assert.NotContains(t, string(out), "block tu")
	assert.Contains(t, string(out), "refused=1")
}
