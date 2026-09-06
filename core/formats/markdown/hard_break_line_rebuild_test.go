package markdown_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHardBreakOnItsOwnLineRebuildsAsOneBlock is the #2448 reproducer, found by
// FuzzRoundTripMarkdown. "0\n\\\n0" is one paragraph whose middle line is only
// a backslash, which CommonMark 6.7 reads as a hard break. The break's spelling
// rides a placeholder and the newline it contributes is the text after it, so a
// line with no other content left two newlines in the block text and the
// rebuild wrote a blank line: the paragraph re-read as two. The rebuild path
// now spells the break with a backslash when its line is otherwise empty.
func TestHardBreakOnItsOwnLineRebuildsAsOneBlock(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"0\n\\\n0",
		"0\n\\\n0\n",
		"a\n\\\nb\n\\\nc",
		"> 0\n> \\\n> 0",
		"- 0\n  \\\n  0",
		"0\n\\\n\\\n0",
		"a\n\\\nb  \nc",
	}
	for i, in := range inputs {
		t.Run(fmt.Sprintf("%d %q", i, in), func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			before := readBlocks(t, in)
			require.Len(t, before, 1, "source should read as one block")

			out1, keys1, ok := tripMarkdown(ctx, []byte(in))
			require.True(t, ok, "first trip declined %q", in)

			after := readBlocks(t, string(out1))
			assert.Len(t, after, 1, "rebuilt %q re-read as %d blocks:\n%q", in, len(after), out1)

			out2, keys2, ok2 := tripMarkdown(ctx, out1)
			require.True(t, ok2, "re-reading %q failed", out1)
			assert.Equal(t, string(out1), string(out2), "the rebuild is not idempotent")
			assert.Len(t, keys2, len(keys1), "block count changed on the second pass")

			_, keys3, ok3 := tripMarkdown(ctx, out2)
			require.True(t, ok3, "re-reading %q failed", out2)
			assert.Equal(t, keys2, keys3, "content identity drifted after the first pass")
		})
	}
}

// TestHardBreakOpeningABlockSpellsNothing is the other control. A break at the
// very start of a block has no line before it to keep from going empty, and the
// leading whitespace is trimmed on the way out, so a backslash there would only
// become text: ">\\\n#\\\n00" gained one and its "#" moved onto a line of
// its own, where it re-read as a heading.
func TestHardBreakOpeningABlockSpellsNothing(t *testing.T) {
	t.Parallel()
	for _, in := range []string{">\\\n#\\\n00", "\\\n#\\\n00", "\\\n0"} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			out1, keys1, ok := tripMarkdown(ctx, []byte(in))
			require.True(t, ok)
			assert.NotContains(t, string(out1), "\\\n", "no backslash break at the block's edge")

			_, keys2, ok2 := tripMarkdown(ctx, out1)
			require.True(t, ok2)
			assert.Len(t, keys2, len(keys1), "block count changed on the second pass")
		})
	}
}

// TestHardBreakWithContentOnItsLineIsUnchanged is the control. A hard break
// after text on the same line stays the accepted lossiness it has always been:
// the rebuild path writes a soft break, and nothing gains a backslash.
func TestHardBreakWithContentOnItsLineIsUnchanged(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"a\\\nb\n", "a\nb\n"},
		{"a  \nb\n", "a\nb\n"},
		{"> a  \n> b\n", "> a\n> b\n"},
		{"- a  \n  b\n", "- a\nb\n"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			out, _, ok := tripMarkdown(context.Background(), []byte(tc.in))
			require.True(t, ok)
			assert.Equal(t, tc.want, string(out))
		})
	}
}

// A nested blockquote loses its outer marker on this path whatever the break
// looks like, which is #2464 and not this fix; the shapes here stay one level
// deep so the assertion is about the break.

// TestHardBreakOnItsOwnLineKeepsItsBytes pins the shape of the output: the
// rebuild of the reproducer is the source, so nothing about the paragraph
// changed on the way through.
func TestHardBreakOnItsOwnLineKeepsItsBytes(t *testing.T) {
	t.Parallel()
	out, _, ok := tripMarkdown(context.Background(), []byte("0\n\\\n0"))
	require.True(t, ok)
	assert.Equal(t, "0\n\\\n0\n", string(out))

	out, _, ok = tripMarkdown(context.Background(), []byte("> 0\n> \\\n> 0"))
	require.True(t, ok)
	assert.Equal(t, "> 0\n> \\\n> 0\n", string(out))
}
