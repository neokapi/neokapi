package contextop

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/profile"
)

// ObservedRule is the term rule an observation states: the project writes
// `term`, and avoids the forms given in insteadOf plus the variants
// [AvoidedForms] derives from them. The first avoided form is the rule's term
// and the rest are its forms.
//
// A rule whose avoided forms differ from the term only in case matches as
// written, so the rule about `quickcast` never fires on `Quickcast`. With no
// form to avoid, the rule records the term alone, which advises nothing.
func ObservedRule(term string, insteadOf []string) profile.TermRule {
	term = strings.TrimSpace(term)
	forms := AvoidedForms(term, insteadOf)
	if len(forms) == 0 {
		return profile.TermRule{Term: term}
	}
	rule := profile.TermRule{Term: forms[0], Forms: forms[1:], Replacement: term}
	for _, form := range forms {
		if strings.EqualFold(form, term) {
			rule.CaseSensitive = true
			break
		}
	}
	return rule
}

// AvoidedForms lists the forms a project that writes `term` avoids: the forms
// given in insteadOf, then the spacing, hyphen and case variants of a
// compound. It is deterministic and derives only plausible variants:
//
//   - A compound's parts come from the term's own spaces, hyphens and case
//     changes (`KapiDesktop`, `Kapi Desktop`), or, for a closed word such as
//     `Quickcast`, from an insteadOf form that spells the same letters with a
//     break (`Quick cast`). A word with neither has no parts to vary.
//   - A compound yields the spaced form, the hyphenated form and, when the
//     term is capitalised or written closed, the closed form and the form with
//     every part capitalised: `Quickcast` gives `Quick cast`, `Quick-cast` and
//     `QuickCast`. A lower-case phrase such as `content memory` yields only
//     `content-memory`, because nobody writes it closed.
//   - A capitalised term yields its lower-case spelling: `Quickcast` gives
//     `quickcast`.
//
// The term itself is never among the forms, and each form is listed once.
func AvoidedForms(term string, insteadOf []string) []string {
	term = strings.TrimSpace(term)
	if term == "" {
		return nil
	}
	var out []string
	add := func(form string) {
		form = strings.TrimSpace(form)
		if form == "" || form == term {
			return
		}
		for _, held := range out {
			if held == form {
				return
			}
		}
		out = append(out, form)
	}
	for _, form := range insteadOf {
		add(form)
	}

	parts := compoundParts(term)
	if len(parts) < 2 {
		for _, form := range insteadOf {
			if split := splitLike(term, form); len(split) >= 2 {
				parts = split
				break
			}
		}
	}
	capitalised := startsUpper(term)
	if len(parts) >= 2 {
		spaced := strings.ContainsAny(term, " -_")
		add(strings.Join(parts, " "))
		add(strings.Join(parts, "-"))
		if capitalised || !spaced {
			add(strings.Join(parts, ""))
		}
		if capitalised {
			titled := make([]string, len(parts))
			for i, p := range parts {
				titled[i] = upperFirst(p)
			}
			add(strings.Join(titled, ""))
		}
	}
	if capitalised {
		add(strings.ToLower(term))
	}
	return out
}

// compoundParts splits a term at its spaces, hyphens and underscores, and at
// each change from a lower-case letter to an upper-case one.
func compoundParts(term string) []string {
	var parts []string
	for _, word := range strings.FieldsFunc(term, func(r rune) bool { return r == ' ' || r == '-' || r == '_' }) {
		start := 0
		var prev rune
		for i, r := range word {
			if i > 0 && unicode.IsLower(prev) && unicode.IsUpper(r) {
				parts = append(parts, word[start:i])
				start = i
			}
			prev = r
		}
		parts = append(parts, word[start:])
	}
	return parts
}

// splitLike splits a closed term where another spelling of the same letters
// breaks: `Quickcast` split like `Quick cast` is `Quick` and `cast`. It
// answers nil when the two do not spell the same letters.
func splitLike(term, form string) []string {
	formParts := compoundParts(form)
	if len(formParts) < 2 || !strings.EqualFold(strings.Join(formParts, ""), term) {
		return nil
	}
	out := make([]string, 0, len(formParts))
	rest := term
	for _, p := range formParts {
		n := utf8.RuneCountInString(p)
		cut := 0
		for i := 0; i < n; i++ {
			_, size := utf8.DecodeRuneInString(rest[cut:])
			cut += size
		}
		out = append(out, rest[:cut])
		rest = rest[cut:]
	}
	return out
}

// startsUpper reports whether a term begins with an upper-case letter.
func startsUpper(term string) bool {
	r, _ := utf8.DecodeRuneInString(term)
	return unicode.IsUpper(r)
}

// upperFirst capitalises the first letter of a word and leaves the rest.
func upperFirst(word string) string {
	r, size := utf8.DecodeRuneInString(word)
	if size == 0 {
		return word
	}
	return string(unicode.ToUpper(r)) + word[size:]
}
