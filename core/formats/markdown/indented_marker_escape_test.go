package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIndentedMarkerOnAContinuationLineIsEscaped is the #2504 reproducer. The
// escape that keeps a rebuilt block's continuation lines from re-reading as new
// blocks tested the line's first byte, and CommonMark allows up to three spaces
// of indentation before a block marker, so " # 0" opened a heading and the one
// block became two.
func TestIndentedMarkerOnAContinuationLineIsEscaped(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		written string
	}{
		{"one space before a heading", "<div>a\n # 0", "a\n \\# 0\n"},
		{"three spaces before a bullet", "<div>a\n   - b", "a\n   \\- b\n"},
		{"two spaces before a quote", "<div>a\n  > b", "a\n  \\> b\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := roundtrip(t, tc.input)
			assert.Equal(t, tc.written, out)
			require.Len(t, readBlocks(t, out), 1, "the rebuilt block did not stay one block")
			assert.Equal(t, out, roundtrip(t, out), "the rebuild is not idempotent")
		})
	}
}

// TestFourSpacesIsIndentedCodeNotAMarker pins the boundary: a fourth space
// makes the line indented code, where the marker opens nothing.
func TestFourSpacesIsIndentedCodeNotAMarker(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "a\n    - b\n", roundtrip(t, "<div>a\n    - b"))
}
