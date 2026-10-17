package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/internal/testutil"
)

// TestDocusaurusAdmonitionHeaderKeepsItsWhitespace is the #3204 reproducer.
// The reader spelled the skeleton of a `:::keyword Title` header as the marker,
// the keyword and one space whatever the source had between them, and let the
// line's trailing whitespace go, so a tab or a second space cost the header a
// byte on an untranslated round trip.
func TestDocusaurusAdmonitionHeaderKeepsItsWhitespace(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"tab before the keyword", ":::\tnote Title\n"},
		{"two spaces before the title", ":::note  Title\n"},
		{"tab before the title", ":::note\tTitle\n"},
		{"trailing spaces", ":::note Title  \n"},
		{"trailing tab", ":::tip Title\t\n"},
		{"crlf line ending", ":::note Title\r\n"},
		{"keyword with trailing spaces", ":::note  \n"},
		{"bare closer with trailing space", ":::note\nBody.\n::: \n"},
		{"header inside a document", "# Heading\n\n:::note  Title\n\nBody text.\n\n:::\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
		})
	}
}

// The title is the block whatever the whitespace around it spells, so a
// translation lands between the source's own bytes.
func TestDocusaurusAdmonitionTitleIsTheBlock(t *testing.T) {
	t.Parallel()
	for _, input := range []string{":::note\tTitle\n", ":::note  Title  \n"} {
		blocks := testutil.FilterBlocks(readParts(t, input))
		require.Len(t, blocks, 1)
		require.Equal(t, "Title", blocks[0].SourceText())
		require.Equal(t, "docusaurus-admonition-title", blocks[0].Type)
	}
}
