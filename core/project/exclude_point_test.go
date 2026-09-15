package project_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A path `defaults.exclude` matches is content no item claims. ResolveContent
// never expands it, and a lookup that starts from the path agrees: no item or
// collection claims it, and its content and its comments sit at the project's
// default point.
func TestAnExcludedPathIsClaimedByNoItem(t *testing.T) {
	proj, err := project.Load(writeRecipe(t, `version: v1
name: excluded-points
defaults:
  source_language: en
  exclude:
    - "config/generated.yaml"
profiles:
  site:
    channels: [web]
collections:
  - name: config
    channel: site/web
    source_only: true
    content:
      - path: "config/*.yaml"
        comments: true
`))
	require.NoError(t, err)
	defaults, err := proj.ResolveGovernanceFor(project.GovernancePoint{})
	require.NoError(t, err)

	for path, tc := range map[string]struct {
		claimed bool
		want    string
	}{
		"config/app.yaml":       {claimed: true, want: "site/web"},
		"config/generated.yaml": {claimed: false, want: defaults.Ref().String()},
	} {
		_, _, claimed := proj.ItemForPath(path)
		assert.Equal(t, tc.claimed, claimed, path)
		_, _, contentClaimed := proj.ContentItemForPath(path, false)
		assert.Equal(t, tc.claimed, contentClaimed, path)
		for _, comments := range []bool{false, true} {
			rc, err := proj.ResolveGovernanceFor(project.GovernancePoint{Path: path, Comments: comments})
			require.NoError(t, err)
			assert.Equal(t, tc.want, rc.Ref().String(), "%s, comments %v", path, comments)
		}
	}
}
