package host

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

// commentLimitsProfile is a voice profile that sets the default comment limits and
// declares no term or pattern.
const commentLimitsProfile = "name: Comment limits\nstyle:\n  comments: {}\n"

// namedProfileCheck checks file with the voice profile at profile named on the
// command line, with the flags in set.
func namedProfileCheck(t *testing.T, profile, file string, set map[string]string) check.Report {
	t.Helper()
	cmd := executionCommand(t)
	cmd.Flags().String("profile-file", profile, "")
	for flag, value := range set {
		require.NoError(t, cmd.Flags().Set(flag, value))
	}
	report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{file})
	require.NoError(t, err)
	return report
}

// TestCheckNamedCommentLimitsProfile names a voice profile that holds comment
// limits and no term or pattern. Its comment analyzers check a file's comments
// and decide the verdict, and voice.rules, with no rule to apply, is not
// applicable. Content holding no comment leaves the profile nothing to check.
func TestCheckNamedCommentLimitsProfile(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	limits := writeCheckInput(t, dir, "comments.yaml", commentLimitsProfile)
	g := newLimitsGo()
	code := writeCheckInput(t, dir, "parse.go", g.src)

	t.Run("the comment analyzers decide the verdict", func(t *testing.T) {
		report := namedProfileCheck(t, limits, code, nil)
		assert.Equal(t, check.VerdictPassed, report.Verdict, "kapi check fails on a critical finding by default: %v", report.DidNotRun)
		body := limitFindings(report)["func/Body/comment"]
		require.Len(t, body, 1, "%+v", report.Findings)
		assert.Equal(t, "comment.length", body[0].Rule)
		assert.Positive(t, report.Target.Blocks)

		rules := analyzerRun(t, report, "voice.rules")
		assert.Equal(t, check.AnalyzerNotApplicable, rules.Status)
		assert.False(t, rules.Required)
		assert.Nil(t, rules.Canary)
		assert.Contains(t, rules.Reason, "comment limits")
		for _, id := range []string{commentSentenceAnalyzer, commentLengthAnalyzer} {
			run := analyzerRun(t, report, id)
			assert.Equal(t, check.AnalyzerFindings, run.Status, id)
			assert.True(t, run.Required, id)
			require.NotNil(t, run.Canary, id)
			assert.Equal(t, check.CanaryCaught, run.Canary.Status, id)
		}

		gated := namedProfileCheck(t, limits, code, map[string]string{"max-major": "0"})
		assert.Equal(t, check.VerdictFailed, gated.Verdict, gated.DidNotRun)
		assert.Equal(t, []string{"major findings 2 exceed limit 0"}, gated.Gate.Failed)
	})

	t.Run("in a check scoped to a diff", func(t *testing.T) {
		work := t.TempDir()
		writeCheckInput(t, work, "parse.go", g.src)
		t.Chdir(work)
		cmd := diffCommand(t)
		cmd.Flags().String("profile-file", limits, "")
		require.NoError(t, cmd.Flags().Set("diff-file", StdinName))
		cmd.SetIn(bytes.NewBufferString(addFilePatch("parse.go", g.src)))
		report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, nil)
		require.NoError(t, err)

		assert.NotEqual(t, check.VerdictDidNotRun, report.Verdict, report.DidNotRun)
		assert.Equal(t, check.AnalyzerNotApplicable, analyzerRun(t, report, "voice.rules").Status)
		assert.Len(t, limitFindings(report)["func/Body/comment"], 1, "%+v", report.Findings)
		density := analyzerRun(t, report, commentDensityAnalyzer)
		require.NotNil(t, density.Canary)
		assert.Equal(t, check.CanaryCaught, density.Canary.Status)
	})

	t.Run("a file holding no comment did not run", func(t *testing.T) {
		bare := writeCheckInput(t, dir, "bare.go", "package code\n\nfunc Parse() {}\n")
		report := namedProfileCheck(t, limits, bare, nil)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
		assert.Equal(t, check.CauseNothingToCheck, report.DidNotRunCause)
	})

	t.Run("must fail: comment limits over content holding no comment did not run", func(t *testing.T) {
		catalog := writeCheckInput(t, dir, "app.json", `{"title":"Hello world"}`)
		report := namedProfileCheck(t, limits, catalog, nil)
		rules := analyzerRun(t, report, "voice.rules")
		assert.Equal(t, check.AnalyzerDidNotRun, rules.Status)
		assert.True(t, rules.Required)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict, "the named profile's limits had no comment to check")
	})

	t.Run("must fail: a profile holding no rule of any kind did not run over comments", func(t *testing.T) {
		tone := writeCheckInput(t, dir, "tone.yaml", "id: tone\nname: Tone\ntone:\n  personality: [plain]\n")
		report := namedProfileCheck(t, tone, code, nil)
		rules := analyzerRun(t, report, "voice.rules")
		assert.Equal(t, check.AnalyzerDidNotRun, rules.Status)
		assert.True(t, rules.Required)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
	})

	t.Run("must fail: a profile holding a term keeps voice.rules required", func(t *testing.T) {
		withTerm := writeCheckInput(t, dir, "term.yaml", "name: Comments and a term\nvocabulary:\n  forbidden_terms:\n    - term: risk-free\nstyle:\n  comments: {}\n")
		report := namedProfileCheck(t, withTerm, code, nil)
		rules := analyzerRun(t, report, "voice.rules")
		assert.Equal(t, check.AnalyzerPassed, rules.Status)
		assert.True(t, rules.Required)
		require.NotNil(t, rules.Canary)
		assert.Equal(t, check.CanaryCaught, rules.Canary.Status)
		assert.Equal(t, check.VerdictPassed, report.Verdict, report.DidNotRun)
	})
}
