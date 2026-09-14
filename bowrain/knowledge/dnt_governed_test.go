package knowledge_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

func rawOp(op knowledge.OpType, payload string) knowledge.ChangeSetOp {
	return knowledge.ChangeSetOp{Op: op, Payload: json.RawMessage(payload)}
}

// TestDoNotTranslateIsAGovernedChange: setting or clearing a concept's
// do-not-translate flag, and creating a concept that carries it, change what the
// checks enforce, so each is a governed op. An update that leaves the flag alone
// stays ordinary.
func TestDoNotTranslateIsAGovernedChange(t *testing.T) {
	cases := []struct {
		name     string
		op       knowledge.ChangeSetOp
		governed bool
	}{
		{"setting the flag", rawOp(knowledge.OpConceptUpdate, `{"concept_id": "c-kapi", "do_not_translate": true}`), true},
		{"clearing the flag", rawOp(knowledge.OpConceptUpdate, `{"concept_id": "c-kapi", "do_not_translate": false}`), true},
		{"an update that leaves the flag alone", rawOp(knowledge.OpConceptUpdate, `{"concept_id": "c-kapi", "definition": "The CLI."}`), false},
		{"creating a concept with the flag", rawOp(knowledge.OpConceptCreate,
			`{"concept": {"id": "c-kapi", "do_not_translate": true, "terms": [{"text": "kapi", "locale": "en", "status": "approved"}]}}`), true},
		{"creating a concept without the flag", rawOp(knowledge.OpConceptCreate,
			`{"concept": {"id": "c-save", "terms": [{"text": "save", "locale": "en", "status": "approved"}]}}`), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.NoError(t, knowledge.ValidateOp(c.op))
			governed, err := knowledge.IsGovernedOp(c.op)
			require.NoError(t, err)
			assert.Equal(t, c.governed, governed)
		})
	}
}

// TestConceptUpdateAppliesDoNotTranslate: a governed concept update sets and
// clears the flag, and an update that names only other fields leaves it.
func TestConceptUpdateAppliesDoNotTranslate(t *testing.T) {
	ctx := context.Background()
	base := terms.NewInMemoryStore()
	require.NoError(t, base.AddConcept(ctx, terms.Concept{
		ID: "c-kapi", Definition: "The CLI.",
		Terms: []terms.Term{{Text: "kapi", Locale: "en", Status: model.TermApproved}},
	}))
	flag := func(tb *terms.InMemoryStore) terms.Concept {
		t.Helper()
		c, ok, err := tb.GetConcept(ctx, "c-kapi")
		require.NoError(t, err)
		require.True(t, ok)
		return c
	}

	set, err := knowledge.ApplyOpsToTerms(ctx, base, []knowledge.ChangeSetOp{
		rawOp(knowledge.OpConceptUpdate, `{"concept_id": "c-kapi", "do_not_translate": true}`),
	})
	require.NoError(t, err)
	assert.True(t, flag(set).DoNotTranslate, "the update sets the flag")
	assert.Equal(t, "The CLI.", flag(set).Definition, "an update naming only the flag keeps the rest")

	kept, err := knowledge.ApplyOpsToTerms(ctx, set, []knowledge.ChangeSetOp{
		rawOp(knowledge.OpConceptUpdate, `{"concept_id": "c-kapi", "definition": "The kapi CLI."}`),
	})
	require.NoError(t, err)
	assert.True(t, flag(kept).DoNotTranslate, "an update that omits the flag leaves it")

	cleared, err := knowledge.ApplyOpsToTerms(ctx, kept, []knowledge.ChangeSetOp{
		rawOp(knowledge.OpConceptUpdate, `{"concept_id": "c-kapi", "do_not_translate": false}`),
	})
	require.NoError(t, err)
	assert.False(t, flag(cleared).DoNotTranslate, "the update clears the flag")
}
