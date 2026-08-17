package tools

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/icu"
	"github.com/neokapi/neokapi/core/model"
)

// placeholderToken matches the common interpolation/placeholder styles seen in
// multilingual content, longest/most-specific forms first so "{{x}}" wins over
// "{x}". Covered: {{name}}, ${name}, %(name)s (Python), %1$s (positional),
// %s/%d/%@ (printf/ObjC), {name}/{0} (ICU/.NET), <0>…</0> (numbered tags).
var placeholderToken = regexp.MustCompile(
	`\{\{[^{}]+\}\}|\$\{[^{}]+\}|%\([^)]+\)[a-zA-Z]|%\d+\$[a-zA-Z]|%[sdifeEgGxXobpqv@%]|\{[^{}]+\}|</?[0-9]+>`)

// nonBracePlaceholderToken is placeholderToken without the brace forms: the
// styles that can sit inside an ICU message as ordinary text, where the ICU
// parse steps over them.
var nonBracePlaceholderToken = regexp.MustCompile(
	`%\([^)]+\)[a-zA-Z]|%\d+\$[a-zA-Z]|%[sdifeEgGxXobpqv@%]|</?[0-9]+>`)

// placeholderDelta is one placeholder whose presence differs between a source
// string and its translation.
//
// SourceCount and TargetCount carry occurrence counts when the comparison was
// by count. Counted is false where the comparison was by presence: inside an
// ICU plural or select, how many times a placeholder occurs is a property of
// the language — Norwegian writes two branches where Polish writes four — so
// only whether it survived at all can be asked.
type placeholderDelta struct {
	Token       string
	SourceCount int
	TargetCount int
	Counted     bool
}

// missingMessage is the finding text for a placeholder the translation dropped.
func (d placeholderDelta) missingMessage(locale model.LocaleID) string {
	if d.Counted {
		return fmt.Sprintf("Placeholder %s is missing from the %s target (source %d×, target %d×)",
			d.Token, locale, d.SourceCount, d.TargetCount)
	}
	return fmt.Sprintf("Placeholder %s is missing from the %s target", d.Token, locale)
}

// extraMessage is the finding text for a placeholder the translation invented.
func (d placeholderDelta) extraMessage(locale model.LocaleID) string {
	return fmt.Sprintf("Placeholder %s appears in the %s target but not the source", d.Token, locale)
}

// comparePlaceholders reports the interpolation placeholders a translation
// dropped and, when flagExtra is set, the ones it invented.
//
// When either side is an ICU message that chooses between sub-messages — a
// plural, select or selectordinal — the comparison is structural: what a
// translation owes is the argument name, the picker keyword and offset, the #
// a plural formats, and every simple argument. The sub-message text is what a
// translator writes, and the set of categories is what the target language
// requires, so neither is compared. Every other string keeps the literal
// comparison, which is the right one for the interpolation styles ICU does not
// describe.
func comparePlaceholders(source, target string, flagExtra bool) (missing, extra []placeholderDelta) {
	if src, tgt, ok := icuPickerTokens(source, target); ok {
		return compareICUPlaceholders(src, tgt, source, target, flagExtra)
	}
	return compareLiteralPlaceholders(source, target, flagExtra)
}

// icuPickerTokens parses both sides as ICU MessageFormat, and reports ok only
// when both parse and at least one of them carries a picker. A message with no
// picker gains nothing from the structural comparison and would lose the
// non-ICU styles the literal comparison covers.
func icuPickerTokens(source, target string) (src, tgt icu.Tokens, ok bool) {
	src, err := icu.PlaceholderTokens(source)
	if err != nil {
		return src, tgt, false
	}
	tgt, err = icu.PlaceholderTokens(target)
	if err != nil {
		return src, tgt, false
	}
	return src, tgt, src.Picker || tgt.Picker
}

func compareICUPlaceholders(src, tgt icu.Tokens, source, target string, flagExtra bool) (missing, extra []placeholderDelta) {
	// The frame the picker sits in is compared by count: an occurrence dropped
	// there is an occurrence lost.
	for _, tok := range sortedKeys(src.Frame) {
		if n := tgt.Frame[tok]; n < src.Frame[tok] {
			missing = append(missing, placeholderDelta{
				Token: tok, SourceCount: src.Frame[tok], TargetCount: n, Counted: true,
			})
		}
	}
	// What appears inside a sub-message is compared by presence, and a token
	// that moved between the frame and a sub-message has not been lost.
	for _, tok := range sortedKeys(src.Inner) {
		if !tgt.Inner[tok] && tgt.Frame[tok] == 0 {
			missing = append(missing, placeholderDelta{Token: tok})
		}
	}
	// Placeholders from other interpolation styles are text as far as ICU is
	// concerned, and a sub-message may hold one. They are compared by presence
	// for the same reason the inner tokens are.
	srcOther := countMatches(nonBracePlaceholderToken, source)
	tgtOther := countMatches(nonBracePlaceholderToken, target)
	for _, tok := range sortedKeys(srcOther) {
		if tgtOther[tok] == 0 {
			missing = append(missing, placeholderDelta{Token: tok})
		}
	}

	if !flagExtra {
		return missing, nil
	}
	for _, tok := range sortedKeys(tgt.Frame) {
		if src.Frame[tok] == 0 && !src.Inner[tok] {
			extra = append(extra, placeholderDelta{Token: tok})
		}
	}
	for _, tok := range sortedKeys(tgt.Inner) {
		if !src.Inner[tok] && src.Frame[tok] == 0 {
			extra = append(extra, placeholderDelta{Token: tok})
		}
	}
	for _, tok := range sortedKeys(tgtOther) {
		if srcOther[tok] == 0 {
			extra = append(extra, placeholderDelta{Token: tok})
		}
	}
	return missing, extra
}

func compareLiteralPlaceholders(source, target string, flagExtra bool) (missing, extra []placeholderDelta) {
	srcCounts := countMatches(placeholderToken, source)
	tgtCounts := countMatches(placeholderToken, target)
	for _, tok := range sortedKeys(srcCounts) {
		if tgtCounts[tok] < srcCounts[tok] {
			missing = append(missing, placeholderDelta{
				Token: tok, SourceCount: srcCounts[tok], TargetCount: tgtCounts[tok], Counted: true,
			})
		}
	}
	if !flagExtra {
		return missing, nil
	}
	for _, tok := range sortedKeys(tgtCounts) {
		if srcCounts[tok] == 0 {
			extra = append(extra, placeholderDelta{Token: tok})
		}
	}
	return missing, extra
}

// PlaceholdersCarried reports whether a translation still carries every
// interpolation placeholder its source carries.
//
// The read-side counterpart of the check: a caller deciding whether a
// source→target pairing is worth keeping asks this rather than assembling
// findings for a reader. Only what the target *dropped* is asked about — an
// invented placeholder is a defect a check reports, but it does not make the
// pairing a translation of some other sentence.
func PlaceholdersCarried(source, target string) bool {
	missing, _ := comparePlaceholders(source, target, false)
	return len(missing) == 0
}

// placeholderFindings reports the placeholder integrity of one translation as
// check findings: dropped placeholders are critical, because a program that
// interpolates into a string that lost its slot breaks where the reader is
// looking; invented ones are major.
func placeholderFindings(locale model.LocaleID, source, target string) []check.Finding {
	missing, extra := comparePlaceholders(source, target, true)
	var findings []check.Finding
	for _, d := range missing {
		findings = append(findings, check.Finding{
			Category:     "placeholder",
			Severity:     check.SeverityCritical,
			Message:      d.missingMessage(locale),
			Suggestion:   fmt.Sprintf("Keep %s in the target", d.Token),
			OriginalText: d.Token,
		})
	}
	for _, d := range extra {
		findings = append(findings, check.Finding{
			Category:     "placeholder",
			Severity:     check.SeverityMajor,
			Message:      d.extraMessage(locale),
			OriginalText: d.Token,
		})
	}
	return findings
}

func countMatches(re *regexp.Regexp, s string) map[string]int {
	counts := map[string]int{}
	for _, m := range re.FindAllString(s, -1) {
		counts[m]++
	}
	return counts
}

// sortedKeys returns a map's keys in deterministic order, so findings arrive in
// the same order on every run.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
