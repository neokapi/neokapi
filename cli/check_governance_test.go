package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `kapi check` gates content against the vocabulary in force where that content
// sits. These tests drive the verb itself over a fixture project, so what they
// assert is what a user sees: a file under a content item's own `channel:` is
// checked against a different profile's terms than its neighbour, and a profile
// whose validity window has closed stops supplying terms at all.

// governedCheckProject writes a project whose docs collection is governed by
// `promo`, with its legal sub-tree routed to `legal` by a per-item channel. Each
// profile forbids a different word, so which findings appear says which
// vocabulary fired. window is inserted into the promo profile (e.g. a
// `valid_to:` line) and may be empty.
func governedCheckProject(t *testing.T, window string) (root, guide, legal string) {
	t.Helper()
	root = writeProjectRecipe(t, `version: v1
name: governed-check
defaults:
  source_language: en
  target_languages: [fr]
  voice:
    profile_file: house-voice.yaml
profiles:
  promo:
    channels: [docs]
    voice: promo-voice.yaml
`+window+`  legal:
    channels: [docs]
    voice: legal-voice.yaml
collections:
  - name: docs
    channel: promo/docs
    content:
      - path: "docs/legal/**/*.json"
        channel: legal/docs
      - path: "docs/**/*.json"
`)
	write := func(rel, body string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}
	write("house-voice.yaml", forbidding("House Style", "house", "utilize", "use"))
	write("promo-voice.yaml", forbidding("Promo Voice", "promo", "cheap", "affordable"))
	write("legal-voice.yaml", forbidding("Legal Voice", "legal", "guarantee", "warrant"))

	// Both files carry every forbidden word, so the findings that appear are
	// decided entirely by which profile governs the file.
	body := `{"copy": "A cheap utilize guarantee."}`
	write("docs/guide.json", body)
	write("docs/legal/eula.json", body)

	return root, filepath.Join(root, "docs", "guide.json"),
		filepath.Join(root, "docs", "legal", "eula.json")
}

// forbidding renders a voice profile that forbids one term.
func forbidding(name, id, term, replacement string) string {
	return "id: " + id + "\nname: " + name + `
vocabulary:
  forbidden_terms:
    - term: ` + term + `
      replacement: ` + replacement + `
      severity: major
`
}

// findingMessages collects a report's finding messages for a family.
func findingMessages(r check.Report, family string) []string {
	var out []string
	for _, d := range r.Findings {
		if d.Check == family {
			out = append(out, d.Message)
		}
	}
	return out
}

// TestCheck_ItemChannelOverrideSelectsTheGoverningVocabulary is the per-item
// channel made real in a check: two files of one collection are gated against
// two different profiles' terms.
func TestCheck_ItemChannelOverrideSelectsTheGoverningVocabulary(t *testing.T) {
	root, guide, legal := governedCheckProject(t, "")
	t.Chdir(root)

	a := &App{SourceLang: "en"}
	guideReport, err := a.ComputeCheck(NewCheckCmd(a), []string{guide})
	require.NoError(t, err)
	guideMsgs := findingMessages(guideReport, "voice")
	assert.Contains(t, joined(guideMsgs), "cheap", "the collection's channel governs the ordinary file: %v", guideMsgs)
	assert.NotContains(t, joined(guideMsgs), "guarantee", "the other profile's terms must not fire here")

	legalReport, err := a.ComputeCheck(NewCheckCmd(a), []string{legal})
	require.NoError(t, err)
	legalMsgs := findingMessages(legalReport, "voice")
	assert.Contains(t, joined(legalMsgs), "guarantee", "the item's own channel governs the file it matches: %v", legalMsgs)
	assert.NotContains(t, joined(legalMsgs), "cheap", "the collection's terms must not reach the overridden file")
}

// TestCheck_ExpiredProfileFallsThroughWithAVisibleNote covers both boundaries
// and the control. A profile outside its window supplies no terms; the project
// default's fire instead, and the swap is reported on stderr.
func TestCheck_ExpiredProfileFallsThroughWithAVisibleNote(t *testing.T) {
	tests := []struct {
		name     string
		window   string
		wantTerm string
		wantGone string
		wantNote string
	}{
		{
			name:     "after valid_to the profile supplies no terms",
			window:   "    valid_to: 2020-01-01\n",
			wantTerm: "utilize",
			wantGone: "cheap",
			wantNote: `governance: profile "promo" expired 2020-01-01; governing with the project default`,
		},
		{
			name:     "before valid_from it supplies none yet",
			window:   "    valid_from: 2099-01-01\n",
			wantTerm: "utilize",
			wantGone: "cheap",
			wantNote: `governance: profile "promo" is not in force until 2099-01-01; governing with the project default`,
		},
		{
			name:     "an always-valid profile is untouched",
			window:   "",
			wantTerm: "cheap",
			wantGone: "utilize",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, guide, _ := governedCheckProject(t, tt.window)
			t.Chdir(root)

			a := &App{SourceLang: "en"}
			cmd := NewCheckCmd(a)
			var stderr bytes.Buffer
			cmd.SetErr(&stderr)

			report, err := a.ComputeCheck(cmd, []string{guide})
			require.NoError(t, err)
			msgs := joined(findingMessages(report, "voice"))
			assert.Contains(t, msgs, tt.wantTerm)
			assert.NotContains(t, msgs, tt.wantGone)

			if tt.wantNote == "" {
				assert.NotContains(t, stderr.String(), "governance:", "nothing lapsed, so nothing to report")
				return
			}
			assert.Contains(t, stderr.String(), tt.wantNote,
				"a check that swapped vocabularies on a date must say so")
		})
	}
}

// joined renders finding messages as one string for substring assertions.
func joined(msgs []string) string {
	var b bytes.Buffer
	for _, m := range msgs {
		b.WriteString(m)
		b.WriteByte('\n')
	}
	return b.String()
}
