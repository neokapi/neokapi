package backend

// The feed of context operations, and the decisions a person makes on them.
//
// An agent works in a project through the CLI or its MCP server, in another
// process. Everything it records goes into this machine account's workspace
// operation log, which is the same log the workspace watcher polls, so what it
// records appears here within a second of being written. A decision made here
// is appended to that log and written into the project's stores, so the
// agent's next read carries it. The two processes share a store and nothing
// else: no IPC, no service between them.
//
// Reading goes through contextop.Ledger, which folds the log and reports the
// status each subject-bearing operation ended up at. Deciding goes through
// host.App, which owns the policy about who may keep, drop, revert and
// widen. This file records no operation of its own.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host"
)

// contextFeedTimeout caps one read or one decision. The feed is interactive,
// and a store that has not answered in this long is better reported than
// waited on.
const contextFeedTimeout = 30 * time.Second

// contextFeedDefaultLimit is how many operations a feed carries when the
// caller names no limit. The log holds one operation per context change, so
// this is a bound on the screen rather than on the store.
const contextFeedDefaultLimit = 200

// contextSessionQuietAfter is how long a session must have been silent before
// the feed calls it finished. A summary that appears while an agent is still
// recording would be a count of half its work.
const contextSessionQuietAfter = 2 * time.Minute

// ContextActorDTO is who performed an operation.
type ContextActorDTO struct {
	// Kind is "person", "agent" or "tool".
	Kind string `json:"kind"`
	// Name is the person's handle, the agent's client name, or the tool's
	// name. A person working alone has none.
	Name string `json:"name,omitempty"`
	// Session groups one agent run's operations.
	Session string `json:"session,omitempty"`
	// Host is the machine or client the agent ran under, when the actor's name
	// carries one as "<client>@<host>".
	Host string `json:"host,omitempty"`
}

// ContextEvidenceDTO is where a subject was seen.
type ContextEvidenceDTO struct {
	Path  string `json:"path,omitempty"`
	Unit  string `json:"unit,omitempty"`
	Quote string `json:"quote,omitempty"`
}

// ContextSubjectDTO is what an operation is about, flattened for a surface.
// Kind says which fields carry anything.
type ContextSubjectDTO struct {
	// Kind is "term", "memory", "note", or empty for an operation that acts
	// on another.
	Kind string `json:"kind,omitempty"`
	// Term, Replacement and Advisory are the rule, for a term subject: Term is
	// the form to avoid, Forms the other forms it avoids, and Replacement the
	// form to use.
	Term        string   `json:"term,omitempty"`
	Forms       []string `json:"forms,omitempty"`
	Replacement string   `json:"replacement,omitempty"`
	Advisory    bool     `json:"advisory,omitempty"`
	// Source, Target and TargetLocale are the wording pair, for a content
	// memory subject.
	Source       string `json:"source,omitempty"`
	Target       string `json:"target,omitempty"`
	TargetLocale string `json:"target_locale,omitempty"`
	// Text is the prose of a note.
	Text string `json:"text,omitempty"`
	// Describe is the one line the log prints for the subject.
	Describe string `json:"describe,omitempty"`
}

// ContextCorrectionDTO is the wording a correction changed.
type ContextCorrectionDTO struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ContextScopeDTO is how far a subject answers.
type ContextScopeDTO struct {
	// Level is "project" or "workspace".
	Level string `json:"level"`
	// Coordinates are the axes of the point the evidence was seen at.
	Coordinates map[string]string `json:"coordinates,omitempty"`
	// Describe renders the scope the way the log prints it.
	Describe string `json:"describe"`
}

// ContextFeedEntry is one recorded operation as the feed shows it.
type ContextFeedEntry struct {
	ID  string `json:"id"`
	Seq int64  `json:"seq"`
	// ProjectKey and ProjectName name the project whose work produced it.
	ProjectKey  string `json:"project_key"`
	ProjectName string `json:"project_name,omitempty"`
	// Kind is "observe", "correct", "import", "edit", "keep", "drop",
	// "withdraw", "revert" or "widen".
	Kind string `json:"kind"`
	// Status is "suggested", "established", "contested", "withdrawn",
	// "dropped" or "reverted".
	Status string `json:"status"`
	// ContestedBy names the operations on the other side of a disagreement,
	// for a contested entry.
	ContestedBy []string              `json:"contested_by"`
	Actor       ContextActorDTO       `json:"actor"`
	Subject     ContextSubjectDTO     `json:"subject"`
	Correction  *ContextCorrectionDTO `json:"correction,omitempty"`
	Evidence    []ContextEvidenceDTO  `json:"evidence"`
	Scope       ContextScopeDTO       `json:"scope"`
	// Target is the operation this one acts on, and TargetSession the session
	// a revert undid.
	Target        string `json:"target,omitempty"`
	TargetSession string `json:"target_session,omitempty"`
	Note          string `json:"note,omitempty"`
	// At is when the log accepted it, RFC3339 in UTC.
	At string `json:"at"`
	// Decidable reports a suggestion carrying a rule a person can keep or
	// drop. A note and a correction that suggested nothing are recorded facts
	// with nothing to decide. A contested suggestion is decidable too, and
	// keeping it waits until the other side is dropped.
	Decidable bool `json:"decidable"`
	// Revertible reports a rule in force that a person can take back out.
	Revertible bool `json:"revertible"`
	// WidenTo are the widenings open to this rule: "workspace", and each axis
	// its point is specific about.
	WidenTo []string `json:"widen_to"`
	// Recipe is the checkout a decision about this entry is made through,
	// empty when no readable checkout of the project is on this machine.
	Recipe string `json:"recipe,omitempty"`
}

// ContextFeedGroup is one session's work: an agent run, or a person's
// operations on one day.
type ContextFeedGroup struct {
	// ID addresses the group on screen and stays the same across refreshes.
	ID string `json:"id"`
	// Session is the agent session id, empty for a person's or a tool's work.
	Session string          `json:"session,omitempty"`
	Actor   ContextActorDTO `json:"actor"`
	// Day is the local date a person's or a tool's group covers, RFC3339 date
	// only. Empty for an agent session.
	Day string `json:"day,omitempty"`
	// ProjectKey and ProjectName name the project, empty when the group spans
	// more than one.
	ProjectKey  string `json:"project_key,omitempty"`
	ProjectName string `json:"project_name,omitempty"`
	// Recipe is the checkout decisions about this group go through.
	Recipe string `json:"recipe,omitempty"`
	// First and Last bound the group in time, RFC3339 in UTC.
	First string `json:"first"`
	Last  string `json:"last"`
	// Awaiting is how many of the group's suggestions await a decision.
	Awaiting int `json:"awaiting"`
	// Recorded, Corrected, Kept and Dropped are the counts a summary line
	// reads out.
	Recorded  int `json:"recorded"`
	Corrected int `json:"corrected"`
	Kept      int `json:"kept"`
	Dropped   int `json:"dropped"`
	// Quiet reports a group nothing has been added to for a while, which is
	// when its summary is a count of finished work.
	Quiet   bool               `json:"quiet"`
	Entries []ContextFeedEntry `json:"entries"`
}

// ContextAwaiting is how many suggestions one project has awaiting a decision.
type ContextAwaiting struct {
	ProjectKey  string `json:"project_key"`
	ProjectName string `json:"project_name,omitempty"`
	Count       int    `json:"count"`
}

// ContextFeed is what a person and the agents beside them have recorded.
type ContextFeed struct {
	// ProjectKey is the project the feed was narrowed to, empty for the whole
	// workspace.
	ProjectKey string `json:"project_key,omitempty"`
	// Groups are the sessions, newest first.
	Groups []ContextFeedGroup `json:"groups"`
	// Awaiting is the per-project count of suggestions awaiting a decision,
	// over the whole workspace whatever the feed was narrowed to, so the home
	// screen can show a count beside every project from one read.
	Awaiting []ContextAwaiting `json:"awaiting"`
	// AwaitingTotal sums Awaiting over the workspace.
	AwaitingTotal int `json:"awaiting_total"`
	// AwaitingHere counts the suggestions awaiting a decision in what this feed
	// shows, which is the whole workspace when it was not narrowed.
	AwaitingHere int `json:"awaiting_here"`
	// Truncated reports that the limit cut the feed short.
	Truncated bool `json:"truncated"`
	// ReadOnly reports a workspace that answers reads and refuses decisions.
	ReadOnly bool `json:"read_only"`
}

// ContextDecisionRequest is one decision about one operation.
type ContextDecisionRequest struct {
	// Project is the project's workspace key, which the feed carries on every
	// entry.
	Project string `json:"project"`
	// ID is the operation being decided.
	ID string `json:"id"`
	// Replacement and Advisory edit the rule as it is kept. Empty (nil)
	// leaves the rule as suggested.
	Replacement string `json:"replacement,omitempty"`
	Advisory    *bool  `json:"advisory,omitempty"`
	// WidenTo widens the rule in the same step: "workspace", or an axis name
	// the rule stops being specific about.
	WidenTo string `json:"widen_to,omitempty"`
	// Note is whatever the person wants to record about the decision.
	Note string `json:"note,omitempty"`
}

// ContextRevertRequest undoes one operation or a whole session.
type ContextRevertRequest struct {
	Project string `json:"project"`
	// ID names one operation. Session names every operation one agent session
	// recorded. Exactly one is set.
	ID      string `json:"id,omitempty"`
	Session string `json:"session,omitempty"`
	Note    string `json:"note,omitempty"`
}

// ContextRevertSummary is what a revert undid, for the confirmation a person
// reads before asking for it and the report afterwards.
type ContextRevertSummary struct {
	// Session is the session reverted, empty when one operation was named.
	Session string `json:"session,omitempty"`
	// Operations is how many operations stop answering.
	Operations int `json:"operations"`
	// Rules names the rules taken back out of the project's stores.
	Rules []string `json:"rules"`
	// Subjects is the one-line description of each operation, so the
	// confirmation can name what goes.
	Subjects []string `json:"subjects"`
}

// ContextWidenTarget is one project a widened rule would answer in.
type ContextWidenTarget struct {
	ProjectKey  string `json:"project_key"`
	ProjectName string `json:"project_name,omitempty"`
	// Current is true for the project the rule already answers in.
	Current bool `json:"current"`
	// CheckedOut reports a copy of the project's files on this machine.
	CheckedOut bool `json:"checked_out"`
}

// ContextWidenPoint is one point in the project's recipe a widened rule would
// newly answer at.
type ContextWidenPoint struct {
	// Ref addresses the point the way a collection names it.
	Ref         string            `json:"ref"`
	Label       string            `json:"label"`
	Coordinates map[string]string `json:"coordinates,omitempty"`
	// Collections sit exactly here.
	Collections []string `json:"collections"`
}

// ContextWidenPreview is what widening a rule would change, read before a
// person accepts it.
type ContextWidenPreview struct {
	// To is the widening asked for: "workspace", or the axis dropped.
	To string `json:"to"`
	// From and Scope are the rule's point now and after.
	From  ContextScopeDTO `json:"from"`
	Scope ContextScopeDTO `json:"scope"`
	// Rule is the rule itself, so the preview names what would reach further.
	Rule ContextSubjectDTO `json:"rule"`
	// Projects are the workspace's projects the widened rule answers in,
	// listed for a widening to the workspace.
	Projects []ContextWidenTarget `json:"projects"`
	// Points are the recipe's declared points the widened rule newly covers,
	// listed for a widening past one axis.
	Points []ContextWidenPoint `json:"points"`
	// ContentImpact is false: which files hold the term, and how many times,
	// is not computed here. The frontend says so in as many words rather than
	// letting a reader read reach as impact.
	ContentImpact bool `json:"content_impact"`
}

// ContextFeed reads the workspace's context operations, newest first, grouped
// by session. An empty projectKey reads every project.
func (a *App) ContextFeed(projectKey string, limit int) (*ContextFeed, error) {
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()
	return a.contextFeed(ctx, workspace.ProjectKey(projectKey), limit)
}

// ProjectContextFeed reads the context operations of the project a tab holds.
func (a *App) ProjectContextFeed(tabID string, limit int) (*ContextFeed, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.workspaceKey == "" {
		return nil, errors.New("this tab holds no project the workspace knows")
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()
	return a.contextFeed(ctx, op.workspaceKey, limit)
}

// contextFeed folds the log once and builds both the feed and the per-project
// counts from that one read.
func (a *App) contextFeed(ctx context.Context, key workspace.ProjectKey, limit int) (*ContextFeed, error) {
	if limit <= 0 {
		limit = contextFeedDefaultLimit
	}
	ws, err := a.hostEngine().Workspace(ctx)
	if err != nil {
		return nil, err
	}
	registry, err := a.contextProjectIndex(ctx, ws)
	if err != nil {
		return nil, err
	}
	ledger := contextop.NewLedger(ws, contextop.PersonDecides)

	// The counts cover the workspace whatever the feed shows, so the home
	// screen reads every project's badge from one call.
	all, err := ledger.Records(ctx, contextop.Filter{})
	if err != nil {
		return nil, err
	}

	out := &ContextFeed{
		ProjectKey: string(key),
		Groups:     []ContextFeedGroup{},
		Awaiting:   []ContextAwaiting{},
		ReadOnly:   ws.Describe().ReadOnly,
	}
	awaiting := map[workspace.ProjectKey]int{}
	shown := make([]contextop.Record, 0, len(all))
	for _, r := range all {
		if decidable(r) {
			awaiting[r.Project]++
		}
		if key != "" && r.Project != key {
			continue
		}
		if decidable(r) {
			out.AwaitingHere++
		}
		shown = append(shown, r)
	}
	if len(shown) > limit {
		shown, out.Truncated = shown[:limit], true
	}
	out.Groups = groupContextFeed(shown, registry, a.hostEngine().GovernanceInstant())
	for _, count := range sortedAwaiting(awaiting) {
		count.ProjectName = registry.name(workspace.ProjectKey(count.ProjectKey))
		out.Awaiting = append(out.Awaiting, count)
		out.AwaitingTotal += count.Count
	}
	return out, nil
}

// ContextAwaitingCounts reports how many suggestions each project has awaiting a
// decision, for a home screen that shows the badge without reading the feed.
func (a *App) ContextAwaitingCounts() ([]ContextAwaiting, error) {
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()

	ws, err := a.hostEngine().Workspace(ctx)
	if err != nil {
		return nil, err
	}
	registry, err := a.contextProjectIndex(ctx, ws)
	if err != nil {
		return nil, err
	}
	records, err := contextop.NewLedger(ws, contextop.PersonDecides).Records(ctx, contextop.Filter{})
	if err != nil {
		return nil, err
	}
	awaiting := map[workspace.ProjectKey]int{}
	for _, r := range records {
		if decidable(r) {
			awaiting[r.Project]++
		}
	}
	out := sortedAwaiting(awaiting)
	for i := range out {
		out[i].ProjectName = registry.name(workspace.ProjectKey(out[i].ProjectKey))
	}
	return out, nil
}

// KeepContextSuggestion establishes a suggestion, with whatever edit and
// whatever widening the person asked for in the same step.
func (a *App) KeepContextSuggestion(req ContextDecisionRequest) (*ContextFeedEntry, error) {
	recipe, err := a.contextRecipeFor(req.Project)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()

	written, err := a.hostEngine().KeepContextOperation(ctx, host.ContextKeepRequest{
		Actor:       deskPerson(),
		Project:     recipe,
		ID:          req.ID,
		Replacement: req.Replacement,
		Advisory:    req.Advisory,
		WidenTo:     req.WidenTo,
		Note:        req.Note,
	})
	if err != nil {
		return nil, err
	}
	a.emitEvent("workspace:changed", nil)
	entry := contextFeedEntry(written.Record, "", recipe)
	return &entry, nil
}

// DropContextSuggestion sets a suggestion aside. It stops answering at once.
func (a *App) DropContextSuggestion(req ContextDecisionRequest) (*ContextFeedEntry, error) {
	recipe, err := a.contextRecipeFor(req.Project)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()

	written, err := a.hostEngine().DropContextOperation(ctx, host.ContextDropRequest{
		Actor:   deskPerson(),
		Project: recipe,
		ID:      req.ID,
		Note:    req.Note,
	})
	if err != nil {
		return nil, err
	}
	a.emitEvent("workspace:changed", nil)
	entry := contextFeedEntry(written.Record, "", recipe)
	return &entry, nil
}

// RevertContextOperations undoes one operation or a whole session, taking
// whatever it put in force back out of the project's stores.
func (a *App) RevertContextOperations(req ContextRevertRequest) (*ContextRevertSummary, error) {
	recipe, err := a.contextRecipeFor(req.Project)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()

	result, err := a.hostEngine().RevertContextOperations(ctx, host.ContextRevertRequest{
		Actor:   deskPerson(),
		Project: recipe,
		ID:      req.ID,
		Session: req.Session,
		Note:    req.Note,
	})
	if err != nil {
		return nil, err
	}
	a.emitEvent("workspace:changed", nil)
	out := &ContextRevertSummary{
		Session:    result.Session,
		Operations: len(result.Reverted),
		Rules:      append([]string{}, result.Retracted...),
		Subjects:   []string{},
	}
	for _, r := range result.Reverted {
		if line := r.Subject.Describe(); line != "" {
			out.Subjects = append(out.Subjects, line)
		}
	}
	if out.Rules == nil {
		out.Rules = []string{}
	}
	return out, nil
}

// ContextRevertScope reports what reverting a session would undo, for the
// confirmation a person reads first. It records nothing.
func (a *App) ContextRevertScope(req ContextRevertRequest) (*ContextRevertSummary, error) {
	if (req.ID == "") == (req.Session == "") {
		return nil, errors.New("name an operation or a session, not both and not neither")
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()

	ws, err := a.hostEngine().Workspace(ctx)
	if err != nil {
		return nil, err
	}
	ledger := contextop.NewLedger(ws, contextop.PersonDecides)
	out := &ContextRevertSummary{Session: req.Session, Rules: []string{}, Subjects: []string{}}

	var targets []contextop.Record
	if req.ID != "" {
		target, terr := ledger.Subject(ctx, req.ID)
		if terr != nil {
			return nil, terr
		}
		targets = []contextop.Record{target}
	} else {
		held, herr := ledger.Records(ctx, contextop.Filter{Session: req.Session, Subjects: true})
		if herr != nil {
			return nil, herr
		}
		targets = held
	}
	for _, target := range targets {
		out.Operations++
		if line := target.Subject.Describe(); line != "" {
			out.Subjects = append(out.Subjects, line)
		}
		if target.Established {
			out.Rules = append(out.Rules, target.Subject.Describe())
		}
	}
	return out, nil
}

// WidenContextRule moves an established rule to a broader point.
func (a *App) WidenContextRule(req ContextDecisionRequest) (*ContextFeedEntry, error) {
	recipe, err := a.contextRecipeFor(req.Project)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()

	written, err := a.hostEngine().WidenContextOperation(ctx, host.ContextWidenRequest{
		Actor:   deskPerson(),
		Project: recipe,
		ID:      req.ID,
		To:      req.WidenTo,
		Note:    req.Note,
	})
	if err != nil {
		return nil, err
	}
	a.emitEvent("workspace:changed", nil)
	entry := contextFeedEntry(written.Record, "", recipe)
	return &entry, nil
}

// ContextWidenReach reports where a rule would answer once widened: the
// workspace's projects for a widening to the workspace, and the recipe's
// declared points the rule newly covers for a widening past one axis.
//
// It reports reach and not impact. Which files hold the term, and how many
// times, would have to be read out of the content, and the host API computes
// no such preview, so the surface says as much rather than implying the two
// are the same.
func (a *App) ContextWidenReach(projectKey, id, to string) (*ContextWidenPreview, error) {
	if to == "" {
		return nil, errors.New("name what to widen to")
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()

	ws, err := a.hostEngine().Workspace(ctx)
	if err != nil {
		return nil, err
	}
	target, err := contextop.NewLedger(ws, contextop.PersonDecides).Subject(ctx, id)
	if err != nil {
		return nil, err
	}
	widened, err := widenedScope(target.Scope, to)
	if err != nil {
		return nil, err
	}
	out := &ContextWidenPreview{
		To:       to,
		From:     scopeDTO(target.Scope),
		Scope:    scopeDTO(widened),
		Rule:     subjectDTO(target.Subject),
		Projects: []ContextWidenTarget{},
		Points:   []ContextWidenPoint{},
	}
	if projectKey == "" {
		projectKey = string(target.Project)
	}

	if to == host.WidenToWorkspace {
		registrations, rerr := ws.Projects(ctx)
		if rerr != nil {
			return nil, rerr
		}
		for _, reg := range registrations {
			out.Projects = append(out.Projects, ContextWidenTarget{
				ProjectKey:  string(reg.Key),
				ProjectName: workspaceDisplayName(reg),
				Current:     reg.Key == target.Project,
				CheckedOut:  len(liveCheckouts(reg)) > 0,
			})
		}
		return out, nil
	}

	recipe, err := a.contextRecipeFor(projectKey)
	if err != nil {
		// The rule's own project has no checkout here, so the recipe's points
		// cannot be read. The scope change is still worth showing.
		return out, nil
	}
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, fmt.Errorf("read the recipe to see where the rule would reach: %w", err)
	}
	out.Points = newlyCoveredPoints(proj, target.Scope, widened, a.hostEngine().GovernanceInstant())
	return out, nil
}

// newlyCoveredPoints lists the recipe's declared points the widened scope
// covers and the narrow one does not.
func newlyCoveredPoints(proj *project.KapiProject, from, to contextop.Scope, at time.Time) []ContextWidenPoint {
	byRef := collectionsByPoint(proj, at)
	out := []ContextWidenPoint{}
	for _, ref := range declaredPointRefs(proj) {
		coordinates := project.MergeCoordinates(proj.Defaults.Coordinates, ref.Coordinates(), nil)
		if from.Covers(coordinates) || !to.Covers(coordinates) {
			continue
		}
		collections := byRef[ref.String()]
		if collections == nil {
			collections = []string{}
		}
		out = append(out, ContextWidenPoint{
			Ref:         ref.String(),
			Label:       pointRefLabel(ref),
			Coordinates: coordinates,
			Collections: collections,
		})
	}
	return out
}

// declaredPointRefs is every point the recipe declares: the project's own, and
// each profile's channels.
func declaredPointRefs(proj *project.KapiProject) []project.ChannelRef {
	refs := []project.ChannelRef{{}}
	for _, name := range sortedKeys(proj.Profiles) {
		channels := proj.Profiles[name].Channels
		if len(channels) == 0 {
			refs = append(refs, project.ChannelRef{Profile: name})
			continue
		}
		for _, ch := range channels {
			if ch.ID == "" {
				continue
			}
			refs = append(refs, project.ChannelRef{Profile: name, Channel: ch.ID})
		}
	}
	return refs
}

// widenedScope is the scope a rule answers at once widened. It mirrors what
// host.WidenContextOperation will do, for a preview that has to show the
// result before anything is recorded.
func widenedScope(scope contextop.Scope, to string) (contextop.Scope, error) {
	if to == host.WidenToWorkspace {
		scope.Level = contextop.LevelWorkspace
		return scope, nil
	}
	if _, ok := scope.Coordinates[to]; !ok {
		return contextop.Scope{}, fmt.Errorf("this rule sits at no %q, so there is nothing to widen past (its point is %s)", to, scope.Describe())
	}
	widened := make(map[string]string, len(scope.Coordinates))
	for axis, value := range scope.Coordinates {
		if axis != to {
			widened[axis] = value
		}
	}
	scope.Coordinates = widened
	return scope, nil
}

// contextProjectIndex reads the registry once, so the feed can name a project
// and find the checkout a decision goes through without a lookup per entry.
type contextProjectIndex struct {
	names   map[workspace.ProjectKey]string
	recipes map[workspace.ProjectKey]string
}

func (a *App) contextProjectIndex(ctx context.Context, ws *workspace.Workspace) (*contextProjectIndex, error) {
	regs, err := ws.Projects(ctx)
	if err != nil {
		return nil, err
	}
	out := &contextProjectIndex{
		names:   make(map[workspace.ProjectKey]string, len(regs)),
		recipes: make(map[workspace.ProjectKey]string, len(regs)),
	}
	for _, reg := range regs {
		out.names[reg.Key] = workspaceDisplayName(reg)
		if live := liveCheckouts(reg); len(live) > 0 {
			out.recipes[reg.Key] = filepath.Join(live[0], project.RecipeFileName)
		}
	}
	return out, nil
}

func (i *contextProjectIndex) name(key workspace.ProjectKey) string   { return i.names[key] }
func (i *contextProjectIndex) recipe(key workspace.ProjectKey) string { return i.recipes[key] }

// liveCheckouts are the directories of a registration that still hold a
// readable recipe.
func liveCheckouts(reg workspace.Registration) []string {
	var out []string
	for _, dir := range reg.Checkouts {
		if available, _ := recipeStatus(filepath.Join(dir, project.RecipeFileName)); available {
			out = append(out, dir)
		}
	}
	return out
}

// contextRecipeFor is the recipe a decision about one project goes through.
//
// Every context operation names a project by its workspace key, and the host
// API resolves a project from a recipe path, so a decision needs a checkout on
// this machine. A project registered from a machine that no longer has it can
// be read here and not decided on.
func (a *App) contextRecipeFor(key string) (string, error) {
	if key == "" {
		return "", errors.New("name the project the operation belongs to")
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()

	ws, err := a.hostEngine().Workspace(ctx)
	if err != nil {
		return "", err
	}
	reg, ok, err := ws.Lookup(ctx, workspace.ProjectKey(key))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("this workspace holds no project %q", key)
	}
	live := liveCheckouts(reg)
	if len(live) == 0 {
		return "", fmt.Errorf("no copy of %s is on this machine, so a decision cannot be written into its files",
			workspaceDisplayName(reg))
	}
	return filepath.Join(live[0], project.RecipeFileName), nil
}

// deskPerson is who the desktop records as. The app holds no account, and the
// person at the keyboard is the one acting, which is what an actor with no
// name means.
func deskPerson() contextop.Actor {
	return contextop.Actor{Kind: contextop.ActorPerson}
}

// decidable reports a suggestion a person can keep or drop: one still
// awaiting a decision that states a rule. A note and a correction that
// suggested nothing are facts on the record with nothing to decide.
func decidable(r contextop.Record) bool {
	if !r.Status.Advises() || r.Established {
		return false
	}
	switch r.Subject.Kind {
	case contextop.SubjectTerm, contextop.SubjectMemory:
		return true
	}
	return false
}

// groupContextFeed collects the records into sessions, newest first.
//
// An agent session groups by the id its operations carry. A person at a
// terminal and a tool record no session, so their work groups by actor and
// local day, which is the span a person recognises as "what I did".
func groupContextFeed(records []contextop.Record, registry *contextProjectIndex, now time.Time) []ContextFeedGroup {
	order := []string{}
	groups := map[string]*ContextFeedGroup{}
	for _, r := range records {
		id, day := contextGroupKey(r)
		group, ok := groups[id]
		if !ok {
			group = &ContextFeedGroup{
				ID:          id,
				Session:     r.Actor.Session,
				Actor:       actorDTO(r.Actor),
				Day:         day,
				ProjectKey:  string(r.Project),
				ProjectName: registry.name(r.Project),
				Recipe:      registry.recipe(r.Project),
				Entries:     []ContextFeedEntry{},
			}
			groups[id] = group
			order = append(order, id)
		}
		if group.ProjectKey != string(r.Project) {
			// A session that worked in more than one project names none.
			group.ProjectKey, group.ProjectName, group.Recipe = "", "", ""
		}
		group.Entries = append(group.Entries, contextFeedEntry(r, registry.name(r.Project), registry.recipe(r.Project)))
		countInto(group, r)
	}

	out := make([]ContextFeedGroup, 0, len(order))
	for _, id := range order {
		group := groups[id]
		first, last := spanOf(group.Entries)
		group.First, group.Last = first, last
		if parsed, err := time.Parse(time.RFC3339, last); err == nil {
			group.Quiet = now.Sub(parsed) >= contextSessionQuietAfter
		}
		out = append(out, *group)
	}
	return out
}

// contextGroupKey is the group an operation belongs to, and the local day for
// a group that has one.
func contextGroupKey(r contextop.Record) (id, day string) {
	if r.Actor.Session != "" {
		return "session:" + r.Actor.Session, ""
	}
	day = r.At.Local().Format(time.DateOnly)
	kind := string(r.Actor.Kind)
	if kind == "" {
		kind = string(contextop.ActorPerson)
	}
	return "actor:" + kind + "/" + r.Actor.Name + ":" + day, day
}

// countInto adds one operation to a group's summary counts.
func countInto(group *ContextFeedGroup, r contextop.Record) {
	switch r.Kind {
	case contextop.KindObserve:
		group.Recorded++
	case contextop.KindCorrect:
		group.Corrected++
	case contextop.KindKeep:
		group.Kept++
	case contextop.KindDrop:
		group.Dropped++
	}
	if decidable(r) {
		group.Awaiting++
	}
}

// spanOf bounds a group in time. Entries arrive newest first.
func spanOf(entries []ContextFeedEntry) (first, last string) {
	if len(entries) == 0 {
		return "", ""
	}
	return entries[len(entries)-1].At, entries[0].At
}

// contextFeedEntry renders one record for the feed.
func contextFeedEntry(r contextop.Record, projectName, recipe string) ContextFeedEntry {
	out := ContextFeedEntry{
		ID:            r.ID,
		Seq:           r.Seq,
		ProjectKey:    string(r.Project),
		ProjectName:   projectName,
		Kind:          string(r.Kind),
		Status:        string(r.Status),
		ContestedBy:   append([]string{}, r.ContestedBy...),
		Actor:         actorDTO(r.Actor),
		Subject:       subjectDTO(r.Subject),
		Evidence:      []ContextEvidenceDTO{},
		Scope:         scopeDTO(r.Scope),
		Target:        r.Target,
		TargetSession: r.TargetSession,
		Note:          r.Note,
		At:            r.At.UTC().Format(time.RFC3339),
		Decidable:     decidable(r),
		WidenTo:       []string{},
		Recipe:        recipe,
	}
	if r.Correction != nil {
		out.Correction = &ContextCorrectionDTO{From: r.Correction.From, To: r.Correction.To}
	}
	for _, e := range r.Evidence {
		out.Evidence = append(out.Evidence, ContextEvidenceDTO{Path: e.Path, Unit: e.Unit, Quote: e.Quote})
	}
	if r.Established && r.Kind.Bears() {
		out.Revertible = true
		out.WidenTo = widenOptions(r.Scope)
	}
	return out
}

// widenOptions are the widenings open to a rule: the whole workspace, and each
// axis its point is specific about. A rule already answering workspace-wide
// has nowhere further to go.
func widenOptions(scope contextop.Scope) []string {
	out := []string{}
	if scope.Level != contextop.LevelWorkspace {
		out = append(out, host.WidenToWorkspace)
	}
	axes := make([]string, 0, len(scope.Coordinates))
	for axis := range scope.Coordinates {
		axes = append(axes, axis)
	}
	sort.Strings(axes)
	return append(out, axes...)
}

// actorDTO renders an actor, splitting a "<client>@<host>" name so a reader
// sees which machine an agent ran on.
func actorDTO(a contextop.Actor) ContextActorDTO {
	kind := string(a.Kind)
	if kind == "" {
		kind = string(contextop.ActorPerson)
	}
	out := ContextActorDTO{Kind: kind, Name: a.Name, Session: a.Session}
	if name, hostName, found := strings.Cut(a.Name, "@"); found && name != "" && hostName != "" {
		out.Name, out.Host = name, hostName
	}
	return out
}

// subjectDTO flattens a subject for the feed.
func subjectDTO(s contextop.Subject) ContextSubjectDTO {
	out := ContextSubjectDTO{Kind: string(s.Kind), Text: s.Text, Describe: s.Describe()}
	switch s.Kind {
	case contextop.SubjectTerm:
		if s.Term != nil {
			out.Term, out.Forms, out.Replacement, out.Advisory = s.Term.Term, s.Term.Forms, s.Term.Replacement, s.Term.Advisory
		}
	case contextop.SubjectMemory:
		if s.Memory != nil {
			out.Source, out.Target, out.TargetLocale = s.Memory.Source, s.Memory.Target, s.Memory.TargetLocale
		}
	}
	return out
}

// scopeDTO renders a scope, filling in the level the log leaves implicit.
func scopeDTO(s contextop.Scope) ContextScopeDTO {
	level := s.Level
	if level == "" {
		level = contextop.LevelProject
	}
	return ContextScopeDTO{Level: string(level), Coordinates: s.Coordinates, Describe: s.Describe()}
}

// sortedAwaiting orders the per-project counts by size, then by key, so the
// list is stable between reads.
func sortedAwaiting(counts map[workspace.ProjectKey]int) []ContextAwaiting {
	out := make([]ContextAwaiting, 0, len(counts))
	for key, count := range counts {
		out = append(out, ContextAwaiting{ProjectKey: string(key), Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].ProjectKey < out[j].ProjectKey
	})
	return out
}
