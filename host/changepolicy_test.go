package host

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
)

func TestChangePolicy(t *testing.T) {
	person := change.Actor{Kind: change.ActorPerson, Name: "asgeir"}
	agent := change.Actor{Kind: change.ActorAgent, Name: "claude", Session: "s_01"}
	flow := change.Actor{Kind: change.ActorTool, Name: "tool:converge"}
	// unnamed is the sender of a surface that stamped none; robot is a kind
	// the contract does not define.
	unnamed := change.Actor{}
	robot := change.Actor{Kind: "robot", Name: "r2"}

	at := change.Ref{Doc: "docs/guide.md", Block: "p", Edition: model.EditionKey{Locale: "fr"}}
	text := "Nous utilisons le gadget."
	content := change.Op{Kind: change.KindSetContent, At: at, IfMatch: "r:0123456789abcdef",
		Body: &change.SetContent{Text: &text}}
	blind := content
	blind.IfMatch = change.AnyRevision
	deleteBlind := change.Op{Kind: change.KindDeleteBlock, At: change.Ref{Doc: "docs/guide.md", Block: "p"},
		Body: &change.DeleteBlock{IfMatch: map[string]string{"": "r:0123456789abcdef", "fr": change.AnyRevision}}}
	decide := func(o change.Outcome) change.Op {
		return change.Op{Kind: change.KindDecide, At: at, IfMatch: "r:0123456789abcdef", Body: &change.Decide{Outcome: o}}
	}
	term := change.Op{Kind: change.KindTerm, Body: &change.Term{Action: "upsert", Term: "use", Status: "preferred"}}
	memory := change.Op{Kind: change.KindMemory, Body: &change.Memory{Action: "add",
		From: change.MemoryText{Edition: "en", Text: "Save"}, To: change.MemoryText{Edition: "fr", Text: "Enregistrer"}}}
	recipe := change.Op{Kind: change.KindRecipe, Body: &change.Recipe{Path: "defaults.target_languages", Value: []byte(`["fr"]`)}}
	provenance := change.Op{Kind: change.KindProvenance, At: at, Body: &change.Provenance{Status: model.Status("draft")}}

	enforce := &change.Set{Gate: change.GateEnforce}
	report := &change.Set{Gate: change.GateReport}

	cases := []struct {
		name  string
		actor change.Actor
		set   *change.Set
		op    change.Op
		// field is the refused field; empty permits the operation.
		field string
	}{
		{"an agent changes content", agent, enforce, content, ""},
		{"an agent may not choose gate report", agent, report, content, "gate"},
		{"a person may choose gate report", person, report, content, ""},
		{"a tool in a flow may choose gate report", flow, report, content, ""},
		{"a flow's writes carry no change set", flow, nil, content, ""},
		{"an agent may not write blindly", agent, enforce, blind, "if_match"},
		{"an agent may not delete a block blindly", agent, enforce, deleteBlind, "if_match"},
		{"a person may write blindly", person, enforce, blind, ""},
		{"an agent pre-reviews", agent, enforce, decide(change.OutcomeAdvise), ""},
		{"an agent may not establish", agent, enforce, decide(change.OutcomeEstablish), "outcome"},
		{"an agent may not reject", agent, enforce, decide(change.OutcomeReject), "outcome"},
		{"an agent may not withdraw a decision", agent, enforce, decide(change.OutcomeWithdraw), "outcome"},
		{"a person establishes", person, enforce, decide(change.OutcomeEstablish), ""},
		{"a person rejects", person, enforce, decide(change.OutcomeReject), ""},
		{"a tool in a flow pre-reviews", flow, nil, decide(change.OutcomeAdvise), ""},
		{"a tool in a flow may not establish", flow, nil, decide(change.OutcomeEstablish), "outcome"},
		{"a tool in a flow may not reject", flow, nil, decide(change.OutcomeReject), "outcome"},
		{"a tool in a flow may not withdraw a decision", flow, nil, decide(change.OutcomeWithdraw), "outcome"},
		{"an agent may not write a term", agent, enforce, term, "op"},
		{"an agent may not write a memory pair", agent, enforce, memory, "op"},
		{"an agent may not change the recipe", agent, enforce, recipe, "op"},
		{"a person writes a term", person, enforce, term, ""},
		{"a person writes a memory pair", person, enforce, memory, ""},
		{"a person changes the recipe", person, enforce, recipe, ""},
		{"a tool in a flow records provenance", flow, nil, provenance, ""},
		{"an agent may not record provenance", agent, enforce, provenance, "op"},
		{"a person may not record provenance", person, enforce, provenance, "op"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ChangePolicy{}.Permit(tc.actor, tc.set, tc.op)
			if tc.field == "" {
				assert.Nil(t, err)
				return
			}
			require.NotNil(t, err)
			assert.Equal(t, change.CodeNotPermitted, err.Code)
			assert.Equal(t, tc.field, err.Field)
			assert.NotEmpty(t, err.Message)
		})
	}

	// A sender the surface did not name, or named with a kind the contract
	// does not define, is refused everything a person may do, so a surface
	// that forgets to stamp its sender fails closed.
	for _, sender := range []change.Actor{unnamed, robot} {
		for _, tc := range []struct {
			name string
			set  *change.Set
			op   change.Op
		}{
			{"change content", enforce, content},
			{"choose gate report", report, content},
			{"write blindly", enforce, blind},
			{"establish", enforce, decide(change.OutcomeEstablish)},
			{"pre-review", enforce, decide(change.OutcomeAdvise)},
			{"write a term", enforce, term},
			{"record provenance", nil, provenance},
		} {
			t.Run(fmt.Sprintf("a sender of kind %q may not %s", sender.Kind, tc.name), func(t *testing.T) {
				err := ChangePolicy{}.Permit(sender, tc.set, tc.op)
				require.NotNil(t, err)
				assert.Equal(t, change.CodeNotPermitted, err.Code)
				assert.Contains(t, err.Message, fmt.Sprintf("kind %q", sender.Kind))
			})
		}
	}
}

// An agent refused a term write is told what to do instead, and the refusal
// is the context policy's: the same words `kapi apply` gives for a term entry.
func TestChangePolicy_AssetRefusalNamesTheRoute(t *testing.T) {
	agent := change.Actor{Kind: change.ActorAgent, Name: "claude", Session: "s_01"}
	err := ChangePolicy{}.Permit(agent, &change.Set{}, change.Op{Kind: change.KindTerm, Body: &change.Term{Action: "upsert", Term: "use"}})
	require.NotNil(t, err)
	assert.Contains(t, err.Message, "agent claude/s_01")
	assert.Contains(t, err.Message, "context_observe")

	err = ChangePolicy{}.Permit(agent, &change.Set{}, change.Op{Kind: change.KindRecipe, Body: &change.Recipe{Path: "name", Value: []byte(`"x"`)}})
	require.NotNil(t, err)
	assert.Contains(t, err.Message, "kapi.yaml")
}

// The asset operations go through the context policy the ChangePolicy names,
// so an embedding that grants an agent a wider right grants it here too.
func TestChangePolicy_AssetsFollowTheContextPolicy(t *testing.T) {
	agent := change.Actor{Kind: change.ActorAgent, Name: "claude", Session: "s_01"}
	var asked []contextop.Transition
	allow := func(tr contextop.Transition) error {
		asked = append(asked, tr)
		return nil
	}
	term := change.Op{Kind: change.KindTerm, Body: &change.Term{Action: "upsert", Term: "use"}}
	assert.Nil(t, ChangePolicy{Context: allow}.Permit(agent, &change.Set{}, term))
	require.Len(t, asked, 1)
	assert.Equal(t, contextop.KindEdit, asked[0].Kind)
	assert.Equal(t, contextop.SubjectTerm, asked[0].Subject)
	assert.Equal(t, contextop.Actor{Kind: contextop.ActorAgent, Name: "claude", Session: "s_01"}, asked[0].Actor)

	refuse := func(contextop.Transition) error { return errors.New("no") }
	person := change.Actor{Kind: change.ActorPerson}
	err := ChangePolicy{Context: refuse}.Permit(person, &change.Set{}, term)
	require.NotNil(t, err)
	assert.Equal(t, change.CodeNotPermitted, err.Code)
}
