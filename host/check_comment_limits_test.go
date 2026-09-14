package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
)

// proseWords is n words of prose that repeat no word next to itself, cut into
// lines of at most perLine words.
func limitProse(first string, n, perLine int, stop bool) []string {
	vocabulary := []string{"reads", "each", "value", "from", "the", "input", "and", "keeps", "what", "it", "finds"}
	var lines []string
	line := []string{first}
	for i := 1; i < n; i++ {
		if len(line) == perLine {
			lines = append(lines, strings.Join(line, " "))
			line = nil
		}
		line = append(line, vocabulary[(i-1)%len(vocabulary)])
	}
	last := strings.Join(line, " ")
	if stop {
		last += "."
	}
	return append(lines, last)
}

// limitsGo is a Go file with three comments over the default comment limits:
// the doc comment of Parse holds a sentence of 55 words, the doc comment of
// Retry one of 75 words, and the comment inside Body holds 105 words in short
// sentences. lines records where each comment sits.
type limitsGo struct {
	src   string
	lines map[string]format.LineRange
	// sentences are the long sentences as a reader sees them.
	sentences map[string]string
}

func newLimitsGo() limitsGo {
	var src []string
	g := limitsGo{lines: map[string]format.LineRange{}, sentences: map[string]string{}}
	comment := func(block, indent string, text []string) {
		first := len(src) + 1
		for _, l := range text {
			src = append(src, indent+"// "+l)
		}
		g.lines[block] = format.LineRange{First: first, Last: len(src)}
	}
	src = append(src, "package code", "")
	parse := limitProse("Parse", 55, 30, true)
	comment("func/Parse", "", parse)
	g.sentences["func/Parse"] = strings.Join(parse, " ")
	src = append(src, "func Parse() {}", "")
	retry := limitProse("Retry", 75, 25, true)
	comment("func/Retry", "", retry)
	g.sentences["func/Retry"] = strings.Join(retry, " ")
	src = append(src, "func Retry() {}", "", "func Body() {")
	var body []string
	for range 21 {
		body = append(body, limitProse("Then", 5, 5, true)...)
	}
	comment("func/Body/comment", "\t", body)
	src = append(src, "\t_ = 1", "}", "")
	g.src = strings.Join(src, "\n")
	return g
}

// commentLimitsProject declares code/*.go at site/web and writes file there.
// Its comments sit at source/comments, whose voice sets the comment limits,
// when apart is set, and at site/web, whose voice sets none, otherwise.
func commentLimitsProject(t *testing.T, apart bool, file string) string {
	t.Helper()
	isolateCheckExecution(t)
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	comments := "true"
	if apart {
		comments = "\n          channel: source/comments"
	}
	write("kapi.yaml", `version: v1
name: comment-limits
defaults:
  source_language: en
profiles:
  site:
    channels: [web]
  source:
    channels: [comments]
collections:
  - name: code
    channel: site/web
    source_only: true
    content:
      - path: "code/*.go"
        comments: `+comments+`
`)
	write(".kapi/profiles/site/voice.yaml", "name: Site\n")
	write(".kapi/profiles/source/voice.yaml", "name: Source comments\nstyle:\n  sentence_length: short\n  comments: {}\n")
	write("code/parse.go", file)
	return root
}

// limitFindings are the comment-limit findings of a report, keyed by block.
func limitFindings(report check.Report) map[string][]check.Diagnostic {
	out := map[string][]check.Diagnostic{}
	for _, d := range report.Findings {
		if d.Check == commentCheck {
			out[d.Location.Block] = append(out[d.Location.Block], d)
		}
	}
	return out
}

func TestCheckHoldsCommentsToTheirLimits(t *testing.T) {
	g := newLimitsGo()

	t.Run("each comment over a limit is a finding at its exact place", func(t *testing.T) {
		report := checkProject(t, commentLimitsProject(t, true, g.src))
		require.NotEqual(t, check.VerdictDidNotRun, report.Verdict, report.DidNotRun)
		found := limitFindings(report)
		require.Len(t, found, 3, "%+v", report.Findings)

		type want struct {
			rule     string
			severity check.Severity
			message  string
			snippet  string
		}
		for block, w := range map[string]want{
			"func/Parse":        {"comment.sentence-length", check.SeverityMinor, "Sentence has 55 words, over the limit of 50", g.sentences["func/Parse"]},
			"func/Retry":        {"comment.sentence-length", check.SeverityMajor, "Sentence has 75 words, over the limit of 70", g.sentences["func/Retry"]},
			"func/Body/comment": {"comment.length", check.SeverityMajor, "Comment has 105 words, over the limit of 100", ""},
		} {
			require.Len(t, found[block], 1, block)
			d := found[block][0]
			assert.Equal(t, w.rule, d.Rule, block)
			assert.Equal(t, w.severity, d.Severity, block)
			assert.Equal(t, w.message, d.Message, block)
			assert.Equal(t, w.snippet, d.Location.Snippet, block)
			require.NotNil(t, d.Location.Lines, block)
			assert.Equal(t, g.lines[block], *d.Location.Lines, "%s names the lines of its comment", block)
			require.NotNil(t, d.Point, block)
			assert.Equal(t, check.Point{Profile: "source", Channel: "comments", Comments: true}, *d.Point, block)
		}
		for block := range map[string]bool{"func/Parse": true, "func/Retry": true} {
			require.NotNil(t, found[block][0].Location.Anchor, "a sentence finding points into its comment")
		}

		for _, id := range []string{commentSentenceAnalyzer, commentLengthAnalyzer} {
			run := analyzerRun(t, report, id)
			assert.Equal(t, check.AnalyzerFindings, run.Status, id)
			assert.True(t, run.Required, id)
			require.NotNil(t, run.Canary, id)
			assert.Equal(t, check.CanaryCaught, run.Canary.Status, id)
			require.NotNil(t, run.Point, id)
			assert.Equal(t, "comments", run.Point.Channel, id)
		}
		assert.Equal(t, check.VerdictPassed, report.Verdict, "kapi check fails on a critical finding by default, and these are minor and major")
	})

	t.Run("a major finding fails a gate that allows no major finding", func(t *testing.T) {
		root := commentLimitsProject(t, true, g.src)
		cmd := executionCommand(t)
		cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
		require.NoError(t, cmd.Flags().Set("max-major", "0"))
		report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, nil)
		require.NoError(t, err)
		assert.Equal(t, check.VerdictFailed, report.Verdict)
		assert.Equal(t, []string{"major findings 2 exceed limit 0"}, report.Gate.Failed)
	})

	t.Run("must fail: comments held to a voice that sets no limits report nothing", func(t *testing.T) {
		report := checkProject(t, commentLimitsProject(t, false, g.src))
		assert.Empty(t, limitFindings(report))
		for _, id := range []string{commentSentenceAnalyzer, commentLengthAnalyzer} {
			assert.Equal(t, check.AnalyzerNotRequested, analyzerRun(t, report, id).Status, id)
		}
	})

	t.Run("must fail: a sentence check that finds nothing invalidates the run", func(t *testing.T) {
		saved := commentSentenceFindings
		commentSentenceFindings = func(context.Context, check.SentenceBreak, *model.Block, check.CommentLimits, model.LocaleID) ([]check.Finding, error) {
			return nil, nil
		}
		t.Cleanup(func() { commentSentenceFindings = saved })

		report := checkProject(t, commentLimitsProject(t, true, g.src))
		assert.Equal(t, check.AnalyzerInvalid, analyzerRun(t, report, commentSentenceAnalyzer).Status)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
		assert.Equal(t, check.CauseCheckerInvalid, report.DidNotRunCause)
	})

	t.Run("must fail: a length check that finds nothing invalidates the run", func(t *testing.T) {
		saved := commentLengthFindings
		commentLengthFindings = func(*model.Block, check.CommentLimits) []check.Finding { return nil }
		t.Cleanup(func() { commentLengthFindings = saved })

		report := checkProject(t, commentLimitsProject(t, true, g.src))
		assert.Equal(t, check.AnalyzerInvalid, analyzerRun(t, report, commentLengthAnalyzer).Status)
		assert.Equal(t, check.CauseCheckerInvalid, report.DidNotRunCause)
	})

	t.Run("must fail: without a sentence break the sentence check did not run", func(t *testing.T) {
		saved := sentenceBreak
		sentenceBreak = func() (segment.Segmenter, error) {
			return nil, fmt.Errorf("%w: %q", segment.ErrEngineUnavailable, "uax29")
		}
		t.Cleanup(func() { sentenceBreak = saved })

		report := checkProject(t, commentLimitsProject(t, true, g.src))
		run := analyzerRun(t, report, commentSentenceAnalyzer)
		assert.Equal(t, check.AnalyzerDidNotRun, run.Status)
		assert.True(t, run.Required)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict, "zero coverage is not a pass")
		assert.Equal(t, check.CauseContentNotChecked, report.DidNotRunCause)
	})
}

func TestDiffCheckHoldsTouchedCommentsToTheirLimits(t *testing.T) {
	g := newLimitsGo()
	retry := g.lines["func/Retry"]
	lines := strings.Split(g.src, "\n")
	patch := fmt.Sprintf("--- a/code/parse.go\n+++ b/code/parse.go\n@@ -%d +%d @@\n-// Retry once.\n+%s\n",
		retry.First, retry.First, lines[retry.First-1])

	report := diffCheckProject(t, commentLimitsProject(t, true, g.src), patch)
	found := limitFindings(report)
	require.Len(t, found, 1, "only the comment the change touched is checked: %+v", report.Findings)
	require.Len(t, found["func/Retry"], 1)
	d := found["func/Retry"][0]
	assert.Equal(t, "comment.sentence-length", d.Rule)
	require.NotNil(t, d.Location.Lines)
	assert.Equal(t, retry, *d.Location.Lines)
	assert.Equal(t, check.CanaryCaught, analyzerRun(t, report, commentSentenceAnalyzer).Canary.Status)
}

func TestShipGateHoldsCommentsToTheirLimits(t *testing.T) {
	g := newLimitsGo()
	root := commentLimitsProject(t, true, g.src)
	out, err := (&App{}).computeVerify(sourceShipCommand(t, root), nil)
	require.NoError(t, err)
	qa, ok := gateByName(out, gateChecks)
	require.True(t, ok)

	severities := map[string]string{}
	for _, f := range qa.Findings {
		if strings.HasPrefix(f.Message, "Sentence has") || strings.Contains(f.Message, "words, over the limit") {
			severities[f.Block] = f.Severity
		}
	}
	assert.Equal(t, map[string]string{"func/Parse": "warning", "func/Retry": "error", "func/Body/comment": "error"}, severities,
		"the ship gate fails on a major finding and warns on a minor one")
	assert.False(t, qa.Pass)
}
