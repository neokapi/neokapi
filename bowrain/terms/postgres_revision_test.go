//go:build integration

package terms_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/storage"
	pgterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

func revisionConcept(definition string) terms.Concept {
	return terms.Concept{
		ID:         "save",
		Definition: definition,
		Terms: []terms.Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "Lagre", Locale: "nb", Status: model.TermPreferred},
		},
	}
}

// N writers that all read the same revision race to change the concept:
// exactly one write lands, and every other writer is refused with what the
// winner stored, so none of them overwrites a change it never saw.
func TestPostgresTerms_ConcurrentUpdatesOnOneBase(t *testing.T) {
	tb := openTestPostgresTerms(t)
	ctx := context.Background()

	base, err := tb.CreateConcept(ctx, revisionConcept("original"), "")
	require.NoError(t, err)
	require.NotEmpty(t, base)

	const writers = 12
	type result struct {
		definition string
		revision   string
		err        error
	}
	results := make([]result, writers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range writers {
		wg.Go(func() {
			<-start
			def := fmt.Sprintf("writer %d", i)
			rev, err := tb.UpdateConcept(ctx, revisionConcept(def), "", base)
			results[i] = result{definition: def, revision: rev, err: err}
		})
	}
	close(start)
	wg.Wait()

	var won []result
	for _, r := range results {
		if r.err == nil {
			won = append(won, r)
		}
	}
	require.Len(t, won, 1, "exactly one writer on a shared base wins")
	winner := won[0]

	stored, rev, ok, err := tb.GetConceptAt(ctx, "save")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, winner.definition, stored.Definition)
	assert.Equal(t, winner.revision, rev)
	assert.NotEqual(t, base, rev)

	for _, r := range results {
		if r.err == nil {
			continue
		}
		require.ErrorIs(t, r.err, storage.ErrStale)
		var stale *pgterms.StaleConceptError
		require.ErrorAs(t, r.err, &stale)
		assert.Equal(t, winner.definition, stale.Current.Definition, "the refusal carries what the winner stored")
		assert.Equal(t, winner.revision, stale.Revision)
		assert.Len(t, stale.Current.Terms, 2)
	}
}

func TestPostgresTerms_CreateAndUpdateAreSeparate(t *testing.T) {
	tb := openTestPostgresTerms(t)
	ctx := context.Background()

	_, err := tb.UpdateConcept(ctx, revisionConcept("never stored"), "", "rev-from-nowhere")
	require.ErrorIs(t, err, pgterms.ErrConceptNotFound)
	_, _, ok, err := tb.GetConceptAt(ctx, "save")
	require.NoError(t, err)
	assert.False(t, ok, "an update never creates")

	first, err := tb.CreateConcept(ctx, revisionConcept("first"), "")
	require.NoError(t, err)

	_, err = tb.CreateConcept(ctx, revisionConcept("second"), "")
	require.ErrorIs(t, err, pgterms.ErrConceptExists)
	stored, rev, _, err := tb.GetConceptAt(ctx, "save")
	require.NoError(t, err)
	assert.Equal(t, "first", stored.Definition, "a second create leaves the stored concept alone")
	assert.Equal(t, first, rev)

	_, err = tb.UpdateConcept(ctx, revisionConcept("no base"), "", "")
	require.Error(t, err)

	revs, err := tb.ConceptRevisions(ctx, []string{"save", "missing"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"save": first}, revs)
}

// AddConcept never replaces a stored concept: the stored one, and its
// revision, stay as they were.
func TestPostgresTerms_AddConceptNeverReplaces(t *testing.T) {
	tb := openTestPostgresTerms(t)
	ctx := context.Background()

	base, err := tb.CreateConcept(ctx, revisionConcept("first"), "")
	require.NoError(t, err)
	require.ErrorIs(t, tb.AddConcept(ctx, revisionConcept("replaced")), pgterms.ErrConceptExists)
	require.ErrorIs(t, tb.AddConceptWithStream(ctx, revisionConcept("replaced"), "pilot"), pgterms.ErrConceptExists)

	stored, rev, _, err := tb.GetConceptAt(ctx, "save")
	require.NoError(t, err)
	assert.Equal(t, "first", stored.Definition)
	assert.Equal(t, base, rev)
}
