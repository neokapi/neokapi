package mdspike_test

import (
	"context"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/profile/mdspike"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openTermsStore builds an in-memory terms store from the dogfood
// .kapi/terms.json — the real store this repo governs its own prose with, not
// a fixture.
func openTermsStore(t *testing.T) terms.Terminology {
	t.Helper()
	f, err := os.Open(termsJSON)
	require.NoError(t, err)
	defer f.Close()

	store := terms.NewInMemoryStore()
	n, err := terms.ImportJSON(context.Background(), store, f)
	require.NoError(t, err)
	require.Positive(t, n)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func termTexts(rules []profile.TermRule) []string {
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.Term)
	}
	sort.Strings(out)
	return out
}

// TestTermsStoreSourcingChangesCheckBehaviour is the finding the spike exists to
// make concrete: sourcing the ban list from the terms store is not a
// refactoring. It adds prohibitions the profile never declared, and text that
// passes `kapi voice check` today starts failing.
//
// The added bans are the retired spellings CLAUDE.md already forbids in prose,
// so the change is in the direction the project wants. It is still a change in
// whether content ships, and it must be made as a decision, not as a side
// effect of changing the file format.
func TestTermsStoreSourcingChangesCheckBehaviour(t *testing.T) {
	store := openTermsStore(t)
	handAuthored := loadMarkdown(t, "bowrain-voice.md", mdspike.Options{})
	sourced := loadMarkdown(t, "bowrain-voice-from-terms.md", mdspike.Options{Terms: store})

	handBans := termTexts(handAuthored.Vocabulary.ForbiddenTerms)
	sourcedBans := termTexts(sourced.Vocabulary.ForbiddenTerms)
	t.Logf("forbidden terms hand-authored (%d): %v", len(handBans), handBans)
	t.Logf("forbidden terms terms-sourced (%d): %v", len(sourcedBans), sourcedBans)

	// The store contributes exactly the deprecated English terms in the sourced
	// domains, each already carrying its replacement.
	for _, want := range []string{"sievepen", "glossary", "termbase", "translation memory"} {
		assert.NotContains(t, handBans, want, "not banned today")
		assert.Contains(t, sourcedBans, want, "banned once the store is the source")
	}

	// Prose that is clean under the committed profile is not clean under the
	// sourced one. This is the pass/fail move, stated as a score.
	const sample = "The termbase and the translation memory both live in the workspace."
	assert.Empty(t, profile.MatchVocabulary(handAuthored, sample),
		"the committed profile raises nothing on retired spellings")

	hits := profile.MatchVocabulary(sourced, sample)
	require.Len(t, hits, 2)
	scoreBefore := profile.CalculateScore(profile.HitsToFindings(profile.MatchVocabulary(handAuthored, sample), sample, nil))
	scoreAfter := profile.CalculateScore(profile.HitsToFindings(hits, sample, nil))
	t.Logf("sample score: %d before, %d after (compliant bar %d)",
		scoreBefore.Overall, scoreAfter.Overall, sourced.ComplianceBar())
	assert.Equal(t, 100, scoreBefore.Overall)
	assert.Less(t, scoreAfter.Overall, scoreBefore.Overall)

	// Each derived ban carries the concept it came from, so a finding can pivot
	// back to the concept — the ConceptID that is populated today only when the
	// platform promotes a rule from a correction.
	for _, h := range hits {
		assert.NotEmpty(t, h.ConceptID, "derived ban %q carries its concept", h.Term)
		assert.NotEmpty(t, h.Replacement, "derived ban %q carries the preferred term", h.Term)
	}
}

// TestAdmittedTermsAreNotBanned checks the status mapping the store makes
// possible and the profile cannot express at all: "terms" is an *admitted*
// alternative for the concept whose preferred term is "terms store". A ban list
// derived from status must leave it alone.
func TestAdmittedTermsAreNotBanned(t *testing.T) {
	store := openTermsStore(t)
	sourced := loadMarkdown(t, "bowrain-voice-from-terms.md", mdspike.Options{Terms: store})

	assert.NotContains(t, termTexts(sourced.Vocabulary.ForbiddenTerms), "terms",
		"an admitted alternative is not a prohibition")
}

// TestTermsStoreSourcingBloatsTheGuide measures the other half of the trade.
// The store holds every concept the project governs; a profile's preferred-term
// list is a short editorial selection. Sourcing one from the other replaces a
// five-line section of the prompt with a survey of the terminology.
func TestTermsStoreSourcingBloatsTheGuide(t *testing.T) {
	store := openTermsStore(t)
	handAuthored := loadMarkdown(t, "bowrain-voice.md", mdspike.Options{})
	sourced := loadMarkdown(t, "bowrain-voice-from-terms.md", mdspike.Options{Terms: store})

	before := len(handAuthored.Vocabulary.PreferredTerms)
	after := len(sourced.Vocabulary.PreferredTerms)
	guideBefore := len(profile.RenderVoiceGuide(handAuthored))
	guideAfter := len(profile.RenderVoiceGuide(sourced))
	t.Logf("preferred terms: %d → %d; rendered guide: %d → %d bytes (+%d%%)",
		before, after, guideBefore, guideAfter, (guideAfter-guideBefore)*100/guideBefore)

	assert.Greater(t, after, before*2, "the store's selection is not the profile's selection")
	assert.Greater(t, guideAfter, guideBefore)

	// The two brand coinages the store has no concept for still have to be
	// hand-authored, so sourcing does not remove the profile's vocabulary
	// section — it only removes the part that overlaps.
	texts := termTexts(sourced.Vocabulary.PreferredTerms)
	assert.Contains(t, texts, "workspace")
	assert.Contains(t, texts, "context graph")
}

// TestTermsStoreDefinitionsAreNotPromptText is the sharpest reason not to source
// the preferred list. A concept definition is written for a translator deciding
// which word to use; a profile note is written to be pasted into a model
// prompt. Two of the dogfood store's definitions still spell the retired
// abbreviation, and sourcing carries them into the brand guide the model reads.
func TestTermsStoreDefinitionsAreNotPromptText(t *testing.T) {
	store := openTermsStore(t)
	handAuthored := loadMarkdown(t, "bowrain-voice.md", mdspike.Options{})
	sourced := loadMarkdown(t, "bowrain-voice-from-terms.md", mdspike.Options{Terms: store})

	const retired = "TM"
	assert.NotContains(t, profile.RenderVoiceGuide(handAuthored), retired,
		"the hand-authored guide is clean")

	var leaked []string
	for _, r := range sourced.Vocabulary.PreferredTerms {
		if slices.Contains(strings.Fields(r.Note), retired) || strings.Contains(r.Note, retired+" ") {
			leaked = append(leaked, r.Term)
		}
	}
	sort.Strings(leaked)
	t.Logf("terms whose store definition carries the retired abbreviation: %v", leaked)
	assert.NotEmpty(t, leaked,
		"the store's definitions were not written to be prompt text")
	assert.Contains(t, profile.RenderVoiceGuide(sourced), retired)
}
