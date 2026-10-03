package change

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/model"
)

// Finds. A find is placeholder text, read as set_content's text is: its text
// matches the text of a run sequence with every inline code at zero width,
// and an <x id="…"/> token in it matches only the code with that id, at that
// point. A code a writer spells as characters a find may name by them too: a
// character reference by the characters it stands for, and an ICU argument
// by its source, {count}, which an ICU writer reads back as the argument.
//
// A find that names no code resolves to offsets of the sequence's own text,
// and its replacement is plain text, as replace_text has always taken it. A
// find that names a code matches a span of runs, and the replacement, parsed
// as placeholder text against the edition's codes, takes the place of the
// whole span: a code in the span the replacement leaves out is removed where
// it may be deleted, and otherwise the operation is refused
// guard/codes_changed, as for any other content (section 2.4, rule 2). Half of
// a pair the find passed over without naming it, left out while its other
// half stays, is refused naming that half (halvesPassedOver).

// findElem is one element of a run sequence as a find matches it: a code
// point of its text, or a run with no width of its own (an inline code, a
// plural or a select).
type findElem struct {
	r    rune
	code bool
	// key is the code's kind and id; the zero key for a plural or select.
	key codeKey
	// alias is the text a find may name the code by, if any.
	alias []rune
	// run is the element's run, and off a code point's offset into the
	// run's text.
	run, off int
	// own is the offset in the sequence's own text where the element sits.
	own int
}

// needleElem is one element of a find: a code point, or a code its token
// names.
type needleElem struct {
	r    rune
	code bool
	key  codeKey
}

// findSpan is a match of a find that names inline codes: the elements of the
// sequence it covers, [from, to), and the codes among them the find named, by
// element index.
type findSpan struct {
	from, to int
	named    []int
}

// parsedFind is a find read as placeholder text against a sequence.
type parsedFind struct {
	needle []needleElem
	// tokens are the codes the find names by token, in order.
	tokens []codeKey
}

// parseFind reads find as placeholder text: each <x id="…"/> token is a code,
// everything else is text.
func parseFind(find string, seq []model.Run) parsedFind {
	var p parsedFind
	for _, r := range model.ParseRunsPlaceholderText(find, seq) {
		if k, ok := keyOf(r); ok {
			p.needle = append(p.needle, needleElem{code: true, key: k})
			p.tokens = append(p.tokens, k)
			continue
		}
		if r.Text != nil {
			for _, c := range r.Text.Text {
				p.needle = append(p.needle, needleElem{r: c})
			}
		}
	}
	return p
}

// elements lists the sequence's elements in order.
func (ix *seqIndex) elements() []findElem {
	if ix.elems != nil {
		return ix.elems
	}
	elems := make([]findElem, 0, len(ix.text)+len(ix.seq))
	own := 0
	for i, r := range ix.seq {
		if r.Text != nil {
			off := 0
			for _, c := range r.Text.Text {
				elems = append(elems, findElem{r: c, run: i, off: off, own: own})
				off++
				own++
			}
			continue
		}
		e := findElem{code: true, run: i, own: own}
		if k, ok := keyOf(r); ok {
			e.key = k
			e.alias = codeAlias(r)
			ix.aliases = ix.aliases || e.alias != nil
		}
		elems = append(elems, e)
	}
	ix.elems = elems
	return elems
}

// codeAlias is the text a find may name a code by: the characters a
// character reference stands for, or the source of an ICU argument, which an
// ICU writer reads back as the argument. Nil for any other code.
func codeAlias(r model.Run) []rune {
	if r.Ph == nil {
		return nil
	}
	if chars, ok := model.CharacterReference(r.Ph); ok {
		return []rune(chars)
	}
	if d := r.Ph.Data; len(d) > 2 && strings.HasPrefix(d, "{") && strings.HasSuffix(d, "}") && !strings.ContainsAny(d, "\n") {
		return []rune(d)
	}
	return nil
}

// matcher matches one find against the elements of one sequence.
//
// A code the find may name by its text can be taken that way or passed over
// at no width, so the walk branches at every such code, and n of them in a
// row give 2^n paths to a find that fails. Whether the rest of the find
// matches from a state (hi, ni) past its first element does not depend on the
// path that reached it, so a state at a code that failed once is remembered
// and not walked again. A text element has one way forward and needs no
// memory: the walk between two codes is a straight line.
type matcher struct {
	h      []findElem
	needle []needleElem
	failed map[[2]int]struct{}
}

func newMatcher(h []findElem, needle []needleElem) *matcher {
	return &matcher{h: h, needle: needle, failed: map[[2]int]struct{}{}}
}

// match reports whether the needle matches the elements from hi on, and
// returns the element index after the match and the elements it took as
// codes: a code named by its token or by its alias. Between the elements the
// find names, a code it does not name has no width and is passed over.
func (m *matcher) match(hi int) (end int, named []int, ok bool) {
	h, needle := m.h, m.needle
	var try func(hi, ni int) bool
	try = func(hi, ni int) bool {
		if ni == len(needle) {
			end = hi
			return true
		}
		if hi == len(h) {
			return false
		}
		e, want := h[hi], needle[ni]
		state := [2]int{hi, ni}
		if e.code {
			if _, seen := m.failed[state]; seen {
				return false
			}
		}
		switch {
		case want.code:
			if e.code && e.key == want.key {
				named = append(named, hi)
				if try(hi+1, ni+1) {
					return true
				}
				named = named[:len(named)-1]
			}
		default:
			if !e.code && e.r == want.r && try(hi+1, ni+1) {
				return true
			}
			if e.code && len(e.alias) > 0 && needleHas(needle, ni, e.alias) {
				named = append(named, hi)
				if try(hi+1, ni+len(e.alias)) {
					return true
				}
				named = named[:len(named)-1]
			}
		}
		// A code the find does not name, inside the match, has no width.
		if e.code && ni > 0 && try(hi+1, ni) {
			return true
		}
		if e.code {
			m.failed[state] = struct{}{}
		}
		return false
	}
	if !try(hi, 0) {
		return 0, nil, false
	}
	return end, named, true
}

// needleHas reports whether the code points of needle from ni on begin with
// text.
func needleHas(needle []needleElem, ni int, text []rune) bool {
	if ni+len(text) > len(needle) {
		return false
	}
	for j, c := range text {
		if n := needle[ni+j]; n.code || n.r != c {
			return false
		}
	}
	return true
}

// findMatch is one match of a find in a sequence.
type findMatch struct {
	start, end int // own-text offsets
	span       *findSpan
}

// findAll lists the matches of a parsed find in the sequence, left to right,
// none overlapping the one before it.
func (ix *seqIndex) findAll(p parsedFind) []findMatch {
	if len(p.tokens) == 0 {
		ix.elements()
		if !ix.aliases {
			return ix.findText(p.needle)
		}
	}
	h := ix.elements()
	m := newMatcher(h, p.needle)
	var out []findMatch
	for hi := 0; hi < len(h); {
		end, named, ok := m.match(hi)
		if !ok {
			hi++
			continue
		}
		m := findMatch{start: h[hi].own, end: ownAfter(h[end-1])}
		if len(named) > 0 {
			m.span = &findSpan{from: hi, to: end, named: named}
		}
		out = append(out, m)
		hi = end
	}
	return out
}

// findText lists the matches of a find of text alone in the sequence's own
// text, in which every code has zero width.
func (ix *seqIndex) findText(needle []needleElem) []findMatch {
	runes := make([]rune, len(needle))
	for i, n := range needle {
		runes[i] = n.r
	}
	var out []findMatch
	for i := 0; i+len(runes) <= len(ix.text); {
		if runesAt(ix.text, i, runes) {
			out = append(out, findMatch{start: i, end: i + len(runes)})
			i += len(runes)
			continue
		}
		i++
	}
	return out
}

// ownAfter is the own-text offset just after an element.
func ownAfter(e findElem) int {
	if e.code {
		return e.own
	}
	return e.own + 1
}

// spanPositions are the run positions a span covers, under RangeAnchor's
// attribution: a boundary at the end of a text run is the start of the run
// after it.
func (ix *seqIndex) spanPositions(s *findSpan) (model.RunPos, model.RunPos) {
	h := ix.elements()
	first, last := h[s.from], h[s.to-1]
	start := model.RunPos{Run: first.run, Offset: first.off}
	if first.code {
		start.Offset = 0
	}
	var end model.RunPos
	switch {
	case last.code:
		end = model.RunPos{Run: last.run + 1}
	case last.off+1 < utf8.RuneCountInString(ix.seq[last.run].Text.Text):
		end = model.RunPos{Run: last.run, Offset: last.off + 1}
	default:
		end = model.RunPos{Run: last.run + 1}
	}
	return start, end
}

// unknownToken returns the first code a find names by token that the
// sequence, branches included, does not hold.
func unknownToken(p parsedFind, seq []model.Run) (codeKey, bool) {
	held := map[codeKey]model.Run{}
	indexCodes(seq, held)
	for _, k := range p.tokens {
		if _, ok := held[k]; !ok {
			return k, true
		}
	}
	return codeKey{}, false
}

// spliceSpan replaces the elements [from, to) of seq with repl, splitting the
// text runs the span begins and ends in.
func spliceSpan(ix *seqIndex, from, to int, repl []model.Run) []model.Run {
	h := ix.elements()
	seq := ix.seq
	first := h[from]
	var out []model.Run
	out = append(out, seq[:first.run]...)
	if !first.code && first.off > 0 {
		t := *seq[first.run].Text
		t.Text = string([]rune(t.Text)[:first.off])
		out = append(out, model.Run{Text: &t})
	}
	out = append(out, repl...)
	if to == len(h) {
		// Whatever follows the last element has no width: codes, plurals and
		// selects after the span's end stay where they are.
		last := h[to-1]
		if last.code {
			return append(out, seq[last.run+1:]...)
		}
		rest := []rune(seq[last.run].Text.Text)[last.off+1:]
		if len(rest) > 0 {
			t := *seq[last.run].Text
			t.Text = string(rest)
			out = append(out, model.Run{Text: &t})
		}
		return append(out, seq[last.run+1:]...)
	}
	next := h[to]
	if !next.code && next.off > 0 {
		t := *seq[next.run].Text
		t.Text = string([]rune(t.Text)[next.off:])
		out = append(out, model.Run{Text: &t})
		return append(out, seq[next.run+1:]...)
	}
	// The span ends where a run begins; any run with no element between
	// them (an empty text run) follows the replacement.
	last := h[to-1]
	return append(out, seq[last.run+1:]...)
}

// spanReplacement is the runs a replacement written as placeholder text
// stands for, in place of a span a find named codes in: each token is the
// code of the edition with that id, and the text a code the match named by
// its alias stood for, written again in the replacement, is that code again.
// The replacement keeps the do-not-translate mark when every code point it
// replaces had it.
func spanReplacement(ix *seqIndex, s *findSpan, text string, edition []model.Run) []model.Run {
	h := ix.elements()
	parsed := model.ParseRunsPlaceholderText(text, edition)
	var aliased []model.Run
	var aliases [][]rune
	for _, n := range s.named {
		if a := h[n].alias; len(a) > 0 {
			aliased = append(aliased, ix.seq[h[n].run])
			aliases = append(aliases, a)
		}
	}
	noTranslate := true
	sawText := false
	for _, e := range h[s.from:s.to] {
		if e.code {
			continue
		}
		sawText = true
		noTranslate = noTranslate && ix.seq[e.run].Text.NoTranslate
	}
	out := make([]model.Run, 0, len(parsed))
	for _, r := range parsed {
		if r.Text == nil {
			out = append(out, r)
			continue
		}
		rest := []rune(r.Text.Text)
		for len(rest) > 0 {
			at, which := -1, -1
			for i, a := range aliases {
				if a == nil {
					continue
				}
				if j := indexRunes(rest, a); j >= 0 && (at < 0 || j < at) {
					at, which = j, i
				}
			}
			if at < 0 {
				out = append(out, textRun(string(rest), sawText && noTranslate))
				break
			}
			if at > 0 {
				out = append(out, textRun(string(rest[:at]), sawText && noTranslate))
			}
			out = append(out, aliased[which])
			rest = rest[at+len(aliases[which]):]
			aliases[which] = nil
		}
	}
	return out
}

// halvesPassedOver refuses the replacement of a span a find named codes in
// when the find passed over one half of a paired code without naming it and
// the replacement leaves that half out while the other half stays: the pair
// would be left unbalanced by a code the sender never named. The refusal
// names each such half, so the sender can name it in the find and place it in
// the replacement. A pair whose two halves the span holds and the replacement
// leaves out is removed whole, as for any other content.
func halvesPassedOver(ix *seqIndex, s *findSpan, repl []model.Run) *Error {
	h := ix.elements()
	kept, inSpan := map[codeKey]bool{}, map[codeKey]bool{}
	for _, r := range repl {
		if k, ok := keyOf(r); ok {
			kept[k] = true
		}
	}
	for _, e := range h[s.from:s.to] {
		if e.code {
			inSpan[e.key] = true
		}
	}
	var halves []string
	for i := s.from; i < s.to; i++ {
		e := h[i]
		if !e.code || kept[e.key] || slices.Contains(s.named, i) {
			continue
		}
		partner := codeKey{id: e.key.id}
		switch e.key.kind {
		case model.RunKindPcOpen:
			partner.kind = model.RunKindPcClose
		case model.RunKindPcClose:
			partner.kind = model.RunKindPcOpen
		default:
			continue
		}
		if inSpan[partner] && !kept[partner] {
			continue
		}
		halves = append(halves, e.key.String())
	}
	if len(halves) == 0 {
		return nil
	}
	it := "it"
	if len(halves) > 1 {
		it = "them"
	}
	return &Error{Code: CodeGuard, Subcode: SubcodeCodesChanged, Expected: strings.Join(halves, " "),
		Message: fmt.Sprintf("the find passes over %s without naming %s, and the replacement leaves %s out, which leaves a paired code unbalanced: "+
			"name each code by its token in the find, and in the replacement where it belongs, or send a find of text alone",
			strings.Join(halves, ", "), it, it)}
}

func textRun(s string, noTranslate bool) model.Run {
	return model.Run{Text: &model.TextRun{Text: s, NoTranslate: noTranslate}}
}

// indexRunes is the index of the first occurrence of sub in s, or -1.
func indexRunes(s, sub []rune) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if runesAt(s, i, sub) {
			return i
		}
	}
	return -1
}

// notInSequence is the message of a find that matches nothing in the sequence
// path reaches: the find, the path searched and the text there.
func notInSequence(find string, path model.RunPath, seq []model.Run) string {
	text := model.RunsEditText(seq)
	if len(path) == 0 {
		return fmt.Sprintf("%q is not in the text, which is %q", find, text)
	}
	return fmt.Sprintf("%q is not in %s, whose text is %q", find, pathText(path), text)
}
