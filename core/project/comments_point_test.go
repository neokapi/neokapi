package project_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// channelRecipe is a recipe with two profiles whose defaults and one content
// item place comments.
func channelRecipe(defaults, item string) string {
	return `version: v1
name: comment-points
defaults:
  source_language: en
` + defaults + `profiles:
  site:
    channels: [web]
  source:
    channels: [comments]
collections:
  - name: config
    channel: site/web
    source_only: true
    content:
      - path: "config/*.yaml"
        comments: ` + item + `
`
}

func TestCommentChannelsLoad(t *testing.T) {
	for name, recipe := range map[string]string{
		"on an item":              channelRecipe("", `{channel: source/comments}`),
		"under the defaults":      channelRecipe("  comments:\n    channel: source/comments\n", "true"),
		"beside item directives":  channelRecipe("", `{channel: source/comments, directives: ["okapi-skip:"]}`),
		"on the item's own point": channelRecipe("", `{channel: site/web}`),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := project.Load(writeRecipe(t, recipe))
			require.NoError(t, err)
		})
	}
}

func TestCommentChannelsRejectedAtLoad(t *testing.T) {
	for name, tc := range map[string]struct {
		recipe string
		want   []string
	}{
		"an item channel that names no profile": {
			recipe: channelRecipe("", `{channel: comments}`),
			want:   []string{"content[0]", "comments.channel", "source/comments"},
		},
		"an item channel the profile does not declare": {
			recipe: channelRecipe("", `{channel: source/notes}`),
			want:   []string{"content[0]", "comments.channel", `"source/notes"`},
		},
		"a default channel that names an undeclared profile": {
			recipe: channelRecipe("  comments:\n    channel: nowhere/comments\n", "true"),
			want:   []string{"defaults.comments.channel", `"nowhere"`},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := project.Load(writeRecipe(t, tc.recipe))
			require.Error(t, err)
			for _, want := range tc.want {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

// A file's comments resolve at their own point, and the content its reader
// extracts stays at the item's.
func TestCommentsResolveAtTheirOwnPoint(t *testing.T) {
	for name, tc := range map[string]struct {
		defaults, item, want string
	}{
		"an item's comments channel":                  {item: `{channel: source/comments}`, want: "source/comments"},
		"the defaults' comments channel":              {defaults: "  comments:\n    channel: source/comments\n", item: "true", want: "source/comments"},
		"an item's comments channel over the default": {defaults: "  comments:\n    channel: site/web\n", item: `{channel: source/comments}`, want: "source/comments"},
		"no comments channel":                         {item: "true", want: "site/web"},
	} {
		t.Run(name, func(t *testing.T) {
			proj, err := project.Load(writeRecipe(t, channelRecipe(tc.defaults, tc.item)))
			require.NoError(t, err)
			comments, err := proj.ResolveGovernanceFor(project.GovernancePoint{Path: "config/app.yaml", Comments: true})
			require.NoError(t, err)
			assert.Equal(t, tc.want, comments.Ref().String())
			values, err := proj.ResolveGovernanceFor(project.GovernancePoint{Path: "config/app.yaml"})
			require.NoError(t, err)
			assert.Equal(t, "site/web", values.Ref().String(), "the values stay at the item's point")
		})
	}

	t.Run("a file no item claims takes the defaults' comments channel", func(t *testing.T) {
		proj, err := project.Load(writeRecipe(t, channelRecipe("  comments:\n    channel: source/comments\n", "true")))
		require.NoError(t, err)
		comments, err := proj.ResolveGovernanceFor(project.GovernancePoint{Path: "tools/gen.go", Comments: true})
		require.NoError(t, err)
		assert.Equal(t, "source/comments", comments.Ref().String())
		content, err := proj.ResolveGovernanceFor(project.GovernancePoint{Path: "tools/gen.go"})
		require.NoError(t, err)
		assert.Empty(t, content.Ref().String())
	})
}
