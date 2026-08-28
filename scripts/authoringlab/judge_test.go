package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The panel's controls are the point of it, so these are about the bookkeeping
// that makes them readable rather than about the judging.

// TestSwappingTranslatesTheVerdictBack.
//
// Each pair is judged twice with the order reversed, because a judge that
// always picks the first document is measuring position. That only works if
// "the judge chose A" maps back to the right arm in both orders — get this
// wrong and a perfect position bias reads as a perfect preference.
func TestSwappingTranslatesTheVerdictBack(t *testing.T) {
	p := Pair{Kind: "real", A: "bare text", B: "governed text", AIs: "bare", BIs: "governed"}

	// Unswapped, the judge sees bare first.
	v := Verdict{}
	first, firstIs := p.A, p.AIs
	assert.Equal(t, "bare text", first)
	assert.Equal(t, "bare", firstIs)

	// Swapped, it sees governed first, and choosing "A" must mean governed.
	first, firstIs = p.B, p.BIs
	assert.Equal(t, "governed text", first)
	assert.Equal(t, "governed", firstIs)
	_ = v
}

// TestSummariseCountsPositionAgreement: a pair judged both ways contributes one
// agreement, and only when both orders named the same arm.
func TestSummariseCountsPositionAgreement(t *testing.T) {
	got := summarise([]Verdict{
		// Agrees with itself: governed both ways.
		{Lens: "voice", Model: "m", Audience: "end-user", Kind: "real", Chose: "B", Won: "governed"},
		{Lens: "voice", Model: "m", Audience: "end-user", Kind: "real", Swapped: true, Chose: "A", Won: "governed"},
		// Disagrees: picked whichever came first, which is position bias.
		{Lens: "voice", Model: "m", Audience: "developer", Kind: "real", Chose: "A", Won: "bare"},
		{Lens: "voice", Model: "m", Audience: "developer", Kind: "real", Swapped: true, Chose: "A", Won: "governed"},
	})

	agree := got.PositionAgreement["voice"]
	assert.Equal(t, 1, agree[0], "one pair agreed with itself")
	assert.Equal(t, 2, agree[1], "out of two pairs judged both ways")
}

// TestSummariseKeepsNullPairsOutOfThePreference.
//
// A null pair is two documents from the same arm. Counting it as a preference
// would let the control inflate the number it exists to check.
func TestSummariseKeepsNullPairsOutOfThePreference(t *testing.T) {
	got := summarise([]Verdict{
		{Lens: "audience", Kind: "real", Chose: "B", Won: "governed"},
		{Lens: "audience", Kind: "null", Chose: "A", Won: "bare"},
		{Lens: "audience", Kind: "null", Chose: "B", Won: "bare"},
	})

	assert.Equal(t, 1, got.Preference["audience"]["governed"])
	assert.Equal(t, 0, got.Preference["audience"]["bare"], "the null pairs are not preferences")
	assert.Equal(t, [2]int{1, 1}, got.NullSplit["audience"], "they are a split, and this one is even")
}

// TestSummariseWithholdsTheAggregate.
//
// Two reasons stack here and the test asserts the stronger one. The repository
// already withholds a judged dimension until its agreement with a person is
// measured; on top of that, this panel's own controls came back at chance over
// eight pairs, with one lens reversing direction between runs on identical
// documents. The message has to say which, because "we are being careful" and
// "the numbers cannot separate a lens from a coin" are different claims.
func TestSummariseWithholdsTheAggregate(t *testing.T) {
	got := summarise(nil)
	assert.NotEmpty(t, got.Withheld)
	assert.Contains(t, got.Withheld, "controls",
		"it says the controls are the reason, not caution")
	assert.Contains(t, got.Withheld, "chance")
}

// TestEveryLensAsksSomethingDifferent.
//
// Three copies of one question is one judge with extra cost: identical judges
// share their biases, so agreement between them measures sampling noise. The
// panel is worth having only if the questions differ.
func TestEveryLensAsksSomethingDifferent(t *testing.T) {
	require.Len(t, lenses, 3)
	seen := map[string]bool{}
	for _, l := range lenses {
		assert.NotEmpty(t, l.Why, "%s: says what it catches that the others do not", l.ID)
		assert.False(t, seen[l.Question], "%s asks the same question as another lens", l.ID)
		seen[l.Question] = true

		// Blinding: a question naming the mechanism tells the judge what to
		// look for, and it would find it.
		for _, leak := range []string{"governed", "voice profile", "coordinate", "guide", "bare"} {
			assert.NotContains(t, l.Question, leak,
				"%s names the mechanism, so the judge is no longer blind", l.ID)
		}
	}
}
