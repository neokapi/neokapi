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

// A delete names the revision its caller read. One at an older revision is
// refused with the concept as it stands and deletes nothing; a delete that
// names no revision is refused outright.
func TestPostgresTerms_DeleteNamesTheRevisionItRead(t *testing.T) {
	tb := openTestPostgresTerms(t)
	ctx := context.Background()
	read, err := tb.CreateConcept(ctx, revisionConcept("original"), "")
	require.NoError(t, err)
	_, err = tb.UpdateConcept(ctx, revisionConcept("a person's edit"), "", read)
	require.NoError(t, err)

	require.ErrorIs(t, tb.DeleteConcept(ctx, "save"), pgterms.ErrDeleteNeedsRevision)
	err = tb.DeleteConceptAt(ctx, "save", read)
	var stale *pgterms.StaleConceptError
	require.ErrorAs(t, err, &stale)
	assert.Equal(t, "a person's edit", stale.Current.Definition)
	_, current, ok, err := tb.GetConceptAt(ctx, "save")
	require.NoError(t, err)
	require.True(t, ok, "nothing was deleted")

	require.NoError(t, tb.DeleteConceptAt(ctx, "save", current))
	require.ErrorIs(t, tb.DeleteConceptAt(ctx, "save", current), pgterms.ErrConceptNotFound)
}

// N deleters that read the same revision while a writer changes the concept:
// either the write lands and every delete is refused, or one delete lands and
// the write finds the concept gone. The concept is never deleted with the
// write's change unseen.
func TestPostgresTerms_ConcurrentDeleteAndUpdate(t *testing.T) {
	tb := openTestPostgresTerms(t)
	ctx := context.Background()
	base, err := tb.CreateConcept(ctx, revisionConcept("original"), "")
	require.NoError(t, err)

	const deleters = 8
	errs := make([]error, deleters)
	var updateErr error
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range deleters {
		wg.Go(func() {
			<-start
			errs[i] = tb.DeleteConceptAt(ctx, "save", base)
		})
	}
	wg.Go(func() {
		<-start
		_, updateErr = tb.UpdateConcept(ctx, revisionConcept("a person's edit"), "", base)
	})
	close(start)
	wg.Wait()

	deleted := 0
	for _, err := range errs {
		if err == nil {
			deleted++
		}
	}
	stored, _, held, err := tb.GetConceptAt(ctx, "save")
	require.NoError(t, err)
	if updateErr == nil {
		assert.Zero(t, deleted, "the edit landed, so no delete did")
		require.True(t, held)
		assert.Equal(t, "a person's edit", stored.Definition)
	} else {
		assert.Equal(t, 1, deleted, "one delete landed")
		assert.False(t, held)
	}
}

// Deleting several concepts at their revisions is all or none: one that moved
// keeps every listed concept, and one that is gone already is skipped.
func TestPostgresTerms_DeleteConceptsAtIsAllOrNone(t *testing.T) {
	tb := openTestPostgresTerms(t)
	ctx := context.Background()
	revs := map[string]string{}
	for _, id := range []string{"a", "b", "c"} {
		c := revisionConcept(id)
		c.ID = id
		c.Terms[0].Text, c.Terms[1].Text = "Save "+id, "Lagre "+id
		rev, err := tb.CreateConcept(ctx, c, "")
		require.NoError(t, err)
		revs[id] = rev
	}
	moved := revisionConcept("a person's edit")
	moved.ID = "b"
	moved.Terms[0].Text, moved.Terms[1].Text = "Save b", "Lagre b"
	_, err := tb.UpdateConcept(ctx, moved, "", revs["b"])
	require.NoError(t, err)
	revs["gone"] = "any"

	var stale *pgterms.StaleConceptError
	require.ErrorAs(t, tb.DeleteConceptsAt(ctx, revs), &stale)
	assert.Equal(t, "b", stale.Current.ID)
	held, err := tb.ConceptRevisions(ctx, []string{"a", "b", "c"})
	require.NoError(t, err)
	assert.Len(t, held, 3, "nothing was deleted")

	revs["b"] = held["b"]
	require.NoError(t, tb.DeleteConceptsAt(ctx, revs))
	held, err = tb.ConceptRevisions(ctx, []string{"a", "b", "c"})
	require.NoError(t, err)
	assert.Empty(t, held)
}
