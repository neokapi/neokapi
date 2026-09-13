package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeRecipe(t *testing.T, body string) string {
	t.Helper()
	recipe := filepath.Join(t.TempDir(), project.RecipeFileName)
	require.NoError(t, os.WriteFile(recipe, []byte(body), 0o644))
	return recipe
}

// directiveRecipe is a recipe whose defaults and one content item declare
// comment directives.
func directiveRecipe(defaults, item string) string {
	return `version: v1
name: directives
defaults:
  source_language: en
` + defaults + `collections:
  - name: code
    source_only: true
    content:
      - path: "src/*.go"
        comments: ` + item + `
`
}

func TestCommentDirectivesLoadInEveryForm(t *testing.T) {
	for name, recipe := range map[string]string{
		"comments: true":                 directiveRecipe("", "true"),
		"an empty mapping":               directiveRecipe("", "{}"),
		"item directives":                directiveRecipe("", `{directives: ["okapi-skip:"]}`),
		"defaults directives":            directiveRecipe("  comments:\n    directives: [\"okapi-skip:\"]\n", "true"),
		"defaults and item together":     directiveRecipe("  comments:\n    directives: [\"okapi-skip:\"]\n", `{directives: ["okapi-unmapped:"]}`),
		"a marker that prefixes another": directiveRecipe("", `{directives: ["okapi-", "okapi-skip:"]}`),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := project.Load(writeRecipe(t, recipe))
			require.NoError(t, err)
		})
	}
}

func TestCommentDirectivesRejectedAtLoad(t *testing.T) {
	for name, tc := range map[string]struct {
		recipe string
		want   []string
	}{
		"an empty default marker": {
			recipe: directiveRecipe("  comments:\n    directives: [\"\"]\n", "true"),
			want:   []string{"defaults.comments.directives[0]", "is empty"},
		},
		"a default marker declared twice": {
			recipe: directiveRecipe("  comments:\n    directives: [\"okapi-skip:\", \"okapi-skip:\"]\n", "true"),
			want:   []string{"defaults.comments.directives[1]", `"okapi-skip:"`, "declared twice"},
		},
		"an empty item marker": {
			recipe: directiveRecipe("", `{directives: ["okapi-skip:", ""]}`),
			want:   []string{"collections[0].content[0].comments.directives[1]", "is empty"},
		},
		"an item marker declared twice": {
			recipe: directiveRecipe("", `{directives: ["okapi-skip:", "okapi-skip:"]}`),
			want:   []string{"collections[0].content[0].comments.directives[1]", "declared twice"},
		},
		"an item marker the defaults already declare": {
			recipe: directiveRecipe("  comments:\n    directives: [\"okapi-skip:\"]\n", `{directives: ["okapi-skip:"]}`),
			want:   []string{"collections[0].content[0].comments.directives[0]", "defaults.comments.directives"},
		},
		"a marker starting with whitespace": {
			recipe: directiveRecipe("", `{directives: [" okapi-skip:"]}`),
			want:   []string{"collections[0].content[0].comments.directives[0]", "starts with whitespace"},
		},
		"a marker holding a line break": {
			recipe: directiveRecipe("", `{directives: ["okapi\nskip:"]}`),
			want:   []string{"collections[0].content[0].comments.directives[0]", "line break"},
		},
		"a comments value that is neither a boolean nor a mapping": {
			recipe: directiveRecipe("", `"yes"`),
			want:   []string{"comments"},
		},
		"an unknown key under an item's comments": {
			recipe: directiveRecipe("", `{directive: ["okapi-skip:"]}`),
			want:   []string{`unknown key "directive"`},
		},
		"an unknown key under defaults.comments": {
			recipe: directiveRecipe("  comments:\n    directive: [\"okapi-skip:\"]\n", "true"),
			want:   []string{`unknown key "directive"`},
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

func TestCommentDirectivesResolvePerItem(t *testing.T) {
	proj, err := project.Load(writeRecipe(t, `version: v1
name: directives
defaults:
  source_language: en
  comments:
    directives: ["okapi-skip:"]
collections:
  - name: code
    source_only: true
    content:
      - path: "src/*.go"
        comments: true
      - path: "config/*.yaml"
        comments:
          directives: ["deploy-lock:"]
      - path: "docs/*.md"
        comments: false
      - path: "data/*.json"
`))
	require.NoError(t, err)
	items := proj.Collections[0].Content
	assert.Equal(t, project.ContentComments{Declared: true}, items[0].Comments)
	assert.Equal(t, project.ContentComments{Declared: true, Directives: []string{"deploy-lock:"}}, items[1].Comments)
	assert.False(t, items[2].Comments.Declared)
	assert.False(t, items[3].Comments.Declared)
	assert.Equal(t, []string{"okapi-skip:"}, items[0].CommentDirectives(proj.Defaults))
	assert.Equal(t, []string{"okapi-skip:", "deploy-lock:"}, items[1].CommentDirectives(proj.Defaults))
}

// Saving a loaded recipe writes each form back as it was written.
func TestCommentDirectivesSurviveSave(t *testing.T) {
	recipe := writeRecipe(t, `version: v1
name: directives
defaults:
  source_language: en
  comments:
    directives: ["okapi-skip:"]
collections:
  - name: code
    source_only: true
    content:
      - path: "src/*.go"
        comments: true
      - path: "config/*.yaml"
        comments:
          directives: ["deploy-lock:"]
`)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Name = "renamed"
	require.NoError(t, project.Save(recipe, proj))
	data, err := os.ReadFile(recipe)
	require.NoError(t, err)
	assert.Contains(t, string(data), "comments: true")
	assert.Contains(t, string(data), "deploy-lock:")
	assert.Contains(t, string(data), "okapi-skip:")
	_, err = project.Load(recipe)
	require.NoError(t, err)
}
