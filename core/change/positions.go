package change

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/model"
)

// Positions. A text edit names its text in one of three forms, each resolved
// against one run sequence, the one its path reaches:
//
//   - start and end, code-point offsets into the sequence's own text
//     (model.SequenceText), in which every inline code, and every plural or
//     select, has zero width;
//   - find, placeholder text matched in that text (find.go), with occurrence
//     choosing one of several matches;
//   - range, two run positions.
//
// All three resolve to offsets in the sequence's own text, and the result
// echoes them as run positions under RangeAnchor's attribution rule: a
// boundary at the end of a text run is the start of the run after it.

// resolveSelection resolves a selection in seq to code-point offsets of its
// own text.
func resolveSelection(seq []model.Run, sel Selection, path model.RunPath, field string) (start, end int, err *Error) {
	return indexSequence(seq).resolve(sel, path, field)
}

// resolve resolves a selection to code-point offsets of the sequence's own
// text.
func (ix *seqIndex) resolve(sel Selection, path model.RunPath, field string) (start, end int, err *Error) {
	start, end, _, err = ix.resolveSpan(sel, path, field)
	return start, end, err
}

// seqIndex answers position questions about one run sequence in logarithmic
// time, so an operation with many edits in one sequence reads the sequence
// once rather than once per edit.
type seqIndex struct {
	seq  []model.Run
	text []rune // the sequence's own text (model.SequenceText)
	// own[i] is the own-text offset at the start of run i, and flat[i] the
	// offset in the flattened text (model.RunsText), in which a plural or
	// select has the width of its other branch. Both end with the total.
	own, flat []int
	// elems are the sequence's elements as a find matches them, listed on
	// first use; aliases says a code among them may be named by text.
	elems   []findElem
	aliases bool
}

func indexSequence(seq []model.Run) *seqIndex {
	ix := &seqIndex{seq: seq, own: make([]int, len(seq)+1), flat: make([]int, len(seq)+1)}
	for i, r := range seq {
		w, fw := 0, 0
		switch {
		case r.Text != nil:
			n := len(ix.text)
			ix.text = append(ix.text, []rune(r.Text.Text)...)
			w = len(ix.text) - n
			fw = w
		case r.Plural != nil || r.Select != nil:
			fw = utf8.RuneCountInString(model.RunsText(seq[i : i+1]))
		}
		ix.own[i+1] = ix.own[i] + w
		ix.flat[i+1] = ix.flat[i] + fw
	}
	return ix
}

// posAt is the run position of a code-point offset into the sequence's own
// text, with RangeAnchor's attribution: a boundary at the end of a text run is
// the start of the run after it.
func (ix *seqIndex) posAt(offset int) model.RunPos {
	if offset <= 0 {
		return model.RunPos{}
	}
	// The first run whose end reaches offset is a text run holding it, since a
	// run with no width cannot be the first to reach an offset past zero.
	i := sort.Search(len(ix.seq), func(i int) bool { return ix.own[i+1] >= offset })
	if i == len(ix.seq) {
		return model.RunPos{Run: len(ix.seq)}
	}
	if offset < ix.own[i+1] {
		return model.RunPos{Run: i, Offset: offset - ix.own[i]}
	}
	return model.RunPos{Run: i + 1}
}

// ownOffset is the code-point offset into the sequence's own text of a run
// position.
func (ix *seqIndex) ownOffset(p model.RunPos) int {
	switch {
	case p.Run < 0:
		return 0
	case p.Run >= len(ix.seq):
		return ix.own[len(ix.seq)]
	case ix.seq[p.Run].Text != nil:
		return ix.own[p.Run] + p.Offset
	}
	return ix.own[p.Run]
}

// flatAt is the offset in the flattened text of an offset into the own text.
// Several plurals and selects can sit at one own-text offset, each with the
// width of its branch in the flattened text; after says whether the offset
// lies after them or before them.
func (ix *seqIndex) flatAt(own int, after bool) int {
	n := len(ix.seq)
	i := sort.SearchInts(ix.own, own) // the first run boundary at or past own
	if i > n || ix.own[i] > own {
		// own lies inside text run i-1.
		return ix.flat[i-1] + own - ix.own[i-1]
	}
	if after {
		i = sort.SearchInts(ix.own, own+1) - 1 // the last boundary at own
	}
	return ix.flat[i]
}

// resolveSpan resolves a selection to code-point offsets of the sequence's
// own text and, for a find that names inline codes, the span of runs it
// matched.
func (ix *seqIndex) resolveSpan(sel Selection, path model.RunPath, field string) (start, end int, span *findSpan, err *Error) {
	seq, text := ix.seq, ix.text
	switch {
	case sel.Find != nil:
		start, end, span, err = ix.resolveFind(*sel.Find, sel.Occurrence, path, field)
	case sel.Start != nil:
		start, end = *sel.Start, *sel.End
		if start < 0 || end < start || end > len(text) {
			return 0, 0, nil, &Error{Code: CodeGuard, Subcode: SubcodeBadPosition, Field: field,
				Message: fmt.Sprintf("[%d, %d) is outside the text, which has %d code points", start, end, len(text))}
		}
	case sel.Range != nil:
		a := model.SpanAnchor(sel.Range.Start, sel.Range.End)
		if !a.InBounds(seq) {
			return 0, 0, nil, &Error{Code: CodeGuard, Subcode: SubcodeBadPosition, Field: field + "/range",
				Message: fmt.Sprintf("range %d:%d to %d:%d is outside the %d runs it addresses", sel.Range.Start.Run, sel.Range.Start.Offset, sel.Range.End.Run, sel.Range.End.Offset, len(seq))}
		}
		for i := sel.Range.Start.Run; i < sel.Range.End.Run && i < len(seq); i++ {
			if seq[i].Plural != nil || seq[i].Select != nil {
				return 0, 0, nil, &Error{Code: CodeGuard, Subcode: SubcodeStructureLost, Field: field + "/range",
					Message: fmt.Sprintf("the range covers the %s at run %d; edit one of its branches with path", seq[i].Kind(), i)}
			}
		}
		start, end = ix.ownOffset(sel.Range.Start), ix.ownOffset(sel.Range.End)
	default:
		return 0, 0, nil, &Error{Code: CodeInvalid, Field: field, Message: "names its text by exactly one of find, start and end, or range"}
	}
	if err != nil {
		return 0, 0, nil, err
	}
	if j, ok := ix.structureInside(start, end); ok {
		return 0, 0, nil, &Error{Code: CodeGuard, Subcode: SubcodeStructureLost, Field: field,
			Message: fmt.Sprintf("the text spans the %s at run %d; edit one of its branches with path", seq[j].Kind(), j)}
	}
	if span != nil {
		for _, e := range ix.elements()[span.from:span.to] {
			if e.code && e.key == (codeKey{}) {
				return 0, 0, nil, &Error{Code: CodeGuard, Subcode: SubcodeStructureLost, Field: field,
					Message: fmt.Sprintf("the text spans the %s at run %d; edit one of its branches with path", seq[e.run].Kind(), e.run)}
			}
		}
	}
	return start, end, span, nil
}

// resolveFind finds the occurrence-th match of find in the sequence's text.
func (ix *seqIndex) resolveFind(find string, occurrence int, path model.RunPath, field string) (int, int, *findSpan, *Error) {
	p := parseFind(find, ix.seq)
	matches := ix.findAll(p)
	switch {
	case len(matches) == 0:
		return 0, 0, nil, notInText(ix.seq, find, p, path, field)
	case occurrence == 0 && len(matches) > 1:
		e := &Error{Code: CodeAmbiguous, Field: field + "/find",
			Message: fmt.Sprintf("%q matches %d times; send occurrence to choose one", find, len(matches))}
		for n, m := range matches {
			r := ix.resolvedOf(m, path)
			e.Candidates = append(e.Candidates, Candidate{Occurrence: n + 1, At: &r, Text: around(ix.text, m.start, m.end)})
		}
		return 0, 0, nil, e
	case occurrence > len(matches):
		return 0, 0, nil, &Error{Code: CodeNotFound, Field: field + "/occurrence",
			Message: fmt.Sprintf("%q matches %d times; there is no occurrence %d", find, len(matches), occurrence)}
	}
	m := matches[0]
	if occurrence > 0 {
		m = matches[occurrence-1]
	}
	return m.start, m.end, m.span, nil
}

// resolvedOf is a match as a result echoes it.
func (ix *seqIndex) resolvedOf(m findMatch, path model.RunPath) Resolved {
	if m.span != nil {
		start, end := ix.spanPositions(m.span)
		return resolvedSpan(path, start, end)
	}
	return resolvedSpan(path, ix.posAt(m.start), ix.posAt(m.end))
}

// notInText refuses a find that seq's own text does not hold. A read shows a
// plural or select by one of its branches, and the text of each branch under
// structures, so a find taken from either can lie in a branch: the refusal
// then names each branch that holds it by the path that reaches it, with a
// candidate per match, and says to send the edit with one of those paths. A
// find that holds in no branch either is refused naming the path searched and
// the text there, with up to three matches that differ from it only in case
// as candidates; one that names by token a code the text does not hold is
// refused naming the token.
func notInText(seq []model.Run, find string, p parsedFind, path model.RunPath, field string) *Error {
	e := &Error{Code: CodeNotFound, Field: field + "/find", Message: notInSequence(find, path, seq),
		Searched: &Searched{Path: path, Text: model.RunsEditText(seq)}}
	if k, ok := unknownToken(p, seq); ok {
		e.Message = fmt.Sprintf("%q names %s, a code %s does not hold", find, k, searchedName(path))
		return e
	}
	var branches []string
	var walk func(seq []model.Run, path model.RunPath)
	visit := func(branch []model.Run, at model.RunPath) {
		ix := indexSequence(branch)
		matches := ix.findAll(parseFind(find, branch))
		for n, m := range matches {
			r := ix.resolvedOf(m, at)
			c := Candidate{At: &r, Text: around(ix.text, m.start, m.end)}
			if len(matches) > 1 {
				c.Occurrence = n + 1
			}
			e.Candidates = append(e.Candidates, c)
		}
		if len(matches) > 0 {
			branches = append(branches, pathText(at))
		}
		walk(branch, at)
	}
	walk = func(seq []model.Run, path model.RunPath) {
		for i, r := range seq {
			step := model.RunPathStep{Kind: model.StepIndex, Index: i}
			switch {
			case r.Plural != nil:
				for _, name := range sortedKeys(pluralNames(r.Plural.Forms)) {
					form := model.PluralForm(name)
					visit(r.Plural.Forms[form], append(slices.Clone(path), step, model.RunPathStep{Kind: model.StepPlural, PluralForm: form}))
				}
			case r.Select != nil:
				for _, value := range sortedKeys(r.Select.Cases) {
					visit(r.Select.Cases[value], append(slices.Clone(path), step, model.RunPathStep{Kind: model.StepSelect, SelectValue: value}))
				}
			}
		}
	}
	walk(seq, path)
	switch len(branches) {
	case 0:
		e.Candidates = caseCandidates(seq, find, p, path)
	case 1:
		e.Message = fmt.Sprintf("%q is not in the text around the plural or select; it is in the branch at path %s: send the edit with that path",
			find, branches[0])
	default:
		e.Message = fmt.Sprintf("%q is not in the text around the plural or select; it is in the branches at paths %s: send the edit with the path of the one to change",
			find, strings.Join(branches, ", "))
	}
	return e
}

// searchedName names the sequence a path reaches, in a message.
func searchedName(path model.RunPath) string {
	if len(path) == 0 {
		return "the text"
	}
	return pathText(path)
}

// maxCandidates bounds the candidates a not_found refusal of a find carries.
const maxCandidates = 3

// caseCandidates lists up to three matches of a find of text alone that
// differ from it only in case.
func caseCandidates(seq []model.Run, find string, p parsedFind, path model.RunPath) []Candidate {
	if len(p.tokens) > 0 {
		return nil
	}
	ix := indexSequence(seq)
	needle := []rune(strings.ToLower(find))
	lower := []rune(strings.ToLower(string(ix.text)))
	if len(lower) != len(ix.text) || len(needle) != utf8.RuneCountInString(find) {
		// A case mapping that changes the length leaves no offsets to report.
		return nil
	}
	var out []Candidate
	for i := 0; i+len(needle) <= len(lower) && len(out) < maxCandidates; {
		if !runesAt(lower, i, needle) {
			i++
			continue
		}
		r := resolvedSpan(path, ix.posAt(i), ix.posAt(i+len(needle)))
		out = append(out, Candidate{At: &r, Text: around(ix.text, i, i+len(needle))})
		i += len(needle)
	}
	return out
}

func runesAt(text []rune, i int, needle []rune) bool {
	for j, r := range needle {
		if text[i+j] != r {
			return false
		}
	}
	return true
}

// around is the text of a match with up to 20 code points either side.
func around(text []rune, start, end int) string {
	return string(text[max(0, start-20):min(len(text), end+20)])
}

// posAt is the run position of a code-point offset into seq's own text, with
// RangeAnchor's attribution: a boundary at the end of a text run is the start
// of the run after it.
func posAt(seq []model.Run, offset int) model.RunPos {
	if offset <= 0 {
		return model.RunPos{}
	}
	pos := 0
	for i, r := range seq {
		if r.Text == nil {
			continue
		}
		n := utf8.RuneCountInString(r.Text.Text)
		if n == 0 {
			continue
		}
		if offset < pos+n {
			return model.RunPos{Run: i, Offset: offset - pos}
		}
		if offset == pos+n {
			return model.RunPos{Run: i + 1}
		}
		pos += n
	}
	return model.RunPos{Run: len(seq)}
}

// structureInside reports a plural or select of the sequence that lies
// strictly inside [start, end) of its own text, which an edit there would
// delete.
func (ix *seqIndex) structureInside(start, end int) (int, bool) {
	if end-start < 2 {
		return 0, false
	}
	// Runs whose start lies strictly inside the range.
	from := sort.SearchInts(ix.own, start+1)
	for i := from; i < len(ix.seq) && ix.own[i] < end; i++ {
		if r := ix.seq[i]; r.Plural != nil || r.Select != nil {
			return i, true
		}
	}
	return 0, false
}

// endsInsideStructure reports whether a range anchor that does not resolve
// fails only because an end falls inside a plural or select of the sequence
// it addresses: the run there is the structure, at an offset within the width
// model.RunsText gives it. RangeAnchor makes such an anchor for an offset in
// the text of a structure's other branch, which detectors read.
func endsInsideStructure(a model.Anchor, runs []model.Run) bool {
	if a.Kind == model.AnchorBlock || a.Kind == model.AnchorRun || a.Kind == model.AnchorForm {
		return false
	}
	seq, ok := model.ResolveRunPath(runs, a.Path)
	if !ok || a.End.Run < a.Start.Run {
		return false
	}
	inside := false
	for _, p := range []model.RunPos{a.Start, a.End} {
		if p.Run < 0 || p.Run > len(seq) {
			return false
		}
		if p.Run < len(seq) && (seq[p.Run].Plural != nil || seq[p.Run].Select != nil) && p.Offset > 0 {
			if p.Offset > utf8.RuneCountInString(model.RunsText(seq[p.Run:p.Run+1])) {
				return false
			}
			inside = true
			continue
		}
		if !model.SpanAnchor(p, p).InBounds(seq) {
			return false
		}
	}
	return inside
}

// diffEdits describes the change from old to new as the one region their
// flattened texts differ in, in the RunEdit coordinates overlays anchor by.
func diffEdits(old, new []model.Run) []model.RunEdit {
	a, b := []rune(model.RunsText(old)), []rune(model.RunsText(new))
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	if prefix == len(a) && prefix == len(b) {
		return []model.RunEdit{}
	}
	return []model.RunEdit{{Start: prefix, End: len(a) - suffix, NewLen: len(b) - prefix - suffix}}
}

// newBranchReference returns the branch a new plural form or select case is
// read against: the run path ends at a plural or select whose named branch is
// missing, and the reference is its other branch, or its first.
func newBranchReference(runs []model.Run, path model.RunPath) ([]model.Run, bool) {
	if len(path) < 2 {
		return nil, false
	}
	parent, ok := model.ResolveRunPath(runs, path[:len(path)-2])
	if !ok {
		return nil, false
	}
	idx := path[len(path)-2]
	last := path[len(path)-1]
	if idx.Kind != model.StepIndex || idx.Index < 0 || idx.Index >= len(parent) {
		return nil, false
	}
	r := parent[idx.Index]
	switch {
	case last.Kind == model.StepPlural && r.Plural != nil:
		if _, exists := r.Plural.Forms[last.PluralForm]; exists {
			return nil, false
		}
		return branchOf(r), true
	case last.Kind == model.StepSelect && r.Select != nil:
		if _, exists := r.Select.Cases[last.SelectValue]; exists {
			return nil, false
		}
		return branchOf(r), true
	}
	return nil, false
}

// branchOf is a plural's other form or a select's other case, else the first
// by name.
func branchOf(r model.Run) []model.Run {
	switch {
	case r.Plural != nil:
		if f, ok := r.Plural.Forms[model.PluralOther]; ok {
			return f
		}
		for _, k := range sortedKeys(pluralNames(r.Plural.Forms)) {
			return r.Plural.Forms[model.PluralForm(k)]
		}
	case r.Select != nil:
		if c, ok := r.Select.Cases["other"]; ok {
			return c
		}
		for _, k := range sortedKeys(r.Select.Cases) {
			return r.Select.Cases[k]
		}
	}
	return nil
}

func pluralNames(forms map[model.PluralForm][]model.Run) map[string]bool {
	out := make(map[string]bool, len(forms))
	for k := range forms {
		out[string(k)] = true
	}
	return out
}

// replaceAtPath returns runs with the sequence path reaches replaced by seq.
// It copies every run on the way, so runs itself is not changed. A path whose
// last step names a branch the plural or select lacks adds the branch.
func replaceAtPath(runs []model.Run, path model.RunPath, seq []model.Run) ([]model.Run, bool) {
	if len(path) == 0 {
		return seq, true
	}
	if len(path) < 2 || path[0].Kind != model.StepIndex || path[0].Index < 0 || path[0].Index >= len(runs) {
		return nil, false
	}
	out := append([]model.Run(nil), runs...)
	r := out[path[0].Index]
	step := path[1]
	switch {
	case step.Kind == model.StepPlural && r.Plural != nil:
		inner, ok := r.Plural.Forms[step.PluralForm]
		if !ok && len(path) > 2 {
			return nil, false
		}
		next, ok := replaceAtPath(inner, path[2:], seq)
		if !ok {
			return nil, false
		}
		p := *r.Plural
		p.Forms = make(map[model.PluralForm][]model.Run, len(r.Plural.Forms)+1)
		maps.Copy(p.Forms, r.Plural.Forms)
		p.Forms[step.PluralForm] = next
		out[path[0].Index] = model.Run{Plural: &p}
	case step.Kind == model.StepSelect && r.Select != nil:
		inner, ok := r.Select.Cases[step.SelectValue]
		if !ok && len(path) > 2 {
			return nil, false
		}
		next, ok := replaceAtPath(inner, path[2:], seq)
		if !ok {
			return nil, false
		}
		s := *r.Select
		s.Cases = make(map[string][]model.Run, len(r.Select.Cases)+1)
		maps.Copy(s.Cases, r.Select.Cases)
		s.Cases[step.SelectValue] = next
		out[path[0].Index] = model.Run{Select: &s}
	default:
		return nil, false
	}
	return out, true
}
