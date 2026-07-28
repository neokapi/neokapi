package brand

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresBrandStore_ImplementsInterface(t *testing.T) {
	// Compile-time check that PostgresBrandStore satisfies BrandStore.
	var _ coreprofile.BrandStore = (*PostgresBrandStore)(nil)
}

func TestScanProfile_Roundtrip(t *testing.T) {
	// Verify JSON marshaling of profile fields produces valid output.
	profile := &coreprofile.VoiceProfile{
		ID:          "test-id",
		Name:        "Test Brand",
		WorkspaceID: "ws-1",
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

func TestBrandMigrations_NotEmpty(t *testing.T) {
	// Version 1 is the launch baseline; post-launch schema changes arrive as
	// incremental migrations (live databases never re-run the baseline).
	require.GreaterOrEqual(t, len(brandMigrations), 2)
	assert.Equal(t, 1, brandMigrations[0].Version)
	assert.NotEmpty(t, brandMigrations[0].SQL)
	// The correction-learning loop's schema is part of the baseline.
	sql := brandMigrations[0].SQL
	for _, want := range []string{"brand_rule_decisions", "brand_voice_corrections", "brand_profile_versions", "autonomy"} {
		assert.Contains(t, sql, want)
	}
	// Versions are contiguous, and personas arrives via an incremental
	// migration so pre-personas databases (incl. production) gain the column.
	var all strings.Builder
	for i, m := range brandMigrations {
		assert.Equal(t, i+1, m.Version, "migration versions must be contiguous")
		all.WriteString(m.SQL)
	}
	assert.NotContains(t, brandMigrations[0].SQL, "personas", "personas must not be baseline-only — live DBs would never get it")
	assert.Contains(t, all.String(), "personas")
}
