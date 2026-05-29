package main

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func corpusPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(file), "..", "..", "core", "check", "evaldata", "corpus.json")
}

// TestCheckEval_NoRegressions is the quality gate (the checkeval analogue of the
// parity fail-new gate): the seed cases are gold-labeled for the deterministic
// checks, so a correct check makes zero mistakes. Any false positive or false
// negative is a regression that must be fixed (or the corpus label corrected).
func TestCheckEval_NoRegressions(t *testing.T) {
	corpus, err := LoadCorpus(corpusPath(t))
	require.NoError(t, err)
	require.NotEmpty(t, corpus.Cases)

	rep, err := Evaluate(corpus)
	require.NoError(t, err)

	calibrated := 0
	for _, c := range rep.Cases {
		assert.Zerof(t, c.FP, "false positive in case %q (got %v, expected %v)", c.ID, c.Got, c.Expect)
		assert.Zerof(t, c.FN, "false negative in case %q (got %v, expected %v)", c.ID, c.Got, c.Expect)
		// Score calibration (#758): a calibrated case must yield its pinned score,
		// so a change to the severity weights or a checker's severity choice is caught.
		if c.ExpectScore != nil {
			calibrated++
			assert.Truef(t, c.ScoreOK, "score drift in case %q: got %d, expected %d", c.ID, c.Score, *c.ExpectScore)
		}
	}
	assert.Equal(t, 0, rep.Total.FP, "total false positives")
	assert.Equal(t, 0, rep.Total.FN, "total false negatives")
	assert.InDelta(t, 1.0, rep.Total.F1, 1e-9, "perfect F1 on the gold seed")
	assert.NotZero(t, calibrated, "expected at least one score-calibrated case")
}

func correctionsPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(file), "..", "..", "core", "check", "evaldata", "corrections.json")
}

// TestCorrectionsStream_LoopCatchesItsOwnCorrections is the corrections-as-
// ground-truth gate (#759): promoting the simulated correction stream through
// the real loop must produce checks that flag every original a team kept
// correcting and never flag the corrected fix — and leave a below-threshold
// correction un-enforced. This pins the loop's core claim.
func TestCorrectionsStream_LoopCatchesItsOwnCorrections(t *testing.T) {
	corpus, err := LoadCorrectionsCorpus(correctionsPath(t))
	require.NoError(t, err)
	require.NotEmpty(t, corpus.Corrections)

	rep := EvaluateCorrections(corpus)
	require.NotZero(t, rep.Promoted, "expected at least one promoted rule")
	assert.Zero(t, rep.FN, "a promoted rule missed an original it should have caught")
	assert.Zero(t, rep.FP, "a promoted rule flagged a corrected fix (over-flagging)")
	assert.InDelta(t, 1.0, rep.Recall, 1e-9, "every promoted correction must catch its original")
	for _, c := range rep.Cases {
		assert.Truef(t, c.OK, "correction %q (count %d, promoted %v): orig_flagged=%v fix_flagged=%v",
			c.Term, c.Count, c.Promoted, c.OriginalFlagged, c.CorrectedFlagged)
	}
}
