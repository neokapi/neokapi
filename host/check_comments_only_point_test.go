package host

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

// commentsOnlyApart is what a check of the project's declared content reports
// when the items are declared for their comments alone at source/comments: the
// comments' findings, and none on the YAML value.
var commentsOnlyApart = []governedFinding{
	{"app.yaml", "comment/farewell", "FIXME"},
	{"app.yaml", "comment/farewell", "LegacyName"},
	{"parse.go", "func/Retry", "FIXME"},
}

// An item declared with `comments: {only: true}` is checked at the point its
// comments sit at by every check of the project's declared content, and its
// values are never read. The must-fail cases are the ones in
// check_comments_point_test.go, where the same items with
// `comments: {channel: ...}` report the value's findings too.
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

	t.Run("a check of declared files", func(t *testing.T) {
		root := commentPointProject(t, onlyOnItem)
		cmd := executionCommand(t)
		cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
		report, err := (&App{SourceLang: "en"}).ComputeDeclaredCheck(cmd, []string{filepath.Join(root, "config", "app.yaml"), filepath.Join(root, "code", "parse.go")})
		require.NoError(t, err)
		assert.Equal(t, 4, report.Target.Blocks, "two YAML comments and two Go comments")
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
}

// projectVoice forbids "utilize" at the project's default point.
const projectVoice = `id: project
name: Project
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
`

// namedContentProject is commentPointProject with each item declared for its
// comments alone, and a project voice bound at the default point, where the
// files' own content resolves.
func namedContentProject(t *testing.T) string {
	t.Helper()
	root := commentPointProject(t, onlyOnItem)
	recipe := filepath.Join(root, "kapi.yaml")
	require.NoError(t, os.WriteFile(filepath.Join(root, ".kapi", "voice.yaml"), []byte(projectVoice), 0o600))
	// The import reads the voice and binds it at the default point.
	readProjectContext(t, root)
	p, err := project.Load(recipe)
	require.NoError(t, err)
	require.NotNil(t, p.Defaults.Voice, "the import binds the voice it brings")
	return root
}

// namedContentFindings is what a check of the named files reports: the comments'
// findings at source/comments, and the YAML value's "utilize" at the default
// point. The Go file holds nothing but comments.
var namedContentFindings = append(slices.Clone(commentsOnlyApart), governedFinding{"app.yaml", "value", "utilize"})

// assertNamedPoints holds a value's findings to the project's default point and
// a comment's to source/comments.
func assertNamedPoints(t *testing.T, findings []check.Diagnostic) {
	t.Helper()
	checked := 0
	for _, d := range findings {
		if d.Check != "voice" {
			continue
		}
		checked++
		want := check.Point{}
		if blockKind(d.Location.Block) != "value" {
			want = commentsPoint
		}
		if assert.NotNil(t, d.Point, "%s %s", d.Location.File, d.Location.Block) {
			assert.Equal(t, want, *d.Point, "%s %s", d.Location.File, d.Location.Block)
		}
	}
	assert.Positive(t, checked, "the findings carry points to hold")
}

// A file declared for its comments alone is still content when a caller names
// it. A check of the named file reads its values at the point they resolve to,
// past the item that claims only the comments, and its comments at their own
// point. A check of the project reads only the comments.
func TestNamedCheckReadsTheContentOfAFileDeclaredForItsCommentsAlone(t *testing.T) {
	t.Run("kapi check", func(t *testing.T) {
		root := namedContentProject(t)
		cmd := executionCommand(t)
		cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
		report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{filepath.Join(root, "config", "app.yaml"), filepath.Join(root, "code", "parse.go")})
		require.NoError(t, err)
		assert.Equal(t, 6, report.Target.Blocks, "the YAML file's two values and two comments, and the Go file's two comments")
		assert.ElementsMatch(t, namedContentFindings, governedFindings(report.Findings))
		assertNamedPoints(t, report.Findings)
	})

	t.Run("MCP check_file", func(t *testing.T) {
		report := checkFileOverMCP(t, namedContentProject(t), "config/app.yaml")
		assert.Equal(t, 4, report.Target.Blocks, "the file's two values and two comments")
		assert.ElementsMatch(t, inFile("app.yaml", namedContentFindings), governedFindings(report.Findings))
		assertNamedPoints(t, report.Findings)
	})

	t.Run("a check of the project reads only the comments", func(t *testing.T) {
		report := checkProject(t, namedContentProject(t))
		assert.Equal(t, 4, report.Target.Blocks, "two YAML comments and two Go comments")
		assert.ElementsMatch(t, commentsOnlyApart, governedFindings(report.Findings))
	})
}
