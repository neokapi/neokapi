package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEvaluationInvariants runs the measurement over the committed corpora,
// rules, forms and labels, and pins what must hold whatever the labels say.
func TestEvaluationInvariants(t *testing.T) {
	rep, err := run("../..", "", "")
	require.NoError(t, err)
	require.NotEmpty(t, rep.Rows)
	assert.Empty(t, rep.Unlabelled, "every demand and fail carries a label")

	for _, r := range rep.Rows {
		if r.Mode == modeSubstring {
			continue
		}
		assert.Zero(t, r.DeletedPassed, "%s %s %s passed a target with the rendering deleted", r.Corpus, r.Lang, r.Mode)
		assert.Zero(t, r.ClippedPassed, "%s %s %s passed a target with a clipped rendering", r.Corpus, r.Lang, r.Mode)

		if r.Mode != modeForms || r.Corpus == "dogfood" {
			continue
		}
		// On the samples, any fail that declared forms leave must be a reviewed
		// rendering that uses a different word.
		for class := range r.FailClasses {
			assert.Equal(t, "different-word", class, "%s %s fails a %s target with declared forms", r.Corpus, r.Lang, class)
		}
		assert.Zero(t, r.FalseFails, "%s %s with declared forms", r.Corpus, r.Lang)
	}
}
