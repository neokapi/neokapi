package yaml

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommentLineText(t *testing.T) {
	for _, tc := range []struct {
		line string
		n    int
		text string
		ok   bool
	}{
		{line: "# Greets.", n: 9, text: " Greets.", ok: true},
		{line: "#okapi-skip: x", n: 14, text: "okapi-skip: x", ok: true},
		{line: "# okapi-skip: x  \t", n: 15, text: " okapi-skip: x  \t", ok: true},
		{line: "greeting: Hello", ok: false},
	} {
		n, text, ok := CommentProvider{}.LineText([]byte(tc.line))
		assert.Equal(t, tc.ok, ok, tc.line)
		assert.Equal(t, tc.n, n, tc.line)
		assert.Equal(t, tc.text, text, tc.line)
	}
}
