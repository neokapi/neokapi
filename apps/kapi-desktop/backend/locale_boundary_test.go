package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BCP-47 is the only locale representation inside kapi, so every desktop method
// taking a locale ARGUMENT canonicalizes it at the boundary. A tab passing
// `nb_NO` must reach the stores and the resolver as `nb-NO`; a raw cast keys a
// lookup that silently matches nothing, and a typo becomes an identity nothing
// rejects — indistinguishable from a language nobody has translated yet.

func TestCanonicalLocaleNormalizesAndRefuses(t *testing.T) {
	loc, err := canonicalLocale("nb_NO")
	require.NoError(t, err)
	assert.Equal(t, "nb-NO", string(loc))

	_, err = canonicalLocale("not a locale at all")
	assert.Error(t, err, "a string that is not a locale is refused, not carried")

	_, err = canonicalLocale("")
	assert.Error(t, err)
}

func TestCanonicalLocalesNormalizesAList(t *testing.T) {
	got, err := canonicalLocales([]string{"nb_NO", "de_DE"})
	require.NoError(t, err)
	assert.Equal(t, []string{"nb-NO", "de-DE"}, got)

	// A filter naming no language does not narrow by language.
	got, err = canonicalLocales(nil)
	require.NoError(t, err)
	assert.Empty(t, got)

	_, err = canonicalLocales([]string{"nb_NO", "!!!"})
	assert.Error(t, err, "one bad entry refuses the list")
}

// context.go — ContextSearch takes a locale argument.
func TestContextSearchCanonicalizesItsLocale(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)
	seedConcept(t, app, tab)

	posix, err := app.ContextSearch(tab.ID, "sign in", "nb_NO", 10)
	require.NoError(t, err, "a POSIX-spelled locale is a locale")
	canonical, err := app.ContextSearch(tab.ID, "sign in", "nb-NO", 10)
	require.NoError(t, err)
	assert.Equal(t, canonical, posix, "the two spellings ask one question")

	_, err = app.ContextSearch(tab.ID, "sign in", "not a locale at all", 10)
	assert.Error(t, err)
}

// checks.go — the filter's languages are the caller's locales.
func TestRunChecksCanonicalizesFilterLanguages(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	posix, err := app.RunChecks(tab.ID, ProjectFilter{Languages: []string{"nb_NO"}})
	require.NoError(t, err)
	canonical, err := app.RunChecks(tab.ID, ProjectFilter{Languages: []string{"nb-NO"}})
	require.NoError(t, err)
	assert.Equal(t, len(canonical.Files), len(posix.Files),
		"the two spellings check the same files")

	_, err = app.RunChecks(tab.ID, ProjectFilter{Languages: []string{"!!!"}})
	assert.Error(t, err)
}

// review.go — the decision path shares one body, so it is gated once.
func TestReviewDecisionsCanonicalizeTheirLocale(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	// The unit does not exist, so both spellings fail the same way: what is
	// asserted is that the locale was accepted and normalized before the
	// lookup, not that the lookup succeeded.
	posix := app.SignOffReviewItem(tab.ID, "nb_NO", "docs/help/billing.json", "nope")
	canonical := app.SignOffReviewItem(tab.ID, "nb-NO", "docs/help/billing.json", "nope")
	assert.Equal(t, canonical != nil, posix != nil)
	if canonical != nil {
		assert.Equal(t, canonical.Error(), posix.Error(),
			"nb_NO and nb-NO reach the same unit")
	}

	err := app.RejectReviewItem(tab.ID, "not a locale at all", "docs/help/billing.json", "k", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a locale")

	err = app.UpdateReviewTarget(tab.ID, "!!!", "docs/help/billing.json", "k", "text")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a locale")
}

// review_ai.go — both AI entry points take a locale argument.
func TestReviewAICanonicalizesItsLocale(t *testing.T) {
	app := NewApp()
	tab, _ := newContextProject(t, app)

	_, err := app.ReviewAIAction(tab.ID, "not a locale at all", "f", "k", "explain", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a locale")

	_, err = app.RunAIPreReview(tab.ID, "!!!", PreReviewScope{}, PreReviewPolicy{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a locale")
}

// inspect.go takes no locale argument: its locales are the project's, already
// canonical from the recipe loader. Asserted so the absence is a fact rather
// than an oversight.
func TestInspectTakesNoLocaleArgument(t *testing.T) {
	app := NewApp()
	tab, root := newContextProject(t, app)

	out, err := app.InspectFileAnnotated(tab.ID, root+"/docs/help/billing.json")
	require.NoError(t, err)
	assert.NotEmpty(t, out)
}
