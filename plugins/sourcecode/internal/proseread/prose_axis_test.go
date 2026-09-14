package proseread_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/plugins/sourcecode/internal/proseread"
)

// The format reads a file's comments through the comment provider the host's
// comment layer reads them with, so the plugin has one comment path: a comment
// arrives as a comment block named for what it sits on, grouped with the lines
// beside it, and without its `#` marker.
func TestCommentsArriveThroughTheCommentProvider(t *testing.T) {
	src, err := os.ReadFile("testdata/kapi-desktop.rb")
	require.NoError(t, err)
	parts, err := proseread.ReadParts(src, model.LocaleEnglish, "kapi-desktop.rb", proseread.Options{Comments: true})
	require.NoError(t, err)

	var names []string
	for _, p := range parts {
		b, ok := p.Resource.(*model.Block)
		if !ok || p.Type != model.PartBlock || !comment.IsBlock(b) {
			continue
		}
		names = append(names, b.Name)
		assert.NotContains(t, b.SourceText(), "#", "a comment block holds the comment's text, not its marker")
	}
	assert.Equal(t, []string{"comment", "cask/comment"}, names)

	t.Run("must fail: a file whose comments cannot be located is an error, never a partial read", func(t *testing.T) {
		_, err := proseread.ReadParts([]byte("# Parses.\ndef parse(\n"), model.LocaleEnglish, "broken.rb", proseread.Options{Comments: true})
		require.ErrorIs(t, err, comment.ErrUnlocated)
	})
}
