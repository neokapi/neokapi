//go:build !js

package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/format"
)

func TestDiffCheckRange(t *testing.T) {
	isolateCheckExecution(t)
	app := &App{SourceLang: "en"}
	// The findings of a..b, which b's lines hold wherever the check runs.
	wantB := map[string][]format.LineRange{
		"code.go":  {{First: 7, Last: 7}},
		"added.md": {{First: 1, Last: 1}},
		"new.md":   {{First: 7, Last: 7}},
	}

	t.Run("a range from a checkout of another commit reads b", func(t *testing.T) {
		dir := rangeRepo(t)
		t.Chdir(dir)
		report, err := app.ComputeCheck(rangeCommand(t, "a..b"), nil)
		require.NoError(t, err)

		require.NotNil(t, report.Scope)
		assert.Equal(t, "git diff a..b", report.Scope.Diff)
		assert.Len(t, report.Scope.Files, 4, "the untracked page and the uncommitted edit are no part of a..b: %+v", report.Scope.Files)
		for path, lines := range map[string]format.LineRange{"code.go": {First: 7, Last: 7}, "added.md": {First: 1, Last: 1}, "new.md": {First: 7, Last: 7}} {
			entry := scopeEntry(t, report, path)
			assert.Equal(t, check.ScopeChecked, entry.Status, "%s: %s", path, entry.Reason)
			require.Len(t, entry.Blocks, 1, path)
			assert.Equal(t, lines, entry.Blocks[0].Lines, "%s: b's lines", path)
		}
		assert.Equal(t, check.ScopeDeleted, scopeEntry(t, report, "gone.md").Status, "a file absent at b")
		assert.Equal(t, 3, report.Target.Blocks)
		assert.Equal(t, wantB, doubledWordLines(t, report))
		assert.NotEqual(t, check.VerdictDidNotRun, report.Verdict, report.DidNotRun)
		require.NoError(t, scopeMatchesDiff(report, gitDiffCommits(t, dir, "a", "b")))
	})

	t.Run("c...b is the change b made since it left c, and c..b also undoes c", func(t *testing.T) {
		dir := rangeRepo(t)
		t.Chdir(dir)
		mergeBase, err := app.ComputeCheck(rangeCommand(t, "c...b"), nil)
		require.NoError(t, err)
		assert.Equal(t, "git diff c...b", mergeBase.Scope.Diff)
		assert.Len(t, mergeBase.Scope.Files, 4)
		assert.Equal(t, wantB, doubledWordLines(t, mergeBase))
		assert.Equal(t, 3, mergeBase.Target.Blocks)

		twoDot, err := app.ComputeCheck(rangeCommand(t, "c..b"), nil)
		require.NoError(t, err)
		assert.Len(t, twoDot.Scope.Files, 5)
		doc := scopeEntry(t, twoDot, "doc.md")
		assert.Equal(t, check.ScopeChecked, doc.Status, "b holds doc.md as a left it, so c..b changes it back")
		assert.Equal(t, wantB, doubledWordLines(t, twoDot), "b's doc.md holds no doubled word")
	})

	t.Run("an empty side names HEAD", func(t *testing.T) {
		dir := rangeRepo(t)
		t.Chdir(dir)
		report, err := app.ComputeCheck(rangeCommand(t, "a.."), nil)
		require.NoError(t, err)
		assert.Equal(t, map[string][]format.LineRange{"doc.md": {{First: 3, Last: 3}}}, doubledWordLines(t, report),
			"a..HEAD is c's commit, and not the working tree's edit to code.go")
		assert.Len(t, report.Scope.Files, 1)
	})

	t.Run("must fail: a file read from the working tree is refused", func(t *testing.T) {
		dir := rangeRepo(t)
		t.Chdir(dir)
		saved := readScopedObject
		readScopedObject = func(ctx context.Context, objects *gitObjects, path string) ([]byte, error) {
			if path == "code.go" {
				return os.ReadFile(filepath.Join(objects.root, path))
			}
			return saved(ctx, objects, path)
		}
		t.Cleanup(func() { readScopedObject = saved })

		_, err := app.ComputeCheck(rangeCommand(t, "a..b"), nil)
		require.ErrorContains(t, err, "does not match the file")
		assert.ErrorContains(t, err, "at b")
	})

	t.Run("a binary change at b did not run", func(t *testing.T) {
		dir := t.TempDir()
		git(t, dir, "init", "-q")
		writeCheckInput(t, dir, "page.md", "Text.\n")
		git(t, dir, "add", ".")
		git(t, dir, "commit", "-q", "-m", "a")
		git(t, dir, "tag", "a")
		writeCheckInput(t, dir, "page.md", "Text\x00 with a zero byte.\n")
		git(t, dir, "commit", "-q", "-am", "b")
		git(t, dir, "tag", "b")
		git(t, dir, "checkout", "-q", "a")
		t.Chdir(dir)

		report, err := app.ComputeCheck(rangeCommand(t, "a..b"), nil)
		require.NoError(t, err)
		entry := scopeEntry(t, report, "page.md")
		assert.Equal(t, check.ScopeDidNotRun, entry.Status)
		assert.Contains(t, entry.Reason, "binary")
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
	})

	t.Run("what is not a range of commits is refused", func(t *testing.T) {
		dir := rangeRepo(t)
		t.Chdir(dir)
		for spec, want := range map[string]string{
			"b":               "is not a range",
			"--output=x..b":   "is not a revision",
			"a..--output=x":   "is not a revision",
			"a..b..c":         "is not a revision",
			"a..no-such-rev":  "names no commit",
			"no-such-rev...b": "names no commit",
		} {
			_, err := app.ComputeCheck(rangeCommand(t, spec), nil)
			require.ErrorContains(t, err, want, spec)
		}
		_, statErr := os.Stat(filepath.Join(dir, "x"))
		assert.ErrorIs(t, statErr, os.ErrNotExist)
	})

	t.Run("outside a git work tree", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_, err := app.ComputeCheck(rangeCommand(t, "a..b"), nil)
		require.ErrorContains(t, err, "--diff-range needs a git work tree")
	})

	t.Run("one diff source at a time", func(t *testing.T) {
		dir := rangeRepo(t)
		t.Chdir(dir)
		cmd := rangeCommand(t, "a..b")
		require.NoError(t, cmd.Flags().Set("diff-against", "HEAD"))
		_, err := app.ComputeCheck(cmd, nil)
		require.ErrorContains(t, err, "each name a diff")
	})

	t.Run("check_file checks a range as the CLI does", func(t *testing.T) {
		dir := rangeRepo(t)
		t.Chdir(dir)
		cli, err := app.ComputeCheck(rangeCommand(t, "c...b"), nil)
		require.NoError(t, err)
		_, mcp, err := app.checkFileMCP(t.Context(), checkFileInput{DiffRange: "c...b"})
		require.NoError(t, err)
		require.NotNil(t, mcp.Scope)
		assert.Equal(t, cli.Scope, mcp.Scope)
		assert.Equal(t, cli.Findings, mcp.Findings)
		assert.Equal(t, 3, mcp.Target.Blocks)

		_, _, err = app.checkFileMCP(t.Context(), checkFileInput{DiffRange: "a..b", Staged: true})
		require.ErrorContains(t, err, "each name a diff")
	})
}
