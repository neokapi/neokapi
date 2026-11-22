package profile

import (
	"regexp/syntax"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/model"
)

// maxListedWords is the most words PatternWords lists before it gives up and
// leaves the rule to its regex.
const maxListedWords = 40

// patternOpenEnd marks where a pattern accepts any further word characters,
// such as the "\w*" in "util\w*".
const patternOpenEnd = "…"

// PatternWords is the words and phrases a pattern rule matches, when its regex
// is a finite list of them: `\b(powerful|blazing)\b` is
// ["powerful", "blazing"], and `game[- ]changing` is ["game changing",
// "game-changing"]. A run of word characters the regex leaves open at the end
// of a word, as in `util\w*`, reads as "util…". Word boundaries, anchors and
// case folding say nothing a reader needs and are dropped.
//
// A regex that matches more than a short list of strings, leaves a run open
// anywhere but the end of a word, or does not parse has no word list and
// yields nil. Words keep the order of the regex.
func PatternWords(regex string) []string {
	re, err := syntax.Parse(strings.TrimSpace(regex), syntax.Perl)
	if err != nil {
		return nil
	}
	words, ok := expandPattern(re, false)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, w := range words {
		w = strings.Join(strings.Fields(w), " ")
		if w == "" || seen[w] {
			continue
		}
		// An open run anywhere but the end of a word ("…ly") is a shape, not
		// a word, and reads better as the regex.
		if strings.Contains(strings.TrimSuffix(w, patternOpenEnd), patternOpenEnd) {
			return nil
		}
		seen[w] = true
		out = append(out, w)
	}
	return out
}

// expandPattern is every string re matches, or false when the set is not a
// short finite list.
func expandPattern(re *syntax.Regexp, inner bool) ([]string, bool) {
	switch re.Op {
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine,
		syntax.OpBeginText, syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return []string{""}, true
	case syntax.OpLiteral:
		s := string(re.Rune)
		if re.Flags&syntax.FoldCase != 0 {
			s = strings.ToLower(s)
		}
		return []string{s}, true
	case syntax.OpCharClass:
		chars, ok := classChars(re.Rune)
		if !ok {
			return nil, false
		}
		if re.Flags&syntax.FoldCase != 0 || foldedPairs(chars) {
			chars = lowerUnique(chars)
		}
		return chars, true
	case syntax.OpCapture:
		return expandPattern(re.Sub[0], inner)
	case syntax.OpQuest:
		sub, ok := expandPattern(re.Sub[0], true)
		if !ok {
			return nil, false
		}
		return append([]string{""}, sub...), true
	case syntax.OpStar, syntax.OpPlus:
		// Only an open run of word characters reads as words: "util\w*".
		if re.Sub[0].Op == syntax.OpCharClass && wordClass(re.Sub[0].Rune) {
			return []string{patternOpenEnd}, true
		}
		return nil, false
	case syntax.OpConcat:
		out := []string{""}
		for _, sub := range re.Sub {
			parts, ok := expandPattern(sub, true)
			if !ok {
				return nil, false
			}
			next := make([]string, 0, len(out)*len(parts))
			for _, a := range out {
				for _, b := range parts {
					next = append(next, a+b)
				}
			}
			if len(next) > maxListedWords {
				return nil, false
			}
			out = next
		}
		return out, true
	case syntax.OpAlternate:
		var out []string
		for _, sub := range re.Sub {
			parts, ok := expandPattern(sub, true)
			if !ok {
				return nil, false
			}
			out = append(out, parts...)
			if len(out) > maxListedWords {
				return nil, false
			}
		}
		return out, true
	}
	return nil, false
}

// classChars is each character of a character class's ranges, when the class
// is a handful of characters ([- ], [sz]).
func classChars(ranges []rune) ([]string, bool) {
	var out []string
	for i := 0; i+1 < len(ranges); i += 2 {
		for r := ranges[i]; r <= ranges[i+1]; r++ {
			out = append(out, string(r))
			if len(out) > 4 {
				return nil, false
			}
		}
	}
	return out, len(out) > 0
}

// foldedPairs says a class is only upper and lower case of the same letters,
// as [Pp] is.
func foldedPairs(chars []string) bool {
	return len(lowerUnique(chars)) < len(chars)
}

func lowerUnique(chars []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range chars {
		c = strings.ToLower(c)
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// wordClass says a class holds the letters a word is made of, as \w and
// [a-z] do.
func wordClass(ranges []rune) bool {
	for i := 0; i+1 < len(ranges); i += 2 {
		if ranges[i] <= 'a' && ranges[i+1] >= 'z' {
			return true
		}
	}
	return false
}

// PatternExcerpt is one place a pattern rule matches, with the text around
// it.
type PatternExcerpt struct {
	Before string `json:"before"`
	Match  string `json:"match"`
	After  string `json:"after"`
}

// patternExcerptContext is how many bytes of text an excerpt keeps on each
// side of a match, before it is trimmed back to a word.
const patternExcerptContext = 60

// PatternExcerpts is up to limit places the rule matches a block's runs,
// matched the way a check matches it (MatchPatternsInRuns) but with any rate
// ceiling lifted, so a reader sees what the rule counts. A required pattern
// shows text that keeps to it.
func PatternExcerpts(p Pattern, runs []model.Run, limit int) []PatternExcerpt {
	if limit <= 0 || strings.TrimSpace(p.Regex) == "" {
		return nil
	}
	p.Rate = nil
	probe := &VoiceProfile{}
	probe.Style.ProhibitedPatterns = []Pattern{p}
	hits := MatchPatternsInRuns(probe, runs)
	if len(hits) == 0 {
		return nil
	}
	text := model.RunsText(runs)
	var out []PatternExcerpt
	for _, h := range hits {
		if h.Start < 0 || h.End > len(text) || h.Start >= h.End {
			continue
		}
		out = append(out, PatternExcerpt{
			Before: excerptBefore(text[:h.Start]),
			Match:  text[h.Start:h.End],
			After:  excerptAfter(text[h.End:]),
		})
		if len(out) == limit {
			break
		}
	}
	return out
}

func excerptBefore(s string) string {
	cut := s
	if len(s) > patternExcerptContext {
		i := len(s) - patternExcerptContext
		for i < len(s) && !utf8.RuneStart(s[i]) {
			i++
		}
		cut = s[i:]
		// Start at a word, not inside one.
		if j := strings.IndexFunc(cut, unicode.IsSpace); j >= 0 && j < len(cut)-1 {
			cut = cut[j+1:]
		}
		cut = patternOpenEnd + cut
	}
	return collapseSpace(cut, true)
}

func excerptAfter(s string) string {
	cut := s
	if len(s) > patternExcerptContext {
		i := patternExcerptContext
		for i > 0 && !utf8.RuneStart(s[i]) {
			i--
		}
		cut = s[:i]
		if j := strings.LastIndexFunc(cut, unicode.IsSpace); j > 0 {
			cut = cut[:j]
		}
		cut += patternOpenEnd
	}
	return collapseSpace(cut, false)
}

// collapseSpace turns runs of whitespace into one space, keeping a space at
// the edge that meets the match.
func collapseSpace(s string, before bool) string {
	words := strings.Fields(s)
	out := strings.Join(words, " ")
	if out == "" {
		if s != "" {
			return " "
		}
		return ""
	}
	if before && unicode.IsSpace(lastRune(s)) {
		out += " "
	}
	if !before && unicode.IsSpace(firstRune(s)) {
		out = " " + out
	}
	return out
}

func lastRune(s string) rune  { r, _ := utf8.DecodeLastRuneInString(s); return r }
func firstRune(s string) rune { r, _ := utf8.DecodeRuneInString(s); return r }
