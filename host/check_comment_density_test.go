package host

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/diffscope"
	"github.com/neokapi/neokapi/core/format"
)

// denseGo is a Go file whose doc comment of nine lines sits beside four code
// lines, and packageDocGo one whose nine comment lines are its package doc.
var (
	denseGo      = "package code\n\n" + nineLines("Parse") + "func Parse() {\n\t_ = 1\n}\n"
	packageDocGo = nineLines("Package code") + "package code\n\nfunc Parse() {\n\t_ = 1\n}\n"
)

func nineLines(first string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// %s reads the input.\n", first)
	for i := 2; i <= 9; i++ {
		fmt.Fprintf(&b, "// It keeps value %d.\n", i)
	}
	return b.String()
}

// addFilePatch is a diff that adds path whole with content.
func addFilePatch(path, content string) string {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	return fmt.Sprintf("diff --git a/%[1]s b/%[1]s\nnew file mode 100644\n--- /dev/null\n+++ b/%[1]s\n@@ -0,0 +1,%[2]d @@\n+%[3]s\n",
		path, len(lines), strings.Join(lines, "\n+"))
}

func densityFindings(report check.Report) []check.Diagnostic {
	var out []check.Diagnostic
	for _, d := range report.Findings {
		if d.Rule == "comment.density" {
			out = append(out, d)
		}
	}
	return out
}

func TestAddedLinesLeaveDeletionsOut(t *testing.T) {
	changes := []diffscope.Change{
		{Lines: format.LineRange{First: 3, Last: 5}},
		{Lines: format.LineRange{First: 8, Last: 9}, Deletion: true, After: 8},
		{Lines: format.LineRange{First: 12, Last: 12}},
	}
	assert.Equal(t, []format.LineRange{{First: 3, Last: 5}, {First: 12, Last: 12}}, addedLines(changes),
		"a deletion names the lines beside it, which it did not add")
}

func TestDiffCheckMeasuresCommentDensity(t *testing.T) {
	t.Run("a change adding more comment lines than code lines is a finding", func(t *testing.T) {
		report := diffCheckProject(t, commentLimitsProject(t, true, denseGo), addFilePatch("code/parse.go", denseGo))
		found := densityFindings(report)
		require.Len(t, found, 1, "%+v", report.Findings)
		d := found[0]
		assert.Equal(t, check.SeverityMajor, d.Severity)
		assert.Equal(t, "Change adds 9 comment lines and 4 code lines, more than 1 comment lines for each code line", d.Message)
		assert.Empty(t, d.Location.Block, "density is a property of the change, not of one comment")
		require.NotNil(t, d.Location.Lines)
		assert.Equal(t, format.LineRange{First: 3, Last: 11}, *d.Location.Lines, "the first and last comment lines the change added")
		require.NotNil(t, d.Point)
		assert.Equal(t, check.Point{Profile: "source", Channel: "comments", Comments: true}, *d.Point)

		run := analyzerRun(t, report, commentDensityAnalyzer)
		assert.Equal(t, check.AnalyzerFindings, run.Status)
		require.NotNil(t, run.Canary)
		assert.Equal(t, check.CanaryCaught, run.Canary.Status)
	})

	t.Run("the package doc comment is not counted", func(t *testing.T) {
		report := diffCheckProject(t, commentLimitsProject(t, true, packageDocGo), addFilePatch("code/parse.go", packageDocGo))
		assert.Empty(t, densityFindings(report))
		assert.Equal(t, check.AnalyzerPassed, analyzerRun(t, report, commentDensityAnalyzer).Status)
	})

	t.Run("a whole-file check reports density as unsupported", func(t *testing.T) {
		report := checkProject(t, commentLimitsProject(t, true, denseGo))
		run := analyzerRun(t, report, commentDensityAnalyzer)
		assert.Equal(t, check.AnalyzerUnsupported, run.Status)
		assert.Contains(t, run.Reason, "property of a change")
		assert.Empty(t, densityFindings(report))
	})

	t.Run("must fail: a density check that reads comment markers misses its canary", func(t *testing.T) {
		saved := lineKinds
		lineKinds = func(_ *comment.File, src []byte) []comment.LineKind {
			kinds := []comment.LineKind{comment.LineBlank}
			for line := range strings.SplitSeq(string(src), "\n") {
				switch t := strings.TrimSpace(line); {
				case t == "":
					kinds = append(kinds, comment.LineBlank)
				case strings.HasPrefix(t, "//"):
					kinds = append(kinds, comment.LineComment)
				default:
					kinds = append(kinds, comment.LineCode)
				}
			}
			return kinds
		}
		t.Cleanup(func() { lineKinds = saved })

		report := diffCheckProject(t, commentLimitsProject(t, true, denseGo), addFilePatch("code/parse.go", denseGo))
		assert.Equal(t, check.AnalyzerInvalid, analyzerRun(t, report, commentDensityAnalyzer).Status)
		assert.Equal(t, check.CauseCheckerInvalid, report.DidNotRunCause)
	})

	t.Run("must fail: a density check that finds nothing invalidates the run", func(t *testing.T) {
		saved := commentDensityFindings
		commentDensityFindings = func(check.ChangeLines, check.CommentLimits) []check.Finding { return nil }
		t.Cleanup(func() { commentDensityFindings = saved })

		report := diffCheckProject(t, commentLimitsProject(t, true, denseGo), addFilePatch("code/parse.go", denseGo))
		assert.Equal(t, check.AnalyzerInvalid, analyzerRun(t, report, commentDensityAnalyzer).Status)
	})
}
