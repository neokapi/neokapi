package markdown_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNestedBlockquoteRebuildKeepsItsMarkers is the #2464 reproducer. The
// rebuild path recovered a blockquote's opening marker from the first
// continuation line that carried one, took a single level of it whatever the
// line spelled, and had nothing at all to take for a quote with no continuation
// line: ">> a" came back as the paragraph "a", the quote structure gone.
//
// The reader records the marker the block's own first line carried, and the
// continuation recovery reads the whole sequence, indent included.
func TestNestedBlockquoteRebuildKeepsItsMarkers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		out   string
	}{
		{"nested, both lines marked", ">> a\n>> b\n", ">> a\n>> b\n"},
		{"nested with spaces", "> > a\n> > b\n", "> > a\n> > b\n"},
		{"nested, single line", ">> a\n", ">> a\n"},
		{"single level, single line", "> a\n", "> a\n"},
		{"three levels", ">>> a\n>>> b\n", ">>> a\n>>> b\n"},
		{"no space after the marker", ">a\n>b\n", ">a\n>b\n"},
		{"indented continuation marker", ">>0\n >0\n", ">>0\n >0\n"},
		{"indented marker, one level", ">0\n >0\n", ">0\n >0\n"},
		{"lazy continuation", "> a\nb\n> c\n", "> a\nb\n> c\n"},
		{"hard break body", "> a  \n> b\n", "> a\n> b\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			before := readBlocks(t, tc.input)
			require.Len(t, before, 1, "input is not one block")

			out := rebuildBlocks(t, before...)
			assert.Equal(t, tc.out, out)

			after := readBlocks(t, out)
			require.Len(t, after, 1, "rebuilt %q re-read as %d blocks", out, len(after))
			assert.Equal(t, out, rebuildBlocks(t, after...), "rebuild is not idempotent for %q", tc.input)
		})
	}
}

// TestQuoteMarkerPropertyRecordsTheFirstLine pins what the reader stores: the
// whole marker sequence the block's first line carried, and nothing for a block
// that opens no quote.
func TestQuoteMarkerPropertyRecordsTheFirstLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input  string
		marker string
	}{
		{"> a\n", "> "},
		{">> a\n", ">> "},
		{"> > a\n", "> > "},
		{">a\n", ">"},
		{"  > a\n", "  > "},
		{"a\n", ""},
		{"- a\n", ""},
		{"- > a\n", ""},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			blocks := readBlocks(t, tc.input)
			require.NotEmpty(t, blocks)
			assert.Equal(t, tc.marker, blocks[0].Properties[markdown.BlockPropQuoteMarker])
		})
	}
}

// TestNestedBlockquoteRoundTripsThroughTheHarness drives the reproducers
// through the real read -> rebuild-write -> read harness. The fuzz reproducers
// are committed under testdata/fuzz.
func TestNestedBlockquoteRoundTripsThroughTheHarness(t *testing.T) {
	t.Parallel()
	for _, in := range []string{">> a\n>> b", ">> a", "> > a", ">>0\n >0", ">0\n >0"} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			out1, keys1, ok := tripMarkdown(ctx, []byte(in))
			require.True(t, ok, "first trip declined %q", in)
			_, keys2, ok2 := tripMarkdown(ctx, out1)
			require.True(t, ok2, "re-reading %q lost every block", out1)
			assert.Len(t, keys2, len(keys1), "block count changed via %q", out1)
		})
	}
}
