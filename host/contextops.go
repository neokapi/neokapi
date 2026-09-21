// Context operations: the history of how a project's context came to be, and
// the one API every surface drives it through.
//
// A project starts with no terms, no voice rules and an empty content memory.
// What it knows accumulates out of ordinary work, and each step is recorded:
// an observation, a proposal with the evidence behind it, a correction someone
// made, and a person's decision about each. core/contextop owns the vocabulary
// and the append-only log; this file is what the CLI, the agent tools and the
// desktop call.
//
// Three things are worth knowing before reading on.
//
// A proposal advises from the moment it is recorded and binds only once a
// person confirms it. Until then it is projected into checks as an advisory
// finding at neutral severity, which carries no penalty and trips no gate.
//
// Confirming writes the rule where the existing subsystems already read it:
// through the same appliers `kapi apply` uses, into the committed source the
// recipe binds and from there into the project's terms store, voice profile or
// content memory. The log is the history, not a second home for the rule.
//
// Every transition goes through core/contextop's policy. Today an agent may
// observe, propose and record a correction; only a person confirms, edits,
// discards another actor's work, withdraws a confirmed rule or widens one.
package host

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
)

// ContextOperation is one recorded change to a project's context, as a surface
// shows it.
type ContextOperation struct {
	contextop.Record
	// Landed names what a confirmation wrote and where, empty for every other
	// operation. It is the assetResult detail from the applier that wrote it.
	Landed string `json:"landed,omitempty"`
}

// ContextOperationList is what `kapi context log` prints.
type ContextOperationList struct {
	// Project is the project the log was read for.
	Project string `json:"project,omitempty"`
	// Operations are the matching operations, newest first.
	Operations []ContextOperation `json:"operations"`
}

// ContextRevertResult is what reverting one operation or a whole session did.
type ContextRevertResult struct {
	// Session is the session reverted, empty when one operation was named.
	Session string `json:"session,omitempty"`
	// Reverted are the operations that stopped answering.
	Reverted []ContextOperation `json:"reverted"`
	// Retracted names the rules taken back out of the project's stores.
	Retracted []string `json:"retracted,omitempty"`
}

// ContextObserveRequest records a fact somebody noticed, with no rule implied.
type ContextObserveRequest struct {
	// Actor is who is acting. An empty Kind reads as a person, which is what a
	// command line is.
	Actor contextop.Actor
	// Project is the recipe path of the project the observation is about.
	Project string
	// Text is the fact, in the actor's own words.
	Text string
	// Evidence is where it was seen.
	Evidence []contextop.Evidence
}

// ContextProposeRequest proposes a candidate rule. Exactly one of Term, Voice
// and Memory is set.
type ContextProposeRequest struct {
	// Actor is who is acting. An empty Kind reads as a person, which is what a
	// command line is.
	Actor contextop.Actor
	// Project is the recipe path of the project the proposal is about.
	Project string
	// Term proposes a term rule: one word, what to write instead, how hard it
	// bites.
	Term *profile.TermRule
	// Voice proposes a rule for a list in the project's voice profile.
	Voice *contextop.VoiceRule
	// Memory proposes a source and target pair for the content memory.
	Memory *contextop.MemoryPair
	// Evidence is where the wording behind the proposal was seen. A proposal
	// with none is a preference; a proposal with evidence can be argued with.
	Evidence []contextop.Evidence
	// Note is whatever the actor wants to say about it.
	Note string
}

// ContextCorrectRequest records that someone changed wording at a location. A
// correction is evidence first; Propose asks for the rule it implies to be
// recorded with it.
type ContextCorrectRequest struct {
	// Actor is who is acting. An empty Kind reads as a person, which is what a
	// command line is.
	Actor contextop.Actor
	// Project is the recipe path of the project the correction is in.
	Project string
	// From is the wording that was there and To is what replaced it.
	From string
	To   string
	// Evidence is where the change was made.
	Evidence []contextop.Evidence
	// Propose carries the correction into a candidate term rule, so the next
	// use of the old wording is reported. Without it the correction is recorded
	// as evidence and nothing else.
	Propose bool
	// Severity is the severity the proposed rule carries once a person confirms
	// it. Empty leaves it unset, which fails a check when confirmed.
	Severity string
	// Note is whatever the actor wants to say about it.
	Note string
}

// ContextLogRequest narrows a reading of a project's context history.
type ContextLogRequest struct {
	// Project is the recipe path. Empty reads the whole workspace.
	Project string
	// Session narrows to one agent session.
	Session string
	// Status narrows to candidate, confirmed, discarded or reverted.
	Status contextop.Status
	// Actor narrows to one actor, by name or by kind.
	Actor string
	// Since drops everything older.
	Since time.Time
	// Subjects narrows to the operations that carry a rule or a fact of their
	// own, leaving out the decisions about them.
	Subjects bool
	// Limit keeps the most recent N. Zero keeps them all.
	Limit int
}

// ContextConfirmRequest makes a candidate binding, optionally editing it and
// optionally widening it in the same step.
type ContextConfirmRequest struct {
	// Actor is who is acting. An empty Kind reads as a person, which is what a
	// command line is.
	Actor contextop.Actor
	// Project is the recipe path.
	Project string
	// ID is the operation being confirmed. Naming a decision about a rule
	// reaches the rule.
	ID string
	// Replacement, when set, replaces what the rule says to write instead.
	Replacement string
	// Severity, when set, replaces how hard the rule bites. `minor` and
	// `neutral` report; everything else fails a check.
	Severity string
	// WidenTo widens the rule as it is confirmed: "workspace" puts it in force
	// in every project, and an axis name drops that axis from the rule's point
	// so it answers more widely.
	WidenTo string
	// Note is whatever the person wants to say about the decision.
	Note string
}

// ContextDiscardRequest rejects a candidate.
type ContextDiscardRequest struct {
	// Actor is who is acting. An empty Kind reads as a person.
	Actor   contextop.Actor
	Project string
	ID      string
	Note    string
}

// ContextRevertRequest undoes one operation, or everything one session did.
type ContextRevertRequest struct {
	// Actor is who is acting. An empty Kind reads as a person.
	Actor   contextop.Actor
	Project string
	// ID is the operation to undo. Empty when Session names a whole session.
	ID string
	// Session undoes every operation one agent session recorded.
	Session string
	Note    string
}

// ContextWidenRequest moves a confirmed rule to a broader point.
type ContextWidenRequest struct {
	// Actor is who is acting. An empty Kind reads as a person.
	Actor   contextop.Actor
	Project string
	ID      string
	// To is "workspace", or the name of a coordinate axis the rule should stop
	// being specific about.
	To   string
	Note string
}

// ContextSessionRequest asks what one session did.
type ContextSessionRequest struct {
	Project string
	Session string
}

// WidenToWorkspace is the scope that puts a rule in force in every project of
// the workspace.
const WidenToWorkspace = "workspace"

// RecordContextObservation records a fact somebody noticed. It implies no rule,
// so nothing about a check changes; it is the material a proposal is later
// drawn from.
func (a *App) RecordContextObservation(ctx context.Context, req ContextObserveRequest) (ContextOperation, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextOperation{}, err
	}
	if req.Text == "" {
		return ContextOperation{}, errors.New("an observation needs something to say")
	}
	record, err := s.ledger.Append(ctx, s.stamp(contextop.Record{
		Actor:    req.Actor,
		Kind:     contextop.KindObserve,
		Subject:  contextop.Subject{Kind: contextop.SubjectNote, Text: req.Text},
		Evidence: req.Evidence,
	}, req.Evidence))
	if err != nil {
		return ContextOperation{}, err
	}
	return ContextOperation{Record: record}, nil
}

// ProposeContextRule records a candidate rule with the evidence behind it.
//
// The rule advises from this moment: a check reports it as a proposal at
// neutral severity, which fails nothing. It binds when a person confirms it.
func (a *App) ProposeContextRule(ctx context.Context, req ContextProposeRequest) (ContextOperation, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextOperation{}, err
	}
	subject, err := proposedSubject(req)
	if err != nil {
		return ContextOperation{}, err
	}
	record, err := s.ledger.Append(ctx, s.stamp(contextop.Record{
		Actor:    req.Actor,
		Kind:     contextop.KindPropose,
		Subject:  subject,
		Evidence: req.Evidence,
		Note:     req.Note,
	}, req.Evidence))
	if err != nil {
		return ContextOperation{}, err
	}
	return ContextOperation{Record: record}, nil
}

// proposedSubject reads the one subject a proposal names.
func proposedSubject(req ContextProposeRequest) (contextop.Subject, error) {
	named := 0
	for _, set := range []bool{req.Term != nil, req.Voice != nil, req.Memory != nil} {
		if set {
			named++
		}
	}
	if named != 1 {
		return contextop.Subject{}, errors.New("a proposal names exactly one of a term rule, a voice rule and a content-memory pair")
	}
	switch {
	case req.Term != nil:
		if req.Term.Term == "" {
			return contextop.Subject{}, errors.New("a term rule needs a term")
		}
		return contextop.Subject{Kind: contextop.SubjectTerm, Term: req.Term}, nil
	case req.Voice != nil:
		if req.Voice.Rule.Term == "" {
			return contextop.Subject{}, errors.New("a voice rule needs a term")
		}
		if !validVoiceList(req.Voice.List) {
			return contextop.Subject{}, fmt.Errorf("a voice rule sits in one of %v, not %q", contextop.VoiceLists, req.Voice.List)
		}
		return contextop.Subject{Kind: contextop.SubjectVoice, Voice: req.Voice}, nil
	default:
		if req.Memory.Source == "" || req.Memory.Target == "" || req.Memory.TargetLocale == "" {
			return contextop.Subject{}, errors.New("a content-memory pair needs a source, a target and a target locale")
		}
		return contextop.Subject{Kind: contextop.SubjectMemory, Memory: req.Memory}, nil
	}
}

func validVoiceList(list string) bool { return slices.Contains(contextop.VoiceLists, list) }

// RecordContextCorrection records wording somebody changed, and, when asked,
// the candidate rule that change implies.
func (a *App) RecordContextCorrection(ctx context.Context, req ContextCorrectRequest) (ContextOperation, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextOperation{}, err
	}
	if req.From == "" || req.To == "" {
		return ContextOperation{}, errors.New("a correction needs the wording that was there and the wording that replaced it")
	}
	record := contextop.Record{
		Actor:      req.Actor,
		Kind:       contextop.KindCorrect,
		Correction: &contextop.Correction{From: req.From, To: req.To},
		Evidence:   req.Evidence,
		Note:       req.Note,
	}
	if req.Propose {
		record.Subject = contextop.Subject{Kind: contextop.SubjectTerm, Term: &profile.TermRule{
			Term:        req.From,
			Replacement: req.To,
			Severity:    req.Severity,
			Note:        req.Note,
		}}
	}
	written, err := s.ledger.Append(ctx, s.stamp(record, req.Evidence))
	if err != nil {
		return ContextOperation{}, err
	}
	return ContextOperation{Record: written}, nil
}

// ContextOperations reads a project's context history, newest first.
func (a *App) ContextOperations(ctx context.Context, req ContextLogRequest) (ContextOperationList, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextOperationList{}, err
	}
	records, err := s.ledger.Records(ctx, contextop.Filter{
		Project:  s.key,
		Session:  req.Session,
		Status:   req.Status,
		Actor:    req.Actor,
		Since:    req.Since,
		Subjects: req.Subjects,
		Limit:    req.Limit,
	})
	if err != nil {
		return ContextOperationList{}, err
	}
	out := ContextOperationList{Project: string(s.key), Operations: make([]ContextOperation, 0, len(records))}
	for _, r := range records {
		out.Operations = append(out.Operations, ContextOperation{Record: r})
	}
	return out, nil
}

// ConfirmContextOperation makes a candidate binding.
//
// It records the confirmation, with whatever edits and widening the person
// asked for, and then writes the rule where the subsystems read it: a term into
// the committed terms source and the project's terms store, a voice rule into
// the voice profile, a content-memory pair into the memory bundle. A rule
// widened to the workspace goes into the workspace's own rule store instead,
// because no project owns it.
func (a *App) ConfirmContextOperation(ctx context.Context, req ContextConfirmRequest) (ContextOperation, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextOperation{}, err
	}
	target, err := s.ledger.Subject(ctx, req.ID)
	if err != nil {
		return ContextOperation{}, err
	}
	if target.Subject.Kind == contextop.SubjectNote {
		return ContextOperation{}, fmt.Errorf("operation %s states no rule to confirm", target.ID)
	}

	confirm := contextop.Record{
		Actor:   req.Actor,
		Kind:    contextop.KindConfirm,
		Target:  target.ID,
		Project: target.Project,
		Note:    req.Note,
		Scope:   target.Scope,
	}
	if edited, changed := editSubject(target.Subject, req.Replacement, req.Severity); changed {
		confirm.Subject = edited
		target.Subject = edited
	}
	if req.WidenTo != "" {
		widened, werr := widenScope(target.Scope, req.WidenTo)
		if werr != nil {
			return ContextOperation{}, werr
		}
		confirm.Scope, target.Scope = widened, widened
	}

	// The decision is recorded before it is carried out. A write that fails
	// after the log has it leaves a confirmation with nothing behind it, which
	// a person can see and repeat; a write that succeeded with no record of who
	// asked for it is the thing nobody can act on.
	written, err := s.ledger.Append(ctx, confirm)
	if err != nil {
		return ContextOperation{}, err
	}
	target.Status = contextop.StatusConfirmed
	landed, err := s.land(ctx, target)
	if err != nil {
		return ContextOperation{}, err
	}
	return ContextOperation{Record: written, Landed: landed}, nil
}

// editSubject applies a confirmation's edits to the rule being confirmed, and
// reports whether anything moved. A content-memory pair and a note carry no
// replacement or severity, so an edit of either is a no-op.
func editSubject(subject contextop.Subject, replacement, severity string) (contextop.Subject, bool) {
	if replacement == "" && severity == "" {
		return subject, false
	}
	apply := func(rule *profile.TermRule) bool {
		changed := false
		if replacement != "" && rule.Replacement != replacement {
			rule.Replacement, changed = replacement, true
		}
		if severity != "" && rule.Severity != severity {
			rule.Severity, changed = severity, true
		}
		return changed
	}
	switch subject.Kind {
	case contextop.SubjectTerm:
		rule := *subject.Term
		if apply(&rule) {
			subject.Term = &rule
			return subject, true
		}
	case contextop.SubjectVoice:
		voice := *subject.Voice
		if apply(&voice.Rule) {
			subject.Voice = &voice
			return subject, true
		}
	}
	return subject, false
}

// widenScope moves a scope outward. "workspace" puts the rule in force in every
// project; any other value names a coordinate axis the rule stops being
// specific about.
func widenScope(scope contextop.Scope, to string) (contextop.Scope, error) {
	if to == WidenToWorkspace {
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

// DiscardContextOperation rejects a candidate. It stops answering at once, and
// a rule already confirmed is taken back out of the project's stores.
func (a *App) DiscardContextOperation(ctx context.Context, req ContextDiscardRequest) (ContextOperation, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextOperation{}, err
	}
	target, err := s.ledger.Subject(ctx, req.ID)
	if err != nil {
		return ContextOperation{}, err
	}
	written, err := s.ledger.Append(ctx, contextop.Record{
		Actor:   req.Actor,
		Kind:    contextop.KindDiscard,
		Target:  target.ID,
		Project: target.Project,
		Note:    req.Note,
	})
	if err != nil {
		return ContextOperation{}, err
	}
	retracted, err := s.retract(ctx, target)
	if err != nil {
		return ContextOperation{}, err
	}
	return ContextOperation{Record: written, Landed: retracted}, nil
}

// RevertContextOperations undoes one operation, or everything one session did.
//
// Reverting a session puts the project's answers back where they were before
// the session started: every candidate it recorded stops advising, and every
// rule it got confirmed is taken back out of the stores it was written to.
func (a *App) RevertContextOperations(ctx context.Context, req ContextRevertRequest) (ContextRevertResult, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextRevertResult{}, err
	}
	if (req.ID == "") == (req.Session == "") {
		return ContextRevertResult{}, errors.New("revert names an operation or a session, not both and not neither")
	}

	var targets []contextop.Record
	revert := contextop.Record{Actor: req.Actor, Kind: contextop.KindRevert, Note: req.Note, Project: s.key}
	if req.ID != "" {
		target, terr := s.ledger.Subject(ctx, req.ID)
		if terr != nil {
			return ContextRevertResult{}, terr
		}
		targets = append(targets, target)
		revert.Target, revert.Project = target.ID, target.Project
	} else {
		held, herr := s.ledger.Records(ctx, contextop.Filter{Session: req.Session, Subjects: true})
		if herr != nil {
			return ContextRevertResult{}, herr
		}
		targets = held
		revert.TargetSession = req.Session
	}

	if _, err := s.ledger.Append(ctx, revert); err != nil {
		return ContextRevertResult{}, err
	}

	out := ContextRevertResult{Session: req.Session}
	for _, target := range targets {
		retracted, rerr := s.retract(ctx, target)
		if rerr != nil {
			return ContextRevertResult{}, rerr
		}
		target.Status = contextop.StatusReverted
		out.Reverted = append(out.Reverted, ContextOperation{Record: target})
		if retracted != "" {
			out.Retracted = append(out.Retracted, retracted)
		}
	}
	return out, nil
}

// WidenContextOperation moves a confirmed rule to a broader point: out to the
// whole workspace, or past one axis of the point its evidence was seen at.
func (a *App) WidenContextOperation(ctx context.Context, req ContextWidenRequest) (ContextOperation, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextOperation{}, err
	}
	target, err := s.ledger.Subject(ctx, req.ID)
	if err != nil {
		return ContextOperation{}, err
	}
	if target.Status != contextop.StatusConfirmed {
		return ContextOperation{}, fmt.Errorf("operation %s is a %s; confirm it before widening it", target.ID, target.Status)
	}
	widened, err := widenScope(target.Scope, req.To)
	if err != nil {
		return ContextOperation{}, err
	}
	written, err := s.ledger.Append(ctx, contextop.Record{
		Actor:   req.Actor,
		Kind:    contextop.KindWiden,
		Target:  target.ID,
		Project: target.Project,
		Scope:   widened,
		Note:    req.Note,
	})
	if err != nil {
		return ContextOperation{}, err
	}
	target.Scope = widened
	landed, err := s.land(ctx, target)
	if err != nil {
		return ContextOperation{}, err
	}
	return ContextOperation{Record: written, Landed: landed}, nil
}

// ContextSessionSummary reports what one session did: how many operations of
// each kind it recorded, and what became of them.
func (a *App) ContextSessionSummary(ctx context.Context, req ContextSessionRequest) (contextop.SessionSummary, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return contextop.SessionSummary{}, err
	}
	return s.ledger.Session(ctx, req.Session)
}

// contextOpsSession is one project's context log, opened once for one call.
type contextOpsSession struct {
	app    *App
	ledger *contextop.Ledger
	ws     *workspace.Workspace
	key    workspace.ProjectKey
	proj   *project.KapiProject
	// recipe is the recipe path and root the directory holding it.
	recipe string
	root   string
	// cmd carries the project through to the appliers that land a confirmed
	// rule, which read it the way a command line would.
	cmd Command
}

// contextOps opens a project's context log. An empty recipe path resolves the
// project the way every other command does.
func (a *App) contextOps(ctx context.Context, recipePath string) (*contextOpsSession, error) {
	cmd := NewEnvCommand(ctx, "context")
	cmd.Flags().String(projectFlagName, recipePath, "")
	resolved, err := ResolveProjectPath(cmd)
	if err != nil {
		return nil, err
	}
	if resolved == "" {
		return nil, errors.New("no kapi project: context operations belong to a project")
	}
	cmd.Flags().Set(projectFlagName, resolved) //nolint:errcheck // the flag was just registered as a string

	ws, err := a.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	identity, _ := recipeIdentity(resolved)
	if identity == "" {
		return nil, fmt.Errorf("the recipe at %s carries neither an id nor a name, so its context has nowhere to live", DisplayName(resolved))
	}
	proj, err := project.LoadWithOptions(resolved, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, fmt.Errorf("load project for context operations: %w", err)
	}
	return &contextOpsSession{
		app:    a,
		ledger: contextop.NewLedger(ws, contextop.PersonDecides),
		ws:     ws,
		key:    workspace.ProjectKey(identity),
		proj:   proj,
		recipe: resolved,
		root:   filepath.Dir(resolved),
		cmd:    cmd,
	}, nil
}

// stamp fills in what the session knows about an operation the caller did not:
// the project it belongs to, and the governance in force where its evidence was
// seen.
func (s *contextOpsSession) stamp(r contextop.Record, evidence []contextop.Evidence) contextop.Record {
	r.Project = s.key
	basis, scope := s.basisAt(evidence)
	if r.Basis == (contextop.Basis{}) {
		r.Basis = basis
	}
	if r.Scope.Level == "" && len(r.Scope.Coordinates) == 0 {
		r.Scope = scope
	}
	return r
}

// basisAt resolves what governs where the evidence was seen, and the point the
// rule is therefore scoped to. Evidence naming no path resolves the project's
// own default point, which is where a fact about the project as a whole sits.
func (s *contextOpsSession) basisAt(evidence []contextop.Evidence) (contextop.Basis, contextop.Scope) {
	point := project.GovernancePoint{At: s.app.GovernanceInstant()}
	var collection string
	for _, e := range evidence {
		if e.Path != "" {
			point.Path = e.Path
			collection = s.proj.CollectionForPath(e.Path)
			break
		}
	}
	rc, err := s.proj.ResolveGovernanceFor(point)
	if err != nil || rc == nil {
		return contextop.Basis{}, contextop.Scope{Level: contextop.LevelProject}
	}
	coordinates := project.MergeCoordinates(
		s.proj.Defaults.Coordinates, rc.Ref().Coordinates(), collectionCoordinates(s.proj, collection))
	return contextop.Basis{Profile: rc.Profile, Channel: rc.Channel},
		contextop.Scope{Level: contextop.LevelProject, Coordinates: coordinates}
}
