package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildWeighsDelimitersAfterTheLeadingMarkerEscape covers literal
// asterisks that open a block once the inline HTML before them is dropped,
// with an emphasis on the line after: "<A>***" over "*0*". The run of three
// could pair with nothing as written and stood; the block path then escaped
// its first asterisk, as it escapes a thematic break, and the two it left
// could close against the emphasis, which the pass after escaped. The
// leading marker is escaped before the delimiters are weighed.
func TestRebuildWeighsDelimitersAfterTheLeadingMarkerEscape(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"asterisk run then emphasis", "<A>***\n*0*"},
		{"dash run then emphasis", "<A>---\n*0*"},
		{"underscore run then emphasis", "<A>___\n_0_"},
		// Control: a run before text.
		{"asterisk run then text", "<A>***\n0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
	assert.Equal(t, "\\*\\*\\*\n*0*\n", assertRebuildKeepsItsBlocks(t, "<A>***\n*0*"))
}
