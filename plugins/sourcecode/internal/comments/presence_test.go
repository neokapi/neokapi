package comments_test

import (
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/plugins/sourcecode/internal/comments"
)

// The presence tests on the Prose maturity axis (docs/internals/format-maturity.md
// §2.8) for the languages this plugin reads the comments of: the build holds a
// grammar for the language, and the plugin's manifest declares it, which is how
// kapi learns to send the language's files here. They claim no rung.
func TestProseP0_typescript(t *testing.T) { assertPresent(t, "typescript") }

func TestProseP0_tsx(t *testing.T) { assertPresent(t, "tsx") }

func TestProseP0_javascript(t *testing.T) { assertPresent(t, "javascript") }

func assertPresent(t *testing.T, language string) {
	t.Helper()
	_, ok := comments.Lookup(language)
	require.True(t, ok, "this build reads no %s comments", language)
	declared := false
	for _, c := range readManifest(t).Capabilities.Comments {
		declared = declared || c.Language == language
	}
	assert.True(t, declared, "manifest.json does not declare %s comments, so kapi never sends its files here", language)

	t.Run("must fail: an unknown language is absent", func(t *testing.T) {
		_, ok := comments.Lookup(language + "x")
		assert.False(t, ok)
	})
}

// The manifest is what the host reads the comment languages from, and the code
// is what answers for them. Every language, extension and canary must agree.
func TestManifestDeclaresEveryCommentLanguage(t *testing.T) {
	m := readManifest(t)
	var declared []manifest.CommentLanguage
	for _, l := range comments.Languages() {
		declared = append(declared, manifest.CommentLanguage{
			Language:    l.Name,
			DisplayName: l.DisplayName,
			Extensions:  l.Extensions,
			Canary:      manifest.CommentCanary{Name: l.Canary.Name, Source: string(l.Canary.Source), Block: l.Canary.Block},
		})
	}
	got := slices.Clone(m.Capabilities.Comments)
	slices.SortFunc(got, func(a, b manifest.CommentLanguage) int {
		if a.Language < b.Language {
			return -1
		}
		return 1
	})
	assert.Equal(t, declared, got)
}

func readManifest(t *testing.T) *manifest.Manifest {
	t.Helper()
	data, err := os.ReadFile("../../manifest.json")
	require.NoError(t, err)
	m, err := manifest.Parse(data)
	require.NoError(t, err)
	return m
}
