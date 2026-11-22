package reviewdeck

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeckReasonOfNamesEveryClassification(t *testing.T) {
	score := func(n int) *int { return &n }
	cases := []struct {
		facts DeckFacts
		want  DeckReason
	}{
		{DeckFacts{Rejected: true}, DeckReasonRejected},
		{DeckFacts{FailingFindings: 1, Findings: 1}, DeckReasonFailingFindings},
		{DeckFacts{FailingFindings: 2, Findings: 2}, DeckReasonFailingFindings},
		{DeckFacts{Stale: true}, DeckReasonStaleSource},
		{DeckFacts{RejectedRedraft: true}, DeckReasonRedraft},
		{DeckFacts{SourceChars: 900}, DeckReasonLongSource},
		{DeckFacts{Findings: 1}, DeckReasonFindings},
		{DeckFacts{Findings: 3}, DeckReasonFindings},
		{DeckFacts{}, DeckReasonNoPreReview},
		{DeckFacts{Score: score(40)}, DeckReasonLowScore},
		{DeckFacts{Score: score(95)}, DeckReasonFast},
	}
	for _, tc := range cases {
		_, reason := ClassifyDeckCard(tc.facts, DeckPolicy{})
		assert.Equal(t, tc.want, DeckReasonOf(reason), reason)
	}
	assert.Empty(t, DeckReasonOf("something else"))
}
