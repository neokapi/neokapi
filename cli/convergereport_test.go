package cli

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProjectConvergence_Composes verifies the exported entry point bundles the
// same coverage, source-readiness, and review-queue derivations the CLI uses.
func TestProjectConvergence_Composes(t *testing.T) {
	root := writeReviewProject(t) // fully-translated nb, reviewed:50 gate, .klftm bound

	a := &App{}
	report, err := a.ProjectConvergence(context.Background(), filepath.Join(root, "proj.kapi"), "en")
	require.NoError(t, err)

	require.Len(t, report.Locales, 1)
	assert.Equal(t, "nb", report.Locales[0].Locale)
	assert.Equal(t, 100, report.Locales[0].Pct["translated"])
	assert.Equal(t, 0, report.Locales[0].Pct["reviewed"], "no approved corrections yet")
	assert.False(t, report.Locales[0].Shippable, "reviewed:50 unmet")

	// Both translated units await review.
	assert.Len(t, report.Review, 2)

	// Approving one lifts reviewed coverage and shrinks the queue on the next call.
	writeReviewedCorrection(t, root, "Apple", "Eple")
	report2, err := a.ProjectConvergence(context.Background(), filepath.Join(root, "proj.kapi"), "en")
	require.NoError(t, err)
	assert.Equal(t, 50, report2.Locales[0].Pct["reviewed"])
	assert.True(t, report2.Locales[0].Shippable)
	assert.Len(t, report2.Review, 1)
}
