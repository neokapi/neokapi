package host

import (
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

func stagedCommand(t *testing.T) *EnvCommand {
	t.Helper()
	cmd := diffCommand(t)
	cmd.Flags().Bool("staged", false, "")
	require.NoError(t, cmd.Flags().Set("staged", "true"))
	return cmd
}

// gitDiffCached is the staged diff of the repository in dir, as git writes it.
func gitDiffCached(t *testing.T, dir string) []diffscope.File {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "diff", "--cached", "--no-color", "--no-ext-diff", "--src-prefix=a/", "--dst-prefix=b/", "-M").Output()
	require.NoError(t, err)
	files, err := diffscope.Parse(out)
	require.NoError(t, err)
	return files
}

// doubledWordLines are the lines of each doubled-word finding in a report,
// keyed by file.
func doubledWordLines(t *testing.T, report check.Report) map[string][]format.LineRange {
	t.Helper()
	out := map[string][]format.LineRange{}
	for _, d := range report.Findings {
		if d.Rule != "hygiene.doubled-word" {
			continue
		}
		require.NotNil(t, d.Location.Lines, "%+v", d)
		out[d.Location.File] = append(out[d.Location.File], *d.Location.Lines)
	}
	return out
}

// stagedRepo commits code.go and doc.md and stages an edit to each that adds a
// doubled word. It then edits both again without staging: the working tree
// drops each staged doubled word, adds one of its own elsewhere, and moves the
// staged lines down. An untracked page holds a doubled word too.
func stagedRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	writeCheckInput(t, dir, "code.go", "package code\n\n// Parse reads the input.\nfunc Parse() {}\n\n// Retry tries once more.\nfunc Retry() {}\n")
	writeCheckInput(t, dir, "doc.md", "# Title\n\nFirst paragraph.\n\nSecond paragraph.\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "v1")

	writeCheckInput(t, dir, "code.go", "package code\n\n// Parse reads the the input.\nfunc Parse() {}\n\n// Retry tries once more.\nfunc Retry() {}\n")
	writeCheckInput(t, dir, "doc.md", "# Title\n\nFirst paragraph.\n\nSecond paragraph, the the staged one.\n")
	git(t, dir, "add", "code.go", "doc.md")

	writeCheckInput(t, dir, "code.go", "package code\n\nimport \"fmt\"\n\nvar _ = fmt.Sprint\n\n// Parse reads the input.\nfunc Parse() {}\n\n// Retry tries once once more.\nfunc Retry() {}\n")
	writeCheckInput(t, dir, "doc.md", "# Title\n\nAn inserted paragraph.\n\nFirst first paragraph.\n\nSecond paragraph.\n")
	writeCheckInput(t, dir, "untracked.md", "An untracked the the page.\n")
	return dir
}

// TestDiffCheckStagedReadsCommentsFromTheIndex holds a staged Go file's
// comments to their limits and measures the density of the change, with the
// working tree holding another version of the file or none. The recipe
// declares the file by its path in the index.
func TestDiffCheckStagedReadsCommentsFromTheIndex(t *testing.T) {
	for name, worktree := range map[string]func(t *testing.T, path string){
		"a working tree holding another version": func(t *testing.T, path string) {
			require.NoError(t, os.WriteFile(path, []byte(packageDocGo), 0o600))
		},
		"a working tree without the file": func(t *testing.T, path string) {
			require.NoError(t, os.Remove(path))
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := commentLimitsProject(t, true, denseGo)
			path := filepath.Join(root, "code", "parse.go")
			require.NoError(t, os.Remove(path))
			git(t, root, "init", "-q")
			git(t, root, "add", ".")
			git(t, root, "commit", "-q", "-m", "project")
			require.NoError(t, os.WriteFile(path, []byte(denseGo), 0o600))
			git(t, root, "add", "code/parse.go")
			worktree(t, path)
			t.Chdir(root)

			cmd := stagedCommand(t)
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
