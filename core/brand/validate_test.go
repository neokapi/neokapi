package brand

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// problemFields returns the set of field paths flagged by ValidateProfile, for
// concise assertions.
func problemFields(probs []ProfileProblem) map[string]string {
	m := make(map[string]string, len(probs))
	for _, p := range probs {
		m[p.Field] = p.Message
	}
	return m
}

func TestValidateProfile_Valid(t *testing.T) {
	// Only name is required; a name-only profile is valid.
	assert.Empty(t, ValidateProfile(&VoiceProfile{Name: "Minimal"}))

	// A fully populated, in-range profile is valid.
	p := &VoiceProfile{
		Name:        "Full",
		Description: "desc",
		Tone: ToneProfile{
			Personality: []string{"clear"},
			Formality:   "neutral",
			Emotion:     "warm",
			Humor:       "light",
		},
		Style: StyleRules{
			SentenceLength: "varied",
			PersonPOV:      "second",
			Contractions:   "always",
			ProhibitedPatterns: []Pattern{
				{Regex: `\b(synergy|leverage)\b`, Description: "jargon", Severity: "minor"},
			},
		},
		Vocabulary: VocabularyRules{
			ForbiddenTerms: []TermRule{
				{Term: "utilize", Replacement: "use", Severity: "minor"},
				// A forbidden term with an empty replacement ("remove it") is valid.
				{Term: "in order to", Replacement: ""},
			},
		},
		Examples: []VoiceExample{
			{Before: "We utilize X.", After: "We use X.", Category: "vocabulary"},
		},
	}
	assert.Empty(t, ValidateProfile(p), "fully populated in-range profile must validate")
}

func TestValidateProfile_MissingName(t *testing.T) {
	probs := ValidateProfile(&VoiceProfile{})
	require.NotEmpty(t, probs)
	assert.Equal(t, "name is required", problemFields(probs)["name"])
}

func TestValidateProfile_InvalidEnums(t *testing.T) {
	p := &VoiceProfile{
		Name: "Bad enums",
		Tone: ToneProfile{
			Formality: "snooty",
			Emotion:   "icy",
			Humor:     "constant",
		},
		Style: StyleRules{
			SentenceLength: "epic",
			PersonPOV:      "fourth",
			Contractions:   "rarely",
		},
		Examples: []VoiceExample{
			{Before: "a", After: "b", Category: "mood"},
		},
	}
	fields := problemFields(ValidateProfile(p))
	for _, f := range []string{
		"tone.formality", "tone.emotion", "tone.humor",
		"style.sentence_length", "style.person_pov", "style.contractions",
		"examples[0].category",
	} {
		assert.Contains(t, fields, f, "expected a problem for %s", f)
	}
	assert.Contains(t, fields["tone.formality"], "snooty")
	assert.Contains(t, fields["tone.formality"], "casual, neutral, formal, technical")
}

func TestValidateProfile_BadRegexAndSeverity(t *testing.T) {
	p := &VoiceProfile{
		Name: "Bad patterns",
		Style: StyleRules{
			ProhibitedPatterns: []Pattern{
				{Regex: "(unclosed", Severity: "minor"},    // uncompilable regex
				{Regex: `\bok\b`, Severity: "showstopper"}, // unknown severity
				{Regex: "", Severity: "minor"},             // empty regex
			},
		},
	}
	fields := problemFields(ValidateProfile(p))
	assert.Contains(t, fields, "style.prohibited_patterns[0].regex")
	assert.Contains(t, fields["style.prohibited_patterns[0].regex"], "invalid regex")
	assert.Contains(t, fields, "style.prohibited_patterns[1].severity")
	assert.Equal(t, "pattern regex is empty", fields["style.prohibited_patterns[2].regex"])
}

func TestValidateProfile_EmptyTerms(t *testing.T) {
	p := &VoiceProfile{
		Name: "Empty terms",
		Vocabulary: VocabularyRules{
			PreferredTerms:  []TermRule{{Term: "  "}},                  // whitespace-only term
			ForbiddenTerms:  []TermRule{{Term: "", Severity: "minor"}}, // empty term
			CompetitorTerms: []TermRule{{Term: "Globex", Severity: "nope"}},
		},
	}
	fields := problemFields(ValidateProfile(p))
	assert.Equal(t, "term is empty", fields["vocabulary.preferred_terms[0].term"])
	assert.Equal(t, "term is empty", fields["vocabulary.forbidden_terms[0].term"])
	assert.Contains(t, fields, "vocabulary.competitor_terms[0].severity")
}

func TestDecodeProfileStrict_UnknownField(t *testing.T) {
	yamlStr := `name: Typo
tonee:
  formality: neutral
`
	_, err := DecodeProfileStrict(strings.NewReader(yamlStr))
	require.Error(t, err, "strict decode must reject an unknown field")
	assert.Contains(t, err.Error(), "tonee")
}

func TestDecodeProfileStrict_Empty(t *testing.T) {
	// An empty document decodes to a zero-value profile with no error; the
	// missing name is caught by ValidateProfile, not the decoder.
	p, err := DecodeProfileStrict(strings.NewReader(""))
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Empty(t, p.Name)
}

func TestDecodeProfileStrict_KnownFieldsOK(t *testing.T) {
	yamlStr := `name: Good
tone:
  formality: neutral
`
	p, err := DecodeProfileStrict(strings.NewReader(yamlStr))
	require.NoError(t, err)
	assert.Equal(t, "Good", p.Name)
	assert.Equal(t, "neutral", p.Tone.Formality)
}
