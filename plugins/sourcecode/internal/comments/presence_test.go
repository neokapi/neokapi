package comments_test

import (
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/comment"
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

func TestProseP0_python(t *testing.T) { assertPresent(t, "python") }

func TestProseP0_bash(t *testing.T) { assertPresent(t, "bash") }

func TestProseP0_css(t *testing.T) { assertPresent(t, "css") }

func TestProseP0_rust(t *testing.T) { assertPresent(t, "rust") }

func TestProseP0_java(t *testing.T) { assertPresent(t, "java") }

func TestProseP0_csharp(t *testing.T) { assertPresent(t, "csharp") }

func TestProseP0_c(t *testing.T) { assertPresent(t, "c") }

func TestProseP0_cpp(t *testing.T) { assertPresent(t, "cpp") }

func TestProseP0_ruby(t *testing.T) { assertPresent(t, "ruby") }

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
			Markers:     markersOf(l.Markers),
			Canary:      manifest.CommentCanary{Name: l.Canary.Name, Source: string(l.Canary.Source), Block: l.Canary.Block},
		})
	}
	got := slices.Clone(m.Capabilities.Comments)
	for i := range got {
		// A rewrite declaration is the host's to act on: the plugin never
		// writes, so no field of it has a counterpart in this package.
		// TestManifestRewriteCanariesAreLocated holds its canary to the code.
		got[i].Rewrite = nil
	}
	slices.SortFunc(got, func(a, b manifest.CommentLanguage) int {
		if a.Language < b.Language {
			return -1
		}
		return 1
	})
	assert.Equal(t, declared, got)
}

// A language the manifest declares writable carries a write canary this package
// reads as the host needs it read: the comment the canary rewrites, and its
// delimited comment, are located under the ids the declaration names.
func TestManifestRewriteCanariesAreLocated(t *testing.T) {
	writable := 0
	for _, l := range readManifest(t).Capabilities.Comments {
		if l.Rewrite == nil {
			continue
		}
		writable++
		c := l.Rewrite.Canary
		f, err := comments.Locate(l.Language, c.Name, []byte(c.Source))
		require.NoError(t, err, l.Language)
		var ids []string
		for _, b := range f.Blocks() {
			ids = append(ids, b.ID)
		}
		assert.Contains(t, ids, c.Block, "%s: the comment the rewrite canary rewrites", l.Language)
		if c.Delimited != "" {
			assert.Contains(t, ids, c.Delimited, "%s: the rewrite canary's delimited comment", l.Language)
		}
	}
	require.Positive(t, writable, "the manifest declares no comment language writable")
}

func markersOf(m comment.Markers) manifest.CommentMarkers {
	out := manifest.CommentMarkers{Line: m.Line, Splice: m.Splice}
	for _, b := range m.Block {
		out.Block = append(out.Block, manifest.CommentBlockMarker{Open: b.Open, Close: b.Close, Nested: b.Nested})
	}
	return out
}

func readManifest(t *testing.T) *manifest.Manifest {
	t.Helper()
	data, err := os.ReadFile("../../manifest.json")
	require.NoError(t, err)
	m, err := manifest.Parse(data)
	require.NoError(t, err)
	return m
}
