package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/project"
)

// ignoredPointProject is commentPointProject with each item placing its comments
// at source/comments, a .kapiignore that ignores config/app.yaml, and a sibling
// config/other.yaml that the rules leave alone.
func ignoredPointProject(t *testing.T) string {
	t.Helper()
	root := commentPointProject(t, onItem)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".kapiignore"), []byte("config/app.yaml\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config", "other.yaml"), []byte(pointYAML), 0o600))
	return root
}

// A file the project's ignore rules match is content the project does not
// declare, as a file under `defaults.exclude` is. A check that names it reads it
// as undeclared, at the project's default point, and `kapi context` names no
// collection for it. A sibling the rules leave alone keeps its item's point.
func TestANamedIgnoredFileSitsAtTheDefaultPoint(t *testing.T) {
	root := ignoredPointProject(t)
	recipe := filepath.Join(root, "kapi.yaml")
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	defaults, err := proj.ResolveGovernanceFor(project.GovernancePoint{})
	require.NoError(t, err)

	named := func(t *testing.T, file string) check.Report {
		t.Helper()
		cmd := executionCommand(t)
		cmd.Flags().String(projectFlagName, recipe, "")
		report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{filepath.Join(root, "config", file)})
		require.NoError(t, err)
		return report
	}

	t.Run("a check of the ignored file", func(t *testing.T) {
		report := named(t, "app.yaml")
		assert.Equal(t, 2, report.Target.Blocks, "the two values, and none of the comments")
		assert.Empty(t, governedFindings(report.Findings), "no voice is bound at the default point, and the project's terms retire neither word")
	})

	t.Run("a check of its sibling", func(t *testing.T) {
		report := named(t, "other.yaml")
		assert.Equal(t, 4, report.Target.Blocks, "two values and two comments")
		var want []governedFinding
		for _, f := range inFile("app.yaml", commentsApart) {
			f.file = "other.yaml"
			want = append(want, f)
		}
		assert.ElementsMatch(t, want, governedFindings(report.Findings))
		assertPoints(t, report.Findings, true)
	})

	t.Run("kapi context", func(t *testing.T) {
		for file, want := range map[string]struct{ collection, point string }{
			"app.yaml":   {"", defaults.Ref().String()},
			"other.yaml": {"config", "site/web"},
		} {
			src, done := (&App{}).ContextSourcesAt(bindingsCmd(t, recipe), ContextPointRequest{Path: filepath.Join(root, "config", file)})
			assert.Equal(t, want.collection, src.Collection, file)
			if assert.NotNil(t, src.Governance, file) {
				assert.Equal(t, want.point, src.Governance.Ref().String(), file)
			}
			done()
		}
	})
}
