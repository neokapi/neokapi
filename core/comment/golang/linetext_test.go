package golang

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLineText(t *testing.T) {
	for _, tc := range []struct {
		line string
		n    int
		text string
		ok   bool
	}{
		{line: "// Parse reads.", n: 15, text: " Parse reads.", ok: true},
		{line: "//okapi-skip: x", n: 15, text: "okapi-skip: x", ok: true},
		{line: "/* okapi-skip: x */", n: 19, text: " okapi-skip: x ", ok: true},
		{line: "/* a */ x := 1", n: 7, text: " a ", ok: true},
		{line: "/* opens a comment over several lines", ok: false},
		{line: " * okapi-skip: x", ok: false},
		{line: "okapi-skip: x", ok: false},
		{line: "*/", ok: false},
	} {
		n, text, ok := Provider{}.LineText([]byte(tc.line))
		assert.Equal(t, tc.ok, ok, tc.line)
		assert.Equal(t, tc.n, n, tc.line)
		assert.Equal(t, tc.text, text, tc.line)
	}
}
