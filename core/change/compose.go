package change

import (
	"fmt"
	"slices"

	"github.com/neokapi/neokapi/core/model"
)

// Composing the operations of one change set on one edition.
//
// The operations apply in order. Each reads the edition as the operations
// before it left it, except where its sender named a place by position: the
// start and end offsets of a text edit, a run range, and the run index a path
// walks through all name the edition as the change set found it, the revision
// the sender read and sent as if_match. A sender computes every position of a
// change set against that one read (kapi check prints one fix per finding,
// each guarded by the revision the check read), so the service moves each
// position through the text the earlier operations changed, in the order they
// were sent. A position inside text an earlier operation replaced has no place
// in the edition as it stands, and the operation is refused as an overlap that
// names the operation whose change it overlaps. A find matches the text as the
// earlier operations left it, and an annotation's anchor marks that text too.

// seqChange is the text one operation changed in one run sequence of an
// edition: its edits, in the sequence's own-text offsets as the operation found
// it, or the whole sequence for a set_content.
type seqChange struct {
	// op is the operation's place in the ops ApplyBlock was given.
	op int
	// seq is the sequence, by the structures its path walks through.
	seq []ordStep
	// whole is a set_content of the sequence: nothing in it keeps a place.
	whole bool
	// edits are sorted and do not overlap.
	edits []seqEdit
}

// seqEdit is one replacement in a sequence's own text.
type seqEdit struct {
	start, end, newLen int
}

// ordStep is one step of a run path with the run index replaced by the
// structure's ordinal among the plurals and selects of its sequence. A text
// edit never adds or removes a plural or select, so an ordinal path names the
// same branch before and after one, where a run index may move as text runs
// merge, split or empty.
type ordStep struct {
	n      int
	branch model.RunPathStep
}

// ordinalsOf turns a run path into ordinal steps over runs. The last branch
// may be one the structure lacks, as a set_content that adds a plural form
// names it.
func ordinalsOf(runs []model.Run, path model.RunPath) ([]ordStep, bool) {
	if len(path)%2 != 0 {
		return nil, false
	}
	var out []ordStep
	seq := runs
	for i := 0; i < len(path); i += 2 {
		idx, br := path[i], path[i+1]
		if idx.Kind != model.StepIndex || idx.Index < 0 || idx.Index >= len(seq) || !structured(seq[idx.Index]) {
			return nil, false
		}
		n := 0
		for _, r := range seq[:idx.Index] {
			if structured(r) {
				n++
			}
		}
		out = append(out, ordStep{n: n, branch: br})
		next, ok := branchSeq(seq[idx.Index], br)
		if !ok && i+2 < len(path) {
			return nil, false
		}
		seq = next
	}
	return out, true
}

// pathOf turns ordinal steps back into a run path over runs.
func pathOf(runs []model.Run, ords []ordStep) (model.RunPath, bool) {
	var out model.RunPath
	seq := runs
	for i, o := range ords {
		at, n := -1, 0
		for j, r := range seq {
			if !structured(r) {
				continue
			}
			if n == o.n {
				at = j
				break
			}
			n++
		}
		if at < 0 {
			return nil, false
		}
		out = append(out, model.RunPathStep{Kind: model.StepIndex, Index: at}, o.branch)
		next, ok := branchSeq(seq[at], o.branch)
		if !ok && i+1 < len(ords) {
			return nil, false
		}
		seq = next
	}
	return out, true
}

func structured(r model.Run) bool { return r.Plural != nil || r.Select != nil }

// branchSeq is the branch of a plural or select a path step names.
func branchSeq(r model.Run, step model.RunPathStep) ([]model.Run, bool) {
	switch {
	case step.Kind == model.StepPlural && r.Plural != nil:
		f, ok := r.Plural.Forms[step.PluralForm]
		return f, ok
	case step.Kind == model.StepSelect && r.Select != nil:
		c, ok := r.Select.Cases[step.SelectValue]
		return c, ok
	}
	return nil, false
}

// sameOrds reports whether two ordinal paths name one sequence.
func sameOrds(a, b []ordStep) bool { return slices.Equal(a, b) }

// ordsUnder reports whether the sequence b names lies in, or is, the one a
// names.
func ordsUnder(a, b []ordStep) bool { return len(a) <= len(b) && slices.Equal(a, b[:len(a)]) }

// PathIn returns the path in to that reaches the plural form or select case
// path reaches in from, where to is from after text edits: plurals and selects
// are matched by their order in each sequence, which a text edit keeps while
// the run index of each may move.
func PathIn(from, to []model.Run, path model.RunPath) (model.RunPath, bool) {
	ords, ok := ordinalsOf(from, path)
	if !ok {
		return nil, false
	}
	return pathOf(to, ords)
}

// moved reports whether a position an operation names in edition st is moved
// to where it lies now: an operation before it changed the edition's runs,
// and the operations name the edition as the change set found it.
func (w *workset) moved(st *edState) bool { return st.rebuilt && !w.env.Chained }

// currentPath is the path to the sequence a path names in the edition as the
// change set found it, in the edition as the operations before this one left
// it. A path that reaches into content an earlier set_content replaced, or
// that the edition did not hold at the start, is read in the edition as it
// stands.
func (w *workset) currentPath(st *edState, path model.RunPath) model.RunPath {
	if len(path) == 0 || !w.moved(st) {
		return path
	}
	ords, ok := ordinalsOf(st.startRuns, path)
	if !ok {
		return path
	}
	for _, c := range st.changes {
		if c.whole && len(c.seq) < len(ords) && ordsUnder(c.seq, ords) {
			return path
		}
	}
	if p, ok := pathOf(st.ed.Runs, ords); ok {
		return p
	}
	return path
}

// movedSelection resolves a selection that names its text by position (start
// and end, or a run range) in the edition as the change set found it, and
// moves it through the text the operations before this one changed in the
// same sequence. It returns offsets into the sequence's own text as it stands.
func (w *workset) movedSelection(st *edState, sel Selection, field string) (start, end int, err *Error) {
	ords, ok := ordinalsOf(st.startRuns, sel.Path)
	if !st.startPresent {
		ok = false
	}
	for _, c := range st.changes {
		// A set_content of the sequence, or of one it lies in, leaves no
		// place for a position in it. A path the edition did not hold at the
		// start lies in the edition's whole content only.
		if c.whole && (ok && ordsUnder(c.seq, ords) || !ok && len(c.seq) == 0) {
			return 0, 0, w.overlap(c.op, field, "replaced whole")
		}
	}
	if !ok {
		seq, found := model.ResolveRunPath(st.ed.Runs, sel.Path)
		if !found {
			return 0, 0, &Error{Code: CodeNotFound, Field: field + "/path", Message: fmt.Sprintf("path %s reaches no plural form or select case", pathText(sel.Path))}
		}
		return resolveSelection(seq, sel, sel.Path, field)
	}
	key := pathText(sel.Path)
	ix := st.startIndex[key]
	if ix == nil {
		seq, _ := model.ResolveRunPath(st.startRuns, sel.Path)
		ix = indexSequence(seq)
		if st.startIndex == nil {
			st.startIndex = map[string]*seqIndex{}
		}
		st.startIndex[key] = ix
	}
	if start, end, err = ix.resolve(sel, sel.Path, field); err != nil {
		return 0, 0, err
	}
	for _, c := range st.changes {
		if !sameOrds(c.seq, ords) {
			continue
		}
		var moved bool
		if start, end, moved = moveRange(c.edits, start, end); !moved {
			return 0, 0, w.overlap(c.op, field, "changed")
		}
	}
	return start, end, nil
}

// overlap is the refusal of a position inside text an earlier operation
// changed.
func (w *workset) overlap(earlier int, field, did string) *Error {
	return &Error{Code: CodeGuard, Subcode: SubcodeOverlap, Field: field,
		Message: fmt.Sprintf("operation %d names a position in text operation %d %s; positions name the edition as the change set found it, "+
			"so send the two changes as one operation, or name this text by find", w.index(w.at), w.index(earlier), did)}
}

// index is the place an operation of ApplyBlock's ops has in its change set.
func (w *workset) index(i int) int {
	if i >= 0 && i < len(w.env.Indexes) {
		return w.env.Indexes[i]
	}
	return i
}

// moveRange moves [start, end) of a sequence's own text through the edits an
// earlier operation made to it, which are sorted and in the offsets the
// sequence had before them. It reports false when the range overlaps text an
// edit replaced or encloses a place an edit inserted text at.
//
// The start of a range that holds text follows text inserted where it lies,
// and its end stays before it, so neither range takes in the other's text. A
// range that holds no text, an insertion, goes after text an earlier operation
// inserted at the same place, in the order the two were sent.
func moveRange(edits []seqEdit, start, end int) (int, int, bool) {
	for _, e := range edits {
		switch {
		case start == end && e.start < start && start < e.end:
			return 0, 0, false
		case start < end && e.start < e.end && e.start < end && start < e.end:
			return 0, 0, false
		case start < end && e.start == e.end && start < e.start && e.start < end:
			return 0, 0, false
		}
	}
	return movePos(edits, start, true), movePos(edits, end, start == end), true
}

// movePos moves an offset that lies inside no replaced text past the edits
// before it. after places an offset where text was inserted after that text.
func movePos(edits []seqEdit, p int, after bool) int {
	out := p
	for _, e := range edits {
		if e.end < p || (e.end == p && (e.start < e.end || after)) {
			out += e.newLen - (e.end - e.start)
		}
	}
	return out
}

// noteChange records the text an operation changed in one sequence of an
// edition, so later positions move through it.
func (w *workset) noteChange(st *edState, path model.RunPath, before []model.Run, whole bool, edits []seqEdit) {
	ords, ok := ordinalsOf(before, path)
	if !ok {
		return
	}
	st.changes = append(st.changes, seqChange{op: w.at, seq: ords, whole: whole, edits: edits})
}
