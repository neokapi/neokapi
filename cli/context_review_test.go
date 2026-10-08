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

// The person's and the assistant's halves of `kapi context`, end to end on the
// command line: an assistant notes a rule, a person decides about it in review,
// both by flag and at the prompt, and a reset takes the context back.

// writeContextReviewProject is a project whose guide writes "use" where writers often
// write "utilise".
func writeContextReviewProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	// Each test names its own project: the workspace, and the context log in
	// it, is shared by every test in this process.
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
		[]byte("# Guide\n\nUse the editor to write. Some writers utilise it.\n"), 0o644))
	return filepath.Join(root, "kapi.yaml")
}

// noteID records a term note as an assistant and returns its id.
func noteID(t *testing.T, a *App, recipe, term, insteadOf string) string {
	t.Helper()
	out := runContext(t, a, "note", "--term", term, "--instead-of", insteadOf,
		"--seen-in", "docs/guide.md", "-p", recipe, "--json")
	var op struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &op), out)
	require.NotEmpty(t, op.ID)
	assert.Equal(t, "suggested", op.Status, "a note is a suggestion")
	return op.ID
}

// statusOf reads one operation's status from `kapi context log`.
func statusOf(t *testing.T, a *App, recipe, id string) string {
	t.Helper()
	out := runContext(t, a, "log", "-p", recipe, "--json")
	var log struct {
		Operations []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"operations"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &log), out)
	for _, op := range log.Operations {
		if op.ID == id {
			return op.Status
		}
	}
	t.Fatalf("operation %s is not in the log", id)
	return ""
}

// promptReader answers each prompt review asks with what answer says about
// the text written since the previous answer.
type promptReader struct {
	out      *bytes.Buffer
	answer   func(string) string
	pending  []byte
	consumed int
}

func (r *promptReader) Read(p []byte) (int, error) {
	if len(r.pending) == 0 {
		text := r.out.String()
		r.pending = []byte(r.answer(text[r.consumed:]) + "\n")
		r.consumed = len(text)
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

// runContextWithInput runs `kapi context` with a person answering its prompts.
func runContextWithInput(t *testing.T, a *App, answer func(string) string, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "kapi"}
	AddCommandGroups(a, root)
	output.AddPersistentFlags(root.PersistentFlags())
	root.AddCommand(NewContextCmd(a))
	root.SetArgs(append([]string{"context"}, args...))
	root.SilenceUsage, root.SilenceErrors = true, true
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(&promptReader{out: &out, answer: answer})
	err := root.Execute()
	return out.String(), err
}

func TestContextReviewDecidesByFlag(t *testing.T) {
	t.Setenv("KAPI_ACTOR", "person")
	recipe := writeContextReviewProject(t)
	a := &App{}

	kept := noteID(t, a, recipe, "use", "utilise")
	dropped := noteID(t, a, recipe, "editor", "redactor")

	out := runContext(t, a, "review", "--keep", kept, "--drop", dropped, "-p", recipe)
	assert.Contains(t, out, "Dropped")
	assert.Equal(t, "established", statusOf(t, a, recipe, kept))
	assert.Equal(t, "dropped", statusOf(t, a, recipe, dropped))

	// Changing your mind about a rule is a new decision: dropping it.
	runContext(t, a, "review", "--drop", kept, "-p", recipe)
	assert.Equal(t, "dropped", statusOf(t, a, recipe, kept))

	_, err := runContextE(t, a, "review", "--widen-to", "workspace", "-p", recipe)
	require.Error(t, err, "--widen-to applies what --keep names")
}

func TestContextReviewListsWhatIsWaitingWithoutATerminal(t *testing.T) {
	t.Setenv("KAPI_ACTOR", "person")
	recipe := writeContextReviewProject(t)
	a := &App{}
	id := noteID(t, a, recipe, "use", "utilise")

	out := runContext(t, a, "review", "--peek", "-p", recipe)
	assert.Contains(t, out, id[:10], "each waiting suggestion is listed with the id a decision takes")
	assert.Contains(t, out, "kapi context review --keep <id>")
	assert.Equal(t, "suggested", statusOf(t, a, recipe, id), "listing decides nothing")
}

func TestContextReviewWalksTheSuggestionsAtAPrompt(t *testing.T) {
	t.Setenv("KAPI_ACTOR", "person")
	recipe := writeContextReviewProject(t)
	a := &App{}
	kept := noteID(t, a, recipe, "use", "utilise")
	dropped := noteID(t, a, recipe, "editor", "redactor")

	prev := reviewInteractive
	reviewInteractive = func(*cobra.Command) bool { return true }
	t.Cleanup(func() { reviewInteractive = prev })

	// Suggestions are asked about in the order the digest lists them; answer
	// by what each one is about.
	out, err := runContextWithInput(t, a, func(prompt string) string {
		if strings.Contains(prompt, "redactor") {
			return "d"
		}
		return "k"
	}, "review", "--peek", "-p", recipe)
	require.NoError(t, err, out)
	assert.Contains(t, out, "Kept 1, dropped 1")
	assert.Equal(t, "established", statusOf(t, a, recipe, kept))
	assert.Equal(t, "dropped", statusOf(t, a, recipe, dropped))
}

func TestContextResetGoesBackAndComesBack(t *testing.T) {
	t.Setenv("KAPI_ACTOR", "person")
	recipe := writeContextReviewProject(t)
	a := &App{}
	first := noteID(t, a, recipe, "use", "utilise")
	second := noteID(t, a, recipe, "editor", "redactor")

	preview := runContext(t, a, "reset", "--before", second, "--dry-run", "-p", recipe)
	assert.Contains(t, preview, "Would set aside 1 suggestion or rule")
	assert.Equal(t, "suggested", statusOf(t, a, recipe, second), "a dry run records nothing")

	out := runContext(t, a, "reset", "--before", second, "-p", recipe, "--json")
	var res struct {
		Reset struct {
			ID string `json:"id"`
		} `json:"reset"`
		SetAside []struct {
			ID string `json:"id"`
		} `json:"set_aside"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res), out)
	require.Len(t, res.SetAside, 1)
	assert.Equal(t, second, res.SetAside[0].ID)
	assert.Equal(t, "reset", statusOf(t, a, recipe, second))
	assert.Equal(t, "suggested", statusOf(t, a, recipe, first), "what came before the point stands")

	runContext(t, a, "reset", "--before", res.Reset.ID, "-p", recipe)
	assert.Equal(t, "suggested", statusOf(t, a, recipe, second), "a reset before the reset brings it back")
}

func TestContextNoteRecordsAChangeAndTakesANoteBack(t *testing.T) {
	recipe := writeContextReviewProject(t)
	a := &App{}
	t.Setenv("KAPI_ACTOR", "agent")
	t.Setenv("KAPI_AGENT_NAME", "codex")
	t.Setenv("KAPI_AGENT_SESSION", "s-cli-note")

	out := runContext(t, a, "note", "--from", "utilise", "--to", "use", "--seen-in", "docs/guide.md",
		"--suggest", "-p", recipe, "--json")
	var op struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &op), out)
	assert.Equal(t, "correct", op.Kind)

	runContext(t, a, "note", "--withdraw", op.ID, "--why", "entered backwards", "-p", recipe)
	assert.Equal(t, "withdrawn", statusOf(t, a, recipe, op.ID))

	_, err := runContextE(t, a, "note", "--from", "utilise", "-p", recipe)
	require.ErrorContains(t, err, "--to")

	_, err = runContextE(t, a, "review", "--keep", op.ID, "-p", recipe)
	require.Error(t, err, "an assistant does not decide")
}
