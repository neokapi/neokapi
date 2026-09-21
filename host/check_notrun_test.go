package host

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/diffscope"
	"github.com/neokapi/neokapi/core/format"
)

// TestCheckNotRun_ABrokenCheckerReadsDifferentlyFromNothingToCheck runs the two
// did_not_run cases an agent loop must never confuse, a checker that failed its
// canary and a diff with nothing in scope, and asserts that the text, the JSON
// and the error each tell them apart even though they share exit code 4.
func TestCheckNotRun_ABrokenCheckerReadsDifferentlyFromNothingToCheck(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	src := writeCheckInput(t, dir, "app.json", `{"title":"Hello world"}`)
	writeCheckInput(t, dir, "doc.md", "# Title\n\nPara.\n\n")
	patch := writeCheckInput(t, dir, "blank.diff", "--- a/doc.md\n+++ b/doc.md\n@@ -3,0 +4 @@ Para.\n+\n")
	t.Chdir(dir)
	app := &App{SourceLang: "en"}

	run := func(cmd *EnvCommand, args []string, jsonOut bool) (string, error) {
		t.Helper()
		if jsonOut {
			cmd.Flags().String("output-format", "json", "")
		}
		var out bytes.Buffer
		cmd.SetOut(&out)
		err := app.RunCheck(cmd, args)
		require.Equal(t, ExitNotRun, ExitCode(cmd, err))
		return out.String(), err
	}

	real := hygieneTool
	hygieneTool = inertHygiene
	t.Cleanup(func() { hygieneTool = real })
	brokenText, brokenErr := run(executionCommand(t), []string{src}, false)
	brokenJSON, _ := run(executionCommand(t), []string{src}, true)
	hygieneTool = real

	emptyCmd := diffCommand(t)
	require.NoError(t, emptyCmd.Flags().Set("diff-file", patch))
	emptyText, emptyErr := run(emptyCmd, nil, false)
	emptyJSONCmd := diffCommand(t)
	require.NoError(t, emptyJSONCmd.Flags().Set("diff-file", patch))
	emptyJSON, _ := run(emptyJSONCmd, nil, true)

	assert.Contains(t, brokenText, "Did not run: a checker failed its canary, so this run's result cannot be trusted. (checker_invalid)")
	assert.Contains(t, emptyText, "Did not run: there was nothing in scope to check. (nothing_to_check)")
	assert.NotContains(t, brokenText, "nothing in scope")
	assert.NotContains(t, emptyText, "canary")

	assert.Contains(t, brokenErr.Error(), "checker_invalid")
	assert.Contains(t, emptyErr.Error(), "nothing_to_check")

	var broken, empty check.Report
	require.NoError(t, json.Unmarshal([]byte(brokenJSON), &broken))
	require.NoError(t, json.Unmarshal([]byte(emptyJSON), &empty))
	assert.Equal(t, check.VerdictDidNotRun, broken.Verdict)
	assert.Equal(t, check.VerdictDidNotRun, empty.Verdict)
	assert.Equal(t, check.CauseCheckerInvalid, broken.DidNotRunCause)
	assert.Equal(t, check.CauseNothingToCheck, empty.DidNotRunCause)
}

// TestDiffCheck_RealCommitDeletionBesideABlock checks the diff of a real commit
// (fac49dfd3, which removed the last item of a list in
// docs/internals/video-revision-runbook.md). The removed lines follow the
// previous item's last line, so the deletion borders that item without
// changing it.
func TestDiffCheck_RealCommitDeletionBesideABlock(t *testing.T) {
	isolateCheckExecution(t)
	fixture, err := filepath.Abs(filepath.Join("testdata", "diffscope", "list-item-removal"))
	require.NoError(t, err)
	post, err := os.ReadFile(filepath.Join(fixture, "video-revision-runbook.md"))
	require.NoError(t, err)
	pre, err := os.ReadFile(filepath.Join(fixture, "video-revision-runbook.pre.md"))
	require.NoError(t, err)
	patch, err := os.ReadFile(filepath.Join(fixture, "commit.diff"))
	require.NoError(t, err)

	files, err := diffscope.Parse(patch)
	require.NoError(t, err)
	require.Len(t, files, 1)
	f := files[0]
	require.Equal(t, []diffscope.Change{{Lines: format.LineRange{First: 120, Last: 121}, Deletion: true, After: 120}}, f.Changes)

	app := &App{SourceLang: "en"}
	app.InitRegistries()
	postRead, err := app.readWithExtents(t.Context(), "runbook.md", post, "markdown", nil, "en")
	require.NoError(t, err)
	require.NoError(t, postRead.unlocated)

	// The placement on its own takes the item the removed lines followed.
	touched := diffscope.Touched(postRead.extents, f.Changes)
	require.Len(t, touched, 1)
	item := touched[0]
	assert.Equal(t, format.LineRange{First: 118, Last: 120}, item.Lines)
	assert.Equal(t, touched, diffscope.Bordered(touched, f.Changes))

	// The pre-image rebuilt from the diff is git's own, and it holds that item
	// unchanged on the same lines, so the item leaves the scope.
	rebuilt, err := f.PreImage(post)
	require.NoError(t, err)
	require.Equal(t, string(pre), string(rebuilt))
	preRead, err := app.readWithExtents(t.Context(), "runbook.md", rebuilt, "markdown", nil, "en")
	require.NoError(t, err)
	require.NoError(t, preRead.unlocated)
	assert.Empty(t, diffscope.Settle(f, post, touched, rebuilt, preRead.extents))

	// The must-fail half: had the commit also reworded that item, the pre-image
	// would not match it and the item would stay in scope.
	reworded := bytes.Replace(rebuilt, []byte("a path the app never used."), []byte("a path nobody used."), 1)
	require.NotEqual(t, rebuilt, reworded)
	rewordedRead, err := app.readWithExtents(t.Context(), "runbook.md", reworded, "markdown", nil, "en")
	require.NoError(t, err)
	assert.Len(t, diffscope.Settle(f, post, touched, reworded, rewordedRead.extents), 1)

	// The run itself.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs", "internals"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "internals", "video-revision-runbook.md"), post, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "commit.diff"), patch, 0o644))
	t.Chdir(dir)
	cmd := diffCommand(t)
	require.NoError(t, cmd.Flags().Set("diff-file", "commit.diff"))
	report, err := app.ComputeCheck(cmd, nil)
	require.NoError(t, err)
	entry := scopeEntry(t, report, filepath.FromSlash("docs/internals/video-revision-runbook.md"))
	assert.Equal(t, check.ScopeUntouched, entry.Status, "the only block the deletion borders is unchanged")
	assert.Empty(t, entry.Blocks)
	assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
	assert.Equal(t, check.CauseNothingToCheck, report.DidNotRunCause)
}

// TestCheckNotRun_TheMCPToolsTellTheCausesApart runs the same two cases through
// the agent-facing tools: check_text with a broken checker, and check_file with
// a diff that touches no block.
func TestCheckNotRun_TheMCPToolsTellTheCausesApart(t *testing.T) {
	isolateCheckExecution(t)
	dir := t.TempDir()
	writeCheckInput(t, dir, "doc.md", "# Title\n\nPara.\n\n")
	t.Chdir(dir)
	app := &App{SourceLang: "en"}

	real := hygieneTool
	hygieneTool = inertHygiene
	t.Cleanup(func() { hygieneTool = real })
	_, broken, err := app.checkTextMCP(t.Context(), checkTextInput{Text: "Hello world"})
	require.NoError(t, err)
	hygieneTool = real

	_, empty, err := app.checkFileMCP(t.Context(), checkFileInput{Diff: "--- a/doc.md\n+++ b/doc.md\n@@ -3,0 +4 @@ Para.\n+\n"})
	require.NoError(t, err)

	assert.Equal(t, check.VerdictDidNotRun, broken.Verdict)
	assert.Equal(t, check.VerdictDidNotRun, empty.Verdict)
	assert.Equal(t, check.CauseCheckerInvalid, broken.DidNotRunCause)
	assert.Equal(t, check.CauseNothingToCheck, empty.DidNotRunCause)
	require.NotEmpty(t, broken.DidNotRun)
	require.NotEmpty(t, empty.DidNotRun)
	assert.Contains(t, broken.DidNotRun[0], "canary")
	assert.NotContains(t, empty.DidNotRun[0], "canary")
}
