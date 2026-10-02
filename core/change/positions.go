package change

import (
	"fmt"
	"maps"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/model"
)

// Positions. A text edit names its text in one of three forms, each resolved
// against one run sequence, the one its path reaches:
//
//   - start and end, code-point offsets into the sequence's own text
//     (model.SequenceText), in which every inline code, and every plural or
//     select, has zero width;
//   - find, literal text matched in that text, with occurrence choosing one
//     of several matches;
//   - range, two run positions.
//
// All three resolve to offsets in the sequence's own text, and the result
// echoes them as run positions under RangeAnchor's attribution rule: a
// boundary at the end of a text run is the start of the run after it.

// resolveSelection resolves a selection in seq to code-point offsets of its
// own text.
func resolveSelection(seq []model.Run, sel Selection, path model.RunPath, field string) (start, end int, err *Error) {
	text := []rune(model.SequenceText(seq))
	switch {
	case sel.Find != nil:
		start, end, err = resolveFind(seq, text, *sel.Find, sel.Occurrence, path, field)
	case sel.Start != nil:
		start, end = *sel.Start, *sel.End
		if start < 0 || end < start || end > len(text) {
			return 0, 0, &Error{Code: CodeGuard, Subcode: SubcodeBadPosition, Field: field,
				Message: fmt.Sprintf("[%d, %d) is outside the text, which has %d code points", start, end, len(text))}
		}
	case sel.Range != nil:
		a := model.SpanAnchor(sel.Range.Start, sel.Range.End)
		if !a.InBounds(seq) {
			return 0, 0, &Error{Code: CodeGuard, Subcode: SubcodeBadPosition, Field: field + "/range",
				Message: fmt.Sprintf("range %d:%d to %d:%d is outside the %d runs it addresses", sel.Range.Start.Run, sel.Range.Start.Offset, sel.Range.End.Run, sel.Range.End.Offset, len(seq))}
		}
		for i := sel.Range.Start.Run; i < sel.Range.End.Run && i < len(seq); i++ {
			if seq[i].Plural != nil || seq[i].Select != nil {
				return 0, 0, &Error{Code: CodeGuard, Subcode: SubcodeStructureLost, Field: field + "/range",
					Message: fmt.Sprintf("the range covers the %s at run %d; edit one of its branches with path", seq[i].Kind(), i)}
			}
		}
		start, end = ownOffset(seq, sel.Range.Start), ownOffset(seq, sel.Range.End)
	default:
		return 0, 0, &Error{Code: CodeInvalid, Field: field, Message: "names its text by exactly one of find, start and end, or range"}
	}
	if err != nil {
		return 0, 0, err
	}
	if j, ok := structureInside(seq, start, end); ok {
		return 0, 0, &Error{Code: CodeGuard, Subcode: SubcodeStructureLost, Field: field,
			Message: fmt.Sprintf("the text spans the %s at run %d; edit one of its branches with path", seq[j].Kind(), j)}
	}
	return start, end, nil
}

// resolveFind finds the occurrence-th match of find in text.
func resolveFind(seq []model.Run, text []rune, find string, occurrence int, path model.RunPath, field string) (int, int, *Error) {
	needle := []rune(find)
	var matches []int
	for i := 0; i+len(needle) <= len(text); {
		if runesAt(text, i, needle) {
			matches = append(matches, i)
			i += len(needle)
			continue
		}
		i++
	}
	switch {
	case len(matches) == 0:
		return 0, 0, &Error{Code: CodeNotFound, Field: field + "/find", Message: fmt.Sprintf("%q is not in the text", find)}
	case occurrence == 0 && len(matches) > 1:
		e := &Error{Code: CodeAmbiguous, Field: field + "/find",
			Message: fmt.Sprintf("%q matches %d times; send occurrence to choose one", find, len(matches))}
		for n, at := range matches {
			r := Resolved{Path: path, Start: posAt(seq, at), End: posAt(seq, at+len(needle))}
			e.Candidates = append(e.Candidates, Candidate{Occurrence: n + 1, At: &r, Text: around(text, at, at+len(needle))})
		}
		return 0, 0, e
	case occurrence > len(matches):
		return 0, 0, &Error{Code: CodeNotFound, Field: field + "/occurrence",
			Message: fmt.Sprintf("%q matches %d times; there is no occurrence %d", find, len(matches), occurrence)}
	}
	at := matches[0]
	if occurrence > 0 {
		at = matches[occurrence-1]
	}
	return at, at + len(needle), nil
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

// ownOffset is the code-point offset into seq's own text of a run position.
func ownOffset(seq []model.Run, p model.RunPos) int {
	off := 0
	for i := 0; i < p.Run && i < len(seq); i++ {
		if seq[i].Text != nil {
			off += utf8.RuneCountInString(seq[i].Text.Text)
		}
	}
	if p.Run < len(seq) && seq[p.Run].Text != nil {
		off += p.Offset
	}
	return off
}

// structureInside reports a plural or select of seq that lies strictly inside
// [start, end) of its own text, which an edit there would delete.
func structureInside(seq []model.Run, start, end int) (int, bool) {
	off := 0
	for i, r := range seq {
		if r.Text != nil {
			off += utf8.RuneCountInString(r.Text.Text)
			continue
		}
		if (r.Plural != nil || r.Select != nil) && start < off && off < end {
			return i, true
		}
	}
	return 0, false
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
