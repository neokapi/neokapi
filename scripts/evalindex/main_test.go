package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verify authored registry entries against repository files and coverage rules.

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
//
// A blocked eval is the other case and takes the opposite rule below: its
// harness runs, so it has a command and a page, and what it lacks is a working
// surface to point at.
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

// Blocked evaluations must include reproduction steps, a results page and an issue reference.
func TestABlockedEvalNamesTheBlocker(t *testing.T) {
	for _, e := range evals {
		if e.Status != StatusBlocked {
			continue
		}
		t.Run(e.ID, func(t *testing.T) {
			assert.NotEmpty(t, e.Reproduce, "a blocked eval still runs; say how")
			assert.NotEmpty(t, e.Page, "link the page that shows what is ready")
			assert.Regexp(t, `#\d+`, e.Misses, "name the issue the eval is waiting on")
		})
	}
}

// Model-judged evaluations must document human validation or be marked unvalidated.
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

// Evaluations that are not fully measured must state their limitations.
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

// Each registered extractor must resolve a headline from its current dataset.
func TestARegisteredHeadlineResolves(t *testing.T) {
	root := repoRoot(t)
	index, err := Build()
	require.NoError(t, err)

	for _, e := range index.Evals {
		if _, registered := extractors[e.ID]; !registered || e.Data == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, e.Data)); err != nil {
			continue // the dataset has not been generated on this machine
		}
		t.Run(e.ID, func(t *testing.T) {
			require.NotNil(t, e.Headline,
				"an extractor is registered for this eval and it returned nothing, so it is reading keys %s does not have", e.Data)
			assert.NotEmpty(t, e.Headline.Value)
			assert.NotEmpty(t, e.Headline.Of, "a number with no unit is not a headline")
			assert.Contains(t, []string{"ok", "warn", "gap"}, e.Headline.Tone)
		})
	}
}

// Report-specific freshness keys must exist in their datasets. A missing key would hide the measurement date.
func TestAFreshAtKeyExists(t *testing.T) {
	root := repoRoot(t)
	for _, e := range evals {
		if e.FreshAt == "" || e.Data == "" {
			continue
		}
		t.Run(e.ID, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(root, e.Data))
			require.NoError(t, err)
			var doc map[string]any
			require.NoError(t, json.Unmarshal(body, &doc))
			assert.Contains(t, doc, e.FreshAt,
				"the card reads its date from %q, which %s does not have", e.FreshAt, e.Data)
		})
	}
}

// TestCoverageIsNotFlattering: the summary counts must match the cards. A
// coverage number computed anywhere other than from the cards themselves would
// be the first thing to drift, and the last thing anyone would check.
func TestCoverageIsNotFlattering(t *testing.T) {
	index, err := Build()
	require.NoError(t, err)

	assert.Equal(t, len(evals), index.Coverage.Total(), "every eval is counted exactly once")
	// The named fields are what the page reads, so they have to agree with the
	// tally they are derived from.
	assert.Equal(t, index.Coverage.ByStatus[StatusMeasured], index.Coverage.Measured)
	assert.Equal(t, index.Coverage.ByStatus[StatusBlocked], index.Coverage.Blocked)
	assert.Equal(t, index.Coverage.ByStatus[StatusAbsent], index.Coverage.Absent)
	// Not `Absent > 0`. Absent went to zero when the four unbuilt evals were
	// built, and the assertion failed for the one reason it should not: the
	// work was done. What it was guarding is that the registry keeps saying
	// where it falls short — so the claim to check is that not everything is
	// measured, which stays true and stays meaningful whichever gap-shaped
	// status the shortfalls carry.
	assert.Less(t, index.Coverage.Measured, len(evals),
		"every eval reads as fully measured, which means either the work is finished or the registry has stopped listing what it misses")

	perBand := 0
	for _, n := range index.Coverage.PerBand {
		perBand += n
	}
	assert.Equal(t, len(evals), perBand, "the per-band counts must add up to every eval, once each")
}
