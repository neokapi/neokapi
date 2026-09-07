package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRawHTMLBlockIsNeverRemarkedAsAQuote is the #2505 reproducer. An
// html-text block's text is markup the reader captured verbatim, so a ">" on
// one of its lines is content. The rebuild path recovered a blockquote marker
// from any continuation line carrying one, moved it onto the block's own first
// line, and the pass after that split the block in two.
func TestRawHTMLBlockIsNeverRemarkedAsAQuote(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"quote marker on the last line", "<p>.\n- <\n>"},
		{"quote marker mid block", "<div>a\n> b\nc"},
		{"quote marker on every line", "<div>a\n> b\n> c"},
		{"quote marker left by dropped inline html", "0\n<a>>\n%"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := roundtrip(t, tc.input)
			assert.NotEqual(t, byte('>'), out[0], "the block was re-marked as a quote")
			require.Len(t, readBlocks(t, out), 1, "the rebuilt block did not stay one block")
			assert.Equal(t, out, roundtrip(t, out), "the rebuild is not idempotent")
		})
	}
}
