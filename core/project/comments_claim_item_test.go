package project_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// Each layer of a file is claimed by the first item in recipe order that claims
// it. The values go to the first item that claims more than the comments. The
// comments go to the first item declared for them alone, or to the item that
// claims the values when it declares the comments and no comments-only item
// comes before it. An item listed after the one that claims the values claims
// nothing, whatever it declares.
func TestResolvedFileNamesTheItemThatClaimsItsComments(t *testing.T) {
	const (
		only = `  - name: comments-only
    channel: source/comments
    content:
      - path: "config/*.yaml"
        comments:
          only: true
`
		valuesWithComments = `  - name: values-with-comments
    channel: site/web
    content:
      - path: "config/*.yaml"
        comments: true
`
		values = `  - name: values
    channel: site/web
    content:
      - path: "config/*.yaml"
`
	)
	for name, tc := range map[string]struct {
		collections, values, comments string
		// point is where the comments resolve.
		point string
	}{
		"a comments-only item before a value item": {only + values, "values", "comments-only", "source/comments"},
		"a comments-only item after a value item":  {values + only, "values", "comments-only", "source/comments"},
		"a value item that declares the comments before a comments-only item": {
			valuesWithComments + only, "values-with-comments", "values-with-comments", "site/web",
		},
		"a value item that declares the comments after a comments-only item": {
			only + valuesWithComments, "values-with-comments", "comments-only", "source/comments",
		},
		"a value item after the item that claims the values": {values + valuesWithComments, "values", "", "site/web"},
		"no item declares the comments":                      {values, "values", "", "site/web"},
	} {
		t.Run(name, func(t *testing.T) {
			recipe := writeClaimOrderProject(t, `version: v1
name: claim-item
defaults:
  source_language: en
profiles:
  site:
    channels: [web]
  source:
    channels: [comments]
collections:
`+tc.collections)
			proj, err := project.Load(recipe)
			require.NoError(t, err)
			files, err := project.NewProjectContext(proj, recipe).ResolveContent(contentRegistry(t))
			require.NoError(t, err)
			require.Len(t, files, 1)
			rf := files[0]
			assert.Equal(t, "config/app.yaml", filepath.ToSlash(rf.Relative))
			assert.Equal(t, tc.values, rf.Collection)
			assert.False(t, rf.CommentsOnly())

			item, i, ok := proj.CommentItemForPath("config/app.yaml", false)
			if tc.comments == "" {
				assert.False(t, ok)
				assert.Nil(t, rf.CommentItem)
			} else if assert.True(t, ok) && assert.NotNil(t, rf.CommentItem) {
				assert.Equal(t, tc.comments, proj.Collections[i].Name)
				assert.Equal(t, item, *rf.CommentItem)
			}

			rc, err := proj.ResolveGovernanceFor(project.GovernancePoint{Path: "config/app.yaml", Comments: true})
			require.NoError(t, err)
			assert.Equal(t, tc.point, rc.Ref().String())
		})
	}
}
