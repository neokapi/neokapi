// Package rulesview reads a project's operation log as a person sees it: the
// digest of what kapi learned about how the project writes, and the questions
// a person answers about each suggestion waiting for a decision.
//
// The package holds the model and no input or output. Its callers load the
// recipe, open the log and read the project's source, then pass plain values
// in: the records as the reader sees them (SeenBy), a function that says where
// a scope applies, and the blocks to count uses in. The desktop, the command
// line and the platform server read the same digest and ask the same
// questions through it.
package rulesview

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/workspace"
)

// Digest themes, in the order a digest lists them.
const (
	// ThemeNames holds term rules about how a name is spelled: the preferred
	// form and the forms to avoid differ only in case, spacing or punctuation
	// ("Quickcast", not "Quick cast").
	ThemeNames = "names"
	// ThemeWords holds term rules that replace one word with another, or say
	// a word to avoid.
	ThemeWords = "words"
	// ThemeWriting holds what was noticed about how the project writes, in
	// prose: address, tone.
	ThemeWriting = "writing"
	// ThemeMemory holds wording pairs for the content memory.
	ThemeMemory = "memory"
)

// themes is every theme with the title a text digest prints for it.
var themes = []struct{ id, title string }{
	{ThemeNames, "Names and spellings"},
	{ThemeWords, "Words to avoid"},
	{ThemeWriting, "How the project writes"},
	{ThemeMemory, "Wording in other languages"},
}

// earlierEstablished caps the established rules a digest repeats from before
// the marker, so "Earlier" stays a glance back rather than a history.
const earlierEstablished = 10

// week is the span "new this week" counts over.
const week = 7 * 24 * time.Hour

// WidenToWorkspace is the widening that puts a rule in force in every project
// of the workspace.
const WidenToWorkspace = "workspace"

// Digest is what kapi learned about how one project writes.
type Digest struct {
	// Project is the project's workspace key, and ProjectName what a person
	// calls it. BuildDigest leaves ProjectName empty for the caller to fill.
	Project     string `json:"project"`
	ProjectName string `json:"project_name,omitempty"`
	// Since is when the person last looked: an item recorded after it is new.
	// Zero when nobody has looked.
	Since time.Time `json:"since,omitzero"`
	// Conflicts need a person: two rules disagree, or the evidence turned
	// against a rule.
	Conflicts []Conflict `json:"conflicts"`
	// Established are the rules that came into force, newest first: every one
	// since the marker, then a few from before it.
	Established []Item `json:"established"`
	// Suggested are the suggestions still advising, grouped by theme.
	Suggested []Theme `json:"suggested"`
	// Drift is content moving away from an established rule.
	Drift []Drift `json:"drift"`
	// Numbers is the project in numbers.
	Numbers Numbers `json:"numbers"`
}

// Item is one rule, suggestion or fact as the digest shows it.
type Item struct {
	// ID names the operation every action on the item passes back, and Short
	// is the form a person types.
	ID    string `json:"id"`
	Short string `json:"short"`
	// Status is the operation's folded status.
	Status contextop.Status `json:"status"`
	// Theme is which suggestion theme the item belongs to.
	Theme string `json:"theme"`
	// Sentence is the rule as a sentence: "Write Quickcast, not Quick cast or
	// QuickCast."
	Sentence string `json:"sentence"`
	// Subject is the rule itself, for a surface that renders its parts.
	Subject contextop.Subject `json:"subject"`
	// Quote is the evidence the item was seen in, with the file it links to.
	Quote *contextop.Evidence `json:"quote,omitempty"`
	// Collection is the collection the evidence sits in, empty when the
	// project declares none or the recipe is not at hand.
	Collection string `json:"collection,omitempty"`
	// Standing is the evidence as plain counts: "seen in 3 sessions · 14 of
	// 15 uses in docs/ · merged in #412".
	Standing string `json:"standing,omitempty"`
	// Usage is the project's own words back to it, from the latest usage
	// count, for a rule a check has counted.
	Usage *Usage `json:"usage,omitempty"`
	// NoticedBy is who recorded it.
	NoticedBy contextop.Actor `json:"noticed_by"`
	// At is when it was recorded.
	At time.Time `json:"at"`
	// New reports an item recorded (or, for an established rule, established)
	// after the marker.
	New bool `json:"new"`
	// Scope is how far the rule answers, as the log prints it.
	Scope string `json:"scope"`
	// Keepable reports a suggestion a person can keep as it stands. A
	// contested suggestion with a rival waits until the rival is dropped.
	Keepable bool `json:"keepable"`
	// Droppable reports a suggestion or a rule a person can set aside. A rule
	// in force is taken back out of the stores as it is dropped.
	Droppable bool `json:"droppable"`
	// Widenable reports a rule in force a person can apply more widely.
	Widenable bool `json:"widenable"`
	// WidenTo are the widenings open to a widenable rule: "workspace", and
	// each axis of its point (WidenOptions).
	WidenTo []string `json:"widen_to,omitempty"`
	// EstablishedAt and How say, for an established rule, when it came into
	// force and on what: "merged in #412", "your correction in billing.md",
	// "kept by you".
	EstablishedAt time.Time `json:"established_at,omitzero"`
	How           []string  `json:"how,omitempty"`
}

// Established reports an item that is a rule in force.
func (it Item) Established() bool { return !it.EstablishedAt.IsZero() }

// Usage is how often the project's content writes a rule's forms.
type Usage struct {
	// Preferred is the form the rule says to write, and PreferredCount its
	// uses.
	Preferred      string `json:"preferred"`
	PreferredCount int    `json:"preferred_count"`
	// Rejected are the forms the rule avoids, and RejectedCount their uses
	// together.
	Rejected      []string `json:"rejected"`
	RejectedCount int      `json:"rejected_count"`
	// Within is the part of the project counted, as a path prefix; empty for
	// the whole project.
	Within string `json:"within,omitempty"`
	// Line says it in a sentence: `docs/ says "studio" 41 times and
	// "business" twice`.
	Line string `json:"line"`
}

// Conflict is a disagreement a person settles: two or more rules saying
// different things about one word, or one rule the evidence turned against.
type Conflict struct {
	// Sides are the rules that disagree. Choosing one keeps it and drops the
	// others.
	Sides []Item `json:"sides"`
	// ByEvidence reports a rule contested by evidence alone, with no rival
	// rule: keeping it again is the choice, and so is dropping it.
	ByEvidence bool `json:"by_evidence"`
	// Reason says what the disagreement is, in a sentence.
	Reason string `json:"reason"`
}

// Theme is one theme's suggestions, split by collection when they sit in more
// than one.
type Theme struct {
	// Theme is the theme's id (ThemeNames and its neighbours), and Title what
	// a text digest prints.
	Theme  string  `json:"theme"`
	Title  string  `json:"title"`
	Groups []Group `json:"groups"`
}

// Group is the suggestions of one theme in one collection. A person may keep
// a whole group at once.
type Group struct {
	// Collection is empty for suggestions whose evidence sits in no
	// collection, and for a theme whose suggestions all sit in one.
	Collection string `json:"collection,omitempty"`
	Items      []Item `json:"items"`
}

// Drift is content moving away from an established rule.
type Drift struct {
	Rule Item `json:"rule"`
	// Line says what moved: `docs/ says "Quick cast" 3 times and "Quickcast"
	// 11 times`.
	Line string `json:"line"`
	// Rejected is how many times the content now writes a rejected form, and
	// Before the count when the rule came into force.
	Rejected int `json:"rejected"`
	Before   int `json:"before"`
}

// Numbers is the project in numbers.
type Numbers struct {
	// Rules counts the rules in force.
	Rules int `json:"rules"`
	// NewThisWeek counts the rules that came into force in the last seven
	// days.
	NewThisWeek int `json:"new_this_week"`
	// Suggested counts the suggestions advising, and Conflicts the
	// disagreements.
	Suggested int `json:"suggested"`
	Conflicts int `json:"conflicts"`
	// New counts the items recorded since the marker, across every section.
	New int `json:"new"`
}

// BuildDigest assembles one project's digest from a folded log, newest first
// as contextop.Ledger.Records returns it. since is the marker, zero to count
// everything as new; now is the instant "new this week" counts back from.
// collectionFor names the collection a path sits in, nil when the project
// declares none. It reads nothing but its arguments.
func BuildDigest(records []contextop.Record, key workspace.ProjectKey, since, now time.Time, collectionFor func(string) string) Digest {
	out := Digest{
		Project:     string(key),
		Since:       since,
		Conflicts:   []Conflict{},
		Established: []Item{},
		Suggested:   []Theme{},
		Drift:       []Drift{},
	}
	byID := make(map[string]contextop.Record, len(records))
	for _, r := range records {
		byID[r.ID] = r
	}
	acts := Acts(records, byID)
	isNew := func(at time.Time) bool { return since.IsZero() || at.After(since) }
	item := func(r contextop.Record) Item {
		it := NewItem(r, collectionFor)
		it.New = isNew(r.At)
		return it
	}

	conflicted := map[string]bool{}
	themed := map[string]map[string][]Item{}
	var established []Item
	for _, r := range records {
		if r.Project != key || !r.Kind.Bears() || !r.Status.Answers() {
			continue
		}
		switch {
		case r.Kind == contextop.KindImport:
			// A file read in is a record of where terms, a voice or wording
			// came from, not a rule a person decided: it stays in the log and
			// out of the digest, where "established" means a rule in force.
			continue
		case r.Status == contextop.StatusContested:
			if conflicted[r.ID] {
				continue
			}
			out.Conflicts = append(out.Conflicts, conflict(r, byID, conflicted, item))
		case r.Established:
			it := item(r)
			it.EstablishedAt, it.How = Establishment(r, acts[r.ID], byID)
			it.New = isNew(it.EstablishedAt)
			out.Numbers.Rules++
			if now.Sub(it.EstablishedAt) < week {
				out.Numbers.NewThisWeek++
			}
			if it.Usage == nil {
				it.Usage = latestUsage(r, acts[r.ID])
			}
			established = append(established, it)
			if drift, ok := drifted(r, acts[r.ID], it); ok {
				out.Drift = append(out.Drift, drift)
			}
		case r.Subject.Kind != contextop.SubjectNone:
			it := item(r)
			out.Numbers.Suggested++
			if themed[it.Theme] == nil {
				themed[it.Theme] = map[string][]Item{}
			}
			themed[it.Theme][it.Collection] = append(themed[it.Theme][it.Collection], it)
		}
	}
	out.Numbers.Conflicts = len(out.Conflicts)

	sort.SliceStable(established, func(i, j int) bool { return established[i].EstablishedAt.After(established[j].EstablishedAt) })
	earlier := 0
	for _, it := range established {
		if !it.New {
			if earlier == earlierEstablished {
				continue
			}
			earlier++
		}
		out.Established = append(out.Established, it)
	}

	for _, theme := range themes {
		byCollection := themed[theme.id]
		if len(byCollection) == 0 {
			continue
		}
		t := Theme{Theme: theme.id, Title: theme.title, Groups: []Group{}}
		collections := slices.Sorted(maps.Keys(byCollection))
		if len(collections) == 1 {
			// One collection says nothing a reader needs told.
			t.Groups = append(t.Groups, Group{Items: byCollection[collections[0]]})
		} else {
			for _, c := range collections {
				t.Groups = append(t.Groups, Group{Collection: c, Items: byCollection[c]})
			}
		}
		out.Suggested = append(out.Suggested, t)
	}

	for _, c := range out.Conflicts {
		for _, side := range c.Sides {
			if side.New {
				out.Numbers.New++
			}
		}
	}
	for _, it := range out.Established {
		if it.New {
			out.Numbers.New++
		}
	}
	for _, t := range out.Suggested {
		for _, g := range t.Groups {
			for _, it := range g.Items {
				if it.New {
					out.Numbers.New++
				}
			}
		}
	}
	return out
}

// Acts indexes, per subject-bearing operation, the operations that act on it,
// oldest first. byID holds every record by its id.
func Acts(records []contextop.Record, byID map[string]contextop.Record) map[string][]contextop.Record {
	out := map[string][]contextop.Record{}
	for _, act := range slices.Backward(records) {
		if act.Kind.Bears() || act.Target == "" {
			continue
		}
		if b, ok := bearer(byID, act); ok {
			out[b.ID] = append(out[b.ID], act)
		}
	}
	return out
}

// bearer follows an act's target to the operation that carries the subject.
func bearer(byID map[string]contextop.Record, r contextop.Record) (contextop.Record, bool) {
	for range 16 {
		if r.Kind.Bears() {
			return r, true
		}
		next, ok := byID[r.Target]
		if !ok {
			return contextop.Record{}, false
		}
		r = next
	}
	return contextop.Record{}, false
}

// NewItem renders one subject-bearing operation. collectionFor names the
// collection a path sits in, nil when the project declares none. New is left
// for the caller, which holds the marker.
func NewItem(r contextop.Record, collectionFor func(string) string) Item {
	it := Item{
		ID:        r.ID,
		Short:     contextop.ShortID(r.ID),
		Status:    r.Status,
		Theme:     ThemeOf(r.Subject),
		Sentence:  Sentence(r.Subject),
		Subject:   r.Subject,
		Standing:  r.Standing.Describe(),
		NoticedBy: r.Actor,
		At:        r.At,
		Scope:     r.Scope.Describe(),
	}
	if it.Sentence == "" && r.Correction != nil {
		// A correction that states no rule of its own is the change itself.
		it.Sentence = fmt.Sprintf("Changed %q to %q.", r.Correction.From, r.Correction.To)
	}
	for i := range r.Evidence {
		e := r.Evidence[i]
		if it.Quote == nil || (it.Quote.Quote == "" && e.Quote != "") {
			it.Quote = &e
		}
	}
	if it.Quote != nil && it.Quote.Path != "" && collectionFor != nil {
		it.Collection = collectionFor(it.Quote.Path)
	}
	if rule, ok := r.Rule(); ok && r.Standing != nil && r.Standing.Uses != nil && r.Standing.Uses.Total > 0 {
		u := r.Standing.Uses
		it.Usage = NewUsage(rule.Replacement, append([]string{rule.Term}, rule.Forms...), u.Preferred, u.Total-u.Preferred, u.Within)
	}
	switch {
	case r.Established:
		it.Droppable = true
		if r.Status == contextop.StatusEstablished {
			it.WidenTo = WidenOptions(r.Subject.Kind, r.Scope)
			it.Widenable = len(it.WidenTo) > 0
		}
	case r.Subject.Kind == contextop.SubjectNote:
		it.Droppable = true
	default:
		it.Droppable = true
		it.Keepable = r.Status == contextop.StatusSuggested || r.ContestedByEvidence()
	}
	return it
}

// WidenOptions are the widenings open to an established subject at scope: the
// whole workspace, and each axis its point is specific about. A rule already
// answering workspace-wide has nowhere further to go, and a note holds within
// its project at the widest, so neither is offered the workspace.
func WidenOptions(subject contextop.SubjectKind, scope contextop.Scope) []string {
	out := []string{}
	if scope.Level != contextop.LevelWorkspace && subject != contextop.SubjectNote {
		out = append(out, WidenToWorkspace)
	}
	axes := make([]string, 0, len(scope.Coordinates))
	for axis := range scope.Coordinates {
		axes = append(axes, axis)
	}
	slices.Sort(axes)
	return append(out, axes...)
}

// NewUsage says how the content writes a rule's forms.
func NewUsage(preferred string, rejected []string, preferredCount, rejectedCount int, within string) *Usage {
	where := "The project"
	if within != "" {
		where = within
	}
	line := fmt.Sprintf("%s says %q %s", where, preferred, Times(preferredCount))
	if rejectedCount > 0 {
		quoted := make([]string, len(rejected))
		for i, f := range rejected {
			quoted[i] = strconv.Quote(f)
		}
		line += fmt.Sprintf(" and %s %s", OrList(quoted), Times(rejectedCount))
	}
	return &Usage{
		Preferred:      preferred,
		PreferredCount: preferredCount,
		Rejected:       rejected,
		RejectedCount:  rejectedCount,
		Within:         within,
		Line:           line,
	}
}

// Times renders a count of uses the way a sentence says it.
func Times(n int) string {
	switch n {
	case 1:
		return "once"
	case 2:
		return "twice"
	}
	return strconv.Itoa(n) + " times"
}

// conflict gathers one contested record and the rules on its other side,
// marking every one it takes so the disagreement is listed once.
func conflict(r contextop.Record, byID map[string]contextop.Record, taken map[string]bool, item func(contextop.Record) Item) Conflict {
	// Every side of a conflict can be chosen: choosing keeps it and drops the
	// others, and keeping a rule the evidence turned against is the choice.
	side := func(r contextop.Record) Item {
		it := item(r)
		it.Keepable, it.Droppable = true, !r.Established
		return it
	}
	c := Conflict{Sides: []Item{}}
	taken[r.ID] = true
	c.Sides = append(c.Sides, side(r))
	// A correction that suggests the opposite rule is evidence against this
	// one and a rival to it at once: the person chooses between the two rules,
	// so it is listed as a side rather than as the evidence alone.
	if r.ContestedByEvidence() && !contestedByRival(r, byID, taken) {
		c.ByEvidence = true
		c.Reason = contestedByEvidenceReason(r, byID)
		return c
	}
	for _, id := range r.ContestedBy {
		other, ok := byID[id]
		if !ok || taken[id] {
			continue
		}
		if !other.Kind.Bears() {
			// A correction or a count against the rule, rather than a rival.
			continue
		}
		taken[id] = true
		c.Sides = append(c.Sides, side(other))
	}
	if len(c.Sides) > 1 {
		c.Reason = "These rules say different things about the same word. Choose one, and the others are set aside."
	} else {
		c.Reason = contestedByEvidenceReason(r, byID)
		c.ByEvidence = true
	}
	return c
}

// contestedByRival reports a record contested by a rule of its own that the
// digest has not yet listed: a suggestion or a correction stating a rule.
func contestedByRival(r contextop.Record, byID map[string]contextop.Record, taken map[string]bool) bool {
	for _, id := range r.ContestedBy {
		other, ok := byID[id]
		if ok && !taken[id] && other.Kind.Bears() && other.Subject.Kind == contextop.SubjectTerm {
			return true
		}
	}
	return false
}

// contestedByEvidenceReason says what turned the evidence against a rule.
func contestedByEvidenceReason(r contextop.Record, byID map[string]contextop.Record) string {
	for _, id := range r.ContestedBy {
		against, ok := byID[id]
		if !ok {
			continue
		}
		switch {
		case against.Kind == contextop.KindCorrect && against.Correction != nil:
			where := ""
			if len(against.Evidence) > 0 && against.Evidence[0].Path != "" {
				where = " in " + against.Evidence[0].Path
			}
			return fmt.Sprintf("%s changed %q back to %q%s.", ActorPhrase(against.Actor, true), against.Correction.From, against.Correction.To, where)
		case against.Kind == contextop.KindSignal && against.Signal != nil && against.Signal.Source == contextop.SignalUsage:
			return fmt.Sprintf("The content moved to a form this rule avoids: %d uses now.", against.Signal.Rejected)
		case against.Kind == contextop.KindWithdraw:
			return "Its author withdrew it after a person backed it."
		}
	}
	return "The evidence no longer agrees on this rule."
}

// Establishment reads when a rule came into force and on what, from the
// operations that act on it (Acts) and every record by its id.
func Establishment(r contextop.Record, acts []contextop.Record, byID map[string]contextop.Record) (time.Time, []string) {
	switch r.Kind {
	case contextop.KindImport:
		how := "imported"
		if r.Note != "" {
			how += " (" + r.Note + ")"
		}
		return r.At, []string{how}
	case contextop.KindEdit:
		return r.At, []string{"written by " + ActorPhrase(r.Actor, false)}
	}
	var at time.Time
	var how []string
	for _, act := range acts {
		switch act.Kind {
		case contextop.KindKeep:
			at, how = act.At, []string{"kept by " + ActorPhrase(act.Actor, false)}
		case contextop.KindEstablish:
			if at.IsZero() || act.At.After(at) {
				at, how = act.At, becauseLines(act.Because, byID)
			}
		}
	}
	if at.IsZero() {
		at = r.At
	}
	return at, how
}

// EstablishedBy reads who put a rule in force, from the same operations
// Establishment reads: the person who kept it, the actor that settled it on
// the evidence, or the one that imported or wrote it.
func EstablishedBy(r contextop.Record, acts []contextop.Record) contextop.Actor {
	if r.Kind == contextop.KindImport || r.Kind == contextop.KindEdit {
		return r.Actor
	}
	who := r.Actor
	var at time.Time
	for _, act := range acts {
		switch act.Kind {
		case contextop.KindKeep:
			at, who = act.At, act.Actor
		case contextop.KindEstablish:
			if at.IsZero() || act.At.After(at) {
				at, who = act.At, act.Actor
			}
		}
	}
	return who
}

// becauseLines says what an establish operation rested on.
func becauseLines(because []string, byID map[string]contextop.Record) []string {
	var out []string
	for _, id := range because {
		ev, ok := byID[id]
		if !ok {
			continue
		}
		switch {
		case ev.Kind == contextop.KindSignal && ev.Signal != nil && ev.Signal.Source == contextop.SignalMerge:
			if ev.Signal.PR > 0 {
				out = append(out, "merged in #"+strconv.Itoa(ev.Signal.PR))
			} else if ev.Signal.Commit != "" {
				out = append(out, "merged in "+shortCommit(ev.Signal.Commit))
			} else {
				out = append(out, "merged")
			}
		case ev.Kind == contextop.KindCorrect:
			line := ActorPhrase(ev.Actor, true) + " correction"
			if ActorPhrase(ev.Actor, true) == "you" {
				line = "your correction"
			}
			if len(ev.Evidence) > 0 && ev.Evidence[0].Path != "" {
				line += " in " + ev.Evidence[0].Path
			}
			out = append(out, line)
		case ev.Kind == contextop.KindKeep:
			out = append(out, "kept by "+ActorPhrase(ev.Actor, false))
		}
	}
	return out
}

// shortCommit abbreviates a commit to seven characters.
func shortCommit(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

// latestUsage is the latest usage count recorded against a rule, nil when no
// check has counted it.
func latestUsage(r contextop.Record, acts []contextop.Record) *Usage {
	rule, ok := r.Rule()
	if !ok || rule.Replacement == "" {
		return nil
	}
	var latest *contextop.Signal
	for _, act := range acts {
		if act.Kind == contextop.KindSignal && act.Signal != nil && act.Signal.Source == contextop.SignalUsage && act.Status.Answers() {
			latest = act.Signal
		}
	}
	if latest == nil || latest.Preferred+latest.Rejected == 0 {
		return nil
	}
	return NewUsage(rule.Replacement, append([]string{rule.Term}, rule.Forms...), latest.Preferred, latest.Rejected, latest.Within)
}

// drifted reports an established rule whose content moved to a form it
// avoids: the latest usage count writes a rejected form more often than the
// baseline. The baseline is the last count taken when the rule came into force,
// or, for a rule nobody had counted by then, the first count after it, so
// rejected uses the content already held when the rule was kept are not drift.
func drifted(r contextop.Record, acts []contextop.Record, it Item) (Drift, bool) {
	rule, ok := r.Rule()
	if !ok || rule.Replacement == "" {
		return Drift{}, false
	}
	var base, latest *contextop.Signal
	for _, act := range acts {
		if act.Kind != contextop.KindSignal || act.Signal == nil || act.Signal.Source != contextop.SignalUsage || !act.Status.Answers() {
			continue
		}
		switch {
		case !act.At.After(it.EstablishedAt):
			base = act.Signal
		case base == nil:
			base = act.Signal
		default:
			latest = act.Signal
		}
	}
	if base == nil || latest == nil || latest.Within != base.Within || latest.Rejected <= base.Rejected {
		return Drift{}, false
	}
	usage := NewUsage(rule.Replacement, append([]string{rule.Term}, rule.Forms...), latest.Preferred, latest.Rejected, latest.Within)
	it.Usage = usage
	return Drift{Rule: it, Line: usage.Line, Rejected: latest.Rejected, Before: base.Rejected}, true
}
