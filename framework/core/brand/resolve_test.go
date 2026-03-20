package brand

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveProfile_Nil(t *testing.T) {
	assert.Nil(t, ResolveProfile(nil, "en", "web"))
}

func TestResolveProfile_NoOverrides(t *testing.T) {
	profile := &VoiceProfile{
		ID:   "test",
		Name: "Test Profile",
		Tone: ToneProfile{Formality: "casual", Humor: "light"},
	}

	resolved := ResolveProfile(profile, "", "")

	require.NotNil(t, resolved)
	assert.Equal(t, "casual", resolved.Tone.Formality)
	assert.Equal(t, "light", resolved.Tone.Humor)
}

func TestResolveProfile_LocaleOverride(t *testing.T) {
	profile := &VoiceProfile{
		ID:    "test",
		Name:  "Test Profile",
		Tone:  ToneProfile{Formality: "casual", Humor: "light"},
		Style: StyleRules{PersonPOV: "second"},
		Vocabulary: VocabularyRules{
			PreferredTerms: []TermRule{{Term: "app", Replacement: "application"}},
		},
		Locales: map[string]LocaleOverride{
			"ja-JP": {
				Formality:           "formal",
				Humor:               "none",
				PersonPOV:           "third",
				VocabularyOverrides: []TermRule{{Term: "san", Note: "use honorifics"}},
				ExampleOverrides:    []VoiceExample{{Before: "Hey!", After: "Dear customer,"}},
			},
		},
	}

	resolved := ResolveProfile(profile, "ja-JP", "")

	require.NotNil(t, resolved)
	assert.Equal(t, "formal", resolved.Tone.Formality)
	assert.Equal(t, "none", resolved.Tone.Humor)
	assert.Equal(t, "third", resolved.Style.PersonPOV)
	assert.Len(t, resolved.Vocabulary.PreferredTerms, 2) // original + override
	assert.Len(t, resolved.Examples, 1)
}

func TestResolveProfile_ChannelOverride(t *testing.T) {
	profile := &VoiceProfile{
		ID:    "test",
		Name:  "Test Profile",
		Tone:  ToneProfile{Formality: "casual", Humor: "light", Personality: []string{"friendly"}},
		Style: StyleRules{PersonPOV: "second", ActiveVoice: true},
		Channels: map[string]ChannelOverride{
			"support": {
				Tone:  &ToneProfile{Formality: "formal", Emotion: "empathetic", Personality: []string{"caring"}},
				Style: &StyleRules{PersonPOV: "first_plural", ActiveVoice: false},
			},
		},
	}

	resolved := ResolveProfile(profile, "", "support")

	require.NotNil(t, resolved)
	assert.Equal(t, "formal", resolved.Tone.Formality)
	assert.Equal(t, "empathetic", resolved.Tone.Emotion)
	assert.Equal(t, "first_plural", resolved.Style.PersonPOV)
	assert.False(t, resolved.Style.ActiveVoice)
}

func TestResolveProfile_LocaleAndChannel(t *testing.T) {
	profile := &VoiceProfile{
		ID:   "test",
		Name: "Test Profile",
		Tone: ToneProfile{Formality: "casual", Humor: "light"},
		Locales: map[string]LocaleOverride{
			"de-DE": {Formality: "formal"},
		},
		Channels: map[string]ChannelOverride{
			"marketing": {
				Tone: &ToneProfile{Formality: "casual", Humor: "frequent"},
			},
		},
	}

	// Channel override replaces tone entirely, so locale's formality override
	// is applied first, then channel replaces the whole tone.
	resolved := ResolveProfile(profile, "de-DE", "marketing")

	require.NotNil(t, resolved)
	// Channel override replaces the entire tone
	assert.Equal(t, "casual", resolved.Tone.Formality)
	assert.Equal(t, "frequent", resolved.Tone.Humor)
}

func TestResolveProfile_UnknownLocale(t *testing.T) {
	profile := &VoiceProfile{
		ID:   "test",
		Tone: ToneProfile{Formality: "casual"},
		Locales: map[string]LocaleOverride{
			"ja-JP": {Formality: "formal"},
		},
	}

	resolved := ResolveProfile(profile, "fr-FR", "")

	require.NotNil(t, resolved)
	assert.Equal(t, "casual", resolved.Tone.Formality) // unchanged
}
