// Package ref carries the freshness ref: one composite value that answers, for
// a single synchronized stream, how far what is held here sits from what is
// held there — on every axis at once, in one place.
//
// A ref has four components, and they are deliberately not the same kind of
// value:
//
//	content   — a monotonic position. Ahead and behind are meaningful, and a
//	            transfer interrupted at a position resumes from it.
//	context   — an identity hash of the governing context.
//	terms     — an identity hash of the governed terminology.
//	decisions — an identity hash of the committed decision record.
//
// The three identity components are compared for equality and for nothing
// else. Two unequal hashes carry no ordering, so a ref never claims one side is
// newer than the other: it reports that governance moved and leaves the
// reconciliation to a caller who can ask a human. Cheap observation, explicit
// resolution.
//
// Componentwise comparison is what makes the composite safe to write against. A
// writer asserts only the component it owns — Assert — so ordinary content
// traffic, which moves nothing but the position, can never manufacture a
// governance conflict for a writer that is nowhere near it.
package ref

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
)

// Component names one axis of a ref. The values are stable identifiers: they
// name the component in a compare-and-swap assertion and in a conflict a caller
// renders, so both sides of a protocol spell them the same way.
type Component string

const (
	// ComponentContent is the monotonic position component.
	ComponentContent Component = "content"
	// ComponentContext is the governing context's identity.
	ComponentContext Component = "context"
	// ComponentTerms is the governed terminology's identity.
	ComponentTerms Component = "terms"
	// ComponentDecisions is the committed decision record's identity.
	ComponentDecisions Component = "decisions"
)

// Components lists the ref's components in report order — position first,
// then the governance identities.
func Components() []Component {
	return []Component{ComponentContent, ComponentContext, ComponentTerms, ComponentDecisions}
}

// GovernanceComponents lists the identity components — every component except
// the position. They are what a status report and a staleness note speak about:
// the position moves in the ordinary course of content traffic and says nothing
// about governance.
func GovernanceComponents() []Component {
	return []Component{ComponentContext, ComponentTerms, ComponentDecisions}
}

// DefaultStream is the stream a ref belongs to when no caller named one. It is
// the protocol's default stream name, spelled here because the framework cannot
// reach the recipe vocabulary that also spells it; the two are asserted equal
// where both are in scope.
const DefaultStream = "main"

// Ref is one stream's freshness ref.
//
// The zero Ref makes no claim about anything: position nought and three empty
// identities. That is what a side which has never synchronized holds, and it is
// distinct from a side that holds nothing — an empty identity compares as
// unknown rather than as different, so a missing ref costs a round trip and
// never a wrong answer.
type Ref struct {
	// Content is the position in the stream's ordered change feed.
	Content int64 `json:"content"`
	// Context identifies the governing context: which collections exist, the
	// point each occupies, and the voice profile governing it.
	Context string `json:"context,omitempty"`
	// Terms identifies the governed terminology in force.
	Terms string `json:"terms,omitempty"`
	// Decisions identifies the committed decision record.
	Decisions string `json:"decisions,omitempty"`
}

// Identity returns the ref's value for an identity component, and the empty
// string for the position component, which has no identity.
func (r Ref) Identity(c Component) string {
	switch c {
	case ComponentContext:
		return r.Context
	case ComponentTerms:
		return r.Terms
	case ComponentDecisions:
		return r.Decisions
	default:
		return ""
	}
}

// WithIdentity returns the ref with one identity component replaced. The
// position component is returned unchanged for ComponentContent, which has no
// identity to set — use Advance for that.
func (r Ref) WithIdentity(c Component, value string) Ref {
	switch c {
	case ComponentContext:
		r.Context = value
	case ComponentTerms:
		r.Terms = value
	case ComponentDecisions:
		r.Decisions = value
	}
	return r
}

// IsZero reports whether the ref makes no claim at all.
func (r Ref) IsZero() bool {
	return r == Ref{}
}

// Advance moves the position component forward to cursor, and does nothing when
// cursor does not lead the one already held.
//
// The monotonicity is load-bearing, not defensive. A position is what a
// resumable transfer resumes from, so a call that would move it backwards is
// asking for content already consumed to be consumed again — and a caller that
// takes its new position from a response which does not carry one (a queued
// write answering before the work lands, say) hands over a nought that means
// "no position in this answer", never "rewind to the beginning".
func (r Ref) Advance(cursor int64) Ref {
	if cursor > r.Content {
		r.Content = cursor
	}
	return r
}

// Status is where one component of a ref sits relative to the same component of
// another.
type Status string

const (
	// StatusUnknown means one side makes no claim about the component, so no
	// comparison is possible. Nothing is wrong; nothing is known either.
	StatusUnknown Status = "unknown"
	// StatusCurrent means both sides hold the same value.
	StatusCurrent Status = "current"
	// StatusBehind means the local position trails the remote one.
	StatusBehind Status = "behind"
	// StatusAhead means the local position leads the remote one.
	StatusAhead Status = "ahead"
	// StatusMoved means two identities differ. It says only that: which side
	// moved, and how, is not a question a hash can answer.
	StatusMoved Status = "moved"
)

// ComponentDiff is one component's standing across two refs.
type ComponentDiff struct {
	Component Component `json:"component"`
	Status    Status    `json:"status"`
	// Local and Remote are the compared values, rendered as strings so a
	// position and an identity report through the same field.
	Local  string `json:"local,omitempty"`
	Remote string `json:"remote,omitempty"`
}

// Divergence is the whole comparison of two refs: the summary a status report
// renders, a staleness gate consults, and a writer reads to decide whether it
// has anything to send.
type Divergence struct {
	Content   ComponentDiff `json:"content"`
	Context   ComponentDiff `json:"context"`
	Terms     ComponentDiff `json:"terms"`
	Decisions ComponentDiff `json:"decisions"`
}

// Compare reports how local stands against remote, component by component.
//
// The position compares by order; the three identities compare by equality. An
// identity either side leaves empty compares as unknown: an empty value is the
// absence of a claim, and reading it as a difference would make every first
// contact look like a conflict.
func Compare(local, remote Ref) Divergence {
	return Divergence{
		Content:   comparePosition(local.Content, remote.Content),
		Context:   compareIdentity(ComponentContext, local.Context, remote.Context),
		Terms:     compareIdentity(ComponentTerms, local.Terms, remote.Terms),
		Decisions: compareIdentity(ComponentDecisions, local.Decisions, remote.Decisions),
	}
}

func comparePosition(local, remote int64) ComponentDiff {
	d := ComponentDiff{
		Component: ComponentContent,
		Local:     strconv.FormatInt(local, 10),
		Remote:    strconv.FormatInt(remote, 10),
	}
	switch {
	case local == remote:
		d.Status = StatusCurrent
	case local < remote:
		d.Status = StatusBehind
	default:
		d.Status = StatusAhead
	}
	return d
}

func compareIdentity(c Component, local, remote string) ComponentDiff {
	d := ComponentDiff{Component: c, Local: local, Remote: remote}
	switch {
	case local == "" || remote == "":
		d.Status = StatusUnknown
	case local == remote:
		d.Status = StatusCurrent
	default:
		d.Status = StatusMoved
	}
	return d
}

// All returns every component's standing in report order.
func (d Divergence) All() []ComponentDiff {
	return []ComponentDiff{d.Content, d.Context, d.Terms, d.Decisions}
}

// Current reports whether nothing diverged: every component is either equal or
// unknown. An unknown component is not a divergence — it is a question neither
// side asked.
func (d Divergence) Current() bool {
	for _, c := range d.All() {
		switch c.Status {
		case StatusCurrent, StatusUnknown:
		default:
			return false
		}
	}
	return true
}

// Moved names the governance components whose identities differ. The position
// is never included: it moves in the ordinary course of content traffic, and
// calling that "moved governance" is exactly the conflation the composite ref
// exists to end.
func (d Divergence) Moved() []Component {
	var out []Component
	for _, c := range d.All() {
		if c.Component != ComponentContent && c.Status == StatusMoved {
			out = append(out, c.Component)
		}
	}
	return out
}

// Conflict is a compare-and-swap failure: a writer asserted the component value
// it last observed, and the component had moved.
type Conflict struct {
	Component Component
	Expected  string
	Actual    string
}

// Error renders the conflict as the instruction it implies. A caller that
// cannot fetch first cannot resolve it, and no retry of the same write will.
func (c *Conflict) Error() string {
	return fmt.Sprintf("%s governance moved since it was last observed (expected %s, found %s). Pull first",
		c.Component, shortIdentity(c.Expected), shortIdentity(c.Actual))
}

// Assert compares the component value a writer last observed against the one in
// force, and returns a *Conflict when they differ.
//
// An empty expectation asserts nothing and always succeeds. That is what keeps
// the compare-and-swap additive: a client that predates the ref sends no
// assertion and writes exactly as it always did, while a client that sends one
// is protected against the writer it could not see.
func Assert(component Component, expected, actual string) error {
	if expected == "" || expected == actual {
		return nil
	}
	return &Conflict{Component: component, Expected: expected, Actual: actual}
}

// Fold folds a set of named identity hashes into one component hash: the names
// in sorted order, each written with its hash, through SHA-256.
//
// It is the one definition of "an identity over a set of identities" the
// freshness layer has. Both sides of a protocol fold with it, over their own
// view of the same set, and compare the results — so a component hash means the
// same thing wherever it was computed, and adding a second fold with a
// different byte layout would silently make two correct sides disagree forever.
func Fold(parts map[string]string) string {
	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	slices.Sort(names)

	h := sha256.New()
	for _, name := range names {
		h.Write([]byte(name))
		h.Write([]byte(parts[name]))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Identity hashes an ordered list of fields into the identity of one member of
// a folded set: the fields in the order given, NUL-separated, through SHA-256.
//
// Order is the caller's to fix, and it is part of the definition — two fields
// swapped is a different identity. The separator is what stops a member whose
// fields are ("ab", "c") from hashing the same as one whose fields are
// ("a", "bc"), which would let a rename in one field pass as no change at all.
func Identity(fields ...string) string {
	h := sha256.New()
	for _, f := range fields {
		h.Write([]byte(f))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// shortIdentity trims a hash for a message. A full SHA-256 in an error line
// buries the sentence that tells the reader what to do about it.
func shortIdentity(s string) string {
	if s == "" {
		return "none"
	}
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
