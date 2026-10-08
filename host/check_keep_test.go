package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/contextop"
)

// A rename held in one part of a project leaves the old name correct
// everywhere else. The rules files and the context answer say so ("Keep as it
// is"), but an agent told by a person to rename the legal terms renames them
// anyway, and the new name is no forbidden term in legal/, so a check that
// only enforced the rules held there passed it. These hold `kapi check` to
// the same line the rules files print: the new name in a file where the old
// one is correct is a finding there, which fails like any term rule.

// keepFindings are the findings about a rename applied where it does not hold.
func keepFindings(report check.Report, file string) []check.Diagnostic {
	var out []check.Diagnostic
	for _, d := range report.Findings {
		if strings.HasSuffix(d.Location.File, file) && strings.Contains(d.Message, "is correct here") {
			out = append(out, d)
		}
	}
	return out
}

func checkFiles(t *testing.T, app *App, root string, files ...string) check.Report {
	t.Helper()
	cmd := executionCommand(t)
	cmd.Flags().String(projectFlagName, recipeOf(root), "")
	abs := make([]string, len(files))
	for i, f := range files {
		abs[i] = filepath.Join(root, filepath.FromSlash(f))
	}
	report, err := app.ComputeCheck(cmd, abs)
	require.NoError(t, err)
	return report
}

func TestCheckFlagsARenameAppliedWhereTheOldNameStays(t *testing.T) {
	app, root := spacesProject(t)
	write := func(rel, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(body), 0o644))
	}
	write("legal/terms.md", "The Space is provided as is. The Space Admin accepts these terms.\n")
	write("api/workspaces.md", "GET /workspaces lists every workspace. Leave space between fields.\n")
	write("help/getting-started.md", "Each board belongs to a Space.\n\nThe Space Admin invites people.\n")

	report := checkFiles(t, app, root, "legal/terms.md", "api/workspaces.md", "help/getting-started.md")

	legal := keepFindings(report, "legal/terms.md")
	require.Len(t, legal, 3, "each use of a renamed name is a finding where the old one is correct (Space, and Space inside Space Admin, and Space Admin)")
	var messages []string
	for _, d := range legal {
		messages = append(messages, d.Message)
		assert.True(t, d.Fails, "a kept rename fails here like any other term rule: %s", d.Message)
		assert.True(t, strings.HasPrefix(d.Rule, "terms."), d.Rule)
	}
	joined := strings.Join(messages, "\n")
	assert.Contains(t, joined, `"Workspace" is correct here; the rename to "Space" holds only in changelog/ and help/`)
	assert.Contains(t, joined, `"Workspace admin" is correct here; the rename to "Space Admin" holds only in changelog/ and help/`)
	assert.Equal(t, check.VerdictFailed, report.Verdict, "the check fails on the over-applied rename")

	assert.Empty(t, keepFindings(report, "api/workspaces.md"), "lower-case space is a word, not the renamed name")
	assert.Empty(t, keepFindings(report, "help/getting-started.md"), "the rename holds in help/")
}

func TestCheckReportsAnAdvisoryRenameWithoutFailing(t *testing.T) {
	app, root := spacesProject(t)
	op, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor: person, Project: recipeOf(root), Term: "Board", InsteadOf: []string{"Project"},
		Evidence: []contextop.Evidence{{Path: "help/getting-started.md"}},
		Text:     "the help pages call projects boards",
	})
	require.NoError(t, err)
	_, err = app.DecideContextReview(t.Context(), ContextReviewRequest{
		Actor: person, Project: recipeOf(root), Keep: []string{op.ID}, WidenTo: "channel", Advisory: new(true),
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "legal", "terms.md"), []byte("The Workspace and each Board are provided as is.\n"), 0o644))

	report := checkFiles(t, app, root, "legal/terms.md")
	findings := keepFindings(report, "legal/terms.md")
	require.Len(t, findings, 1)
	assert.False(t, findings[0].Fails, "an advisory rule reports and never fails")
}
