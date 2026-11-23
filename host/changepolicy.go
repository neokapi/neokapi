package host

import (
	"fmt"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/contextop"
)

// ChangePolicy is the actor policy kapi holds a change set to: the context
// policy (contextop.PersonDecides) extended to the change contract, whichever
// surface carries the change set.
//
// A person may send every operation, choose either gate and write blindly. An
// agent may change content, and may not:
//
//   - send a term, memory or recipe operation; it records an observation or a
//     correction instead, and a person keeps it.
//   - decide with any outcome but prereview.
//   - send if_match "*", which writes without having read the edition.
//   - choose gate report, which lands an edit over the failing findings it
//     introduces.
//
// A tool in a flow may choose report, and only a tool records provenance. A
// tool, like an agent, decides only with prereview; a surface that carries a
// person's decision (a pull, a merge) sends it as that person.
//
// A sender of any other kind, or of none, is refused every operation.
type ChangePolicy struct {
	// Context decides the transitions the asset operations stand for. Nil is
	// contextop.PersonDecides, the policy in force.
	Context contextop.Policy
}

var _ change.Policy = ChangePolicy{}

// Permit implements change.Policy.
func (p ChangePolicy) Permit(actor change.Actor, set *change.Set, op change.Op) *change.Error {
	switch actor.Kind {
	case change.ActorPerson, change.ActorAgent, change.ActorTool:
	default:
		return &change.Error{
			Code: change.CodeNotPermitted,
			Message: fmt.Sprintf("a sender of kind %q may not send a change set: the surface that carries it "+
				"names a person, an agent or a tool in a flow as its sender", actor.Kind),
		}
	}
	if set != nil && set.Gate == change.GateReport && actor.Kind == change.ActorAgent {
		return notPermitted("gate", actor, "choose gate report",
			"an edit lands over the findings it introduces only when a person overrides them; fix the wording, or ask a person")
	}
	if actor.Kind == change.ActorAgent && blindWrite(op) {
		return notPermitted("if_match", actor, `send if_match "*"`,
			"an agent writes over the revision it read; read the edition and send its revision")
	}
	switch op.Kind {
	case change.KindTerm, change.KindMemory, change.KindRecipe:
		return p.asset(actor, op.Kind)
	case change.KindDecide:
		d, _ := op.Body.(*change.Decide)
		if actor.Kind != change.ActorPerson && (d == nil || d.Outcome != change.OutcomePrereview) {
			return notPermitted("outcome", actor, "decide "+outcomeOf(d),
				"a review decision is a person's; record a pre-review with outcome prereview")
		}
	case change.KindProvenance:
		if actor.Kind != change.ActorTool {
			return notPermitted("op", actor, "record provenance",
				"only a tool in a flow says how it produced an edition")
		}
	}
	return nil
}

// asset asks the context policy whether actor may write the project's context
// or its recipe directly. The write is an edit of the store, which the policy
// gives to a person.
func (p ChangePolicy) asset(actor change.Actor, kind change.Kind) *change.Error {
	decide := p.Context
	if decide == nil {
		decide = contextop.PersonDecides
	}
	subject, route := contextop.SubjectNone, agentAssetRoute(kindRecipe)
	switch kind {
	case change.KindTerm:
		subject, route = contextop.SubjectTerm, agentAssetRoute(kindTerm)
	case change.KindMemory:
		subject, route = contextop.SubjectMemory, agentAssetRoute(kindMemory)
	}
	err := decide(contextop.Transition{
		Actor:   contextActorOf(actor),
		Kind:    contextop.KindEdit,
		Subject: subject,
	})
	if err == nil {
		return nil
	}
	return &change.Error{Code: change.CodeNotPermitted, Field: "op", Message: err.Error() + "; " + route}
}

// blindWrite reports whether op names AnyRevision as a revision it read.
func blindWrite(op change.Op) bool {
	if op.IfMatch == change.AnyRevision {
		return true
	}
	if d, ok := op.Body.(*change.DeleteBlock); ok && d != nil {
		for _, rev := range d.IfMatch {
			if rev == change.AnyRevision {
				return true
			}
		}
	}
	return false
}

// outcomeOf names a decision's outcome for a refusal.
func outcomeOf(d *change.Decide) string {
	if d == nil || d.Outcome == "" {
		return "with no outcome"
	}
	return string(d.Outcome)
}

// notPermitted is a policy refusal: who tried what, and why not.
func notPermitted(field string, actor change.Actor, what, why string) *change.Error {
	return &change.Error{
		Code:    change.CodeNotPermitted,
		Field:   field,
		Message: fmt.Sprintf("%s may not %s: %s", actorText(actor), what, why),
	}
}

// actorText renders an actor the way the context log does: "agent
// claude/s_01", "person asgeir".
func actorText(actor change.Actor) string {
	return contextActorOf(actor).String()
}
