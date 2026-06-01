package brand

import (
	"testing"

	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

func TestPostgresBrandStore_ImplementsInterface(t *testing.T) {
	// Compile-time check that PostgresBrandStore satisfies BrandStore.
	var _ corebrand.BrandStore = (*PostgresBrandStore)(nil)
}

func TestScanProfile_Roundtrip(t *testing.T) {
	// Verify JSON marshaling of profile fields produces valid output.
	profile := &corebrand.VoiceProfile{
		ID:          "test-id",
		Name:        "Test Brand",
		WorkspaceID: "ws-1",
		Tone: corebrand.ToneProfile{
			Personality: []string{"friendly", "professional"},
			Formality:   "neutral",
			Emotion:     "warm",
			Humor:       "light",
		},
		Style: corebrand.StyleRules{
			ActiveVoice:    true,
			SentenceLength: "medium",
			PersonPOV:      "second",
			Contractions:   "sometimes",
		},
		Vocabulary: corebrand.VocabularyRules{
			PreferredTerms: []corebrand.TermRule{
				{Term: "use", Replacement: "utilize", Note: "prefer simpler word"},
			},
		},
		Examples: []corebrand.VoiceExample{
			{Before: "We utilize this.", After: "We use this.", Explanation: "simpler"},
		},
		Locales:  map[model.LocaleID]corebrand.LocaleOverride{"de": {Formality: "formal"}},
		Channels: map[string]corebrand.ChannelOverride{},
		Version:  1,
	}

	assert.NotEmpty(t, profile.ID)
	assert.Equal(t, "Test Brand", profile.Name)
	assert.Equal(t, "neutral", profile.Tone.Formality)
	assert.Len(t, profile.Vocabulary.PreferredTerms, 1)
	assert.Len(t, profile.Locales, 1)
}

func TestBrandMigrations_NotEmpty(t *testing.T) {
	// Single clean baseline (pre-launch — no migration history to preserve).
	assert.Len(t, brandMigrations, 1)
	assert.Equal(t, 1, brandMigrations[0].Version)
	assert.NotEmpty(t, brandMigrations[0].SQL)
	// The correction-learning loop's schema is part of the baseline.
	sql := brandMigrations[0].SQL
	for _, want := range []string{"brand_rule_decisions", "brand_voice_corrections", "brand_profile_versions", "autonomy"} {
		assert.Contains(t, sql, want)
	}
}
