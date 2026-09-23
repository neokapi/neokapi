package host

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/diffscope"
	"github.com/neokapi/neokapi/core/format"
)

// diffCheckFiles lays files out in a fresh directory with no project, changes
// into it, and runs cmd there over patch. A nil cmd is a plain diff-scoped check.
func diffCheckFiles(t *testing.T, cmd *EnvCommand, files map[string]string, patch string) check.Report {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		writeCheckInput(t, dir, name, content)
	}
	t.Chdir(dir)
	if cmd == nil {
		cmd = diffCommand(t)
	}
	require.NoError(t, cmd.Flags().Set("diff-file", StdinName))
	cmd.SetIn(bytes.NewBufferString(patch))
	report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, nil)
	require.NoError(t, err)
	return report
}

// parseGo holds a doubled word on line 3 and another on line 7. editParse
// changes line 4, inside the first comment.
const (
	parseGo   = "package demo\n\n// Parse reads the the input.\n// It stops at the end.\nfunc Parse() {}\n\n// Other is is untouched.\nfunc Other() {}\n"
	editParse = "--- a/parse.go\n+++ b/parse.go\n@@ -4 +4 @@\n-// It stops at the finish.\n+// It stops at the end.\n"
)

// A diff scopes the comments in a Go file the way it scopes a reader's blocks:
// a changed line widens to the comment it sits in, that comment is checked
// whole, and a comment the change did not touch is not checked at all.
func TestDiffCheckScopesGoComments(t *testing.T) {
	isolateCheckExecution(t)

	t.Run("a change inside a comment checks that whole comment and nothing else", func(t *testing.T) {
		report := diffCheckFiles(t, nil, map[string]string{"parse.go": parseGo}, editParse)

		entry := scopeEntry(t, report, "parse.go")
		assert.Equal(t, check.ScopeChecked, entry.Status)
		assert.Equal(t, []check.ScopeBlock{{Block: "func/Parse", Lines: format.LineRange{First: 3, Last: 4}}}, entry.Blocks)

		require.Len(t, report.Findings, 1, "the untouched comment's doubled word is not reported")
		finding := report.Findings[0]
		assert.Equal(t, "hygiene.doubled-word", finding.Rule)
		assert.Equal(t, "func/Parse", finding.Location.Block, "the fault sits on line 3, which the change did not touch, inside the comment it did")
		require.NotNil(t, finding.Location.Lines)
		assert.Equal(t, format.LineRange{First: 3, Last: 4}, *finding.Location.Lines)

		for _, id := range []string{"comments.go", "formatter.gofmt"} {
			run := analyzerRun(t, report, id)
			require.NotNil(t, run.Canary, id)
			assert.Equal(t, check.CanaryCaught, run.Canary.Status, id)
		}
		assert.Equal(t, check.VerdictPassed, report.Verdict, report.DidNotRun)
	})

	t.Run("a change to code alone touches no comment", func(t *testing.T) {
		report := diffCheckFiles(t, nil, map[string]string{"parse.go": parseGo},
			"--- a/parse.go\n+++ b/parse.go\n@@ -5 +5 @@\n-func Parse(){}\n+func Parse() {}\n")
		assert.Equal(t, check.ScopeUntouched, scopeEntry(t, report, "parse.go").Status)
		assert.Empty(t, report.Findings)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
		assert.Equal(t, check.CauseNothingToCheck, report.DidNotRunCause)
	})

	t.Run("a change to directives alone checks nothing", func(t *testing.T) {
		// A directive group of its own on line 3, and a directive on line 7 in the
		// group of a comment that holds a doubled word.
		const directives = "package demo\n\n//go:generate stringer -type=Kind\n\n// Parse reads the the input.\n//\n//go:noinline\nfunc Parse() {}\n"
		report := diffCheckFiles(t, nil, map[string]string{"parse.go": directives},
			"--- a/parse.go\n+++ b/parse.go\n@@ -3 +3 @@\n-//go:generate stringer\n+//go:generate stringer -type=Kind\n@@ -7 +7 @@\n-//go:nosplit\n+//go:noinline\n")
		entry := scopeEntry(t, report, "parse.go")
		assert.Equal(t, check.ScopeUntouched, entry.Status, "%+v", entry.Blocks)
		assert.Empty(t, report.Findings)
		assert.Equal(t, check.CauseNothingToCheck, report.DidNotRunCause)
	})

	t.Run("must fail: without a Go comment provider the file has no reader", func(t *testing.T) {
		saved := commentProviders
		commentProviders = comment.NewRegistry()
		t.Cleanup(func() { commentProviders = saved })

		report := diffCheckFiles(t, nil, map[string]string{"parse.go": parseGo}, editParse)
		assert.Equal(t, check.ScopeNoReader, scopeEntry(t, report, "parse.go").Status)
		assert.Empty(t, report.Findings)
	})
}

// A deletion that only borders a comment leaves the comment out of scope when
// the file before the change held it unchanged, which the check learns by
// locating the comments of the pre-image the diff rebuilds.
func TestDiffCheckGoCommentBesideADeletion(t *testing.T) {
	isolateCheckExecution(t)

	t.Run("a deletion bordering a comment it leaves unchanged checks nothing", func(t *testing.T) {
		// Lines 6 and 7 of the earlier file, a function and a blank line, are
		// removed; the comment after them holds a doubled word and is unchanged.
		const after = "package demo\n\n// Parse reads the input.\nfunc Parse() {}\n\n// Other is is untouched.\nfunc Other() {}\n"
		report := diffCheckFiles(t, nil, map[string]string{"other.go": after},
			"--- a/other.go\n+++ b/other.go\n@@ -6,2 +5,0 @@\n-func removed() {}\n-\n")

		entry := scopeEntry(t, report, "other.go")
		assert.Equal(t, check.ScopeUntouched, entry.Status, "%+v", entry.Blocks)
		assert.Empty(t, report.Findings)
		assert.Equal(t, check.CauseNothingToCheck, report.DidNotRunCause)
	})

	t.Run("must fail: a deletion that removes a comment's last line checks that comment", func(t *testing.T) {
		// The earlier file's comment ran over lines 3 and 4; line 4 is removed.
		const after = "package demo\n\n// Other is is untouched.\nfunc Other() {}\n"
		report := diffCheckFiles(t, nil, map[string]string{"other.go": after},
			"--- a/other.go\n+++ b/other.go\n@@ -4 +3,0 @@\n-// This line goes.\n")

		entry := scopeEntry(t, report, "other.go")
		assert.Equal(t, check.ScopeChecked, entry.Status)
		assert.Equal(t, []check.ScopeBlock{{Block: "func/Other", Lines: format.LineRange{First: 3, Last: 3}}}, entry.Blocks)
		require.Len(t, report.Findings, 1)
		assert.Equal(t, "hygiene.doubled-word", report.Findings[0].Rule)
	})
}

// The formatter is an analyzer the Go comment provider brings, and a
// diff-scoped check runs it over the comments the change touched.
func TestDiffCheckGoCommentFormatter(t *testing.T) {
	isolateCheckExecution(t)
	// B's comment is indented with spaces, which gofmt rewrites; A's is not.
	const indent = "package demo\n\nfunc A() {\n\t// First comment.\n\t_ = 1\n}\n\nfunc B() {\n  // Second comment.\n\t_ = 2\n}\n"
	const editA = "--- a/indent.go\n+++ b/indent.go\n@@ -4 +4 @@\n-\t// First.\n+\t// First comment.\n"

	t.Run("a disagreement in a comment the change did not touch is not reported", func(t *testing.T) {
		report := diffCheckFiles(t, nil, map[string]string{"indent.go": indent}, editA)

		whole, err := (&App{SourceLang: "en"}).ComputeCheck(executionCommand(t), []string{"indent.go"})
		require.NoError(t, err)
		existing := findingsOf(whole, formatterCheck)
		require.Len(t, existing, 1, "the file already holds a comment gofmt rewrites")
		assert.Equal(t, "func/B/comment", existing[0].Location.Block)

		assert.Equal(t, []check.ScopeBlock{{Block: "func/A/comment", Lines: format.LineRange{First: 4, Last: 4}}}, scopeEntry(t, report, "indent.go").Blocks)
		assert.Empty(t, findingsOf(report, formatterCheck))
		run := analyzerRun(t, report, "formatter.gofmt")
		assert.Equal(t, check.AnalyzerPassed, run.Status)
		assert.Equal(t, 0, run.Findings)
		require.NotNil(t, run.Canary)
		assert.Equal(t, check.CanaryCaught, run.Canary.Status)
		assert.Equal(t, check.VerdictPassed, report.Verdict, report.Findings)
	})

	t.Run("must fail: a comment the change touched and gofmt rewrites fails the check", func(t *testing.T) {
		report := diffCheckFiles(t, nil, map[string]string{"indent.go": indent},
			"--- a/indent.go\n+++ b/indent.go\n@@ -9 +9 @@\n-  // Second.\n+  // Second comment.\n")

		formatter := findingsOf(report, formatterCheck)
		require.Len(t, formatter, 1)
		assert.Equal(t, "func/B/comment", formatter[0].Location.Block)
		assert.Equal(t, "indent.go", formatter[0].Location.File)
		require.NotNil(t, formatter[0].Location.Lines)
		assert.Equal(t, format.LineRange{First: 9, Last: 9}, *formatter[0].Location.Lines)
		assert.Equal(t, 1, analyzerRun(t, report, "formatter.gofmt").Findings)
		assert.Equal(t, check.VerdictFailed, report.Verdict)
	})

	t.Run("a formatter that cannot compare the file did not run", func(t *testing.T) {
		swapCommentProviders(t, failingFormatterProvider{})
		report := diffCheckFiles(t, nil, map[string]string{"parse.go": parseGo}, editParse)

		run := analyzerRun(t, report, "formatter.gofmt")
		assert.Equal(t, check.AnalyzerDidNotRun, run.Status)
		assert.Contains(t, run.Reason, "could not compare")
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
		assert.Equal(t, check.CauseContentNotChecked, report.DidNotRunCause)
	})
}

// A diff-scoped check records the same analyzers for a Go file as a whole-file
// check of it, in the same order and with the same outcomes. Reader validation
// is left out of the comparison: a diff-scoped check refuses --validate.
func TestDiffCheckGoCommentAnalyzersMatchAWholeFileCheck(t *testing.T) {
	isolateCheckExecution(t)
	type analyzer struct {
		ID       string
		Status   check.AnalyzerStatus
		Required bool
		Canary   check.CanaryStatus
	}
	analyzersOf := func(report check.Report) []analyzer {
		require.NotNil(t, report.Execution)
		var out []analyzer
		for _, run := range report.Execution.Analyzers {
			if run.File != "parse.go" || run.ID == "reader.validation" {
				continue
			}
			a := analyzer{ID: run.ID, Status: run.Status, Required: run.Required}
			if run.Canary != nil {
				a.Canary = run.Canary.Status
			}
			out = append(out, a)
		}
		return out
	}

	diff := diffCheckFiles(t, nil, map[string]string{"parse.go": parseGo}, editParse)
	whole, err := (&App{SourceLang: "en"}).ComputeCheck(executionCommand(t), []string{"parse.go"})
	require.NoError(t, err)

	scoped := analyzersOf(diff)
	ids := make([]string, len(scoped))
	for i, a := range scoped {
		ids[i] = a.ID
	}
	assert.Subset(t, ids, []string{"comments.go", "formatter.gofmt", "hygiene"})
	assert.Equal(t, analyzersOf(whole), scoped)
}

// goCommentCommit lays out the Go files a real commit changed, as that commit
// left them, and returns its diff. The commit (99763a528) rewrote one link line
// inside a doc comment in each of core/format/reader.go and core/format/writer.go,
// files that hold many comments it did not touch.
func goCommentCommit(t *testing.T) (string, []byte) {
	t.Helper()
	fixture, err := filepath.Abs(filepath.Join("testdata", "diffscope", "go-comment-link"))
	require.NoError(t, err)
	patch, err := os.ReadFile(filepath.Join(fixture, "commit.diff"))
	require.NoError(t, err)
	dir := t.TempDir()
	for _, rel := range []string{"core/format/reader.go", "core/format/writer.go"} {
		data, err := os.ReadFile(filepath.Join(fixture, filepath.FromSlash(rel)))
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, filepath.Dir(filepath.FromSlash(rel))), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), data, 0o644))
	}
	return dir, patch
}

// TestDiffCheckGoCommentsInARealCommit checks a real commit's diff over Go files
// with no project: only the comment each changed line sits in is checked, and
// each such comment is checked whole although the commit changed one line of it.
func TestDiffCheckGoCommentsInARealCommit(t *testing.T) {
	isolateCheckExecution(t)
	checkCommit := func(t *testing.T) (check.Report, []diffscope.File) {
		t.Helper()
		dir, patch := goCommentCommit(t)
		t.Chdir(dir)
		cmd := diffCommand(t)
		require.NoError(t, cmd.Flags().Set("diff-file", StdinName))
		// Every comment breaks this rule, so the findings name exactly the comments checked.
		cmd.Flags().StringSlice("forbid", []string{`\S`}, "")
		cmd.SetIn(bytes.NewReader(patch))
		report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, nil)
		require.NoError(t, err)
		files, err := diffscope.Parse(patch)
		require.NoError(t, err)
		return report, files
	}
	reader, writer := filepath.FromSlash("core/format/reader.go"), filepath.FromSlash("core/format/writer.go")

	t.Run("only the comments the commit touched are checked, each whole", func(t *testing.T) {
		report, files := checkCommit(t)

		want := map[string]check.ScopeBlock{
			reader: {Block: "type/StreamingReader", Lines: format.LineRange{First: 63, Last: 75}},
			writer: {Block: "type/StreamingWriter", Lines: format.LineRange{First: 49, Last: 65}},
		}
		for path, block := range want {
			entry := scopeEntry(t, report, path)
			assert.Equal(t, check.ScopeChecked, entry.Status, path)
			assert.Equal(t, []check.ScopeBlock{block}, entry.Blocks, path)
		}
		assert.Equal(t, 2, report.Target.Blocks)

		patterns := 0
		for _, d := range report.Findings {
			if d.Check != "pattern" {
				continue
			}
			patterns++
			block := want[d.Location.File]
			assert.Equal(t, block.Block, d.Location.Block)
			require.NotNil(t, d.Location.Lines)
			assert.Equal(t, block.Lines, *d.Location.Lines, "a comment touched on one line is reported over all its lines")
		}
		assert.Equal(t, 2, patterns)
		require.NoError(t, scopeMatchesDiff(report, files), "every checked comment meets a change")
		assert.NotEqual(t, check.VerdictDidNotRun, report.Verdict, report.DidNotRun)
	})

	t.Run("must fail: a scope that also claims an untouched comment does not match the diff", func(t *testing.T) {
		report, files := checkCommit(t)
		wrong := *report.Scope
		wrong.Files = nil
		for _, f := range report.Scope.Files {
			if f.Path == reader {
				f.Blocks = append(f.Blocks, check.ScopeBlock{Block: "type/StreamingReader/StreamingReader", Lines: format.LineRange{First: 77, Last: 78}})
			}
			wrong.Files = append(wrong.Files, f)
		}
		report.Scope = &wrong
		require.Error(t, scopeMatchesDiff(report, files))
	})

	t.Run("must fail: without a Go comment provider the files have no reader", func(t *testing.T) {
		saved := commentProviders
		commentProviders = comment.NewRegistry()
		t.Cleanup(func() { commentProviders = saved })
		report, _ := checkCommit(t)
		assert.Equal(t, check.ScopeNoReader, scopeEntry(t, report, reader).Status)
		assert.Equal(t, check.ScopeNoReader, scopeEntry(t, report, writer).Status)
	})
}

// Inside a project a diff checks the Go comments the recipe declares, under the
// governance of each file's point, and lists a changed Go file it does not
// declare as out of scope.
func TestDiffCheckGoCommentsInAProject(t *testing.T) {
	root := commentProjectFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "tools"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "tools", "gen.go"), []byte("package tools\n\n// Gen helps you utilize the the input.\nfunc Gen() {}\n"), 0o600))
	patch := "--- a/src/parse.go\n+++ b/src/parse.go\n@@ -3 +3 @@\n-// Parse helps with the input.\n+// Parse helps you utilize the input.\n" +
		"--- a/tools/gen.go\n+++ b/tools/gen.go\n@@ -3 +3 @@\n-// Gen helps.\n+// Gen helps you utilize the the input.\n"
	writeCheckInput(t, root, "change.diff", patch)
	t.Chdir(root)

	cmd := diffCommand(t)
	cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
	require.NoError(t, cmd.Flags().Set("diff-file", "change.diff"))
	report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, nil)
	require.NoError(t, err)

	parse := scopeEntry(t, report, filepath.FromSlash("src/parse.go"))
	assert.Equal(t, check.ScopeChecked, parse.Status)
	assert.Equal(t, []check.ScopeBlock{{Block: "func/Parse", Lines: format.LineRange{First: 3, Last: 3}}}, parse.Blocks)
	gen := scopeEntry(t, report, filepath.FromSlash("tools/gen.go"))
	assert.Equal(t, check.ScopeOutOfScope, gen.Status)

	voice := findingsOf(report, "voice")
	require.Len(t, voice, 1, "the declared comment is held to the rule at its point")
	assert.Equal(t, "func/Parse", voice[0].Location.Block)
	assert.Empty(t, findingsOf(report, "hygiene"), "the undeclared file's doubled word is not read")
}
