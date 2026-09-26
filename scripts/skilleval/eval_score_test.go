package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recordings under testdata/agenteval are real output of the binary this
// scoring reads: `kapi context log --json` after a person held six rules and an
// agent recorded twelve entries, and `kapi check --diff-against --json` over a
// version that breaks four held rules and one that keeps them all. Scoring them
// proves the scoring without a model call, and a change in the product's JSON
// shows up here as a failing test rather than as a silent zero in a report.

func recordedEvalStore(t *testing.T) EvalStore {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "agenteval", "context-log.json"))
	require.NoError(t, err)
	store, err := evalParseStore(data)
	require.NoError(t, err)
	return store
}

func recordedEvalCheck(t *testing.T, name string) EvalCheck {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "agenteval", name))
	require.NoError(t, err)
	check := evalParseCheck(data)
	require.True(t, check.Ran, check.Error)
	return check
}

func evalTestFixture(t *testing.T) EvalFixture {
	t.Helper()
	fixture, err := generateEvalFixture()
	require.NoError(t, err)
	return fixture
}

func TestEvalParseStoreReadsTheRecordedLog(t *testing.T) {
	store := recordedEvalStore(t)
	byID := map[string]EvalOperation{}
	for _, op := range store.Operations {
		byID[op.ID] = op
	}
	require.Contains(t, byID, "14")
	assert.Equal(t, "Loom Wise", byID["14"].Term)
	assert.Equal(t, "Loomwise", byID["14"].Replacement)
	assert.Equal(t, "Loomwise, one word", byID["14"].Text)
	assert.Equal(t, evalActorAgent, byID["14"].Actor)
	assert.Equal(t, "run-1", byID["14"].Session)
	assert.Equal(t, "e-mail", byID["25"].Term, "a correction's from is the rule's term")
	assert.Equal(t, "email", byID["25"].Replacement)
	assert.Equal(t, "Headings use sentence case", byID["20"].Text)
	assert.Equal(t, []EvalEvidence{{Path: "docs/plans.md", Quote: "Changing plan"}}, byID["20"].Evidence)
	assert.Equal(t, evalActorPerson, byID["1"].Actor)
}

// The held terms are what a Measure 1 cell refuses to run without, read back
// through the vocabulary in eval_product.go.
func TestEvalHeldTermsReadsWhatAPersonHolds(t *testing.T) {
	held := evalHeldTerms(recordedEvalStore(t))
	assert.ElementsMatch(t, []string{"loom wise", "log in", "e-mail", "employee", "!", "color"}, held)
}

func TestScoreEvalGrowAgainstTheRecordedLog(t *testing.T) {
	fixture := evalTestFixture(t)
	score := scoreEvalGrow(fixture, recordedEvalStore(t).Operations)

	verdicts := map[string]string{}
	for _, record := range score.Records {
		verdicts[record.Op.ID] = record.Verdict
		assert.NotEmpty(t, record.Reason, "record %s says why", record.Op.ID)
	}
	assert.Equal(t, map[string]string{
		"14": evalVerdictKey,      // "Loom Wise" becomes "Loomwise"
		"15": evalVerdictKey,      // "logged in" becomes "signed in"
		"16": evalVerdictKey,      // "catalog" becomes "catalogue", a British spelling the key does not list
		"17": evalVerdictDecoy,    // "time sheet" becomes "timesheet"
		"18": evalVerdictHarmless, // "utilize" becomes "use": never written, and "use" is
		"19": evalVerdictNoise,    // "shift" becomes "slot": the project writes "shift" throughout
		"20": evalVerdictHarmless, // an observation quoting the fixture
		"21": evalVerdictKey,      // the product name, stated
		"22": evalVerdictKey,      // the second person, stated
		"23": evalVerdictNoise,    // an observation with no evidence
		"24": evalVerdictDecoy,    // an observation telling a writer what to do with the decoy
		"25": evalVerdictKey,      // "e-mail" corrected to "email"
	}, verdicts, "person entries are left out; each agent entry is placed")

	assert.Equal(t, []string{"british-spelling", "email", "product-name", "sign-in"}, score.Recalled)
	assert.Equal(t, 7, score.RecallOf)
	assert.InDelta(t, 4.0/7.0, score.recall(), 1e-9)
	assert.Equal(t, 8, score.Precise)
	assert.True(t, score.DecoyProposed)
}

func TestScoreEvalApplyAgainstTheRecordedChecks(t *testing.T) {
	key := evalKey()
	first := recordedEvalCheck(t, "check-broken.json")
	final := recordedEvalCheck(t, "check-clean.json")
	firstAdded := "## Trouble logging in\nIf the employee cannot log in, check their e-mail!"
	finalAdded := "## Trouble signing in\nIf a team member cannot sign in, check their email."
	transcript := EvalTranscript{Calls: []EvalCall{
		{Kind: evalKindWrite}, {Kind: evalKindCheck}, {Kind: evalKindWrite}, {Kind: evalKindCheck},
	}}

	score := scoreEvalApply(key, firstAdded, finalAdded, first, final, transcript)
	assert.ElementsMatch(t, []string{"sign-in", "email", "avoid-employee", "second-person", "no-exclamation"}, score.ApplicableFirst)
	assert.ElementsMatch(t, []string{"sign-in", "email", "avoid-employee", "no-exclamation"}, score.BrokenFirst)
	assert.Equal(t, []string{"second-person"}, score.FollowedFirst)
	assert.Equal(t, 4, score.FailingFirst)
	assert.Empty(t, score.BrokenFinal)
	assert.Equal(t, 0, score.FailingFinal)
	assert.ElementsMatch(t, score.ApplicableFirst, score.ApplicableFinal)
	assert.True(t, score.RanCheck)
	assert.True(t, score.CheckedBeforeDone)
	assert.True(t, score.Wrote)
}

func TestEvalFindingFails(t *testing.T) {
	assert.True(t, evalFindingFails(EvalCheckFinding{Fails: true}))
	assert.False(t, evalFindingFails(EvalCheckFinding{}), "an advisory rule reports")
	// An unconfirmed suggestion never fails, so it never counts.
	assert.False(t, evalFindingFails(EvalCheckFinding{Fails: true, Suggested: true}))
	check := EvalCheck{Findings: []EvalCheckFinding{
		{Term: "utilise"},
		{Fails: true, Suggested: true, Term: "e-mail"},
		{Fails: true, Term: "log in"},
	}}
	assert.Len(t, check.failing(), 1)
	assert.Equal(t, []string{"sign-in"}, evalBrokenConventions(evalKey(), check))
}

func TestEvalCheckedBeforeDone(t *testing.T) {
	calls := func(kinds ...string) []EvalCall {
		out := []EvalCall{}
		for _, kind := range kinds {
			out = append(out, EvalCall{Kind: kind})
		}
		return out
	}
	tests := []struct {
		name            string
		calls           []EvalCall
		ran, beforeDone bool
	}{
		{"never checked", calls(evalKindWrite), false, false},
		{"checked, then wrote again", calls(evalKindWrite, evalKindCheck, evalKindWrite), true, false},
		{"checked last", calls(evalKindWrite, evalKindCheck, evalKindOther), true, true},
		{"checked and wrote nothing", calls(evalKindCheck), true, true},
	}
	for _, tc := range tests {
		ran, beforeDone := evalCheckedBeforeDone(tc.calls)
		assert.Equal(t, tc.ran, ran, tc.name)
		assert.Equal(t, tc.beforeDone, beforeDone, tc.name)
	}
}

func TestEvalApplicable(t *testing.T) {
	key := evalKey()
	assert.Empty(t, evalApplicable(key, "  \n", nil), "no text gives no occasion")
	assert.ElementsMatch(t, []string{"second-person", "no-exclamation"},
		evalApplicable(key, "Open the app.", nil), "the habits govern any prose")
	assert.Contains(t, evalApplicable(key, "Choose the Fullhouse plan.", nil), "paid-plan-name")
	assert.Contains(t, evalApplicable(key, "Pick a colour.", nil), "british-spelling")
	assert.Contains(t, evalApplicable(key, "", []string{"email"}), "email",
		"a convention with a failing finding applied")
}

func TestEvalAddedText(t *testing.T) {
	before := "# Title\n\nFirst paragraph.\n\nSecond paragraph.\n"
	after := "# Title\n\nFirst paragraph.\n\nA new paragraph.\n\nSecond paragraph.\n"
	assert.Equal(t, "A new paragraph.", evalAddedText(before, after))
	assert.Empty(t, evalAddedText(before, before))
	assert.Equal(t, "# New\nBody.", evalAddedText("", "# New\n\nBody.\n"))
	assert.Equal(t, "Same.", evalAddedText("Same.\n", "Same.\nSame.\n"), "a repeated line is new the second time")
}

func TestEvalBritishPair(t *testing.T) {
	for us, uk := range map[string]string{
		"organize": "organise", "organization": "organisation", "analyze": "analyse", "color": "colour",
		"colors": "colours", "center": "centre", "catalog": "catalogue", "canceled": "cancelled",
		"traveling": "travelling", "license": "licence",
	} {
		assert.True(t, evalBritishPair(us, uk), "%s and %s", us, uk)
	}
	for us, uk := range map[string]string{"colour": "color", "shift": "slot", "use": "use", "": "x"} {
		assert.False(t, evalBritishPair(us, uk), "%s and %s", us, uk)
	}
}

func TestEvalDecoyProposal(t *testing.T) {
	decoy := evalKey().decoy()
	tests := []struct {
		name string
		op   EvalOperation
		want bool
	}{
		{"a rule naming it", EvalOperation{Kind: "observe", Term: "time sheet", Replacement: "timesheet"}, true},
		{"a rule in the other direction", EvalOperation{Kind: "observe", Term: "Timesheets", Replacement: "time sheets"}, true},
		{"a correction", EvalOperation{Kind: "correct", Term: "timesheet", Replacement: "time sheet"}, true},
		{"an observation that directs", EvalOperation{Kind: "observe", Text: "Prefer timesheet"}, true},
		{"an observation that describes", EvalOperation{Kind: "observe", Text: "The docs spell timesheet two ways"}, false},
		{"a neutral observation", EvalOperation{Kind: "observe", Text: "Pages cover timesheets and payroll exports"}, false},
		{"an unrelated rule", EvalOperation{Kind: "observe", Term: "log in", Replacement: "sign in"}, false},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, evalDecoyProposal(decoy, tc.op) != "", tc.name)
	}
}

func TestEvalGrounded(t *testing.T) {
	fixture := evalTestFixture(t)
	tests := []struct {
		name     string
		evidence EvalEvidence
		want     bool
	}{
		{"a fixture file", EvalEvidence{Path: "docs/plans.md"}, true},
		{"a relative path", EvalEvidence{Path: "./docs/plans.md"}, true},
		{"a line reference", EvalEvidence{Path: "docs/plans.md:12"}, true},
		{"an absolute path in the cell", EvalEvidence{Path: "/tmp/cell/" + evalRepoName + "/docs/plans.md"}, true},
		{"a quote from the named file", EvalEvidence{Path: "docs/plans.md", Quote: "Changing  plan"}, true},
		{"a quote from another file", EvalEvidence{Path: "docs/plans.md", Quote: "Clocking in and out"}, false},
		{"a quote with no path", EvalEvidence{Quote: "Clocking in and out"}, true},
		{"the agent's own new page", EvalEvidence{Path: "docs/open-shifts.md"}, false},
		{"nothing", EvalEvidence{}, false},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, evalGrounded(fixture, tc.evidence), tc.name)
	}
}

func TestEvalRecordConventionsLeavesPersonsAndDecisionsOut(t *testing.T) {
	fixture := evalTestFixture(t)
	score := scoreEvalGrow(fixture, []EvalOperation{
		{ID: "1", Kind: "observe", Actor: evalActorPerson, Term: "Full House", Replacement: "Fullhouse"},
		{ID: "2", Kind: "keep", Actor: evalActorAgent},
		{ID: "3", Kind: "observe", Actor: evalActorAgent, Term: "Full House", Replacement: "Fullhouse"},
	})
	require.Len(t, score.Records, 1)
	assert.Equal(t, "3", score.Records[0].Op.ID)
	assert.Equal(t, []string{"paid-plan-name"}, score.Recalled)
}

func TestEvalObservationMatchesAName(t *testing.T) {
	key := evalKey()
	product := evalConventionByID(key, "product-name")
	assert.True(t, evalObservationMatches(product, "Loomwise is always spelt as one word"))
	assert.False(t, evalObservationMatches(product, "Loomwise pages keep paragraphs short"),
		"naming the product is not a statement about how it is written")
	assert.True(t, evalObservationMatches(product, "Never write Loom Wise"))
}
