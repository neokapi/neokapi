package store

import (
	"context"
	"time"
)

// A review deck is the pending-review queue of one language, grouped by the
// attention each translation needs. The group of each (block, locale) is
// computed once and kept as ReviewDeckFacts, so a deck page is read, ordered
// and counted in SQL and only the page's blocks are read whole.

// ReviewDeckQuery scopes a review deck: one stream, one language, optionally
// one collection, under one policy.
type ReviewDeckQuery struct {
	ProjectID string
	Stream    string
	Locale    string
	// CollectionID narrows the deck as PendingReviewQuery.CollectionID does.
	CollectionID *string
	// Policy names the thresholds the facts were computed under. Facts
	// computed under another policy are classified again.
	Policy string
}

// ReviewDeckBasis is what a translation's facts are computed from, as the
// stored rows hold it. Facts stand while every value still holds.
type ReviewDeckBasis struct {
	TargetUpdatedAt time.Time
	SourceRevision  string
	PreReviewAt     *time.Time
	DecisionAt      *time.Time
}

// ReviewDeckEntry is one pending translation whose facts are missing or no
// longer hold, with what its classification reads from the decision ledger.
type ReviewDeckEntry struct {
	PendingReviewRef
	Basis ReviewDeckBasis
	// DecisionState, DecisionBasis and DecisionRevision are the ledger row's
	// review state, the source revision it blessed and the translation
	// revision it judged; empty without a row.
	DecisionState    string
	DecisionBasis    string
	DecisionRevision string
}

// ReviewDeckFacts is one translation's place in the deck.
type ReviewDeckFacts struct {
	BlockID string
	Locale  string
	Basis   ReviewDeckBasis
	Policy  string
	// GroupRank orders the groups; Group and Reason are what the deck shows.
	GroupRank      int
	Group          string
	Reason         string
	Findings       int
	Failing        int
	TermCompliance TermCompliance
	VoiceScore     *int
	VoiceBar       *int
}

// ReviewDeckKey is a position in a deck: the keyset its pages are cut on.
type ReviewDeckKey struct {
	GroupRank int
	ItemName  string
	Position  int
	BlockID   string
}

// ReviewDeckRow is one card of a deck page, without the block's content.
type ReviewDeckRow struct {
	PendingReviewRef
	Position int
	Facts    ReviewDeckFacts
}

// ReviewDeckCounts counts a deck: the classified translations per group rank,
// and those whose facts are missing or no longer hold.
type ReviewDeckCounts struct {
	ByRank       map[int]int
	Unclassified int
}

// ReviewDeckStore keeps the review deck's facts. The PostgreSQL store
// implements it.
type ReviewDeckStore interface {
	// UnclassifiedReview returns up to limit pending translations whose facts
	// are missing or no longer hold, in queue order.
	UnclassifiedReview(ctx context.Context, q ReviewDeckQuery, limit int) ([]ReviewDeckEntry, error)
	// RecordReviewDeckFacts stores facts in place of any held for the same
	// translations.
	RecordReviewDeckFacts(ctx context.Context, projectID, stream string, facts []ReviewDeckFacts) error
	// ListReviewDeck returns up to limit classified translations after the
	// key, ordered by group, item, position and block. A group rank below 0
	// reads every group.
	ListReviewDeck(ctx context.Context, q ReviewDeckQuery, groupRank int, after *ReviewDeckKey, limit int) ([]ReviewDeckRow, error)
	// CountReviewDeck counts the deck by group, and what is not classified.
	CountReviewDeck(ctx context.Context, q ReviewDeckQuery) (ReviewDeckCounts, error)
}

// ReviewDeckStores finds the review deck store behind a content store,
// looking through any decorator that names what it wraps, as ContextObjects
// does. The server's content store is wrapped for event emission, so a plain
// type assertion on it never finds the deck.
func ReviewDeckStores(cs ContentStore) (ReviewDeckStore, bool) {
	for cs != nil {
		if s, ok := cs.(ReviewDeckStore); ok {
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
