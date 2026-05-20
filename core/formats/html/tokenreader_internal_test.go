package html

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

// TestRemainingContentCursorIsExactAndConstant walks the tokenizer over a
// self-similar div/span-heavy document and asserts that remainingContent
// returns the precise suffix of the input at the live tokenizer position on
// every token.
//
// This is the white-box guard for the #608 N1 fix. The previous
// implementation located the position with bytes.Index(content, Buffered())
// scanning from byte 0 — O(n) per call and, on self-similar content,
// returning the FIRST (earlier, wrong) match rather than the live position.
// The cursor-based implementation tracks consumed bytes in O(1) and is
// byte-exact regardless of content repetition. We verify exactness against an
// independently computed cursor (the running sum of len(Raw())).
func TestRemainingContentCursorIsExactAndConstant(t *testing.T) {
	// Build self-similar content: identical opening byte sequences repeated
	// many times so any bytes.Index-from-zero strategy would match an
	// earlier occurrence and return the wrong offset. The trailing unique
	// id keeps each block distinct without changing the leading bytes.
	var sb strings.Builder
	sb.WriteString("<html><body>")
	const blocks = 400
	for i := 0; i < blocks; i++ {
		sb.WriteString("<div>Text <span>x</span> tail " + strconv.Itoa(i) + "</div>")
	}
	sb.WriteString("</body></html>")
	content := []byte(sb.String())

	s := &tokenReaderState{content: content}
	tokenizer := html.NewTokenizer(bytes.NewReader(content))
	tokenizer.SetMaxBuf(0)

	// expectedOffset is the byte position just past the last-read token,
	// computed independently of s.consumed.
	expectedOffset := 0
	tokens := 0
	for {
		tt := s.next(tokenizer)
		if tt == html.ErrorToken {
			break
		}
		expectedOffset += len(tokenizer.Raw())
		tokens++

		// The cursor must equal the independently summed offset.
		require.Equal(t, expectedOffset, s.consumed,
			"consumed cursor diverged from cumulative Raw() length at token %d", tokens)

		// remainingContent must return exactly the suffix at the live
		// position — not an earlier self-similar match.
		remaining := s.remainingContent(tokenizer)
		require.Equal(t, content[expectedOffset:], remaining,
			"remainingContent must be the exact live suffix at token %d", tokens)
	}

	require.Greater(t, tokens, blocks, "should have walked the whole document")
	assert.Equal(t, len(content), s.consumed,
		"cursor should rest at end-of-document after the final token")
}

// TestRemainingContentFallbackWithoutContent verifies the documented fallback:
// when content is not saved (older callers / tests), remainingContent returns
// tokenizer.Buffered() rather than slicing a nil content.
func TestRemainingContentFallbackWithoutContent(t *testing.T) {
	content := []byte("<html><body><div>Hi <span>x</span></div></body></html>")
	s := &tokenReaderState{} // content intentionally left nil
	tokenizer := html.NewTokenizer(bytes.NewReader(content))
	tokenizer.SetMaxBuf(0)
	tokenizer.Next() // consume one token so Buffered() is non-trivial
	got := s.remainingContent(tokenizer)
	assert.Equal(t, tokenizer.Buffered(), got)
}
