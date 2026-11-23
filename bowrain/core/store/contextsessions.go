package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/contextop"
)

// A ContextSession is one person's short pass over a project's context deck.
// It holds the cards it dealt for its owner while it runs, so two people
// working on the project at once never decide the same card. A session runs
// until its owner ends it or until ExpiresAt, which each decision moves on.
type ContextSession struct {
	ID        string
	ProjectID string
	UserID    string
	// Cards are the cards the session dealt, in deal order.
	Cards []ContextSessionCard
	// Reach is the blocks each term of the project reached when the session
	// started, by concept id. The next session compares against it to say
	// whose reach grew.
	Reach map[string]int
	// Decisions are the decisions made in the session, oldest first.
	Decisions []ContextSessionDecision
	StartedAt time.Time
	ExpiresAt time.Time
	// EndedAt is zero while the session runs.
	EndedAt time.Time
	// DealCards, when set, gives the cards the session deals from the items
	// it holds. StartContextSession calls it inside the transaction that
	// records the session and its holds, and records what it returns, so a
	// session starts with its holds and its cards or not at all. An error
	// from it fails the start and leaves nothing held.
	DealCards func(held []contextop.ItemRef) ([]ContextSessionCard, error) `json:"-"`
}

// ContextSessionCard is one card a session dealt: the item it names, and what
// the session summary reads about it.
type ContextSessionCard struct {
	Item contextop.ItemRef `json:"item"`
	Kind string            `json:"kind"`
	// Term is the card's term, when it names one.
	Term string `json:"term,omitempty"`
	// Concept is the concept a concept card or a term-translation card
	// belongs to.
	Concept string `json:"concept,omitempty"`
	// Blocks is the card's reach when it was dealt; zero when unknown.
	Blocks int `json:"blocks,omitempty"`
}

// The outcomes a session records for a decision.
const (
	SessionKept     = "kept"
	SessionChanged  = "changed"
	SessionRejected = "rejected"
)

// ContextSessionDecision is one decision made in a session.
type ContextSessionDecision struct {
	Item contextop.ItemRef `json:"item"`
	// Outcome is kept, changed (kept in the reviewer's own wording) or
	// rejected.
	Outcome string    `json:"outcome"`
	At      time.Time `json:"at"`
}

// Live reports whether the session still runs at now.
func (s ContextSession) Live(now time.Time) bool {
	return s.EndedAt.IsZero() && now.Before(s.ExpiresAt)
}

// ContextSessionHold is a card a live session holds.
type ContextSessionHold struct {
	Item      contextop.ItemRef
	SessionID string
	UserID    string
}

// ContextSessionStore holds the context sessions of projects and the cards
// they hold.
type ContextSessionStore interface {
	// StartContextSession records sess and holds for it, in order, each of
	// items no live session of the project holds, until it holds limit of
	// them. It first ends the project's sessions that expired, and releases
	// their holds. Two sessions started at once hold disjoint cards. It
	// returns the items it holds, in the order given.
	StartContextSession(ctx context.Context, sess ContextSession, items []contextop.ItemRef, limit int) ([]contextop.ItemRef, error)
	// SetContextSessionCards records the cards a session dealt.
	SetContextSessionCards(ctx context.Context, id string, cards []ContextSessionCard) error
	// GetContextSession reads one session; false when none has that id.
	GetContextSession(ctx context.Context, id string) (ContextSession, bool, error)
	// ContextSessionHolds lists the cards the project's live sessions hold.
	ContextSessionHolds(ctx context.Context, projectID string, now time.Time) ([]ContextSessionHold, error)
	// RecordContextSessionDecision appends d to the session, releases the
	// session's hold on d.Item and moves its expiry to expires.
	RecordContextSessionDecision(ctx context.Context, id string, d ContextSessionDecision, expires time.Time) error
	// EndContextSession ends a session at at, unless it ended already, and
	// releases its holds.
	EndContextSession(ctx context.Context, id string, at time.Time) error
	// LastEndedContextSession is the project's session that ended last,
	// counting an expired session as ended at its expiry; false when none
	// has.
	LastEndedContextSession(ctx context.Context, projectID string, now time.Time) (ContextSession, bool, error)
	// ContextSessionsSince lists the project's sessions with a decision
	// after since, oldest first.
	ContextSessionsSince(ctx context.Context, projectID string, since time.Time) ([]ContextSession, error)
}

// ContextSessions finds the session store behind a content store, looking
// through any decorator that names what it wraps, as ItemCheckTallies does.
func ContextSessions(cs ContentStore) (ContextSessionStore, bool) {
	for cs != nil {
		if s, ok := cs.(ContextSessionStore); ok {
			return s, true
		}
		inner, ok := cs.(interface{ Unwrap() ContentStore })
		if !ok {
			return nil, false
		}
		cs = inner.Unwrap()
	}
	return nil, false
}

// StorableText reports whether a store can hold s, as text and inside JSON:
// PostgreSQL refuses a NUL byte in both. Every value a session row, its
// holds, its cards and its reach carry passes through this one test.
func StorableText(s string) bool { return !strings.ContainsRune(s, 0) }

// StorableItem reports whether every part of an item reference can be
// stored.
func StorableItem(r contextop.ItemRef) bool {
	return StorableText(string(r.Kind)) && StorableText(r.Profile) && StorableText(r.ID)
}

// StorableCards returns cards as a store can hold them. A card whose item
// cannot be stored is left out, since a session cannot name it; a display
// field (Term, Concept, Kind) that cannot be stored is left empty, since the
// summary reads it and nothing decides by it. left is how many cards were
// left out.
func StorableCards(cards []ContextSessionCard) (out []ContextSessionCard, left int) {
	out = make([]ContextSessionCard, 0, len(cards))
	for _, c := range cards {
		if !StorableItem(c.Item) {
			left++
			continue
		}
		if !StorableText(c.Kind) {
			c.Kind = ""
		}
		if !StorableText(c.Term) {
			c.Term = ""
		}
		if !StorableText(c.Concept) {
			c.Concept = ""
		}
		out = append(out, c)
	}
	return out, left
}

// StorableReach returns reach without the concepts whose id cannot be
// stored.
func StorableReach(reach map[string]int) map[string]int {
	out := make(map[string]int, len(reach))
	for k, v := range reach {
		if StorableText(k) {
			out[k] = v
		}
	}
	return out
}

// EncodeSessionJSON encodes a session field for a store, as "[]" or "{}" when
// v is empty.
func EncodeSessionJSON(v any, empty string) string {
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return empty
	}
	return string(b)
}
