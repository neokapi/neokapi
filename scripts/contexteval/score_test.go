package main

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests drive the scorer with hand-written translations, so every
// pass/fail rule is pinned without a model in the loop — the same reason the
// check tools themselves are deterministic.

// scoreOne runs a full de corpus pass where one fixture gets the given
// translation and every other fixture is left missing, then returns the
// verdicts for that fixture's checks by kind.
func scoreOne(t *testing.T, target, key, translation string) PassScore {
	t.Helper()
	corpus := CorpusFor(target)
	blocks := corpus.Blocks()
	loc := model.LocaleID(target)
	found := false
	for i, f := range corpus.Fixtures {
		if f.Key == key {
			tool.NewVariantView(blocks[i]).SetTargetText(loc, translation)
			found = true
		}
	}
	require.True(t, found, "no fixture %q for %s", key, target)
	score, err := scorePass(context.Background(), corpus, blocks, "test")
	require.NoError(t, err)
	return score
}

func kindCounts(s PassScore, dim, kind string) Counts {
	if c := s.ByKind[dim+"/"+kind]; c != nil {
		return *c
	}
	return Counts{}
}

func TestTermMandateScoresWithTheRealTermCheck(t *testing.T) {
	pass := scoreOne(t, "de", "nav.overview.title", "Öffnen Sie den Leitstand, um den heutigen Verkehr zu prüfen.")
	assert.Equal(t, Counts{Scored: 1, Passed: 1}, kindCounts(pass, DimTerminology, "term"))

	fail := scoreOne(t, "de", "nav.overview.title", "Öffnen Sie das Dashboard, um den heutigen Verkehr zu prüfen.")
	assert.Equal(t, Counts{Scored: 1, Passed: 0}, kindCounts(fail, DimTerminology, "term"))
}

func TestDNTScoresWithTheRealDNTCheck(t *testing.T) {
	pass := scoreOne(t, "de", "tidewatch.intro.body", "Tidewatch warnt Sie, bevor sich die Bedingungen ändern.")
	assert.Equal(t, Counts{Scored: 1, Passed: 1}, kindCounts(pass, DimTerminology, "dnt"))

	fail := scoreOne(t, "de", "tidewatch.intro.body", "Gezeitenwacht warnt Sie, bevor sich die Bedingungen ändern.")
	assert.Equal(t, Counts{Scored: 1, Passed: 0}, kindCounts(fail, DimTerminology, "dnt"))
}

func TestTranslatedProductNameFailsBothDNTChecks(t *testing.T) {
	// compass.overview.tagline carries two dnt expectations: the verbatim
	// survival (real dnt-check) and the "did not become Kompass" pattern.
	fail := scoreOne(t, "de", "compass.overview.tagline", "Der Kompass zeigt jedes Wasserfahrzeug in einer Ansicht.")
	assert.Equal(t, Counts{Scored: 2, Passed: 0}, kindCounts(fail, DimTerminology, "dnt"))

	pass := scoreOne(t, "de", "compass.overview.tagline", "Compass zeigt jedes Wasserfahrzeug in einer Ansicht.")
	assert.Equal(t, Counts{Scored: 2, Passed: 2}, kindCounts(pass, DimTerminology, "dnt"))
}

func TestForbiddenVocabularyScoresWithTheRealBrandVocabCheck(t *testing.T) {
	fail := scoreOne(t, "de", "users.alerts.perms", "Jeder Nutzer kann seine Schwellenwerte anpassen.")
	assert.Equal(t, Counts{Scored: 1, Passed: 0}, kindCounts(fail, DimVoice, "vocab"))

	pass := scoreOne(t, "de", "users.alerts.perms", "Jeder Benutzer kann seine Schwellenwerte anpassen.")
	assert.Equal(t, Counts{Scored: 1, Passed: 1}, kindCounts(pass, DimVoice, "vocab"))
}

func TestConflictWinnerIsExcused(t *testing.T) {
	// "Nutzer-ID" contains the forbidden "Nutzer" as a whole word (hyphen
	// boundary). The fixture declares the glossary pin the winner, so the hit
	// is excused — obeying the pin must not score as a vocabulary violation.
	got := scoreOne(t, "de", "account.link.label", "Geben Sie Ihre Nutzer-ID ein, um die mobile Anwendung zu verknüpfen.")
	assert.Equal(t, Counts{Scored: 1, Passed: 1}, kindCounts(got, DimTerminology, "term-conflict"))
	assert.Equal(t, Counts{Scored: 1, Passed: 1}, kindCounts(got, DimVoice, "vocab-conflict"))

	// Dropping the pin fails the term check — and stays excused on vocabulary
	// only for the pinned compound, not for a stray standalone "Nutzer".
	stray := scoreOne(t, "de", "account.link.label", "Geben Sie Ihre Kennung ein, damit der Nutzer die Anwendung verknüpfen kann.")
	assert.Equal(t, Counts{Scored: 1, Passed: 0}, kindCounts(stray, DimTerminology, "term-conflict"))
	assert.Equal(t, Counts{Scored: 1, Passed: 1}, kindCounts(stray, DimVoice, "vocab-conflict"),
		"a standalone 'Nutzer' matches the same rule term and is excused by term identity — a documented limit of excusing by matched text")
}

func TestFormalityPattern(t *testing.T) {
	informal := scoreOne(t, "de", "onboarding.done.note", "Hey, du bist startklar. Hol dir deine Dateien, wann immer du willst.")
	assert.Equal(t, Counts{Scored: 1, Passed: 0}, kindCounts(informal, DimVoice, "formality"))

	formal := scoreOne(t, "de", "onboarding.done.note", "Sie sind startklar. Rufen Sie Ihre Dateien jederzeit ab.")
	assert.Equal(t, Counts{Scored: 1, Passed: 1}, kindCounts(formal, DimVoice, "formality"))
}

func TestFrenchFormalityIsNotFooledByAccents(t *testing.T) {
	// Go's \b is ASCII-only, so a naive `\btes\b` matches the tail of "êtes"
	// and `\bte\b` the middle of "boîte" — flagging the very formal phrasing
	// the profile asks for. The pattern guards with explicit letter classes;
	// this pins that a formal sentence full of accents scores clean.
	formal := scoreOne(t, "fr", "onboarding.done.note", "Vous êtes prêt. Récupérez vos fichiers à tout moment, à côté de votre boîte de réception.")
	assert.Equal(t, Counts{Scored: 1, Passed: 1}, kindCounts(formal, DimVoice, "formality"))

	informal := scoreOne(t, "fr", "onboarding.done.note", "Hé, tu es prêt. Récupère tes fichiers quand tu veux.")
	assert.Equal(t, Counts{Scored: 1, Passed: 0}, kindCounts(informal, DimVoice, "formality"))
}

func TestDistractorAcceptsTheNaturalCompound(t *testing.T) {
	// "Schiffskompass" is the idiomatic rendering of "the ship's compass"; a
	// \b-anchored pattern would fail it and record a correct translation as a
	// distractor violation.
	compound := scoreOne(t, "de", "nav.instrument.calibrate", "Kalibrieren Sie den Schiffskompass vor der Abfahrt.")
	assert.Equal(t, Counts{Scored: 1, Passed: 1}, kindCounts(compound, DimTerminology, "dnt-distractor"))
}

func TestContractionPatternSparesPossessives(t *testing.T) {
	kept := scoreOne(t, "en-GB", "status.update.progress", "We're updating your workspace. It'll only take a moment. Don't close this window.")
	assert.Equal(t, Counts{Scored: 1, Passed: 0}, kindCounts(kept, DimVoice, "contractions"))

	expanded := scoreOne(t, "en-GB", "status.update.progress", "We are updating the vessel's workspace. It will only take a moment. Do not close this window.")
	assert.Equal(t, Counts{Scored: 1, Passed: 1}, kindCounts(expanded, DimVoice, "contractions"),
		"a possessive apostrophe is not a contraction")
}

func TestNorwegianVocabTrap(t *testing.T) {
	naive := scoreOne(t, "nb", "harbor.traffic.title", "Spor hver båt i havnen.")
	assert.Equal(t, Counts{Scored: 1, Passed: 0}, kindCounts(naive, DimVoice, "vocab"))

	swapped := scoreOne(t, "nb", "harbor.traffic.title", "Spor hver farkost i havnen.")
	assert.Equal(t, Counts{Scored: 1, Passed: 1}, kindCounts(swapped, DimVoice, "vocab"))
}

func TestNorwegianMandateSurvivesInflection(t *testing.T) {
	// Norwegian suffixes plural and definiteness onto the stem, so the mandated
	// compound must be chosen to survive: "alarmmeldinger" still contains
	// "alarmmelding". This pins that an obedient plural rendering passes both
	// the term mandate and the digits rule.
	obedient := scoreOne(t, "nb", "alerts.badge.tooltip", "2 nye alarmmeldinger krever din oppmerksomhet.")
	assert.Equal(t, Counts{Scored: 1, Passed: 1}, kindCounts(obedient, DimTerminology, "term"))
	assert.Equal(t, Counts{Scored: 1, Passed: 1}, kindCounts(obedient, DimInstruction, "digits"))

	naive := scoreOne(t, "nb", "alerts.badge.tooltip", "To nye varsler krever din oppmerksomhet.")
	assert.Equal(t, Counts{Scored: 1, Passed: 0}, kindCounts(naive, DimTerminology, "term"))
	assert.Equal(t, Counts{Scored: 1, Passed: 0}, kindCounts(naive, DimInstruction, "digits"))
}

func TestDigitsInstruction(t *testing.T) {
	spelled := scoreOne(t, "fr", "alerts.count.new", "Vous avez trois nouveaux avis de vigilance dans votre boîte de réception.")
	assert.Equal(t, Counts{Scored: 1, Passed: 0}, kindCounts(spelled, DimInstruction, "digits"))

	digits := scoreOne(t, "fr", "alerts.count.new", "Vous avez 3 nouveaux avis de vigilance dans votre boîte de réception.")
	assert.Equal(t, Counts{Scored: 1, Passed: 1}, kindCounts(digits, DimInstruction, "digits"))
}

func TestEchoIsExcludedNotScored(t *testing.T) {
	corpus := CorpusFor("de")
	blocks := corpus.Blocks()
	loc := model.LocaleID("de")
	// Echo every fixture: on a target language different from the source, an
	// echo is untranslated output, and scoring it would let a lazy model "pass"
	// every DNT check for free.
	for i, f := range corpus.Fixtures {
		tool.NewVariantView(blocks[i]).SetTargetText(loc, f.Source)
	}
	score, err := scorePass(context.Background(), corpus, blocks, "test")
	require.NoError(t, err)
	assert.Equal(t, len(corpus.Fixtures), score.Untranslated)
	assert.Empty(t, score.ByDim, "echoed fixtures must not be scored")
}

func TestEchoIsLegitimateForEnGB(t *testing.T) {
	// en → en-GB identity is a valid rendering; it is scored, not excluded.
	got := scoreOne(t, "en-GB", "fleet.category.color", "Choose a color for each vessel category.")
	assert.Equal(t, Counts{Scored: 1, Passed: 0}, kindCounts(got, DimInstruction, "spelling"),
		"the un-adapted spelling still fails, but as a scored check, not an echo")
}

func TestMissingIsAHole(t *testing.T) {
	corpus := CorpusFor("de")
	blocks := corpus.Blocks() // no targets set at all
	score, err := scorePass(context.Background(), corpus, blocks, "test")
	require.NoError(t, err)
	assert.Equal(t, len(corpus.Fixtures), score.Missing)
	assert.Empty(t, score.ByDim)
}

func TestRateSentinelDistinguishesUnscoredFromZero(t *testing.T) {
	assert.Equal(t, float64(-1), Counts{}.Rate())
	assert.Equal(t, float64(0), Counts{Scored: 3}.Rate())
	assert.Equal(t, float64(100), Counts{Scored: 3, Passed: 3}.Rate())
}
