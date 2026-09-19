package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Masking and comparing ask opposite questions of the same text, so they read it
// with different tokens.
//
// Comparing a source against its target wants the narrow reading: a braced run
// of prose or quoted JSON is not a placeholder, and reporting one as dropped
// says a translation lost something it never had.
//
// Masking for term matching wants the greedy reading. Everything that looks
// like program syntax is overwritten so a term cannot match inside it, run into
// it from the adjacent text, or bridge it as the gap in a term of several
// words. A brace the narrow reading declines is still syntax a term must not be
// found in.
//
// These pin the greedy half, so a change made for the comparison cannot loosen
// term matching without a test saying so.

func TestTermText_QuotedJSONIsMasked(t *testing.T) {
	src := `emits {"decision":"block","reason":"findings"} when a gate fails`
	got := TermText(src)

	assert.Len(t, got, len(src), "an offset into the projection is an offset into the text")
	assert.False(t, ContainsTerm(got, "findings"),
		"a word inside a JSON literal is not a use of the term: %q", got)
	assert.False(t, ContainsTerm(got, "decision"),
		"nor is a JSON key: %q", got)
	assert.True(t, ContainsTerm(got, "gate"), "the prose around it still counts")
}

func TestTermText_BracedSyntaxWithoutAPickerIsMasked(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      string
		masked  []string
		visible []string
	}{
		{
			name:    "a typed argument",
			in:      "Declared draught {draught, number} m against {available} m.",
			masked:  []string{"available", "number"},
			visible: []string{"draught"},
		},
		{
			name:    "a braced pair of words",
			in:      "a {pattern, format} pair of berths",
			masked:  []string{"pattern", "format"},
			visible: []string{"berths"},
		},
		{
			name:    "a JSON shape carrying angle brackets",
			in:      `the {"socket": "<path>"} entry names the berth`,
			masked:  []string{"socket", "path"},
			visible: []string{"berth"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := TermText(tc.in)
			assert.Len(t, got, len(tc.in))
			for _, term := range tc.masked {
				assert.False(t, ContainsTerm(got, term),
					"%q must not read %q as a term: %q", tc.in, term, got)
			}
			for _, term := range tc.visible {
				assert.True(t, ContainsTerm(got, term), "%q uses %q", tc.in, term)
			}
		})
	}
}
