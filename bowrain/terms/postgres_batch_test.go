//go:build integration

package terms_test

import (
	"testing"

	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPostgresTerms_ConceptsBatchedHydration guards the finding-6 fix: Concepts
// hydrates every concept and its terms in two batched queries instead of 1+2N
// per-concept round trips. The batching must group terms by concept without
// cross-contamination and return every concept.
func TestPostgresTerms_ConceptsBatchedHydration(t *testing.T) {
	tb := openTestPostgresTerms(t)
	ctx := t.Context()

	for _, c := range softwareConcepts() {
		require.NoError(t, tb.AddConcept(ctx, c))
	}

	got, err := tb.Concepts(ctx)
	require.NoError(t, err)

	byID := map[string]terms.Concept{}
	for _, c := range got {
		byID[c.ID] = c
	}
	require.Len(t, byID, 3, "every concept must be hydrated")

	// Each concept keeps exactly its own terms — the batched grouping must not
	// leak one concept's terms onto another.
	assert.Len(t, byID["c1"].Terms, 3)
	assert.Len(t, byID["c2"].Terms, 3)
	assert.Len(t, byID["c3"].Terms, 4)

	texts := map[string]bool{}
	for _, term := range byID["c1"].Terms {
		texts[term.Text] = true
	}
	assert.True(t, texts["Save"] && texts["Sauvegarder"] && texts["Speichern"],
		"c1 must carry its own terms")
	assert.False(t, texts["Cancel"], "c2's terms must not leak into c1")
}
