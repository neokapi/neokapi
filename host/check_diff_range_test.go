package host

import (
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
