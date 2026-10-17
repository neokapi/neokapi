package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStrikethroughAroundAnAutolinkKeepsBothSpellings is the #3202 reproducer.
// An autolink records no source segment, so a strikethrough whose only child
// is one could not be located from its content and fell back to the two-tilde
// spelling; the autolink, located from that spelling, then started one byte
// into its own angle bracket and came back bare. "~<https://x>~" was written
// as "~~https://x~~".
func TestStrikethroughAroundAnAutolinkKeepsBothSpellings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"single tilde", "~<https://x>~\n"},
		{"double tilde", "~~<https://x>~~\n"},
		{"in a sentence", "a ~<https://x>~ b\n"},
		{"inside emphasis", "*~<https://x>~*\n"},
		{"autolink then text", "~<https://x> gone~\n"},
		{"text then autolink", "~gone <https://x>~\n"},
		{"in a blockquote", "> ~<https://x>~\n"},
		{"in a list item", "- ~<https://x>~\n"},
		{"after a link", "[a](b)~<https://x>~\n"},
		{"email autolink", "~<me@x.org>~\n"},
		{"two pairs", "~<https://x>~ and ~~<https://y>~~\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
		})
	}
}
