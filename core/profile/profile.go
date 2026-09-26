package profile

import (
	"maps"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/model"
)

// VoiceProfile defines a voice: tone, style measures, pattern rules, guidance
// and examples. Word rules ("write this, not that") are terms, held in the
// terms store; a profile read from a file carries the file's own word rules
// beside it (see CarriedTerms).
type VoiceProfile struct {
	// A nil list is omitted on the wire; an explicit empty list requests removal.
	Constraints     []Constraint `json:"constraints,omitzero" yaml:"constraints,omitempty"`
	constraintScope ConstraintScope
	ID              string                            `json:"id" yaml:"id,omitempty"`
	Name            string                            `json:"name" yaml:"name"`
	Description     string                            `json:"description,omitempty" yaml:"description,omitempty"`
	Tone            ToneProfile                       `json:"tone" yaml:"tone"`
	Style           StyleRules                        `json:"style" yaml:"style"`
	Examples        []VoiceExample                    `json:"examples" yaml:"examples"`
	Locales         map[model.LocaleID]LocaleOverride `json:"locales,omitempty" yaml:"locales,omitempty"`
	Channels        map[string]ChannelOverride        `json:"channels,omitempty" yaml:"channels,omitempty"`
	Personas        map[string]PersonaOverride        `json:"personas,omitempty" yaml:"personas,omitempty"`
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

	// carried are the word rules the file the profile was read from carries
	// beside it: a starter pack's terms, or a voice file's `terms:` list. They
	// are never stored with the profile and never serialized.
	carried CarriedTerms
}

// CarriedTerms are the word rules a voice file carries beside its voice, and
// where they come from ("pack technical-docs", or "voice file").
type CarriedTerms struct {
	From  string
	Rules []TermRule
}

// CarriedTerms returns the word rules the file the profile was read from
// carries. A profile from a store carries none: its words are terms in the
// terms store. A nil profile carries none.
func (p *VoiceProfile) CarriedTerms() CarriedTerms {
	if p == nil {
		return CarriedTerms{}
	}
	return p.carried
}

// VoiceOnly returns the profile without the word rules its file carries: the
// voice alone, for a check that holds those rules as terms. A nil profile
// returns nil.
func (p *VoiceProfile) VoiceOnly() *VoiceProfile {
	if p == nil || len(p.carried.Rules) == 0 {
		return p
	}
	c := *p
	c.carried = CarriedTerms{}
	return &c
}

// Carry attaches word rules to the profile, naming where they come from, and
// returns the profile.
func (p *VoiceProfile) Carry(from string, rules []TermRule) *VoiceProfile {
	if p != nil {
		p.carried = CarriedTerms{From: from, Rules: rules}
	}
	return p
}

// DefaultMinScore is the compliance bar applied when a profile does not set its
// own MinScore: one failing finding (25-point penalty) already drops a
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
// the evaluation flow touches (tone, style patterns, carried terms, examples,
// locale/channel overrides), so a candidate profile can be built and
// mutated without affecting the baseline. Returns nil for a nil receiver.
func (p *VoiceProfile) Clone() *VoiceProfile {
	if p == nil {
		return nil
	}
	c := *p
	c.Constraints = cloneConstraints(p.Constraints)
	c.Tone.Personality = append([]string(nil), p.Tone.Personality...)
	c.Style.ProhibitedPatterns = append([]Pattern(nil), p.Style.ProhibitedPatterns...)
	c.Style.RequiredPatterns = append([]Pattern(nil), p.Style.RequiredPatterns...)
	c.Style.Comments = p.Style.Comments.clone()
	c.carried = CarriedTerms{From: p.carried.From, Rules: cloneRules(p.carried.Rules)}
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

// AllForms is the term and every other shape the rule declares, with blanks and
// duplicates removed. It is what the matcher scans for.
func (r TermRule) AllForms() []string {
	out := make([]string, 0, len(r.Forms)+1)
	seen := map[string]bool{}
	for _, f := range append([]string{r.Term}, r.Forms...) {
		f = strings.TrimSpace(f)
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// cloneRules copies a rule list and each rule's Forms, so a candidate profile
// built for evaluation cannot write through to the baseline's slices.
func cloneRules(in []TermRule) []TermRule {
	if in == nil {
		return nil
	}
	out := make([]TermRule, len(in))
	copy(out, in)
	for i := range out {
		out[i].Forms = append([]string(nil), in[i].Forms...)
		out[i].ReplacementForms = append([]string(nil), in[i].ReplacementForms...)
		if in[i].Accepted != nil {
			out[i].Accepted = make([]Rendering, len(in[i].Accepted))
			for j, a := range in[i].Accepted {
				out[i].Accepted[j] = Rendering{Text: a.Text, Forms: append([]string(nil), a.Forms...)}
			}
		}
	}
	return out
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
	// Comments are the limits a check holds code comments to. Nil asks for no
	// comment checks.
	Comments *CommentRules `json:"comments,omitempty" yaml:"comments,omitempty"`
}

// Pattern describes a regex-based text pattern rule.
type Pattern struct {
	Regex       string `json:"regex" yaml:"regex"`
	Description string `json:"description" yaml:"description"`
	// Advisory makes a match report without failing a check. A pattern rule
	// fails where the voice is bound unless it is marked advisory.
	Advisory bool `json:"advisory,omitempty" yaml:"advisory,omitempty"`

	// Rate turns a prohibition into a ceiling: not "never" but "not this
	// often".
	//
	// Most of what is actually measured about writing is a density over a
	// window, and a regex matches without counting. This repository's own
	// CLAUDE.md states a rule in exactly that shape — "one per 1,000 words is
	// the ceiling" — which a profile could not express. Vale needed
	// `occurrence` as a first-class check for the same reason. See #2242.
	//
	// Nil means every match is a violation, which is what every existing rule
	// says and keeps saying.
	Rate *PatternRate `json:"rate,omitempty" yaml:"rate,omitempty"`

	// Scope limits where the pattern applies. Empty means everywhere, which is
	// what existing rules do.
	//
	// `prose` is what most vocabulary rules want and cannot currently say: a
	// ban on implementation words should not fire inside a code sample that
	// necessarily contains them. Vale skips code by default; this asks, because
	// changing what an existing rule matches is not a thing to do silently.
	Scope string `json:"scope,omitempty" yaml:"scope,omitempty"` // "", "prose", "code", "heading"

	// NotAfter, when set, is a regular expression tested against the text before
	// a match, which is not a violation when it matches there. Go's regular
	// expressions have no lookbehind, so a rule whose meaning turns on the
	// preceding words says so here: the past-habitual "used to" is a
	// violation, and "is used to" and "an id, used to flag" are not. Anchor it
	// with `$` to the end of that text.
	NotAfter string `json:"not_after,omitempty" yaml:"not_after,omitempty"`
}

// PatternRate is a density ceiling: at most Max matches per Per words.
//
// Reporting is all-or-nothing on purpose. Under the ceiling nothing is
// reported, because the rule permits it. Over the ceiling every match is
// reported rather than an arbitrary "excess" subset, because which occurrence
// is the excess one is not a question the text can answer, and a writer fixing
// the document wants to see all of them.
type PatternRate struct {
	// Max is how many matches are allowed in each Per words. Zero means none,
	// which is the same as having no rate at all and is rejected in validation
	// rather than silently meaning something.
	Max int `json:"max" yaml:"max"`
	// Per is the window in words. Zero defaults to DefaultRateWindow.
	Per int `json:"per_words,omitempty" yaml:"per_words,omitempty"`
}

// DefaultRateWindow is the window a rate uses when it names none. A thousand
// words is the unit the style guides that state rates state them in.
const DefaultRateWindow = 1000

// Allowance is how many matches this rate permits in a text of n words.
//
// Rounded up, and never below Max: a 300-word document under a
// "one per 1000 words" rule allows one, not zero. A ceiling that tightens as
// the text gets shorter would fail a paragraph for what it permits in a page.
func (r PatternRate) Allowance(words int) int {
	per := r.Per
	if per <= 0 {
		per = DefaultRateWindow
	}
	allowed := r.Max * words / per
	if r.Max*words%per != 0 {
		allowed++
	}
	return max(allowed, r.Max)
}

// Pattern scopes.
const (
	// ScopeProse excludes fenced blocks and inline code spans. What a rule
	// about wording almost always means.
	ScopeProse = "prose"
	// ScopeCode is only inside them, for a rule about samples.
	ScopeCode = "code"
	// ScopeHeading is only headings, where a house style often differs from
	// the body's.
	ScopeHeading = "heading"
)

// TermRule is one "write this, not that" rule: the form to reject (Term and
// its Forms), the form to use (Replacement), a note, and whether a use of the
// rejected form fails a check. A rule with a Replacement and no Term names a
// preferred form and rejects nothing.
type TermRule struct {
	Term        string `json:"term,omitempty" yaml:"term,omitempty"`
	Replacement string `json:"replacement,omitempty" yaml:"replacement,omitempty"`
	Note        string `json:"note,omitempty" yaml:"note,omitempty"`
	// Advisory makes a use of the term report without failing a check. A rule
	// fails unless it is marked advisory.
	Advisory bool `json:"advisory,omitempty" yaml:"advisory,omitempty"`
	// Competitor marks the rejected form as a competitor's name.
	Competitor bool `json:"competitor,omitempty" yaml:"competitor,omitempty"`
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
	// Forms are the other surface shapes this term takes: inflections,
	// declensions, whatever the language does to it. Matched alongside Term.
	//
	// Declared rather than derived. A rule about `utilize` means `utilizes`
	// too, and matching the bare stem alone let "the platform utilizes your
	// data" through at 100/100 (#2226) — but the shapes a word takes are
	// per-language knowledge, and generating them from English suffix rules
	// produced non-words for Norwegian and reached none of the forms it
	// actually uses.
	//
	// `kapi terms expand` fills these in, asking a model once in the terms'
	// own language and writing the result for review. The knowledge is the
	// model's; the matching stays exact and language-neutral.
	Forms []string `json:"forms,omitempty" yaml:"forms,omitempty"`

	// CaseSensitive, when set, says whether the term and its forms match in
	// their own casing. Unset, MatchesCase decides from the rule itself.
	CaseSensitive *bool `json:"case_sensitive,omitempty" yaml:"case_sensitive,omitempty"`

	// Scope limits where the rule applies, with the same values a Pattern uses.
	// Empty means everywhere, which is what every existing rule does.
	//
	// A word rule about implementation vocabulary says "prose" here, so it
	// does not fire inside the code sample the document exists to explain.
	Scope string `json:"scope,omitempty" yaml:"scope,omitempty"`

	// ReplacementForms are the surface forms Replacement takes in the language
	// it is written in: the Norwegian plural "varsler" for "varsel". A check
	// that holds a text to Replacement accepts any of them as Replacement.
	ReplacementForms []string `json:"replacement_forms,omitempty" yaml:"replacement_forms,omitempty"`

	// Accepted are further renderings that satisfy the rule, each with its own
	// forms: the admitted and approved terms a concept carries beside its
	// preferred one. Replacement stays the wording a translation is asked to
	// use, and a check accepts Replacement or any of these.
	Accepted []Rendering `json:"accepted,omitempty" yaml:"accepted,omitempty"`
}

// MatchesCase reports whether the rule matches its term and forms in their own
// casing. CaseSensitive answers when it is set. Otherwise a rule matches case
// sensitively when the form it asks for is capitalised, as a product or
// feature name is ("write Quickcast, not QuickCast" must not fire on the
// lower-case slug "quickcast"), or when its whole content is capitalisation
// (a rejected form that differs from the preferred one only in case, such as
// `term: Ripgrep, replacement: ripgrep`).
func (r TermRule) MatchesCase() bool {
	if r.CaseSensitive != nil {
		return *r.CaseSensitive
	}
	preferred := strings.TrimSpace(r.Replacement)
	if preferred == "" {
		return false
	}
	if strings.ToLower(preferred) != preferred {
		return true
	}
	for _, f := range r.AllForms() {
		if strings.EqualFold(f, preferred) && f != preferred {
			return true
		}
	}
	return false
}

// Rendering is one acceptable wording for what a rule requires, with the
// surface forms it takes.
type Rendering struct {
	Text  string   `json:"text" yaml:"text"`
	Forms []string `json:"forms,omitempty" yaml:"forms,omitempty"`
}

// Renderings is every wording that satisfies the rule: Replacement with its
// forms first, then each accepted rendering, blanks and repeats removed. It is
// empty when the rule names no replacement.
func (r TermRule) Renderings() []Rendering {
	if strings.TrimSpace(r.Replacement) == "" {
		return nil
	}
	out := []Rendering{{Text: strings.TrimSpace(r.Replacement), Forms: r.ReplacementForms}}
	seen := map[string]bool{strings.ToLower(out[0].Text): true}
	for _, a := range r.Accepted {
		text := strings.TrimSpace(a.Text)
		if text == "" || seen[strings.ToLower(text)] {
			continue
		}
		seen[strings.ToLower(text)] = true
		out = append(out, Rendering{Text: text, Forms: a.Forms})
	}
	return out
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
	Formality        string         `json:"formality,omitempty" yaml:"formality,omitempty"`
	Humor            string         `json:"humor,omitempty" yaml:"humor,omitempty"`
	PersonPOV        string         `json:"person_pov,omitempty" yaml:"person_pov,omitempty"`
	CulturalNotes    string         `json:"cultural_notes,omitempty" yaml:"cultural_notes,omitempty"`
	ExampleOverrides []VoiceExample `json:"example_overrides,omitempty" yaml:"example_overrides,omitempty"`
}

// ChannelOverride provides channel-specific adjustments to a voice profile:
// a tone and style that replace the resolved ones where the channel applies.
type ChannelOverride struct {
	Tone  *ToneProfile `json:"tone,omitempty" yaml:"tone,omitempty"`
	Style *StyleRules  `json:"style,omitempty" yaml:"style,omitempty"`
}

// PersonaOverride layers an individual author's voice on top of a profile: a
// tone and style that replace the resolved ones, applied after any channel's,
// so a persona's win over a channel's.
type PersonaOverride struct {
	Tone  *ToneProfile `json:"tone,omitempty" yaml:"tone,omitempty"`
	Style *StyleRules  `json:"style,omitempty" yaml:"style,omitempty"`
}
