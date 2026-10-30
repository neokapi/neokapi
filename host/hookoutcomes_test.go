package host

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
)

// hookGuide is the one content file of the hook fixture. "Log in" breaks the
// kept rule.
const hookGuide = "# Welcome\n\nSign in to start.\n\nLog in with your account.\n"

// hookProject writes a source-only project and keeps the rule "Sign in, not
// Log in" in it, so the Stop hook's terminology gate fails on the guide.
func hookProject(t *testing.T, app *App, name string) string {
	t.Helper()
	root := t.TempDir()
	recipe := "version: v1\nid: " + projectIDFor(name) + "\nname: " + name + "\n" + `defaults:
  source_language: en
collections:
  - name: Docs
    source_only: true
    content:
      - path: "docs/**/*.md"
`
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte(hookGuide), 0o644))
	agent := contextop.Actor{Kind: contextop.ActorAgent, Name: "claude-code", Session: "setup"}
	op, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor: agent, Project: recipeOf(root), Term: "Sign in", InsteadOf: []string{"Log in"},
		Evidence: []contextop.Evidence{{Path: "docs/guide.md"}},
	})
	require.NoError(t, err)
	_, err = app.KeepContextOperations(t.Context(), ContextKeepRequest{
		Actor: contextop.Actor{Kind: contextop.ActorPerson}, Project: recipeOf(root), ID: op.ID,
	})
	require.NoError(t, err)
	return root
}

// hookApp is one App over a workspace of its own.
func hookApp(t *testing.T) *App {
	t.Helper()
	app, _ := contextOpsApp(t)
	// The hook finds its project from the session directory.
	t.Setenv("KAPI_NO_PROJECT", "")
	return app
}

// transcript writes a Claude Code transcript: a person's message, then the
// assistant editing each file, the last entry's id being turn.
func transcript(t *testing.T, path, turn string, files ...string) {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, `{"type":"user","uuid":"%s-prompt","message":{"role":"user","content":"Tidy the guide."}}`+"\n", turn)
	for i, f := range files {
		fmt.Fprintf(&b, `{"type":"assistant","uuid":"%s-edit-%d","message":{"role":"assistant","content":[{"type":"tool_use","id":"t%d","name":"Edit","input":{"file_path":%q,"old_string":"a","new_string":"b"}}]}}`+"\n", turn, i, i, f)
		fmt.Fprintf(&b, `{"type":"user","uuid":"%s-result-%d","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t%d","content":"ok"}]}}`+"\n", turn, i, i)
	}
	fmt.Fprintf(&b, `{"type":"assistant","uuid":"%s","message":{"role":"assistant","content":[{"type":"text","text":"Done."}]}}`+"\n", turn)
	require.NoError(t, os.WriteFile(path, []byte(b.String()), 0o644))
}

// runStop runs the Stop hook with a payload for session in root, and returns
// what it printed on each channel.
func runStop(t *testing.T, app *App, root, session, transcriptPath string, asJSON bool) (string, string) {
	t.Helper()
	t.Chdir(root)
	payload, err := json.Marshal(map[string]any{
		"hook_event_name": "Stop", "cwd": root, "session_id": session, "transcript_path": transcriptPath,
	})
	require.NoError(t, err)
	cmd := NewEnvCommand(t.Context(), "stop")
	cmd.Flags().Bool("json", false, "")
	if asJSON {
		require.NoError(t, cmd.Flags().Set("json", "true"))
	}
	var out, errOut bytes.Buffer
	cmd.SetIn(bytes.NewReader(payload))
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	require.NoError(t, app.RunHookStop(cmd), "the hook's verdict is its stdout, never its exit")
	return out.String(), errOut.String()
}

// checkRuns reads the project's check operations, newest first.
func checkRuns(t *testing.T, app *App, root string) []ContextOperation {
	t.Helper()
	log, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root), Kinds: []contextop.Kind{contextop.KindCheck}})
	require.NoError(t, err)
	return log.Operations
}

func TestHookStop_RecordsTheRunAsTheSessionsAssistant(t *testing.T) {
	app := hookApp(t)
	root := hookProject(t, app, "hookrecords")
	tr := filepath.Join(t.TempDir(), "transcript.jsonl")
	transcript(t, tr, "turn-1", filepath.Join(root, "docs", "guide.md"))

	stdout, stderr := runStop(t, app, root, "s1", tr, false)
	var dec StopHookDecision
	require.NoError(t, json.Unmarshal([]byte(stdout), &dec), stdout)
	assert.Equal(t, "block", dec.Decision, "recording the run changes no decision")
	assert.Empty(t, stderr)

	runs := checkRuns(t, app, root)
	require.Len(t, runs, 1)
	r := runs[0]
	assert.Equal(t, contextop.KindCheck, r.Kind)
	assert.Equal(t, contextop.Actor{Kind: contextop.ActorAgent, Name: "claude-code", Session: "s1"}, r.Actor)
	require.NotNil(t, r.Check)
	assert.Equal(t, "stop", r.Check.Hook)
	assert.Equal(t, "turn-1", r.Check.Turn)
	assert.Equal(t, "failed", r.Check.Verdict)
	assert.False(t, r.Check.Clean)
	assert.Equal(t, []string{"docs/guide.md"}, r.Check.Files)
	assert.True(t, r.Check.Edited, "the files come from the turn's edits")
	assert.Equal(t, 1, r.Check.Covered)
	assert.Equal(t, 1, r.Check.Failing)
	assert.Equal(t, []string{"terms.vocabulary"}, r.Check.Rules)
	require.Len(t, r.Check.Findings, 1)
	assert.Equal(t, contextop.CheckFinding{File: "docs/guide.md", Rule: "terms.vocabulary", Quote: "Log in", Block: r.Check.Findings[0].Block}, r.Check.Findings[0])

	var line bytes.Buffer
	require.NoError(t, ContextOperationList{Operations: runs}.FormatText(&line))
	assert.Contains(t, line.String(), "agent claude-code/s1  checked 1 file · 1 failing (terms.vocabulary)")
	assert.Contains(t, line.String(), "  after editing docs/guide.md")
}

func TestHookStop_RecordsOneRunPerTurn(t *testing.T) {
	app := hookApp(t)
	root := hookProject(t, app, "hookturns")
	tr := filepath.Join(t.TempDir(), "transcript.jsonl")
	transcript(t, tr, "turn-1", filepath.Join(root, "docs", "guide.md"))

	first, _ := runStop(t, app, root, "s1", tr, true)
	again, _ := runStop(t, app, root, "s1", tr, true)
	var a, b StopHookResult
	require.NoError(t, json.Unmarshal([]byte(first), &a), first)
	require.NoError(t, json.Unmarshal([]byte(again), &b), again)
	require.NotEmpty(t, a.Record, "--json names the record")
	assert.Equal(t, "block", a.Decision)
	assert.Equal(t, a.Record, b.Record, "the hook running twice for one turn is one record")
	assert.Len(t, checkRuns(t, app, root), 1)

	// The same turn in another session is another run.
	runStop(t, app, root, "s2", tr, false)
	assert.Len(t, checkRuns(t, app, root), 2)
}

func TestSessionOutcomes_ACleanRunAfterAFailingOneCaughtTheFinding(t *testing.T) {
	app := hookApp(t)
	root := hookProject(t, app, "hookcaught")
	guide := filepath.Join(root, "docs", "guide.md")
	tr := filepath.Join(t.TempDir(), "transcript.jsonl")

	transcript(t, tr, "turn-1", guide)
	runStop(t, app, root, "s1", tr, false)
	// Another session fails on the same file and fixes nothing.
	runStop(t, app, root, "s2", filepath.Join(t.TempDir(), "absent.jsonl"), false)

	require.NoError(t, os.WriteFile(guide, []byte(strings.Replace(hookGuide, "Log in", "Sign in", 1)), 0o644))
	transcript(t, tr, "turn-2", guide)
	stdout, _ := runStop(t, app, root, "s1", tr, false)
	assert.Empty(t, stdout, "a clean project lets the assistant stop")

	got, err := app.SessionOutcomes(t.Context(), ContextSessionRequest{Project: recipeOf(root), Session: "s1"})
	require.NoError(t, err)
	assert.Equal(t, SessionOutcomes{Session: "s1", Checked: 2, Failing: 0, Caught: 1, Files: 1}, got)

	other, err := app.SessionOutcomes(t.Context(), ContextSessionRequest{Project: recipeOf(root), Session: "s2"})
	require.NoError(t, err)
	assert.Equal(t, SessionOutcomes{Session: "s2", Checked: 1, Failing: 1, Caught: 0, Files: 0}, other,
		"a finding fixed in another session is not this session's catch")

	// A third session's clean run caught nothing: it never saw the finding.
	runStop(t, app, root, "s3", tr, false)
	third, err := app.SessionOutcomes(t.Context(), ContextSessionRequest{Project: recipeOf(root), Session: "s3"})
	require.NoError(t, err)
	assert.Zero(t, third.Caught)

	runs := checkRuns(t, app, root)
	var log bytes.Buffer
	require.NoError(t, ContextOperationList{Operations: runs}.FormatText(&log))
	assert.Contains(t, log.String(), "agent claude-code/s1  checked 1 file · clean · 1 caught")
}

func TestHookStop_TwoUsesInOneBlockAreTwoFindings(t *testing.T) {
	app := hookApp(t)
	root := hookProject(t, app, "hookrepeat")
	guide := filepath.Join(root, "docs", "guide.md")
	twice := strings.Replace(hookGuide, "Log in with your account.", "Log in with your account, then Log in again.", 1)
	require.NoError(t, os.WriteFile(guide, []byte(twice), 0o644))
	tr := filepath.Join(t.TempDir(), "transcript.jsonl")

	transcript(t, tr, "turn-1", guide)
	runStop(t, app, root, "s1", tr, false)
	once := strings.Replace(twice, "then Log in again", "then Sign in again", 1)
	require.NoError(t, os.WriteFile(guide, []byte(once), 0o644))
	transcript(t, tr, "turn-2", guide)
	runStop(t, app, root, "s1", tr, false)

	runs := checkRuns(t, app, root)
	require.Len(t, runs, 2)
	assert.Equal(t, 2, runs[1].Check.Failing, "two uses of the words in one block are two findings, however many gates report them")
	assert.Equal(t, 1, runs[0].Check.Failing)
	assert.Equal(t, 1, runs[0].Caught, "fixing one of the two is one caught")

	got, err := app.SessionOutcomes(t.Context(), ContextSessionRequest{Project: recipeOf(root), Session: "s1"})
	require.NoError(t, err)
	assert.Equal(t, SessionOutcomes{Session: "s1", Checked: 2, Failing: 1, Caught: 1, Files: 1}, got)
}

func TestHookStop_AnUnrecordableRunKeepsItsDecision(t *testing.T) {
	app := hookApp(t)
	// A recipe with neither an id nor a name gives its context nowhere to
	// live, so the run cannot be recorded. Its target drops a placeholder,
	// which fails the checks gate without any rule in a store.
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	write("kapi.yaml", `version: v1
defaults:
  source_language: en
  target_languages: [fr]
collections:
  - path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`)
	write("locales/en/app.json", `{"greeting": "Hello {name}, welcome!"}`+"\n")
	write("locales/fr/app.json", `{"greeting": "Bonjour, bienvenue !"}`+"\n")

	stdout, stderr := runStop(t, app, root, "s1", "", true)
	var res StopHookResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &res), stdout)
	assert.Equal(t, "block", res.Decision, "the gates decide, whatever became of the record")
	assert.Contains(t, res.Reason, "locales/fr/app.json")
	assert.Empty(t, res.Record, "nothing was recorded")
	assert.Contains(t, stderr, "the run is not recorded in the project's log")
	assert.Contains(t, stderr, "neither an id nor a name")
}

func TestCaughtBetween_AFindingIsTheSameByFileRuleAndQuote(t *testing.T) {
	run := func(findings ...contextop.CheckFinding) contextop.CheckRun {
		return contextop.CheckRun{Verdict: "failed", Failing: len(findings), Findings: findings}
	}
	logIn := contextop.CheckFinding{File: "docs/guide.md", Rule: "terms.vocabulary", Quote: "Log in", Block: "p#2"}
	moved := logIn
	moved.Block = "p#5"
	elsewhere := logIn
	elsewhere.File = "docs/other.md"
	login := logIn
	login.Quote = "login"

	assert.Equal(t, 0, caughtBetween(run(logIn), run(moved)), "a finding that moved to another block is still there")
	assert.Equal(t, 1, caughtBetween(run(logIn), run()), "a finding gone from the file was caught")
	assert.Equal(t, 1, caughtBetween(run(logIn), run(elsewhere)), "the same words in another file are another finding")
	assert.Equal(t, 1, caughtBetween(run(logIn), run(login)), "other words under the same rule are another finding")
	assert.Equal(t, 1, caughtBetween(run(logIn, moved), run(moved)), "one of two occurrences was fixed")

	cut := run(login)
	cut.Failing = 3
	assert.Equal(t, 0, caughtBetween(run(logIn), cut), "a run that lists fewer findings than it counted says nothing about the rest")
}

func TestSessionOutcomesOf_ARunThatDidNotRunComparesNothing(t *testing.T) {
	logIn := contextop.CheckFinding{File: "docs/guide.md", Rule: "terms.vocabulary", Quote: "Log in"}
	rec := func(id, session string, run contextop.CheckRun) contextop.Record {
		return contextop.Record{ID: id, Project: "p", Kind: contextop.KindCheck,
			Actor: contextop.Actor{Kind: contextop.ActorAgent, Name: "claude-code", Session: session}, Check: &run}
	}
	records := []contextop.Record{
		rec("4", "s1", contextop.CheckRun{Verdict: "passed", Clean: true, Files: []string{"docs/a.md", "docs/b.md", "docs/c.md"}}),
		rec("3", "s1", contextop.CheckRun{Verdict: "passed", Clean: true, Files: []string{"docs/guide.md"}, Edited: true}),
		rec("2", "s1", contextop.CheckRun{Verdict: "did_not_run", Files: []string{"docs/guide.md"}, Edited: true}),
		rec("1", "s1", contextop.CheckRun{Verdict: "failed", Failing: 1, Findings: []contextop.CheckFinding{logIn}, Files: []string{"docs/guide.md", "docs/other.md"}, Edited: true}),
	}
	got := SessionOutcomesOf(records)["s1"]
	assert.Equal(t, SessionOutcomes{Session: "s1", Checked: 4, Failing: 0, Caught: 1, Files: 2}, got,
		"a run that reached no verdict sits between the two it does not separate, "+
			"and a run with no transcript names no file the session edited")
}
