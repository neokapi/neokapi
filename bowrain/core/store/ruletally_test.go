package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRuleTallies_MergeByKeyAndRankByCount(t *testing.T) {
	var r RuleTallies
	r.Add(RuleTally{Rule: "qa.trailing-whitespace", Message: "Trailing whitespace differs", Count: 1})
	r.Add(RuleTally{Rule: "terms.vocabulary", Subject: "utilize", Message: "Use \"use\"", Count: 1})
	r.Add(RuleTally{Rule: "terms.vocabulary", Subject: "utilize", ConceptID: "c-use", Fails: true, Count: 2})
	r.Add(RuleTally{Rule: "terms.vocabulary", Subject: "leverage", Count: 1})
	r.Add(RuleTally{Rule: "", Count: 4})
	r.Add(RuleTally{Rule: "voice.style", Count: 0})

	assert.Equal(t, 3, r.Len(), "a tally with no rule or no count is not kept")
	assert.Equal(t, []RuleTally{
		{Key: "terms.vocabulary:utilize", Rule: "terms.vocabulary", Subject: "utilize", Message: "Use \"use\"", ConceptID: "c-use", Fails: true, Count: 3},
		{Key: "qa.trailing-whitespace", Rule: "qa.trailing-whitespace", Message: "Trailing whitespace differs", Count: 1},
	}, r.Top(2), "the most findings first, then by key")
	assert.Len(t, r.Top(0), 3)
}

func TestRuleTallies_EncodeRoundTrips(t *testing.T) {
	assert.Equal(t, "[]", EncodeRuleTallies(nil))
	assert.Nil(t, DecodeRuleTallies([]byte("[]")))
	assert.Nil(t, DecodeRuleTallies([]byte("not json")))
	in := []RuleTally{{Key: "voice.style:!", Rule: "voice.style", Subject: "!", Count: 2}}
	assert.Equal(t, in, DecodeRuleTallies([]byte(EncodeRuleTallies(in))))
}
