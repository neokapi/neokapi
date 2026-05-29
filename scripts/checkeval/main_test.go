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

	for _, c := range rep.Cases {
		assert.Zerof(t, c.FP, "false positive in case %q (got %v, expected %v)", c.ID, c.Got, c.Expect)
		assert.Zerof(t, c.FN, "false negative in case %q (got %v, expected %v)", c.ID, c.Got, c.Expect)
	}
	assert.Equal(t, 0, rep.Total.FP, "total false positives")
	assert.Equal(t, 0, rep.Total.FN, "total false negatives")
	assert.InDelta(t, 1.0, rep.Total.F1, 1e-9, "perfect F1 on the gold seed")
}
