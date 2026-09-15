package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// A file `defaults.exclude` matches is content the project does not declare. A
// check that names it reads it as undeclared, so the comments its pattern's item
// declares go unread, and holds what it reads to the project's default point
// rather than to that item's point. `kapi context` names no collection for it.
func TestANamedExcludedFileSitsAtTheDefaultPoint(t *testing.T) {
	root := commentPointProject(t, onItem)
	recipe := filepath.Join(root, "kapi.yaml")
	data, err := os.ReadFile(recipe)
	require.NoError(t, err)
	const anchor = "  source_language: en\n"
	require.Contains(t, string(data), anchor)
	data = []byte(strings.Replace(string(data), anchor, anchor+"  exclude:\n    - \"config/app.yaml\"\n", 1))
	require.NoError(t, os.WriteFile(recipe, data, 0o600))
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	defaults, err := proj.ResolveGovernanceFor(project.GovernancePoint{})
	require.NoError(t, err)

	cmd := executionCommand(t)
	cmd.Flags().String(projectFlagName, recipe, "")
	report, err := (&App{SourceLang: "en"}).ComputeCheck(cmd, []string{filepath.Join(root, "config", "app.yaml")})
	require.NoError(t, err)
	assert.Equal(t, 2, report.Target.Blocks, "the two values, and none of the comments")
	assert.Empty(t, governedFindings(report.Findings), "no voice is bound at the default point, and the project's terms retire neither word")

	src, done := (&App{}).ContextSourcesAt(bindingsCmd(t, recipe), ContextPointRequest{Path: filepath.Join(root, "config", "app.yaml")})
	defer done()
	assert.Empty(t, src.Collection)
	if assert.NotNil(t, src.Governance) {
		assert.Equal(t, defaults.Ref().String(), src.Governance.Ref().String())
	}
}
