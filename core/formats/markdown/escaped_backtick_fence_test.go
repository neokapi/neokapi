package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEscapedBacktickBesideCodeSpanFence is the #2618 reproducer. A
// backslash-escaped backtick is a literal (CommonMark 2.4), and the walk that
// recovers a code span's fence from source counted one that touched the
// opening fence as part of it, so the rebuilt opener was a tick longer than the
// source spelled.
func TestEscapedBacktickBesideCodeSpanFence(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"escaped tick before the opener", "A tick \\``code` in a sentence.\n"},
		{"escaped tick at the start of the line", "\\``a`\n"},
		{"escaped tick before a double fence", "\\```` a ``\n"},
		{"escaped backslash before the opener", "\\\\`code` here.\n"},
		{"escaped tick after the closer", "A `code`\\` tick.\n"},
		{"content ending in a backslash", "A `a\\` span.\n"},
		{"escaped tick on both sides", "\\``code`\\` here.\n"},
		{"escaped tick in a blockquote", "> A tick \\``code` here.\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			require.Equal(t, tc.input, roundtrip(t, tc.input), "rebuild path is not byte-exact")
		})
	}
}
