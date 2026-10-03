//go:build !js

package host

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/format"
)

func TestDiffCheckStaged(t *testing.T) {
	isolateCheckExecution(t)
	app := &App{SourceLang: "en"}

	t.Run("the index is checked, not the working tree", func(t *testing.T) {
		dir := stagedRepo(t)
		t.Chdir(dir)
		cmd := stagedCommand(t)
		report, err := app.ComputeCheck(cmd, nil)
		require.NoError(t, err)

		require.NotNil(t, report.Scope)
		assert.Equal(t, "staged", report.Scope.Diff)
		assert.Len(t, report.Scope.Files, 2, "the untracked page is not a staged change: %+v", report.Scope.Files)
		for path, lines := range map[string]format.LineRange{"code.go": {First: 3, Last: 3}, "doc.md": {First: 5, Last: 5}} {
			entry := scopeEntry(t, report, path)
			assert.Equal(t, check.ScopeChecked, entry.Status, entry.Reason)
			require.Len(t, entry.Blocks, 1, path)
			assert.Equal(t, lines, entry.Blocks[0].Lines, "%s: the lines the index holds, not the working tree's", path)
		}
		assert.Equal(t, 2, report.Target.Blocks)
		assert.Equal(t, map[string][]format.LineRange{
			"code.go": {{First: 3, Last: 3}},
			"doc.md":  {{First: 5, Last: 5}},
		}, doubledWordLines(t, report), "the staged doubled words and none of the working tree's")
		assert.NotEqual(t, check.VerdictDidNotRun, report.Verdict, report.DidNotRun)
		require.NoError(t, scopeMatchesDiff(report, gitDiffCached(t, dir)))

		var out bytes.Buffer
		cmd.SetOut(&out)
		_ = app.RunCheck(cmd, nil)
		assert.Contains(t, out.String(), "Diff scope (staged)")
	})

	t.Run("must fail: content read from the working tree is refused", func(t *testing.T) {
		dir := stagedRepo(t)
		t.Chdir(dir)
		saved := readScopedObject
		readScopedObject = func(_ context.Context, objects *gitObjects, path string) ([]byte, error) {
			return os.ReadFile(filepath.Join(objects.root, filepath.FromSlash(path)))
		}
		t.Cleanup(func() { readScopedObject = saved })

		_, err := app.ComputeCheck(stagedCommand(t), nil)
		require.ErrorContains(t, err, "does not match the file")
		assert.ErrorContains(t, err, "in the index")
	})

	t.Run("added, deleted and renamed files, and a staged file the working tree lacks", func(t *testing.T) {
		dir := t.TempDir()
		git(t, dir, "init", "-q")
		guide := "# Guide\n\nOne paragraph of the guide.\n\nTwo paragraphs of the guide.\n\nThree paragraphs of the guide.\n\nFour paragraphs of the guide.\n"
		writeCheckInput(t, dir, "old.md", guide)
		writeCheckInput(t, dir, "gone.md", "Gone.\n")
		writeCheckInput(t, dir, "keep.md", "Kept.\n")
		git(t, dir, "add", ".")
		git(t, dir, "commit", "-q", "-m", "v1")

		git(t, dir, "mv", "old.md", "new.md")
		writeCheckInput(t, dir, "new.md", strings.Replace(guide, "Three paragraphs", "Three the the paragraphs", 1))
		git(t, dir, "rm", "-q", "gone.md")
		writeCheckInput(t, dir, "added.md", "An added the the page.\n")
		git(t, dir, "add", "new.md", "added.md")
		require.NoError(t, os.Remove(filepath.Join(dir, "added.md")))
		writeCheckInput(t, dir, "new.md", "Rewritten in the working tree.\n")
		t.Chdir(dir)

		report, err := app.ComputeCheck(stagedCommand(t), nil)
		require.NoError(t, err)
		require.NotNil(t, report.Scope)
		assert.Len(t, report.Scope.Files, 3, "%+v", report.Scope.Files)
		assert.Equal(t, check.ScopeDeleted, scopeEntry(t, report, "gone.md").Status)
		added := scopeEntry(t, report, "added.md")
		assert.Equal(t, check.ScopeChecked, added.Status, added.Reason)
		renamed := scopeEntry(t, report, "new.md")
		assert.Equal(t, check.ScopeChecked, renamed.Status, renamed.Reason)
		require.Len(t, renamed.Blocks, 1)
		assert.Equal(t, format.LineRange{First: 7, Last: 7}, renamed.Blocks[0].Lines)
		assert.Equal(t, 2, report.Target.Blocks)
		assert.Equal(t, map[string][]format.LineRange{
			"added.md": {{First: 1, Last: 1}},
			"new.md":   {{First: 7, Last: 7}},
		}, doubledWordLines(t, report))
		require.NoError(t, scopeMatchesDiff(report, gitDiffCached(t, dir)))
	})

	t.Run("a binary change did not run", func(t *testing.T) {
		dir := t.TempDir()
		git(t, dir, "init", "-q")
		writeCheckInput(t, dir, "page.md", "Text.\n")
		git(t, dir, "add", ".")
		git(t, dir, "commit", "-q", "-m", "v1")
		writeCheckInput(t, dir, "page.md", "Text\x00 with a zero byte.\n")
		git(t, dir, "add", "page.md")
		t.Chdir(dir)

		report, err := app.ComputeCheck(stagedCommand(t), nil)
		require.NoError(t, err)
		entry := scopeEntry(t, report, "page.md")
		assert.Equal(t, check.ScopeDidNotRun, entry.Status)
		assert.Contains(t, entry.Reason, "binary")
		assert.Equal(t, check.VerdictDidNotRun, report.Verdict)
	})

	t.Run("the first commit of a repository", func(t *testing.T) {
		dir := t.TempDir()
		git(t, dir, "init", "-q")
		writeCheckInput(t, dir, "page.md", "A first the the page.\n")
		git(t, dir, "add", "page.md")
		t.Chdir(dir)

		report, err := app.ComputeCheck(stagedCommand(t), nil)
		require.NoError(t, err)
		assert.Equal(t, check.ScopeChecked, scopeEntry(t, report, "page.md").Status)
		assert.Equal(t, 1, report.Target.Blocks)
		assert.Equal(t, map[string][]format.LineRange{"page.md": {{First: 1, Last: 1}}}, doubledWordLines(t, report))
	})

	t.Run("an index holding a conflict is refused", func(t *testing.T) {
		dir := t.TempDir()
		git(t, dir, "init", "-q", "-b", "main")
		writeCheckInput(t, dir, "page.md", "Base.\n")
		git(t, dir, "add", ".")
		git(t, dir, "commit", "-q", "-m", "base")
		git(t, dir, "checkout", "-q", "-b", "side")
		writeCheckInput(t, dir, "page.md", "Side.\n")
		git(t, dir, "commit", "-q", "-am", "side")
		git(t, dir, "checkout", "-q", "main")
		writeCheckInput(t, dir, "page.md", "Main.\n")
		git(t, dir, "commit", "-q", "-am", "main")
		merge := exec.Command("git", "-C", dir, "-c", "user.email=t@example.com", "-c", "user.name=t", "merge", "side")
		require.Error(t, merge.Run(), "the merge conflicts")
		t.Chdir(dir)

		_, err := app.ComputeCheck(stagedCommand(t), nil)
		require.ErrorContains(t, err, "unmerged")
		assert.ErrorContains(t, err, "page.md")
	})

	t.Run("outside a git work tree", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_, err := app.ComputeCheck(stagedCommand(t), nil)
		require.ErrorContains(t, err, "--staged needs a git work tree")
	})

	t.Run("one diff source at a time", func(t *testing.T) {
		dir := stagedRepo(t)
		t.Chdir(dir)
		cmd := stagedCommand(t)
		require.NoError(t, cmd.Flags().Set("diff-against", "HEAD"))
		_, err := app.ComputeCheck(cmd, nil)
		require.ErrorContains(t, err, "each name a diff")
	})

	t.Run("check_file checks the staged changes as the CLI does", func(t *testing.T) {
		dir := stagedRepo(t)
		t.Chdir(dir)
		cli, err := app.ComputeCheck(stagedCommand(t), nil)
		require.NoError(t, err)
		_, mcp, err := app.checkFileMCP(t.Context(), checkFileInput{Staged: true})
		require.NoError(t, err)
		require.NotNil(t, mcp.Scope)
		assert.Equal(t, cli.Scope, mcp.Scope)
		assert.Equal(t, cli.Findings, mcp.Findings)
		assert.Equal(t, 2, mcp.Target.Blocks)

		_, _, err = app.checkFileMCP(t.Context(), checkFileInput{Staged: true, DiffAgainst: "HEAD"})
		require.ErrorContains(t, err, "each name a diff")
	})
}
