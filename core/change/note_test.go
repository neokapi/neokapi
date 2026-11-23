package change

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// SplitNote reads only the two forms released clients wrote, and the person's
// text comes back byte for byte. LegacyNote rebuilds exactly what was stored,
// so the decision identity of every note is unchanged.
func TestSplitNote(t *testing.T) {
	for _, tc := range []struct{ name, written, text, host string }{
		{"the suffix after a note", "Too formal. (recorded as a person in a claude-code shell)", "Too formal.", "claude-code"},
		{"the phrase alone", "recorded as a person in a codex shell", "", "codex"},
		{"a note ending in a semicolon keeps it", "Fix this; (recorded as a person in a claude-code shell)", "Fix this;", "claude-code"},
		{"a note ending in spaces keeps them", "Fix this   (recorded as a person in a claude-code shell)", "Fix this  ", "claude-code"},
		{"a person's own sentence is not the phrase", "I was recorded as a person in a bash shell", "I was recorded as a person in a bash shell", ""},
		{"a host no client wrote", "Fine. (recorded as a person in a bash shell)", "Fine. (recorded as a person in a bash shell)", ""},
		{"the phrase without its brackets after a note", "Fine. recorded as a person in a codex shell", "Fine. recorded as a person in a codex shell", ""},
		{"the suffix in the middle", "a (recorded as a person in a claude-code shell) b", "a (recorded as a person in a claude-code shell) b", ""},
		{"the suffix twice reads the last one only", "a (recorded as a person in a codex shell) (recorded as a person in a claude-code shell)", "a (recorded as a person in a codex shell)", "claude-code"},
		{"the suffix alone with no note before it", " (recorded as a person in a claude-code shell)", " (recorded as a person in a claude-code shell)", ""},
		{"a plain note", "Too formal.", "Too formal.", ""},
		{"no note", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, host := SplitNote(tc.written)
			assert.Equal(t, tc.text, text)
			assert.Equal(t, tc.host, host)
			assert.Equal(t, tc.written, LegacyNote(text, host), "the identity form is what was stored")
		})
	}
}
