package project_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// onlyPointRecipe places the comments of config/*.yaml and code/*.go at
// source/comments, with an item for each that claims nothing but comments
// unless a reader parses the file. later is appended as further collections.
func onlyPointRecipe(later string) string {
	return `version: v1
name: comments-only-points
defaults:
  source_language: en
profiles:
  site:
    channels: [web]
  source:
    channels: [comments]
collections:
  - name: source-comments
    channel: source/comments
    source_only: true
    content:
      - path: "config/*.yaml"
        comments:
          only: true
      - path: "code/*.go"
        comments: true
` + later
}

// laterClaims claims the same files for their content, at site/web.
const laterClaims = `  - name: site
    channel: site/web
    source_only: true
    content:
      - path: "config/*.yaml"
      - path: "code/*.go"
`

// An item that claims only a file's comments governs only its comment layer. A
// point for the file's own content resolves past it, to the next item that
// claims the file or to the project's default point, and the comments stay at
// the item's point.
func TestContentResolvesPastAnItemThatClaimsOnlyComments(t *testing.T) {
	for name, tc := range map[string]struct {
		later, path string
		noReader    bool
		// want is the content's point; empty means the project's default point.
		want       string
		collection string
		// claimed is CollectionForPath, which knows no reader: the collection of
		// the item that claims the file's values, or its comments when no item
		// claims the values.
		claimed string
		// claimsOnly is ClaimsOnlyComments: an item claims the file's comments
		// and none claims its values.
		claimsOnly bool
	}{
		"an only item and no other claim": {
			path: "config/app.yaml", want: "", collection: "", claimed: "source-comments", claimsOnly: true,
		},
		"an only item before an item that claims the file": {
			later: laterClaims, path: "config/app.yaml", want: "site/web", collection: "site", claimed: "site", claimsOnly: false,
		},
		"comments on a file no reader parses": {
			path: "code/parse.go", noReader: true, want: "", collection: "", claimed: "source-comments", claimsOnly: true,
		},
		"comments on a file no reader parses, before an item that claims it": {
			later: laterClaims, path: "code/parse.go", noReader: true, want: "site/web", collection: "site", claimed: "source-comments", claimsOnly: false,
		},
		"comments on a file a reader parses stay a claim on its content": {
			path: "code/parse.go", want: "source/comments", collection: "source-comments", claimed: "source-comments", claimsOnly: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			proj, err := project.Load(writeRecipe(t, onlyPointRecipe(tc.later)))
			require.NoError(t, err)
			want := tc.want
			if want == "" {
				defaults, derr := proj.ResolveGovernanceFor(project.GovernancePoint{})
				require.NoError(t, derr)
				want = defaults.Ref().String()
			}

			content, err := proj.ResolveGovernanceFor(project.GovernancePoint{Path: tc.path, NoReader: tc.noReader})
			require.NoError(t, err)
			assert.Equal(t, want, content.Ref().String(), "the file's content resolves past a claim on its comments alone")
			assert.Equal(t, tc.collection, proj.ContentCollectionForPath(tc.path, tc.noReader))

			comments, err := proj.ResolveGovernanceFor(project.GovernancePoint{Path: tc.path, Comments: true, NoReader: tc.noReader})
			require.NoError(t, err)
			assert.Equal(t, "source/comments", comments.Ref().String(), "the comments stay at the item's point")
			assert.Equal(t, tc.claimed, proj.CollectionForPath(tc.path), "the file is named by the item that claims its values")
			assert.Equal(t, tc.claimsOnly, proj.ClaimsOnlyComments(tc.path, tc.noReader))
		})
	}
}
