package comments

import (
	"regexp"
	"strings"
)

// directiveForm is one kind of comment a tool reads as an instruction. match
// returns the name recorded on the exclusion.
//
// Each form is tested on its own: the directive fixture holds a comment only
// that form matches, so dropping any one of them lets a directive through as
// prose and fails the test.
type directiveForm struct {
	name  string
	match func(t commentText) (form string, ok bool)
}

// leadingWord returns the run of word characters a line opens with. A word may
// hold letters, digits, `@`, `#`, `_` and `-`, so `@ts-ignore:` yields
// `@ts-ignore`.
func leadingWord(line string) string {
	end := 0
	for end < len(line) {
		c := line[end]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '@' || c == '#' || c == '_' || c == '-' {
			end++
			continue
		}
		break
	}
	return line[:end]
}

// wordForm matches a comment whose first line opens with one of words.
func wordForm(name string, words ...string) directiveForm {
	return directiveForm{name: name, match: func(t commentText) (string, bool) {
		w := leadingWord(t.first())
		for _, want := range words {
			if w == want {
				return w, true
			}
		}
		return "", false
	}}
}

// prefixForm matches a comment whose first line opens with a word that starts
// with one of prefixes. The form is that word.
func prefixForm(name string, prefixes ...string) directiveForm {
	return directiveForm{name: name, match: func(t commentText) (string, bool) {
		w := leadingWord(t.first())
		for _, p := range prefixes {
			if strings.HasPrefix(w, p) {
				return w, true
			}
		}
		return "", false
	}}
}

// regexpForm matches a comment whose first line matches re, and records form.
func regexpForm(name, form string, kinds []unitKind, re *regexp.Regexp) directiveForm {
	return directiveForm{name: name, match: func(t commentText) (string, bool) {
		if kinds != nil && !hasKind(kinds, t.kind) {
			return "", false
		}
		return form, re.MatchString(t.first())
	}}
}

func hasKind(kinds []unitKind, k unitKind) bool {
	for _, want := range kinds {
		if want == k {
			return true
		}
	}
	return false
}
