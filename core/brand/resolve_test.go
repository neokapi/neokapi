package brand

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBrandStore is a minimal BrandStore for testing profile resolution.
type mockBrandStore struct {
	profiles map[string]*VoiceProfile
}

func (m *mockBrandStore) GetProfile(_ context.Context, id string) (*VoiceProfile, error) {
	p, ok := m.profiles[id]
	if !ok {
		return nil, fmt.Errorf("profile not found: %s", id)
	}
	return p, nil
}

func (m *mockBrandStore) CreateProfile(context.Context, *VoiceProfile) error { return nil }
func (m *mockBrandStore) UpdateProfile(context.Context, *VoiceProfile) error { return nil }
func (m *mockBrandStore) DeleteProfile(context.Context, string) error        { return nil }
func (m *mockBrandStore) ListProfiles(context.Context, string) ([]*VoiceProfile, error) {
	return nil, nil
}
func (m *mockBrandStore) ListProfileVersions(context.Context, string) ([]*ProfileVersion, error) {
	return nil, nil
}
func (m *mockBrandStore) GetProfileVersion(context.Context, string, int) (*ProfileVersion, error) {
	return nil, nil
}
func (m *mockBrandStore) GetProfileAtTag(context.Context, string, string) (*VoiceProfile, error) {
	return nil, nil
}
func (m *mockBrandStore) CreateProfileTag(context.Context, *ProfileTag) error { return nil }
func (m *mockBrandStore) ListProfileTags(context.Context, string) ([]*ProfileTag, error) {
	return nil, nil
}
func (m *mockBrandStore) DeleteProfileTag(context.Context, string, string) error { return nil }
func (m *mockBrandStore) StoreScore(context.Context, *StoredScore) error         { return nil }
func (m *mockBrandStore) GetScores(context.Context, string, string) ([]*StoredScore, error) {
	return nil, nil
}
func (m *mockBrandStore) GetScoreTrends(context.Context, string, int) ([]*ScoreTrend, error) {
	return nil, nil
}
func (m *mockBrandStore) GetScoresByStream(context.Context, string, string) ([]*StoredScore, error) {
	return nil, nil
}
func (m *mockBrandStore) StoreCorrection(context.Context, *Correction) error      { return nil }
func (m *mockBrandStore) RecordRuleDecision(context.Context, *RuleDecision) error { return nil }
func (m *mockBrandStore) GetRuleDecision(context.Context, string, string) (*RuleDecision, error) {
	return nil, nil
}
func (m *mockBrandStore) ListRuleDecisions(context.Context, string) ([]*RuleDecision, error) {
	return nil, nil
}
func (m *mockBrandStore) GetSuggestedRules(context.Context, string, int) ([]*SuggestedRule, error) {
	return nil, nil
}
func (m *mockBrandStore) Close() error { return nil }

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

func TestResolveProfileFromContext(t *testing.T) {
	store := &mockBrandStore{
		profiles: map[string]*VoiceProfile{
			"ws-default": {ID: "ws-default", Name: "Workspace Default", Tone: ToneProfile{Formality: "formal"}},
			"proj-voice": {ID: "proj-voice", Name: "Project Voice", Tone: ToneProfile{Formality: "neutral"}},
			"stream-exp": {ID: "stream-exp", Name: "Stream Experiment", Tone: ToneProfile{Formality: "casual"}},
			"col-voice":  {ID: "col-voice", Name: "Collection Voice", Tone: ToneProfile{Formality: "technical"}},
			"explicit":   {ID: "explicit", Name: "Explicit", Tone: ToneProfile{Formality: "formal"}},
			"with-channel": {
				ID: "with-channel", Name: "With Channel", Tone: ToneProfile{Formality: "formal"},
				Channels: map[string]ChannelOverride{
					"email": {Tone: &ToneProfile{Formality: "casual", Personality: []string{"friendly"}}},
				},
			},
		},
	}

	tests := []struct {
		name          string
		rc            ResolveContext
		wantName      string
		wantFormality string
		wantNil       bool
	}{
		{
			name:    "no bindings returns nil",
			rc:      ResolveContext{},
			wantNil: true,
		},
		{
			name:          "workspace default",
			rc:            ResolveContext{WorkspaceProfileID: "ws-default"},
			wantName:      "Workspace Default",
			wantFormality: "formal",
		},
		{
			name: "project overrides workspace",
			rc: ResolveContext{
				WorkspaceProfileID: "ws-default",
				ProjectProperties:  map[string]string{PropertyProfileID: "proj-voice"},
			},
			wantName:      "Project Voice",
			wantFormality: "neutral",
		},
		{
			name: "stream overrides project",
			rc: ResolveContext{
				ProjectProperties: map[string]string{PropertyProfileID: "proj-voice"},
				StreamProperties:  map[string]string{PropertyProfileID: "stream-exp"},
			},
			wantName:      "Stream Experiment",
			wantFormality: "casual",
		},
		{
			name: "collection overrides stream",
			rc: ResolveContext{
				StreamProperties: map[string]string{PropertyProfileID: "stream-exp"},
				CollectionConfig: map[string]string{PropertyProfileID: "col-voice"},
			},
			wantName:      "Collection Voice",
			wantFormality: "technical",
		},
		{
			name: "explicit overrides everything",
			rc: ResolveContext{
				ExplicitProfileID:  "explicit",
				WorkspaceProfileID: "ws-default",
				ProjectProperties:  map[string]string{PropertyProfileID: "proj-voice"},
				StreamProperties:   map[string]string{PropertyProfileID: "stream-exp"},
				CollectionConfig:   map[string]string{PropertyProfileID: "col-voice"},
			},
			wantName:      "Explicit",
			wantFormality: "formal",
		},
		{
			name: "channel override applied from collection config",
			rc: ResolveContext{
				ProjectProperties: map[string]string{PropertyProfileID: "with-channel"},
				CollectionConfig:  map[string]string{PropertyChannel: "email"},
			},
			wantName:      "With Channel",
			wantFormality: "casual", // channel override replaces tone
		},
		{
			name: "channel resolution: collection wins over project",
			rc: ResolveContext{
				ProjectProperties: map[string]string{
					PropertyProfileID: "with-channel",
					PropertyChannel:   "nonexistent",
				},
				CollectionConfig: map[string]string{PropertyChannel: "email"},
			},
			wantName:      "With Channel",
			wantFormality: "casual",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := ResolveProfileFromContext(t.Context(), tt.rc, store)
			require.NoError(t, err)

			if tt.wantNil {
				assert.Nil(t, profile)
				return
			}

			require.NotNil(t, profile)
			assert.Equal(t, tt.wantName, profile.Name)
			assert.Equal(t, tt.wantFormality, profile.Tone.Formality)
		})
	}
}

func TestStoreProfileResolver(t *testing.T) {
	store := &mockBrandStore{
		profiles: map[string]*VoiceProfile{
			"test-id": {ID: "test-id", Name: "Test"},
		},
	}
	resolver := &StoreProfileResolver{Store: store}

	profile, err := resolver.ResolveProfile(t.Context(), ResolveContext{
		ExplicitProfileID: "test-id",
	})
	require.NoError(t, err)
	require.NotNil(t, profile)
	assert.Equal(t, "Test", profile.Name)
}
