// Context operations: the history of how a project's context came to be, and
// the one API every surface drives it through.
//
// A project starts with no terms, no voice rules and an empty content memory.
// What it knows accumulates out of ordinary work, and each step is recorded:
// an observation, a correction someone made, what a person imported, and a
// person's decision about each. core/contextop owns the vocabulary and the
// append-only log; this file is what the CLI, the agent tools and the desktop
// call.
//
// Three things are worth knowing before reading on.
//
// A suggestion advises from the moment it is recorded and binds only once a
// person keeps it. Until then it is projected into checks as a suggested
// finding, which reports, weighs nothing in the score and never fails.
//
// Keeping writes the rule where the existing subsystems already read it:
// through the same appliers `kapi apply` uses, into the project's terms store
// or content memory. The log is the history, not a second home for the rule.
//
// Every transition goes through core/contextop's policy. An agent may observe
// and record a correction, and withdraw its own suggestion in the session that
// recorded it; only a person keeps, edits, drops, reverts an established rule
// or widens one.
//
// A caller that states an actor kind is taken at its word: the MCP tools state
// the agent, and the desktop states the person. A request that states none came
// from a command line, where the environment answers
// (host/contextactor.go).
package host

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
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
	// Landed names what a keep wrote and where, or what a contest took out of
	// force, empty for every other operation. It is the assetResult detail from
	// the applier that wrote it.
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

// ContextObserveRequest records something somebody noticed: a fact in prose,
// or, with Term, the form the project uses for a word and the forms it avoids.
type ContextObserveRequest struct {
	// Actor is who is acting. An empty Kind is a command line, where the
	// environment answers (host/contextactor.go).
	Actor contextop.Actor
	// Project is the recipe path of the project the observation is about.
	Project string
	// Text is the fact, in the actor's own words.
	Text string
	// Term is the form the project uses for a word. With it the observation
	// states a term rule: write Term, and avoid InsteadOf and the variants
	// contextop.AvoidedForms derives from it.
	Term string
	// InsteadOf are forms the project avoids for Term.
	InsteadOf []string
	// Evidence is where it was seen.
	Evidence []contextop.Evidence
}

// ContextCorrectRequest records that someone changed wording at a location. A
// correction is evidence first; Suggest asks for the rule it implies to be
// recorded with it.
type ContextCorrectRequest struct {
	// Actor is who is acting. An empty Kind is a command line, where the
	// environment answers (host/contextactor.go).
	Actor contextop.Actor
	// Project is the recipe path of the project the correction is in.
	Project string
	// From is the wording that was there and To is what replaced it.
	From string
	To   string
	// Evidence is where the change was made.
	Evidence []contextop.Evidence
	// Suggest carries the correction into a suggested term rule, so the next
	// use of the old wording is reported. Without it the correction is recorded
	// as evidence and nothing else.
	Suggest bool
	// Advisory makes the suggested rule report without failing a check once a
	// person keeps it. Unset, an established rule fails.
	Advisory bool
	// Note is whatever the actor wants to say about it.
	Note string
}

// ContextLogRequest narrows a reading of a project's context history.
type ContextLogRequest struct {
	// Project is the recipe path. Empty reads the whole workspace.
	Project string
	// Session narrows to one agent session.
	Session string
	// Status narrows to one status: suggested, established, contested,
	// withdrawn, dropped or reverted.
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

// ContextKeepRequest establishes suggestions: the ones IDs names, or everything
// one session suggested. One suggestion may be edited and widened as it is
// kept.
type ContextKeepRequest struct {
	// Actor is who is acting. An empty Kind is a command line, where the
	// environment answers (host/contextactor.go).
	Actor contextop.Actor
	// Project is the recipe path.
	Project string
	// ID names one operation to keep, and IDs several; both may be given.
	// Naming a decision about a rule reaches the rule.
	ID  string
	IDs []string
	// Session keeps everything one session suggested that nothing disagrees
	// with, in place of IDs.
	Session string
	// Replacement, when set, replaces what the rule says to write instead. It
	// edits one rule, so it takes one id.
	Replacement string
	// Advisory, when set, replaces whether the rule only reports (true) or
	// fails a check (false).
	Advisory *bool
	// WidenTo widens the rule as it is kept: "workspace" puts it in force in
	// every project, and an axis name drops that axis from the rule's point so
	// it answers more widely.
	WidenTo string
	// Note is whatever the person wants to say about the decision.
	Note string
}

// ContextKeepResult is what keeping did.
type ContextKeepResult struct {
	// Kept are the keep operations recorded, one per suggestion.
	Kept []ContextOperation `json:"kept"`
	// Skipped are the suggestions a session keep left alone, with the reason:
	// a contested suggestion waits for a person to choose.
	Skipped []ContextKeepSkip `json:"skipped,omitempty"`
}

// ContextKeepSkip is one suggestion a keep left alone.
type ContextKeepSkip struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// ContextDropRequest sets a suggestion aside. Only a person drops; the author
// of a suggestion withdraws it instead.
type ContextDropRequest struct {
	// Actor is who is acting. An empty Kind is a command line, where the
	// environment answers (host/contextactor.go).
	Actor   contextop.Actor
	Project string
	ID      string
	Note    string
}

// ContextWithdrawRequest takes back a suggestion its author recorded, in the
// session that recorded it.
type ContextWithdrawRequest struct {
	// Actor is who is acting. An empty Kind is a command line, where the
	// environment answers (host/contextactor.go).
	Actor   contextop.Actor
	Project string
	ID      string
	Note    string
}

// ContextRevertRequest undoes one operation, or everything one session did.
type ContextRevertRequest struct {
	// Actor is who is acting. An empty Kind is a command line, where the
	// environment answers (host/contextactor.go).
	Actor   contextop.Actor
	Project string
	// ID is the operation to undo. Empty when Session names a whole session.
	ID string
	// Session undoes every operation one agent session recorded.
	Session string
	Note    string
}

// ContextWidenRequest moves an established rule to a broader point.
type ContextWidenRequest struct {
	// Actor is who is acting. An empty Kind is a command line, where the
	// environment answers (host/contextactor.go).
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

// ContextLogSessionSelf is the value ContextLogRequest.Session takes for the
// session this run records under. An agent that reached kapi from a shell
// learns its session from the environment rather than from a flag, so this is
// how it reads its own work back without being told the id.
const ContextLogSessionSelf = "this"

// actorFor resolves who a request belongs to and folds the resolution into the
// note the operation carries.
//
// A caller that states a kind is taken at its word, and states its own session
// where it has one: the MCP tools do both. An empty kind is a command line,
// where the environment answers and the session is noted here, so the desktop
// feed and `kapi context log --session` see one kind of agent session whichever
// surface recorded it.
func (s *contextOpsSession) actorFor(ctx context.Context, stated contextop.Actor, note string) (contextop.Actor, string, error) {
	if stated.Kind != "" {
		return stated, note, nil
	}
	resolved, err := s.app.commandActor()
	if err != nil {
		return contextop.Actor{}, "", err
	}
	s.noteAgentSession(ctx, resolved.Actor)
	return resolved.Actor, resolved.NoteWith(note), nil
}

// noteAgentSession records that an agent is at work in this project.
//
// Every failure is swallowed, for the reason NoteMCPSession gives: the note is
// something the workspace offers other surfaces, and an agent mid-task has
// nothing to do about a workspace that will not take one.
func (s *contextOpsSession) noteAgentSession(ctx context.Context, actor contextop.Actor) {
	if actor.Kind != contextop.ActorAgent || actor.Session == "" {
		return
	}
	ws, err := s.app.Workspace(ctx)
	if err != nil || ws == nil {
		return
	}
	now := time.Now().UTC()
	_ = ws.NoteAgentSession(ctx, workspace.AgentSession{
		ID:       actor.Session,
		Project:  s.key,
		Agent:    actor.Name,
		Started:  now,
		LastSeen: now,
	})
}

// teachRefusal answers a policy refusal with what to do instead.
//
// The reader is almost always an agent mid-task, and the useful next move is
// to put the suggestions in front of the person rather than to try the command
// again.
func teachRefusal(err error) error {
	if err == nil || !errors.Is(err, contextop.ErrRefused) {
		return err
	}
	return fmt.Errorf("%w\ndeciding belongs to the person working here: "+
		"show them what is waiting with `kapi context log --status suggested` and let them decide", err)
}

// RecordContextObservation records something somebody noticed. A fact in
// prose implies no rule; an observation naming a term states a term rule that
// advises from this moment and binds once a person keeps it.
func (a *App) RecordContextObservation(ctx context.Context, req ContextObserveRequest) (ContextOperation, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextOperation{}, err
	}
	subject, err := observedSubject(req)
	if err != nil {
		return ContextOperation{}, err
	}
	actor, note, err := s.actorFor(ctx, req.Actor, "")
	if err != nil {
		return ContextOperation{}, err
	}
	before, err := s.ledger.Records(ctx, contextop.Filter{Project: s.key, Subjects: true})
	if err != nil {
		return ContextOperation{}, err
	}
	record, err := s.ledger.Append(ctx, s.stamp(contextop.Record{
		Actor:    actor,
		Kind:     contextop.KindObserve,
		Subject:  subject,
		Evidence: req.Evidence,
		Note:     note,
	}, req.Evidence))
	if err != nil {
		return ContextOperation{}, teachRefusal(err)
	}
	return s.settled(ctx, record, before)
}

// observedSubject reads what an observation states: a term rule when it names a
// term, and a note otherwise.
func observedSubject(req ContextObserveRequest) (contextop.Subject, error) {
	text := strings.TrimSpace(req.Text)
	term := strings.TrimSpace(req.Term)
	if term == "" {
		if len(req.InsteadOf) > 0 {
			return contextop.Subject{}, errors.New("the forms to avoid need the form the project uses: name it with the term")
		}
		if text == "" {
			return contextop.Subject{}, errors.New("an observation needs something to say")
		}
		return contextop.Subject{Kind: contextop.SubjectNote, Text: text}, nil
	}
	rule := contextop.ObservedRule(term, req.InsteadOf)
	return contextop.Subject{Kind: contextop.SubjectTerm, Term: &rule, Text: text}, nil
}

// RecordContextCorrection records wording somebody changed, and, when asked,
// the suggested rule that change implies.
//
// A person's correction that reverses an established rule contests the rule,
// which then advises instead of binding, so the person's own edit never fails
// their build. The rule is taken out of the store it was written to until a
// person keeps it again or sets the correction aside.
func (a *App) RecordContextCorrection(ctx context.Context, req ContextCorrectRequest) (ContextOperation, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextOperation{}, err
	}
	if req.From == "" || req.To == "" {
		return ContextOperation{}, errors.New("a correction needs the wording that was there and the wording that replaced it")
	}
	actor, note, err := s.actorFor(ctx, req.Actor, req.Note)
	if err != nil {
		return ContextOperation{}, err
	}
	record := contextop.Record{
		Actor:      actor,
		Kind:       contextop.KindCorrect,
		Correction: &contextop.Correction{From: req.From, To: req.To},
		Evidence:   req.Evidence,
		Note:       note,
	}
	if req.Suggest {
		record.Subject = contextop.Subject{Kind: contextop.SubjectTerm, Term: &profile.TermRule{
			Term:        req.From,
			Replacement: req.To,
			Advisory:    req.Advisory,
			Note:        req.Note,
		}}
	}
	before, err := s.ledger.Records(ctx, contextop.Filter{Project: s.key, Subjects: true})
	if err != nil {
		return ContextOperation{}, err
	}
	written, err := s.ledger.Append(ctx, s.stamp(record, req.Evidence))
	if err != nil {
		return ContextOperation{}, teachRefusal(err)
	}
	return s.settled(ctx, written, before)
}

// ContextOperations reads a project's context history, newest first.
func (a *App) ContextOperations(ctx context.Context, req ContextLogRequest) (ContextOperationList, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextOperationList{}, err
	}
	session, err := a.logSession(req.Session)
	if err != nil {
		return ContextOperationList{}, err
	}
	records, err := s.ledger.Records(ctx, contextop.Filter{
		Project:  s.key,
		Session:  session,
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

// logSession reads the session a log request narrows to, answering
// ContextLogSessionSelf with the session this run records under.
func (a *App) logSession(session string) (string, error) {
	if session != ContextLogSessionSelf {
		return session, nil
	}
	resolved, err := a.commandActor()
	if err != nil {
		return "", err
	}
	if resolved.Actor.Session == "" {
		return "", fmt.Errorf(
			"%q is the session an agent run records under, and this one is a %s with no session: name a session id, or leave --session out",
			ContextLogSessionSelf, resolved.Actor.Kind)
	}
	return resolved.Actor.Session, nil
}

// KeepContextOperations establishes suggestions.
//
// It records a keep for each, with whatever edits and widening the person
// asked for, and then writes the rule where the subsystems read it: a term into
// the project's terms store, a content-memory pair into the content memory. A
// rule widened to the workspace goes into the workspace's own rule store
// instead, because no project owns it.
//
// A contested suggestion is kept only once a person has chosen: naming one
// explicitly is refused with the other side named, and a session keep leaves
// it for later and says so. A contested established rule is a person's own rule
// that a later correction reversed, and keeping it again is the choice.
func (a *App) KeepContextOperations(ctx context.Context, req ContextKeepRequest) (ContextKeepResult, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextKeepResult{}, err
	}
	if req.ID != "" {
		req.IDs = append([]string{req.ID}, req.IDs...)
	}
	if (len(req.IDs) == 0) == (req.Session == "") {
		return ContextKeepResult{}, errors.New("keep names operations or a session, not both and not neither")
	}
	if req.Replacement != "" && len(req.IDs) != 1 {
		return ContextKeepResult{}, errors.New("changing the rule as it is kept edits one rule: name one operation")
	}

	var out ContextKeepResult
	var targets []contextop.Record
	if req.Session != "" {
		held, herr := s.ledger.Records(ctx, contextop.Filter{Session: req.Session, Subjects: true})
		if herr != nil {
			return ContextKeepResult{}, herr
		}
		// Oldest first, so the keeps read in the order the session worked.
		for _, r := range slices.Backward(held) {
			switch {
			case r.Actor.Session != req.Session || r.Established || !r.Status.Advises():
			case r.Status == contextop.StatusContested:
				out.Skipped = append(out.Skipped, ContextKeepSkip{ID: r.ID, Reason: contestedReason(r)})
			default:
				targets = append(targets, r)
			}
		}
	} else {
		for _, id := range req.IDs {
			target, terr := s.ledger.Subject(ctx, id)
			if terr != nil {
				return ContextKeepResult{}, terr
			}
			if err := keepable(target); err != nil {
				return ContextKeepResult{}, err
			}
			targets = append(targets, target)
		}
	}

	actor, note, err := s.actorFor(ctx, req.Actor, req.Note)
	if err != nil {
		return ContextKeepResult{}, err
	}
	for _, target := range targets {
		op, kerr := s.keep(ctx, actor, note, target, req)
		if kerr != nil {
			return out, kerr
		}
		out.Kept = append(out.Kept, op)
	}
	return out, nil
}

// KeepContextOperation keeps one suggestion and returns the keep it recorded,
// for a surface that decides one entry at a time.
func (a *App) KeepContextOperation(ctx context.Context, req ContextKeepRequest) (ContextOperation, error) {
	if req.Session != "" || len(req.IDs)+btoi(req.ID != "") != 1 {
		return ContextOperation{}, errors.New("keep one operation: name exactly one id")
	}
	res, err := a.KeepContextOperations(ctx, req)
	if err != nil {
		return ContextOperation{}, err
	}
	return res.Kept[0], nil
}

// btoi counts a condition as one.
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// keepable refuses a keep a person has not chosen yet, and one of a subject
// nothing answers for any more.
func keepable(r contextop.Record) error {
	switch {
	case r.Status == contextop.StatusContested && !r.Established && !r.ContestedByEvidence():
		return fmt.Errorf("operation %s cannot be kept yet: %s. Choose first: drop the side you do not want with `kapi context drop`, or revert the established rule",
			contextop.ShortID(r.ID), contestedReason(r))
	case !r.Status.Answers():
		return fmt.Errorf("operation %s is %s: record it again to keep it", contextop.ShortID(r.ID), r.Status)
	}
	return nil
}

// contestedReason names the other side of a disagreement.
func contestedReason(r contextop.Record) string {
	others := make([]string, len(r.ContestedBy))
	for i, id := range r.ContestedBy {
		others[i] = "#" + contextop.ShortID(id)
	}
	return "it is contested by " + strings.Join(others, ", ")
}

// keep records one keep and lands the rule it establishes.
func (s *contextOpsSession) keep(ctx context.Context, actor contextop.Actor, note string, target contextop.Record, req ContextKeepRequest) (ContextOperation, error) {
	keep := contextop.Record{
		Actor:   actor,
		Kind:    contextop.KindKeep,
		Target:  target.ID,
		Project: target.Project,
		Note:    note,
		Scope:   target.Scope,
	}
	if edited, changed := editSubject(target.Subject, req.Replacement, req.Advisory); changed {
		keep.Subject = edited
		target.Subject = edited
	}
	if req.WidenTo != "" {
		widened, werr := widenScope(target.Scope, req.WidenTo)
		if werr != nil {
			return ContextOperation{}, werr
		}
		keep.Scope, target.Scope = widened, widened
	}

	// The decision is recorded before it is carried out. A write that fails
	// after the log has it leaves a keep with nothing behind it, which a person
	// can see and repeat; a write that succeeded with no record of who asked
	// for it is the thing nobody can act on.
	written, err := s.ledger.Append(ctx, keep)
	if err != nil {
		return ContextOperation{}, teachRefusal(err)
	}
	target.Status = contextop.StatusEstablished
	landed, err := s.land(ctx, target)
	if err != nil {
		return ContextOperation{}, err
	}
	return ContextOperation{Record: written, Landed: landed}, nil
}

// editSubject applies a keep's edits to the rule being kept, and reports
// whether anything moved. A content-memory pair and a note carry no
// replacement or advisory marking, so an edit of either is a no-op.
func editSubject(subject contextop.Subject, replacement string, advisory *bool) (contextop.Subject, bool) {
	if replacement == "" && advisory == nil {
		return subject, false
	}
	apply := func(rule *profile.TermRule) bool {
		changed := false
		if replacement != "" && rule.Replacement != replacement {
			rule.Replacement, changed = replacement, true
		}
		if advisory != nil && rule.Advisory != *advisory {
			rule.Advisory, changed = *advisory, true
		}
		return changed
	}
	if subject.Kind == contextop.SubjectTerm && subject.Term != nil {
		rule := *subject.Term
		if apply(&rule) {
			subject.Term = &rule
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

// DropContextOperation sets a suggestion aside. It stops answering at once. An
// established rule is reverted rather than dropped, because reverting is what
// takes it back out of the stores it was written to.
func (a *App) DropContextOperation(ctx context.Context, req ContextDropRequest) (ContextOperation, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextOperation{}, err
	}
	target, err := s.ledger.Subject(ctx, req.ID)
	if err != nil {
		return ContextOperation{}, err
	}
	if target.Established {
		return ContextOperation{}, fmt.Errorf("operation %s is an established rule: revert it with `kapi context revert %s`", target.ID, target.ID)
	}
	if !target.Status.Answers() {
		return ContextOperation{}, fmt.Errorf("operation %s is already %s", target.ID, target.Status)
	}
	return s.setAside(ctx, contextop.KindDrop, req.Actor, target, req.Note)
}

// WithdrawContextOperation takes back a suggestion its author recorded. The
// policy decides who may: the author, in the session that recorded it.
func (a *App) WithdrawContextOperation(ctx context.Context, req ContextWithdrawRequest) (ContextOperation, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextOperation{}, err
	}
	target, err := s.ledger.Subject(ctx, req.ID)
	if err != nil {
		return ContextOperation{}, err
	}
	return s.setAside(ctx, contextop.KindWithdraw, req.Actor, target, req.Note)
}

// setAside records a drop or a withdrawal and settles whatever the suggestion
// was contesting.
func (s *contextOpsSession) setAside(ctx context.Context, kind contextop.Kind, stated contextop.Actor, target contextop.Record, note string) (ContextOperation, error) {
	actor, note, err := s.actorFor(ctx, stated, note)
	if err != nil {
		return ContextOperation{}, err
	}
	before, err := s.ledger.Records(ctx, contextop.Filter{Project: s.key, Subjects: true})
	if err != nil {
		return ContextOperation{}, err
	}
	written, err := s.ledger.Append(ctx, contextop.Record{
		Actor:   actor,
		Kind:    kind,
		Target:  target.ID,
		Project: target.Project,
		Note:    note,
	})
	if err != nil {
		return ContextOperation{}, teachRefusal(err)
	}
	return s.settled(ctx, written, before)
}

// RevertContextOperations undoes one operation, or everything one session did.
//
// Reverting a session puts the project's answers back where they were before
// the session started: every suggestion it recorded stops advising, and every
// rule of it a person kept is taken back out of the stores it was written to.
func (a *App) RevertContextOperations(ctx context.Context, req ContextRevertRequest) (ContextRevertResult, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextRevertResult{}, err
	}
	if (req.ID == "") == (req.Session == "") {
		return ContextRevertResult{}, errors.New("revert names an operation or a session, not both and not neither")
	}

	actor, note, err := s.actorFor(ctx, req.Actor, req.Note)
	if err != nil {
		return ContextRevertResult{}, err
	}

	var targets []contextop.Record
	revert := contextop.Record{Actor: actor, Kind: contextop.KindRevert, Note: note, Project: s.key}
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

	before, err := s.ledger.Records(ctx, contextop.Filter{Project: s.key, Subjects: true})
	if err != nil {
		return ContextRevertResult{}, err
	}
	if _, err := s.ledger.Append(ctx, revert); err != nil {
		return ContextRevertResult{}, teachRefusal(err)
	}
	if _, err := s.reconcile(ctx, before); err != nil {
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

// WidenContextOperation moves an established rule to a broader point: out to
// the whole workspace, or past one axis of the point its evidence was seen at.
func (a *App) WidenContextOperation(ctx context.Context, req ContextWidenRequest) (ContextOperation, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextOperation{}, err
	}
	target, err := s.ledger.Subject(ctx, req.ID)
	if err != nil {
		return ContextOperation{}, err
	}
	if target.Status != contextop.StatusEstablished {
		return ContextOperation{}, fmt.Errorf("operation %s is %s; keep it before widening it", target.ID, target.Status)
	}
	widened, err := widenScope(target.Scope, req.To)
	if err != nil {
		return ContextOperation{}, err
	}
	actor, note, err := s.actorFor(ctx, req.Actor, req.Note)
	if err != nil {
		return ContextOperation{}, err
	}
	written, err := s.ledger.Append(ctx, contextop.Record{
		Actor:   actor,
		Kind:    contextop.KindWiden,
		Target:  target.ID,
		Project: target.Project,
		Scope:   widened,
		Note:    note,
	})
	if err != nil {
		return ContextOperation{}, teachRefusal(err)
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
	// cmd carries the project through to the appliers that land an
	// established rule, which read it the way a command line would.
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
