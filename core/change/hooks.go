package change

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
)

// The service calls three hooks a host supplies. Each is optional: a nil
// CommitCheck checks nothing, a nil Policy permits every operation, and a nil
// Recorder records nothing. core/change defines them and knows nothing about
// projects; kapi's host and the Bowrain server implement them.

// EditionChange is one edition a change set changed, as the commit check and
// the recorder see it. Runs are read-only.
type EditionChange struct {
	Ref  Ref
	Role Role
	// Key is the block's durable key where reconciliation assigned one.
	Key string
	// Before and After are the edition's runs around the change; Before is
	// nil when the change set created the edition, and After is nil when it
	// removed it.
	Before []model.Run
	After  []model.Run
	// BeforeRev and AfterRev are the edition revisions around the change.
	// AfterRev is model.AbsentRevision when the change set removed the
	// edition, by remove_edition or by deleting its block, and BeforeRev when
	// it created one.
	BeforeRev string
	AfterRev  string
	// Basis is the authoritative edition's revision a derived edition was
	// made from, when the change set recorded one.
	Basis string
	// Block is the whole block after the change, for checks that need the
	// other editions (a term rule reads the source beside its target).
	Block *model.Block
}

// CheckOutcome is what a commit check found on one edition, before and after
// the change.
type CheckOutcome struct {
	Before []Finding
	After  []Finding
}

// CommitCheck runs the deterministic rules that govern the changed editions
// before anything is written. It returns one outcome per change, in order,
// and the governance fingerprint it resolved, which the record stores.
type CommitCheck interface {
	Check(ctx context.Context, changes []EditionChange) (outcomes []CheckOutcome, fingerprint string, err error)
}

// Introduced returns the failing findings an outcome has after the change and
// did not have before. The service refuses a change only for these; a
// violation the edition already had is reported and not held against it.
// Findings are matched by rule and message.
func Introduced(o CheckOutcome) []Finding {
	type key struct{ rule, message string }
	had := make(map[key]int, len(o.Before))
	for _, f := range o.Before {
		if f.Fails {
			had[key{f.Rule, f.Message}]++
		}
	}
	var out []Finding
	for _, f := range o.After {
		if !f.Fails {
			continue
		}
		k := key{f.Rule, f.Message}
		if had[k] > 0 {
			had[k]--
			continue
		}
		out = append(out, f)
	}
	return out
}

// Policy decides what an actor may send. It returns nil to permit op, or an
// *Error with CodeNotPermitted that names the rule.
type Policy interface {
	Permit(actor Actor, set *Set, op Op) *Error
}

// Transition is one edition change as the record keeps it.
type Transition struct {
	EditionChange
	// ContentHash and ContextHash are the identity signals reconciliation
	// uses to re-attach history after a reorder.
	ContentHash string
	ContextHash string
}

// Record is what the service hands the recorder after the homes committed.
type Record struct {
	Actor Actor
	// Origin says which surface applied the change: apply, desktop,
	// flow:<name>, merge, pull or observed.
	Origin string
	// Set is the change set as sent; nil for a flow's writes.
	Set *Set
	// Fingerprint is the governance the commit check used.
	Fingerprint string
	// Overridden are the findings a person chose to land with gate: report.
	Overridden  []Finding
	Docs        []DocResult
	Transitions []Transition
}

// Recorder records an applied change set and returns the record's id, which
// the result carries. A recorder that fails after the homes committed leaves
// the content written; the next read records the transition as observed.
type Recorder interface {
	Record(ctx context.Context, rec Record) (id string, err error)
}
