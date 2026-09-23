package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// evalEvidence lays out a saved evaluation the way a run leaves it, so the
// report can be rendered without a model call, a live host or a kapi binary.
func evalEvidence(t *testing.T) (string, evalStudyRecord) {
	t.Helper()
	dir := t.TempDir()
	record := evalStudyRecord{
		Schema: evalSchema, Fingerprint: "fingerprint-1", Manifest: validEvalManifest(),
		KapiVersion: "kapi version v1.3.0", KapiCommit: "abc1234", CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, writePairedJSON(filepath.Join(dir, "study.json"), record))
	return dir, record
}

func writeEvalAttempt(t *testing.T, dir string, session EvalSession, fingerprint string, result *evalAttemptResult) {
	t.Helper()
	attemptDir := filepath.Join(dir, session.Phase, session.ID)
	require.NoError(t, os.MkdirAll(attemptDir, 0o700))
	attempt := evalAttempt{Schema: evalSchema, Session: session, Phase: session.Phase, Fingerprint: fingerprint,
		HostVersion: session.Host.Host + " 1.0"}
	require.NoError(t, writePairedJSON(filepath.Join(attemptDir, "started.json"), attempt))
	if result != nil {
		require.NoError(t, writePairedJSON(filepath.Join(attemptDir, "result.json"), *result))
	}
}

func evalSessionOf(t *testing.T, manifest EvalManifest, phase, id string) EvalSession {
	t.Helper()
	for _, session := range evalSchedule(manifest, phase) {
		if session.ID == id {
			return session
		}
	}
	t.Fatalf("no session %s in %s", id, phase)
	return EvalSession{}
}

// A full set of runs for one host, scored from the recordings, and a second
// host whose runs have not happened.
func TestBuildEvalReportScoresSavedAttempts(t *testing.T) {
	dir, record := evalEvidence(t)
	fixture := evalTestFixture(t)
	first := recordedEvalCheck(t, "check-broken.json")
	final := recordedEvalCheck(t, "check-clean.json")
	store := recordedEvalStore(t)
	for _, task := range record.Manifest.Tasks {
		writeEvalAttempt(t, dir, evalSessionOf(t, record.Manifest, evalPhaseApply, "apply-"+task+"-claude"), record.Fingerprint,
			&evalAttemptResult{Status: "completed", CheckFirst: &first, CheckFinal: &final, UserDataUntouched: true,
				FirstAdded: "If the employee cannot log in, check their e-mail!",
				FinalAdded: "If a team member cannot sign in, check their email.", HeldBefore: 6, HeldAfter: 6})
		writeEvalAttempt(t, dir, evalSessionOf(t, record.Manifest, evalPhaseGrow, "grow-"+task+"-claude"), record.Fingerprint,
			&evalAttemptResult{Status: "completed", Recorded: store.Operations, UserDataUntouched: true})
	}
	writeEvalAttempt(t, dir, evalSessionOf(t, record.Manifest, evalPhaseSmoke, "smoke-troubleshooting-section-codex"),
		record.Fingerprint, &evalAttemptResult{Status: "completed", Recorded: []EvalOperation{}})

	report, err := buildEvalReport(dir, record, fixture, nil)
	require.NoError(t, err)
	assert.Equal(t, 7, report.Attempts)
	require.Len(t, report.Apply, 3)
	require.Len(t, report.Grow, 4)
	assert.Len(t, report.growMeasured(), 3, "the smoke run is listed and left out of the verdicts")

	claudeApply, codexApply := report.ApplyHosts[0], report.ApplyHosts[1]
	assert.Equal(t, 3, claudeApply.Completed)
	assert.Equal(t, 15, claudeApply.Applicable)
	assert.Equal(t, 3, claudeApply.Followed)
	assert.Equal(t, "fail", claudeApply.Verdict, "a fifth of the rules followed first time is under the bar")
	assert.Equal(t, "unmeasured: 0 of 3 runs completed", codexApply.Verdict)

	claudeGrow, codexGrow := report.GrowHosts[0], report.GrowHosts[1]
	assert.InDelta(t, 4.0/7.0, claudeGrow.MeanRecall, 1e-9)
	assert.Equal(t, 36, claudeGrow.Records)
	assert.Equal(t, 24, claudeGrow.Precise)
	assert.True(t, claudeGrow.DecoyProposed)
	assert.Equal(t, "fail", claudeGrow.Verdict, "the decoy was proposed and precision is under the bar")
	assert.Equal(t, "unmeasured: 0 of 3 runs completed", codexGrow.Verdict, "a smoke run is not a measured run")

	rendered := renderEvalReport(report)
	for _, heading := range []string{"## Measure 1: agents apply context", "## Measure 2: agents grow context",
		"## Measure 3: settling", "## Measure 4: review is worth it"} {
		assert.Contains(t, rendered, heading)
	}
	assert.Contains(t, rendered, "smoke-troubleshooting-section-codex (smoke)")
}

func TestEvalGrowVerdictPassesAtTheBar(t *testing.T) {
	rows := []EvalGrowRow{}
	for _, task := range []string{"a", "b", "c"} {
		rows = append(rows, EvalGrowRow{Session: task, Host: "claude", Status: "completed", Score: EvalGrowScore{
			Recalled: []string{"product-name", "feature-name", "email", "sign-in"}, RecallOf: 7,
			Records: make([]EvalRecordScore, 5), Precise: 4,
		}})
	}
	assert.Equal(t, "pass", evalGrowVerdict("claude", 3, rows).Verdict)

	rows[1].Score.DecoyProposed = true
	rows[1].Status = "timeout"
	verdict := evalGrowVerdict("claude", 3, rows)
	assert.True(t, verdict.DecoyProposed, "a decoy proposed in an unfinished run still counts")
	assert.Equal(t, "unmeasured: 2 of 3 runs completed", verdict.Verdict)
}

func TestEvalApplyVerdictNeedsACleanFinish(t *testing.T) {
	row := func(followed, applicable, failingFinal int) EvalApplyRow {
		return EvalApplyRow{Host: "codex", Status: "completed", Score: EvalApplyScore{
			ApplicableFirst: make([]string, applicable), FollowedFirst: make([]string, followed), FailingFinal: failingFinal}}
	}
	assert.Equal(t, "pass", evalApplyVerdict("codex", 2, []EvalApplyRow{row(9, 10, 0), row(10, 10, 0)}).Verdict)
	assert.Equal(t, "fail", evalApplyVerdict("codex", 2, []EvalApplyRow{row(10, 10, 1), row(10, 10, 0)}).Verdict,
		"one failing finding at the end fails the host")
	assert.Equal(t, "fail", evalApplyVerdict("codex", 2, []EvalApplyRow{row(8, 10, 0), row(8, 10, 0)}).Verdict)
	assert.Equal(t, "unmeasured: no run gave occasion to follow a rule",
		evalApplyVerdict("codex", 1, []EvalApplyRow{row(0, 0, 0)}).Verdict)
}

func TestBuildEvalReportKeepsAReservedAttempt(t *testing.T) {
	dir, record := evalEvidence(t)
	writeEvalAttempt(t, dir, evalSessionOf(t, record.Manifest, evalPhaseGrow, "grow-release-note-codex"), record.Fingerprint, nil)
	report, err := buildEvalReport(dir, record, evalTestFixture(t), nil)
	require.NoError(t, err)
	require.Len(t, report.Grow, 1)
	assert.Equal(t, "reserved", report.Grow[0].Status)
	assert.Contains(t, report.Grow[0].Error, "no result was committed")
}

func TestBuildEvalReportRefusesAnAttemptFromAnotherStudy(t *testing.T) {
	dir, record := evalEvidence(t)
	writeEvalAttempt(t, dir, evalSessionOf(t, record.Manifest, evalPhaseApply, "apply-release-note-codex"), "other", nil)
	_, err := buildEvalReport(dir, record, evalTestFixture(t), nil)
	require.ErrorContains(t, err, "fingerprint differs")
}

func TestReportEvalWithoutAStudy(t *testing.T) {
	err := reportEval(EvalOptions{Dir: t.TempDir(), Fixture: evalTestFixture(t)})
	require.ErrorContains(t, err, "no evaluation has been recorded here")
}

func TestReportEvalWritesTheSheetAndFoldsTheAnswers(t *testing.T) {
	dir, record := evalEvidence(t)
	store := recordedEvalStore(t)
	writeEvalAttempt(t, dir, evalSessionOf(t, record.Manifest, evalPhaseGrow, "grow-feature-page-claude"), record.Fingerprint,
		&evalAttemptResult{Status: "completed", Recorded: store.Operations})
	opts := EvalOptions{Dir: dir, Fixture: evalTestFixture(t)}
	require.NoError(t, reportEval(opts))
	sheet := evalLatest(t, dir, "report-*-review.md")
	assert.Contains(t, sheet, "## The decoy")
	assert.Contains(t, sheet, "| grow-feature-page-claude | 17 | observe |")
	for _, question := range evalReviewQuestions {
		assert.Contains(t, sheet, question)
	}
	assert.Contains(t, evalLatest(t, dir, "report-*[0-9]Z.md"), "No answers are recorded yet")

	answers := "minutes: 6\nkeep:\n  grow-feature-page-claude: [\"14\", \"21\"]\nlearned: |\n  The plans page never names a price in dollars.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "review-answers.yaml"), []byte(answers), 0o600))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, reportEval(opts))
	rendered := evalLatest(t, dir, "report-*[0-9]Z.md")
	assert.Contains(t, rendered, "Minutes spent: 6")
	assert.Contains(t, rendered, "Records worth keeping: 2 of 12")
	assert.Contains(t, rendered, "never names a price in dollars")
}

func TestReadEvalReviewAnswers(t *testing.T) {
	dir := t.TempDir()
	missing, err := readEvalReviewAnswers(filepath.Join(dir, "absent.yaml"))
	require.NoError(t, err)
	assert.Nil(t, missing, "no file is no answer yet")

	path := filepath.Join(dir, "answers.yaml")
	require.NoError(t, os.WriteFile(path, []byte("minutes: 3\nkept: []\n"), 0o600))
	_, err = readEvalReviewAnswers(path)
	require.ErrorContains(t, err, "kept", "a misspelt key is refused rather than read as no answer")

	require.NoError(t, os.WriteFile(path, []byte("minutes: -1\n"), 0o600))
	_, err = readEvalReviewAnswers(path)
	require.ErrorContains(t, err, "negative")

	require.NoError(t, os.WriteFile(path, []byte(evalReviewAnswersTemplate), 0o600))
	template, err := readEvalReviewAnswers(path)
	require.NoError(t, err, "the template a person fills in reads as it stands")
	assert.Empty(t, template.Keep)
}

func TestEvalReviewGroupsFollowTheKey(t *testing.T) {
	fixture := evalTestFixture(t)
	rows := []EvalGrowRow{{Session: "grow-a", Phase: evalPhaseGrow, Score: scoreEvalGrow(fixture, recordedEvalStore(t).Operations)}}
	groups := evalReviewGroups(fixture.Key, rows)
	require.Len(t, groups, len(fixture.Key.planted())+3)
	titles := []string{}
	counts := map[string]int{}
	for _, group := range groups {
		titles = append(titles, group.Title)
		counts[group.Title] = len(group.Records)
	}
	assert.Equal(t, fixture.Key.planted()[0].Summary, titles[0], "the key's order")
	assert.Equal(t, 2, counts[evalConventionByID(fixture.Key, "product-name").Summary])
	assert.Equal(t, 2, counts["Outside the key, and harmless"])
	assert.Equal(t, 2, counts["Outside the key, and noise"])
	assert.True(t, strings.HasPrefix(titles[len(titles)-3], "The decoy"))
}

func evalLatest(t *testing.T, dir, pattern string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	require.NoError(t, err)
	require.NotEmpty(t, matches, pattern)
	data, err := os.ReadFile(matches[len(matches)-1])
	require.NoError(t, err)
	return string(data)
}
