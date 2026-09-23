package contextop

import (
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/profile"
)

// contest marks the records that disagree, in place, after the fold has
// settled every other status.
//
// Three disagreements are detected:
//
//   - a person's correction that reverses an established rule, recorded after
//     the rule was last established: the rule is contested by the correction,
//     so it reports instead of failing the person's own edit;
//   - a suggestion that disagrees with an established rule: the suggestion is
//     contested by the rule, and the rule stays in force;
//   - two suggestions that disagree: both are contested, each naming the other.
//
// Two rules disagree when they share a form to avoid and name different forms
// to use, or when one avoids the form the other says to use. Rules at points
// that cannot meet, in different projects or at different coordinates, never
// disagree.
//
// establishedAt holds, per record, the position of the act that last
// established it.
func contest(records []Record, establishedAt []int64) {
	for c := range records {
		corr := records[c]
		if corr.Kind != KindCorrect || corr.Correction == nil || corr.Actor.Kind != ActorPerson || !corr.Status.Answers() {
			continue
		}
		for e := range records {
			rec := records[e]
			if !rec.Established || rec.Seq > corr.Seq || establishedAt[e] > corr.Seq || !meet(rec, corr) {
				continue
			}
			rule, ok := rec.Rule()
			if !ok || rule.Replacement == "" {
				continue
			}
			cs := rule.CaseSensitive
			if formKey(corr.Correction.From, cs) == formKey(rule.Replacement, cs) && slices.Contains(avoided(rule, cs), formKey(corr.Correction.To, cs)) {
				markContested(&records[e], corr.ID)
			}
		}
	}

	var live []int
	for i, r := range records {
		if !r.Kind.Bears() || !r.Status.Answers() {
			continue
		}
		if rule, ok := r.Rule(); ok && rule.Replacement != "" {
			live = append(live, i)
		}
	}
	for x := range live {
		for y := x + 1; y < len(live); y++ {
			a, b := &records[live[x]], &records[live[y]]
			if a.Established && b.Established {
				continue
			}
			if !meet(*a, *b) {
				continue
			}
			ra, _ := a.Rule()
			rb, _ := b.Rule()
			if !disagree(ra, rb) {
				continue
			}
			switch {
			case a.Established:
				markContested(b, a.ID)
			case b.Established:
				markContested(a, b.ID)
			default:
				markContested(a, b.ID)
				markContested(b, a.ID)
			}
		}
	}
}

// markContested sets a record contested by another, naming it once.
func markContested(r *Record, by string) {
	r.Status = StatusContested
	if !slices.Contains(r.ContestedBy, by) {
		r.ContestedBy = append(r.ContestedBy, by)
	}
}

// meet reports whether two records answer at a common point: the same project,
// or either widened to the workspace, and no coordinate they both name set to
// different values.
func meet(a, b Record) bool {
	if a.Project != b.Project && a.Scope.Level != LevelWorkspace && b.Scope.Level != LevelWorkspace {
		return false
	}
	for axis, value := range a.Scope.Coordinates {
		if other, ok := b.Scope.Coordinates[axis]; ok && other != value {
			return false
		}
	}
	return true
}

// disagree reports whether two term rules say different things about one
// word. A rule that is case-sensitive compares forms as written, so
// `Quickcast` and `quickcast` stay two forms.
func disagree(a, b profile.TermRule) bool {
	cs := a.CaseSensitive || b.CaseSensitive
	avoidA, avoidB := avoided(a, cs), avoided(b, cs)
	useA, useB := formKey(a.Replacement, cs), formKey(b.Replacement, cs)
	if useA != useB {
		for _, form := range avoidA {
			if slices.Contains(avoidB, form) {
				return true
			}
		}
	}
	return slices.Contains(avoidA, useB) || slices.Contains(avoidB, useA)
}

// avoided lists the forms a rule says to avoid, keyed for comparison.
func avoided(rule profile.TermRule, caseSensitive bool) []string {
	out := make([]string, 0, 1+len(rule.Forms))
	for _, form := range append([]string{rule.Term}, rule.Forms...) {
		if key := formKey(form, caseSensitive); key != "" {
			out = append(out, key)
		}
	}
	return out
}

// formKey keys a form for comparison: as written when case matters, folded
// when it does not.
func formKey(form string, caseSensitive bool) string {
	form = strings.TrimSpace(form)
	if caseSensitive {
		return form
	}
	return strings.ToLower(form)
}
