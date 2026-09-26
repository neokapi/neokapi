package contextop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Settling
//
// A suggestion becomes an established rule by evidence, and what counts as
// evidence is read from the log alone, so every machine holding the same log
// reaches the same answer whatever order the log arrived in.
//
// A suggestion is established when at least one signal comes from a person and
// nothing open contradicts it. The person signals are a keep (which
// establishes at once, and is handled by the fold like any person's decision),
// a correction toward the rule, and the rule reaching the default branch (a
// merge signal). Signals that only add to a suggestion's standing are another
// session recording the same rule and the project's content writing the
// preferred form (a usage signal). Against a suggestion are a correction away
// from it, its withdrawal, and the content moving to a rejected form after it
// was recorded; any of them while a person signal exists leaves the
// suggestion contested for a person to decide. Time alone settles nothing.
//
// Suggestions that state the same rule (the same form to use, sharing a form
// to avoid, at points that meet) settle together: the evidence for one is the
// evidence for all of them, and each is established by an operation of its
// own.

// SignalSource names where a signal's evidence came from.
type SignalSource string

const (
	// SignalMerge is a change that reached the project's default branch: a
	// merged pull request or a push. It is a person's signal, because a person
	// merged it.
	SignalMerge SignalSource = "merge"
	// SignalUsage counts how often the project's content writes the preferred
	// form and the rejected forms. It adds to a suggestion's standing, and
	// content moving to a rejected form counts against it.
	SignalUsage SignalSource = "usage"
)

// Signal is the evidence a signal operation carries.
type Signal struct {
	// Source is where the evidence came from.
	Source SignalSource `json:"source"`
	// Commit is the commit that reached the default branch, for a merge.
	Commit string `json:"commit,omitempty"`
	// PR is the pull request that merged it, where known.
	PR int `json:"pr,omitempty"`
	// Merger is who merged it, where known.
	Merger string `json:"merger,omitempty"`
	// Preferred counts the uses of the form to write: added lines for a merge,
	// uses in content for a usage count.
	Preferred int `json:"preferred,omitempty"`
	// Rejected counts the uses of the forms to avoid: removed lines for a
	// merge, uses in content for a usage count.
	Rejected int `json:"rejected,omitempty"`
	// Within is the part of the project a usage count covers, as a path
	// prefix: "docs/". Empty covers the whole project.
	Within string `json:"within,omitempty"`
}

// Standing is the evidence for and against a suggestion, as plain counts. It
// is never collapsed into a score: a reader sees what the rule rests on.
type Standing struct {
	// Sessions counts the distinct sessions, or people, that recorded the rule.
	Sessions int `json:"sessions,omitempty"`
	// Corrections counts the corrections toward the rule.
	Corrections int `json:"corrections,omitempty"`
	// Merges names the changes that reached the default branch with the rule:
	// "#412" for a pull request, a short commit otherwise.
	Merges []string `json:"merges,omitempty"`
	// Uses is the latest count of the rule's forms in the project's content.
	Uses *Uses `json:"uses,omitempty"`
	// Against counts the signals against the rule, and AgainstBy names them.
	Against   int      `json:"against,omitempty"`
	AgainstBy []string `json:"against_by,omitempty"`
}

// ContestedByEvidence reports that a record is contested by signals against it
// alone, with no rival rule on the other side. A person keeping it is then the
// decision, and nothing else needs dropping first.
func (r Record) ContestedByEvidence() bool {
	if r.Status != StatusContested || r.Standing == nil || len(r.ContestedBy) == 0 {
		return false
	}
	for _, id := range r.ContestedBy {
		if !slices.Contains(r.Standing.AgainstBy, id) {
			return false
		}
	}
	return true
}

// Uses is how often the content writes the preferred form, out of every use of
// the rule's forms.
type Uses struct {
	Preferred int    `json:"preferred"`
	Total     int    `json:"total"`
	Within    string `json:"within,omitempty"`
}

// Describe renders standing as the counts a log line shows:
// `seen in 3 sessions · 14 of 15 uses in docs/ · merged in #412`.
func (s *Standing) Describe() string {
	if s == nil {
		return ""
	}
	var parts []string
	if s.Sessions > 1 {
		parts = append(parts, fmt.Sprintf("seen in %d sessions", s.Sessions))
	}
	if s.Corrections > 0 {
		parts = append(parts, plural(s.Corrections, "correction", "corrections")+" toward it")
	}
	if s.Uses != nil && s.Uses.Total > 0 {
		use := fmt.Sprintf("%d of %s", s.Uses.Preferred, plural(s.Uses.Total, "use", "uses"))
		if s.Uses.Within != "" {
			use += " in " + s.Uses.Within
		}
		parts = append(parts, use)
	}
	if len(s.Merges) > 0 {
		parts = append(parts, "merged in "+strings.Join(s.Merges, ", "))
	}
	if s.Against > 0 {
		parts = append(parts, fmt.Sprintf("%d against", s.Against))
	}
	return strings.Join(parts, " · ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// settleFacts is what the fold's first pass learned about each record that
// settling reads.
type settleFacts struct {
	// kept marks a record a person established: kept, imported or written.
	kept []bool
	// establishedBy lists the establish operations that named each record.
	establishedBy [][]string
	// withdrawnBy is the withdrawal that set each record aside, if any.
	withdrawnBy []string
	// droppedAt is the position of the drop that set each record aside.
	droppedAt []int64
	// signals lists, per record, the positions of the signal operations
	// naming it.
	signals [][]int
}

func newSettleFacts(n int) *settleFacts {
	return &settleFacts{
		kept:          make([]bool, n),
		establishedBy: make([][]string, n),
		withdrawnBy:   make([]string, n),
		droppedAt:     make([]int64, n),
		signals:       make([][]int, n),
	}
}

// settle decides, in place, the status of every suggestion settling
// considers. It runs after the fold has applied every person's decision and
// before contest marks the disagreements between rules.
func settle(records []Record, f *settleFacts) {
	// Candidates are the term rules no person has decided about: suggestions,
	// and the ones their author withdrew.
	var candidates, dropped, live []int
	for i, r := range records {
		rule, ok := r.Rule()
		if !r.Kind.Bears() || !ok || rule.Replacement == "" {
			continue
		}
		if r.Status.Answers() {
			live = append(live, i)
		}
		if f.kept[i] {
			continue
		}
		switch r.Status {
		case StatusSuggested, StatusWithdrawn:
			candidates = append(candidates, i)
		case StatusDropped:
			dropped = append(dropped, i)
		}
	}
	// A rule that says something else about the same word, at a point that
	// meets, is open against every suggestion it disagrees with.
	rivals := map[int][]string{}
	for _, pair := range candidatePairs(records, live) {
		a, b := live[pair[0]], live[pair[1]]
		ra, _ := records[a].Rule()
		rb, _ := records[b].Rule()
		if meet(records[a], records[b]) && disagree(ra, rb) {
			rivals[a] = append(rivals[a], records[b].ID)
			rivals[b] = append(rivals[b], records[a].ID)
		}
	}
	for _, group := range agreeing(records, candidates) {
		settleGroup(records, f, group, dropped, rivals)
	}
}

// agreeing groups candidates that state the same rule at points that meet.
func agreeing(records []Record, candidates []int) [][]int {
	parent := make(map[int]int, len(candidates))
	pairs := candidatePairs(records, candidates)
	root := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for _, i := range candidates {
		parent[i] = i
	}
	for _, pair := range pairs {
		i, j := candidates[pair[0]], candidates[pair[1]]
		if same(records[i], records[j]) {
			parent[root(j)] = root(i)
		}
	}
	groups := map[int][]int{}
	var roots []int
	for _, i := range candidates {
		r := root(i)
		if _, ok := groups[r]; !ok {
			roots = append(roots, r)
		}
		groups[r] = append(groups[r], i)
	}
	out := make([][]int, 0, len(roots))
	for _, r := range roots {
		out = append(out, groups[r])
	}
	return out
}

// same reports whether two records state one rule: the same form to use, a
// form to avoid in common, at points that meet.
func same(a, b Record) bool {
	ra, _ := a.Rule()
	rb, _ := b.Rule()
	if !meet(a, b) {
		return false
	}
	cs := ra.MatchesCase() || rb.MatchesCase()
	if formKey(ra.Replacement, cs) != formKey(rb.Replacement, cs) {
		return false
	}
	avoidB := avoided(rb, cs)
	for _, form := range avoided(ra, cs) {
		if slices.Contains(avoidB, form) {
			return true
		}
	}
	return false
}

// settleGroup settles one group of suggestions stating the same rule.
func settleGroup(records []Record, f *settleFacts, group, dropped []int, rivals map[int][]string) {
	first := records[group[0]]
	rule, _ := first.Rule()
	cs := rule.MatchesCase()

	// A person dropping the rule is the person's answer: what was recorded
	// before the drop, and the evidence gathered before it, no longer counts.
	var cutoff int64
	for _, d := range dropped {
		if same(records[d], first) && f.droppedAt[d] > cutoff {
			cutoff = f.droppedAt[d]
		}
	}
	var members []int
	for _, i := range group {
		if records[i].Seq > cutoff {
			members = append(members, i)
		}
	}
	if len(members) == 0 {
		return
	}
	member := make(map[string]bool, len(members))
	for _, i := range members {
		member[records[i].ID] = true
	}

	standing := &Standing{}
	var person, against []string
	sessions := map[string]bool{}
	for _, i := range members {
		a := records[i].Actor
		sessions[string(a.Kind)+"\x00"+a.Name+"\x00"+a.Session] = true
		if f.withdrawnBy[i] != "" {
			against = append(against, f.withdrawnBy[i])
		}
	}
	standing.Sessions = len(sessions)

	// Corrections toward the rule and away from it, recorded by anyone: a
	// correction is what a person changed, whoever wrote it down.
	for _, c := range records {
		if c.Kind != KindCorrect || c.Correction == nil || !c.Status.Answers() || c.Seq <= cutoff || !meet(c, first) {
			continue
		}
		from, to := formKey(c.Correction.From, cs), formKey(c.Correction.To, cs)
		use := formKey(rule.Replacement, cs)
		switch {
		case to == use && slices.Contains(avoidedAll(records, members, cs), from):
			// A group of one does not settle on the correction it was drawn
			// from: that is the suggestion itself, not evidence for it.
			if len(members) == 1 && member[c.ID] {
				continue
			}
			person = append(person, c.ID)
			standing.Corrections++
		case from == use && slices.Contains(avoidedAll(records, members, cs), to):
			against = append(against, c.ID)
		}
	}

	// Merges and usage counts naming a member.
	var usage []Record
	for _, i := range members {
		for _, s := range f.signals[i] {
			sig := records[s]
			if sig.Signal == nil || sig.Seq <= cutoff || !sig.Status.Answers() {
				continue
			}
			switch sig.Signal.Source {
			case SignalMerge:
				if sig.Signal.Preferred == 0 && sig.Signal.Rejected == 0 {
					continue
				}
				if !slices.Contains(person, sig.ID) {
					person = append(person, sig.ID)
					standing.Merges = append(standing.Merges, mergeLabel(*sig.Signal))
				}
			case SignalUsage:
				usage = append(usage, sig)
			}
		}
	}
	if len(usage) > 0 {
		slices.SortFunc(usage, func(a, b Record) int { return int(a.Seq - b.Seq) })
		latest := usage[len(usage)-1].Signal
		standing.Uses = &Uses{Preferred: latest.Preferred, Total: latest.Preferred + latest.Rejected, Within: latest.Within}
		// The content moving to a rejected form after the suggestion was
		// recorded counts against it.
		base := usage[0].Signal
		for _, u := range usage[1:] {
			if u.Signal.Within == base.Within && u.Signal.Rejected > base.Rejected {
				against = append(against, u.ID)
			}
		}
	}
	slices.Sort(person)
	against = dedupe(against)
	var open []string
	for _, i := range members {
		open = append(open, rivals[i]...)
	}
	open = dedupe(append(slices.Clone(against), open...))
	standing.Against, standing.AgainstBy = len(against), against
	merges := dedupe(standing.Merges)
	standing.Merges = merges

	for _, i := range members {
		r := &records[i]
		r.Standing = standing
		switch {
		case len(person) == 0:
			// Nothing a person did backs it: whatever an establish operation
			// once rested on no longer holds, and it advises.
		case len(open) > 0:
			r.Status = StatusContested
			for _, id := range open {
				if !slices.Contains(r.ContestedBy, id) {
					r.ContestedBy = append(r.ContestedBy, id)
				}
			}
			r.Established = len(f.establishedBy[i]) > 0
		case len(f.establishedBy[i]) > 0:
			r.Status = StatusEstablished
			r.Established = true
		default:
			r.pending = person
		}
	}
}

// avoidedAll lists every form the members of a group say to avoid.
func avoidedAll(records []Record, members []int, cs bool) []string {
	var out []string
	for _, i := range members {
		rule, _ := records[i].Rule()
		for _, form := range avoided(rule, cs) {
			if !slices.Contains(out, form) {
				out = append(out, form)
			}
		}
	}
	return out
}

// mergeLabel names a merge the way a standing line shows it.
func mergeLabel(s Signal) string {
	if s.PR > 0 {
		return "#" + strconv.Itoa(s.PR)
	}
	if len(s.Commit) > 7 {
		return s.Commit[:7]
	}
	return s.Commit
}

func dedupe(in []string) []string {
	var out []string
	for _, s := range in {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// address is the content address of an operation whose meaning is its
// evidence: an establishment is the suggestion and the signals it rested on,
// and a signal is the suggestion and what was seen. Recording either twice, on
// one machine or on two, is one operation. Every other kind is addressed by
// its id alone.
func address(r Record) string {
	var parts []string
	switch r.Kind {
	case KindEstablish:
		parts = append([]string{string(r.Kind), r.Target}, r.Because...)
	case KindSignal:
		if r.Signal == nil {
			return ""
		}
		s := r.Signal
		parts = []string{string(r.Kind), r.Target, string(s.Source), s.Commit, s.Within,
			strconv.Itoa(s.Preferred), strconv.Itoa(s.Rejected)}
	default:
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return OpKindPrefix + string(r.Kind) + ":" + hex.EncodeToString(sum[:16])
}

// SettleActor is the actor settling records under: a tool acting on evidence
// the log already holds.
var SettleActor = Actor{Kind: ActorTool, Name: "settle"}

// Settle records an establish operation for every suggestion the log now
// supports establishing and no establish operation has recorded yet, and
// returns the suggestions it established, as the log now folds them.
//
// It is idempotent: an establishment's id is derived from its evidence, so
// settling the same log twice, or on two machines, records one operation.
func (l *Ledger) Settle(ctx context.Context, f Filter) ([]Record, error) {
	all, err := l.fold(ctx)
	if err != nil {
		return nil, err
	}
	var targets []Record
	for _, r := range all {
		if len(r.pending) > 0 && f.matches(r) {
			targets = append(targets, r)
		}
	}
	var out []Record
	for _, target := range targets {
		if _, err := l.Append(ctx, Record{
			Actor:   SettleActor,
			Kind:    KindEstablish,
			Target:  target.ID,
			Project: target.Project,
			Scope:   target.Scope,
			Because: target.pending,
		}); err != nil {
			return out, err
		}
		settled, err := l.Get(ctx, target.ID)
		if err != nil {
			return out, err
		}
		out = append(out, settled)
	}
	return out, nil
}
