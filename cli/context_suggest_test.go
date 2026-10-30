package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `kapi context suggest` on the command line: it prints what it filed in the
// queue's own words, --dry-run files nothing, and --json is the list.

// writeSuggestProject is a one-language project whose files write one word
// two ways and address the reader as you.
func writeSuggestProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	name := strings.ToLower(strings.NewReplacer("/", "-", "_", "-").Replace(t.Name()))
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(`version: v1
name: `+name+`
defaults:
  source_language: en
collections:
  - name: docs
    content:
      - path: "docs/*.md"
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte(
		"# Guide\n\nYou get an email when your plan renews.\n\nYou can reply to the email.\n\nYou can also send an e-mail.\n"), 0o644))
	return filepath.Join(root, "kapi.yaml")
}

func TestContextSuggestPrintsWhatItFiled(t *testing.T) {
	recipe := writeSuggestProject(t)
	a := &App{}

	out := runContext(t, a, "suggest", "-p", recipe)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 4, out)
	assert.Regexp(t, `^Filed\s+\S+\s+Write email, not e-mail or e mail\. \(email 2, e-mail 1; 3 blocks in 1 file\)$`, lines[0])
	assert.Regexp(t, `^Filed\s+\S+\s+The files address the reader as you \(person_pov: second\): 3 of 3 blocks with a personal pronoun say you, and none says we\.$`, lines[1])
	assert.Equal(t, "", lines[2])
	assert.Equal(t, "Decide with kapi context review.", lines[3])

	again := runContext(t, a, "suggest", "-p", recipe)
	assert.Regexp(t, `(?m)^Unchanged\s+\S+\s+Write email, not e-mail or e mail\.`, again)

	log := runContext(t, a, "log", "-p", recipe, "--status", "suggested", "--json")
	var ops struct {
		Operations []struct {
			Actor struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"actor"`
		} `json:"operations"`
	}
	require.NoError(t, json.Unmarshal([]byte(log), &ops), log)
	require.Len(t, ops.Operations, 2, "a second run filed nothing")
	for _, op := range ops.Operations {
		assert.Equal(t, "tool", op.Actor.Kind)
		assert.Equal(t, "kapi", op.Actor.Name)
	}
}

func TestContextSuggestDryRunFilesNothing(t *testing.T) {
	recipe := writeSuggestProject(t)
	a := &App{}

	out := runContext(t, a, "suggest", "--dry-run", "-p", recipe)
	assert.Regexp(t, `(?m)^Would file\s+Write email, not e-mail or e mail\.`, out)
	assert.NotContains(t, out, "Decide with")

	log := runContext(t, a, "log", "-p", recipe, "--json")
	var ops struct {
		Operations []json.RawMessage `json:"operations"`
	}
	require.NoError(t, json.Unmarshal([]byte(log), &ops), log)
	assert.Empty(t, ops.Operations)
}

func TestContextSuggestJSON(t *testing.T) {
	recipe := writeSuggestProject(t)
	a := &App{}

	out := runContext(t, a, "suggest", "-p", recipe, "--json")
	var res struct {
		Project     string `json:"project"`
		Files       int    `json:"files"`
		Blocks      int    `json:"blocks"`
		Suggestions []struct {
			Kind     string `json:"kind"`
			Outcome  string `json:"outcome"`
			ID       string `json:"id"`
			Short    string `json:"short"`
			Sentence string `json:"sentence"`
			Forms    []struct {
				Form  string `json:"form"`
				Count int    `json:"count"`
			} `json:"forms"`
			Blocks int      `json:"blocks"`
			Files  []string `json:"files"`
			Quote  *struct {
				Path  string `json:"path"`
				Quote string `json:"quote"`
			} `json:"quote"`
		} `json:"suggestions"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res), out)
	assert.Equal(t, 1, res.Files)
	assert.Equal(t, 4, res.Blocks)
	require.Len(t, res.Suggestions, 2)
	s := res.Suggestions[0]
	assert.Equal(t, "spelling", s.Kind)
	assert.Equal(t, "filed", s.Outcome)
	assert.NotEmpty(t, s.ID)
	assert.NotEmpty(t, s.Short)
	assert.Equal(t, []string{"docs/guide.md"}, s.Files)
	require.Len(t, s.Forms, 2)
	assert.Equal(t, "email", s.Forms[0].Form)
	assert.Equal(t, 2, s.Forms[0].Count)
	require.NotNil(t, s.Quote)
	assert.Equal(t, "You can also send an e-mail.", s.Quote.Quote)
	assert.Equal(t, "address", res.Suggestions[1].Kind)
}

// TestContextSuggestNamesNoLanguageWork: in a one-language project the pass
// and its help name no work in other languages.
func TestContextSuggestNamesNoLanguageWork(t *testing.T) {
	recipe := writeSuggestProject(t)
	a := &App{}
	banned := languageWork
	for _, args := range [][]string{
		{"suggest", "-p", recipe},
		{"suggest", "--dry-run", "-p", recipe},
		{"suggest", "--help"},
	} {
		out := runContext(t, a, args...)
		require.NotEmpty(t, strings.TrimSpace(out))
		for line := range strings.SplitSeq(out, "\n") {
			if banned.MatchString(line) {
				t.Errorf("kapi context %s names language work: %q", strings.Join(args, " "), line)
			}
		}
	}
}
