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
	// Keeping and widening in one step sets it on the keep.
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
// Anyone may say what they saw. An agent or a tool observes and records a
// correction, and each of those takes effect as a suggestion that no check can
// fail on. Only a person establishes a rule: keeping a suggestion, editing
// what another actor suggested, importing context, writing a rule directly,
// dropping a suggestion, reverting an established rule, and widening are all a
// person's.
//
// Anyone may withdraw their own suggestion in the session that recorded it,
// which is how an agent cleans up after itself without asking anyone. A later
// session has no standing to withdraw what an earlier one recorded.
func PersonDecides(t Transition) error {
	if t.Kind == KindWithdraw {
		return withdrawable(t)
	}
	if t.Actor.Kind == ActorPerson {
		return nil
	}
	switch t.Kind {
	case KindObserve, KindCorrect:
		if t.Widening {
			return refuse(t, "only a person widens a rule beyond the point its evidence was seen at")
		}
		return nil
	case KindImport, KindEdit:
		return refuse(t, "only a person imports context or writes a rule directly")
	case KindKeep:
		return refuse(t, "only a person keeps a suggestion")
	case KindDrop:
		return refuse(t, "only a person drops a suggestion; withdraw your own instead")
	case KindWiden:
		return refuse(t, "only a person widens a rule beyond the point its evidence was seen at")
	case KindRevert:
		if !t.Targeted {
			return refuse(t, "only a person reverts a whole session")
		}
		if t.Target.Established {
			return refuse(t, "only a person reverts an established rule")
		}
		if !sameActor(t.Actor, t.Target.Actor) {
			return refuse(t, "only a person acts on another actor's operation")
		}
		return nil
	}
	return refuse(t, fmt.Sprintf("%q is not an operation kind", t.Kind))
}

// withdrawable decides a withdrawal, which is the same for every actor: its
// author takes back a suggestion in the session that recorded it.
func withdrawable(t Transition) error {
	if !t.Targeted {
		return refuse(t, "a withdrawal names one operation")
	}
	if !sameActor(t.Actor, t.Target.Actor) {
		return refuse(t, "only its author withdraws a suggestion, in the session that recorded it")
	}
	if !t.Target.Status.Advises() || t.Target.Established {
		return refuse(t, fmt.Sprintf("operation %s is %s, and only a suggestion can be withdrawn", t.Target.ID, t.Target.Status))
	}
	return nil
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

// Allow records every transition, whoever records it. It is what a test
// driving the log directly uses, and what an embedding that enforces its own
// permissions elsewhere passes.
func Allow(Transition) error { return nil }
