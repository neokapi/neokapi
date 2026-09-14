package host

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

// commentsOnlyApart is what a check reports when the items are declared for
// their comments alone at source/comments: the comments' findings, and none on
// the YAML value.
var commentsOnlyApart = []governedFinding{
	{"app.yaml", "comment/farewell", "FIXME"},
	{"app.yaml", "comment/farewell", "LegacyName"},
	{"parse.go", "func/Retry", "FIXME"},
}

// An item declared with `comments: {only: true}` is checked at the point its
// comments sit at, by every check surface, and its values are never read. The
// must-fail cases are the ones in check_comments_point_test.go, where the same
// items with `comments: {channel: ...}` report the value's findings too.
func TestChecksReadOnlyTheCommentsOfACommentsOnlyItem(t *testing.T) {
	t.Run("a project check", func(t *testing.T) {
		report := checkProject(t, commentPointProject(t, onlyOnItem))
		assert.NotEqual(t, check.VerdictDidNotRun, report.Verdict, report.DidNotRun)
		assert.Equal(t, 4, report.Target.Blocks, "two YAML comments and two Go comments")
		assert.ElementsMatch(t, commentsOnlyApart, governedFindings(report.Findings))
		require.NotNil(t, report.Execution)
		assert.Equal(t, map[string][]string{"app.yaml": {"comments"}, "parse.go": {"comments"}}, contextChannels(report.Execution.Contexts))
		assertContextPoints(t, report.Execution.Contexts)
		assertPoints(t, report.Findings, true)
		assert.Equal(t, map[string][]string{"app.yaml": {"source/comments"}, "parse.go": {"source/comments"}}, ruleRuns(report.Execution.Analyzers))
		for _, run := range report.Execution.Analyzers {
			if run.ID == "reader.validation" && filepath.Base(run.File) == "app.yaml" {
				assert.NotEqual(t, check.AnalyzerPassed, run.Status, "no reader parsed the values, so reader validation never passes")
			}
		}
	})

	t.Run("named files", func(t *testing.T) {
		root := commentPointProject(t, onlyOnItem)
		cmd := executionCommand(t)
		cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
		report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{filepath.Join(root, "config", "app.yaml"), filepath.Join(root, "code", "parse.go")})
		require.NoError(t, err)
		assert.Equal(t, 4, report.Target.Blocks)
		assert.ElementsMatch(t, commentsOnlyApart, governedFindings(report.Findings))
		assertPoints(t, report.Findings, true)
	})

	t.Run("a diff-scoped check", func(t *testing.T) {
		const patch = "--- a/config/app.yaml\n+++ b/config/app.yaml\n@@ -1,3 +1,3 @@\n-# Callers use this value.\n-greeting: Hello\n-# Confirm the farewell.\n" +
			"+# Callers utilize this value, and ScopedName is fine in a comment.\n+greeting: We utilize ScopedName and LegacyName. FIXME later\n+# FIXME: LegacyName is retired here.\n"
		report := diffCheckProject(t, commentPointProject(t, onlyOnItem), patch)
		assert.Equal(t, 2, report.Target.Blocks, "the two comments the change touches, and no value")
		assert.ElementsMatch(t, inFile("app.yaml", commentsOnlyApart), governedFindings(report.Findings))
		assertPoints(t, report.Findings, true)
	})

	t.Run("the ship gates", func(t *testing.T) {
		out, err := (&App{}).computeVerify(sourceShipCommand(t, commentPointProject(t, onlyOnItem)), nil)
		require.NoError(t, err)
		for name, want := range map[string][]governedFinding{
			gateTerms:  caughtOnly(commentsOnlyApart, "ScopedName", "LegacyName"),
			gateVoice:  commentsOnlyApart,
			gateChecks: commentsOnlyApart,
		} {
			gate, ok := gateByName(out, name)
			require.True(t, ok, "the %s gate ran", name)
			require.NotNil(t, gate.Coverage, name)
			assert.Positive(t, gate.Coverage.Blocks, name)
			var got []governedFinding
			for _, f := range gate.Findings {
				if f.Block == "" {
					continue
				}
				got = append(got, governedFinding{file: filepath.Base(f.File), block: blockKind(f.Block), caught: caughtWord(f.Message)})
				if assert.NotNil(t, f.Point, "%s %s %s", name, f.File, f.Block) {
					assert.Equal(t, commentsPoint, *f.Point, "%s %s %s", name, f.File, f.Block)
				}
			}
			assert.ElementsMatch(t, want, got, name)
		}
	})

	t.Run("MCP check_file", func(t *testing.T) {
		report := checkFileOverMCP(t, commentPointProject(t, onlyOnItem), "config/app.yaml")
		assert.Equal(t, 2, report.Target.Blocks)
		assert.ElementsMatch(t, inFile("app.yaml", commentsOnlyApart), governedFindings(report.Findings))
		assertPoints(t, report.Findings, true)
	})
}
