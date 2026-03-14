package brand

import (
	"time"
)

// VoiceProfile defines a brand voice configuration with tone, style, and vocabulary rules.
type VoiceProfile struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Description string                    `json:"description,omitempty"`
	Tone        ToneProfile               `json:"tone"`
	Style       StyleRules                `json:"style"`
	Vocabulary  VocabularyRules           `json:"vocabulary"`
	Examples    []VoiceExample            `json:"examples"`
	Locales     map[string]LocaleOverride `json:"locales,omitempty"`
	Channels    map[string]ChannelOverride `json:"channels,omitempty"`
	WorkspaceID string                    `json:"workspace_id"`
	Version     int                       `json:"version"`
	CreatedAt   time.Time                 `json:"created_at"`
	UpdatedAt   time.Time                 `json:"updated_at"`
	CreatedBy   string                    `json:"created_by,omitempty"`
}

// ToneProfile describes the desired tone characteristics.
type ToneProfile struct {
	Personality []string `json:"personality"`        // e.g. ["friendly", "knowledgeable", "direct"]
	Formality   string   `json:"formality"`          // "casual", "neutral", "formal", "technical"
	Emotion     string   `json:"emotion"`            // "warm", "neutral", "authoritative"
	Humor       string   `json:"humor"`              // "none", "light", "frequent"
	Guidelines  string   `json:"guidelines,omitempty"`
}

// StyleRules defines writing style constraints.
type StyleRules struct {
	ActiveVoice        bool      `json:"active_voice"`
	SentenceLength     string    `json:"sentence_length"`      // "short", "medium", "varied"
	PersonPOV          string    `json:"person_pov"`           // "first_plural", "second", "third"
	Contractions       string    `json:"contractions"`         // "always", "sometimes", "never"
	ProhibitedPatterns []Pattern `json:"prohibited_patterns,omitempty"`
	RequiredPatterns   []Pattern `json:"required_patterns,omitempty"`
}

// Pattern describes a regex-based text pattern rule.
type Pattern struct {
	Regex       string `json:"regex"`
	Description string `json:"description"`
	Severity    string `json:"severity"` // "minor", "major", "critical"
}

// VocabularyRules defines term usage constraints.
type VocabularyRules struct {
	PreferredTerms  []TermRule        `json:"preferred_terms,omitempty"`
	ForbiddenTerms  []TermRule        `json:"forbidden_terms,omitempty"`
	CompetitorTerms []TermRule        `json:"competitor_terms,omitempty"`
	Abbreviations   map[string]string `json:"abbreviations,omitempty"`
}

// TermRule describes a vocabulary constraint for a specific term.
type TermRule struct {
	Term        string `json:"term"`
	Replacement string `json:"replacement,omitempty"`
	Note        string `json:"note,omitempty"`
	Severity    string `json:"severity,omitempty"` // "minor", "major", "critical"
}

// VoiceExample shows a before/after transformation for brand voice.
type VoiceExample struct {
	Before      string `json:"before"`
	After       string `json:"after"`
	Explanation string `json:"explanation,omitempty"`
	Category    string `json:"category,omitempty"` // "tone", "style", "vocabulary"
}

// LocaleOverride provides locale-specific adjustments to a voice profile.
type LocaleOverride struct {
	Formality           string         `json:"formality,omitempty"`
	Humor               string         `json:"humor,omitempty"`
	PersonPOV           string         `json:"person_pov,omitempty"`
	CulturalNotes       string         `json:"cultural_notes,omitempty"`
	VocabularyOverrides []TermRule     `json:"vocabulary_overrides,omitempty"`
	ExampleOverrides    []VoiceExample `json:"example_overrides,omitempty"`
}

// ChannelOverride provides channel-specific adjustments to a voice profile.
type ChannelOverride struct {
	Tone  *ToneProfile `json:"tone,omitempty"`
	Style *StyleRules  `json:"style,omitempty"`
}
