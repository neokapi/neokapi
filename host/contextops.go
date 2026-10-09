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
// recorded it; only a person keeps, edits, drops or widens a rule, or resets
// the context.
//
// A caller that states an actor kind is taken at its word: the MCP tools state
// the agent, and the desktop states the person. A request that states none came
// from a command line, where the environment answers
// (host/contextactor.go).
//
// Every request names its project the same way (contextOps): by the recipe
// path of a checkout, or by the workspace key a project is registered under. A
// project registered from another machine has its log, its terms and its
// content memory in this workspace and none of its files, and the key is how
// a surface reads and decides on it here.
package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
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
	// Project is the recipe path, or the workspace key of a project with no
	// checkout on this machine. Empty resolves the project the way every other
	// command does.
	Project string
	// AllProjects reads every project in the workspace instead, for a surface
	// that shows the workspace's whole feed. Project is then left unread.
	AllProjects bool
	// Session narrows to one agent session.
	Session string
	// Status narrows to one status: suggested, established, contested,
	// withdrawn, dropped or reset.
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
	// Project is the recipe path, or the workspace key of a project with no
	// checkout on this machine.
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

// WidenToProject is the scope that puts a rule settled under one profile in
// force under every profile of its project.
const WidenToProject = "project"

// ContextWidenOptions are the widenings open to a rule at scope: the whole
// workspace, and each axis its point is specific about. A rule already
// answering workspace-wide has nowhere further to go.
func ContextWidenOptions(scope contextop.Scope) []string {
	out := []string{}
	if scope.Level != contextop.LevelWorkspace {
		out = append(out, WidenToWorkspace)
	}
	axes := make([]string, 0, len(scope.Coordinates))
	for axis := range scope.Coordinates {
		axes = append(axes, axis)
	}
	slices.Sort(axes)
	return append(out, axes...)
}

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
		"ask them to run `kapi context review`, which walks through what is waiting", err)
}

// teachRefusalTo answers a refusal addressed to whoever was refused. An agent
// is told to put the decision in front of the person; a person is the one who
// decides, and is told how to set the operation aside themselves.
func teachRefusalTo(err error, actor contextop.Actor, id string) error {
	if err == nil || !errors.Is(err, contextop.ErrRefused) || actor.Kind != contextop.ActorPerson {
		return teachRefusal(err)
	}
	return fmt.Errorf("%w\nto set aside what someone else recorded, drop it: `kapi context review --drop %s`",
		err, contextop.ShortID(id))
}

// RecordContextObservation records something somebody noticed. A fact in
// prose implies no rule; an observation naming a term states a term rule that
// advises from this moment and binds once a person keeps it.
func (a *App) RecordContextObservation(ctx context.Context, req ContextObserveRequest) (ContextOperation, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextOperation{}, err
	}
	defer s.refreshRulesFiles(ctx)
	subject, err := observedSubject(req)
	if err != nil {
		return ContextOperation{}, err
	}
	if err := s.observedInFiles(ctx, subject, req.Evidence); err != nil {
		return ContextOperation{}, err
	}
	if subject.Term != nil {
		if err := s.entriesContradict(ctx, *subject.Term, req.Evidence, "observation"); err != nil {
			return ContextOperation{}, err
		}
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
	for _, form := range req.InsteadOf {
		if describesTerm(term, form) {
			return contextop.Subject{}, fmt.Errorf("%q holds the term %q with more words: give the form writers write in its place, "+
				"such as a split, hyphenated or differently spelled form, or say what you saw in text", form, term)
		}
	}
	rule := contextop.ObservedRule(term, req.InsteadOf)
	if rule.Replacement == "" {
		// The term is the form the project uses. With nothing to avoid, the
		// rule would hold that form alone, which reads as a word to avoid and
		// would flag the project's own spelling of it. What the observation
		// says in text is recorded as a note instead.
		if text != "" {
			return contextop.Subject{Kind: contextop.SubjectNote, Text: text}, nil
		}
		return contextop.Subject{}, fmt.Errorf("%q is the form the project uses, and nothing names the forms to avoid: "+
			"give the spellings writers get wrong in instead_of (--instead-of), such as a split, hyphenated "+
			"or differently cased form, or say what you saw in text, which is recorded as a note", term)
	}
	return contextop.Subject{Kind: contextop.SubjectTerm, Term: &rule, Text: text}, nil
}

// describesTerm reports whether an avoided form is a description of the term
// rather than a form of it: the term with two or more words added, as
// "Harbor Help product name variant" is of "Harbor Help".
func describesTerm(term, form string) bool {
	t, f := strings.ToLower(strings.TrimSpace(term)), strings.ToLower(strings.TrimSpace(form))
	return t != "" && strings.Contains(f, t) && len(strings.Fields(f))-len(strings.Fields(t)) >= 2
}

// maxObservedFile bounds the size of a file whose bytes an observation's
// evidence is looked for in.
const maxObservedFile = 8 << 20

// observedInFiles refuses a term observation whose evidence names a file of
// the project in which neither the term nor a form it avoids can be seen:
// what the observation says was seen there is not there.
//
// A file holds a form when its bytes do, or when the text of its blocks does
// as a read shows it, so a term in a compressed format, spelled with a
// character reference or an escape, or split by an inline code counts. Case
// and runs of white space are ignored, so a term a writer wrapped across two
// lines counts too. A file no reader opens, or that lies outside the project,
// is passed over.
func (s *contextOpsSession) observedInFiles(ctx context.Context, subject contextop.Subject, evidence []contextop.Evidence) error {
	if subject.Kind != contextop.SubjectTerm || subject.Term == nil || s.root == "" {
		return nil
	}
	var forms []string
	for _, f := range append([]string{subject.Term.Replacement, subject.Term.Term}, subject.Term.Forms...) {
		if f = foldedText(f); f != "" {
			forms = append(forms, f)
		}
	}
	if len(forms) == 0 {
		return nil
	}
	for _, e := range evidence {
		if e.Path == "" || filepath.IsAbs(e.Path) {
			continue
		}
		path := filepath.Join(s.root, filepath.FromSlash(e.Path))
		rel, err := filepath.Rel(s.root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Size() <= maxObservedFile {
			if data, err := os.ReadFile(path); err == nil && holdsAnyForm(string(data), forms) {
				continue
			}
		}
		seen, read := s.blocksHoldForm(ctx, filepath.ToSlash(rel), forms)
		if !read || seen {
			continue
		}
		return fmt.Errorf("%s holds neither %q nor a form it avoids, so the observation was not seen there: "+
			"name the file you saw it in, or record what you know in text", e.Path, subject.Term.Replacement)
	}
	return nil
}

// blocksHoldForm reports whether the text of a block of the project's file at
// rel, an edition the file holds or a branch of a plural or select, holds one
// of forms, which are folded (foldedText). read is false when no reader opens
// the file.
func (s *contextOpsSession) blocksHoldForm(ctx context.Context, rel string, forms []string) (seen, read bool) {
	if !s.checkedOut() {
		return false, false
	}
	svc, err := s.app.ChangeService(ctx, ChangeServiceOptions{Project: s.recipe, Origin: "context"})
	if err != nil {
		return false, false
	}
	holds := func(text string) bool {
		// An inline code has no width in a find, and a writer may also have
		// put one between two words, as a line break.
		return holdsAnyForm(placeholderTokenRe.ReplaceAllString(text, ""), forms) ||
			holdsAnyForm(placeholderTokenRe.ReplaceAllString(text, " "), forms)
	}
	_, err = svc.ReadEach(ctx, change.ReadRequest{Doc: rel, OwnEdition: true}, func(_ *model.Block, b change.BlockRead) error {
		texts := []string{b.Text}
		for _, ed := range b.Editions {
			texts = append(texts, ed.Text)
		}
		for _, st := range b.Structures {
			for _, branch := range st.Branches {
				texts = append(texts, branch)
			}
		}
		if slices.ContainsFunc(texts, holds) {
			seen = true
			return change.ErrStop
		}
		return nil
	})
	if err != nil && !errors.Is(err, change.ErrStop) {
		return false, false
	}
	return seen, true
}

// placeholderTokenRe matches an inline code's token in placeholder text.
var placeholderTokenRe = regexp.MustCompile(`<x id="[^"]*"/>`)

// foldedText is text with its case folded and each run of white space a
// single space, for a comparison that ignores both.
func foldedText(text string) string {
	return strings.Join(strings.Fields(strings.ToLower(text)), " ")
}

// holdsAnyForm reports whether text, folded, holds one of forms, which are
// folded already.
func holdsAnyForm(text string, forms []string) bool {
	body := foldedText(text)
	return slices.ContainsFunc(forms, func(f string) bool { return strings.Contains(body, f) })
}

// maxContradictionPages bounds how much of a file entriesContradict reads.
const maxContradictionPages = 10

// entriesContradict refuses a term rule recorded from one entry of a file
// when another entry of the same file holds the form the rule avoids: a rule
// for the whole file would report that entry, which the change left as it
// was. Book is "Bestill" on a button and "Bok" in a library, and a rule made
// at the button would call the library wrong. Only evidence that names a
// block is weighed, against the file's blocks and the editions it holds.
func (s *contextOpsSession) entriesContradict(ctx context.Context, rule profile.TermRule, evidence []contextop.Evidence, what string) error {
	re := formMatcher(append([]string{rule.Term}, rule.Forms...), rule.MatchesCase())
	if re == nil {
		return nil
	}
	for _, e := range evidence {
		if e.Path == "" || e.Unit == "" {
			continue
		}
		others := s.entriesHolding(ctx, e.Path, e.Unit, re)
		if len(others) == 0 {
			continue
		}
		const named = 3
		list := strings.Join(others[:min(len(others), named)], ", ")
		if len(others) > named {
			list += fmt.Sprintf(" and %d more", len(others)-named)
		}
		return fmt.Errorf("%s holds %q at %s as well as at %s, so a rule against it would report %s, which the change left as it was: "+
			"record the %s without the rule (it keeps the entry it was made at), and say in a note which entries the wording belongs to",
			e.Path, rule.Term, list, e.Unit, list, what)
	}
	return nil
}

// entriesHolding lists the blocks of the project's file at rel, other than
// except, whose text or an edition the file holds matches re. A file the
// project cannot read lists none, and a project with no checkout has none to
// read.
func (s *contextOpsSession) entriesHolding(ctx context.Context, rel, except string, re *regexp.Regexp) []string {
	if !s.checkedOut() {
		return nil
	}
	svc, err := s.app.ChangeService(ctx, ChangeServiceOptions{Project: s.recipe, Origin: "context"})
	if err != nil {
		return nil
	}
	var out []string
	req := change.ReadRequest{Doc: rel, Limit: change.MaxReadLimit, OwnEdition: true}
	for range maxContradictionPages {
		page, err := svc.Read(ctx, req)
		if err != nil {
			return out
		}
		for _, b := range page.Blocks {
			if b.Ref.Block == except {
				continue
			}
			hit := re.MatchString(b.Text)
			for _, ed := range b.Editions {
				hit = hit || re.MatchString(ed.Text)
			}
			if hit {
				out = append(out, b.Ref.Block)
			}
		}
		if page.Next == "" {
			return out
		}
		req.Cursor = page.Next
	}
	return out
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
	defer s.refreshRulesFiles(ctx)
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
		rule := &profile.TermRule{
			Term:        req.From,
			Replacement: req.To,
			Advisory:    req.Advisory,
			Note:        req.Note,
		}
		if err := s.entriesContradict(ctx, *rule, req.Evidence, "correction"); err != nil {
			return ContextOperation{}, err
		}
		record.Subject = contextop.Subject{Kind: contextop.SubjectTerm, Term: rule}
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

// ContextOperations reads a project's context history, newest first, or the
// whole workspace's when the request asks for every project.
func (a *App) ContextOperations(ctx context.Context, req ContextLogRequest) (ContextOperationList, error) {
	var (
		ledger *contextop.Ledger
		key    workspace.ProjectKey
	)
	if req.AllProjects {
		ws, err := a.Workspace(ctx)
		if err != nil {
			return ContextOperationList{}, err
		}
		ledger = contextop.NewLedger(ws, contextop.PersonDecides)
	} else {
		s, err := a.contextOps(ctx, req.Project)
		if err != nil {
			return ContextOperationList{}, err
		}
		ledger, key = s.ledger, s.key
	}
	session, err := a.logSession(req.Session)
	if err != nil {
		return ContextOperationList{}, err
	}
	records, err := ledger.Records(ctx, contextop.Filter{
		Project:  key,
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
	out := ContextOperationList{Project: string(key), Operations: make([]ContextOperation, 0, len(records))}
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
	defer s.refreshRulesFiles(ctx)
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
		return fmt.Errorf("operation %s cannot be kept yet: %s. Choose it with `kapi context review --choose %s`, which sets the other side aside",
			contextop.ShortID(r.ID), contestedReason(r), contextop.ShortID(r.ID))
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
		widened, werr := widenScope(target, req.WidenTo)
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
	landed, err := s.landAt(ctx, target, req.WidenTo != "")
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
// project; "project" in every profile and channel of this one; any other value
// names a coordinate axis the rule stops being specific about.
//
// The steps nest. Widening to the project drops the profile the evidence was
// seen under, and with it the product and channel axes the profile derives,
// so the rule holds at every point of the project. Widening to the workspace
// drops them too, and the project. An axis no profile derives, such as a
// brand the recipe declares, stays: a rule widened from one brand's project
// holds wherever that brand does.
func widenScope(target contextop.Record, to string) (contextop.Scope, error) {
	scope := target.Scope
	switch to {
	case WidenToWorkspace:
		if scope.Level == contextop.LevelWorkspace {
			return contextop.Scope{}, errors.New("this rule already holds across the workspace")
		}
		scope.Level = contextop.LevelWorkspace
		scope.AllProfiles = false
		scope.Coordinates = withoutProfileAxes(scope.Coordinates)
		return scope, nil
	case WidenToProject:
		if scope.Level == contextop.LevelWorkspace {
			return contextop.Scope{}, errors.New("this rule holds across the workspace, which includes the project")
		}
		narrowed := withoutProfileAxes(scope.Coordinates)
		if scope.AllProfiles || (target.Basis.Profile == "" && len(narrowed) == len(scope.Coordinates)) {
			return contextop.Scope{}, errors.New("this rule already holds across the project: it was settled under no profile")
		}
		scope.AllProfiles = true
		scope.Coordinates = narrowed
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

// withoutProfileAxes drops the axes a profile derives (product and channel)
// from a scope's coordinates, keeping the rest.
func withoutProfileAxes(coordinates map[string]string) map[string]string {
	out := make(map[string]string, len(coordinates))
	for axis, value := range coordinates {
		if axis != project.ProductAxis && axis != project.ChannelAxis {
			out[axis] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// DropContextOperation sets a suggestion or a rule aside. It stops answering at
// once, and an established rule is taken back out of the stores keeping wrote
// it to. Changing your mind about a rule is this: a new decision, recorded
// beside the one it replaces.
func (a *App) DropContextOperation(ctx context.Context, req ContextDropRequest) (ContextOperation, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextOperation{}, err
	}
	defer s.refreshRulesFiles(ctx)
	target, err := s.ledger.Subject(ctx, req.ID)
	if err != nil {
		return ContextOperation{}, err
	}
	if !target.Status.Answers() {
		return ContextOperation{}, fmt.Errorf("operation %s is already %s", target.ID, target.Status)
	}
	dropped, err := s.setAside(ctx, contextop.KindDrop, req.Actor, target, req.Note)
	if err != nil || !target.Established {
		return dropped, err
	}
	where, err := s.retract(ctx, target)
	if err != nil {
		return dropped, err
	}
	if where != "" {
		dropped.Landed = joinLanded(dropped.Landed, "taken back out of "+where)
	}
	return dropped, nil
}

// WithdrawContextOperation takes back a suggestion its author recorded. The
// policy decides who may: the author, in the session that recorded it.
func (a *App) WithdrawContextOperation(ctx context.Context, req ContextWithdrawRequest) (ContextOperation, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextOperation{}, err
	}
	defer s.refreshRulesFiles(ctx)
	target, err := s.ledger.Subject(ctx, req.ID)
	if err != nil {
		return ContextOperation{}, err
	}
	return s.setAside(ctx, contextop.KindWithdraw, req.Actor, target, req.Note)
}

// joinLanded adds one more thing an operation did to the line that says so.
func joinLanded(landed, more string) string {
	if landed == "" {
		return more
	}
	return landed + "; " + more
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
		return ContextOperation{}, teachRefusalTo(err, actor, target.ID)
	}
	return s.settled(ctx, written, before)
}

// WidenContextOperation moves an established rule to a broader point: out to
// the whole workspace, or past one axis of the point its evidence was seen at.
func (a *App) WidenContextOperation(ctx context.Context, req ContextWidenRequest) (ContextOperation, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextOperation{}, err
	}
	defer s.refreshRulesFiles(ctx)
	target, err := s.ledger.Subject(ctx, req.ID)
	if err != nil {
		return ContextOperation{}, err
	}
	if target.Status != contextop.StatusEstablished {
		return ContextOperation{}, fmt.Errorf("operation %s is %s; keep it before widening it", target.ID, target.Status)
	}
	widened, err := widenScope(target, req.To)
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
	landed, err := s.landWidened(ctx, target)
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
//
// A session opened by key for a project with no checkout on this machine has
// no recipe, root or loaded project: it reads and writes the project's stores
// in the workspace and nothing in its files.
type contextOpsSession struct {
	app    *App
	ledger *contextop.Ledger
	ws     *workspace.Workspace
	key    workspace.ProjectKey
	// proj is the loaded recipe, nil without a checkout.
	proj *project.KapiProject
	// recipe is the recipe path and root the directory holding it, both
	// empty without a checkout.
	recipe string
	root   string
	// changed says a rule was written to, or taken out of, a store in this
	// call, so the project's rules files are refreshed when it ends.
	changed bool
}

// checkedOut reports a session over a checkout on this machine.
func (s *contextOpsSession) checkedOut() bool { return s.recipe != "" }

// refreshRulesFiles refreshes the project's rules files when the call changed
// the rules in force (host/rulesfiles.go). A project with no checkout has no
// files to refresh.
func (s *contextOpsSession) refreshRulesFiles(ctx context.Context) {
	if s.changed && s.checkedOut() {
		s.app.refreshRulesFilesQuietly(ctx, s.recipe)
	}
}

// contextOps opens a project's context log.
//
// project is the recipe path of a checkout, or the workspace key a project is
// registered under. An empty path resolves the project the way every other
// command does. A key opens the checkout the registry lists when a readable
// one is on this machine, and the project's stores in the workspace alone
// otherwise, which is enough to read its log and to decide on it: a kept rule
// lands in the terms store or content memory the workspace holds for the
// project, and the next checkout anywhere reads it from there.
//
// A value that names a file or a directory is a path. One that names nothing
// on disk and is registered is a key; one that names nothing and is not
// registered is reported the way a mistyped path always was.
func (a *App) contextOps(ctx context.Context, project string) (*contextOpsSession, error) {
	if key, ok, err := a.registeredContextProject(ctx, project); err != nil {
		return nil, err
	} else if ok {
		return a.contextOpsFor(ctx, key)
	}
	return a.contextOpsAt(ctx, project)
}

// registeredContextProject reports whether project names a workspace key
// rather than a path: nothing on disk has that name, and the registry holds
// it.
func (a *App) registeredContextProject(ctx context.Context, project string) (workspace.ProjectKey, bool, error) {
	if project == "" {
		return "", false, nil
	}
	if _, err := os.Stat(project); err == nil {
		return "", false, nil
	}
	ws, err := a.Workspace(ctx)
	if err != nil {
		return "", false, err
	}
	key := workspace.ProjectKey(project)
	_, ok, err := ws.Lookup(ctx, key)
	if err != nil {
		return "", false, err
	}
	return key, ok, nil
}

// contextOpsFor opens a registered project's context log by its key: through
// a checkout on this machine when the registry lists a readable one of this
// project, and over the workspace's stores alone otherwise.
func (a *App) contextOpsFor(ctx context.Context, key workspace.ProjectKey) (*contextOpsSession, error) {
	ws, err := a.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	reg, ok, err := ws.Lookup(ctx, key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("this workspace holds no project %q", key)
	}
	if recipe := registeredCheckout(reg); recipe != "" {
		return a.contextOpsAt(ctx, recipe)
	}
	return &contextOpsSession{
		app:    a,
		ledger: contextop.NewLedger(ws, contextop.PersonDecides),
		ws:     ws,
		key:    key,
	}, nil
}

// registeredCheckout is the recipe of the first checkout a registration lists
// that is readable on this machine and still belongs to the project, empty
// when there is none. A checkout that was deleted, or whose directory now
// holds another project, stays in the registry until something opens the
// project again, so each one is checked rather than trusted.
func registeredCheckout(reg workspace.Registration) string {
	for _, dir := range reg.Checkouts {
		recipe := filepath.Join(dir, project.RecipeFileName)
		if identity, _ := recipeIdentity(recipe); identity == string(reg.Key) {
			return recipe
		}
	}
	return ""
}

// contextOpsAt opens a project's context log through its checkout. An empty
// recipe path resolves the project the way every other command does.
func (a *App) contextOpsAt(ctx context.Context, recipePath string) (*contextOpsSession, error) {
	cmd := NewEnvCommand(ctx, "context")
	cmd.Flags().String(projectFlagName, recipePath, "")
	resolved, err := ResolveProjectPath(cmd)
	if err != nil {
		return nil, err
	}
	if resolved == "" {
		return nil, errors.New("no kapi project: context operations belong to a project")
	}

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
// Without a checkout there is no recipe to resolve against, and the rule is
// scoped to the project as a whole.
func (s *contextOpsSession) basisAt(evidence []contextop.Evidence) (contextop.Basis, contextop.Scope) {
	if s.proj == nil {
		return contextop.Basis{}, contextop.Scope{Level: contextop.LevelProject}
	}
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
