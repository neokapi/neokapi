// The context digest: what kapi learned about how a project writes since a
// person last looked.
//
// Review is discovery, never a gate. A suggestion advises from the moment it
// is recorded, and one nobody answers keeps advising, so the digest is news
// rather than a queue: it puts the few things that need a person first
// (conflicts), then what became established and how, then the suggestions
// grouped by theme, then the content drifting away from an established rule,
// and last the project in numbers. It has nothing to clear and never counts
// unread work.
//
// "Since you last looked" is a marker per project in this machine account's
// config (ContextDigestMarkerPath), never in the operation log: when a person
// looked is a fact about the reader, and the log holds facts about the
// project. Items older than the marker stay in the digest, flagged as not
// new, so a surface can show them under "Earlier".
//
// Every surface reads the digest through ContextDigest: the desktop's project
// home, `kapi context review`, and later a pull-request comment.
package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/terms"
)

// Digest themes, in the order a digest lists them.
const (
	// DigestThemeNames holds term rules about how a name is spelled: the
	// preferred form and the forms to avoid differ only in case, spacing or
	// punctuation ("Quickcast", not "Quick cast").
	DigestThemeNames = "names"
	// DigestThemeWords holds term rules that replace one word with another,
	// or say a word to avoid.
	DigestThemeWords = "words"
	// DigestThemeWriting holds what was noticed about how the project writes,
	// in prose: address, tone.
	DigestThemeWriting = "writing"
	// DigestThemeMemory holds wording pairs for the content memory.
	DigestThemeMemory = "memory"
)

// digestThemes is every theme with the title a text digest prints for it.
var digestThemes = []struct{ id, title string }{
	{DigestThemeNames, "Names and spellings"},
	{DigestThemeWords, "Words to avoid"},
	{DigestThemeWriting, "How the project writes"},
	{DigestThemeMemory, "Wording in other languages"},
}

// digestEarlierEstablished caps the established rules a digest repeats from
// before the marker, so "Earlier" stays a glance back rather than a history.
const digestEarlierEstablished = 10

// digestWeek is the span "new this week" counts over.
const digestWeek = 7 * 24 * time.Hour

// ContextDigestRequest asks for one project's digest.
type ContextDigestRequest struct {
	// Project is the recipe path. Empty resolves the project the way every
	// other command does, unless Key names one.
	Project string
	// Key names the project by its workspace key instead, for a surface that
	// holds the key rather than a checkout. The recipe of a checkout on this
	// machine is read, when there is one, to name the collections.
	Key workspace.ProjectKey
	// Since overrides the marker. Zero reads the marker this machine holds for
	// the project, and a project never looked at has none, so everything is
	// new.
	Since time.Time
}

// ContextDigest is what kapi learned about how one project writes.
type ContextDigest struct {
	// Project is the project's workspace key, and ProjectName what a person
	// calls it.
	Project     string `json:"project"`
	ProjectName string `json:"project_name,omitempty"`
	// Since is when the person last looked: an item recorded after it is new.
	// Zero when nobody has looked from this machine.
	Since time.Time `json:"since,omitzero"`
	// Conflicts need a person: two rules disagree, or the evidence turned
	// against a rule.
	Conflicts []DigestConflict `json:"conflicts"`
	// Established are the rules that came into force, newest first: every one
	// since the marker, then a few from before it.
	Established []DigestItem `json:"established"`
	// Suggested are the suggestions still advising, grouped by theme.
	Suggested []DigestTheme `json:"suggested"`
	// Drift is content moving away from an established rule.
	Drift []DigestDrift `json:"drift"`
	// Numbers is the project in numbers.
	Numbers DigestNumbers `json:"numbers"`
}

// DigestItem is one rule, suggestion or fact as the digest shows it.
type DigestItem struct {
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
	// project declares none or the recipe is not on this machine.
	Collection string `json:"collection,omitempty"`
	// Standing is the evidence as plain counts: "seen in 3 sessions · 14 of
	// 15 uses in docs/ · merged in #412".
	Standing string `json:"standing,omitempty"`
	// Usage is the project's own words back to it, from the latest usage
	// count, for a rule a check has counted.
	Usage *DigestUsage `json:"usage,omitempty"`
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
	// each axis of its point (ContextWidenOptions).
	WidenTo []string `json:"widen_to,omitempty"`
	// EstablishedAt and How say, for an established rule, when it came into
	// force and on what: "merged in #412", "your correction in billing.md",
	// "kept by you".
	EstablishedAt time.Time `json:"established_at,omitzero"`
	How           []string  `json:"how,omitempty"`
}

// DigestUsage is how often the project's content writes a rule's forms.
type DigestUsage struct {
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

// DigestConflict is a disagreement a person settles: two or more rules saying
// different things about one word, or one rule the evidence turned against.
type DigestConflict struct {
	// Sides are the rules that disagree. Choosing one keeps it and drops the
	// others.
	Sides []DigestItem `json:"sides"`
	// ByEvidence reports a rule contested by evidence alone, with no rival
	// rule: keeping it again is the choice, and so is dropping it.
	ByEvidence bool `json:"by_evidence"`
	// Reason says what the disagreement is, in a sentence.
	Reason string `json:"reason"`
}

// DigestTheme is one theme's suggestions, split by collection when they sit in
// more than one.
type DigestTheme struct {
	// Theme is the theme's id (DigestThemeNames and its neighbours), and Title
	// what a text digest prints.
	Theme  string        `json:"theme"`
	Title  string        `json:"title"`
	Groups []DigestGroup `json:"groups"`
}

// DigestGroup is the suggestions of one theme in one collection. A person may
// keep a whole group at once.
type DigestGroup struct {
	// Collection is empty for suggestions whose evidence sits in no
	// collection, and for a theme whose suggestions all sit in one.
	Collection string       `json:"collection,omitempty"`
	Items      []DigestItem `json:"items"`
}

// DigestDrift is content moving away from an established rule.
type DigestDrift struct {
	Rule DigestItem `json:"rule"`
	// Line says what moved: `docs/ says "Quick cast" 3 times and "Quickcast"
	// 11 times`.
	Line string `json:"line"`
	// Rejected is how many times the content now writes a rejected form, and
	// Before the count when the rule came into force.
	Rejected int `json:"rejected"`
	Before   int `json:"before"`
}

// DigestNumbers is the project in numbers.
type DigestNumbers struct {
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

// ContextDigest assembles one project's digest from the operation log. It
// records nothing and leaves the marker where it is.
func (a *App) ContextDigest(ctx context.Context, req ContextDigestRequest) (ContextDigest, error) {
	ws, err := a.Workspace(ctx)
	if err != nil {
		return ContextDigest{}, err
	}
	key, proj, err := a.digestProject(ctx, ws, req)
	if err != nil {
		return ContextDigest{}, err
	}
	records, err := a.contextLedger(ctx, ws).Records(ctx, contextop.Filter{})
	if err != nil {
		return ContextDigest{}, err
	}
	since := req.Since
	if since.IsZero() {
		since = ContextDigestMarker(key)
	}
	var collectionFor func(string) string
	if proj != nil && len(proj.Collections) > 0 {
		collectionFor = proj.CollectionForPath
	}
	if reader, rerr := a.commandActor(); rerr == nil {
		records = seenBy(records, reader.Actor)
	}
	out := buildDigest(records, key, since, a.GovernanceInstant(), collectionFor)
	if reg, ok, lerr := ws.Lookup(ctx, key); lerr == nil && ok {
		out.ProjectName = workspaceRegistrationName(reg)
	}
	return out, nil
}

// digestProject resolves the project a digest is for, and loads its recipe
// when a checkout of it is on this machine.
func (a *App) digestProject(ctx context.Context, ws *workspace.Workspace, req ContextDigestRequest) (workspace.ProjectKey, *project.KapiProject, error) {
	if req.Key == "" {
		s, err := a.contextOps(ctx, req.Project)
		if err != nil {
			return "", nil, err
		}
		return s.key, s.proj, nil
	}
	reg, ok, err := ws.Lookup(ctx, req.Key)
	if err != nil {
		return "", nil, err
	}
	if !ok {
		return "", nil, fmt.Errorf("this workspace holds no project %q", req.Key)
	}
	for _, dir := range reg.Checkouts {
		recipe := filepath.Join(dir, project.RecipeFileName)
		if _, serr := os.Stat(recipe); serr != nil {
			continue
		}
		if proj, lerr := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true}); lerr == nil {
			return req.Key, proj, nil
		}
	}
	return req.Key, nil, nil
}

// ContextNews is what one project's digest holds that the person has not seen:
// the line a project list shows beside the project, in place of any count of
// work waiting.
type ContextNews struct {
	Project     string `json:"project"`
	ProjectName string `json:"project_name,omitempty"`
	// New counts the digest's items recorded since the person last looked, and
	// Conflicts the disagreements that need them.
	New       int `json:"new"`
	Conflicts int `json:"conflicts"`
	// Since is the person's marker for the project, zero when they never
	// looked.
	Since time.Time `json:"since,omitzero"`
}

// ContextNews reports, for every project in the workspace whose digest holds
// something new since the person last looked, how much. It folds the log once.
// A project with nothing new is left out, so a list shows nothing beside it.
func (a *App) ContextNews(ctx context.Context) ([]ContextNews, error) {
	ws, err := a.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	regs, err := ws.Projects(ctx)
	if err != nil {
		return nil, err
	}
	records, err := a.contextLedger(ctx, ws).Records(ctx, contextop.Filter{})
	if err != nil {
		return nil, err
	}
	markers := loadDigestMarkers()
	now := a.GovernanceInstant()
	out := []ContextNews{}
	for _, reg := range regs {
		since := markers.Projects[string(reg.Key)]
		d := buildDigest(records, reg.Key, since, now, nil)
		if d.Numbers.New == 0 {
			continue
		}
		out = append(out, ContextNews{
			Project:     string(reg.Key),
			ProjectName: workspaceRegistrationName(reg),
			New:         d.Numbers.New,
			Conflicts:   len(d.Conflicts),
			Since:       since,
		})
	}
	return out, nil
}

// ContextSessionQuietAfter is how long a session must have been silent before
// a surface calls it finished. A session with an operation more recent than
// this is working now, and a summary of it would count half its work.
const ContextSessionQuietAfter = 2 * time.Minute

// ContextActivity is one session's part in a project's context: an agent
// session, or a person's or a tool's work on one local day.
type ContextActivity struct {
	// Actor is who did it.
	Actor contextop.Actor `json:"actor"`
	// At is the session's latest operation in the project.
	At time.Time `json:"at"`
	// Working reports an agent session with an operation in the last
	// ContextSessionQuietAfter.
	Working bool `json:"working"`
	// Recorded counts the session's observations, and Corrected its
	// corrections.
	Recorded  int `json:"recorded"`
	Corrected int `json:"corrected"`
	// NewByTheme counts, per digest theme (DigestThemeNames and its
	// neighbours), what the session recorded that the digest still holds and
	// the person has not seen.
	NewByTheme map[string]int `json:"new_by_theme,omitempty"`
}

// ContextProjectStanding is one project's context in numbers, for a list of
// every project in the workspace.
type ContextProjectStanding struct {
	Project     string `json:"project"`
	ProjectName string `json:"project_name,omitempty"`
	// Rules counts the rules in force, Suggested the suggestions advising and
	// Conflicts the disagreements that need a person.
	Rules     int `json:"rules"`
	Suggested int `json:"suggested"`
	Conflicts int `json:"conflicts"`
	// Terms counts the concepts in the project's terms store: the ones a
	// person or a file put there, and the ones a kept rule recorded. A project
	// whose store is missing or cannot be read counts none.
	Terms int `json:"terms"`
	// New counts the digest's items recorded since the person last looked.
	New int `json:"new"`
	// Since is the person's marker for the project, zero when they never
	// looked.
	Since time.Time `json:"since,omitzero"`
	// Latest is the most recent session that recorded or corrected
	// something in the project, nil when none has.
	Latest *ContextActivity `json:"latest,omitempty"`
}

// ContextStandings reports every project registered in the workspace with its
// context in numbers and its latest session. It folds the log once, and
// includes a project with nothing recorded, so a list can show every project
// beside its numbers. Each project's terms are counted in its own context
// store, which no other registered project shares, so a list that sums them
// counts each concept once.
func (a *App) ContextStandings(ctx context.Context) ([]ContextProjectStanding, error) {
	ws, err := a.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	regs, err := ws.Projects(ctx)
	if err != nil {
		return nil, err
	}
	records, err := a.contextLedger(ctx, ws).Records(ctx, contextop.Filter{})
	if err != nil {
		return nil, err
	}
	if reader, rerr := a.commandActor(); rerr == nil {
		records = seenBy(records, reader.Actor)
	}
	markers := loadDigestMarkers()
	// The wall clock rather than the run's governance instant: a desktop
	// holds one App for hours, and "working now" is about this moment.
	now := time.Now().UTC()
	out := make([]ContextProjectStanding, 0, len(regs))
	for _, reg := range regs {
		since := markers.Projects[string(reg.Key)]
		s := contextStanding(records, reg.Key, since, now)
		s.ProjectName = workspaceRegistrationName(reg)
		s.Terms = contextTermCount(ctx, ws, reg.Key)
		out = append(out, s)
	}
	return out, nil
}

// contextTermCount counts the concepts in one project's terms store, the number
// the Terms section of the project's context reports. It opens a store only
// when the workspace already holds one, and reads it without binding the terms
// schema, so reading the standings writes nothing. A store that is missing,
// carries no terms schema, or cannot be read counts zero: one unreadable
// project must not take the numbers of every other one with it.
func contextTermCount(ctx context.Context, ws *workspace.Workspace, key workspace.ProjectKey) int {
	held, err := ws.HasContext(ctx, key)
	if err != nil || !held {
		return 0
	}
	db, err := ws.Context(ctx, key)
	if err != nil {
		return 0
	}
	if isStore, err := terms.IsStore(ctx, db); err != nil || !isStore {
		return 0
	}
	n, err := terms.CountConcepts(ctx, db)
	if err != nil {
		return 0
	}
	return n
}

// contextStanding folds one project's standing from the log, newest first as
// contextop.Ledger.Records returns it.
func contextStanding(records []contextop.Record, key workspace.ProjectKey, since, now time.Time) ContextProjectStanding {
	d := buildDigest(records, key, since, now, nil)
	return ContextProjectStanding{
		Project:   string(key),
		Rules:     d.Numbers.Rules,
		Suggested: d.Numbers.Suggested,
		Conflicts: d.Numbers.Conflicts,
		New:       d.Numbers.New,
		Since:     since,
		Latest:    latestActivity(records, key, since, now),
	}
}

// latestActivity is the most recent session that recorded or corrected
// something in a project. Sessions group the way the feed groups them: an
// agent's by the session id its operations carry, a person's or a tool's by
// actor and local day. A keep or a drop never makes a session the latest, so
// a person deciding leaves the assistant's session in place.
func latestActivity(records []contextop.Record, key workspace.ProjectKey, since, now time.Time) *ContextActivity {
	var out *ContextActivity
	group := ""
	for _, r := range records {
		if r.Project == key && (r.Kind == contextop.KindObserve || r.Kind == contextop.KindCorrect) {
			group = contextActivityGroup(r)
			out = &ContextActivity{Actor: r.Actor}
			break
		}
	}
	if out == nil {
		return nil
	}
	for _, r := range records {
		if r.Project != key || contextActivityGroup(r) != group {
			continue
		}
		if r.At.After(out.At) {
			out.At = r.At
		}
		switch r.Kind {
		case contextop.KindObserve:
			out.Recorded++
		case contextop.KindCorrect:
			out.Corrected++
		}
		if r.Kind.Bears() && r.Status.Answers() && r.Subject.Kind != contextop.SubjectNone && (since.IsZero() || r.At.After(since)) {
			if out.NewByTheme == nil {
				out.NewByTheme = map[string]int{}
			}
			out.NewByTheme[digestTheme(r.Subject)]++
		}
	}
	out.Working = out.Actor.Kind == contextop.ActorAgent && now.Sub(out.At) < ContextSessionQuietAfter
	return out
}

// ContextWorkspaceRule is one rule a person applied to the whole workspace:
// it holds in every project, beside each project's own rules.
type ContextWorkspaceRule struct {
	// ID names the operation whose rule it is, and Short the form a person
	// types.
	ID    string `json:"id"`
	Short string `json:"short"`
	// Sentence is the rule as a sentence, and Subject the rule itself.
	Sentence string            `json:"sentence"`
	Subject  contextop.Subject `json:"subject"`
	// Project is the project the rule was decided in, and ProjectName what a
	// person calls it.
	Project     string `json:"project"`
	ProjectName string `json:"project_name,omitempty"`
	// At is when the rule came into force, and How on what: "kept by you",
	// "merged in #412".
	At  time.Time `json:"at"`
	How []string  `json:"how,omitempty"`
}

// ContextWorkspaceRules lists the rules in force across the workspace, the
// ones every check resolves beside a project's own (contextop.WidenedRules),
// oldest first, each with how it came into force.
func (a *App) ContextWorkspaceRules(ctx context.Context) ([]ContextWorkspaceRule, error) {
	ws, err := a.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	widened, err := contextop.WidenedRules(ctx, ws)
	if err != nil {
		return nil, err
	}
	out := make([]ContextWorkspaceRule, 0, len(widened))
	if len(widened) == 0 {
		return out, nil
	}
	records, err := a.contextLedger(ctx, ws).Records(ctx, contextop.Filter{})
	if err != nil {
		return nil, err
	}
	if reader, rerr := a.commandActor(); rerr == nil {
		records = seenBy(records, reader.Actor)
	}
	names := map[workspace.ProjectKey]string{}
	if regs, rerr := ws.Projects(ctx); rerr == nil {
		for _, reg := range regs {
			names[reg.Key] = workspaceRegistrationName(reg)
		}
	}
	return workspaceRules(widened, records, names), nil
}

// workspaceRules renders the widened rules, reading how each came into force
// from the folded log. A rule whose operation the log no longer holds keeps
// the moment it was widened and says nothing of how.
func workspaceRules(widened []contextop.WidenedRule, records []contextop.Record, names map[workspace.ProjectKey]string) []ContextWorkspaceRule {
	byID := make(map[string]contextop.Record, len(records))
	for _, r := range records {
		byID[r.ID] = r
	}
	acts := digestActs(records, byID)
	out := make([]ContextWorkspaceRule, 0, len(widened))
	for _, w := range widened {
		rule := ContextWorkspaceRule{
			ID:          w.Operation,
			Short:       contextop.ShortID(w.Operation),
			Sentence:    ruleSentence(w.Subject),
			Subject:     w.Subject,
			Project:     string(w.Project),
			ProjectName: names[w.Project],
			At:          w.At,
		}
		if r, ok := byID[w.Operation]; ok && r.Project == w.Project {
			rule.At, rule.How = establishment(r, acts[r.ID], byID)
		}
		out = append(out, rule)
	}
	return out
}

// contextActivityGroup is the session an operation belongs to.
func contextActivityGroup(r contextop.Record) string {
	if r.Actor.Session != "" {
		return "session:" + r.Actor.Session
	}
	kind := r.Actor.Kind
	if kind == "" {
		kind = contextop.ActorPerson
	}
	return "actor:" + string(kind) + "/" + r.Actor.Name + ":" + r.At.Local().Format(time.DateOnly)
}

// workspaceRegistrationName is what a person calls a registered project.
func workspaceRegistrationName(reg workspace.Registration) string {
	if reg.Name != "" {
		return reg.Name
	}
	return string(reg.Key)
}

// buildDigest assembles a digest from a folded log, newest first as
// contextop.Ledger.Records returns it. collectionFor names the collection a
// path sits in, nil when the project declares none.
func buildDigest(records []contextop.Record, key workspace.ProjectKey, since, now time.Time, collectionFor func(string) string) ContextDigest {
	out := ContextDigest{
		Project:     string(key),
		Since:       since,
		Conflicts:   []DigestConflict{},
		Established: []DigestItem{},
		Suggested:   []DigestTheme{},
		Drift:       []DigestDrift{},
	}
	byID := make(map[string]contextop.Record, len(records))
	for _, r := range records {
		byID[r.ID] = r
	}
	acts := digestActs(records, byID)
	isNew := func(at time.Time) bool { return since.IsZero() || at.After(since) }
	item := func(r contextop.Record) DigestItem {
		it := digestItem(r, collectionFor)
		it.New = isNew(r.At)
		return it
	}

	conflicted := map[string]bool{}
	themed := map[string]map[string][]DigestItem{}
	var established []DigestItem
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
			out.Conflicts = append(out.Conflicts, digestConflict(r, byID, conflicted, item))
		case r.Established:
			it := item(r)
			it.EstablishedAt, it.How = establishment(r, acts[r.ID], byID)
			it.New = isNew(it.EstablishedAt)
			out.Numbers.Rules++
			if now.Sub(it.EstablishedAt) < digestWeek {
				out.Numbers.NewThisWeek++
			}
			if it.Usage == nil {
				it.Usage = latestUsage(r, acts[r.ID])
			}
			established = append(established, it)
			if drift, ok := digestDrift(r, acts[r.ID], it); ok {
				out.Drift = append(out.Drift, drift)
			}
		case r.Subject.Kind != contextop.SubjectNone:
			it := item(r)
			out.Numbers.Suggested++
			if themed[it.Theme] == nil {
				themed[it.Theme] = map[string][]DigestItem{}
			}
			themed[it.Theme][it.Collection] = append(themed[it.Theme][it.Collection], it)
		}
	}
	out.Numbers.Conflicts = len(out.Conflicts)

	sort.SliceStable(established, func(i, j int) bool { return established[i].EstablishedAt.After(established[j].EstablishedAt) })
	earlier := 0
	for _, it := range established {
		if !it.New {
			if earlier == digestEarlierEstablished {
				continue
			}
			earlier++
		}
		out.Established = append(out.Established, it)
	}

	for _, theme := range digestThemes {
		byCollection := themed[theme.id]
		if len(byCollection) == 0 {
			continue
		}
		t := DigestTheme{Theme: theme.id, Title: theme.title, Groups: []DigestGroup{}}
		collections := slices.Sorted(maps.Keys(byCollection))
		if len(collections) == 1 {
			// One collection says nothing a reader needs told.
			t.Groups = append(t.Groups, DigestGroup{Items: byCollection[collections[0]]})
		} else {
			for _, c := range collections {
				t.Groups = append(t.Groups, DigestGroup{Collection: c, Items: byCollection[c]})
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

// digestActs indexes, per subject-bearing operation, the operations that act
// on it, oldest first.
func digestActs(records []contextop.Record, byID map[string]contextop.Record) map[string][]contextop.Record {
	out := map[string][]contextop.Record{}
	for _, act := range slices.Backward(records) {
		if act.Kind.Bears() || act.Target == "" {
			continue
		}
		if bearer, ok := digestBearer(byID, act); ok {
			out[bearer.ID] = append(out[bearer.ID], act)
		}
	}
	return out
}

// digestBearer follows an act's target to the operation that carries the
// subject.
func digestBearer(byID map[string]contextop.Record, r contextop.Record) (contextop.Record, bool) {
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

// digestItem renders one subject-bearing operation.
func digestItem(r contextop.Record, collectionFor func(string) string) DigestItem {
	it := DigestItem{
		ID:        r.ID,
		Short:     contextop.ShortID(r.ID),
		Status:    r.Status,
		Theme:     digestTheme(r.Subject),
		Sentence:  ruleSentence(r.Subject),
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
		it.Usage = digestUsage(rule.Replacement, append([]string{rule.Term}, rule.Forms...), u.Preferred, u.Total-u.Preferred, u.Within)
	}
	switch {
	case r.Established:
		it.Droppable = true
		if r.Status == contextop.StatusEstablished {
			it.WidenTo = ContextWidenOptions(r.Scope)
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

// digestTheme sorts a subject into a suggestion theme.
func digestTheme(s contextop.Subject) string {
	switch s.Kind {
	case contextop.SubjectMemory:
		return DigestThemeMemory
	case contextop.SubjectNote:
		return DigestThemeWriting
	}
	rule, ok := s.Rule()
	if !ok || rule.Replacement == "" {
		return DigestThemeWords
	}
	want := spellingKey(rule.Replacement)
	for _, form := range append([]string{rule.Term}, rule.Forms...) {
		if spellingKey(form) == want {
			return DigestThemeNames
		}
	}
	return DigestThemeWords
}

// spellingKey folds case, spacing and joining punctuation, so two spellings of
// one name compare equal.
func spellingKey(form string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(form) {
		switch r {
		case ' ', '-', '_', '.', ' ':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ruleSentence states a subject as the sentence a person reads.
func ruleSentence(s contextop.Subject) string {
	switch s.Kind {
	case contextop.SubjectTerm:
		rule, ok := s.Rule()
		if !ok {
			return ""
		}
		avoid := orList(append([]string{rule.Term}, rule.Forms...))
		if rule.Replacement == "" {
			return "Avoid " + avoid + "."
		}
		return "Write " + rule.Replacement + ", not " + avoid + "."
	case contextop.SubjectMemory:
		if s.Memory == nil {
			return ""
		}
		return fmt.Sprintf("Translate %q into %s as %q.", s.Memory.Source, s.Memory.TargetLocale, s.Memory.Target)
	case contextop.SubjectNote:
		return s.Text
	}
	return ""
}

// orList joins forms the way a sentence lists alternatives: "a", "a or b",
// "a, b or c".
func orList(forms []string) string {
	switch len(forms) {
	case 0:
		return ""
	case 1:
		return forms[0]
	}
	return strings.Join(forms[:len(forms)-1], ", ") + " or " + forms[len(forms)-1]
}

// digestUsage says how the content writes a rule's forms.
func digestUsage(preferred string, rejected []string, preferredCount, rejectedCount int, within string) *DigestUsage {
	where := "The project"
	if within != "" {
		where = within
	}
	line := fmt.Sprintf("%s says %q %s", where, preferred, times(preferredCount))
	if rejectedCount > 0 {
		quoted := make([]string, len(rejected))
		for i, f := range rejected {
			quoted[i] = strconv.Quote(f)
		}
		line += fmt.Sprintf(" and %s %s", orList(quoted), times(rejectedCount))
	}
	return &DigestUsage{
		Preferred:      preferred,
		PreferredCount: preferredCount,
		Rejected:       rejected,
		RejectedCount:  rejectedCount,
		Within:         within,
		Line:           line,
	}
}

// times renders a count of uses the way a sentence says it.
func times(n int) string {
	switch n {
	case 1:
		return "once"
	case 2:
		return "twice"
	}
	return strconv.Itoa(n) + " times"
}

// digestConflict gathers one contested record and the rules on its other
// side, marking every one it takes so the disagreement is listed once.
func digestConflict(r contextop.Record, byID map[string]contextop.Record, taken map[string]bool, item func(contextop.Record) DigestItem) DigestConflict {
	// Every side of a conflict can be chosen: choosing keeps it and drops the
	// others, and keeping a rule the evidence turned against is the choice.
	side := func(r contextop.Record) DigestItem {
		it := item(r)
		it.Keepable, it.Droppable = true, !r.Established
		return it
	}
	c := DigestConflict{Sides: []DigestItem{}}
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
			return fmt.Sprintf("%s changed %q back to %q%s.", actorPhrase(against.Actor, true), against.Correction.From, against.Correction.To, where)
		case against.Kind == contextop.KindSignal && against.Signal != nil && against.Signal.Source == contextop.SignalUsage:
			return fmt.Sprintf("The content moved to a form this rule avoids: %d uses now.", against.Signal.Rejected)
		case against.Kind == contextop.KindWithdraw:
			return "Its author withdrew it after a person backed it."
		}
	}
	return "The evidence no longer agrees on this rule."
}

// establishment reads when a rule came into force and on what, from the
// operations that act on it.
func establishment(r contextop.Record, acts []contextop.Record, byID map[string]contextop.Record) (time.Time, []string) {
	switch r.Kind {
	case contextop.KindImport:
		how := "imported"
		if r.Note != "" {
			how += " (" + r.Note + ")"
		}
		return r.At, []string{how}
	case contextop.KindEdit:
		return r.At, []string{"written by " + actorPhrase(r.Actor, false)}
	}
	var at time.Time
	var how []string
	for _, act := range acts {
		switch act.Kind {
		case contextop.KindKeep:
			at, how = act.At, []string{"kept by " + actorPhrase(act.Actor, false)}
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
			line := actorPhrase(ev.Actor, true) + " correction"
			if actorPhrase(ev.Actor, true) == "you" {
				line = "your correction"
			}
			if len(ev.Evidence) > 0 && ev.Evidence[0].Path != "" {
				line += " in " + ev.Evidence[0].Path
			}
			out = append(out, line)
		case ev.Kind == contextop.KindKeep:
			out = append(out, "kept by "+actorPhrase(ev.Actor, false))
		}
	}
	return out
}

// seenBy returns the records as the person reading the digest sees them: what
// they did themselves carries no name, so it reads as "you", and what a
// teammate did carries the teammate's. The records are copied, never changed in
// place.
func seenBy(records []contextop.Record, reader contextop.Actor) []contextop.Record {
	if reader.Kind != contextop.ActorPerson || reader.Name == "" {
		return records
	}
	out := make([]contextop.Record, len(records))
	for i, r := range records {
		if r.Actor.Kind == contextop.ActorPerson && r.Actor.Name == reader.Name {
			r.Actor.Name = ""
		}
		out[i] = r
	}
	return out
}

// actorPhrase names an actor in a sentence. A person with no name is the
// person reading, so "you"; possessive asks for the form that precedes a noun.
func actorPhrase(a contextop.Actor, possessive bool) string {
	switch a.Kind {
	case contextop.ActorPerson, "":
		if a.Name == "" {
			return "you"
		}
		if possessive {
			return a.Name + "'s"
		}
		return a.Name
	case contextop.ActorAgent:
		name := a.Name
		if name == "" {
			name = "an agent"
		}
		if possessive {
			return name + "'s"
		}
		return name
	}
	name := "kapi " + a.Name
	if possessive {
		return name + "'s"
	}
	return name
}

// latestUsage is the latest usage count recorded against a rule, nil when no
// check has counted it.
func latestUsage(r contextop.Record, acts []contextop.Record) *DigestUsage {
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
	return digestUsage(rule.Replacement, append([]string{rule.Term}, rule.Forms...), latest.Preferred, latest.Rejected, latest.Within)
}

// digestDrift reports an established rule whose content moved to a form it
// avoids: the latest usage count writes a rejected form more often than the
// baseline. The baseline is the last count taken when the rule came into force,
// or, for a rule nobody had counted by then, the first count after it, so
// rejected uses the content already held when the rule was kept are not drift.
func digestDrift(r contextop.Record, acts []contextop.Record, it DigestItem) (DigestDrift, bool) {
	rule, ok := r.Rule()
	if !ok || rule.Replacement == "" {
		return DigestDrift{}, false
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
		return DigestDrift{}, false
	}
	usage := digestUsage(rule.Replacement, append([]string{rule.Term}, rule.Forms...), latest.Preferred, latest.Rejected, latest.Within)
	it.Usage = usage
	return DigestDrift{Rule: it, Line: usage.Line, Rejected: latest.Rejected, Before: base.Rejected}, true
}

// FormatText renders the digest the way `kapi context review` lists it.
func (d ContextDigest) FormatText(w io.Writer) error {
	var b strings.Builder
	name := d.ProjectName
	if name == "" {
		name = d.Project
	}
	nothingNew := d.Numbers.New == 0 && len(d.Conflicts) == 0 && len(d.Drift) == 0
	if nothingNew && !d.Since.IsZero() {
		fmt.Fprintf(&b, "Nothing new since %s.\n", d.Since.Local().Format("Monday 2 January"))
	} else if !d.Since.IsZero() {
		fmt.Fprintf(&b, "Since you last looked (%s):\n", d.Since.Local().Format("Monday 2 January 15:04"))
	}

	if len(d.Conflicts) > 0 {
		b.WriteString("\nNeeds you\n")
		for _, c := range d.Conflicts {
			fmt.Fprintf(&b, "  %s\n", c.Reason)
			for _, side := range c.Sides {
				writeDigestItem(&b, side, "    ")
			}
		}
	}
	if len(d.Established) > 0 {
		b.WriteString("\nEstablished\n")
		writeNewThenEarlier(&b, d.Established, "  ", func(it DigestItem) {
			writeDigestItem(&b, it, "  ")
		})
	}
	if len(d.Suggested) > 0 {
		b.WriteString("\nSuggested\n")
		for _, t := range d.Suggested {
			fmt.Fprintf(&b, "  %s\n", t.Title)
			for _, g := range t.Groups {
				indent := "    "
				if g.Collection != "" {
					fmt.Fprintf(&b, "    in %s\n", g.Collection)
					indent = "      "
				}
				writeNewThenEarlier(&b, g.Items, indent, func(it DigestItem) { writeDigestItem(&b, it, indent) })
			}
		}
	}
	if len(d.Drift) > 0 {
		b.WriteString("\nDrift\n")
		for _, dr := range d.Drift {
			writeDigestItem(&b, dr.Rule, "  ")
		}
	}
	fmt.Fprintf(&b, "\nkapi knows %d %s for %s", d.Numbers.Rules, pluralWord(d.Numbers.Rules, "rule", "rules"), name)
	if d.Numbers.NewThisWeek > 0 {
		fmt.Fprintf(&b, "; %d %s new this week", d.Numbers.NewThisWeek, pluralWord(d.Numbers.NewThisWeek, "is", "are"))
	}
	b.WriteString(".")
	if d.Numbers.Suggested > 0 {
		fmt.Fprintf(&b, " %d %s advising.", d.Numbers.Suggested, pluralWord(d.Numbers.Suggested, "suggestion is", "suggestions are"))
	}
	b.WriteString("\n")
	if len(d.Conflicts) > 0 || d.Numbers.Suggested > 0 {
		b.WriteString("\nDecide each one with `kapi context review` in a terminal, or name it: " +
			"`kapi context review --keep <id>` (`--use <form>` changes it as you keep it, `--widen-to workspace` applies it more widely), " +
			"`kapi context review --drop <id>`.\n")
	}
	if len(d.Conflicts) > 0 {
		b.WriteString("Choose a side of a conflict with `kapi context review --choose <id>`, which sets the other side aside.\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// writeNewThenEarlier writes the new items, then the rest under "Earlier".
func writeNewThenEarlier(b *strings.Builder, items []DigestItem, indent string, write func(DigestItem)) {
	earlier := false
	for _, it := range items {
		if it.New {
			write(it)
		}
	}
	for _, it := range items {
		if it.New {
			continue
		}
		if !earlier {
			b.WriteString(indent + "Earlier\n")
			earlier = true
		}
		write(it)
	}
}

// writeDigestItem writes one item's lines.
func writeDigestItem(b *strings.Builder, it DigestItem, indent string) {
	fmt.Fprintf(b, "%s%s  %s\n", indent, it.Short, it.Sentence)
	if it.Quote != nil && (it.Quote.Quote != "" || it.Quote.Path != "") {
		line := it.Quote.Path
		if it.Quote.Quote != "" {
			line = strconv.Quote(it.Quote.Quote)
			if it.Quote.Path != "" {
				line += " in " + it.Quote.Path
			}
		}
		fmt.Fprintf(b, "%s      seen %s\n", indent, line)
	}
	if it.Usage != nil {
		fmt.Fprintf(b, "%s      %s\n", indent, it.Usage.Line)
	}
	var facts []string
	if it.Standing != "" {
		facts = append(facts, it.Standing)
	}
	if it.Established() {
		facts = append(facts, it.How...)
	} else {
		facts = append(facts, "noticed by "+actorPhrase(it.NoticedBy, false))
	}
	if len(facts) > 0 {
		fmt.Fprintf(b, "%s      %s\n", indent, strings.Join(facts, " · "))
	}
}

// Established reports an item that is a rule in force.
func (it DigestItem) Established() bool { return !it.EstablishedAt.IsZero() }

func pluralWord(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// contextDigestMarkerFile is the marker file's name under ConfigDir().
const contextDigestMarkerFile = "context-digest.json"

// ContextDigestMarkerPath is where this machine account keeps when its person
// last looked at each project's digest.
func ContextDigestMarkerPath() string {
	return filepath.Join(ConfigDir(), contextDigestMarkerFile)
}

// contextDigestMarkers is the marker file: the last look per project key.
type contextDigestMarkers struct {
	Projects map[string]time.Time `json:"projects"`
}

// ContextDigestMarker is when the person on this machine last looked at a
// project's digest, zero when never.
func ContextDigestMarker(key workspace.ProjectKey) time.Time {
	return loadDigestMarkers().Projects[string(key)]
}

// MarkContextDigestSeen records that the person looked at a project's digest
// at the instant given, so the next digest reads what came after. It returns
// the marker it replaced.
func MarkContextDigestSeen(key workspace.ProjectKey, at time.Time) (time.Time, error) {
	if key == "" {
		return time.Time{}, errors.New("name the project whose digest was read")
	}
	// One read-modify-write at a time in this process: a surface that opens a
	// digest can mark it seen twice in one moment (React runs an effect twice
	// in development), and two writers racing on one temporary file lost the
	// rename. Another process writes its own temporary file, so the rename
	// stays whole across processes too.
	digestMarkerMu.Lock()
	defer digestMarkerMu.Unlock()
	m := loadDigestMarkers()
	previous := m.Projects[string(key)]
	if at.Before(previous) {
		return previous, nil
	}
	m.Projects[string(key)] = at.UTC()
	path := ContextDigestMarkerPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return previous, fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return previous, err
	}
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return previous, fmt.Errorf("write the digest marker: %w", err)
	}
	tmp := f.Name()
	if _, err := f.Write(append(data, '\n')); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return previous, fmt.Errorf("write the digest marker: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return previous, fmt.Errorf("write the digest marker: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return previous, fmt.Errorf("write the digest marker: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return previous, fmt.Errorf("replace the digest marker: %w", err)
	}
	return previous, nil
}

// digestMarkerMu serialises the marker file's read-modify-write in this
// process.
var digestMarkerMu sync.Mutex

// loadDigestMarkers reads the marker file. A missing or unreadable file means
// nobody has looked, which is the same starting state as an empty one.
func loadDigestMarkers() contextDigestMarkers {
	m := contextDigestMarkers{Projects: map[string]time.Time{}}
	data, err := os.ReadFile(ContextDigestMarkerPath())
	if err != nil {
		return m
	}
	if json.Unmarshal(data, &m) != nil || m.Projects == nil {
		m.Projects = map[string]time.Time{}
	}
	return m
}

// NoteContextDigestRead advances the marker after a digest was read at a
// command line. Only a person moves it: the marker records when the person
// last looked, and an agent or a tool reading the digest leaves it where it
// is.
func (a *App) NoteContextDigestRead(d ContextDigest) error {
	resolved, err := a.commandActor()
	if err != nil || resolved.Actor.Kind != contextop.ActorPerson {
		return nil //nolint:nilerr // an actor that cannot be resolved is no person
	}
	_, err = MarkContextDigestSeen(workspace.ProjectKey(d.Project), time.Now().UTC())
	return err
}
