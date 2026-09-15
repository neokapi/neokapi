package project_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// A recipe declares a comment point when it places comments anywhere other than
// their file's own point: under defaults.comments.channel, under an item's
// comments.channel, or on an item declared for its comments alone, whose file's
// content resolves past it.
func TestDeclaresCommentPoint(t *testing.T) {
	const (
		head     = "version: v1\nname: comment-point\ndefaults:\n  source_language: en\n"
		profiles = "profiles:\n  source:\n    channels: [comments]\n"
	)
	for name, tc := range map[string]struct {
		recipe string
		want   bool
	}{
		"defaults.comments.channel": {
			recipe: head + "  comments:\n    channel: source/comments\n" + profiles,
			want:   true,
		},
		"an item's comments.channel": {
			recipe: head + profiles + "collections:\n  - name: code\n    content:\n" +
				"      - path: \"code/*.go\"\n        comments:\n          channel: source/comments\n",
			want: true,
		},
		"an item declared for its comments alone": {
			recipe: head + "collections:\n  - name: workflows\n    content:\n" +
				"      - path: \".github/workflows/*.yaml\"\n        comments:\n          only: true\n",
			want: true,
		},
		"comments: true places no point of their own": {
			recipe: head + "collections:\n  - name: code\n    content:\n      - path: \"code/*.go\"\n        comments: true\n",
			want:   false,
		},
		"directives place no point": {
			recipe: head + "  comments:\n    directives: [\"okapi:\"]\n" +
				"collections:\n  - name: code\n    content:\n      - path: \"code/*.go\"\n        comments:\n          directives: [\"lint:\"]\n",
			want: false,
		},
		"no comments declared": {
			recipe: head + "collections:\n  - path: \"src/*.json\"\n",
			want:   false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			proj, err := project.Load(writeRecipe(t, tc.recipe))
			require.NoError(t, err)
			assert.Equal(t, tc.want, proj.DeclaresCommentPoint())
		})
	}
}
