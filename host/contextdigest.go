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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/rulesview"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host/screens"
	"github.com/neokapi/neokapi/terms"
)

// Digest themes, in the order a digest lists them (rulesview.ThemeNames and
// its neighbours).
const (
	DigestThemeNames   = rulesview.ThemeNames
	DigestThemeWords   = rulesview.ThemeWords
	DigestThemeWriting = rulesview.ThemeWriting
	DigestThemeMemory  = rulesview.ThemeMemory
)

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
	Conflicts []screens.DigestConflict `json:"conflicts"`
	// Established are the rules that came into force, newest first: every one
	// since the marker, then a few from before it.
	Established []screens.DigestItem `json:"established"`
	// Suggested are the suggestions still advising, grouped by theme.
	Suggested []DigestTheme `json:"suggested"`
	// Drift is content moving away from an established rule.
	Drift []DigestDrift `json:"drift"`
	// Numbers is the project in numbers.
	Numbers DigestNumbers `json:"numbers"`
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
	Collection string               `json:"collection,omitempty"`
	Items      []screens.DigestItem `json:"items"`
}

// DigestDrift is content moving away from an established rule.
type DigestDrift struct {
	Rule screens.DigestItem `json:"rule"`
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
	d, err := a.digestModel(ctx, req)
	if err != nil {
		return ContextDigest{}, err
	}
	return digestFrom(d), nil
}

// digestModel assembles the digest as core/rulesview holds it. The recipe and
// the operation log are read here, the records are seen by the person
// reading, and rulesview.BuildDigest groups them.
func (a *App) digestModel(ctx context.Context, req ContextDigestRequest) (rulesview.Digest, error) {
	ws, err := a.Workspace(ctx)
	if err != nil {
		return rulesview.Digest{}, err
	}
	key, proj, err := a.digestProject(ctx, ws, req)
	if err != nil {
		return rulesview.Digest{}, err
	}
	records, err := a.contextLedger(ctx, ws).Records(ctx, contextop.Filter{})
	if err != nil {
		return rulesview.Digest{}, err
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
	out := rulesview.BuildDigest(records, key, since, a.GovernanceInstant(), collectionFor)
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
			out.NewByTheme[rulesview.ThemeOf(r.Subject)]++
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
	acts := rulesview.Acts(records, byID)
	out := make([]ContextWorkspaceRule, 0, len(widened))
	for _, w := range widened {
		rule := ContextWorkspaceRule{
			ID:          w.Operation,
			Short:       contextop.ShortID(w.Operation),
			Sentence:    rulesview.Sentence(w.Subject),
			Subject:     w.Subject,
			Project:     string(w.Project),
			ProjectName: names[w.Project],
			At:          w.At,
		}
		if r, ok := byID[w.Operation]; ok && r.Project == w.Project {
			rule.At, rule.How = rulesview.Establishment(r, acts[r.ID], byID)
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
		writeNewThenEarlier(&b, d.Established, "  ", func(it screens.DigestItem) {
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
				writeNewThenEarlier(&b, g.Items, indent, func(it screens.DigestItem) { writeDigestItem(&b, it, indent) })
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
func writeNewThenEarlier(b *strings.Builder, items []screens.DigestItem, indent string, write func(screens.DigestItem)) {
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
func writeDigestItem(b *strings.Builder, it screens.DigestItem, indent string) {
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

// The digest's types are bound by the desktop and declared here and in
// host/screens. core/rulesview holds the same shapes for every surface, and
// the functions below convert one to the other field by field.

// buildDigest assembles a digest from a folded log, newest first as
// contextop.Ledger.Records returns it (rulesview.BuildDigest).
func buildDigest(records []contextop.Record, key workspace.ProjectKey, since, now time.Time, collectionFor func(string) string) ContextDigest {
	return digestFrom(rulesview.BuildDigest(records, key, since, now, collectionFor))
}

// digestItem renders one subject-bearing operation (rulesview.NewItem).
func digestItem(r contextop.Record, collectionFor func(string) string) screens.DigestItem {
	return digestItemFrom(rulesview.NewItem(r, collectionFor))
}

// digestUsage says how the content writes a rule's forms (rulesview.NewUsage).
func digestUsage(preferred string, rejected []string, preferredCount, rejectedCount int, within string) *screens.DigestUsage {
	return digestUsageFrom(rulesview.NewUsage(preferred, rejected, preferredCount, rejectedCount, within))
}

// seenBy returns the records as the person reading them sees them
// (rulesview.SeenBy).
func seenBy(records []contextop.Record, reader contextop.Actor) []contextop.Record {
	return rulesview.SeenBy(records, reader)
}

// actorPhrase names an actor in a sentence (rulesview.ActorPhrase).
func actorPhrase(a contextop.Actor, possessive bool) string {
	return rulesview.ActorPhrase(a, possessive)
}

// digestFrom converts a digest from the model's shape to the bound one.
func digestFrom(d rulesview.Digest) ContextDigest {
	out := ContextDigest{
		Project:     d.Project,
		ProjectName: d.ProjectName,
		Since:       d.Since,
		Conflicts:   make([]screens.DigestConflict, len(d.Conflicts)),
		Established: digestItemsFrom(d.Established),
		Suggested:   make([]DigestTheme, len(d.Suggested)),
		Drift:       make([]DigestDrift, len(d.Drift)),
		Numbers:     DigestNumbers(d.Numbers),
	}
	for i, c := range d.Conflicts {
		out.Conflicts[i] = screens.DigestConflictFrom(c)
	}
	for i, t := range d.Suggested {
		theme := DigestTheme{Theme: t.Theme, Title: t.Title, Groups: make([]DigestGroup, len(t.Groups))}
		for j, g := range t.Groups {
			theme.Groups[j] = DigestGroup{Collection: g.Collection, Items: digestItemsFrom(g.Items)}
		}
		out.Suggested[i] = theme
	}
	for i, dr := range d.Drift {
		out.Drift[i] = DigestDrift{Rule: digestItemFrom(dr.Rule), Line: dr.Line, Rejected: dr.Rejected, Before: dr.Before}
	}
	return out
}

// digestItemsFrom converts a list of items (screens.DigestItemsFrom).
func digestItemsFrom(items []rulesview.Item) []screens.DigestItem {
	return screens.DigestItemsFrom(items)
}

// digestItemFrom converts one item (screens.DigestItemFrom).
func digestItemFrom(it rulesview.Item) screens.DigestItem { return screens.DigestItemFrom(it) }

// digestUsageFrom converts a usage count, whose two shapes are one.
func digestUsageFrom(u *rulesview.Usage) *screens.DigestUsage {
	return (*screens.DigestUsage)(u)
}

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
