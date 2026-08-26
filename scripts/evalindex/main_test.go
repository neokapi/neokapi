package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The registry is authored, so it can say anything. These tests are what stop
// it: every claim it makes about the repository is checked against the
// repository, and the ones it makes about itself are checked for coherence.
//
// A page whose job is evidence has to hold its own index to the standard it
// holds the evals to. A card claiming an eval is measured, pointing at a data
// file that does not exist, would be worse than no page at all.

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	return root
}

func TestCommittedIndexIsCurrent(t *testing.T) {
	index, err := Build()
	require.NoError(t, err)
	fresh, err := Marshal(index)
	require.NoError(t, err)

	committed, err := os.ReadFile(filepath.Join(repoRoot(t), DefaultOut))
	require.NoError(t, err, "the index is missing — run: go run ./scripts/evalindex")

	assert.Equal(t, string(committed), string(fresh),
		"the committed index is stale — regenerate with: go run ./scripts/evalindex")
}

// TestAMeasuredEvalHasSomethingToShow: a card claiming a measurement must point
// at the measurement. Whether the command and the page resolve is checked in
// resolve_test.go; this is the assertion that they were named at all.
func TestAMeasuredEvalHasSomethingToShow(t *testing.T) {
	root := repoRoot(t)

	for _, e := range evals {
		if e.Status == StatusAbsent {
			continue
		}
		t.Run(e.ID, func(t *testing.T) {
			assert.NotEmpty(t, e.Reproduce, "a number nobody can reproduce is an assertion with a table")

			if e.Data == "" {
				// Legitimate: some evals run live in the reader's browser, and
				// some publish only into a local sandbox. The card has to say
				// which — TestAnUnpublishedEvalSaysSo holds it to that.
				return
			}
			_, err := os.Stat(filepath.Join(root, e.Data))
			assert.NoError(t, err, "card names data at %s, which does not exist", e.Data)
		})
	}
}

// TestAnAbsentEvalPromisesNothing: an unbuilt eval must not carry a reproduce
// command or a page, because both would be false. What it must carry is the
// shape of what is missing.
func TestAnAbsentEvalPromisesNothing(t *testing.T) {
	for _, e := range evals {
		if e.Status != StatusAbsent {
			continue
		}
		t.Run(e.ID, func(t *testing.T) {
			assert.Empty(t, e.Reproduce, "nothing to run")
			assert.Empty(t, e.Data, "nothing to read")
			assert.Empty(t, e.Page, "nothing to link")
			assert.NotEmpty(t, e.Covers, "say what it would measure, or the gap has no shape")
			assert.NotEmpty(t, e.Misses, "say plainly that it is not built")
		})
	}
}

// TestAJudgedEvalDeclaresItsValidation.
//
// A judge's opinion cannot be trusted above its measured agreement with a
// person. An eval that publishes judged numbers without that measurement is
// reporting the judge, not the thing under test — the failure mode this class of
// eval is known for. So the card must either carry the validation or say the
// status is unvalidated; there is no third option.
func TestAJudgedEvalDeclaresItsValidation(t *testing.T) {
	for _, e := range evals {
		if e.Method != MethodJudged || e.Status == StatusAbsent {
			continue
		}
		t.Run(e.ID, func(t *testing.T) {
			if e.Validation == "" {
				assert.Equal(t, StatusUnvalidated, e.Status,
					"a judged eval with no measured agreement is unvalidated, and the card must say so")
				return
			}
			assert.NotEqual(t, StatusUnvalidated, e.Status,
				"validation is recorded, so the status should reflect it")
		})
	}
}

// TestASpendingEvalCommitsItsData: an eval that costs money to run cannot be
// regenerated on every build, so its results must be committed — otherwise the
// page shows nothing until someone pays.
func TestASpendingEvalCommitsItsData(t *testing.T) {
	for _, e := range evals {
		if !e.Spends || e.Status == StatusAbsent {
			continue
		}
		t.Run(e.ID, func(t *testing.T) {
			assert.NotEmpty(t, e.Data,
				"a spending eval commits its results; a build cannot be asked to pay for them")
		})
	}
}

// TestEveryEvalStatesWhatItMisses.
//
// Covers is advertising; Misses is evidence. An eval whose card says only what
// it does invites a reader to assume the rest, and the assumption is always more
// generous than the truth. Fully measured evals may omit it — but the ones this
// page most needs a reader to understand are the partial ones.
func TestEveryEvalStatesWhatItMisses(t *testing.T) {
	for _, e := range evals {
		if e.Status == StatusMeasured {
			continue
		}
		t.Run(e.ID, func(t *testing.T) {
			assert.NotEmpty(t, e.Misses,
				"status is %q, so the card owes the reader an account of what is not covered", e.Status)
		})
	}
}

// TestCoverageIsNotFlattering: the summary counts must match the cards. A
// coverage number computed anywhere other than from the cards themselves would
// be the first thing to drift, and the last thing anyone would check.
func TestCoverageIsNotFlattering(t *testing.T) {
	index, err := Build()
	require.NoError(t, err)

	total := index.Coverage.Measured + index.Coverage.Partial +
		index.Coverage.Unvalidated + index.Coverage.Absent
	assert.Equal(t, len(evals), total, "every eval is counted exactly once")
	assert.Positive(t, index.Coverage.Absent,
		"if nothing is absent, either the work is finished or the registry has stopped listing gaps")
}
