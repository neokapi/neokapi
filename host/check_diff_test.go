package host

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/diffscope"
	"github.com/neokapi/neokapi/core/format"
)

func diffCommand(t *testing.T) *EnvCommand {
	t.Helper()
	cmd := executionCommand(t)
	cmd.Flags().String("diff-file", "", "")
	cmd.Flags().String("diff-against", "", "")
	return cmd
}

func scopeEntry(t *testing.T, report check.Report, path string) check.ScopeFile {
	t.Helper()
	require.NotNil(t, report.Scope)
	for _, f := range report.Scope.Files {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("%s is not in the scope: %+v", path, report.Scope.Files)
	return check.ScopeFile{}
}

// scopeMatchesDiff holds a diff-scoped report to the diff it came from: every
// block the report says it checked must share a line with a change in its file.
// It works line by line and shares no code with diffscope.Touched, so it cannot
// inherit that function's mistakes.
func scopeMatchesDiff(report check.Report, files []diffscope.File) error {
	changes := map[string][]diffscope.Change{}
	for _, f := range files {
		changes[f.Path()] = f.Changes
	}
	for _, sf := range report.Scope.Files {
		for _, b := range sf.Blocks {
			touched := false
			for _, c := range changes[filepath.ToSlash(sf.Path)] {
				for line := b.Lines.First; line <= b.Lines.Last; line++ {
					if line >= c.Lines.First && line <= c.Lines.Last {
						touched = true
					}
				}
			}
			if !touched {
				return fmt.Errorf("%s: block %s (lines %d-%d) was checked but the diff does not touch it", sf.Path, b.Block, b.Lines.First, b.Lines.Last)
			}
		}
	}
	return nil
}

// TestDiffCheck_RealCommit checks the diff of a real commit to this repository
// (7a2c3c4f5, which bumped the Go version in docs/internals/TESTING.md) against
// the file as that commit left it. The commit changed one line of a five-line
// paragraph and one line inside a code fence.
func TestDiffCheck_RealCommit(t *testing.T) {
	isolateCheckExecution(t)
	fixture, err := filepath.Abs(filepath.Join("testdata", "diffscope", "go-bump"))
	require.NoError(t, err)
	post, err := os.ReadFile(filepath.Join(fixture, "TESTING.md"))
	require.NoError(t, err)
	patch, err := os.ReadFile(filepath.Join(fixture, "commit.diff"))
	require.NoError(t, err)

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs", "internals"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "internals", "TESTING.md"), post, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "commit.diff"), patch, 0o644))
	t.Chdir(dir)

	cmd := diffCommand(t)
	require.NoError(t, cmd.Flags().Set("diff-file", "commit.diff"))
	// Every block breaks this rule, so the findings name exactly the blocks checked.
	cmd.Flags().StringSlice("forbid", []string{`\S`}, "")
	app := &App{SourceLang: "en"}
	report, err := app.ComputeCheck(cmd, nil)
	require.NoError(t, err)

	entry := scopeEntry(t, report, filepath.FromSlash("docs/internals/TESTING.md"))
	assert.Equal(t, check.ScopeChecked, entry.Status)
	require.Len(t, entry.Blocks, 1, "the paragraph the commit edited; its other edit sits in a code fence")
	paragraph := entry.Blocks[0]
	assert.Equal(t, format.LineRange{First: 745, Last: 749}, paragraph.Lines, "touched on line 748, checked whole")
	assert.Equal(t, 1, report.Target.Blocks)

	patterns := 0
	for _, d := range report.Findings {
		assert.Equal(t, paragraph.Block, d.Location.Block, d.Rule)
		require.NotNil(t, d.Location.Lines, d.Rule)
		assert.Equal(t, paragraph.Lines, *d.Location.Lines)
		if d.Check == "pattern" {
			patterns++
		}
	}
	assert.Equal(t, 1, patterns)

	files, err := diffscope.Parse(patch)
	require.NoError(t, err)
	require.NoError(t, scopeMatchesDiff(report, files))

	// The must-fail half: a scope that also claims an untouched block.
	wrong := report
	wrongScope := *report.Scope
	wrongScope.Files = []check.ScopeFile{entry}
	wrongScope.Files[0].Blocks = append([]check.ScopeBlock{{Block: "untouched", Lines: format.LineRange{First: 741, Last: 741}}}, entry.Blocks...)
	wrong.Scope = &wrongScope
	require.Error(t, scopeMatchesDiff(wrong, files))

	// The same file checked whole holds far more blocks than the change touched.
	whole := executionCommand(t)
	whole.Flags().StringSlice("forbid", []string{`\S`}, "")
	full, err := app.ComputeCheck(whole, []string{filepath.FromSlash("docs/internals/TESTING.md")})
	require.NoError(t, err)
	assert.Greater(t, full.Target.Blocks, 50)
}

func TestDiffCheck_ProjectScopeReadsOnlyTheDiff(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	writeCheckInput(t, dir, "a.md", "# Title\n\nFirst paragraph.\n\nSecond paragraph.\n")
	writeCheckInput(t, dir, "b.md", "Other.\n")
	untouched := writeCheckInput(t, dir, "z.md", "Never read.\n")
	recipe := writeCheckInput(t, dir, "kapi.yaml", "version: v1\ndefaults:\n  source_language: en\ncollections:\n  - path: a.md\n  - path: z.md\n")
	patch := writeCheckInput(t, dir, "change.diff", "diff --git a/a.md b/a.md\n--- a/a.md\n+++ b/a.md\n@@ -5 +5 @@\n-Second.\n+Second paragraph.\n"+
		"diff --git a/b.md b/b.md\n--- a/b.md\n+++ b/b.md\n@@ -1 +1 @@\n-Old.\n+Other.\n"+
		"diff --git a/c.md b/c.md\ndeleted file mode 100644\n--- a/c.md\n+++ /dev/null\n@@ -1 +0,0 @@\n-Gone.\n")
	t.Setenv("KAPI_NO_PROJECT", "")
	t.Setenv("KAPI_PROJECT", recipe)
	t.Chdir(dir)

	var read []string
	real := readScopedSource
	readScopedSource = func(path string) ([]byte, error) {
		read = append(read, filepath.Base(path))
		return real(path)
	}
	t.Cleanup(func() { readScopedSource = real })
	// A declared file the diff does not name cannot be read at all, so any read
	// of it fails the run.
	require.NoError(t, os.Chmod(untouched, 0o000))
	t.Cleanup(func() { _ = os.Chmod(untouched, 0o644) })

	cmd := diffCommand(t)
	require.NoError(t, cmd.Flags().Set("diff-file", patch))
	report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, nil)
	require.NoError(t, err)

	a := scopeEntry(t, report, "a.md")
	assert.Equal(t, check.ScopeChecked, a.Status)
	require.Len(t, a.Blocks, 1)
	assert.Equal(t, format.LineRange{First: 5, Last: 5}, a.Blocks[0].Lines)
	b := scopeEntry(t, report, "b.md")
	assert.Equal(t, check.ScopeOutOfScope, b.Status)
	assert.Contains(t, b.Reason, "kapi.yaml")
	assert.Equal(t, check.ScopeDeleted, scopeEntry(t, report, "c.md").Status)
	assert.Len(t, report.Scope.Files, 3, "z.md is not in the diff and is not listed")
	assert.Equal(t, []string{"a.md"}, read, "one read, of the one file the diff changed inside the project")
	assert.Equal(t, check.VerdictPassed, report.Verdict, report.DidNotRun)
}

func TestDiffCheck_Outcomes(t *testing.T) {
	isolateCheckExecution(t)
	app := &App{SourceLang: "en"}
	run := func(t *testing.T, files map[string]string, patch string) (check.Report, error) {
		t.Helper()
		dir := t.TempDir()
		for name, content := range files {
			writeCheckInput(t, dir, name, content)
		}
		t.Chdir(dir)
		cmd := diffCommand(t)
		require.NoError(t, cmd.Flags().Set("diff-file", StdinName))
		cmd.SetIn(bytes.NewBufferString(patch))
		return app.ComputeCheck(cmd, nil)
	}

	t.Run("a change to markup alone touches no block", func(t *testing.T) {
		report, err := run(t, map[string]string{"doc.md": "# Title\n\nPara.\n\n"},
			"--- a/doc.md\n+++ b/doc.md\n@@ -3,0 +4 @@ Para.\n+\n")
		require.NoError(t, err)
		assert.Equal(t, check.ScopeUntouched, scopeEntry(t, report, "doc.md").Status)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
		assert.Equal(t, []string{"the diff touches no content block"}, report.DidNotRun)
	})

	t.Run("content whose position is ambiguous did not run", func(t *testing.T) {
		report, err := run(t, map[string]string{"doc.md": "Text\n\n    indented code\n\nMore text\n", "ok.md": "Fine text.\n"},
			"--- a/doc.md\n+++ b/doc.md\n@@ -5 +5 @@\n-More\n+More text\n--- a/ok.md\n+++ b/ok.md\n@@ -1 +1 @@\n-Fine.\n+Fine text.\n")
		require.NoError(t, err)
		doc := scopeEntry(t, report, "doc.md")
		assert.Equal(t, check.ScopeDidNotRun, doc.Status)
		assert.Contains(t, doc.Reason, "ambiguous")
		assert.Equal(t, check.ScopeChecked, scopeEntry(t, report, "ok.md").Status)
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict, "a changed file left unchecked leaves the check unverified")
	})

	t.Run("a file no format reads", func(t *testing.T) {
		report, err := run(t, map[string]string{"notes.unknownext": "x\n", "ok.md": "Fine text.\n"},
			"--- a/notes.unknownext\n+++ b/notes.unknownext\n@@ -1 +1 @@\n-y\n+x\n--- a/ok.md\n+++ b/ok.md\n@@ -1 +1 @@\n-Fine.\n+Fine text.\n")
		require.NoError(t, err)
		assert.Equal(t, check.ScopeNoReader, scopeEntry(t, report, "notes.unknownext").Status)
		assert.Equal(t, check.VerdictPassed, report.Verdict, report.DidNotRun)
	})

	t.Run("a diff taken from another tree is refused", func(t *testing.T) {
		_, err := run(t, map[string]string{"doc.md": "# Title\n\nPara.\n"},
			"--- a/doc.md\n+++ b/doc.md\n@@ -3 +3 @@\n-Old.\n+New.\n")
		require.ErrorContains(t, err, "does not match the file")
	})
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false"}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func TestDiffCheck_AgainstARevision(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	writeCheckInput(t, dir, "doc.md", "# Title\n\nOne.\n\nTwo.\n")
	git(t, dir, "add", "doc.md")
	git(t, dir, "commit", "-q", "-m", "v1")
	writeCheckInput(t, dir, "doc.md", "# Title\n\nOne.\n\nTwo, changed.\n")
	writeCheckInput(t, dir, "new.md", "A new page.\n")
	t.Chdir(dir)

	cmd := diffCommand(t)
	require.NoError(t, cmd.Flags().Set("diff-against", "HEAD"))
	var out bytes.Buffer
	cmd.SetOut(&out)
	app := &App{SourceLang: "en"}
	require.NoError(t, app.RunCheck(cmd, nil))
	assert.Contains(t, out.String(), "Diff scope (git diff HEAD)")

	report, err := app.ComputeCheck(cmd, nil)
	require.NoError(t, err)
	doc := scopeEntry(t, report, "doc.md")
	assert.Equal(t, check.ScopeChecked, doc.Status)
	require.Len(t, doc.Blocks, 1)
	assert.Equal(t, format.LineRange{First: 5, Last: 5}, doc.Blocks[0].Lines)
	assert.Equal(t, check.ScopeChecked, scopeEntry(t, report, "new.md").Status, "an untracked file is part of the change")

	narrowed, err := app.ComputeCheck(cmd, []string{"new.md"})
	require.NoError(t, err)
	assert.Equal(t, check.ScopeOutOfScope, scopeEntry(t, narrowed, "doc.md").Status)

	bad := diffCommand(t)
	require.NoError(t, bad.Flags().Set("diff-against", "--output=written"))
	_, err = app.ComputeCheck(bad, nil)
	require.ErrorContains(t, err, "not a revision")
	_, statErr := os.Stat(filepath.Join(dir, "written"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestDiffCheck_MCPMatchesTheCLI(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	writeCheckInput(t, dir, "doc.md", "# Title\n\nOne.\n\nTwo, changed.\n")
	patch := "--- a/doc.md\n+++ b/doc.md\n@@ -5 +5 @@\n-Two.\n+Two, changed.\n"
	t.Chdir(dir)
	app := &App{SourceLang: "en"}

	cmd := diffCommand(t)
	require.NoError(t, cmd.Flags().Set("diff-file", StdinName))
	cmd.SetIn(bytes.NewBufferString(patch))
	cli, err := app.ComputeCheck(cmd, nil)
	require.NoError(t, err)

	_, mcp, err := app.checkFileMCP(t.Context(), checkFileInput{Diff: patch})
	require.NoError(t, err)
	require.NotNil(t, mcp.Scope)
	assert.Equal(t, cli.Scope.Files, mcp.Scope.Files)
	assert.Equal(t, cli.Verdict, mcp.Verdict)

	_, _, err = app.checkFileMCP(t.Context(), checkFileInput{})
	require.ErrorContains(t, err, "file is required")
	_, _, err = app.checkFileMCP(t.Context(), checkFileInput{Diff: patch, Target: "x.md"})
	require.Error(t, err)
}
