package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
)

// TestHTMLBlockEntityKeepsItsSpelling is the #3201 reproducer. The HTML
// subfilter decoded a character reference into the html-text block, and the
// skeleton path wrote the decoded character back where the source spelled the
// reference. A paragraph already turns a reference into a placeholder carrying
// its source bytes; the HTML block path does the same now.
func TestHTMLBlockEntityKeepsItsSpelling(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"named reference in a div", "<div>&amp;</div>\n"},
		{"reference after a comment", "<!-- c -->&amp;\n"},
		{"reference inside text", "<div>Fish &amp; chips</div>\n"},
		{"numeric references", "<p>&#38; and &#x26; and &#39;</p>\n"},
		{"several references", "<div>&lt;b&gt; &quot;q&quot;</div>\n"},
		{"multi-line block", "<div>\n  Fish &amp; chips\n  and &copy; 2026\n</div>\n"},
		{"reference in a blockquoted block", "> <div>&amp;</div>\n"},
		{"between paragraphs", "Before.\n\n<div>&amp;</div>\n\nAfter.\n"},
		{"unknown name stays literal", "<div>&bogus; x</div>\n"},
		{"bare ampersand", "<div>a & b</div>\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
		})
	}
}

// The reference is a placeholder whose equivalent is the decoded character, so
// a translator and a length check still see one "&".
func TestHTMLBlockEntityIsAPlaceholder(t *testing.T) {
	t.Parallel()
	blocks := testutil.FilterBlocks(readParts(t, "<div>Fish &amp; chips</div>\n"))
	require.Len(t, blocks, 1)
	require.Equal(t, "html-text", blocks[0].Type)
	runs := blocks[0].SourceRuns()
	require.Len(t, runs, 3)
	assert.Equal(t, "Fish ", runs[0].Text.Text)
	require.NotNil(t, runs[1].Ph)
	assert.Equal(t, "&amp;", runs[1].Ph.Data)
	assert.Equal(t, "&", runs[1].Ph.Equiv)
	assert.Equal(t, "md:entity", runs[1].Ph.SubType)
	assert.Equal(t, " chips", runs[2].Text.Text)
	assert.Equal(t, "Fish &amp; chips", model.RenderRunsWithData(runs))
}
