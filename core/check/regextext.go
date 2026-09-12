package check

import (
	"regexp"
	"regexp/syntax"
	"strings"
)

// maxCanaryRepeat bounds how many times a counted repetition is written out, so
// a pattern such as `a{100000}` cannot make a canary of unbounded size.
const maxCanaryRepeat = 1000

// TextMatching returns a short text in which re finds a non-empty match, for a
// canary that a forbidden pattern must flag. It builds candidates from the
// pattern's syntax tree and keeps the first one re actually matches, so a
// returned text is never a guess. ok is false when no candidate matches, as for
// a pattern that can only match the empty string.
func TextMatching(re *regexp.Regexp) (string, bool) {
	parsed, err := syntax.Parse(re.String(), syntax.Perl)
	if err != nil {
		return "", false
	}
	parsed = parsed.Simplify()
	var bases []string
	if parsed.Op == syntax.OpAlternate {
		for _, sub := range parsed.Sub {
			if s, ok := generate(sub); ok {
				bases = append(bases, s)
			}
		}
	} else if s, ok := generate(parsed); ok {
		bases = append(bases, s)
	}
	for _, base := range bases {
		// A word boundary or an anchor can reject the bare text and accept it with
		// a neighbour, or the other way round.
		for _, candidate := range []string{base, " " + base + " ", "x" + base + "x"} {
			if loc := re.FindStringIndex(candidate); loc != nil && loc[1] > loc[0] {
				return candidate, true
			}
		}
	}
	return "", false
}

// TextNotMatching returns a text re does not match, for a canary that a
// required pattern must flag as absent. ok is false when re matches every
// candidate, as a pattern like `.*` does.
func TextNotMatching(re *regexp.Regexp) (string, bool) {
	for _, candidate := range []string{"canary", "", "0", " ", "\u2063"} {
		if !re.MatchString(candidate) {
			return candidate, true
		}
	}
	return "", false
}

func generate(re *syntax.Regexp) (string, bool) {
	switch re.Op {
	case syntax.OpNoMatch:
		return "", false
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText,
		syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return "", true
	case syntax.OpLiteral:
		return string(re.Rune), true
	case syntax.OpCharClass:
		return classRune(re.Rune)
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		return "a", true
	case syntax.OpCapture:
		return generate(re.Sub[0])
	case syntax.OpStar, syntax.OpPlus, syntax.OpQuest:
		// One occurrence satisfies all three and keeps the match non-empty.
		return generate(re.Sub[0])
	case syntax.OpRepeat:
		if re.Max == 0 {
			return "", true
		}
		sub, ok := generate(re.Sub[0])
		if !ok {
			return "", false
		}
		return strings.Repeat(sub, min(max(re.Min, 1), maxCanaryRepeat)), true
	case syntax.OpConcat:
		var b strings.Builder
		for _, sub := range re.Sub {
			s, ok := generate(sub)
			if !ok {
				return "", false
			}
			b.WriteString(s)
		}
		return b.String(), true
	case syntax.OpAlternate:
		for _, sub := range re.Sub {
			if s, ok := generate(sub); ok {
				return s, true
			}
		}
		return "", false
	}
	return "", false
}

// classRune picks a character from a class's ranges, preferring a letter or a
// digit, which no boundary assertion treats as a separator.
func classRune(ranges []rune) (string, bool) {
	if len(ranges) < 2 {
		return "", false
	}
	for _, prefer := range []rune{'a', 'A', '0'} {
		for i := 0; i+1 < len(ranges); i += 2 {
			if ranges[i] <= prefer && prefer <= ranges[i+1] {
				return string(prefer), true
			}
		}
	}
	for i := 0; i+1 < len(ranges); i += 2 {
		if lo := max(ranges[i], ' '); lo <= ranges[i+1] {
			return string(lo), true
		}
	}
	return string(ranges[0]), true
}
