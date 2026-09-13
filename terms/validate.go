package terms

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// A Problem is one finding about a set of concepts. An error makes a concept
// unusable to a store or a check; a warning names something a check will read
// differently from what the author meant.
type Problem struct {
	ConceptID string         `json:"concept_id,omitempty"`
	Locale    model.LocaleID `json:"locale,omitempty"`
	Term      string         `json:"term,omitempty"`
	Message   string         `json:"message"`
	Warning   bool           `json:"warning,omitempty"`
}

// ValidateConcepts reports the problems in a set of concepts, in concept order.
// sourceLocale is the language the source terms are written in, and a term in
// any other language is a target term.
//
// Errors: a concept with no id or an id used twice, a term with no text or no
// locale, a status outside the lifecycle vocabulary.
//
// Warnings concern forms. A target term in a language that inflects, with no
// forms declared, is recognised only where a rendering contains its exact
// spelling, so "Varsler" is not a use of "varsel". A form spelled the same as
// another term in the same language makes one word a use of two entries.
// Terms of a do-not-translate concept, and deprecated or forbidden terms, need
// no forms: the first stay as written, and the others are never a required
// rendering.
func ValidateConcepts(concepts []Concept, sourceLocale model.LocaleID) []Problem {
	type spelling struct{ conceptID, text string }
	spellings := map[string]map[string]spelling{}
	for _, c := range concepts {
		for _, t := range c.Terms {
			lang := BaseLanguage(t.Locale)
			key := strings.ToLower(strings.TrimSpace(t.Text))
			if lang == "" || key == "" {
				continue
			}
			if spellings[lang] == nil {
				spellings[lang] = map[string]spelling{}
			}
			if _, ok := spellings[lang][key]; !ok {
				spellings[lang][key] = spelling{c.ID, t.Text}
			}
		}
	}

	sourceLang := BaseLanguage(sourceLocale)
	var out []Problem
	seenID := map[string]bool{}
	for _, c := range concepts {
		switch {
		case strings.TrimSpace(c.ID) == "":
			out = append(out, Problem{Message: "a concept has no id"})
		case seenID[c.ID]:
			out = append(out, Problem{ConceptID: c.ID, Message: "the concept id is used more than once"})
		}
		seenID[c.ID] = true

		for _, t := range c.Terms {
			at := Problem{ConceptID: c.ID, Locale: t.Locale, Term: t.Text}
			switch {
			case strings.TrimSpace(t.Text) == "":
				at.Message = "a term has no text"
				out = append(out, at)
				continue
			case t.Locale.IsEmpty():
				at.Message = "the term has no locale"
				out = append(out, at)
				continue
			case t.Status != "" && !KnownTermStatus(t.Status):
				at.Message = fmt.Sprintf("unknown status %q", t.Status)
				out = append(out, at)
			}

			lang := BaseLanguage(t.Locale)
			forms := NormalizeForms(t.Text, t.Forms)
			for _, form := range forms {
				other, ok := spellings[lang][strings.ToLower(form)]
				if !ok {
					continue
				}
				w := at
				w.Warning = true
				w.Message = fmt.Sprintf("the form %q is spelled the same as the term %q of concept %s; declare the word as one or the other", form, other.text, other.conceptID)
				out = append(out, w)
			}

			if len(forms) == 0 && lang != sourceLang && !c.DoNotTranslate &&
				t.Status != model.TermDeprecated && t.Status != model.TermForbidden &&
				LanguageInflects(t.Locale) {
				w := at
				w.Warning = true
				w.Message = fmt.Sprintf("no forms declared in %s, a language that inflects, so only renderings that contain %q are recognised as this term", lang, t.Text)
				out = append(out, w)
			}
		}
	}
	return out
}

// BaseLanguage is the language subtag of a locale, lowercased: "nb" for nb-NO.
// It is "" for an empty locale.
func BaseLanguage(loc model.LocaleID) string {
	s := string(model.NormalizeLocale(loc))
	if i := strings.IndexAny(s, "-_"); i > 0 {
		s = s[:i]
	}
	return strings.ToLower(s)
}

// uninflected are the languages whose nouns keep their written shape: number
// and case are carried by separate words, particles or repetition, all of
// which sit outside the word, so a rendering contains the term as written.
var uninflected = map[string]bool{
	"zh": true, "yue": true, "wuu": true,
	"ja": true, "ko": true,
	"vi": true, "th": true, "lo": true, "km": true, "my": true,
	"id": true, "ms": true,
}

// LanguageInflects reports whether a term in the locale's language can appear
// in content in a shape that does not contain its written form, so that the
// term needs declared forms to be recognised there. It is false for the
// languages in uninflected, and for a locale that names no natural language: an
// empty one, und, mul, zxx, and the private-use range qaa-qtz, where pseudo
// locales such as qps live.
func LanguageInflects(loc model.LocaleID) bool {
	lang := BaseLanguage(loc)
	switch lang {
	case "", "und", "mul", "zxx":
		return false
	}
	if len(lang) == 3 && lang[0] == 'q' && lang[1] >= 'a' && lang[1] <= 't' {
		return false
	}
	return !uninflected[lang]
}
