package terms

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
	fw "github.com/neokapi/neokapi/terms"
)

// concept builds a concept carrying one preferred term per locale.
func concept(id string, locales ...model.LocaleID) fw.Concept {
	cp := fw.Concept{ID: id, Domain: "commerce"}
	for _, locale := range locales {
		cp.Terms = append(cp.Terms, fw.Term{
			Text: id + "-" + string(locale), Locale: locale, Status: model.TermPreferred,
		})
	}
	return cp
}

// TestCoverageFromConcepts mirrors the cases the web dashboard's
// computeLocaleCoverage is pinned to, so the number a reader sees does not move
// when the panel switches from its 100-concept sample to this aggregate.
func TestCoverageFromConcepts(t *testing.T) {
	t.Run("no concepts", func(t *testing.T) {
		assert.Empty(t, CoverageFromConcepts(nil))
	})

	t.Run("one row per locale, sorted by coverage", func(t *testing.T) {
		got := CoverageFromConcepts([]fw.Concept{
			concept("a", "en-US", "de-DE"),
			concept("b", "en-US"),
			concept("c", "en-US", "fr-FR"),
		})
		assert.Equal(t, LocaleCoverage{Locale: "en-US", Present: 3, Total: 3, Pct: 100}, got[0])
		assert.Equal(t, []model.LocaleID{"en-US", "de-DE", "fr-FR"},
			[]model.LocaleID{got[0].Locale, got[1].Locale, got[2].Locale})
		// 1/3 rounds to 33, matching the client's Math.round.
		assert.Equal(t, LocaleCoverage{Locale: "de-DE", Present: 1, Total: 3, Pct: 33}, got[1])
	})

	t.Run("a locale counts once per concept", func(t *testing.T) {
		got := CoverageFromConcepts([]fw.Concept{{
			ID: "dup",
			Terms: []fw.Term{
				{Text: "a", Locale: "en-US", Status: model.TermPreferred},
				{Text: "b", Locale: "en-US", Status: model.TermAdmitted},
			},
		}})
		assert.Equal(t, []LocaleCoverage{{Locale: "en-US", Present: 1, Total: 1, Pct: 100}}, got)
	})

	t.Run("a concept with no terms still counts in the total", func(t *testing.T) {
		got := CoverageFromConcepts([]fw.Concept{concept("a", "en-US"), {ID: "bare"}})
		assert.Equal(t, []LocaleCoverage{{Locale: "en-US", Present: 1, Total: 2, Pct: 50}}, got)
	})

	t.Run("a term with no locale is not a locale", func(t *testing.T) {
		got := CoverageFromConcepts([]fw.Concept{{
			ID:    "unlocalized",
			Terms: []fw.Term{{Text: "a", Status: model.TermPreferred}},
		}})
		assert.Empty(t, got)
	})
}

func TestCountsFromConcepts(t *testing.T) {
	got := CountsFromConcepts([]fw.Concept{
		{ID: "a", Terms: []fw.Term{
			{Text: "sign in", Locale: "en", Status: model.TermPreferred},
			{Text: "log in", Locale: "en", Status: model.TermForbidden},
		}},
		{ID: "b", Terms: []fw.Term{
			// Two preferred terms in one concept count the concept once.
			{Text: "cart", Locale: "en", Status: model.TermPreferred},
			{Text: "basket", Locale: "en-GB", Status: model.TermPreferred},
		}},
		{ID: "c"},
	})

	assert.Equal(t, 3, got.Total, "a concept with no terms is still a concept")
	assert.Equal(t, 2, got.ByStatus[model.TermPreferred])
	assert.Equal(t, 1, got.ByStatus[model.TermForbidden])
	assert.Zero(t, got.ByStatus[model.TermProposed])
}

func TestCoveragePctRounds(t *testing.T) {
	// Halves round up, as Math.round does on the client.
	assert.Equal(t, 50, coveragePct(1, 2))
	assert.Equal(t, 13, coveragePct(1, 8))
	assert.Equal(t, 0, coveragePct(0, 3))
	assert.Equal(t, 0, coveragePct(1, 0))
}
