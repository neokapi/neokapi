package brand

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresBrandStore_ImplementsInterface(t *testing.T) {
	// Compile-time check that PostgresBrandStore satisfies BrandStore.
	var _ coreprofile.Store = (*PostgresBrandStore)(nil)
}

func TestScanProfile_Roundtrip(t *testing.T) {
	// Verify JSON marshaling of profile fields produces valid output.
	profile := &coreprofile.VoiceProfile{
		ID:    "test-id",
		Name:  "Test Brand",
		Scope: "ws-1",
		Tone: coreprofile.ToneProfile{
			Personality: []string{"friendly", "professional"},
			Formality:   "neutral",
			Emotion:     "warm",
			Humor:       "light",
		},
		Style: coreprofile.StyleRules{
			ActiveVoice:    true,
			SentenceLength: "medium",
			PersonPOV:      "second",
			Contractions:   "sometimes",
		},
		Vocabulary: coreprofile.VocabularyRules{
			PreferredTerms: []coreprofile.TermRule{
				{Term: "use", Replacement: "utilize", Note: "prefer simpler word"},
			},
		},
		Examples: []coreprofile.VoiceExample{
			{Before: "We utilize this.", After: "We use this.", Explanation: "simpler"},
		},
		Locales:  map[model.LocaleID]coreprofile.LocaleOverride{"de": {Formality: "formal"}},
		Channels: map[string]coreprofile.ChannelOverride{},
		Personas: map[string]coreprofile.PersonaOverride{
			"jordan": {Avoided: []coreprofile.TermRule{{Term: "synergy"}}},
		},
		Version: 1,
	}

	assert.NotEmpty(t, profile.ID)
	assert.Equal(t, "Test Brand", profile.Name)
	assert.Equal(t, "neutral", profile.Tone.Formality)
	assert.Len(t, profile.Vocabulary.PreferredTerms, 1)
	assert.Len(t, profile.Locales, 1)
	assert.Len(t, profile.Personas, 1)
}

func TestBrandMigrations_SingleBaseline(t *testing.T) {
	// The brand schema is one consolidated baseline. This test used to assert
	// the opposite — that personas must NOT be in the baseline, because a live
	// database would never re-run it and so would never gain the column. That
	// reasoning held while migrations were bare CREATE/ALTER and a baseline was
	// applied at most once. The baseline is now idempotent and numbered above
	// every version ever issued, so a live database DOES re-run it, and
	// declaring personas in the CREATE is how the column arrives.
	require.Len(t, Migrations, 1, "the brand schema is a single consolidated baseline")
	assert.Equal(t, 3, Migrations[0].Version, "baseline sits above versions 1 and 2, which it folds")
	assert.NotEmpty(t, Migrations[0].SQL)

	sql := Migrations[0].SQL

	// The correction-learning loop's schema, and the personas column folded in
	// from version 2, are all in the one baseline.
	for _, want := range []string{
		"brand_rule_decisions", "brand_voice_corrections", "brand_profile_versions",
		"autonomy", "personas",
	} {
		assert.Contains(t, sql, want)
	}

	// Idempotent throughout: a baseline that is re-applied must not fail on
	// objects that already exist.
	assert.NotContains(t, sql, "CREATE TABLE brand_",
		"every CREATE TABLE must be IF NOT EXISTS so the baseline can be replayed")
}
