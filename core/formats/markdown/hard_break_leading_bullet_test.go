package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHardBreaksOpeningABlockBeforeABulletLine is the #3199 reproducer. The
// paragraph opens with two backslash hard breaks on lines of their own, then
// a line that is only "* " and a line "0". A break that opens a block spells
// nothing on the rebuild path (#2448), but the second break followed a
// newline the first had written, so it was spelled with a backslash on a line
// of its own. The leading newline was trimmed and the backslash line stayed:
// the rebuilt paragraph still opened with a hard break, which the pass after
// dropped, and "* " moved onto the first line, where it is a bullet. The sink
// now spells nothing for a break while the output holds no content, so the
// first pass already writes the paragraph the way every later pass does, with
// the bullet escaped on its first line.
func TestHardBreaksOpeningABlockBeforeABulletLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"two breaks then a bullet line", "\\\n\\\n* \n0"},
		// A "- " line after the breaks is a setext underline, so a dash is
		// not a shape here.
		{"two breaks then a plus line", "\\\n\\\n+ \n0"},
		{"two breaks then an ordered marker line", "\\\n\\\n1. \n0"},
		{"three breaks then a bullet line", "\\\n\\\n\\\n* \n0"},
		{"two breaks then text", "\\\n\\\n0"},
		{"two breaks then a bullet line with a trailing newline", "\\\n\\\n* \n0\n"},
		// In a list item the text after a break carries the item's indent,
		// which is whitespace too.
		{"two breaks then a bullet line in a list item", "- \\\n  \\\n  * \n  0"},
		{"one break then a bullet line in a list item", "- \\\n  * \n  0"},
		// The controls: one break, which #2448 already spells as nothing, and
		// a break after content, which it spells with a backslash.
		{"one break then a bullet line", "\\\n* \n0"},
		{"break after content then a bullet line", "0\n\\\n* \n0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			require.Len(t, readBlocks(t, tc.input), 1, "source should read as one block")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Len(t, readBlocks(t, out), 1, "the rebuilt paragraph re-read as more than one block: %q", out)
		})
	}
}

// TestHardBreaksOpeningABlockSpelling pins the shape: the leading breaks spell
// nothing and the bullet is escaped on the first line.
func TestHardBreaksOpeningABlockSpelling(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"\\\n\\\n* \n0", "\\* \n0\n"},
		{"\\\n\\\n\\\n+ \n0", "\\+ \n0\n"},
		{"\\\n\\\n0", "0\n"},
		{"- \\\n  \\\n  * \n  0", "- \\* \n  0\n"},
		{"0\n\\\n* \n0", "0\n\\\n* \n0\n"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, roundtrip(t, tc.in))
		})
	}
}
