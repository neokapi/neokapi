package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/workspace"
)

func ledgerOp(t *testing.T, id string, kind contextop.Kind, body map[string]any) workspace.Op {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	return workspace.Op{ID: id, Kind: contextop.OpKindPrefix + string(kind), Payload: raw}
}

// A context change names the term rules and notes it touched, the subject of
// a keep read from the operation it keeps, and no more than the bound.
func TestSampleLedgerOps(t *testing.T) {
	rule := func(term, replacement string) map[string]any {
		return map[string]any{"subject": contextop.Subject{
			Kind: contextop.SubjectTerm, Term: &profile.TermRule{Term: term, Replacement: replacement},
		}}
	}
	ops := []workspace.Op{
		ledgerOp(t, "o1", contextop.KindObserve, rule("translation memory", "content memory")),
		ledgerOp(t, "o2", contextop.KindKeep, map[string]any{"target": "held"}),
		ledgerOp(t, "o3", contextop.KindObserve, map[string]any{"subject": contextop.Subject{
			Kind: contextop.SubjectNote, Text: strings.Repeat("a", 200),
		}}),
		ledgerOp(t, "o4", contextop.KindObserve, rule("glossary", "terms")),
	}
	var change platev.ContextChange
	sampleLedgerOps(&change, ops, func(id string) (contextop.Subject, bool) {
		if id != "held" {
			return contextop.Subject{}, false
		}
		return contextop.Subject{Kind: contextop.SubjectTerm, Term: &profile.TermRule{Term: "Quick cast", Replacement: "Quickcast"}}, true
	})

	require.Len(t, change.Samples, platev.MaxContextSamples)
	assert.Equal(t, platev.ContextSample{
		Kind: platev.ContextItemTermRule, Operation: platev.ContextOpAdd,
		Term: "translation memory", Replacement: "content memory",
	}, change.Samples[0])
	assert.Equal(t, "Quickcast", change.Samples[1].Replacement)
	assert.Equal(t, platev.ContextOpKeep, change.Samples[1].Operation)
	assert.Equal(t, platev.ContextItemNote, change.Samples[2].Kind)
	assert.LessOrEqual(t, len([]rune(change.Samples[2].Text)), 141, "a note is cut to a bounded length")

	back := platev.ParseContextChange(change.Data())
	assert.Equal(t, change.Samples, back.Samples)
}
