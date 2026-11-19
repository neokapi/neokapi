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
	// ExcludeCollections leaves out the items of these collections: the
	// source-only ones, whose content has no target language.
	ExcludeCollections []string
	// HighCollections and LowCollections are dealt first and last: the
	// collections whose review priority is high or low. The rest are normal.
	HighCollections []string
	LowCollections  []string
	// Policy names the thresholds the facts were computed under. Facts
	// computed under another policy are classified again.
	Policy string
	// Rule narrows the deck to the translations whose facts count a finding
	// of one rule (RuleTally.Key). Translations without facts are left out
	// of the pages and counted as unclassified, so a caller classifies with
	// Rule empty.
	Rule string
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
	// A rank below 0 sets the translation aside: it is in neither the deck's
	// pages nor its counts while the facts hold.
	GroupRank      int
	Group          string
	Reason         string
	Findings       int
	Failing        int
	TermCompliance TermCompliance
	VoiceScore     *int
	VoiceBar       *int
	// Rules counts the findings by the rule that raised them, at most
	// RuleTallyLimit rules.
	Rules []RuleTally
}

// ReviewDeckKey is a position in a deck: the keyset its pages are cut on.
type ReviewDeckKey struct {
	// Priority is the collection's tier: 0 high, 1 normal, 2 low.
	Priority  int
	GroupRank int
	ItemName  string
	Position  int
	BlockID   string
}

// ReviewDeckRow is one card of a deck page, without the block's content.
type ReviewDeckRow struct {
	PendingReviewRef
	Position int
	// Priority is the tier of the row's collection: 0 high, 1 normal, 2 low.
	Priority int
	Facts    ReviewDeckFacts
}

// ReviewDeckCounts counts a deck: the classified translations per group rank,
// and those whose facts are missing or no longer hold.
type ReviewDeckCounts struct {
	ByRank       map[int]int
	Unclassified int
}

// ReviewDeckCollectionCounts counts the deck of one collection, beside every
// translation of the language the collection holds: the ones waiting, the
// ones approved and the ones set aside.
type ReviewDeckCollectionCounts struct {
	ReviewDeckCounts
	Translations int
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
	// key, ordered by priority, group, item, position and block. Of the
	// translations with the same source and the same translation it returns
	// the first in that order only; ReviewDeckCopies reads the others. A group rank below 0
	// reads every group of the deck; a translation set aside is never read.
	ListReviewDeck(ctx context.Context, q ReviewDeckQuery, groupRank int, after *ReviewDeckKey, limit int) ([]ReviewDeckRow, error)
	// CountReviewDeck counts the deck by group, and what is not classified.
	// A translation set aside is not counted.
	CountReviewDeck(ctx context.Context, q ReviewDeckQuery) (ReviewDeckCounts, error)
	// CountReviewDeckByCollection counts the deck as CountReviewDeck does,
	// once per collection, keyed by collection id ("" for blocks in none).
	// A collection with translations and nothing waiting is present.
	CountReviewDeckByCollection(ctx context.Context, q ReviewDeckQuery) (map[string]ReviewDeckCollectionCounts, error)
	// CountReviewDeckLanguage counts the whole language as
	// CountReviewDeckByCollection counts one collection, so a set of copies
	// in several collections counts once.
	CountReviewDeckLanguage(ctx context.Context, q ReviewDeckQuery) (ReviewDeckCollectionCounts, error)
	// CountReviewDeckRules counts, over the classified translations of one
	// group rank, the findings of each rule, and returns up to limit rules,
	// the most findings first. A set of copies counts once, as
	// CountReviewDeck counts it.
	CountReviewDeckRules(ctx context.Context, q ReviewDeckQuery, groupRank, limit int) ([]RuleTally, error)
	// ReviewDeckCopies returns, for each of blockIDs, the other translations
	// waiting for review in the language with the same source and the same
	// translation, up to limit in all, keyed by the block they copy. It
	// honours q.ExcludeCollections and reads every collection otherwise.
	ReviewDeckCopies(ctx context.Context, q ReviewDeckQuery, blockIDs []string, limit int) (map[string][]PendingReviewRef, error)
	// ListReviewDeckPages returns up to limit pages of the deck, each with
	// its waiting count: a page is a component's strings, or an item's
	// blocks that belong to no component. Pages are ordered by priority, item
	// and the position of their first waiting translation. A translation set
	// aside is not counted, and each copy counts on its own page.
	ListReviewDeckPages(ctx context.Context, q ReviewDeckQuery, limit int) ([]ReviewDeckPage, error)
	// NextReviewInItem returns the first translation waiting in the item
	// after the block, in document order, or "" when none waits.
	NextReviewInItem(ctx context.Context, q ReviewDeckQuery, itemName, afterBlockID string) (string, error)
}

// ReviewDeckPage is a part of the deck a reviewer reads whole: the strings
// of one component, or the blocks of one item that belong to no component.
type ReviewDeckPage struct {
	ItemName     string
	CollectionID string
	// Component names the component; empty for an item's other blocks.
	Component string
	// Priority is the tier of the page's collection: 0 high, 1 normal, 2 low.
	Priority int
	// Waiting counts the page's translations waiting for review.
	Waiting int
	// Findings counts the waiting translations with a failing finding.
	Findings int
	// FirstBlockID is the page's first waiting translation in document order.
	FirstBlockID string
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
