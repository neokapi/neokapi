package change

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Attributes and new codes. A code's native data is the format's own: a runs
// payload never carries it. What a sender may change is what the format's
// writer declares it can write (Capabilities): an attribute of a code
// (set_attribute), and a new paired code of a vocabulary type, around text
// (mark) or named in a runs payload. The writer spells the change into the
// code's native data, escaped for where it is written, so the block holds the
// bytes it will be written with and every writer path, the skeleton replay
// included, writes them.

// codeAt is where a code's opening half or placeholder sits in an edition:
// the path of the sequence that holds it and its index there.
type codeAt struct {
	path model.RunPath
	i    int
}

// findCodes lists where the code with id opens, or sits as a placeholder, in
// runs and the branches of its plurals and selects, in document order.
func findCodes(runs []model.Run, id string, path model.RunPath, into *[]codeAt) {
	for i, r := range runs {
		switch {
		case r.PcOpen != nil && r.PcOpen.ID == id, r.Ph != nil && r.Ph.ID == id:
			*into = append(*into, codeAt{path: slices.Clone(path), i: i})
		case r.Plural != nil:
			for _, k := range sortedKeys(pluralNames(r.Plural.Forms)) {
				p := append(slices.Clone(path), model.RunPathStep{Kind: model.StepIndex, Index: i},
					model.RunPathStep{Kind: model.StepPlural, PluralForm: model.PluralForm(k)})
				findCodes(r.Plural.Forms[model.PluralForm(k)], id, p, into)
			}
		case r.Select != nil:
			for _, k := range sortedKeys(r.Select.Cases) {
				p := append(slices.Clone(path), model.RunPathStep{Kind: model.StepIndex, Index: i},
					model.RunPathStep{Kind: model.StepSelect, SelectValue: k})
				findCodes(r.Select.Cases[k], id, p, into)
			}
		}
	}
}

// codeAttrs is a code's attributes.
func codeAttrs(r model.Run) map[string]string {
	switch {
	case r.PcOpen != nil:
		return r.PcOpen.Attrs
	case r.Ph != nil:
		return r.Ph.Attrs
	}
	return nil
}

// withAttr returns a copy of code r with attribute name set to value.
func withAttr(r model.Run, name, value string) model.Run {
	set := func(m map[string]string) map[string]string {
		out := maps.Clone(m)
		if out == nil {
			out = map[string]string{}
		}
		out[name] = value
		return out
	}
	switch {
	case r.PcOpen != nil:
		c := *r.PcOpen
		c.Attrs = set(c.Attrs)
		return model.Run{PcOpen: &c}
	case r.Ph != nil:
		c := *r.Ph
		c.Attrs = set(c.Attrs)
		return model.Run{Ph: &c}
	}
	return r
}

// unwritableAttr refuses a set_attribute the format does not declare.
func unwritableAttr(caps Capabilities, typ, name string) *Error {
	what := "a code"
	if typ != "" {
		what = "a " + typ + " code"
	}
	writable := caps.Declared.Writable(typ)
	msg := fmt.Sprintf("%s writes no attribute of %s", caps.formatName(), what)
	if len(writable) > 0 {
		msg = fmt.Sprintf("%s cannot write the %s attribute of %s; it writes %s", caps.formatName(), name, what, strings.Join(writable, ", "))
	}
	return &Error{Code: CodeUnsupported, Capability: string(KindSetAttribute), Field: "name",
		Message: msg + ". An attribute the format reads as content, such as a title or an alt text, is changed by editing that content"}
}

// setAttribute applies set_attribute: every occurrence of the code in the
// edition takes the new value, spelled by the format's writer.
func (w *workset) setAttribute(op Op, body *SetAttribute, res *OpResult) *Error {
	st := w.state(op.At.Edition)
	if !st.present {
		return &Error{Code: CodeNotFound, Message: fmt.Sprintf("edition %s does not exist", w.label(st))}
	}
	caps := w.env.Format
	var at []codeAt
	findCodes(st.ed.Runs, body.Code, nil, &at)
	if len(at) == 0 {
		return &Error{Code: CodeNotFound, Field: "code", Message: fmt.Sprintf("edition %s has no code %q", w.label(st), body.Code)}
	}
	next := st.ed.Runs
	for _, loc := range at {
		seq, _ := model.ResolveRunPath(next, loc.path)
		r := seq[loc.i]
		typ := codeType(r)
		if !caps.Declared.CanWrite(typ, body.Name) {
			return unwritableAttr(caps, typ, body.Name)
		}
		if err := attrRunsCode(body.Name, body.Value, codeAttrs(withAttr(r, body.Name, body.Value)), "name", "value"); err != nil {
			return err
		}
		if v, ok := codeAttrs(r)[body.Name]; ok && v == body.Value {
			continue
		}
		var edited []model.Run
		if caps.Attrs != nil {
			out, err := caps.Attrs.WriteAttr(seq, loc.i, body.Name, body.Value)
			if err != nil {
				return &Error{Code: CodeUnsupported, Capability: string(KindSetAttribute), Field: "code",
					Message: fmt.Sprintf("%s cannot write the %s attribute of code %s: %v", caps.formatName(), body.Name, body.Code, err)}
			}
			// The later occurrences were found by their index, so a writer
			// that adds or removes runs would move them.
			if len(out) != len(seq) {
				return &Error{Code: CodeUnsupported, Capability: string(KindSetAttribute), Field: "code",
					Message: fmt.Sprintf("%s's writer changed the number of runs while writing the %s attribute of code %s", caps.formatName(), body.Name, body.Code)}
			}
			edited = out
		} else {
			edited = slices.Clone(seq)
			edited[loc.i] = withAttr(r, body.Name, body.Value)
		}
		var ok bool
		if next, ok = replaceAtPath(next, loc.path, edited); !ok {
			return &Error{Code: CodeNotFound, Field: "code", Message: fmt.Sprintf("code %s is in a branch that no longer exists", body.Code)}
		}
	}
	if sameRuns(st.ed.Runs, next) {
		res.Before, res.After, res.Status = st.startRevision(), st.revision(), OpUnchanged
		return nil
	}
	return w.rewrite(st, next, OverlayRebase{Edits: []model.RunEdit{}}, res)
}

// checkNewCodeAttrs refuses an attribute a new code of type typ may not
// carry: the ones the format writes for the type, and no others, and a value
// that would run code where the document is read (attrRunsCode).
func checkNewCodeAttrs(caps Capabilities, typ string, attrs map[string]string, field string) *Error {
	for _, name := range sortedKeys(attrs) {
		if !caps.Declared.CanWrite(typ, name) {
			e := unwritableAttr(caps, typ, name)
			e.Field = field
			return e
		}
		if err := attrRunsCode(name, attrs[name], attrs, field, field); err != nil {
			return err
		}
	}
	return nil
}

// mark applies mark: a new paired code of a vocabulary type around the text
// the range selects.
func (w *workset) mark(op Op, body *Mark, res *OpResult) *Error {
	st := w.state(op.At.Edition)
	if !st.present {
		return &Error{Code: CodeNotFound, Message: fmt.Sprintf("edition %s does not exist", w.label(st))}
	}
	caps := w.env.Format
	if err := checkNewCodeAttrs(caps, body.Type, body.Attrs, "attrs"); err != nil {
		err.Capability = string(KindMark)
		return err
	}
	cur := st.ed.Runs
	path, perr := w.currentPath(st, body.Range.Path, "range/path")
	if perr != nil {
		return perr
	}
	seq, ok := model.ResolveRunPath(cur, path)
	if !ok {
		return &Error{Code: CodeNotFound, Field: "range/path", Message: fmt.Sprintf("path %s reaches no plural form or select case", pathText(body.Range.Path))}
	}
	var start, end int
	var err *Error
	if w.moved(st) && body.Range.Find == nil {
		// A position names the edition as the change set found it.
		start, end, err = w.movedSelection(st, body.Range, "range")
	} else {
		start, end, err = resolveSelection(seq, body.Range, path, "range")
	}
	if err != nil {
		return err
	}
	if start == end {
		return &Error{Code: CodeInvalid, Field: "range", Message: "the range selects no text; mark wraps text in a new code"}
	}
	id := w.newCodeID()
	open := model.Run{PcOpen: &model.PcOpenRun{ID: id, Type: body.Type, Attrs: maps.Clone(body.Attrs)}}
	closing := model.Run{PcClose: &model.PcCloseRun{ID: id, Type: body.Type}}
	marked, err := wrapSpan(seq, start, end, open, closing)
	if err != nil {
		return err
	}
	if marked, err = synthesizeCodes(w.synthEnv(), marked, map[string]bool{id: true}); err != nil {
		err.Capability = string(KindMark)
		return err
	}
	next, ok := replaceAtPath(cur, path, marked)
	if !ok {
		return &Error{Code: CodeNotFound, Field: "range/path", Message: fmt.Sprintf("path %s reaches no plural form or select case", pathText(path))}
	}
	if err := w.checkCodes(cur, next, nil, res); err != nil {
		return err
	}
	res.Resolved = []Resolved{resolvedSpan(path, posAt(seq, start), posAt(seq, end))}
	return w.rewrite(st, next, OverlayRebase{Edits: []model.RunEdit{}}, res)
}

// synthEnv is what spelling new codes needs: the format's capabilities, the
// vocabulary a code spelled outside the process takes its display from, and
// the block the codes go into.
type synthEnv struct {
	caps  Capabilities
	vocab *model.VocabularyRegistry
	block *model.Block
}

func (w *workset) synthEnv() synthEnv {
	vocab := w.env.Vocabulary
	if vocab == nil {
		vocab = model.DefaultVocabulary()
	}
	return synthEnv{caps: w.env.Format, vocab: vocab, block: w.b}
}

// newCodeID returns an id no code of the block uses, in any edition or in the
// operations applied so far: one above the highest numeric id, so a new code
// never takes the id of a code another edition holds.
func (w *workset) newCodeID() string {
	used := map[string]bool{}
	var collect func(runs []model.Run)
	collect = func(runs []model.Run) {
		for _, r := range runs {
			switch {
			case r.Plural != nil || r.Select != nil:
				forEachBranch(r, collect)
			default:
				if k, ok := keyOf(r); ok {
					used[k.id] = true
				}
			}
		}
	}
	for _, k := range w.b.Editions() {
		if e, ok := w.b.Edition(k); ok {
			collect(e.Runs)
		}
	}
	for _, st := range w.states {
		collect(st.ed.Runs)
	}
	n := 0
	for id := range used {
		if v, err := strconv.Atoi(id); err == nil && v > n {
			n = v
		}
	}
	for {
		n++
		if id := strconv.Itoa(n); !used[id] {
			return id
		}
	}
}

// wrapSpan returns seq with open inserted where the text at code-point offset
// start begins and closing where the text at end ends. A boundary that falls
// between inline codes may sit on either side of each; the new code goes as
// far inside as it can while every code it encloses stays whole, so it nests
// inside or around the codes it meets and never crosses one.
func wrapSpan(seq []model.Run, start, end int, open, closing model.Run) ([]model.Run, *Error) {
	runs := splitTextAt(seq, start, end)
	// The gaps between runs at each offset form one interval.
	var sLo, sHi, eLo, eHi = -1, -1, -1, -1
	off := 0
	for g := 0; g <= len(runs); g++ {
		if off == start {
			if sLo < 0 {
				sLo = g
			}
			sHi = g
		}
		if off == end {
			if eLo < 0 {
				eLo = g
			}
			eHi = g
		}
		if g < len(runs) && runs[g].Text != nil {
			off += utf8.RuneCountInString(runs[g].Text.Text)
		}
	}
	if sLo < 0 || eLo < 0 {
		return nil, guardf(SubcodeBadPosition, "[%d, %d) is outside the text", start, end)
	}
	for s := sHi; s >= sLo; s-- {
		for e := eLo; e <= eHi; e++ {
			if s >= e || !wholeCodes(runs[s:e]) {
				continue
			}
			out := make([]model.Run, 0, len(runs)+2)
			out = append(out, runs[:s]...)
			out = append(out, open)
			out = append(out, runs[s:e]...)
			out = append(out, closing)
			out = append(out, runs[e:]...)
			return out, nil
		}
	}
	return nil, guardf(SubcodeBadPosition, "the text crosses the boundary of an inline code; a new code nests inside or around the codes it meets")
}

// wholeCodes reports whether every paired code in runs opens and closes
// within it, and runs holds no plural or select.
func wholeCodes(runs []model.Run) bool {
	open := map[string]int{}
	for _, r := range runs {
		switch {
		case r.Plural != nil, r.Select != nil:
			return false
		case r.PcOpen != nil:
			open[r.PcOpen.ID]++
		case r.PcClose != nil:
			if open[r.PcClose.ID] == 0 {
				return false
			}
			open[r.PcClose.ID]--
		}
	}
	for _, n := range open {
		if n != 0 {
			return false
		}
	}
	return true
}

// splitTextAt returns seq with each text run that holds one of the offsets
// strictly inside it split there. Both halves keep the run's flags.
func splitTextAt(seq []model.Run, offsets ...int) []model.Run {
	out := make([]model.Run, 0, len(seq)+len(offsets))
	off := 0
	for _, r := range seq {
		if r.Text == nil {
			out = append(out, r)
			continue
		}
		text := []rune(r.Text.Text)
		n := len(text)
		cuts := []int{}
		for _, o := range offsets {
			if o > off && o < off+n && !slices.Contains(cuts, o-off) {
				cuts = append(cuts, o-off)
			}
		}
		slices.Sort(cuts)
		prev := 0
		for _, c := range append(cuts, n) {
			t := *r.Text
			t.Text = string(text[prev:c])
			out = append(out, model.Run{Text: &t})
			prev = c
		}
		off += n
	}
	return out
}

// synthesizeCodes gives each new paired code in runs whose id pending names
// its native form, spelled by the format's writer for where the code sits. A
// writer outside the process (caps.Codes nil) spells it when it writes; the
// code then carries the vocabulary's display and constraints and no data.
func synthesizeCodes(se synthEnv, runs []model.Run, pending map[string]bool) ([]model.Run, *Error) {
	caps := se.caps
	out := slices.Clone(runs)
	for i := range out {
		r := out[i]
		switch {
		case r.Plural != nil:
			p := *r.Plural
			p.Forms = make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))
			for k, form := range r.Plural.Forms {
				next, err := synthesizeCodes(se, form, pending)
				if err != nil {
					return nil, err
				}
				p.Forms[k] = next
			}
			out[i] = model.Run{Plural: &p}
			continue
		case r.Select != nil:
			s := *r.Select
			s.Cases = make(map[string][]model.Run, len(r.Select.Cases))
			for k, c := range r.Select.Cases {
				next, err := synthesizeCodes(se, c, pending)
				if err != nil {
					return nil, err
				}
				s.Cases[k] = next
			}
			out[i] = model.Run{Select: &s}
			continue
		case r.PcOpen == nil || !pending[r.PcOpen.ID]:
			continue
		}
		id := r.PcOpen.ID
		j := slices.IndexFunc(out[i+1:], func(c model.Run) bool { return c.PcClose != nil && c.PcClose.ID == id })
		if j < 0 {
			return nil, &Error{Code: CodeInvalid, Field: "runs", Message: fmt.Sprintf(`the new code <x id="%s"/> opens and does not close in the same run sequence`, id)}
		}
		j += i + 1
		site := format.CodeSite{
			Type:      r.PcOpen.Type,
			Attrs:     maps.Clone(r.PcOpen.Attrs),
			Block:     se.block,
			Enclosing: enclosingCodes(out[:i]),
			Before:    out[:i:i],
			Inner:     out[i+1 : j : j],
			After:     out[j+1:],
		}
		var open, closing model.Run
		if caps.Codes != nil {
			o, c, err := caps.Codes.SynthesizeCode(site)
			if err != nil {
				return nil, &Error{Code: CodeUnsupported, Capability: "synthesize:" + site.Type, Field: "type",
					Message: fmt.Sprintf("%s cannot write a new %s code here: %v", caps.formatName(), site.Type, err)}
			}
			if o.PcOpen == nil || c.PcClose == nil {
				return nil, &Error{Code: CodeUnsupported, Capability: "synthesize:" + site.Type,
					Message: fmt.Sprintf("%s wrote a new %s code that is not a pair", caps.formatName(), site.Type)}
			}
			open, closing = o, c
		} else {
			info := se.vocab.LookupOrFallback(site.Type)
			o := model.PcOpenRun{Type: site.Type, Attrs: site.Attrs, Equiv: info.Equiv, Disp: info.Display.Open,
				Constraints: &model.RunConstraints{Deletable: info.Constraints.Deletable, Cloneable: info.Constraints.Cloneable, Reorderable: info.Constraints.Reorderable}}
			open, closing = model.Run{PcOpen: &o}, model.Run{PcClose: &model.PcCloseRun{Type: site.Type}}
		}
		po, pc := *open.PcOpen, *closing.PcClose
		po.ID, pc.ID = id, id
		out[i], out[j] = model.Run{PcOpen: &po}, model.Run{PcClose: &pc}
	}
	return out, nil
}

// enclosingCodes lists the opening halves of the paired codes still open at
// the end of runs, outermost first.
func enclosingCodes(runs []model.Run) []model.Run {
	var stack []model.Run
	for _, r := range runs {
		switch {
		case r.PcOpen != nil:
			stack = append(stack, r)
		case r.PcClose != nil:
			for k, open := range slices.Backward(stack) {
				if open.PcOpen.ID == r.PcClose.ID {
					stack = slices.Delete(stack, k, k+1)
					break
				}
			}
		}
	}
	return stack
}
