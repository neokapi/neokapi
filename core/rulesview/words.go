package rulesview

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/profile"
)

// SuggestActor is who files what the suggestions pass sees: kapi itself. An
// agent's notes carry the agent; these carry kapi.
var SuggestActor = contextop.Actor{Kind: contextop.ActorTool, Name: "kapi"}

// ThemeOf sorts a subject into a suggestion theme.
func ThemeOf(s contextop.Subject) string {
	switch s.Kind {
	case contextop.SubjectMemory:
		return ThemeMemory
	case contextop.SubjectNote:
		return ThemeWriting
	}
	rule, ok := s.Rule()
	if !ok || rule.Replacement == "" {
		return ThemeWords
	}
	want := SpellingKey(rule.Replacement)
	for _, form := range append([]string{rule.Term}, rule.Forms...) {
		if SpellingKey(form) == want {
			return ThemeNames
		}
	}
	return ThemeWords
}

// SpellingKey folds case, spacing and joining punctuation, so two spellings of
// one name compare equal.
func SpellingKey(form string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(form) {
		switch r {
		case ' ', '-', '_', '.', ' ':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Sentence states a subject as the sentence a person reads.
func Sentence(s contextop.Subject) string {
	switch s.Kind {
	case contextop.SubjectTerm:
		rule, ok := s.Rule()
		if !ok {
			return ""
		}
		avoid := OrList(append([]string{rule.Term}, rule.Forms...))
		if rule.Replacement == "" {
			return "Avoid " + avoid + "."
		}
		return "Write " + rule.Replacement + ", not " + avoid + "."
	case contextop.SubjectMemory:
		if s.Memory == nil {
			return ""
		}
		return fmt.Sprintf("Translate %q into %s as %q.", s.Memory.Source, s.Memory.TargetLocale, s.Memory.Target)
	case contextop.SubjectNote:
		return s.Text
	}
	return ""
}

// OrList joins forms the way a sentence lists alternatives: "a", "a or b",
// "a, b or c".
func OrList(forms []string) string {
	switch len(forms) {
	case 0:
		return ""
	case 1:
		return forms[0]
	}
	return strings.Join(forms[:len(forms)-1], ", ") + " or " + forms[len(forms)-1]
}

// SeenBy returns the records as the person reading them sees them: what they
// did themselves carries no name, so it reads as "you", and what a teammate
// did carries the teammate's. The records are copied, never changed in place.
// A reader who is not a named person sees the records as they are.
func SeenBy(records []contextop.Record, reader contextop.Actor) []contextop.Record {
	if reader.Kind != contextop.ActorPerson || reader.Name == "" {
		return records
	}
	out := make([]contextop.Record, len(records))
	for i, r := range records {
		if r.Actor.Kind == contextop.ActorPerson && r.Actor.Name == reader.Name {
			r.Actor.Name = ""
		}
		out[i] = r
	}
	return out
}

// ActorPhrase names an actor in a sentence. A person with no name is the
// person reading, so "you"; possessive asks for the form that precedes a noun.
func ActorPhrase(a contextop.Actor, possessive bool) string {
	switch a.Kind {
	case contextop.ActorPerson, "":
		if a.Name == "" {
			return "you"
		}
		if possessive {
			return a.Name + "'s"
		}
		return a.Name
	case contextop.ActorAgent:
		name := a.Name
		if name == "" {
			name = "an agent"
		}
		if possessive {
			return name + "'s"
		}
		return name
	}
	name := "kapi"
	if a.Name != "" && a.Name != SuggestActor.Name {
		name += " " + a.Name
	}
	if possessive {
		return name + "'s"
	}
	return name
}

// FormMatcher matches any of the forms as a whole word. It returns nil when
// no form has a letter to match.
func FormMatcher(forms []string, caseSensitive bool) *regexp.Regexp {
	var alts []string
	for _, f := range forms {
		if f = strings.TrimSpace(f); f != "" {
			alts = append(alts, regexp.QuoteMeta(f))
		}
	}
	if len(alts) == 0 {
		return nil
	}
	flags := ""
	if !caseSensitive {
		flags = "(?i)"
	}
	return regexp.MustCompile(flags + `(?:^|[^\p{L}\p{N}_])(?:` + strings.Join(alts, "|") + `)(?:$|[^\p{L}\p{N}_])`)
}

// EditSubject applies a keep's edits to the rule being kept, and reports
// whether anything moved: replacement becomes the wording the rule says to
// write, and advisory, when set, its marking. A content-memory pair and a note
// carry no replacement or advisory marking, so an edit of either is a no-op.
func EditSubject(subject contextop.Subject, replacement string, advisory *bool) (contextop.Subject, bool) {
	if replacement == "" && advisory == nil {
		return subject, false
	}
	apply := func(rule *profile.TermRule) bool {
		changed := false
		if replacement != "" && rule.Replacement != replacement {
			*rule = withPreferred(*rule, replacement)
			changed = true
		}
		if advisory != nil && rule.Advisory != *advisory {
			rule.Advisory, changed = *advisory, true
		}
		return changed
	}
	if subject.Kind == contextop.SubjectTerm && subject.Term != nil {
		rule := *subject.Term
		if apply(&rule) {
			subject.Term = &rule
			return subject, true
		}
	}
	return subject, false
}

// withPreferred makes form the wording a rule says to write. A form the rule
// avoided trades places with the wording it said to write before, so a name
// kept as another of its spellings avoids the spelling the suggestion
// preferred and never avoids the one it now prefers.
func withPreferred(rule profile.TermRule, form string) profile.TermRule {
	avoided := append([]string{rule.Term}, rule.Forms...)
	if i := slices.Index(avoided, form); i >= 0 {
		if rule.Replacement != "" {
			avoided[i] = rule.Replacement
		} else {
			avoided = slices.Delete(avoided, i, i+1)
		}
		if len(avoided) > 0 {
			rule.Term, rule.Forms = avoided[0], slices.Clip(avoided[1:])
			if len(rule.Forms) == 0 {
				rule.Forms = nil
			}
		}
	}
	rule.Replacement = form
	return rule
}
