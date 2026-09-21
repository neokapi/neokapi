package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// A unit decision is an entry in an append-only ledger, addressed by what it
// says rather than by where it sits.
//
// An entry is keyed by the unit AND by the pairing it blessed: the source hash
// and the target hash together with (document, unit, variant). A decision
// therefore applies exactly where its pairing appears. Two branches holding
// different translations of one unit hold two entries, each answering only for
// the branch whose files carry its pairing, and a branch switch changes which
// entry applies while moving nothing.
//
// Addressing an entry by its content is what makes recording idempotent. The
// committed shards, a venue pull and a re-import of either all reach the same
// address for the same decision, so a project may import the same record any
// number of times and hold it once.

// EntryOrigin says how an entry reached the ledger. It is metadata about
// arrival, so it sits outside the content address: a decision that arrives by
// two routes is one entry, stamped with the route that got there first.
type EntryOrigin string

const (
	// OriginLocal is a decision recorded in this checkout: a person in a review
	// pane, an agent through the MCP tools, `kapi apply`.
	OriginLocal EntryOrigin = "local"
	// OriginRun is a basis a convergence pass recorded for its own output.
	OriginRun EntryOrigin = "run"
	// OriginImport is a line read from the committed shards.
	OriginImport EntryOrigin = "import"
	// OriginVenue is a decision pulled from a connected venue, or the record a
	// venue reported back for one it refused.
	OriginVenue EntryOrigin = "venue"
)

// Pairing is what an entry is about: the unit, the source it blessed and the
// translation it blessed. It is the ledger's key.
type Pairing struct {
	Key         Key
	ContentHash string
	TargetHash  string
}

// Pairing returns the pairing a record describes.
func (s UnitState) Pairing() Pairing {
	return Pairing{Key: s.Key(), ContentHash: s.ContentHash, TargetHash: s.TargetHash}
}

// Entry is one immutable record in the decision ledger.
type Entry struct {
	// ID is the content address: the SHA-256 of everything the entry asserts.
	// Two parties that reach the same decision about the same pairing write the
	// same ID.
	ID string
	// State is the record as it was decided, carrying the unit's identity, the
	// pairing, the ladder positions, the decision and the governing context.
	State UnitState
	// Actor is who reached it: empty for a plain human decision in a
	// single-player project, "ai/<model>" for an autonomous AI approval,
	// "agent/<client>" for an agent acting on a person's behalf, and whatever
	// identity a venue reports for a decision made there.
	Actor string
	// Origin says how the entry reached the ledger.
	Origin EntryOrigin
	// Recorded is Go's clock at the moment the entry was written, and it orders
	// entries that share a pairing so the most recent one answers for it.
	//
	// It is the order things happened in this store, which is the order a
	// reader wants: the venue's answer about a decision this project sent it
	// arrives after that decision and settles it, and an approval recorded
	// after the run that drafted the unit settles that. What a record carried
	// in from elsewhere claims about its own age is read separately, at the
	// point it arrives.
	Recorded time.Time
	// Revoked marks the entry as a withdrawal: the pairing it names goes back
	// to having no decision on it. A withdrawal is an entry of its own, so the
	// ledger never rewrites what it already holds.
	Revoked bool
}

// Address is the entry's content address: the SHA-256 over the record, the
// actor and the revocation flag.
//
// The timestamp and the origin are deliberately outside it. Re-importing a
// shard line, pulling the same venue decision twice and replaying a change feed
// would each otherwise write a fresh entry for a decision the ledger already
// holds, and the ledger would grow with every no-op run.
func Address(state UnitState, actor string, revoked bool) (string, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("state: address entry: %w", err)
	}
	sum := sha256.New()
	sum.Write(payload)
	fmt.Fprintf(sum, "\x00%s\x00%t", actor, revoked)
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// Transition is one proposed entry, beside whatever applies at its pairing now.
type Transition struct {
	// Pairing is the unit and the content the proposed entry is about.
	Pairing Pairing
	// Actor is who is recording it, and Origin how it reached this store.
	Actor  string
	Origin EntryOrigin
	// Applies is the record in force at the pairing, and Applied reports
	// whether there was one.
	Applies UnitState
	Applied bool
	// Proposed is the record about to be written, and Revoke reports that the
	// entry withdraws rather than records.
	Proposed UnitState
	Revoke   bool
}

// Policy answers whether an actor may record a transition. Returning an error
// refuses it, and the error reaches the caller that tried to record.
//
// One function decides this for every writer: the CLI, the desktop review pane,
// an agent through the MCP tools, and the venue reconciliation that writes what
// a server reported. Actor and origin ride on every entry so the decision has
// what it needs, and an actor class with narrower or wider rights is a change
// here rather than at each call site.
type Policy func(Transition) error

// AllowAny records every transition, whoever proposes it. It is what a
// single-player project and a connected venue both do: the venue enforces its
// own permissions on its side and reports what it refused, and a checkout's
// record is whatever the person holding the checkout decided.
func AllowAny(Transition) error { return nil }

// Supersedes reports whether a record arriving from elsewhere is newer than
// one already in force, by the `Updated` stamp both ends write.
//
// It is the reconciliation rule for anything coming in: a line read from the
// committed shards, a decision pulled from a venue. An older or identical
// record is left where it is, which is what makes reading a record in
// idempotent, and what stops a colleague's earlier line displacing a decision
// made here since. A record that says nothing about its age is taken as new,
// because something that came in is at least as recent as nothing at all.
func Supersedes(arriving, inForce UnitState) bool {
	if arriving.Updated == "" || inForce.Updated == "" {
		return true
	}
	return arriving.Updated > inForce.Updated
}

// entryTimeText is how an instant is stored and compared. RFC 3339 with
// nanoseconds in UTC sorts lexically in time order, which is what the ledger's
// ORDER BY relies on.
const entryTimeLayout = "2006-01-02T15:04:05.000000000Z"

func entryTimeText(t time.Time) string { return t.UTC().Format(entryTimeLayout) }
