package brand

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVoiceProfile_JSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	profile := VoiceProfile{
		ID:          "prof-1",
		Name:        "Brand Guidelines",
		Description: "Main brand voice",
		Tone: ToneProfile{
			Personality: []string{"friendly", "knowledgeable"},
			Formality:   "neutral",
			Emotion:     "warm",
			Humor:       "light",
			Guidelines:  "Be approachable but professional",
		},
		Style: StyleRules{
			ActiveVoice:    true,
			SentenceLength: "medium",
			PersonPOV:      "second",
			Contractions:   "sometimes",
			ProhibitedPatterns: []Pattern{
				{Regex: `\bsynergy\b`, Description: "avoid corporate jargon", Severity: "minor"},
			},
		},
		Vocabulary: VocabularyRules{
			PreferredTerms: []TermRule{
				{Term: "workspace", Replacement: "", Note: "use instead of 'project'"},
			},
			ForbiddenTerms: []TermRule{
				{Term: "cheap", Replacement: "affordable", Severity: "major"},
			},
			Abbreviations: map[string]string{"API": "Application Programming Interface"},
		},
		Examples: []VoiceExample{
			{Before: "Click here", After: "Select the option", Category: "style"},
		},
		Locales: map[model.LocaleID]LocaleOverride{
			"ja-JP": {Formality: "formal"},
		},
		Channels: map[string]ChannelOverride{
			"support": {Tone: &ToneProfile{Formality: "formal"}},
		},
		WorkspaceID: "ws-1",
		Version:     3,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   "user-1",
	}

	data, err := json.Marshal(profile)
	require.NoError(t, err)

	var decoded VoiceProfile
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, profile.ID, decoded.ID)
	assert.Equal(t, profile.Name, decoded.Name)
	assert.Equal(t, profile.Description, decoded.Description)
	assert.Equal(t, profile.Tone.Personality, decoded.Tone.Personality)
	assert.Equal(t, profile.Tone.Formality, decoded.Tone.Formality)
	assert.Equal(t, profile.Style.ActiveVoice, decoded.Style.ActiveVoice)
	assert.Equal(t, profile.Style.PersonPOV, decoded.Style.PersonPOV)
	assert.Len(t, decoded.Style.ProhibitedPatterns, 1)
	assert.Len(t, decoded.Vocabulary.PreferredTerms, 1)
	assert.Len(t, decoded.Vocabulary.ForbiddenTerms, 1)
	assert.Equal(t, "API", firstKey(decoded.Vocabulary.Abbreviations))
	assert.Len(t, decoded.Examples, 1)
	assert.Contains(t, decoded.Locales, model.LocaleID("ja-JP"))
	assert.Contains(t, decoded.Channels, "support")
	assert.Equal(t, profile.WorkspaceID, decoded.WorkspaceID)
	assert.Equal(t, profile.Version, decoded.Version)
	assert.Equal(t, profile.CreatedBy, decoded.CreatedBy)
}

func TestVoiceProfile_EmptyOptionalFields(t *testing.T) {
	profile := VoiceProfile{
		ID:   "minimal",
		Name: "Minimal Profile",
	}

	data, err := json.Marshal(profile)
	require.NoError(t, err)

	var decoded VoiceProfile
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "minimal", decoded.ID)
	assert.Equal(t, "Minimal Profile", decoded.Name)
	assert.Empty(t, decoded.Description)
	assert.Nil(t, decoded.Locales)
	assert.Nil(t, decoded.Channels)
	assert.Empty(t, decoded.CreatedBy)
}

func firstKey(m map[string]string) string {
	for k := range m {
		return k
	}
	return ""
}
