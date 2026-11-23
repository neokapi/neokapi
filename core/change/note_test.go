package change

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A note a released client wrote splits into what the person typed and the
// agent host, and LegacyNote rebuilds exactly what the client wrote.
func TestSplitNote(t *testing.T) {
	for _, tc := range []struct{ written, text, host string }{
		{"Too formal. (recorded as a person in a claude-code shell)", "Too formal.", "claude-code"},
		{"recorded as a person in a codex shell", "", "codex"},
		{"applied with `kapi apply` (recorded as a person in a claude-code shell)", "applied with `kapi apply`", "claude-code"},
		{"Too formal.", "Too formal.", ""},
		{"", "", ""},
		{"I was recorded as a person in a meeting, not a shell of any kind.", "I was recorded as a person in a meeting, not a shell of any kind.", ""},
	} {
		text, host := SplitNote(tc.written)
		assert.Equal(t, tc.text, text, tc.written)
		assert.Equal(t, tc.host, host, tc.written)
		assert.Equal(t, tc.written, LegacyNote(text, host), "the identity form is what the client wrote")
	}
}
