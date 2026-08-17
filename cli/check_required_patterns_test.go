package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A profile's required patterns were counted as rules and enforced nowhere.
// They are document-scope rules, so `kapi check`'s file loop is where they hold:
// one finding per unsatisfied rule, against the file rather than against any
// block in it, beside the prohibited patterns that fire per block.

// requiredPatternProject writes a project bound to a voice profile that declares
// one prohibited pattern and one required pattern, plus two markdown files: one
// that carries the required line and one that does not. Both carry the
// prohibited word, so a run that reports neither half is visibly wrong.
func requiredPatternProject(t *testing.T) (root, carries, missing string) {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root = writeProjectRecipe(t, `version: v1
name: required-patterns
defaults:
  source_language: en
  voice:
    profile_file: voice.yaml
collections:
  - name: docs
    content:
      - path: "docs/*.md"
`)
	write := func(rel, body string) string {
		full := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
		return full
	}
	write("voice.yaml", `name: Required Patterns
style:
  prohibited_patterns:
    - regex: '\bseamless\b'
      description: Marketing superlative
      severity: major
  required_patterns:
    - regex: '(?i)\bstart free trial\b'
      description: Every landing page carries the call to action
      severity: major
`)
	carries = write("docs/carries.md", "A seamless page.\n\nStart free trial today.\n")
	missing = write("docs/missing.md", "A seamless page.\n\nNothing asks the reader to act.\n")
	return root, carries, missing
}

func TestCheck_RequiredPatternIsEnforcedOverTheDocument(t *testing.T) {
	root, carries, missing := requiredPatternProject(t)
	t.Chdir(root)

	a := &App{SourceLang: "en"}

	missingReport, err := a.ComputeCheck(NewCheckCmd(a), []string{missing})
	require.NoError(t, err)
	msgs := findingMessages(missingReport, "voice")
	assert.Contains(t, joined(msgs), "Required pattern absent: Every landing page carries the call to action",
		"the rule the profile declares must reach a finding: %v", msgs)
	assert.Contains(t, joined(msgs), "Prohibited pattern: Marketing superlative",
		"the prohibited half still fires per block")

	// The absence is the document's, not a block's: it names the file only.
	var required []check.Diagnostic
	for _, d := range missingReport.Findings {
		if d.Rule == "voice.style" && d.Location.Block == "" {
			required = append(required, d)
		}
	}
	require.Len(t, required, 1, "one finding per unsatisfied rule, not one per block")
	assert.Equal(t, "missing.md", filepath.Base(required[0].Location.File))

	carriesReport, err := a.ComputeCheck(NewCheckCmd(a), []string{carries})
	require.NoError(t, err)
	carriesMsgs := findingMessages(carriesReport, "voice")
	assert.NotContains(t, joined(carriesMsgs), "Required pattern absent",
		"a document that carries the required text satisfies the rule: %v", carriesMsgs)
	assert.Contains(t, joined(carriesMsgs), "Prohibited pattern: Marketing superlative")
}

// The ship gate is the pre-release bar over the same profile, so it applies the
// same rules over the same reading: the document-scope half beside the per-block
// half, and each file read as the recipe declares it.
func TestCheckShip_RequiredPatternReachesTheVoiceGate(t *testing.T) {
	root, _, missing := requiredPatternProject(t)
	t.Chdir(root)

	a := &App{}
	cmd := NewCheckCmd(a)
	require.NoError(t, cmd.Flags().Set("ship", "true"))
	require.NoError(t, cmd.Flags().Set("no-fail", "true"))

	out, err := captureStdout(t, func() error { return a.RunCheck(cmd, []string{missing}) })
	require.NoError(t, err)
	assert.Contains(t, out, "Required pattern absent: Every landing page carries the call to action",
		"the ship gate applies every rule the profile counts: %s", out)
}

// A required pattern holds over the whole file, so text in any block satisfies
// it — the rule must not be re-judged per block, which would flag every
// paragraph that does not repeat the notice.
func TestCheck_RequiredPatternIsNotJudgedPerBlock(t *testing.T) {
	root, carries, _ := requiredPatternProject(t)
	t.Chdir(root)

	a := &App{SourceLang: "en"}
	report, err := a.ComputeCheck(NewCheckCmd(a), []string{carries})
	require.NoError(t, err)

	for _, d := range report.Findings {
		assert.NotContains(t, d.Message, "Required pattern absent",
			"the second paragraph carries the call to action for the whole file")
	}
}
