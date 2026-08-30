package profile

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"gopkg.in/yaml.v3"
)

// ProfileProblem is one structural problem found while validating a voice profile
// profile. Field is a dotted path into the profile (e.g.
// "style.prohibited_patterns[0].regex"); Message explains the problem. Field is
// empty for whole-profile problems (e.g. an empty document).
type ProfileProblem struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	// Warning marks a problem that does not make the profile unusable.
	//
	// Everything here used to be fatal, which forced a choice between refusing
	// a profile and saying nothing about it. Tone needs the third answer: an
	// unfamiliar register is a description the guide passes through, worth
	// mentioning and never worth refusing over.
	Warning bool `json:"warning,omitempty"`
}

// Blocking returns the problems that make a profile unusable, dropping the
// advisory ones. A gate calls this; a report shows everything.
func Blocking(probs []ProfileProblem) []ProfileProblem {
	out := make([]ProfileProblem, 0, len(probs))
	for _, p := range probs {
		if !p.Warning {
			out = append(out, p)
		}
	}
	return out
}

// DecodeProfileStrict decodes a VoiceProfile from a YAML stream and rejects
// unknown fields, so callers (e.g. `kapi voice validate`) can flag typo'd or
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
		if errors.Is(err, io.EOF) {
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
// the semantic half of `kapi voice validate`: the loader catches YAML syntax and
// unknown-field errors, ValidateProfile catches missing required fields, invalid
// enum values, uncompilable regex patterns, and empty term/example entries.
//
// Only `name` is required; every other field is optional, so an otherwise empty
// profile with just a name validates. The same rules govern standalone
// profile.yaml files, the embedded starter packs, and store-backed profiles.
func ValidateProfile(p *VoiceProfile) []ProfileProblem {
	var probs []ProfileProblem
	note := func(field, msg string, warning bool) {
		probs = append(probs, ProfileProblem{Field: field, Message: msg, Warning: warning})
	}
	add := func(field, msg string) { note(field, msg, false) }

	if p == nil {
		add("", "profile is empty")
		return probs
	}

	if strings.TrimSpace(p.Name) == "" {
		add("name", "name is required")
	}

	// MinScore is an optional 0–100 bar; 0 means "use the default".
	if p.MinScore < 0 || p.MinScore > 100 {
		add("min_score", fmt.Sprintf("min_score must be between 0 and 100 (got %d)", p.MinScore))
	}

	// Tone enums (each optional; a non-empty value must be in range).
	// Tone is described, not enumerated.
	//
	// These were closed sets, and a profile naming a register outside them was
	// REFUSED. kapi inferred "calm and matter-of-fact; enthusiasm reserved for
	// genuinely good news" from ripgrep's own documentation, and the profile
	// would not load until that was squashed to `neutral` — discarding exactly
	// what distinguished the voice. Moved into free-text guidelines it worked,
	// which shows the enum carried nothing the prose could not.
	//
	// The research agrees from the other side: Wikipedia lists formality among
	// the INEFFECTIVE indicators of machine writing, and register instruction
	// has been measured not to move a model's output. A longer list of labels
	// makes the description longer without making it a demonstration. See #2242.
	noteEnum(note, "tone.formality", p.Tone.Formality, validFormality)
	noteEnum(note, "tone.emotion", p.Tone.Emotion, validEmotion)
	noteEnum(note, "tone.humor", p.Tone.Humor, validHumor)

	// Style enums stay closed. Unlike tone these are read by code — active_voice
	// and person_pov are what the offline check evaluates — so an unrecognised
	// value is a rule that silently does nothing rather than a description that
	// reaches the model.
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
	if slices.Contains(allowed, value) {
		return
	}
	add(field, fmt.Sprintf("unknown value %q (expected one of: %s)", value, strings.Join(allowed, ", ")))
}

// noteEnum mentions a value outside the known set without refusing it.
//
// The known values still mean what they meant, and anything else is prose the
// guide renders as written.
func noteEnum(add func(field, msg string, warning bool), field, value string, valid []string) {
	if value == "" || slices.Contains(valid, value) {
		return
	}
	add(field, fmt.Sprintf("%q is not one of the usual values (%s). It is kept and rendered "+
		"into the voice guide as written.", value, strings.Join(valid, ", ")), true)
}

// validatePatterns checks a list of regex-based style patterns: the regex must
// be non-empty and compilable, and any severity must be a known level.
// validatePatternRule checks what a rate and a scope have to mean to be worth
// writing down.
func validatePatternRule(add func(field, msg string), base string, pat Pattern) {
	if r := pat.Rate; r != nil {
		if r.Max <= 0 {
			add(base+".rate.max", "a rate of zero is the same as having no rate: "+
				"leave `rate` out to forbid the pattern outright")
		}
		if r.Per < 0 {
			add(base+".rate.per_words", "per_words cannot be negative")
		}
	}
	switch pat.Scope {
	case "", ScopeProse, ScopeCode, ScopeHeading:
	default:
		add(base+".scope", fmt.Sprintf("unknown scope %q (expected one of: %s, %s, %s, "+
			"or omit it to match everywhere)", pat.Scope, ScopeProse, ScopeCode, ScopeHeading))
	}
}

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
		validatePatternRule(add, f, pat)
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

// FieldValueSet is the values a constrained field accepts, and what happens to
// one outside them.
type FieldValueSet struct {
	Values []string `json:"values"`
	// Open is true when a value outside Values is kept and rendered rather than
	// refused. Tone is described, not enumerated: a register the list does not
	// name is what distinguishes one voice from another, and squashing it to the
	// nearest label discards exactly that.
	Open bool `json:"open"`
}

// FieldValues returns the value sets ValidateProfile applies, keyed by the field
// path a ProfileProblem names.
//
// An editor offering these cannot drift from what validation accepts, because
// both read the same slices. `severity` and `scope` are keyed bare: they apply
// to every rule and pattern rather than to one path.
func FieldValues() map[string]FieldValueSet {
	return map[string]FieldValueSet{
		"tone.formality":        {Values: slices.Clone(validFormality), Open: true},
		"tone.emotion":          {Values: slices.Clone(validEmotion), Open: true},
		"tone.humor":            {Values: slices.Clone(validHumor), Open: true},
		"style.sentence_length": {Values: slices.Clone(validSentenceLength)},
		"style.person_pov":      {Values: slices.Clone(validPersonPOV)},
		"style.contractions":    {Values: slices.Clone(validContractions)},
		"examples.category":     {Values: slices.Clone(validCategory)},
		"severity":              {Values: slices.Clone(validSeverity)},
		"scope":                 {Values: []string{ScopeProse, ScopeCode, ScopeHeading}},
	}
}
