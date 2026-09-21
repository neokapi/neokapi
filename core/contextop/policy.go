package contextop

import (
	"errors"
	"fmt"
)

// Transition is one operation about to be recorded, beside the operation it
// acts on.
//
// It carries everything a policy needs and nothing a policy has to go and fetch,
// which is what lets the decision sit in one function rather than at each call
// site.
type Transition struct {
	// Actor is who is recording it.
	Actor Actor
	// Kind is what they are doing.
	Kind Kind
	// Subject is what a subject-bearing operation is about.
	Subject SubjectKind
	// Target is the operation being acted on, and Targeted reports whether
	// there is one. A revert that names a session targets no single operation.
	Target   Record
	Targeted bool
	// Widening reports that the transition moves a rule to a broader point.
	// Confirming and widening in one step sets it on the confirm.
	Widening bool
	// Editing reports that the transition changes the rule it acts on.
	Editing bool
}

// Policy answers whether an actor may record a transition. Returning an error
// refuses it, and the error reaches the caller that tried to record.
//
// One function decides this for every writer: `kapi context`, `kapi apply`, an
// agent through the MCP tools, and the desktop feed. Actor, kind and target
// ride on every transition so the decision has what it needs, and an actor
// class with narrower or wider rights is a change here rather than at each call
// site.
type Policy func(Transition) error

// ErrRefused is what every refusal wraps, so a caller can tell a policy
// refusal from a storage failure without matching on wording.
var ErrRefused = errors.New("refused by the context policy")

// PersonDecides is the policy in force.
//
// Anyone may say what they saw. An agent or a tool observes, proposes and
// records a correction, and each of those takes effect as advice that no check
// can fail on. Only a person turns advice into a rule: confirming, editing what
// another actor proposed, discarding another actor's proposal, reverting a
// confirmed rule, and widening are all a person's.
//
// An actor may withdraw its own work: an agent that proposed something may
// discard or revert its own proposal while it is still a candidate, which is
// how a session cleans up after itself without asking anyone.
func PersonDecides(t Transition) error {
	if t.Actor.Kind == ActorPerson {
		return nil
	}
	switch t.Kind {
	case KindObserve, KindPropose, KindCorrect:
		if t.Widening {
			return refuse(t, "only a person widens a rule beyond the point its evidence was seen at")
		}
		return nil
	case KindConfirm:
		return refuse(t, "only a person confirms a rule")
	case KindWiden:
		return refuse(t, "only a person widens a rule beyond the point its evidence was seen at")
	case KindDiscard, KindRevert:
		if !t.Targeted {
			return refuse(t, "only a person reverts a whole session")
		}
		if t.Target.Status == StatusConfirmed {
			return refuse(t, "only a person withdraws a confirmed rule")
		}
		if !sameActor(t.Actor, t.Target.Actor) {
			return refuse(t, "only a person acts on another actor's operation")
		}
		return nil
	}
	return refuse(t, fmt.Sprintf("%q is not an operation kind", t.Kind))
}

// refuse renders a policy refusal, naming the actor and what it tried.
func refuse(t Transition, why string) error {
	return fmt.Errorf("%s may not %s: %s: %w", t.Actor.String(), t.Kind, why, ErrRefused)
}

// sameActor reports whether two actors are the same party. An agent is the same
// party as itself within one session; two sessions of one agent are not, because
// a later run has no standing to withdraw what an earlier one recorded and a
// person reviewed.
func sameActor(a, b Actor) bool {
	return a.Kind == b.Kind && a.Name == b.Name && a.Session == b.Session
}

// Allow records every transition, whoever proposes it. It is what a test
// driving the log directly uses, and what an embedding that enforces its own
// permissions elsewhere passes.
func Allow(Transition) error { return nil }
