package profile

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
// starter packs, so a voice profile works with or without a backing store.
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

// VoiceProfile defines a voice profile configuration with tone, style, and vocabulary rules.
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
	Personas    map[string]PersonaOverride        `json:"personas,omitempty" yaml:"personas,omitempty"`
	// Scope is the opaque partition key the storing host uses to separate one
	// owner's profiles from another's: a server sets it to its tenant key, a
	// single-owner store (the local CLI) leaves it empty. The persisted key
	// stays workspace_id — the name a multi-tenant server writes it under.
	Scope    string         `json:"workspace_id" yaml:"workspace_id,omitempty"`
	Autonomy AutonomyConfig `json:"autonomy,omitzero" yaml:"autonomy,omitempty"`
	// MinScore is the minimum voice-compliance score (0–100) a block must reach
	// to count as compliant in roll-ups (e.g. the dashboard's compliance rate). 0
	// (unset) uses DefaultMinScore; see ComplianceBar.
	MinScore    int       `json:"min_score,omitempty" yaml:"min_score,omitempty"`
	Version     int       `json:"version" yaml:"version,omitempty"`
	VersionNote string    `json:"version_note,omitempty" yaml:"version_note,omitempty"`
	CreatedAt   time.Time `json:"created_at" yaml:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at" yaml:"updated_at,omitempty"`
	CreatedBy   string    `json:"created_by,omitempty" yaml:"created_by,omitempty"`
}

// DefaultMinScore is the compliance bar applied when a profile does not set its
// own MinScore: one critical vocabulary hit (25-point penalty) already drops a
// block below it, while a handful of minor issues does not.
const DefaultMinScore = 80

// ComplianceBar returns the profile's effective minimum compliant score: MinScore
// when set (capped at 100), DefaultMinScore otherwise. A nil profile also
// answers the default, so roll-ups can apply one bar to persisted scores whose
// profile is no longer readable.
func (p *VoiceProfile) ComplianceBar() int {
	if p == nil || p.MinScore <= 0 {
		return DefaultMinScore
	}
	return min(p.MinScore, 100)
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
	if p.Personas != nil {
		c.Personas = make(map[string]PersonaOverride, len(p.Personas))
		maps.Copy(c.Personas, p.Personas)
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
	// DoNotTranslate carries a concept's do-not-translate marking through to
	// the tools. Such a rule names a term and no replacement, which the
	// ordinary rules would skip — "say this instead" needs a this — so the flag
	// is what gives a bare term meaning: leave it exactly as it is.
	DoNotTranslate bool `json:"do_not_translate,omitempty" yaml:"do_not_translate,omitempty"`
	// Exact restricts the rule to the term as written, with no inflections.
	//
	// A term is matched with its regular inflections by default, because the
	// words a profile forbids are mostly verbs and prose uses them inflected: a
	// rule about `utilize` means `utilizes` too, and matching only the bare
	// stem let "the platform utilizes your data" through at 100/100 (#2226).
	//
	// Set this where the exact string is the point: a product name, an
	// identifier, a term whose plural is a different and permitted word.
	Exact bool `json:"exact,omitempty" yaml:"exact,omitempty"`
}

// VoiceExample shows a before/after transformation for voice profile.
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

// PersonaOverride layers an individual author's voice on top of a brand
// profile. It is shaped like ChannelOverride — an optional Tone and Style that
// replace the resolved tone/style — plus additive vocabulary deltas: Preferred
// terms the author leans on and Avoided terms the author personally steers
// clear of.
//
// A persona composes strictly inside the brand's guardrails. Its deltas can
// only tighten vocabulary, never loosen it: Avoided terms add to the profile's
// forbidden set, and a Preferred term that the brand already forbids (or lists
// as a competitor term) is dropped rather than re-allowed. This "brand always
// wins" rule is enforced by ResolveProfile's merge order, not by trusting the
// persona author, so a personal voice can never override a brand prohibition.
type PersonaOverride struct {
	Tone      *ToneProfile `json:"tone,omitempty" yaml:"tone,omitempty"`
	Style     *StyleRules  `json:"style,omitempty" yaml:"style,omitempty"`
	Preferred []TermRule   `json:"preferred_terms,omitempty" yaml:"preferred_terms,omitempty"`
	Avoided   []TermRule   `json:"avoided_terms,omitempty" yaml:"avoided_terms,omitempty"`
}
