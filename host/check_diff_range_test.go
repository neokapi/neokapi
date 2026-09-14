package host

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/diffscope"
	"github.com/neokapi/neokapi/core/format"
)

func rangeCommand(t *testing.T, spec string) *EnvCommand {
	t.Helper()
	cmd := diffCommand(t)
	cmd.Flags().String("diff-range", "", "")
	require.NoError(t, cmd.Flags().Set("diff-range", spec))
	return cmd
}

// gitDiffCommits is the diff between two commits of the repository in dir, as
// git writes it.
func gitDiffCommits(t *testing.T, dir, from, to string) []diffscope.File {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "diff", "--no-color", "--no-ext-diff", "--src-prefix=a/", "--dst-prefix=b/", "-M", from, to).Output()
	require.NoError(t, err)
	files, err := diffscope.Parse(out)
	require.NoError(t, err)
	return files
}

// rangeGuide is a page long enough for git to follow it through a rename with
// one line edited.
const rangeGuide = "# Guide\n\nOne paragraph of the guide.\n\nTwo paragraphs of the guide.\n\nThree paragraphs of the guide.\n\nFour paragraphs of the guide.\n"

// rangeRepo builds a history for range checks, with tags naming its commits:
//
//   - a holds code.go, doc.md, old.md and gone.md.
//   - b, on a branch from a, inserts two code lines above Parse and adds a
//     doubled word to its comment, adds added.md holding a doubled word,
//     deletes gone.md, and renames old.md to new.md with a doubled word.
//   - c, on main from a, adds a doubled word to doc.md.
//
// The working tree is left at c, with an uncommitted edit to code.go and an
// untracked page holding a doubled word.
func rangeRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	writeCheckInput(t, dir, "code.go", "package code\n\n// Parse reads the input.\nfunc Parse() {}\n")
	writeCheckInput(t, dir, "doc.md", "# Title\n\nFirst paragraph.\n")
	writeCheckInput(t, dir, "old.md", rangeGuide)
	writeCheckInput(t, dir, "gone.md", "Gone.\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "a")
	git(t, dir, "tag", "a")

	git(t, dir, "checkout", "-q", "-b", "feature")
	writeCheckInput(t, dir, "code.go", "package code\n\nimport \"fmt\"\n\nvar _ = fmt.Sprint\n\n// Parse reads the the input.\nfunc Parse() {}\n")
	writeCheckInput(t, dir, "added.md", "An added the the page.\n")
	git(t, dir, "rm", "-q", "gone.md")
	git(t, dir, "mv", "old.md", "new.md")
	writeCheckInput(t, dir, "new.md", strings.Replace(rangeGuide, "Three paragraphs", "Three the the paragraphs", 1))
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "b")
	git(t, dir, "tag", "b")

	git(t, dir, "checkout", "-q", "main")
	writeCheckInput(t, dir, "doc.md", "# Title\n\nFirst the the paragraph.\n")
	git(t, dir, "commit", "-q", "-am", "c")
	git(t, dir, "tag", "c")

	writeCheckInput(t, dir, "code.go", "package code\n\n// Parse reads the input, edited.\nfunc Parse() {}\n")
	writeCheckInput(t, dir, "untracked.md", "An untracked the the page.\n")
	return dir
}

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

// TestDiffCheckRangeReadsCommentsFromB holds a Go file that b adds to its
// comment limits and measures the density of the change, from a checkout of a
// where the working tree holds no such file or an untracked one of its own. The
// recipe on disk declares the file by its path at b.
func TestDiffCheckRangeReadsCommentsFromB(t *testing.T) {
	for name, worktree := range map[string]func(t *testing.T, path string){
		"a working tree without the file": func(*testing.T, string) {},
		"an untracked file at the same path": func(t *testing.T, path string) {
			// Checking out a removes the directory git no longer tracks a file in.
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			require.NoError(t, os.WriteFile(path, []byte(packageDocGo), 0o600))
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := commentLimitsProject(t, true, denseGo)
			path := filepath.Join(root, "code", "parse.go")
			require.NoError(t, os.Remove(path))
			git(t, root, "init", "-q")
			git(t, root, "add", ".")
			git(t, root, "commit", "-q", "-m", "project")
			git(t, root, "tag", "a")
			require.NoError(t, os.WriteFile(path, []byte(denseGo), 0o600))
			git(t, root, "add", "code/parse.go")
			git(t, root, "commit", "-q", "-m", "parse")
			git(t, root, "tag", "b")
			git(t, root, "checkout", "-q", "a")
			_, err := os.Stat(path)
			require.ErrorIs(t, err, os.ErrNotExist)
			worktree(t, path)
			t.Chdir(root)

			cmd := rangeCommand(t, "a..b")
			cmd.Flags().String(projectFlagName, filepath.Join(root, "kapi.yaml"), "")
			report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, nil)
			require.NoError(t, err)

			entry := scopeEntry(t, report, filepath.FromSlash("code/parse.go"))
			assert.Equal(t, check.ScopeChecked, entry.Status, entry.Reason)
			assert.Positive(t, report.Target.Blocks)
			found := densityFindings(report)
			require.Len(t, found, 1, "%+v", report.Findings)
			assert.Equal(t, "Change adds 9 comment lines and 4 code lines, more than 1 comment lines for each code line", found[0].Message)
			require.NotNil(t, found[0].Location.Lines)
			assert.Equal(t, format.LineRange{First: 3, Last: 11}, *found[0].Location.Lines)
			run := analyzerRun(t, report, commentDensityAnalyzer)
			require.NotNil(t, run.Canary)
			assert.Equal(t, check.CanaryCaught, run.Canary.Status)
		})
	}
}
