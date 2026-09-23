package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// The fixture is a project with nothing in its context: no voice profile, no
// terms, no content memory. That is what a project looks like on the day it is
// created, and it is the case a candidate has to work in, because a check there
// has no vocabulary analyzer to ride along with.
const contextOpsYAML = "greeting: We utilise the widget every day.\nfarewell: Goodbye\n"

// contextOpsApp is one App over one workspace, so two projects created in one
// test share the workspace a widened rule lives in.
func contextOpsApp(t *testing.T) (*App, string) {
	t.Helper()
	isolateCheckExecution(t)
	workspaceRoot := t.TempDir()
	app := &App{SourceLang: "en"}
	app.SetWorkspaceRoot(workspaceRoot)
	t.Cleanup(app.Shutdown)
	return app, workspaceRoot
}

// contextOpsProject writes a project whose content uses the word a rule will be
// proposed about. The identity is explicit so two projects in one workspace
// keep their own context stores.
func contextOpsProject(t *testing.T, name string) string {
	t.Helper()
	id := projectIDFor(name)
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	write("kapi.yaml", `version: v1
id: `+id+`
name: `+name+`
defaults:
  source_language: en
collections:
  - name: config
    source_only: true
    content:
      - path: "config/*.yaml"
`)
	write("config/app.yaml", contextOpsYAML)
	return root
}

// projectIDFor mints a recipe id in the shape `kapi init` mints: "prj_" and at
// least twenty characters from a-z2-7. The test's own name goes in it, so a
// failure says which project it was about.
func projectIDFor(name string) string {
	id := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '2' && r <= '7') {
			return r
		}
		return -1
	}, name)
	for len(id) < 20 {
		id += "a"
	}
	return "prj_" + id
}

func recipeOf(root string) string { return filepath.Join(root, "kapi.yaml") }

// checkWith runs the project check the way `kapi check --strict` does, which is
// the gate a confirmed rule has to fail and a candidate must not.
func checkWith(t *testing.T, app *App, root string, strict bool) check.Report {
	t.Helper()
	cmd := executionCommand(t)
	cmd.Flags().Bool("strict", strict, "")
	cmd.Flags().Bool("lenient", false, "")
	cmd.Flags().Int("min-score", 0, "")
	cmd.Flags().String(projectFlagName, recipeOf(root), "")
	report, err := app.ComputeCheck(cmd, nil)
	require.NoError(t, err)
	return report
}

// vocabularyFindings are the findings about the word under test, which is the
// only thing these tests put in the content.
func vocabularyFindings(report check.Report) []check.Diagnostic {
	var out []check.Diagnostic
	for _, d := range report.Findings {
		if strings.Contains(d.Message, "utilise") {
			out = append(out, d)
		}
	}
	return out
}

func proposeUtilise(t *testing.T, app *App, root string, actor contextop.Actor) ContextOperation {
	t.Helper()
	op, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor:   actor,
		Project: recipeOf(root),
		Term:    "use", InsteadOf: []string{"utilise"},
		Evidence: []contextop.Evidence{{Path: "config/app.yaml", Unit: "greeting", Quote: "We utilise the widget"}},
		Text:     "the docs say use everywhere else",
	})
	require.NoError(t, err)
	return op
}

var person = contextop.Actor{Kind: contextop.ActorPerson, Name: "asgeir"}

func agentIn(session string) contextop.Actor {
	return contextop.Actor{Kind: contextop.ActorAgent, Name: "claude", Session: session}
}

// TestCandidateAdvisesAndConfirmedBinds is the whole claim in one test: a
// proposal is reported and fails nothing, and the same rule fails the gate once
// a person has confirmed it.
func TestCandidateAdvisesAndConfirmedBinds(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-advise")

	clean := checkWith(t, app, root, true)
	require.Empty(t, vocabularyFindings(clean), "a project with no context says nothing about the word")
	require.NotEqual(t, check.VerdictFailed, clean.Verdict)

	proposed := proposeUtilise(t, app, root, person)
	assert.Equal(t, contextop.StatusSuggested, proposed.Status)

	advised := checkWith(t, app, root, true)
	found := vocabularyFindings(advised)
	require.Len(t, found, 1, "a candidate is reported wherever the word appears")
	assert.True(t, found[0].Advisory, "and it says it is a proposal")
	assert.Equal(t, check.SeverityNeutral, found[0].Severity)
	assert.Contains(t, found[0].Message, "Proposed rule")
	assert.NotEqual(t, check.VerdictFailed, advised.Verdict,
		"must not fail: a rule nobody has confirmed can never fail a check, even under --strict")
	assert.Empty(t, advised.Gate.Failed)
	assert.Zero(t, advised.Summary.Major)
	assert.Zero(t, advised.Summary.Critical)

	confirmed, err := app.KeepContextOperation(t.Context(), ContextKeepRequest{
		Actor:   person,
		Project: recipeOf(root),
		ID:      proposed.ID,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, confirmed.Landed, "confirming writes the rule into the project's store")
	assert.NoFileExists(t, filepath.Join(root, ".kapi", "terms.json"),
		"the rule lands in the store the gate reads, and in no file")

	bound := checkWith(t, app, root, true)
	found = vocabularyFindings(bound)
	require.NotEmpty(t, found, "a confirmed rule is enforced")
	for _, d := range found {
		assert.False(t, d.Advisory, "a confirmed rule is no longer a proposal")
	}
	assert.Equal(t, check.VerdictFailed, bound.Verdict, "and it fails the gate")
}

// TestDiscardAndRevertStopARuleAnswering covers both withdrawals, for a
// candidate and for a rule already in force.
func TestDiscardAndRevertStopARuleAnswering(t *testing.T) {
	tests := []struct {
		name    string
		confirm bool
		undo    func(t *testing.T, app *App, root, id string)
	}{
		{
			name: "dropping a suggestion",
			undo: func(t *testing.T, app *App, root, id string) {
				_, err := app.DropContextOperation(t.Context(), ContextDropRequest{
					Actor: person, Project: recipeOf(root), ID: id,
				})
				require.NoError(t, err)
			},
		},
		{
			name: "reverting a suggestion",
			undo: func(t *testing.T, app *App, root, id string) {
				_, err := app.RevertContextOperations(t.Context(), ContextRevertRequest{
					Actor: person, Project: recipeOf(root), ID: id,
				})
				require.NoError(t, err)
			},
		},
		{
			name:    "reverting an established rule",
			confirm: true,
			undo: func(t *testing.T, app *App, root, id string) {
				_, err := app.RevertContextOperations(t.Context(), ContextRevertRequest{
					Actor: person, Project: recipeOf(root), ID: id,
				})
				require.NoError(t, err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := contextOpsApp(t)
			root := contextOpsProject(t, "ctxops-undo")

			proposed := proposeUtilise(t, app, root, person)
			if tt.confirm {
				_, err := app.KeepContextOperation(t.Context(), ContextKeepRequest{
					Actor:   person,
					Project: recipeOf(root), ID: proposed.ID,
				})
				require.NoError(t, err)
			}
			require.NotEmpty(t, vocabularyFindings(checkWith(t, app, root, true)),
				"the rule answers before it is withdrawn")

			tt.undo(t, app, root, proposed.ID)

			after := checkWith(t, app, root, true)
			assert.Empty(t, vocabularyFindings(after), "a withdrawn rule stops answering at once")
			assert.NotEqual(t, check.VerdictFailed, after.Verdict)
		})
	}
}

// TestRevertingASessionRestoresTheCheckExactly is the reversibility claim: what
// a check says before an agent session and after that session is reverted are
// the same answer, not a similar one.
func TestRevertingASessionRestoresTheCheckExactly(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-session")

	before := checkWith(t, app, root, true)

	session := agentIn("s-nightly")
	var recorded []string
	for _, rule := range []coreprofile.TermRule{
		{Term: "utilise", Replacement: "use"},
		{Term: "widget", Replacement: "component"},
		{Term: "every day", Replacement: "daily"},
	} {
		op, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
			Actor:     session,
			Project:   recipeOf(root),
			Term:      rule.Replacement,
			InsteadOf: []string{rule.Term},
			Evidence:  []contextop.Evidence{{Path: "config/app.yaml"}},
		})
		require.NoError(t, err)
		recorded = append(recorded, op.ID)
	}
	// A person confirmed one of them, so the session put something binding in
	// the project's store as well as three candidates in its log.
	_, err := app.KeepContextOperation(t.Context(), ContextKeepRequest{
		Actor:   person,
		Project: recipeOf(root), ID: recorded[0],
	})
	require.NoError(t, err)

	during := checkWith(t, app, root, true)
	require.NotEqual(t, before.Findings, during.Findings, "the session changed what the check says")
	require.Equal(t, check.VerdictFailed, during.Verdict)

	summary, err := app.ContextSessionSummary(t.Context(), ContextSessionRequest{
		Project: recipeOf(root), Session: "s-nightly",
	})
	require.NoError(t, err)
	assert.Equal(t, 3, summary.Operations)
	assert.Equal(t, 3, summary.ByKind[contextop.KindObserve])
	assert.Equal(t, 1, summary.ByStatus[contextop.StatusEstablished])
	assert.Equal(t, 2, summary.ByStatus[contextop.StatusSuggested])

	reverted, err := app.RevertContextOperations(t.Context(), ContextRevertRequest{
		Actor:   person,
		Project: recipeOf(root), Session: "s-nightly",
	})
	require.NoError(t, err)
	assert.Len(t, reverted.Reverted, 3, "everything the session recorded")
	assert.NotEmpty(t, reverted.Retracted, "and the rule it got confirmed is taken back out")

	after := checkWith(t, app, root, true)
	assert.Equal(t, before.Findings, after.Findings, "the same findings, not similar ones")
	assert.Equal(t, before.Summary, after.Summary)
	assert.Equal(t, before.Gate, after.Gate)
	assert.Equal(t, before.Verdict, after.Verdict)
}

// TestAnAgentCannotConfirm drives the policy through the host API, because a
// policy the API does not consult is a policy that is not in force.
func TestAnAgentCannotConfirm(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-policy")

	proposed := proposeUtilise(t, app, root, agentIn("s1"))
	assert.Equal(t, contextop.StatusSuggested, proposed.Status)

	_, err := app.KeepContextOperation(t.Context(), ContextKeepRequest{
		Actor:   agentIn("s1"),
		Project: recipeOf(root), ID: proposed.ID,
	})
	require.ErrorIs(t, err, contextop.ErrRefused)

	_, err = app.WidenContextOperation(t.Context(), ContextWidenRequest{
		Actor:   agentIn("s1"),
		Project: recipeOf(root), ID: proposed.ID, To: WidenToWorkspace,
	})
	require.Error(t, err, "an unconfirmed rule cannot be widened, and an agent could not widen it anyway")

	still := checkWith(t, app, root, true)
	found := vocabularyFindings(still)
	require.Len(t, found, 1)
	assert.True(t, found[0].Advisory, "the refused confirmation left the rule a proposal")
	assert.NotEqual(t, check.VerdictFailed, still.Verdict)

	confirmed, err := app.KeepContextOperation(t.Context(), ContextKeepRequest{
		Actor:   person,
		Project: recipeOf(root), ID: proposed.ID,
	})
	require.NoError(t, err, "a person confirms the agent's proposal")
	assert.NotEmpty(t, confirmed.Landed)
}

// TestAWidenedRuleAnswersInASecondProject is the scope claim: what is learned
// stays where its evidence was seen until a person says otherwise.
func TestAWidenedRuleAnswersInASecondProject(t *testing.T) {
	app, _ := contextOpsApp(t)
	first := contextOpsProject(t, "ctxops-first")
	second := contextOpsProject(t, "ctxops-second")

	proposed := proposeUtilise(t, app, first, person)
	_, err := app.KeepContextOperation(t.Context(), ContextKeepRequest{
		Actor:   person,
		Project: recipeOf(first), ID: proposed.ID,
	})
	require.NoError(t, err)

	require.Empty(t, vocabularyFindings(checkWith(t, app, second, true)),
		"a rule confirmed in one project says nothing in another")

	widened, err := app.WidenContextOperation(t.Context(), ContextWidenRequest{
		Actor:   person,
		Project: recipeOf(first), ID: proposed.ID, To: WidenToWorkspace,
	})
	require.NoError(t, err)
	assert.Equal(t, "the whole workspace", widened.Landed)

	elsewhere := checkWith(t, app, second, true)
	found := vocabularyFindings(elsewhere)
	require.NotEmpty(t, found, "a widened rule answers in every project of the workspace")
	for _, d := range found {
		assert.False(t, d.Advisory, "a widened rule was confirmed, so it binds")
	}
	assert.Equal(t, check.VerdictFailed, elsewhere.Verdict)

	// And it can be taken back out again.
	_, err = app.RevertContextOperations(t.Context(), ContextRevertRequest{
		Actor:   person,
		Project: recipeOf(first), ID: proposed.ID,
	})
	require.NoError(t, err)
	assert.Empty(t, vocabularyFindings(checkWith(t, app, second, true)),
		"reverting a widened rule stops it answering everywhere")
}

// TestApplyAssetEntriesRecordOperations holds `kapi apply` to the same history:
// the one write verb's asset entries are decisions, and a decision that leaves
// no record cannot be looked at again.
func TestApplyAssetEntriesRecordOperations(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-apply")

	cmd := executionCommand(t)
	cmd.Flags().String(projectFlagName, recipeOf(root), "")

	applied := app.applyRecordedAssetEntry(t.Context(), cmd, changeEntry{
		Kind: kindTerm, Op: "upsert", Term: "utilise", Replacement: "use", Locale: "en",
		Status:   "forbidden",
		Evidence: []contextop.Evidence{{Path: "config/app.yaml"}},
	})
	require.Equal(t, "applied", applied.Status, applied.Detail)
	assert.NotContains(t, applied.Detail, "not recorded")

	log, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	require.Len(t, log.Operations, 1, "one edit by the person who ran apply")
	edit := log.Operations[0]
	assert.Equal(t, contextop.KindEdit, edit.Kind)
	assert.Equal(t, contextop.StatusEstablished, edit.Status, "a person's own edit is established from the start")
	assert.Equal(t, contextop.ActorPerson, edit.Actor.Kind)
	rule, ok := edit.Rule()
	require.True(t, ok)
	assert.Equal(t, "utilise", rule.Term)
	assert.Equal(t, []contextop.Evidence{{Path: "config/app.yaml"}}, edit.Evidence)

	// Re-applying the same entry is a no-op, and records nothing a second time.
	again := app.applyRecordedAssetEntry(t.Context(), cmd, changeEntry{
		Kind: kindTerm, Op: "upsert", Term: "utilise", Replacement: "use", Locale: "en", Status: "forbidden",
	})
	assert.Equal(t, "skipped", again.Status)
	log, err = app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	assert.Len(t, log.Operations, 1, "an entry that changed nothing decides nothing")

	// An agent naming itself on an asset entry is refused before anything is
	// written, because applying one is a decision.
	refused := app.applyRecordedAssetEntry(t.Context(), cmd, changeEntry{
		Kind: kindVoice, Op: "add-rule", List: "forbidden", Term: "leverage", Replacement: "use",
		Actor: &contextop.Actor{Kind: contextop.ActorAgent, Name: "claude", Session: "s1"},
	})
	assert.Equal(t, "error", refused.Status)
	assert.Contains(t, refused.Detail, "may not edit")
	assert.NoFileExists(t, filepath.Join(root, ".kapi", "voice.yaml"),
		"the refusal came before the committed source moved")
}

// TestContextLogFilters covers the narrowing `kapi context log` offers.
func TestContextLogFilters(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-log")

	proposed := proposeUtilise(t, app, root, agentIn("s1"))
	_, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor:    person,
		Project:  recipeOf(root),
		Text:     "the documents address the reader as you",
		Evidence: []contextop.Evidence{{Path: "config/app.yaml"}},
	})
	require.NoError(t, err)
	_, err = app.RecordContextCorrection(t.Context(), ContextCorrectRequest{
		Actor:   agentIn("s1"),
		Project: recipeOf(root),
		From:    "widget", To: "component", Suggest: true, Severity: "minor",
		Evidence: []contextop.Evidence{{Path: "config/app.yaml", Unit: "greeting"}},
	})
	require.NoError(t, err)
	_, err = app.DropContextOperation(t.Context(), ContextDropRequest{
		Actor:   person,
		Project: recipeOf(root), ID: proposed.ID,
	})
	require.NoError(t, err)

	tests := []struct {
		name string
		req  ContextLogRequest
		want int
	}{
		{"everything", ContextLogRequest{}, 4},
		{"what carries a subject", ContextLogRequest{Subjects: true}, 3},
		{"what is still a candidate", ContextLogRequest{Status: contextop.StatusSuggested}, 2},
		{"what was discarded", ContextLogRequest{Status: contextop.StatusDropped}, 1},
		{"one session", ContextLogRequest{Session: "s1"}, 2},
		{"one actor", ContextLogRequest{Actor: "asgeir"}, 2},
		{"a limit", ContextLogRequest{Limit: 1}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.Project = recipeOf(root)
			got, err := app.ContextOperations(t.Context(), tt.req)
			require.NoError(t, err)
			assert.Len(t, got.Operations, tt.want)
		})
	}

	// A correction carries both wordings, and the rule it implies.
	corrections, err := app.ContextOperations(t.Context(), ContextLogRequest{
		Project: recipeOf(root), Status: contextop.StatusSuggested, Subjects: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, corrections.Operations)
	found := false
	for _, op := range corrections.Operations {
		if op.Kind != contextop.KindCorrect {
			continue
		}
		found = true
		require.NotNil(t, op.Correction)
		assert.Equal(t, "widget", op.Correction.From)
		assert.Equal(t, "component", op.Correction.To)
		rule, ok := op.Rule()
		require.True(t, ok, "a correction recorded with --propose states the rule it implies")
		assert.Equal(t, "component", rule.Replacement)
	}
	assert.True(t, found)
}

// TestContextOperationsNeedAProject covers the refusal outside a project, which
// is where an operation has nothing to belong to.
func TestContextOperationsNeedAProject(t *testing.T) {
	app, _ := contextOpsApp(t)
	_, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: filepath.Join(t.TempDir(), "kapi.yaml")})
	assert.Error(t, err)
}
