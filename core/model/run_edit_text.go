package model

import (
	"html"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// The edit text is the form a block takes in the edit loop: what `kapi
// inspect` and MCP read_blocks show, and what `kapi apply` and MCP
// apply_edits take back. It is RunsPlaceholderText with one difference: a
// placeholder that stands for characters, a character reference such as
// `&amp;` or `&rsquo;`, is shown as those characters. The HTML and Markdown
// readers keep a reference as a placeholder so an untouched document keeps
// its spelling, and to a reader of the text it is the character.

// charRefDataRE matches placeholder data that is exactly one character
// reference: named, decimal or hexadecimal.
var charRefDataRE = regexp.MustCompile(`^&(?:[A-Za-z][A-Za-z0-9]*|#[0-9]+|#[xX][0-9A-Fa-f]+);$`)

// CharacterReference reports whether a placeholder stands for characters,
// because its data is one character reference, and returns the characters.
// A name no character table knows is not a reference.
func CharacterReference(p *PlaceholderRun) (string, bool) {
	if p == nil || !charRefDataRE.MatchString(p.Data) {
		return "", false
	}
	s := html.UnescapeString(p.Data)
	if s == p.Data {
		return "", false
	}
	return s, true
}

// RunsEditText renders runs as the edit text: RunsPlaceholderText, with each
// character reference written as the characters it stands for.
func RunsEditText(runs []Run) string {
	var buf strings.Builder
	appendEditText(&buf, runs, nil)
	return buf.String()
}

// charRefSpan is where a top-level character reference sits in an edit text,
// in runes.
type charRefSpan struct {
	start, end int
	run        Run
}

// appendEditText writes the edit text of runs to buf, taking a plural or a
// select's branch as RunsPlaceholderText does. When spans is non-nil it
// records the rune span of every character reference outside a plural or
// select, which is where an edit can keep one.
func appendEditText(buf *strings.Builder, runs []Run, spans *[]charRefSpan) {
	runeCount := utf8.RuneCountInString(buf.String())
	for _, r := range runs {
		before := buf.Len()
		switch {
		case r.Ph != nil:
			chars, ok := CharacterReference(r.Ph)
			if !ok {
				appendRunsPlaceholder(buf, []Run{r})
				break
			}
			if spans != nil {
				n := utf8.RuneCountInString(chars)
				*spans = append(*spans, charRefSpan{start: runeCount, end: runeCount + n, run: r})
			}
			buf.WriteString(chars)
		case r.Plural != nil:
			appendEditText(buf, pluralBranch(r.Plural), nil)
		case r.Select != nil:
			appendEditText(buf, selectBranch(r.Select), nil)
		default:
			appendRunsPlaceholder(buf, []Run{r})
		}
		runeCount += utf8.RuneCountInString(buf.String()[before:])
	}
}

// pluralBranch is the form RunsPlaceholderText writes for a plural: other, or
// the first form present.
func pluralBranch(p *PluralRun) []Run {
	if form, ok := p.Forms[PluralOther]; ok {
		return form
	}
	for _, form := range p.Forms {
		return form
	}
	return nil
}

// selectBranch is the case RunsPlaceholderText writes for a select: other, or
// the first case present.
func selectBranch(s *SelectRun) []Run {
	if c, ok := s.Cases["other"]; ok {
		return c
	}
	for _, c := range s.Cases {
		return c
	}
	return nil
}

// ParseRunsEditText is the inverse of RunsEditText: it rebuilds a block's runs
// from an edit written as edit text, against the runs the block held when the
// edit text was read. Each <x id="…"/> token is the inline code it names, as
// ParseRunsPlaceholderText reads it. A character a character reference stood
// for, which the edit kept where the reference was, becomes that reference
// again, so the document keeps its spelling; a character the edit added is
// text, which the writer encodes for its format.
func ParseRunsEditText(text string, sourceRuns []Run) []Run {
	var oldText strings.Builder
	var spans []charRefSpan
	appendEditText(&oldText, sourceRuns, &spans)
	if len(spans) == 0 {
		return ParseRunsPlaceholderText(text, sourceRuns)
	}

	newRunes := []rune(text)
	// byteAt[i] is the byte offset of rune i of text; byteAt[len] is len(text).
	byteAt := make([]int, len(newRunes)+1)
	off := 0
	for i, r := range newRunes {
		byteAt[i] = off
		off += utf8.RuneLen(r)
	}
	byteAt[len(newRunes)] = off

	tokens := runPlaceholderTagRe.FindAllStringSubmatchIndex(text, -1)
	inToken := func(start, end int) bool {
		for _, t := range tokens {
			if start < t[1] && t[0] < end {
				return true
			}
		}
		return false
	}

	match := alignRunes([]rune(oldText.String()), newRunes)
	// restore maps the byte offset where a kept reference's characters begin
	// in text to the byte offset where they end and the reference's run.
	type restoreAt struct {
		end int
		run Run
	}
	restore := map[int]restoreAt{}
	for _, s := range spans {
		first := match[s.start]
		if first < 0 {
			continue
		}
		kept := true
		for i := s.start; i < s.end; i++ {
			if match[i] != first+(i-s.start) {
				kept = false
				break
			}
		}
		if !kept {
			continue
		}
		start, end := byteAt[first], byteAt[first+(s.end-s.start)]
		if inToken(start, end) {
			continue
		}
		restore[start] = restoreAt{end: end, run: s.run}
	}

	lookup := make(map[runLookupKey]Run, len(sourceRuns))
	indexRunsForLookup(lookup, sourceRuns)
	var out []Run
	appendText := func(s string) {
		if s == "" {
			return
		}
		if n := len(out); n > 0 && out[n-1].Text != nil {
			out[n-1].Text.Text += s
			return
		}
		out = append(out, Run{Text: &TextRun{Text: s}})
	}
	appendSegment := func(from, to int) {
		seg := from
		for i := from; i < to; {
			if r, ok := restore[i]; ok && r.end <= to {
				appendText(text[seg:i])
				out = append(out, r.run)
				i, seg = r.end, r.end
				continue
			}
			_, size := utf8.DecodeRuneInString(text[i:])
			i += size
		}
		appendText(text[seg:to])
	}
	last := 0
	for _, loc := range tokens {
		appendSegment(last, loc[0])
		out = append(out, runFromPlaceholderID(text[loc[2]:loc[3]], lookup))
		last = loc[1]
	}
	appendSegment(last, len(text))
	return carryNoTranslate(out, sourceRuns)
}

// maxAlignCells bounds the table alignRunes builds for the part of two texts
// that differs. Past it the differing parts are not aligned, which only costs
// a reference inside them its spelling.
const maxAlignCells = 1 << 20

// alignRunes matches the runes of a to those of b along a longest common
// subsequence and returns, for each rune of a, the index of the rune of b it
// matches, or -1.
func alignRunes(a, b []rune) []int {
	match := make([]int, len(a))
	for i := range match {
		match[i] = -1
	}
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		match[prefix] = prefix
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		match[len(a)-1-suffix] = len(b) - 1 - suffix
		suffix++
	}
	am, bm := a[prefix:len(a)-suffix], b[prefix:len(b)-suffix]
	if len(am) == 0 || len(bm) == 0 || len(am)*len(bm) > maxAlignCells {
		return match
	}
	// lcs[i][j] is the length of the longest common subsequence of am[i:] and
	// bm[j:], stored row-major in one slice.
	w := len(bm) + 1
	lcs := make([]int32, (len(am)+1)*w)
	for i, ai := range slices.Backward(am) {
		for j, bj := range slices.Backward(bm) {
			switch {
			case ai == bj:
				lcs[i*w+j] = lcs[(i+1)*w+j+1] + 1
			case lcs[(i+1)*w+j] >= lcs[i*w+j+1]:
				lcs[i*w+j] = lcs[(i+1)*w+j]
			default:
				lcs[i*w+j] = lcs[i*w+j+1]
			}
		}
	}
	for i, j := 0, 0; i < len(am) && j < len(bm); {
		switch {
		case am[i] == bm[j]:
			match[prefix+i] = prefix + j
			i++
			j++
		case lcs[(i+1)*w+j] >= lcs[i*w+j+1]:
			i++
		default:
			j++
		}
	}
	return match
}

// EditKeepsInlineCodes is the fidelity guard for an edit written as edit
// text: InlineCodesPreserved, with character references left out on both
// sides. The edit text shows a reference as its character, so an edit may
// drop, move or repeat one; every other code must survive as
// InlineCodesPreserved requires.
func EditKeepsInlineCodes(before, after []Run) bool {
	return InlineCodesPreserved(withoutCharacterReferences(before), withoutCharacterReferences(after))
}

func withoutCharacterReferences(runs []Run) []Run {
	out := make([]Run, 0, len(runs))
	for _, r := range runs {
		if r.Ph != nil {
			if _, ok := CharacterReference(r.Ph); ok {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}
