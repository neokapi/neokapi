package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWhitespaceParagraphEmitsNoBlock covers a paragraph whose text is
// whitespace and line breaks alone. In a blockquote the reader bakes the
// continuation marker into such a block's text, so the marker was its only
// content, and ">\\\n\\\n>\f" rebuilt as an empty quote that read back as no
// block. A paragraph with nothing to translate is skeleton, the way one that
// is a single autolink is.
func TestWhitespaceParagraphEmitsNoBlock(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"breaks and a form feed in a quote", ">\\\n\\\n>\f", nil},
		{"form feed in a quote", ">\f", nil},
		{"form feed alone", "\f", nil},
		{"text then a break and a form feed", "a\n\n\\\n\f", []string{"a"}},
		// Control: a form feed beside text is content.
		{"text and a form feed", "a\f", []string{"a\f"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assert.Equal(t, tc.want, readBlockTexts(t, tc.input))
		})
	}
}
