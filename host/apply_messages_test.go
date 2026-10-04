package host

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/contextop"
)

// A stale refusal's current text of several lines prints indented under the
// operation, so it reads as one value rather than as lines of the report.
func TestPrintChangeResult_IndentsTheCurrentText(t *testing.T) {
	var buf bytes.Buffer
	printChangeResult(&buf, &change.Result{Status: change.SetRefused, Ops: []change.OpResult{{
		I: 0, Op: change.KindSetContent, Status: change.OpRefused,
		At:      &change.Ref{Doc: "docs/a.md", Block: "p"},
		Error:   &change.Error{Code: change.CodeStale, Message: "edition en changed after you read it"},
		Current: &change.Current{Rev: "r:4247d44c3e210651", Text: "It takes about ten minutes.\nThen restart the app."},
	}}})
	lines := strings.Split(buf.String(), "\n")
	require.GreaterOrEqual(t, len(lines), 3)
	assert.Equal(t, "  current r:4247d44c3e210651: It takes about ten minutes.", lines[1])
	assert.Equal(t, "    Then restart the app.", lines[2])
}

// An observation that names a term with nothing to avoid, and says what was
// seen in text, is recorded as a note rather than refused.
func TestObservedSubject_ATermWithNothingToAvoidKeepsItsTextAsANote(t *testing.T) {
	s, err := observedSubject(ContextObserveRequest{Term: "Bestill", Text: "the button says Bestill, not Bok"})
	require.NoError(t, err)
	assert.Equal(t, contextop.SubjectNote, s.Kind)
	assert.Equal(t, "the button says Bestill, not Bok", s.Text)

	_, err = observedSubject(ContextObserveRequest{Term: "Bestill"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "say what you saw in text")
}
