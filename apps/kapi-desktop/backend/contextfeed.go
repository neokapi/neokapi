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
// Reading and deciding both go through host.App, which folds the log, reports
// the status each subject-bearing operation ended up at, and owns the policy
// about who may keep, drop, widen and reset. Every call names the project by
// its workspace key, which the host resolves to a checkout on this machine
// when there is one and to the project's stores in the workspace otherwise,
// so a project registered from another machine is decided on here too. This
// file records no operation of its own.

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
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
	// ID names the operation; every action on it passes it back.
	ID string `json:"id"`
	// Short is the id as a person reads and types it, the form `kapi context
	// log` prints.
	Short string `json:"short"`
	// ProjectKey and ProjectName name the project whose work produced it.
	ProjectKey  string `json:"project_key"`
	ProjectName string `json:"project_name,omitempty"`
	// Kind is "observe", "correct", "import", "edit", "keep", "drop",
	// "withdraw", "reset", "widen", "signal" or "establish".
	Kind string `json:"kind"`
	// Status is "suggested", "established", "contested", "withdrawn",
	// "dropped" or "reset".
	Status string `json:"status"`
	// ContestedBy names the operations on the other side of a disagreement,
	// for a contested entry.
	ContestedBy []string              `json:"contested_by"`
	Actor       ContextActorDTO       `json:"actor"`
	Subject     ContextSubjectDTO     `json:"subject"`
	Correction  *ContextCorrectionDTO `json:"correction,omitempty"`
	Evidence    []ContextEvidenceDTO  `json:"evidence"`
	Scope       ContextScopeDTO       `json:"scope"`
	// Target is the operation this one acts on, and Before the first
	// operation a reset set aside.
	Target string `json:"target,omitempty"`
	Before string `json:"before,omitempty"`
	Note   string `json:"note,omitempty"`
	// At is when the log accepted it, RFC3339 in UTC.
	At string `json:"at"`
	// Standing is the evidence for and against a suggestion as plain counts:
	// "seen in 3 sessions · 14 of 15 uses in docs/ · merged in #412".
	Standing string `json:"standing,omitempty"`
	// Decidable reports a suggestion carrying a rule a person can keep or
	// drop. A note and a correction that suggested nothing are recorded facts
	// with nothing to decide. A contested suggestion is decidable too, and
	// keeping it waits until the other side is dropped.
	Decidable bool `json:"decidable"`
	// Droppable reports a suggestion or a rule in force a person can drop. A
	// rule in force is taken back out of the project's stores as it is
	// dropped.
	Droppable bool `json:"droppable"`
	// WidenTo are the widenings open to this rule: "workspace", and each axis
	// its point is specific about.
	WidenTo []string `json:"widen_to"`
	// Recipe is the checkout of the project on this machine, empty when none
	// is here. A decision needs none: it is recorded in the workspace, where
	// the project's terms and content memory live.
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
	// Recipe is the checkout of the project on this machine, when one is here.
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
	// Checked, Failing, Caught and Files are what the session's check runs
	// found (host.SessionOutcomes): how many runs the Stop hook recorded, the
	// failing findings its latest run left, the findings one run reported and
	// the next no longer found, and the files the runs name. A check run is
	// counted here and listed in no entry.
	Checked int `json:"checked"`
	Failing int `json:"failing"`
	Caught  int `json:"caught"`
	Files   int `json:"files"`
	// Clean reports a session at least one of whose check runs reached a
	// verdict, every one of them clean. Failed reports a session one of whose
	// runs reached a verdict with something failing. A session whose runs
	// reached no verdict reports neither.
	Clean  bool `json:"clean"`
	Failed bool `json:"failed"`
	// Quiet reports a group nothing has been added to for a while, which is
	// when its summary is a count of finished work.
	Quiet   bool               `json:"quiet"`
	Entries []ContextFeedEntry `json:"entries"`
	// Runs are the session's latest check runs, newest first, at most
	// contextFeedMaxRuns of them. Checked counts them all.
	Runs []ContextCheckRun `json:"runs"`
}

// contextFeedMaxRuns bounds the check runs a group lists, and
// contextRunMaxFiles and contextRunMaxFindings what one run lists. The feed
// is read on every change to the log, and a session's detail shows its latest
// runs; the counts stay exact.
const (
	contextFeedMaxRuns    = 20
	contextRunMaxFiles    = 20
	contextRunMaxFindings = 20
)

// ContextCheckRun is one run of a project's checks, as a session's detail
// lists it.
type ContextCheckRun struct {
	// ID names the check operation.
	ID string `json:"id"`
	// ProjectKey is the project the run checked.
	ProjectKey string `json:"project_key"`
	// At is when the log accepted the run, RFC3339 in UTC.
	At string `json:"at"`
	// Files are the files the session's turn edited, when the run knows them
	// (Edited), at most contextRunMaxFiles of them. FileCount counts the
	// edited files, or the files the check read when the run knows no edit.
	Files     []string `json:"files"`
	Edited    bool     `json:"edited"`
	FileCount int      `json:"file_count"`
	// Reached reports a run that reached a verdict. Clean reports one that
	// reached it with nothing failing.
	Reached bool `json:"reached"`
	Clean   bool `json:"clean"`
	// Failing counts the run's failing findings, and Rules are their rule ids,
	// sorted.
	Failing int      `json:"failing"`
	Rules   []string `json:"rules"`
	// Findings are the failing findings, at most contextRunMaxFindings of
	// them, those caught first when the run holds more.
	Findings []ContextCheckFinding `json:"findings"`
	// Caught counts the run's failing findings the session's next run no
	// longer found.
	Caught int `json:"caught"`
}

// ContextCheckFinding is one failing finding of a check run.
type ContextCheckFinding struct {
	File  string `json:"file,omitempty"`
	Rule  string `json:"rule"`
	Quote string `json:"quote,omitempty"`
	// Message is the finding's message, for a finding that quotes nothing.
	Message string `json:"message,omitempty"`
	// Caught reports a finding the session's next run no longer found on the
	// same file: the check reported it and the assistant fixed it.
	Caught bool `json:"caught"`
}

// ContextFeed is what a person and the agents beside them have recorded.
type ContextFeed struct {
	// ProjectKey is the project the feed was narrowed to, empty for the whole
	// workspace.
	ProjectKey string `json:"project_key,omitempty"`
	// Groups are the sessions, newest first.
	Groups []ContextFeedGroup `json:"groups"`
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

// ContextResetRequest rewinds a project's context to how it stood at a point
// in its history.
type ContextResetRequest struct {
	Project string `json:"project"`
	// Before names the point to go back to: an agent session id (the context
	// as it stood before the session's first operation), a date (2006-01-02)
	// or an instant (RFC 3339), or an operation id, such as an earlier
	// reset's.
	Before string `json:"before"`
	Note   string `json:"note,omitempty"`
}

// ContextResetSummary is what a reset sets aside, for the confirmation a
// person reads before asking for it and the report afterwards.
type ContextResetSummary struct {
	// Before is the point the context goes back to, as the request named it.
	Before string `json:"before"`
	// SetAside is how many suggestions and rules stop answering.
	SetAside int `json:"set_aside"`
	// Decisions is how many other operations are set aside with them: keeps,
	// drops, widenings and the rest of what acted on a rule after the point.
	Decisions int `json:"decisions"`
	// Restored is how many suggestions and rules an earlier reset set aside
	// answer again.
	Restored int `json:"restored"`
	// Subjects is the one-line description of each suggestion or rule set
	// aside, so the confirmation can name what goes.
	Subjects []string `json:"subjects"`
	// Rules names the rules in force that are taken back out of the
	// project's stores.
	Rules []string `json:"rules"`
	// Reset is the short id of the reset recorded, empty for a preview. A
	// later reset to before it brings back what it set aside.
	Reset string `json:"reset,omitempty"`
}

// ContextWidenTarget is one other project a widened rule would newly answer
// in.
type ContextWidenTarget struct {
	ProjectKey  string `json:"project_key"`
	ProjectName string `json:"project_name,omitempty"`
	// CheckedOut reports a copy of the project's files on this machine.
	CheckedOut bool `json:"checked_out"`
}

// ContextWidenPoint is one declared point a widened rule would newly answer
// at.
type ContextWidenPoint struct {
	ProjectKey string `json:"project_key"`
	// Ref addresses the point the way a collection names it.
	Ref         string            `json:"ref"`
	Label       string            `json:"label"`
	Coordinates map[string]string `json:"coordinates,omitempty"`
	// Collections sit exactly here.
	Collections []string `json:"collections"`
}

// ContextWidenUnit is one unit a widened rule would newly match.
type ContextWidenUnit struct {
	ProjectKey string `json:"project_key"`
	// Document is the file, relative to the project root, and Unit the block
	// in it.
	Document string `json:"document"`
	Unit     string `json:"unit"`
	// Text is the unit's source text, cut to an excerpt.
	Text string `json:"text"`
	// Matches is how many times the text holds a form the rule avoids.
	Matches int `json:"matches"`
}

// ContextWidenExamined is one project whose projection the preview read.
type ContextWidenExamined struct {
	ProjectKey  string `json:"project_key"`
	ProjectName string `json:"project_name,omitempty"`
	// Units is how many units the projection holds and Matched how many the
	// widened rule would newly match.
	Units   int `json:"units"`
	Matched int `json:"matched"`
}

// ContextWidenGap is one project the preview could not read units from.
type ContextWidenGap struct {
	ProjectKey  string `json:"project_key"`
	ProjectName string `json:"project_name,omitempty"`
	// Reason says why: its files are not on this machine, no projection of
	// its content is built here, or the rule names no wording to look for.
	Reason string `json:"reason"`
}

// ContextWidenCoverage says which projects the preview read units from and
// which it did not, so the dialog states what was counted and what was not.
type ContextWidenCoverage struct {
	Examined    []ContextWidenExamined `json:"examined"`
	NotExamined []ContextWidenGap      `json:"not_examined"`
	// Truncated reports that Units holds the first of more; Examined carries
	// the full counts.
	Truncated bool `json:"truncated"`
}

// ContextWidenPreview is what widening a rule would change, read before a
// person accepts it. It is host.ContextWidenPreview as the dialog shows it.
type ContextWidenPreview struct {
	// To is the widening asked for: "workspace", or the axis dropped.
	To string `json:"to"`
	// From and Scope are the rule's point now and after.
	From  ContextScopeDTO `json:"from"`
	Scope ContextScopeDTO `json:"scope"`
	// Rule is the rule itself, so the preview names what would reach further.
	Rule ContextSubjectDTO `json:"rule"`
	// Projects are the other projects of the workspace the widened rule would
	// newly answer in, listed for a widening to the workspace.
	Projects []ContextWidenTarget `json:"projects"`
	// Points are the declared points the widened rule newly covers, in every
	// project whose recipe is on this machine.
	Points []ContextWidenPoint `json:"points"`
	// Units are the units the rule would newly match, in every project whose
	// projection is built on this machine.
	Units []ContextWidenUnit `json:"units"`
	// Coverage says what the units were read from and what they were not.
	Coverage ContextWidenCoverage `json:"coverage"`
}

// ContextFeed reads the workspace's context operations, newest first, grouped
// by session. An empty projectKey reads every project.
func (a *App) ContextFeed(projectKey string, limit int) (*ContextFeed, error) {
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()
	return a.contextFeed(ctx, workspace.ProjectKey(projectKey), limit)
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
	// One more than the limit, so a cut feed says so. Check runs are read
	// apart: one is recorded for every turn an assistant ends, and they are
	// counted into their session rather than listed.
	log, err := a.hostEngine().ContextOperations(ctx, host.ContextLogRequest{
		Project:     string(key),
		AllProjects: key == "",
		Kinds:       feedKinds(),
		Limit:       limit + 1,
	})
	if err != nil {
		return nil, err
	}
	runs, err := a.hostEngine().ContextOperations(ctx, host.ContextLogRequest{
		Project:     string(key),
		AllProjects: key == "",
		Kinds:       []contextop.Kind{contextop.KindCheck},
	})
	if err != nil {
		return nil, err
	}

	out := &ContextFeed{
		ProjectKey: string(key),
		Groups:     []ContextFeedGroup{},
		ReadOnly:   ws.Describe().ReadOnly,
	}
	shown := make([]contextop.Record, 0, len(log.Operations))
	for _, op := range log.Operations {
		shown = append(shown, op.Record)
	}
	// A cut feed shows the operations from its oldest entry on, and a
	// session that only ran checks before then is cut with them.
	var since time.Time
	if len(shown) > limit {
		shown, out.Truncated = shown[:limit], true
		since = shown[len(shown)-1].At
	}
	checks := make([]contextop.Record, 0, len(runs.Operations))
	for _, op := range runs.Operations {
		checks = append(checks, op.Record)
	}
	out.Groups = groupContextFeed(shown, checks, registry, a.hostEngine().GovernanceInstant(), since)
	return out, nil
}

// feedKinds is every operation kind the feed lists as an entry.
func feedKinds() []contextop.Kind {
	out := make([]contextop.Kind, 0, len(contextop.Kinds))
	for _, k := range contextop.Kinds {
		if k != contextop.KindCheck {
			out = append(out, k)
		}
	}
	return out
}

// KeepContextSuggestion establishes a suggestion, with whatever edit and
// whatever widening the person asked for in the same step.
func (a *App) KeepContextSuggestion(req ContextDecisionRequest) (*ContextFeedEntry, error) {
	project, err := contextProjectKey(req.Project)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()

	// Keeping a suggestion is the first act that gives a folder opened
	// without a recipe its kapi.yaml.
	if err := a.ensureRecipeForKey(workspace.ProjectKey(project)); err != nil {
		return nil, err
	}
	written, err := a.hostEngine().KeepContextOperation(ctx, host.ContextKeepRequest{
		Actor:       deskPerson(),
		Project:     project,
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
	return a.decidedEntry(ctx, written, project)
}

// DropContextSuggestion sets a suggestion aside. It stops answering at once.
func (a *App) DropContextSuggestion(req ContextDecisionRequest) (*ContextFeedEntry, error) {
	project, err := contextProjectKey(req.Project)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()

	written, err := a.hostEngine().DropContextOperation(ctx, host.ContextDropRequest{
		Actor:   deskPerson(),
		Project: project,
		ID:      req.ID,
		Note:    req.Note,
	})
	if err != nil {
		return nil, err
	}
	a.emitEvent("workspace:changed", nil)
	return a.decidedEntry(ctx, written, project)
}

// ResetContext rewinds the project's context to before the point the request
// names. What was recorded from that point on is set aside, stays in the log,
// and the project's stores are rebuilt without it.
func (a *App) ResetContext(req ContextResetRequest) (*ContextResetSummary, error) {
	project, err := contextProjectKey(req.Project)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()

	result, err := a.hostEngine().ResetContext(ctx, host.ContextResetRequest{
		Actor:   deskPerson(),
		Project: project,
		Before:  req.Before,
		Note:    req.Note,
	})
	if err != nil {
		return nil, err
	}
	a.emitEvent("workspace:changed", nil)
	return resetSummary(result), nil
}

// ContextResetScope reports what resetting to before the named point would
// set aside, for the confirmation a person reads first. It records nothing.
func (a *App) ContextResetScope(req ContextResetRequest) (*ContextResetSummary, error) {
	project, err := contextProjectKey(req.Project)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()

	result, err := a.hostEngine().ContextResetScope(ctx, host.ContextResetRequest{
		Actor:   deskPerson(),
		Project: project,
		Before:  req.Before,
		Note:    req.Note,
	})
	if err != nil {
		return nil, err
	}
	return resetSummary(result), nil
}

// resetSummary renders a reset, or its preview, for the confirmation.
func resetSummary(result host.ContextResetResult) *ContextResetSummary {
	out := &ContextResetSummary{
		Before:    result.Before,
		SetAside:  len(result.SetAside),
		Decisions: result.Decisions,
		Restored:  len(result.Restored),
		Subjects:  []string{},
		Rules:     []string{},
	}
	for _, op := range result.SetAside {
		line := op.Subject.Describe()
		if line == "" {
			continue
		}
		out.Subjects = append(out.Subjects, line)
		if op.Established {
			out.Rules = append(out.Rules, line)
		}
	}
	if result.Reset != nil {
		out.Reset = result.Reset.Short
	}
	return out
}

// WidenContextRule moves an established rule to a broader point.
func (a *App) WidenContextRule(req ContextDecisionRequest) (*ContextFeedEntry, error) {
	project, err := contextProjectKey(req.Project)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()

	if err := a.ensureRecipeForKey(workspace.ProjectKey(project)); err != nil {
		return nil, err
	}
	written, err := a.hostEngine().WidenContextOperation(ctx, host.ContextWidenRequest{
		Actor:   deskPerson(),
		Project: project,
		ID:      req.ID,
		To:      req.WidenTo,
		Note:    req.Note,
	})
	if err != nil {
		return nil, err
	}
	a.emitEvent("workspace:changed", nil)
	return a.decidedEntry(ctx, written, project)
}

// ContextWidenReach reports what a rule would newly govern once widened: the
// other projects for a widening to the workspace, the declared points it
// would newly cover, and the units it would newly match wherever a projection
// is built on this machine. The host computes it (host.PreviewContextWidening)
// and says which projects it could not read, so the dialog states what was
// counted and what was not.
func (a *App) ContextWidenReach(projectKey, id, to string) (*ContextWidenPreview, error) {
	project, err := contextProjectKey(projectKey)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextFeedTimeout)
	defer cancel()

	preview, err := a.hostEngine().PreviewContextWidening(ctx, host.ContextWidenPreviewRequest{
		Project: project,
		ID:      id,
		To:      to,
	})
	if err != nil {
		return nil, err
	}
	return widenPreviewDTO(preview), nil
}

// widenPreviewDTO renders a preview for the dialog.
func widenPreviewDTO(preview host.ContextWidenPreview) *ContextWidenPreview {
	out := &ContextWidenPreview{
		To:       preview.To,
		From:     scopeDTO(preview.From),
		Scope:    scopeDTO(preview.Scope),
		Rule:     subjectDTO(preview.Rule),
		Projects: make([]ContextWidenTarget, 0, len(preview.Projects)),
		Points:   make([]ContextWidenPoint, 0, len(preview.Points)),
		Units:    make([]ContextWidenUnit, 0, len(preview.Units)),
		Coverage: ContextWidenCoverage{
			Examined:    make([]ContextWidenExamined, 0, len(preview.Coverage.Examined)),
			NotExamined: make([]ContextWidenGap, 0, len(preview.Coverage.NotExamined)),
			Truncated:   preview.Coverage.Truncated,
		},
	}
	for _, p := range preview.Projects {
		out.Projects = append(out.Projects, ContextWidenTarget{ProjectKey: string(p.Key), ProjectName: p.Name, CheckedOut: p.CheckedOut})
	}
	for _, p := range preview.Points {
		out.Points = append(out.Points, ContextWidenPoint{
			ProjectKey:  string(p.Project),
			Ref:         p.Ref,
			Label:       p.Label,
			Coordinates: p.Coordinates,
			Collections: p.Collections,
		})
	}
	for _, u := range preview.Units {
		out.Units = append(out.Units, ContextWidenUnit{
			ProjectKey: string(u.Project),
			Document:   u.Document,
			Unit:       u.Unit,
			Text:       u.Text,
			Matches:    u.Matches,
		})
	}
	for _, e := range preview.Coverage.Examined {
		out.Coverage.Examined = append(out.Coverage.Examined, ContextWidenExamined{
			ProjectKey: string(e.Project), ProjectName: e.Name, Units: e.Units, Matched: e.Matched,
		})
	}
	for _, g := range preview.Coverage.NotExamined {
		out.Coverage.NotExamined = append(out.Coverage.NotExamined, ContextWidenGap{
			ProjectKey: string(g.Project), ProjectName: g.Name, Reason: g.Reason,
		})
	}
	return out
}

// decidedEntry renders the operation a decision recorded, named and placed
// the way the feed shows it.
func (a *App) decidedEntry(ctx context.Context, written host.ContextOperation, key string) (*ContextFeedEntry, error) {
	ws, err := a.hostEngine().Workspace(ctx)
	if err != nil {
		return nil, err
	}
	registry, err := a.contextProjectIndex(ctx, ws)
	if err != nil {
		return nil, err
	}
	pk := workspace.ProjectKey(key)
	entry := contextFeedEntry(written.Record, registry.name(pk), registry.recipe(pk))
	return &entry, nil
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

// contextProjectKey checks that a decision names its project. The host
// resolves the key to a checkout on this machine when one is here, and to the
// project's stores in the workspace otherwise. An empty key would resolve a
// project from the working directory, which is no project of the desktop's.
func contextProjectKey(key string) (string, error) {
	if key == "" {
		return "", errors.New("name the project the operation belongs to")
	}
	return key, nil
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
//
// checks are the check runs, whose outcomes are counted into the session
// that recorded them. A session that only ran checks is a group with no
// entries and the span of its runs, unless its last run is older than since:
// a feed the limit cut short shows nothing before its oldest entry. A zero
// since admits every session.
func groupContextFeed(records, checks []contextop.Record, registry *contextProjectIndex, now, since time.Time) []ContextFeedGroup {
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

	// A session's span covers its entries and its check runs, so a session
	// still running checks after its last note is neither quiet nor sorted
	// below a session that ran checks since that note.
	runs := checkSessionsOf(checks)
	for _, id := range order {
		group := groups[id]
		group.First, group.Last = spanOf(group.Entries)
		if r, ok := runs.sessions[group.Session]; ok && group.Session != "" {
			if first := r.first.UTC().Format(time.RFC3339); first < group.First {
				group.First = first
			}
			if last := r.last.UTC().Format(time.RFC3339); last > group.Last {
				group.Last = last
			}
		}
	}

	// A session that only ran checks is a group of its own, with no entries.
	// So is a session whose entries the limit cut, when it ran a check since
	// the oldest entry shown.
	for _, session := range runs.order {
		id := "session:" + session
		if _, ok := groups[id]; ok {
			continue
		}
		r := runs.sessions[session]
		if !since.IsZero() && r.last.Before(since) {
			continue
		}
		group := &ContextFeedGroup{
			ID:      id,
			Session: session,
			Actor:   actorDTO(r.actor),
			First:   r.first.UTC().Format(time.RFC3339),
			Last:    r.last.UTC().Format(time.RFC3339),
			Entries: []ContextFeedEntry{},
		}
		if len(r.projects) == 1 {
			group.ProjectKey = string(r.projects[0])
			group.ProjectName = registry.name(r.projects[0])
			group.Recipe = registry.recipe(r.projects[0])
		}
		groups[id] = group
		order = append(order, id)
	}

	outcomes := host.SessionOutcomesOf(checks)
	listed := sessionRunsOf(checks)
	out := make([]ContextFeedGroup, 0, len(order))
	for _, id := range order {
		group := groups[id]
		group.Runs = []ContextCheckRun{}
		if group.Session != "" {
			if r := listed[group.Session]; r != nil {
				group.Runs = r
			}
			if o, ok := outcomes[group.Session]; ok {
				group.Checked, group.Failing, group.Caught, group.Files = o.Checked, o.Failing, o.Caught, o.Files
			}
			if r, ok := runs.sessions[group.Session]; ok {
				group.Clean, group.Failed = r.reached && !r.failed, r.failed
			}
		}
		if parsed, err := time.Parse(time.RFC3339, group.Last); err == nil {
			group.Quiet = now.Sub(parsed) >= host.ContextSessionQuietAfter
		}
		out = append(out, *group)
	}
	// Groups with entries arrive newest first; a session that only ran
	// checks takes its place among them by its last run.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Last > out[j].Last })
	return out
}

// checkSessions is what the feed reads from check runs beside their
// outcomes: who ran them, in which projects, when, and the verdicts they
// reached.
type checkSessions struct {
	// order is the sessions, the newest last run first.
	order    []string
	sessions map[string]*checkSession
}

// checkSession is one session's check runs, summarised.
type checkSession struct {
	actor       contextop.Actor
	projects    []workspace.ProjectKey
	first, last time.Time
	// reached reports a run that reached a verdict; failed, one that reached
	// a verdict with something failing.
	reached, failed bool
}

// checkSessionsOf summarises the check runs by session. A run recorded with
// no session belongs to none.
func checkSessionsOf(checks []contextop.Record) checkSessions {
	out := checkSessions{sessions: map[string]*checkSession{}}
	for _, r := range checks {
		if r.Kind != contextop.KindCheck || r.Check == nil || r.Actor.Session == "" {
			continue
		}
		s, ok := out.sessions[r.Actor.Session]
		if !ok {
			s = &checkSession{actor: r.Actor, first: r.At, last: r.At}
			out.sessions[r.Actor.Session] = s
			out.order = append(out.order, r.Actor.Session)
		}
		if r.At.Before(s.first) {
			s.first = r.At
		}
		if r.At.After(s.last) {
			s.last, s.actor = r.At, r.Actor
		}
		if !slices.Contains(s.projects, r.Project) {
			s.projects = append(s.projects, r.Project)
		}
		if r.Check.Reached() {
			s.reached = true
			s.failed = s.failed || !r.Check.Clean
		}
	}
	sort.SliceStable(out.order, func(i, j int) bool {
		return out.sessions[out.order[i]].last.After(out.sessions[out.order[j]].last)
	})
	return out
}

// sessionRunsOf lists each session's latest check runs, newest first, with
// what each run caught marked on its findings.
//
// The rule is host.SessionOutcomesOf's, applied finding by finding. Runs are
// compared within one project and one session, in the order the log accepted
// them, and only runs that reached a verdict take part. A failing finding of
// one run that the next such run no longer reports, by its file, its rule and
// the wording it quotes, was caught; when the next run lists fewer findings
// than it counted, nothing is caught. The mark sits on the run that last
// reported the finding, so the run that caught it shows what it caught.
func sessionRunsOf(checks []contextop.Record) map[string][]ContextCheckRun {
	sequences := map[string][]contextop.Record{}
	for _, r := range checks {
		if r.Kind != contextop.KindCheck || r.Check == nil || r.Actor.Session == "" {
			continue
		}
		key := string(r.Project) + "\x00" + r.Actor.Session
		sequences[key] = append(sequences[key], r)
	}
	out := map[string][]ContextCheckRun{}
	for _, seq := range sequences {
		sort.SliceStable(seq, func(i, j int) bool { return seq[i].ID < seq[j].ID })
		caught := make([][]bool, len(seq))
		prev := -1
		for i, r := range seq {
			if !r.Check.Reached() {
				continue
			}
			if prev >= 0 {
				caught[prev] = caughtFindings(*seq[prev].Check, *r.Check)
			}
			prev = i
		}
		session := seq[0].Actor.Session
		for i, r := range seq {
			out[session] = append(out[session], checkRunDTO(r, caught[i]))
		}
	}
	for session, runs := range out {
		sort.SliceStable(runs, func(i, j int) bool {
			if runs[i].At != runs[j].At {
				return runs[i].At > runs[j].At
			}
			return runs[i].ID > runs[j].ID
		})
		if len(runs) > contextFeedMaxRuns {
			runs = runs[:contextFeedMaxRuns]
		}
		out[session] = runs
	}
	return out
}

// caughtFindings marks which of prev's listed findings next no longer
// reports. Two findings with one key match one to one, so a finding reported
// twice and fixed once is caught once.
func caughtFindings(prev, next contextop.CheckRun) []bool {
	if !next.Complete() {
		return nil
	}
	remaining := map[string]int{}
	for _, f := range next.Findings {
		remaining[f.Key()]++
	}
	out := make([]bool, len(prev.Findings))
	for i, f := range prev.Findings {
		if remaining[f.Key()] > 0 {
			remaining[f.Key()]--
			continue
		}
		out[i] = true
	}
	return out
}

// checkRunDTO renders one check run, with caught marking its findings by
// position.
func checkRunDTO(r contextop.Record, caught []bool) ContextCheckRun {
	run := *r.Check
	out := ContextCheckRun{
		ID:         r.ID,
		ProjectKey: string(r.Project),
		At:         r.At.UTC().Format(time.RFC3339),
		Files:      []string{},
		Edited:     run.Edited,
		FileCount:  run.Covered,
		Reached:    run.Reached(),
		Clean:      run.Clean,
		Failing:    run.Failing,
		Rules:      append([]string{}, run.Rules...),
		Findings:   []ContextCheckFinding{},
	}
	if run.Edited {
		out.FileCount = len(run.Files)
		out.Files = append(out.Files, run.Files[:min(len(run.Files), contextRunMaxFiles)]...)
	}
	// Every caught finding is listed before any other is, so a run with more
	// findings than it lists still shows what it caught; the list keeps the
	// run's order.
	room := contextRunMaxFindings
	for _, c := range caught {
		if c {
			out.Caught++
		}
	}
	room -= min(out.Caught, room)
	for i, f := range run.Findings {
		isCaught := i < len(caught) && caught[i]
		if !isCaught {
			if room == 0 {
				continue
			}
			room--
		}
		if len(out.Findings) == contextRunMaxFindings {
			break
		}
		out.Findings = append(out.Findings, ContextCheckFinding{File: f.File, Rule: f.Rule, Quote: f.Quote, Message: f.Message, Caught: isCaught})
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
		ID:          r.ID,
		Short:       contextop.ShortID(r.ID),
		ProjectKey:  string(r.Project),
		ProjectName: projectName,
		Kind:        string(r.Kind),
		Status:      string(r.Status),
		ContestedBy: contextop.ShortIDs(r.ContestedBy),
		Actor:       actorDTO(r.Actor),
		Subject:     subjectDTO(r.Subject),
		Evidence:    []ContextEvidenceDTO{},
		Scope:       scopeDTO(r.Scope),
		Target:      contextop.ShortID(r.Target),
		Before:      contextop.ShortID(r.Before),
		Note:        r.Note,
		At:          r.At.UTC().Format(time.RFC3339),
		Standing:    r.Standing.Describe(),
		Decidable:   decidable(r),
		WidenTo:     []string{},
		Recipe:      recipe,
	}
	if r.Correction != nil {
		out.Correction = &ContextCorrectionDTO{From: r.Correction.From, To: r.Correction.To}
	}
	for _, e := range r.Evidence {
		out.Evidence = append(out.Evidence, ContextEvidenceDTO{Path: e.Path, Unit: e.Unit, Quote: e.Quote})
	}
	out.Droppable = out.Decidable
	if r.Established && r.Kind.Bears() {
		out.Droppable = true
		out.WidenTo = host.ContextWidenOptions(r.Subject.Kind, r.Scope)
	}
	return out
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
