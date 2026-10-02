package tools

import (
	"maps"
	"slices"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// textRewrite is one pass of a text transform: the edits it makes to a text,
// as code-point ranges of that text, sorted and non-overlapping.
type textRewrite func(text string) []model.TextEdit

// textPlan builds the EditPlan for a text-level transform applied to the
// source and/or a set of target locales: the shared shape of the simple
// transformers (case, search-replace). Each pass edits the edition's text in
// place, as a replace_text through core/change, so the inline codes, the
// plural and select structure and the run flags the edits leave alone are
// kept, and the source's overlays follow the edits. A plural form or a select
// case is a text of its own. A locale without a committed target is skipped;
// an edition no pass changes produces no edit.
func textPlan(v tool.BlockView, applySource bool, targets []model.LocaleID, passes ...textRewrite) tool.EditPlan {
	var plan tool.EditPlan
	if applySource {
		for _, edits := range textPasses(v.SourceRuns(), passes) {
			plan.AddTextEdits(model.VariantKey{}, edits)
		}
	}
	for _, loc := range targets {
		if !v.HasTarget(loc) {
			continue
		}
		for _, edits := range textPasses(v.TargetRuns(loc), passes) {
			plan.AddTextEdits(model.Variant(loc), edits)
		}
	}
	return plan
}

// textPasses runs passes over runs in order and returns the edits of each pass
// that changed something. A pass reads the text the passes before it left.
func textPasses(runs []model.Run, passes []textRewrite) [][]change.TextEdit {
	var out [][]change.TextEdit
	cur := runs
	for _, pass := range passes {
		var edits []change.TextEdit
		cur = editSequence(cur, nil, pass, &edits)
		if len(edits) > 0 {
			out = append(out, edits)
		}
	}
	return out
}

// editSequence applies pass to the sequence path reaches, seq, and to every
// plural form and select case inside it, appends the edits it made to out, and
// returns seq as edited. A nested sequence is edited first and addressed by
// its position in seq before the edit, which is the order replace_text applies
// edits in, deepest first.
func editSequence(seq []model.Run, path model.RunPath, pass textRewrite, out *[]change.TextEdit) []model.Run {
	var next []model.Run
	for i, r := range seq {
		switch {
		case r.Plural != nil:
			forms := make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))
			for _, form := range slices.Sorted(maps.Keys(r.Plural.Forms)) {
				step := model.RunPathStep{Kind: model.StepPlural, PluralForm: form}
				forms[form] = editSequence(r.Plural.Forms[form], appendPath(path, i, step), pass, out)
			}
			p := *r.Plural
			p.Forms = forms
			next = append(next, model.Run{Plural: &p})
		case r.Select != nil:
			cases := make(map[string][]model.Run, len(r.Select.Cases))
			for _, value := range slices.Sorted(maps.Keys(r.Select.Cases)) {
				step := model.RunPathStep{Kind: model.StepSelect, SelectValue: value}
				cases[value] = editSequence(r.Select.Cases[value], appendPath(path, i, step), pass, out)
			}
			s := *r.Select
			s.Cases = cases
			next = append(next, model.Run{Select: &s})
		default:
			next = append(next, r)
		}
	}
	edits := pass(model.SequenceText(next))
	if len(edits) == 0 {
		return next
	}
	for _, e := range edits {
		start, end := e.Start, e.End
		*out = append(*out, change.TextEdit{Path: slices.Clone(path), Start: &start, End: &end, Text: e.Replacement})
	}
	return model.ApplyTextEdits(next, edits)
}

// appendPath is path extended into run i of the sequence it reaches, then
// into a branch of that run.
func appendPath(path model.RunPath, i int, branch model.RunPathStep) model.RunPath {
	out := slices.Clone(path)
	return append(out, model.RunPathStep{Kind: model.StepIndex, Index: i}, branch)
}
