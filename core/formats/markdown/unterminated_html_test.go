package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUnterminatedHTMLTagKeepsItsBytes is the #2497 reproducer. An HTML tag the
// input ends inside tokenizes to nothing, and the reader advanced the skeleton
// cursor past the whole HTML block, so those bytes reached neither a block nor
// the skeleton and the write path dropped them: "text\n\n<p" came back as
// "text\n\n".
func TestUnterminatedHTMLTagKeepsItsBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"tag alone", "<p"},
		{"tag after a paragraph", "text\n\n<p"},
		{"tag with an attribute", "a <b class\n"},
		{"tag with an unclosed quote", "<span title=\"never closed\n"},
		{"comment never closed", "<!-- open\n"},
		{"tag then more text", "<p\n\nAfter the truncated tag.\n"},
		{"closing tag truncated", "<div>a</div\n"},
		{"truncated inside a paragraph's html", "before\n\n<div>\n<p\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
		})
	}
}

// TestUnterminatedHTMLFixtureRoundTrips covers the same ground on a file.
func TestUnterminatedHTMLFixtureRoundTrips(t *testing.T) {
	t.Parallel()
	input := mustReadFixture(t, "testdata/unterminated-html.md")
	require.Equal(t, input, roundtripWithSkeleton(t, input))
}
