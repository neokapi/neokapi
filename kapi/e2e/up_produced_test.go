//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// upFrRecipe targets fr alone, with no gate, so each run writes its
// translations straight into the target files.
const upFrRecipe = `version: v1
name: up-produced-e2e
defaults:
  source_language: en
  target_languages: [fr]
collections:
  - name: app
    path: src/messages_en.json
    target: src/messages_{lang}.json
  - name: docs
    path: docs/en/*.md
    target: docs/{lang}/*.md
`

// writeFile replaces a file of the project at dir.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644))
}

// produced reports what a run's summary says it produced.
func produced(t *testing.T, r *upTwiceResult) int {
	t.Helper()
	require.NotNil(t, r)
	return r.Produced
}

// A source edit under a converged translation re-drafts that one unit, and the
// run reports it as produced: the unit was a draft before and after, but its
// file now holds different content.
func TestUpProducedCountsARedraftedUnit(t *testing.T) {
	dir, _, up := newUpProject(t, upFrRecipe)
	first := up()
	require.Positive(t, produced(t, first))
	before, err := os.ReadFile(filepath.Join(dir, "src", "messages_fr.json"))
	require.NoError(t, err)

	src, err := os.ReadFile(filepath.Join(dir, "src", "messages_en.json"))
	require.NoError(t, err)
	writeFile(t, dir, "src/messages_en.json", strings.Replace(string(src), `"Settings"`, `"Preferences"`, 1))

	second := up()
	assert.Equal(t, 1, produced(t, second), "the run rewrote settings_label and nothing else")
	after, err := os.ReadFile(filepath.Join(dir, "src", "messages_fr.json"))
	require.NoError(t, err)
	assert.NotEqual(t, string(before), string(after))
}

// A target that already holds one of three units gets the other two, and the
// run reports two, whatever the rounded percentages say.
func TestUpProducedCountsTheUnitsItFills(t *testing.T) {
	recipe := `version: v1
name: up-produced-fill-e2e
defaults:
  source_language: en
  target_languages: [fr]
collections:
  - name: app
    path: src/messages_en.json
    target: src/messages_{lang}.json
`
	dir, _, up := newUpProject(t, recipe)
	writeFile(t, dir, "src/messages_en.json", `{
  "a": "Open the settings",
  "b": "Close the window",
  "c": "Save the file"
}
`)
	writeFile(t, dir, "src/messages_fr.json", `{
  "a": "Ouvrir les réglages"
}
`)
	r := up()
	assert.Equal(t, 2, produced(t, r), "the run filled b and c")
	got, err := os.ReadFile(filepath.Join(dir, "src", "messages_fr.json"))
	require.NoError(t, err)
	assert.Contains(t, string(got), "Ouvrir les réglages", "the existing translation is kept")
}

// A unit removed from one file and a unit added to another do not cancel out:
// the run wrote one translation, the added one.
func TestUpProducedCountsAnAdditionBesideARemoval(t *testing.T) {
	dir, _, up := newUpProject(t, upFrRecipe)
	require.Positive(t, produced(t, up()))

	src, err := os.ReadFile(filepath.Join(dir, "src", "messages_en.json"))
	require.NoError(t, err)
	writeFile(t, dir, "src/messages_en.json", strings.Replace(string(src),
		`"encryption_note": "All files use encryption"`,
		`"encryption_note": "All files use encryption",
  "help_link": "Get help"`, 1))
	writeFile(t, dir, "docs/en/guide.md", strings.Replace(upTwiceGuide,
		"- Choose a [language](https://example.com/languages).\n", "", 1))

	r := up()
	assert.Equal(t, 1, produced(t, r), "the run wrote help_link; the removed list item writes no translation")
}

// A run that fails after it wrote a file exits non-zero and says how many
// translations it wrote before the failure.
func TestUpReportsWhatItWroteBeforeAFailure(t *testing.T) {
	dir, dataRoot, up := newUpProject(t, upFrRecipe)
	require.Positive(t, produced(t, up()))

	src, err := os.ReadFile(filepath.Join(dir, "src", "messages_en.json"))
	require.NoError(t, err)
	writeFile(t, dir, "src/messages_en.json", strings.Replace(string(src), `"Settings"`, `"Preferences"`, 1))
	writeFile(t, dir, "docs/en/guide.md", strings.Replace(upTwiceGuide, "Install the tool", "Install the program", 1))
	// The guide's target cannot be written, so the run fails at it.
	guide := filepath.Join(dir, "docs", "fr", "guide.md")
	require.NoError(t, os.Chmod(guide, 0o444))
	require.NoError(t, os.Chmod(filepath.Dir(guide), 0o555))
	t.Cleanup(func() {
		_ = os.Chmod(filepath.Dir(guide), 0o755)
		_ = os.Chmod(guide, 0o644)
	})

	cmd := exec.Command(kapiBin, "up", "-p", "kapi.yaml")
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), isoEnv...),
		"KAPI_DATA_DIR="+dataRoot, "KAPI_AI_PROVIDER=demo", "ANTHROPIC_API_KEY=", "OPENAI_API_KEY=")
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "the run fails:\n%s", out)
	assert.Contains(t, string(out), "wrote 1 translation(s) to target files before it failed", "%s", out)
}

// A recipe under another name than kapi.yaml is the project's too: the run
// records the translations it writes, and reports them as produced.
func TestUpProducedUnderARecipeNamedOtherwise(t *testing.T) {
	dir, dataRoot, _ := newUpProject(t, upFrRecipe)
	require.NoError(t, os.Rename(filepath.Join(dir, "kapi.yaml"), filepath.Join(dir, "site.yaml")))
	cmd := exec.Command(kapiBin, "up", "-p", "site.yaml")
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), isoEnv...),
		"KAPI_DATA_DIR="+dataRoot, "KAPI_AI_PROVIDER=demo", "ANTHROPIC_API_KEY=", "OPENAI_API_KEY=")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	assert.NotContains(t, string(out), "produced 0", "the run wrote translations and says so:\n%s", out)
	assert.Contains(t, string(out), "produced", "%s", out)
}
