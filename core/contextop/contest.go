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
			cs := rule.MatchesCase()
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
	for _, pair := range candidatePairs(records, live) {
		a, b := &records[live[pair[0]]], &records[live[pair[1]]]
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

// candidatePairs lists the pairs of live rules that could disagree, as
// positions in live with the first below the second, in ascending order.
//
// Two rules can only disagree when they share a form, once case is folded:
// an avoided form both name, or one rule's replacement the other avoids. So the
// rules are bucketed by every form they name, and only rules sharing a bucket
// are compared. A log holding hundreds of rules then costs what its overlapping
// words cost, rather than every rule against every other.
func candidatePairs(records []Record, live []int) [][2]int {
	buckets := map[string][]int{}
	for x, i := range live {
		rule, _ := records[i].Rule()
		seen := map[string]bool{}
		for _, form := range append(avoided(rule, false), formKey(rule.Replacement, false)) {
			if form == "" || seen[form] {
				continue
			}
			seen[form] = true
			buckets[form] = append(buckets[form], x)
		}
	}
	pairs := map[[2]int]bool{}
	for _, members := range buckets {
		for p := range members {
			for q := p + 1; q < len(members); q++ {
				pairs[[2]int{members[p], members[q]}] = true
			}
		}
	}
	out := make([][2]int, 0, len(pairs))
	for pair := range pairs {
		out = append(out, pair)
	}
	slices.SortFunc(out, func(a, b [2]int) int {
		if a[0] != b[0] {
			return a[0] - b[0]
		}
		return a[1] - b[1]
	})
	return out
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
	cs := a.MatchesCase() || b.MatchesCase()
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
