package brand

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"gopkg.in/yaml.v3"
)

// ProfileProblem is one structural problem found while validating a brand voice
// profile. Field is a dotted path into the profile (e.g.
// "style.prohibited_patterns[0].regex"); Message explains the problem. Field is
// empty for whole-profile problems (e.g. an empty document).
type ProfileProblem struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// DecodeProfileStrict decodes a VoiceProfile from a YAML stream and rejects
// unknown fields, so callers (e.g. `kapi brand validate`) can flag typo'd or
// unsupported keys that the lenient LoadProfileYAML silently ignores. It returns
// the decoded profile (best-effort, populated with whatever did decode)
// alongside any decode or unknown-field error. An empty document decodes to a
// zero-value profile with no error (ValidateProfile then reports the missing
// name).
func DecodeProfileStrict(r io.Reader) (*VoiceProfile, error) {
	var p VoiceProfile
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		if err == io.EOF {
			return &p, nil
		}
		return &p, err
	}
	return &p, nil
}

// Valid enum value sets for the constrained string fields. These mirror the
// documented values on the VoiceProfile sub-structs; validation only flags a
// non-empty value that is not in the set, so an omitted field is always allowed.
var (
	validFormality      = []string{"casual", "neutral", "formal", "technical"}
	validEmotion        = []string{"warm", "neutral", "authoritative"}
	validHumor          = []string{"none", "light", "frequent"}
	validSentenceLength = []string{"short", "medium", "varied"}
	validPersonPOV      = []string{"first_plural", "second", "third"}
	validContractions   = []string{"always", "sometimes", "never"}
	validCategory       = []string{"tone", "style", "vocabulary"}
	validSeverity       = []string{
		string(check.SeverityNeutral),
		string(check.SeverityMinor),
		string(check.SeverityMajor),
		string(check.SeverityCritical),
	}
)

// ValidateProfile checks a VoiceProfile for structural problems and returns one
// ProfileProblem per issue (empty when the profile is structurally sound). It is
// the semantic half of `kapi brand validate`: the loader catches YAML syntax and
// unknown-field errors, ValidateProfile catches missing required fields, invalid
// enum values, uncompilable regex patterns, and empty term/example entries.
//
// Only `name` is required; every other field is optional, so an otherwise empty
// profile with just a name validates. The same rules govern standalone
// profile.yaml files, the embedded starter packs, and store-backed profiles.
func ValidateProfile(p *VoiceProfile) []ProfileProblem {
	var probs []ProfileProblem
	add := func(field, msg string) {
		probs = append(probs, ProfileProblem{Field: field, Message: msg})
	}

	if p == nil {
		add("", "profile is empty")
		return probs
	}

	if strings.TrimSpace(p.Name) == "" {
		add("name", "name is required")
	}

	// Tone enums (each optional; a non-empty value must be in range).
	checkEnum(add, "tone.formality", p.Tone.Formality, validFormality)
	checkEnum(add, "tone.emotion", p.Tone.Emotion, validEmotion)
	checkEnum(add, "tone.humor", p.Tone.Humor, validHumor)

	// Style enums.
	checkEnum(add, "style.sentence_length", p.Style.SentenceLength, validSentenceLength)
	checkEnum(add, "style.person_pov", p.Style.PersonPOV, validPersonPOV)
	checkEnum(add, "style.contractions", p.Style.Contractions, validContractions)

	// Style patterns: regex must compile, severity must be in range.
	validatePatterns(add, "style.prohibited_patterns", p.Style.ProhibitedPatterns)
	validatePatterns(add, "style.required_patterns", p.Style.RequiredPatterns)

	// Vocabulary: every term must carry a non-empty term, severity in range.
	validateTerms(add, "vocabulary.preferred_terms", p.Vocabulary.PreferredTerms)
	validateTerms(add, "vocabulary.forbidden_terms", p.Vocabulary.ForbiddenTerms)
	validateTerms(add, "vocabulary.competitor_terms", p.Vocabulary.CompetitorTerms)

	// Examples: before/after carry the transformation; category is optional.
	for i, ex := range p.Examples {
		base := fmt.Sprintf("examples[%d]", i)
		if strings.TrimSpace(ex.Before) == "" {
			add(base+".before", "example before text is empty")
		}
		if strings.TrimSpace(ex.After) == "" {
			add(base+".after", "example after text is empty")
		}
		checkEnum(add, base+".category", ex.Category, validCategory)
	}

	return probs
}

// checkEnum adds a problem when value is non-empty and not one of allowed. An
// empty value is always accepted (the field is optional and falls back to a
// profile-wide default).
func checkEnum(add func(field, msg string), field, value string, allowed []string) {
	if value == "" {
		return
	}
	for _, a := range allowed {
		if value == a {
			return
		}
	}
	add(field, fmt.Sprintf("unknown value %q (expected one of: %s)", value, strings.Join(allowed, ", ")))
}

// validatePatterns checks a list of regex-based style patterns: the regex must
// be non-empty and compilable, and any severity must be a known level.
func validatePatterns(add func(field, msg string), base string, patterns []Pattern) {
	for i, pat := range patterns {
		f := fmt.Sprintf("%s[%d]", base, i)
		switch {
		case strings.TrimSpace(pat.Regex) == "":
			add(f+".regex", "pattern regex is empty")
		default:
			if _, err := regexp.Compile(pat.Regex); err != nil {
				add(f+".regex", fmt.Sprintf("invalid regex %q: %v", pat.Regex, err))
			}
		}
		checkEnum(add, f+".severity", pat.Severity, validSeverity)
	}
}

// validateTerms checks a list of vocabulary term rules: the term text must be
// non-empty and any severity must be a known level. A forbidden term may carry
// an empty replacement (meaning "remove the term"), so the replacement is not
// required.
func validateTerms(add func(field, msg string), base string, terms []TermRule) {
	for i, t := range terms {
		f := fmt.Sprintf("%s[%d]", base, i)
		if strings.TrimSpace(t.Term) == "" {
			add(f+".term", "term is empty")
		}
		checkEnum(add, f+".severity", t.Severity, validSeverity)
	}
}
