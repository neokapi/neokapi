package brand

import (
	"fmt"
	"io"
	"maps"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"gopkg.in/yaml.v3"
)

// LoadProfileYAML decodes a VoiceProfile from a YAML stream. This is the canonical
// loader for standalone, git-shareable `profile.yaml` files and for the embedded
// starter packs, so a brand profile works with or without a backing store.
func LoadProfileYAML(r io.Reader) (*VoiceProfile, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}
	var p VoiceProfile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse profile: %w", err)
	}
	return &p, nil
}

// VoiceProfile defines a brand voice configuration with tone, style, and vocabulary rules.
type VoiceProfile struct {
	ID          string                            `json:"id" yaml:"id,omitempty"`
	Name        string                            `json:"name" yaml:"name"`
	Description string                            `json:"description,omitempty" yaml:"description,omitempty"`
	Tone        ToneProfile                       `json:"tone" yaml:"tone"`
	Style       StyleRules                        `json:"style" yaml:"style"`
	Vocabulary  VocabularyRules                   `json:"vocabulary" yaml:"vocabulary"`
	Examples    []VoiceExample                    `json:"examples" yaml:"examples"`
	Locales     map[model.LocaleID]LocaleOverride `json:"locales,omitempty" yaml:"locales,omitempty"`
	Channels    map[string]ChannelOverride        `json:"channels,omitempty" yaml:"channels,omitempty"`
	WorkspaceID string                            `json:"workspace_id" yaml:"workspace_id,omitempty"`
	Autonomy    AutonomyConfig                    `json:"autonomy,omitzero" yaml:"autonomy,omitempty"`
	Version     int                               `json:"version" yaml:"version,omitempty"`
	VersionNote string                            `json:"version_note,omitempty" yaml:"version_note,omitempty"`
	CreatedAt   time.Time                         `json:"created_at" yaml:"created_at,omitempty"`
	UpdatedAt   time.Time                         `json:"updated_at" yaml:"updated_at,omitempty"`
	CreatedBy   string                            `json:"created_by,omitempty" yaml:"created_by,omitempty"`
}

// Clone returns a deep copy of the profile across the collection-typed fields
// the promotion and evaluation flow touch (tone, style patterns, vocabulary,
// examples, locale/channel overrides), so a candidate profile can be built and
// mutated without affecting the baseline. Returns nil for a nil receiver.
func (p *VoiceProfile) Clone() *VoiceProfile {
	if p == nil {
		return nil
	}
	c := *p
	c.Tone.Personality = append([]string(nil), p.Tone.Personality...)
	c.Style.ProhibitedPatterns = append([]Pattern(nil), p.Style.ProhibitedPatterns...)
	c.Style.RequiredPatterns = append([]Pattern(nil), p.Style.RequiredPatterns...)
	c.Vocabulary.PreferredTerms = append([]TermRule(nil), p.Vocabulary.PreferredTerms...)
	c.Vocabulary.ForbiddenTerms = append([]TermRule(nil), p.Vocabulary.ForbiddenTerms...)
	c.Vocabulary.CompetitorTerms = append([]TermRule(nil), p.Vocabulary.CompetitorTerms...)
	if p.Vocabulary.Abbreviations != nil {
		c.Vocabulary.Abbreviations = make(map[string]string, len(p.Vocabulary.Abbreviations))
		maps.Copy(c.Vocabulary.Abbreviations, p.Vocabulary.Abbreviations)
	}
	c.Examples = append([]VoiceExample(nil), p.Examples...)
	if p.Locales != nil {
		c.Locales = make(map[model.LocaleID]LocaleOverride, len(p.Locales))
		maps.Copy(c.Locales, p.Locales)
	}
	if p.Channels != nil {
		c.Channels = make(map[string]ChannelOverride, len(p.Channels))
		maps.Copy(c.Channels, p.Channels)
	}
	return &c
}

// ProfileVersion is an immutable snapshot of a profile at a point in time.
// Each UpdateProfile() call archives the previous state as a ProfileVersion.
type ProfileVersion struct {
	ProfileID string       `json:"profile_id"`
	Version   int          `json:"version"`
	Snapshot  VoiceProfile `json:"snapshot"`
	Note      string       `json:"note"`
	CreatedBy string       `json:"created_by"`
	CreatedAt time.Time    `json:"created_at"`
}

// ProfileTag is a named reference to a specific profile version.
type ProfileTag struct {
	ProfileID string    `json:"profile_id"`
	Name      string    `json:"name"`    // e.g., "v1.0-launch", "pre-rebrand"
	Version   int       `json:"version"` // points to a specific ProfileVersion
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// ToneProfile describes the desired tone characteristics.
type ToneProfile struct {
	Personality []string `json:"personality" yaml:"personality"` // e.g. ["friendly", "knowledgeable", "direct"]
	Formality   string   `json:"formality" yaml:"formality"`     // "casual", "neutral", "formal", "technical"
	Emotion     string   `json:"emotion" yaml:"emotion"`         // "warm", "neutral", "authoritative"
	Humor       string   `json:"humor" yaml:"humor"`             // "none", "light", "frequent"
	Guidelines  string   `json:"guidelines,omitempty" yaml:"guidelines,omitempty"`
}

// StyleRules defines writing style constraints.
type StyleRules struct {
	ActiveVoice        bool      `json:"active_voice" yaml:"active_voice"`
	SentenceLength     string    `json:"sentence_length" yaml:"sentence_length"` // "short", "medium", "varied"
	PersonPOV          string    `json:"person_pov" yaml:"person_pov"`           // "first_plural", "second", "third"
	Contractions       string    `json:"contractions" yaml:"contractions"`       // "always", "sometimes", "never"
	ProhibitedPatterns []Pattern `json:"prohibited_patterns,omitempty" yaml:"prohibited_patterns,omitempty"`
	RequiredPatterns   []Pattern `json:"required_patterns,omitempty" yaml:"required_patterns,omitempty"`
}

// Pattern describes a regex-based text pattern rule.
type Pattern struct {
	Regex       string `json:"regex" yaml:"regex"`
	Description string `json:"description" yaml:"description"`
	Severity    string `json:"severity" yaml:"severity"` // "minor", "major", "critical"
}

// VocabularyRules defines term usage constraints.
type VocabularyRules struct {
	PreferredTerms  []TermRule        `json:"preferred_terms,omitempty" yaml:"preferred_terms,omitempty"`
	ForbiddenTerms  []TermRule        `json:"forbidden_terms,omitempty" yaml:"forbidden_terms,omitempty"`
	CompetitorTerms []TermRule        `json:"competitor_terms,omitempty" yaml:"competitor_terms,omitempty"`
	Abbreviations   map[string]string `json:"abbreviations,omitempty" yaml:"abbreviations,omitempty"`
}

// TermRule describes a vocabulary constraint for a specific term.
type TermRule struct {
	Term        string `json:"term" yaml:"term"`
	Replacement string `json:"replacement,omitempty" yaml:"replacement,omitempty"`
	Note        string `json:"note,omitempty" yaml:"note,omitempty"`
	Severity    string `json:"severity,omitempty" yaml:"severity,omitempty"` // "minor", "major", "critical"
	// ConceptID is the knowledge-graph concept this rule denotes (one node type:
	// the concept). It is populated when the platform promotes a rule from a
	// concept-backed correction; it stays empty for standalone profiles (a
	// shareable profile.yaml with no backing knowledge graph), which remain valid.
	ConceptID string `json:"concept_id,omitempty" yaml:"concept_id,omitempty"`
}

// VoiceExample shows a before/after transformation for brand voice.
type VoiceExample struct {
	Before      string `json:"before" yaml:"before"`
	After       string `json:"after" yaml:"after"`
	Explanation string `json:"explanation,omitempty" yaml:"explanation,omitempty"`
	Category    string `json:"category,omitempty" yaml:"category,omitempty"` // "tone", "style", "vocabulary"
}

// LocaleOverride provides locale-specific adjustments to a voice profile.
type LocaleOverride struct {
	Formality           string         `json:"formality,omitempty" yaml:"formality,omitempty"`
	Humor               string         `json:"humor,omitempty" yaml:"humor,omitempty"`
	PersonPOV           string         `json:"person_pov,omitempty" yaml:"person_pov,omitempty"`
	CulturalNotes       string         `json:"cultural_notes,omitempty" yaml:"cultural_notes,omitempty"`
	VocabularyOverrides []TermRule     `json:"vocabulary_overrides,omitempty" yaml:"vocabulary_overrides,omitempty"`
	ExampleOverrides    []VoiceExample `json:"example_overrides,omitempty" yaml:"example_overrides,omitempty"`
}

// ChannelOverride provides channel-specific adjustments to a voice profile.
type ChannelOverride struct {
	Tone  *ToneProfile `json:"tone,omitempty" yaml:"tone,omitempty"`
	Style *StyleRules  `json:"style,omitempty" yaml:"style,omitempty"`
}
