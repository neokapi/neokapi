package tools

import (
	"fmt"
	"maps"
	"slices"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// textSeq is one run sequence as a text transform reads it: its own text
// (model.SequenceText), in which every inline code has zero width, and where
// its codes sit in that text.
type textSeq struct {
	text  []rune
	codes []seqCode // in sequence order
}

// seqCode is one inline code of a sequence: its offset in the sequence's own
// text and how an edit around it treats it.
type seqCode struct {
	at   int
	kind codeKind
}

type codeKind uint8

const (
	// pairedCode is one half of a paired code whose other half is in the
	// same sequence. The pair follows the text it wraps, as
	// model.ApplyTextEdits keeps it.
	pairedCode codeKind = iota
	// standaloneCode is a placeholder, a subblock reference, or a paired
	// half whose partner the sequence does not hold.
	standaloneCode
	// structureCode is a plural or a select, whose words are in its branches.
	structureCode
)

func readSeq(runs []model.Run) textSeq {
	var ts textSeq
	open := map[string]int{} // a paired open's index in codes, by id
	for _, r := range runs {
		switch {
		case r.Text != nil:
			ts.text = append(ts.text, []rune(r.Text.Text)...)
		case r.Plural != nil, r.Select != nil:
			ts.codes = append(ts.codes, seqCode{at: len(ts.text), kind: structureCode})
		case r.PcOpen != nil:
			open[r.PcOpen.ID] = len(ts.codes)
			ts.codes = append(ts.codes, seqCode{at: len(ts.text), kind: standaloneCode})
		case r.PcClose != nil:
			kind := standaloneCode
			if i, ok := open[r.PcClose.ID]; ok {
				ts.codes[i].kind = pairedCode
				kind = pairedCode
				delete(open, r.PcClose.ID)
			}
			ts.codes = append(ts.codes, seqCode{at: len(ts.text), kind: kind})
		default:
			ts.codes = append(ts.codes, seqCode{at: len(ts.text), kind: standaloneCode})
		}
	}
	return ts
}

// structureInside reports whether a plural or select sits strictly inside
// [start, end): an edit there would take it with the text.
func (ts textSeq) structureInside(start, end int) bool {
	for _, c := range ts.codes {
		if c.kind == structureCode && start < c.at && c.at < end {
			return true
		}
	}
	return false
}

// textMatch is one change a pass makes to a sequence: the edits that make it,
// over [start, end) of the sequence's own text. A change the edition cannot
// take carries the reason in skip and no edits.
type textMatch struct {
	start, end int
	edits      []model.TextEdit
	skip       string
}

// textPass is one pass of a text transform: the changes it makes to a
// sequence, in text order and not overlapping. A pass marked first makes only
// the first change it finds in the edition, in reading order.
type textPass struct {
	matches func(textSeq) []textMatch
	first   bool
}

// skippedMatch is a change a pass found and could not make.
type skippedMatch struct {
	edition model.VariantKey
	text    string
	reason  string
}

// textPlan builds the EditPlan for a text-level transform applied to the
// source and/or a set of target locales: the shared shape of the simple
// transformers (case, search-replace, ksed). Each pass edits the edition's
// text in place, as a replace_text through core/change, so the inline codes,
// the plural and select structure and the run flags the edits leave alone are
// kept, and the edition's overlays follow the edits. A plural form or a select
// case is a text of its own. A locale without a committed target is skipped;
// an edition no pass changes produces no edit. It also returns the changes the
// passes found and could not make.
func textPlan(v tool.BlockView, applySource bool, targets []model.LocaleID, passes ...textPass) (tool.EditPlan, []skippedMatch) {
	var plan tool.EditPlan
	var skipped []skippedMatch
	edit := func(key model.VariantKey, runs []model.Run) {
		cur := runs
		for _, pass := range passes {
			pe := &passEdits{}
			cur = pe.run(cur, pass)
			plan.AddTextEdits(key, pe.edits)
			for _, m := range pe.skipped {
				skipped = append(skipped, skippedMatch{edition: key, text: m.text, reason: m.reason})
			}
		}
	}
	if applySource {
		edit(model.VariantKey{}, v.SourceRuns())
	}
	for _, loc := range targets {
		if !v.HasTarget(loc) {
			continue
		}
		edit(model.Variant(loc), v.TargetRuns(loc))
	}
	return plan, skipped
}

// reportSkipped records the changes a transform could not make as findings
// that report and do not fail: the run goes on, and the person reading its
// findings learns which text was left as it was.
func reportSkipped(v tool.BlockView, toolName string, skipped []skippedMatch) {
	if len(skipped) == 0 {
		return
	}
	findings := make([]check.Finding, 0, len(skipped))
	for _, s := range skipped {
		f := check.Finding{
			Category:     toolName,
			Message:      fmt.Sprintf("%q was left as it was: %s", s.text, s.reason),
			OriginalText: s.text,
		}
		if !s.edition.IsZero() {
			text, _ := s.edition.MarshalText()
			f.Metadata = map[string]string{"edition": string(text)}
		}
		findings = append(findings, f)
	}
	check.Annotate(v, toolName, findings)
}

// passEdits collects one pass's edits over an edition.
type passEdits struct {
	edits   []change.TextEdit
	skipped []struct{ text, reason string }
}

// run applies pass to runs, the edition's sequence, and returns it edited.
func (pe *passEdits) run(runs []model.Run, pass textPass) []model.Run {
	if !pass.first {
		return pe.editSequence(runs, nil, pass)
	}
	path, m, ok := pe.firstMatch(runs, nil, pass)
	if !ok {
		return runs
	}
	pe.add(path, m.edits)
	seq, _ := model.ResolveRunPath(runs, path)
	return replaceSequence(runs, path, model.ApplyTextEdits(seq, m.edits))
}

func (pe *passEdits) add(path model.RunPath, edits []model.TextEdit) {
	for _, e := range edits {
		start, end := e.Start, e.End
		pe.edits = append(pe.edits, change.TextEdit{Path: slices.Clone(path), Start: &start, End: &end, Text: e.Replacement})
	}
}

func (pe *passEdits) skip(ts textSeq, m textMatch) {
	pe.skipped = append(pe.skipped, struct{ text, reason string }{string(ts.text[m.start:m.end]), m.skip})
}

// editSequence makes every change pass finds in seq, the sequence path
// reaches, and in every plural form and select case inside it, and returns seq
// as edited. A nested sequence is edited first and addressed by its position
// in seq before the edit, which is the order replace_text applies edits in,
// deepest first.
func (pe *passEdits) editSequence(seq []model.Run, path model.RunPath, pass textPass) []model.Run {
	var next []model.Run
	for i, r := range seq {
		switch {
		case r.Plural != nil:
			forms := make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))
			for _, form := range pluralOrder(r.Plural.Forms) {
				step := model.RunPathStep{Kind: model.StepPlural, PluralForm: form}
				forms[form] = pe.editSequence(r.Plural.Forms[form], appendPath(path, i, step), pass)
			}
			p := *r.Plural
			p.Forms = forms
			next = append(next, model.Run{Plural: &p})
		case r.Select != nil:
			cases := make(map[string][]model.Run, len(r.Select.Cases))
			for _, value := range selectOrder(r.Select.Cases) {
				step := model.RunPathStep{Kind: model.StepSelect, SelectValue: value}
				cases[value] = pe.editSequence(r.Select.Cases[value], appendPath(path, i, step), pass)
			}
			s := *r.Select
			s.Cases = cases
			next = append(next, model.Run{Select: &s})
		default:
			next = append(next, r)
		}
	}
	ts := readSeq(next)
	var edits []model.TextEdit
	for _, m := range pass.matches(ts) {
		if m.skip != "" {
			pe.skip(ts, m)
			continue
		}
		edits = append(edits, m.edits...)
	}
	if len(edits) == 0 {
		return next
	}
	pe.add(path, edits)
	return model.ApplyTextEdits(next, edits)
}

// firstMatch finds the first change pass can make in seq, the sequence path
// reaches, in reading order: the text before a plural or select, then each of
// its branches, then the text after it. A change before it that cannot be made
// is recorded as skipped.
func (pe *passEdits) firstMatch(seq []model.Run, path model.RunPath, pass textPass) (model.RunPath, textMatch, bool) {
	ts := readSeq(seq)
	matches := pass.matches(ts)
	mi := 0
	// take returns the next makeable match that starts before limit; an
	// insertion at limit goes before the structure there, so it counts.
	take := func(limit int) (textMatch, bool) {
		for mi < len(matches) {
			m := matches[mi]
			if m.start > limit || m.start == limit && m.end > limit {
				return textMatch{}, false
			}
			mi++
			if m.skip != "" {
				pe.skip(ts, m)
				continue
			}
			return m, true
		}
		return textMatch{}, false
	}
	own := 0
	for i, r := range seq {
		if r.Text != nil {
			own += len([]rune(r.Text.Text))
			continue
		}
		var branches []struct {
			step model.RunPathStep
			seq  []model.Run
		}
		switch {
		case r.Plural != nil:
			for _, form := range pluralOrder(r.Plural.Forms) {
				branches = append(branches, struct {
					step model.RunPathStep
					seq  []model.Run
				}{model.RunPathStep{Kind: model.StepPlural, PluralForm: form}, r.Plural.Forms[form]})
			}
		case r.Select != nil:
			for _, value := range selectOrder(r.Select.Cases) {
				branches = append(branches, struct {
					step model.RunPathStep
					seq  []model.Run
				}{model.RunPathStep{Kind: model.StepSelect, SelectValue: value}, r.Select.Cases[value]})
			}
		default:
			continue
		}
		if m, ok := take(own); ok {
			return path, m, true
		}
		for _, b := range branches {
			if p, m, ok := pe.firstMatch(b.seq, appendPath(path, i, b.step), pass); ok {
				return p, m, true
			}
		}
	}
	if m, ok := take(len(ts.text)); ok {
		return path, m, true
	}
	return nil, textMatch{}, false
}

// pluralOrder lists a plural's forms in the order a reader meets them: the
// CLDR categories from zero to other, then any other form by name.
func pluralOrder(forms map[model.PluralForm][]model.Run) []model.PluralForm {
	rank := map[model.PluralForm]int{model.PluralZero: 1, model.PluralOne: 2, model.PluralTwo: 3,
		model.PluralFew: 4, model.PluralMany: 5, model.PluralOther: 6}
	return slices.SortedFunc(maps.Keys(forms), func(a, b model.PluralForm) int {
		ra, rb := rank[a], rank[b]
		if ra != rb {
			return ra - rb
		}
		switch {
		case a < b:
			return -1
		case a > b:
			return 1
		}
		return 0
	})
}

// selectOrder lists a select's cases by name, with other last.
func selectOrder(cases map[string][]model.Run) []string {
	return slices.SortedFunc(maps.Keys(cases), func(a, b string) int {
		switch {
		case a == b:
			return 0
		case a == "other":
			return 1
		case b == "other":
			return -1
		case a < b:
			return -1
		}
		return 1
	})
}

// replaceSequence returns runs with the sequence path reaches replaced by seq.
func replaceSequence(runs []model.Run, path model.RunPath, seq []model.Run) []model.Run {
	if len(path) < 2 {
		return seq
	}
	out := slices.Clone(runs)
	r := out[path[0].Index]
	step := path[1]
	switch {
	case step.Kind == model.StepPlural && r.Plural != nil:
		p := *r.Plural
		p.Forms = maps.Clone(r.Plural.Forms)
		p.Forms[step.PluralForm] = replaceSequence(p.Forms[step.PluralForm], path[2:], seq)
		out[path[0].Index] = model.Run{Plural: &p}
	case step.Kind == model.StepSelect && r.Select != nil:
		s := *r.Select
		s.Cases = maps.Clone(r.Select.Cases)
		s.Cases[step.SelectValue] = replaceSequence(s.Cases[step.SelectValue], path[2:], seq)
		out[path[0].Index] = model.Run{Select: &s}
	}
	return out
}

// appendPath is path extended into run i of the sequence it reaches, then
// into a branch of that run.
func appendPath(path model.RunPath, i int, branch model.RunPathStep) model.RunPath {
	out := slices.Clone(path)
	return append(out, model.RunPathStep{Kind: model.StepIndex, Index: i}, branch)
}
