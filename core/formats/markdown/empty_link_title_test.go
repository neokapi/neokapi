package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEmptyLinkTitleKeepsItsDelimiters is the #2502 reproducer. The reader
// accepted the closer it found in source only when the parser had resolved a
// title of its own, and an empty title resolves to nothing, so the closer was
// rebuilt from the resolved values and the delimiters went with it.
func TestEmptyLinkTitleKeepsItsDelimiters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"empty single-quoted title", "text [x](a '') more\n"},
		{"empty double-quoted title", "text [x](a \"\") more\n"},
		{"empty parenthesised title", "text [x](a ()) more\n"},
		{"empty title on an empty link", "[](a '')"},
		{"empty title on an image", "An ![i](s '') here.\n"},
		{"space before the closing paren", "a [b](x ) c\n"},
		{"title with content still splits", "[a](b 't')\n"},
		{"image title with content", "![a](b (t))\n"},
		{"plain link", "a [b](/x) c\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
		})
	}
}
