package screens

import (
	"time"

	"github.com/neokapi/neokapi/core/contextop"
)

// ContextActorDTO is who performed an operation.
type ContextActorDTO struct {
	// Kind is "person", "agent" or "tool".
	Kind string `json:"kind"`
	// Name is the person's handle, the agent's client name, or the tool's
	// name. A person working alone has none.
	Name string `json:"name,omitempty"`
	// Session groups one agent run's operations.
	Session string `json:"session,omitempty"`
	// Host is the machine or client the agent ran under, when the actor's name
	// carries one as "<client>@<host>".
	Host string `json:"host,omitempty"`
}

// ContextEvidenceDTO is where a subject was seen.
type ContextEvidenceDTO struct {
	Path  string `json:"path,omitempty"`
	Unit  string `json:"unit,omitempty"`
	Quote string `json:"quote,omitempty"`
}

// DigestItem is one rule, suggestion or fact as the digest shows it.
type DigestItem struct {
	// ID names the operation every action on the item passes back, and Short
	// is the form a person types.
	ID    string `json:"id"`
	Short string `json:"short"`
	// Status is the operation's folded status.
	Status contextop.Status `json:"status"`
	// Theme is which suggestion theme the item belongs to.
	Theme string `json:"theme"`
	// Sentence is the rule as a sentence: "Write Quickcast, not Quick cast or
	// QuickCast."
	Sentence string `json:"sentence"`
	// Subject is the rule itself, for a surface that renders its parts.
	Subject contextop.Subject `json:"subject"`
	// Quote is the evidence the item was seen in, with the file it links to.
	Quote *contextop.Evidence `json:"quote,omitempty"`
	// Collection is the collection the evidence sits in, empty when the
	// project declares none or the recipe is not on this machine.
	Collection string `json:"collection,omitempty"`
	// Standing is the evidence as plain counts: "seen in 3 sessions · 14 of
	// 15 uses in docs/ · merged in #412".
	Standing string `json:"standing,omitempty"`
	// Usage is the project's own words back to it, from the latest usage
	// count, for a rule a check has counted.
	Usage *DigestUsage `json:"usage,omitempty"`
	// NoticedBy is who recorded it.
	NoticedBy contextop.Actor `json:"noticed_by"`
	// At is when it was recorded.
	At time.Time `json:"at"`
	// New reports an item recorded (or, for an established rule, established)
	// after the marker.
	New bool `json:"new"`
	// Scope is how far the rule answers, as the log prints it.
	Scope string `json:"scope"`
	// Keepable reports a suggestion a person can keep as it stands. A
	// contested suggestion with a rival waits until the rival is dropped.
	Keepable bool `json:"keepable"`
	// Droppable reports a suggestion or a rule a person can set aside. A rule
	// in force is taken back out of the stores as it is dropped.
	Droppable bool `json:"droppable"`
	// Widenable reports a rule in force a person can apply more widely.
	Widenable bool `json:"widenable"`
	// WidenTo are the widenings open to a widenable rule: "workspace", and
	// each axis of its point (ContextWidenOptions).
	WidenTo []string `json:"widen_to,omitempty"`
	// EstablishedAt and How say, for an established rule, when it came into
	// force and on what: "merged in #412", "your correction in billing.md",
	// "kept by you".
	EstablishedAt time.Time `json:"established_at,omitzero"`
	How           []string  `json:"how,omitempty"`
}

// Established reports an item that is a rule in force.
func (it DigestItem) Established() bool { return !it.EstablishedAt.IsZero() }

// DigestUsage is how often the project's content writes a rule's forms.
type DigestUsage struct {
	// Preferred is the form the rule says to write, and PreferredCount its
	// uses.
	Preferred      string `json:"preferred"`
	PreferredCount int    `json:"preferred_count"`
	// Rejected are the forms the rule avoids, and RejectedCount their uses
	// together.
	Rejected      []string `json:"rejected"`
	RejectedCount int      `json:"rejected_count"`
	// Within is the part of the project counted, as a path prefix; empty for
	// the whole project.
	Within string `json:"within,omitempty"`
	// Line says it in a sentence: `docs/ says "studio" 41 times and
	// "business" twice`.
	Line string `json:"line"`
}

// DigestConflict is a disagreement a person settles: two or more rules saying
// different things about one word, or one rule the evidence turned against.
type DigestConflict struct {
	// Sides are the rules that disagree. Choosing one keeps it and drops the
	// others.
	Sides []DigestItem `json:"sides"`
	// ByEvidence reports a rule contested by evidence alone, with no rival
	// rule: keeping it again is the choice, and so is dropping it.
	ByEvidence bool `json:"by_evidence"`
	// Reason says what the disagreement is, in a sentence.
	Reason string `json:"reason"`
}
