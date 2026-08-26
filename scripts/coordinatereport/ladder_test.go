package main

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/edit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTheLadderLabelsAgreeWithTheClassifier.
//
// The ladder's `kind` column is a hand label: what a reader would call each
// edit. It is beside the classifier's verdict on the same page, and a reader
// takes their agreement as the finding. So the labels have to be checked, not
// trusted — two of them were wrong when this table was first written, both
// calling a possessive cosmetic, and the page would have taught the opposite of
// what it measures.
func TestTheLadderLabelsAgreeWithTheClassifier(t *testing.T) {
	for _, e := range ladderEdits {
		assert.Equal(t, e.kind, string(edit.Classify(ladderOriginal, e.text)),
			"%q is labelled %s", e.label, e.kind)
	}
}

// TestEveryUnsafeFillIsExplained: a divergence with no harm beside it is a
// table cell saying "wrong" with nothing behind it. If the two policies part
// company, the page has to say what a reader would have seen.
func TestEveryUnsafeFillIsExplained(t *testing.T) {
	ladder, err := buildEditLadder(t.Context())
	require.NoError(t, err)

	for _, r := range ladder.Rungs {
		if r.ByScore != "" && !r.SafeToFill {
			assert.NotEmpty(t, r.Harm, "%q fills wrongly and does not say what that costs", r.Edit)
		}
	}
}

// TestTheLadderShowsAPercentageGettingItWrong is the finding, asserted.
//
// If a percentage ever agrees with the classifier on every rung, the argument
// for classifying has evaporated and should be re-made rather than quietly kept.
func TestTheLadderShowsAPercentageGettingItWrong(t *testing.T) {
	ladder, err := buildEditLadder(t.Context())
	require.NoError(t, err)

	assert.Positive(t, ladder.WrongFills,
		"a percentage must be seen filling something the classifier refuses, or the page argues nothing")

	// And the shape of the failure: the wrong fills outscore edits the same
	// policy refuses. That is what makes it a ranking problem — no floor sorts
	// these correctly, so tuning one cannot help.
	var worstWrong, bestRefused int
	for _, r := range ladder.Rungs {
		if r.ByScore != "" && !r.SafeToFill && r.Score > worstWrong {
			worstWrong = r.Score
		}
		if r.ByScore == "" && r.Score > bestRefused {
			bestRefused = r.Score
		}
	}
	assert.Greater(t, worstWrong, bestRefused,
		"the dangerous edits score higher than the refused ones, which is the whole point")
}

// TestTheDiffShownIsTheComparisonMade: the highlighting on the page has to be
// the classifier's own view, or a reader is checking one thing and being shown
// another.
func TestTheDiffShownIsTheComparisonMade(t *testing.T) {
	ladder, err := buildEditLadder(t.Context())
	require.NoError(t, err)

	for _, r := range ladder.Rungs {
		assert.Equal(t, r.Classified, string(r.Diff.Kind), "%q", r.Edit)

		var prior, current strings.Builder
		for _, s := range r.Diff.Prior {
			prior.WriteString(s.Text)
		}
		for _, s := range r.Diff.Current {
			current.WriteString(s.Text)
		}
		assert.Equal(t, ladderOriginal, prior.String(), "%q: the diff must render the original", r.Edit)
		assert.Equal(t, r.Text, current.String(), "%q: and the edit as made", r.Edit)
	}
}

// TestSegmentingFindsReuseTheBlockCannotSee is the measurement behind the
// second half of the argument: a score is one number for a whole paragraph, so
// one moved sentence condemns the two beside it.
func TestSegmentingFindsReuseTheBlockCannotSee(t *testing.T) {
	split, err := buildSegmentSplit(t.Context())
	require.NoError(t, err)

	require.Len(t, split.Segments, 3, "the paragraph has three sentences")
	assert.Empty(t, split.BlockFilled, "one sentence moved, so the paragraph as a whole is not reusable")
	assert.Positive(t, split.Reusable, "and yet most of it is")
	assert.Positive(t, split.Moved, "with the moved sentence pin-pointed")

	// The paragraph still scores high enough for a percentage to fill it, which
	// is the same ranking failure at a different grain: the sentence that moved
	// is a small fraction of the characters and all of the meaning.
	assert.NotEmpty(t, split.BlockFilledByScore,
		"a fill floor writes the old paragraph back out")
	assert.GreaterOrEqual(t, split.BlockScore, ladderFillFloor)
}

// TestSegmentsAreWholeSentences guards a class of bug that renders as content
// and reads as nonsense.
//
// The segmenter answers in RUNE offsets, and slicing a Go string by a rune count
// shifts by a byte for every character above ASCII. English survives it; the
// first Norwegian to reach this page came out as "r. Du kan si opp når som helst
// fra fakturasi" — a sentence starting mid-word and stopping mid-word, published
// as a measurement.
func TestSegmentsAreWholeSentences(t *testing.T) {
	split, err := buildSegmentSplit(t.Context())
	require.NoError(t, err)

	for _, s := range split.Segments {
		assert.Contains(t, split.Prior, s.Prior, "sentence %d is not a piece of the paragraph", s.Index)
		assert.Contains(t, split.Current, s.Current, "sentence %d is not a piece of the edit", s.Index)
		if s.Filled != "" {
			assert.Contains(t, split.Approved, s.Filled,
				"sentence %d was filled with something the approved paragraph does not contain", s.Index)
		}
	}
}
