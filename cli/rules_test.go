package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host/output"
)

// `kapi rules` end to end on the command line: an assistant notes rules, a
// person decides about them at the prompt and by flag, and the listing, the
// finder and narrowing read what was decided.

// writeRulesProject is a project whose guide writes one name two ways and uses
// "utilise" where the project writes "use".
func writeRulesProject(t *testing.T) string {
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
    source_only: true
    content:
      - path: "docs/*.md"
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "guide.md"),
		[]byte("# Guide\n\nSign in to start.\n\nPlease Sign In first.\n\nSign In again. Some writers utilise it.\n\nOpen the editor.\n"), 0o644))
	return filepath.Join(root, "kapi.yaml")
}

// runRules runs `kapi <args>` on a root holding the rules and context
// commands, answering prompts with answer when it is set.
func runRules(t *testing.T, a *App, answer func(string) string, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "kapi"}
	AddCommandGroups(a, root)
	output.AddPersistentFlags(root.PersistentFlags())
	root.AddCommand(NewRulesCmd(a), NewContextCmd(a))
	root.SetArgs(args)
	root.SilenceUsage, root.SilenceErrors = true, true
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if answer != nil {
		root.SetIn(&promptReader{out: &out, answer: answer})
	}
	err := root.Execute()
	return out.String(), err
}

func mustRules(t *testing.T, a *App, args ...string) string {
	t.Helper()
	out, err := runRules(t, a, nil, args...)
	require.NoError(t, err, out)
	return out
}

// waitingIDs reads the suggestions `kapi context review --peek` lists.
func waitingIDs(t *testing.T, a *App, recipe string) []string {
	t.Helper()
	out := mustRules(t, a, "context", "review", "--peek", "-p", recipe, "--json")
	var digest struct {
		Suggested []struct {
			Groups []struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			} `json:"groups"`
		} `json:"suggested"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &digest), out)
	var ids []string
	for _, th := range digest.Suggested {
		for _, g := range th.Groups {
			for _, it := range g.Items {
				ids = append(ids, it.ID)
			}
		}
	}
	return ids
}

func asTerminal(t *testing.T) {
	t.Helper()
	prev := reviewInteractive
	reviewInteractive = func(cmd *cobra.Command) bool { return output.ResolveFormat(cmd) == output.FormatText }
	t.Cleanup(func() { reviewInteractive = prev })
}

func TestRulesAsksAboutEachSuggestionAtAPrompt(t *testing.T) {
	t.Setenv("KAPI_ACTOR", "person")
	recipe := writeRulesProject(t)
	a := &App{}
	name := noteID(t, a, recipe, "Sign in", "Sign In")
	word := noteID(t, a, recipe, "use", "utilise")
	asTerminal(t)

	// The name: pick its second form, then keep. The word: not a rule.
	out, err := runRules(t, a, func(prompt string) string {
		switch {
		case strings.Contains(prompt, "Which one do your files use?") && !strings.Contains(prompt, "If you keep \"Sign In\""):
			return "2"
		case strings.Contains(prompt, "If you keep \"Sign In\""):
			return "k"
		case strings.Contains(prompt, "utilise"):
			return "n"
		}
		return "q"
	}, "rules", "-p", recipe)
	require.NoError(t, err, out)
	assert.Contains(t, out, "1) Sign in  1 block in 1 file")
	assert.Contains(t, out, "2) Sign In  2 blocks in 1 file")
	assert.Contains(t, out, "Other…")
	assert.Contains(t, out, "If you keep \"Sign in\": the other 4 forms become findings in 2 blocks. Applies to the whole project.")
	assert.Contains(t, out, "If you keep \"Sign In\": the other 4 forms become findings in 1 block. Applies to the whole project.")
	assert.Contains(t, out, "Seen in docs/guide.md")
	assert.Contains(t, out, "k keep, n not a rule, l later, 1-6 pick another, q quit")
	assert.Contains(t, out, "Kept 1, not a rule 1, later 0.")

	assert.Equal(t, "established", statusOf(t, a, recipe, name))
	assert.Equal(t, "dropped", statusOf(t, a, recipe, word))
	assert.Empty(t, waitingIDs(t, a, recipe))
	list := mustRules(t, a, "rules", "list", "-p", recipe)
	assert.Contains(t, list, "Write Sign In, not Sign in,", "the picked form is the one the rule writes")
}

func TestRulesLeavesASuggestionForLaterAtAPrompt(t *testing.T) {
	t.Setenv("KAPI_ACTOR", "person")
	recipe := writeRulesProject(t)
	a := &App{}
	word := noteID(t, a, recipe, "use", "utilise")
	asTerminal(t)

	out, err := runRules(t, a, func(string) string { return "l" }, "rules", "-p", recipe)
	require.NoError(t, err, out)
	assert.Contains(t, out, "Keep this as a rule?")
	assert.Contains(t, out, "If you keep it: \"utilise\" becomes a finding in 1 block. Applies to the whole project.")
	assert.Contains(t, out, "Kept 0, not a rule 0, later 1.")
	assert.Equal(t, "suggested", statusOf(t, a, recipe, word))
	assert.Equal(t, []string{word}, waitingIDs(t, a, recipe))
}

func TestRulesTakesAFormTypedForOther(t *testing.T) {
	t.Setenv("KAPI_ACTOR", "person")
	recipe := writeRulesProject(t)
	a := &App{}
	name := noteID(t, a, recipe, "Sign in", "Sign In")
	asTerminal(t)

	typed := false
	out, err := runRules(t, a, func(prompt string) string {
		switch {
		case strings.HasSuffix(prompt, "Form to write: "):
			typed = true
			return "Log in"
		case typed:
			return "k"
		}
		return "6"
	}, "rules", "-p", recipe)
	require.NoError(t, err, out)
	// A typed form takes the place of the one the suggestion named, as
	// `kapi context review --use` does, so the forms it avoids stay the four.
	assert.Contains(t, out, "If you keep \"Log in\": the other 4 forms become findings in 2 blocks.")
	assert.Equal(t, "established", statusOf(t, a, recipe, name))
	assert.Contains(t, mustRules(t, a, "rules", "list", "-p", recipe), "Write Log in, not ")
}

func TestRulesDecidesByFlag(t *testing.T) {
	t.Setenv("KAPI_ACTOR", "person")
	recipe := writeRulesProject(t)
	a := &App{}
	name := noteID(t, a, recipe, "Sign in", "Sign In")
	word := noteID(t, a, recipe, "use", "utilise")
	other := noteID(t, a, recipe, "editor", "redactor")

	// Without a terminal, kapi rules lists the questions with their ids.
	listed := mustRules(t, a, "rules", "-p", recipe)
	assert.Contains(t, listed, "Which one do your files use?  #"+name[:10])
	assert.Contains(t, listed, "Keep this as a rule?  #"+word[:10])
	assert.Contains(t, listed, "Decide with: kapi rules --keep <id>, --drop <id> or --later <id>")
	assert.Len(t, waitingIDs(t, a, recipe), 3, "listing decides nothing")

	out := mustRules(t, a, "rules", "--keep", name, "--use", "Sign In", "--drop", word, "--later", other, "-p", recipe)
	assert.Contains(t, out, "Dropped")
	assert.Contains(t, out, "Left #"+other[:10]+" for later.")
	assert.Equal(t, "established", statusOf(t, a, recipe, name))
	assert.Equal(t, "dropped", statusOf(t, a, recipe, word))
	assert.Equal(t, "suggested", statusOf(t, a, recipe, other))

	// keep and drop by id are the same decisions.
	mustRules(t, a, "rules", "keep", other, "-p", recipe)
	assert.Equal(t, "established", statusOf(t, a, recipe, other))
	mustRules(t, a, "rules", "drop", other, "-p", recipe)
	assert.Equal(t, "dropped", statusOf(t, a, recipe, other), "dropping a kept rule takes it back out")

	_, err := runRules(t, a, nil, "rules", "--use", "Sign In", "-p", recipe)
	require.ErrorContains(t, err, "--use and --advisory change what --keep or --choose names")

	// The two rules are the kept name and "editor": dropping the editor rule
	// took "redactor" out of the terms store and left the concept's preferred
	// term standing, as a retraction does (host.retractTerm).
	empty := mustRules(t, a, "rules", "-p", recipe)
	assert.Equal(t, "No suggestions to decide. 2 rules · 0 suggested · 0 narrowed.\nRecord one with: kapi context note\n", empty)
}

func TestRulesListFindAndNarrow(t *testing.T) {
	t.Setenv("KAPI_ACTOR", "person")
	recipe := writeRulesProject(t)
	a := &App{}
	name := noteID(t, a, recipe, "Sign in", "Sign In")
	word := noteID(t, a, recipe, "use", "utilise")
	mustRules(t, a, "rules", "keep", word, "-p", recipe)

	out := mustRules(t, a, "rules", "list", "-p", recipe)
	assert.True(t, strings.HasPrefix(out, "1 rule · 1 suggested · 0 narrowed\n\nWords\n"), out)
	assert.Contains(t, out, "Write use, not utilise.  terms · kept by you, ")
	assert.Contains(t, out, "\nSuggested\n  Write Sign in, not Sign In,")

	var listing struct {
		Counts struct {
			Rules     int `json:"rules"`
			Suggested int `json:"suggested"`
			Narrowed  int `json:"narrowed"`
		} `json:"counts"`
		Groups []struct {
			Group string `json:"group"`
			Rules []struct {
				ID      string   `json:"id"`
				From    []string `json:"from"`
				Applies string   `json:"applies"`
				Fails   bool     `json:"fails"`
			} `json:"rules"`
		} `json:"groups"`
	}
	require.NoError(t, json.Unmarshal([]byte(mustRules(t, a, "rules", "list", "-p", recipe, "--json")), &listing))
	assert.Equal(t, 1, listing.Counts.Rules)
	assert.Equal(t, 1, listing.Counts.Suggested)
	require.Len(t, listing.Groups, 2)
	assert.Equal(t, "words", listing.Groups[0].Group)
	assert.Equal(t, word, listing.Groups[0].Rules[0].ID)
	assert.Equal(t, "the whole project", listing.Groups[0].Rules[0].Applies)
	assert.True(t, listing.Groups[0].Rules[0].Fails)
	assert.Equal(t, "suggested", listing.Groups[1].Group)
	assert.Equal(t, name, listing.Groups[1].Rules[0].ID)

	found := mustRules(t, a, "rules", "find", "sign", "-p", recipe)
	assert.Contains(t, found, "Suggested\n  Write Sign in, not Sign In,")
	assert.Contains(t, found, "applies to the whole project")
	var f struct {
		Query     string            `json:"query"`
		Rules     []json.RawMessage `json:"rules"`
		Suggested []struct {
			ID string `json:"id"`
		} `json:"suggested"`
		Decided []json.RawMessage `json:"decided"`
	}
	require.NoError(t, json.Unmarshal([]byte(mustRules(t, a, "rules", "find", "sign", "-p", recipe, "--json")), &f))
	assert.Equal(t, "sign", f.Query)
	require.Len(t, f.Suggested, 1)
	assert.Equal(t, name, f.Suggested[0].ID)
	assert.Empty(t, f.Rules)
	assert.Empty(t, f.Decided)

	// The docs collection sits at the project's own point, which no scope
	// tells apart from the rest of the project.
	_, err := runRules(t, a, nil, "rules", "narrow", word, "--to", "docs", "-p", recipe)
	require.ErrorContains(t, err, "sits at the project's own point")
}
