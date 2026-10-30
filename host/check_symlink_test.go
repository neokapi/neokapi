package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
)

// TestCheckFormats_SymlinkedWorkingDirectory: a file named relative to a
// working directory reached through a symbolic link to the project is read
// under the format the recipe declares, however the recipe was named. Read as
// plain text, the heading would keep its '#'.
func TestCheckFormats_SymlinkedWorkingDirectory(t *testing.T) {
	isolateCheckExecution(t)
	real, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	parent, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	link := filepath.Join(parent, "link")
	require.NoError(t, os.Symlink(real, link))
	writeCheckInput(t, real, "kapi.yaml", `version: v1
name: symlinked
defaults:
  source_language: en
collections:
  - name: docs
    content:
      - path: "docs/*.txt"
        format:
          name: markdown
`)
	require.NoError(t, os.MkdirAll(filepath.Join(real, "docs"), 0o755))
	writeCheckInput(t, filepath.Join(real, "docs"), "a.txt", "# Title\n\nSome text.\n")

	for _, tt := range []struct {
		name, cwd, recipe string
	}{
		{name: "as written", cwd: real, recipe: real},
		{name: "working directory through the link", cwd: link, recipe: real},
		{name: "recipe through the link", cwd: real, recipe: link},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(tt.cwd)
			cmd := executionCommand(t)
			cmd.Flags().String(projectFlagName, filepath.Join(tt.recipe, "kapi.yaml"), "")
			cmd.Flags().StringSlice("forbid", []string{"#"}, "")

			formats, err := (&App{SourceLang: "en"}).newCheckFormats(cmd)
			require.NoError(t, err)
			name, _ := formats.forFile(&App{}, filepath.Join("docs", "a.txt"))
			assert.Equal(t, "markdown", name)

			report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{filepath.Join("docs", "a.txt")})
			require.NoError(t, err)
			assert.Equal(t, check.VerdictPassed, report.Verdict, "the heading is read as markdown: %+v", report.Findings)
		})
	}
}
