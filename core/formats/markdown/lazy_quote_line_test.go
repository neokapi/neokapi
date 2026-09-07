package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLazyQuoteContinuationKeepsOneBlock is the #2485 reproducer. A blockquote
// whose paragraph runs on into a lazy continuation line (CommonMark 5.1) is one
// block whose text carries the unmarked line. The rebuild path restored the
// marker on the first line and left the rest as the block spells them, so a
// bare "#" on line two read back as a heading and the one block became two.
func TestLazyQuoteContinuationKeepsOneBlock(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		written string
	}{
		{"lazy heading line", ">0\n#\\\n0", ">0\n\\#\n0\n"},
		{"lazy table delimiter row", "><a>|a|a\n-|-", ">|a|a\n\\-|-\n"},
		{"lazy setext bar", ">0\n0\n=", ">0\n0\n\\=\n"},
		{"marked line opening a list", "0\n<a>>+ 0", ">0\n>\\+ 0\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := roundtrip(t, tc.input)
			assert.Equal(t, tc.written, out)
			require.Len(t, readBlocks(t, out), 1, "the quote did not stay one block")
			assert.Equal(t, out, roundtrip(t, out), "the rebuild is not idempotent")
		})
	}
}

// TestMarkedQuoteContinuationIsLeftAlone pins the exclusion the escape carves
// out of: a continuation line carrying the ">" the rebuild restores keeps it,
// and escaping those would break the quote into loose paragraphs.
func TestMarkedQuoteContinuationIsLeftAlone(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "> a\n> b\n", roundtrip(t, "> a\n> b"))
	assert.Equal(t, "> a\nb\n> c\n", roundtrip(t, "> a\nb\n> c"))
	assert.Equal(t, ">> a\n>> b\n", roundtrip(t, ">> a\n>> b"))
}
