package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHeadingOfWhitespaceEmitsNoBlock covers a heading whose text is
// whitespace: a form feed or a vertical tab, which the parser keeps as content
// and Go reads as space. The reader skips a paragraph of such text and now
// skips a heading of it the same way. Kept, a setext heading over such lines
// inside a blockquote carried the quote markers as its text, and the ATX line
// the rebuild folds it onto read back as no block at all.
func TestHeadingOfWhitespaceEmitsNoBlock(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		texts []string
	}{
		{"setext heading of vertical tabs in a quote", ">\v\n>\v\n>-", nil},
		{"setext heading of vertical tabs", "\v\n\v\n-", nil},
		{"ATX heading of a vertical tab", "# \v", nil},
		{"ATX heading of a form feed", "# \f", nil},
		// Controls.
		{"ATX heading with text", "# a\v", []string{"a\v"}},
		{"setext heading with text in a quote", "> a\n> \v\n> -", []string{"a\n> \v"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assert.Equal(t, tc.texts, readBlockTexts(t, tc.input))
		})
	}
}
