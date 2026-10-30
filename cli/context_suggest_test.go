package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/credentials/providerenv"
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
	assert.Empty(t, lines[2])
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

// useDemoAI makes the demo provider the configured one and clears every
// provider key the environment could hold, so no test reaches a real model.
func useDemoAI(t *testing.T) {
	t.Helper()
	clearAIEnv(t)
	t.Setenv("KAPI_AI_PROVIDER", "demo")
}

func clearAIEnv(t *testing.T) {
	t.Helper()
	for _, vars := range providerenv.Vars {
		for _, v := range vars {
			t.Setenv(v, "")
		}
	}
	t.Setenv("KAPI_AI_PROVIDER", "")
	t.Setenv("KAPI_AI_MODEL", "")
}

// writeDeeperProject is writeSuggestProject with a product name the files
// write in two blocks.
func writeDeeperProject(t *testing.T) string {
	t.Helper()
	recipe := writeSuggestProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(recipe), "docs", "help.md"), []byte(
		"# Help\n\nAsk Harbor Help about your plan.\n\nHarbor Help answers within a day.\n"), 0o644))
	return recipe
}

func TestContextSuggestAIFilesWhatTheModelProposes(t *testing.T) {
	useDemoAI(t)
	recipe := writeDeeperProject(t)
	a := &App{}

	out := runContext(t, a, "suggest", "--ai", "-p", recipe)
	assert.Regexp(t, `(?m)^Filed\s+\S+\s+Write email, not e-mail or e mail\.`, out, "the pass over the files runs first")
	assert.Regexp(t, `(?m)^Filed\s+\S+\s+Write Harbor Help, not Harbor-Help or HarborHelp\. \(2 blocks in 1 file\)$`, out)
	assert.Regexp(t, `(?m)^Known\s+The files address the reader as you \(person_pov: second\)\.$`, out,
		"the pass filed that already, so the model's reading of it is not filed again")
	assert.Regexp(t, `(?m)^Filed\s+\S+\s+The files keep sentences short \(sentence_length: short\)\.$`, out)
	assert.Regexp(t, `(?m)^demo-stub read 7 blocks in 2 files \(\d+ tokens\):$`, out)
	modelAt := strings.Index(out, "demo-stub read")
	assert.Less(t, strings.Index(out, "Write email"), modelAt, "the pass's lines come first")
	assert.Greater(t, strings.Index(out, "Write Harbor Help"), modelAt, "the model is named before its lines")
	assert.Equal(t, 1, strings.Count(out, "Decide with kapi context review."), out)

	again := runContext(t, a, "suggest", "--ai", "-p", recipe)
	assert.NotRegexp(t, `(?m)^Filed`, again, "a second run files nothing")
	assert.Regexp(t, `(?m)^Unchanged\s+\S+\s+Write Harbor Help`, again)

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
	names := map[string]int{}
	for _, op := range ops.Operations {
		assert.Equal(t, "tool", op.Actor.Kind)
		names[op.Actor.Name]++
	}
	assert.Equal(t, 2, names["kapi"], "the pass over the files files as kapi")
	assert.Positive(t, names["demo-stub"], "the deeper look files as kapi with the model's name")
}

func TestContextSuggestAIDryRunFilesNothing(t *testing.T) {
	useDemoAI(t)
	recipe := writeDeeperProject(t)
	a := &App{}

	out := runContext(t, a, "suggest", "--ai", "--dry-run", "-p", recipe)
	assert.Regexp(t, `(?m)^Would file\s+Write Harbor Help, not Harbor-Help or HarborHelp\.`, out)
	assert.Equal(t, 1, strings.Count(out, "Would file    The files address the reader as you"), out)
	assert.Regexp(t, `(?m)^Known\s+The files address the reader as you \(person_pov: second\)\.$`, out,
		"the pass would file it in the same run, so the model's reading of it is known")
	assert.Regexp(t, `(?m)^demo-stub read 7 blocks in 2 files`, out, "a dry run still asks the model")
	assert.NotContains(t, out, "Decide with")

	log := runContext(t, a, "log", "-p", recipe, "--json")
	var ops struct {
		Operations []json.RawMessage `json:"operations"`
	}
	require.NoError(t, json.Unmarshal([]byte(log), &ops), log)
	assert.Empty(t, ops.Operations)
}

func TestContextSuggestAIJSON(t *testing.T) {
	useDemoAI(t)
	recipe := writeDeeperProject(t)
	a := &App{}

	out := runContext(t, a, "suggest", "--ai", "--dry-run", "-p", recipe, "--json")
	var res struct {
		Suggestions []json.RawMessage `json:"suggestions"`
		AI          *struct {
			Provider    string `json:"provider"`
			Model       string `json:"model"`
			Asked       int    `json:"asked"`
			Suggestions []struct {
				Kind    string `json:"kind"`
				Outcome string `json:"outcome"`
				Measure string `json:"measure"`
				Value   string `json:"value"`
			} `json:"suggestions"`
		} `json:"ai"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res), out)
	require.NotNil(t, res.AI)
	assert.Equal(t, "demo", res.AI.Provider)
	assert.Equal(t, "demo-stub", res.AI.Model)
	assert.Equal(t, 7, res.AI.Asked)
	kinds := map[string]bool{}
	for _, s := range res.AI.Suggestions {
		kinds[s.Kind] = true
		if s.Kind == "voice" {
			assert.NotEmpty(t, s.Measure)
			assert.NotEmpty(t, s.Value)
		}
	}
	assert.True(t, kinds["term"] && kinds["voice"], "%+v", res.AI.Suggestions)

	plain := runContext(t, a, "suggest", "-p", recipe, "--dry-run", "--json")
	assert.NotContains(t, plain, `"ai"`, "without --ai the result carries no deeper look")
}

func TestContextSuggestAIRefusesWithoutAProvider(t *testing.T) {
	for _, tt := range []struct {
		name string
		env  map[string]string
	}{
		{name: "nothing configured"},
		{name: "a key for a provider the run would not build", env: map[string]string{"OPENAI_API_KEY": "sk-test-not-a-key"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			clearAIEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			recipe := writeDeeperProject(t)
			a := &App{}

			_, err := runContextE(t, a, "suggest", "--ai", "-p", recipe)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no AI provider configured")

			log := runContext(t, a, "log", "-p", recipe, "--json")
			var ops struct {
				Operations []json.RawMessage `json:"operations"`
			}
			require.NoError(t, json.Unmarshal([]byte(log), &ops), log)
			assert.Empty(t, ops.Operations, "the refusal comes before the pass files anything")
		})
	}
}

// TestContextSuggestNamesNoLanguageWork: in a one-language project the pass
// and its help name no work in other languages.
func TestContextSuggestNamesNoLanguageWork(t *testing.T) {
	useDemoAI(t)
	recipe := writeDeeperProject(t)
	a := &App{}
	banned := languageWork
	for _, args := range [][]string{
		{"suggest", "-p", recipe},
		{"suggest", "--dry-run", "-p", recipe},
		{"suggest", "--ai", "--dry-run", "-p", recipe},
		{"suggest", "--ai", "-p", recipe},
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
