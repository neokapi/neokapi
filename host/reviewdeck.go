package host

import "fmt"

// A review deck orders the translations waiting for review by how much
// attention each needs: the cards a reviewer can judge at a glance, the ones
// to read closer, and the ones that need a desk. The rule lives here so the
// Bowrain server, its web reviewer and the mobile app sort a queue the same
// way.

// DeckGroup is where a card sits in a review deck.
type DeckGroup string

const (
	// DeckFast is a card with no finding and a high pre-review score.
	DeckFast DeckGroup = "fast"
	// DeckCloser is a card to read closer: a low or missing score, or a
	// finding that does not fail.
	DeckCloser DeckGroup = "closer"
	// DeckDesk is a card that needs the desk: a failing finding, a stale
	// approval, a redraft after a rejection, or a long source.
	DeckDesk DeckGroup = "desk"
)

// DeckGroups lists the groups in deck order.
var DeckGroups = []DeckGroup{DeckFast, DeckCloser, DeckDesk}

// DeckPolicy holds the deck's thresholds. The zero value is the default
// policy (DefaultDeckPolicy).
type DeckPolicy struct {
	// LongSourceChars is the source length, in characters, above which a
	// card goes to the desk.
	LongSourceChars int `json:"long_source_chars"`
	// FastMinScore is the lowest pre-review score a fast card holds.
	FastMinScore int `json:"fast_min_score"`
}

// DefaultDeckPolicy is the policy a deck uses where none is configured.
var DefaultDeckPolicy = DeckPolicy{LongSourceChars: 280, FastMinScore: 80}

// WithDefaults fills each unset threshold from DefaultDeckPolicy.
func (p DeckPolicy) WithDefaults() DeckPolicy {
	if p.LongSourceChars <= 0 {
		p.LongSourceChars = DefaultDeckPolicy.LongSourceChars
	}
	if p.FastMinScore <= 0 {
		p.FastMinScore = DefaultDeckPolicy.FastMinScore
	}
	return p
}

// DeckFacts are what the deck rule reads about one translation.
type DeckFacts struct {
	// Findings counts every finding on the translation; FailingFindings
	// counts those that fail a check.
	Findings        int
	FailingFindings int
	// Score is the pre-review score while it judges the translation as it
	// stands, nil without one.
	Score *int
	// Stale is an approval whose source has moved since.
	Stale bool
	// RejectedRedraft is a translation drafted again after a rejection.
	RejectedRedraft bool
	// SourceChars is the length of the source, in characters.
	SourceChars int
}

// ClassifyDeckCard places one translation in a deck group and says why.
//
// The desk takes a failing finding, a stale approval, a redraft after a
// rejection, and a source longer than the policy's limit, in that order. A
// card with no finding and a score at or above the policy's bar is fast.
// Every other card is read closer.
func ClassifyDeckCard(f DeckFacts, p DeckPolicy) (DeckGroup, string) {
	p = p.WithDefaults()
	switch {
	case f.FailingFindings > 0:
		return DeckDesk, plural(f.FailingFindings, "failing finding", "failing findings")
	case f.Stale:
		return DeckDesk, "the source changed after the translation was approved"
	case f.RejectedRedraft:
		return DeckDesk, "a new draft after a rejection"
	case f.SourceChars > p.LongSourceChars:
		return DeckDesk, fmt.Sprintf("a long source (%d characters)", f.SourceChars)
	case f.Findings > 0:
		return DeckCloser, plural(f.Findings, "finding", "findings")
	case f.Score == nil:
		return DeckCloser, "no pre-review score"
	case *f.Score < p.FastMinScore:
		return DeckCloser, fmt.Sprintf("pre-review score %d", *f.Score)
	}
	return DeckFast, fmt.Sprintf("no findings, pre-review score %d", *f.Score)
}
