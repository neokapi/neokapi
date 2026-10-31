package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/rulesview"
)

// Deciding about suggestions, as `kapi rules` asks a person.
//
// core/rulesview builds the questions and fills in their counts. This file
// reads what it needs: the recipe, the operation log as the person reading
// sees it, and one read of the project's source, made before the first
// question. Every answer is the ContextReviewRequest `kapi context review`
// records its decisions with.

// The kinds of decision a person is asked about.
const (
	RuleDecisionName         = rulesview.DecisionName
	RuleDecisionWord         = rulesview.DecisionWord
	RuleDecisionNote         = rulesview.DecisionNote
	RuleDecisionMemory       = rulesview.DecisionMemory
	RuleDecisionDisagreement = rulesview.DecisionDisagreement
)

// RulesDecideView is every suggestion waiting for a person, as questions.
type RulesDecideView struct {
	Project   string         `json:"project"`
	Decisions []RuleDecision `json:"decisions"`
	// Counts are the rules listing's counts, for the line a project with
	// nothing to decide prints.
	Counts RulesCounts `json:"counts"`
	// Hint is the command that files suggestions from what kapi sees in the
	// files, for a project with none waiting.
	Hint string `json:"hint,omitempty"`

	st     *rulesState
	blocks []rulesview.Block
	// stamp names the source the blocks were read from: each file's path,
	// size and modification time, and the recipe's.
	stamp string
}

// RuleDecision is one question.
type RuleDecision struct {
	// ID names the suggestion, and Short is the form a person types. A
	// disagreement has no id of its own: each option carries its side's.
	ID    string `json:"id,omitempty"`
	Short string `json:"short,omitempty"`
	Kind  string `json:"kind"`
	// Question is what the person is asked.
	Question string `json:"question"`
	// Reason says what a disagreement is about.
	Reason string `json:"reason,omitempty"`
	// Sentence is the suggestion as a sentence.
	Sentence string `json:"sentence"`
	// Options are the forms of a name, or the sides of a disagreement.
	// Chosen is the option a keep takes unless the person picks another: the
	// form the suggestion names, or the side the files use most.
	Options []RuleOption `json:"options,omitempty"`
	Chosen  int          `json:"chosen"`
	// Consequence says what keeping it does, for a decision with no options.
	Consequence string `json:"consequence,omitempty"`
	// Seen is where the suggestion was seen.
	Seen *contextop.Evidence `json:"seen,omitempty"`
	// Applies is where a kept rule holds.
	Applies string `json:"applies"`
	// SuggestedBy names who suggested it.
	SuggestedBy string `json:"suggested_by,omitempty"`

	subject   contextop.Subject
	scope     contextop.Scope
	noticedBy contextop.Actor
}

// RuleOption is one answer to a question.
type RuleOption struct {
	// Form is one wording of a name; ID and Short name one side of a
	// disagreement, and Sentence states it.
	Form     string `json:"form,omitempty"`
	ID       string `json:"id,omitempty"`
	Short    string `json:"short,omitempty"`
	Sentence string `json:"sentence,omitempty"`
	// Blocks and Files count where the project's source writes the form, or
	// the wording the side says to use.
	Blocks int `json:"blocks"`
	Files  int `json:"files"`
	// Consequence says what keeping this option does.
	Consequence string `json:"consequence"`

	subject   contextop.Subject
	scope     contextop.Scope
	noticedBy contextop.Actor
}

// DecideRules reads the suggestions waiting in a project as questions. It
// records nothing.
func (a *App) DecideRules(ctx context.Context, req RulesRequest) (*RulesDecideView, error) {
	st, err := a.rulesState(ctx, req.Project)
	if err != nil {
		return nil, err
	}
	return a.decideView(ctx, st, req.WithCounts, true, req.Reuse)
}

// decideView reads the questions. With counted false it reads no block of the
// project's source, so no option counts its uses and none states what keeping
// it does: what recording an answer needs, and no more. reuse is a view read
// earlier whose blocks are counted again while the source is unchanged.
func (a *App) decideView(ctx context.Context, st *rulesState, withCounts, counted bool, reuse *RulesDecideView) (*RulesDecideView, error) {
	digest, err := a.digestModel(ctx, ContextDigestRequest{Project: st.recipe})
	if err != nil {
		return nil, err
	}
	v := &RulesDecideView{Project: string(st.key), Decisions: []RuleDecision{}, st: st}

	questions := rulesview.Questions(digest, st.records)
	if counted && rulesview.NeedsBlocks(questions) {
		v.blocks, v.stamp = a.ruleBlocks(ctx, st, reuse)
	}
	counter := v.counter()
	for i := range questions {
		if counted {
			counter.Fill(&questions[i])
		}
		v.Decisions = append(v.Decisions, ruleDecisionFrom(questions[i]))
	}

	if len(v.Decisions) == 0 || withCounts {
		listing, lerr := a.listRules(ctx, st)
		if lerr != nil {
			return nil, lerr
		}
		v.Counts = listing.Counts
	}
	if len(v.Decisions) == 0 {
		v.Hint = "kapi context suggest"
	}
	return v, nil
}

// Consequence states what keeping a name with form as the one to write does.
// The form may be one the person typed rather than one of the options.
func (v *RulesDecideView) Consequence(d RuleDecision, form string) string {
	return v.counter().Consequence(d.Model(), form)
}

// counter counts over the blocks the view read, where its project's scopes
// apply.
func (v *RulesDecideView) counter() rulesview.Counter {
	return rulesview.Counter{Space: v.st.space, Applies: v.st.applies, Blocks: slices.Values(v.blocks)}
}

// ruleDecisionFrom converts a question from the model's shape.
func ruleDecisionFrom(d rulesview.Decision) RuleDecision {
	out := RuleDecision{
		ID: d.ID, Short: d.Short, Kind: d.Kind, Question: d.Question, Reason: d.Reason,
		Sentence: d.Sentence, Chosen: d.Chosen, Consequence: d.Consequence, Seen: d.Seen,
		Applies: d.Applies, SuggestedBy: d.SuggestedBy,
		subject: d.Subject, scope: d.Scope, noticedBy: d.NoticedBy,
	}
	for _, o := range d.Options {
		out.Options = append(out.Options, RuleOption{
			Form: o.Form, ID: o.ID, Short: o.Short, Sentence: o.Sentence,
			Blocks: o.Blocks, Files: o.Files, Consequence: o.Consequence,
			subject: o.Subject, scope: o.Scope, noticedBy: o.NoticedBy,
		})
	}
	return out
}

// Model converts a question to the shape core/rulesview holds it in, which
// host/screens shapes for a screen.
func (d RuleDecision) Model() rulesview.Decision {
	out := rulesview.Decision{
		ID: d.ID, Short: d.Short, Kind: d.Kind, Question: d.Question, Reason: d.Reason,
		Sentence: d.Sentence, Chosen: d.Chosen, Consequence: d.Consequence, Seen: d.Seen,
		Applies: d.Applies, SuggestedBy: d.SuggestedBy,
		Subject: d.subject, Scope: d.scope, NoticedBy: d.noticedBy,
	}
	for _, o := range d.Options {
		out.Options = append(out.Options, rulesview.Option{
			Form: o.Form, ID: o.ID, Short: o.Short, Sentence: o.Sentence,
			Blocks: o.Blocks, Files: o.Files, Consequence: o.Consequence,
			Subject: o.subject, Scope: o.scope, NoticedBy: o.noticedBy,
		})
	}
	return out
}

// ruleBlocks reads every block of the project's source once, with the
// coordinates of the point each file sits at, and the stamp of what it read.
// When reuse read the same project's source and no file or the recipe has
// changed since, its blocks are returned without reading a file. A file that
// will not read is left out: a count is advice, and not worth failing the
// question over.
func (a *App) ruleBlocks(ctx context.Context, st *rulesState, reuse *RulesDecideView) ([]rulesview.Block, string) {
	a.InitRegistries()
	pctx := project.NewProjectContext(st.proj, st.recipe)
	resolved, err := pctx.ResolveContent(a.FormatReg)
	if err != nil {
		return nil, ""
	}
	stamp := sourceStamp(st.recipe, resolved)
	if reuse != nil && reuse.st != nil && reuse.st.key == st.key && reuse.stamp == stamp {
		return reuse.blocks, stamp
	}
	formats, err := a.newCheckFormats(st.cmd)
	if err != nil {
		return nil, ""
	}
	var out []rulesview.Block
	read := map[string]bool{}
	for _, rf := range resolved {
		if read[rf.Path] {
			continue
		}
		read[rf.Path] = true
		rel := filepath.ToSlash(rf.Relative)
		_, _, coordinates, perr := governedPoint(st.proj, project.GovernancePoint{Path: rel, At: st.at})
		if perr != nil {
			continue
		}
		name, cfg := formats.forFile(a, rf.Path)
		blocks, berr := a.readBlocksAs(ctx, rf.Path, name, cfg, a.SourceLocale())
		if berr != nil {
			continue
		}
		for _, b := range blocks {
			out = append(out, rulesview.Block{Path: rel, Coordinates: coordinates, Text: b.SourceText()})
		}
	}
	return out, stamp
}

// sourceStamp names the source a count reads: the recipe and each resolved
// file, by path, size and modification time. Two stamps are equal while no
// file has been added, removed or written.
func sourceStamp(recipe string, resolved []project.ResolvedFile) string {
	var b strings.Builder
	line := func(path string) {
		b.WriteString(path)
		if info, err := os.Stat(path); err == nil {
			fmt.Fprintf(&b, "\t%d\t%d", info.Size(), info.ModTime().UnixNano())
		}
		b.WriteByte('\n')
	}
	line(recipe)
	for _, rf := range resolved {
		line(rf.Path)
	}
	return b.String()
}

// KeepRequest is the decision that keeps option (an index into Options, -1
// for none) or, when form is set, the name as a form the person typed.
func (d RuleDecision) KeepRequest(option int, form string) ContextReviewRequest {
	if d.Kind == RuleDecisionDisagreement {
		if option < 0 || option >= len(d.Options) {
			option = d.Chosen
		}
		return ContextReviewRequest{Choose: d.Options[option].ID}
	}
	req := ContextReviewRequest{Keep: []string{d.ID}}
	if form == "" && option >= 0 && option < len(d.Options) {
		form = d.Options[option].Form
	}
	if rule, ok := d.subject.Rule(); ok && form != "" && form != rule.Replacement {
		req.Replacement = form
	}
	return req
}

// DropRequest is the decision that sets the suggestion aside, every side of a
// disagreement included.
func (d RuleDecision) DropRequest() ContextReviewRequest {
	if d.Kind != RuleDecisionDisagreement {
		return ContextReviewRequest{Drop: []string{d.ID}}
	}
	ids := make([]string, len(d.Options))
	for i, o := range d.Options {
		ids[i] = o.ID
	}
	return ContextReviewRequest{Drop: ids}
}

// NoticedBy is who suggested what the question is about: the suggestion's
// actor, or the first side's for a disagreement.
func (d RuleDecision) NoticedBy() contextop.Actor { return d.noticedBy }

// NoticedBy is who suggested the side an option stands for. An option that is
// a form of a name has none of its own.
func (o RuleOption) NoticedBy() contextop.Actor { return o.noticedBy }

// Label is the wording a question is about, for a list that shows each
// question on one line. For a name it is the form the suggestion says to
// write, and for a word rule the wording it avoids, which both sides of a
// disagreement share. A content-memory pair gives its source, and a note its
// prose.
func (d RuleDecision) Label() string { return d.Model().Label() }

// Avoided counts the wordings a word rule's check reports: the term and its
// other forms. A question about anything else counts none.
func (d RuleDecision) Avoided() int { return d.Model().Avoided() }

// Find returns the question id belongs to, and the option it names: id is a
// suggestion's own, or one side's of a disagreement. The option is -1 for a
// suggestion's own id.
func (v *RulesDecideView) Find(id string) (RuleDecision, int, bool) {
	for _, d := range v.Decisions {
		if id != "" && d.ID == id {
			return d, -1, true
		}
		for i, o := range d.Options {
			if id != "" && o.ID == id {
				return d, i, true
			}
		}
	}
	return RuleDecision{}, -1, false
}

// The answers a person gives to a question, as RuleDecideRequest.Action
// names them. Not a rule is a drop.
const (
	RuleAnswerKeep  = "keep"
	RuleAnswerDrop  = "drop"
	RuleAnswerLater = "later"
)

// RuleDecideRequest is one answer to one question, for a surface that asks
// the questions `kapi rules` asks and records each answer as it is given.
type RuleDecideRequest struct {
	// Actor is who is answering. An empty Kind is a command line, where the
	// environment answers (host/contextactor.go).
	Actor contextop.Actor
	// Project is the recipe path.
	Project string
	// ID names the suggestion, or for a disagreement the side a keep chooses.
	ID string
	// Answer is RuleAnswerKeep, RuleAnswerDrop or RuleAnswerLater.
	Answer string
	// Form is the form a kept name says to write: one of the options, or one
	// the person typed. Empty keeps the form the suggestion names.
	Form string
	Note string
}

// RuleDecideResult is what one answer recorded: the review round a keep or a
// drop is, or the suggestions a later leaves waiting.
type RuleDecideResult struct {
	Answer string               `json:"answer"`
	Review *ContextReviewResult `json:"review,omitempty"`
	Later  []string             `json:"later,omitempty"`
}

// DecideRule records one answer to one question: a keep, a not a rule or a
// later, through the calls `kapi rules` makes for k, n and l (KeepRequest,
// DropRequest and LeaveRulesForLater). It reads the questions again first,
// without counting uses, so an answer to a suggestion decided meanwhile is
// refused rather than recorded twice.
func (a *App) DecideRule(ctx context.Context, req RuleDecideRequest) (RuleDecideResult, error) {
	out := RuleDecideResult{Answer: req.Answer}
	st, err := a.rulesState(ctx, req.Project)
	if err != nil {
		return out, err
	}
	v, err := a.decideView(ctx, st, false, false, nil)
	if err != nil {
		return out, err
	}
	d, option, ok := v.Find(req.ID)
	if !ok {
		return out, fmt.Errorf("no suggestion %s waits for a decision", contextop.ShortID(req.ID))
	}
	var review ContextReviewRequest
	switch req.Answer {
	case RuleAnswerKeep:
		review = d.KeepRequest(option, req.Form)
	case RuleAnswerDrop:
		review = d.DropRequest()
	case RuleAnswerLater:
		ids := []string{d.ID}
		if d.Kind == RuleDecisionDisagreement {
			ids = ids[:0]
			for _, o := range d.Options {
				ids = append(ids, o.ID)
			}
		}
		later, lerr := a.LeaveRulesForLater(ctx, req.Project, ids)
		out.Later = later.Later
		return out, lerr
	default:
		return out, fmt.Errorf("answer %q is none of keep, drop and later", req.Answer)
	}
	review.Actor, review.Project, review.Note = req.Actor, req.Project, req.Note
	res, err := a.DecideContextReview(ctx, review)
	out.Review = &res
	return out, err
}

// FormatText lists every question without asking it: what a terminal-less
// `kapi rules` prints, with the id each decision takes.
func (v *RulesDecideView) FormatText(w io.Writer) error {
	if len(v.Decisions) == 0 {
		_, err := fmt.Fprintf(w, "No suggestions to decide. %s.\nFind some in the files with: %s\n", v.Counts.Line(), v.Hint)
		return err
	}
	for i, d := range v.Decisions {
		if _, err := fmt.Fprintf(w, "\n[%d/%d] ", i+1, len(v.Decisions)); err != nil {
			return err
		}
		if err := d.Write(w, d.Chosen); err != nil {
			return err
		}
	}
	line := "\nDecide with: kapi rules --keep <id>, --drop <id> or --later <id>"
	if slices.ContainsFunc(v.Decisions, func(d RuleDecision) bool { return d.Kind == RuleDecisionDisagreement }) {
		// A side of a disagreement is kept by choosing it; --keep refuses it.
		line = "\nDecide with: kapi rules --keep <id>, --choose <id> (one side where two disagree), --drop <id> or --later <id>"
	}
	_, err := fmt.Fprintln(w, line)
	return err
}

// Write prints one question with option chosen marked: the question, the
// options with their counts, what keeping the chosen one does, and where the
// suggestion was seen.
func (d RuleDecision) Write(w io.Writer, chosen int) error {
	var b strings.Builder
	b.WriteString(d.Question)
	if d.Short != "" {
		b.WriteString("  #" + d.Short)
	}
	b.WriteString("\n")
	if d.Reason != "" {
		b.WriteString("  " + capitalSentence(d.Reason) + "\n")
	}
	if d.Sentence != "" && (len(d.Options) == 0 || d.Kind != RuleDecisionName) {
		b.WriteString("  " + d.Sentence + "\n")
	}
	for i, o := range d.Options {
		mark := " "
		if i == chosen {
			mark = "*"
		}
		label := o.Form
		if d.Kind == RuleDecisionDisagreement {
			label = o.Sentence + "  #" + o.Short
		}
		fmt.Fprintf(&b, "  %s %d) %s  %s in %s\n", mark, i+1, label, rulesview.BlockCount(o.Blocks), fileCount(o.Files))
	}
	if d.Kind == RuleDecisionName {
		fmt.Fprintf(&b, "    %d) Other…\n", len(d.Options)+1)
	}
	switch {
	case chosen >= 0 && chosen < len(d.Options):
		b.WriteString("  " + d.Options[chosen].Consequence + "\n")
	case d.Consequence != "":
		b.WriteString("  " + d.Consequence + "\n")
	}
	if line := seenLine(d.Seen); line != "" {
		b.WriteString("  Seen " + line + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// fileCount renders a count of files.
func fileCount(n int) string {
	if n == 1 {
		return "1 file"
	}
	return strconv.Itoa(n) + " files"
}

// seenLine renders where a suggestion was seen: the quote and its file.
func seenLine(e *contextop.Evidence) string {
	if e == nil {
		return ""
	}
	switch {
	case e.Quote != "" && e.Path != "":
		return strconv.Quote(e.Quote) + " in " + e.Path
	case e.Quote != "":
		return strconv.Quote(e.Quote)
	case e.Path != "":
		return "in " + e.Path
	case e.URL != "":
		return "at " + e.URL
	}
	return ""
}

// RulesLaterResult is what leaving suggestions for later did: nothing is
// recorded, and each one keeps advising.
type RulesLaterResult struct {
	Later []string `json:"later"`
}

// FormatText says which suggestions wait.
func (r RulesLaterResult) FormatText(w io.Writer) error {
	for _, id := range r.Later {
		if _, err := fmt.Fprintf(w, "Left #%s for later. It keeps advising until you decide.\n", contextop.ShortID(id)); err != nil {
			return err
		}
	}
	return nil
}

// LeaveRulesForLater checks that each id names a suggestion still waiting and
// records nothing, which is what answering "later" at a review prompt does.
func (a *App) LeaveRulesForLater(ctx context.Context, project string, ids []string) (RulesLaterResult, error) {
	out := RulesLaterResult{Later: []string{}}
	for _, id := range ids {
		subject, err := a.ContextSubject(ctx, project, id)
		if err != nil {
			return out, err
		}
		if subject.Established || !subject.Status.Advises() {
			return out, fmt.Errorf("operation %s is %s, not a suggestion waiting for a decision", contextop.ShortID(subject.ID), subject.Status)
		}
		out.Later = append(out.Later, subject.ID)
	}
	return out, nil
}

// RulesNarrowRequest limits a kept rule to part of a project.
type RulesNarrowRequest struct {
	// Actor is who is acting. An empty Kind is a command line, where the
	// environment answers (host/contextactor.go).
	Actor   contextop.Actor
	Project string
	// ID names the rule, or a decision about it.
	ID string
	// To is a collection the recipe declares, or a folder of the project.
	To   string
	Note string
}

// RulesNarrowResult is what narrowing a rule recorded.
type RulesNarrowResult struct {
	Operation ContextOperation `json:"operation"`
	// To is what the person named, and Collection the collection whose point
	// the rule now holds at: the one named, or the one a folder's files
	// belong to.
	To         string `json:"to"`
	Collection string `json:"collection,omitempty"`
	// Applies is where the rule holds now: every collection at that point.
	Applies string `json:"applies"`
}

// FormatText says where the rule holds now.
func (r RulesNarrowResult) FormatText(w io.Writer) error {
	short := contextop.ShortID(r.Operation.Target)
	line := fmt.Sprintf("Narrowed #%s: it applies to %s now", short, r.Applies)
	if r.Collection != "" && r.Collection != r.To {
		line = fmt.Sprintf("Narrowed #%s to the %s collection, which %s/ belongs to: it applies to %s now", short, r.Collection, r.To, r.Applies)
	}
	if r.Operation.Landed != "" {
		line += " (" + r.Operation.Landed + ")"
	}
	_, err := fmt.Fprintln(w, line+".")
	return err
}

// NarrowRule limits a kept rule to the point a collection or a folder sits at.
//
// A rule's scope is a set of coordinates (contextop.Scope), the same one
// `kapi context review --widen-to` takes axes out of. Narrowing adds the
// coordinates of the point the collection or folder resolves to, and records
// it as a keep of the rule at that scope, the way a keep with a lifetime
// records one. The rule's concept in the terms store moves with it. A place
// that adds no coordinate the rule does not already hold cannot be told
// apart by a scope, and is refused with the reason.
func (a *App) NarrowRule(ctx context.Context, req RulesNarrowRequest) (RulesNarrowResult, error) {
	to := strings.TrimSuffix(filepath.ToSlash(strings.TrimSpace(req.To)), "/")
	if to == "" {
		return RulesNarrowResult{}, errors.New("name a collection or a folder to narrow the rule to, with --to")
	}
	st, err := a.rulesState(ctx, req.Project)
	if err != nil {
		return RulesNarrowResult{}, err
	}
	s, err := a.contextOps(ctx, st.recipe)
	if err != nil {
		return RulesNarrowResult{}, err
	}
	defer s.refreshRulesFiles(ctx)
	target, err := s.ledger.Subject(ctx, req.ID)
	if err != nil {
		return RulesNarrowResult{}, err
	}
	switch {
	case target.Status != contextop.StatusEstablished:
		return RulesNarrowResult{}, fmt.Errorf("operation %s is %s; keep it before narrowing it", contextop.ShortID(target.ID), target.Status)
	case target.Subject.Kind == contextop.SubjectMemory:
		return RulesNarrowResult{}, errors.New("a content-memory pair holds across the project and cannot be narrowed")
	case target.Scope.Level == contextop.LevelWorkspace:
		return RulesNarrowResult{}, errors.New("this rule holds across the workspace, in the workspace's rule store, which holds no collections: drop it and keep it in this project to narrow it")
	}

	point, err := a.narrowPoint(ctx, st, to)
	if err != nil {
		return RulesNarrowResult{}, err
	}
	rc, collection, coordinates, err := governedPoint(st.proj, point)
	if err != nil {
		return RulesNarrowResult{}, err
	}
	narrowed := target.Scope
	merged := maps.Clone(narrowed.Coordinates)
	if merged == nil {
		merged = map[string]string{}
	}
	for axis, value := range coordinates {
		held, ok := merged[axis]
		switch {
		case !ok:
			merged[axis] = value
		case held != value && !st.space.Contains(axis, held, value):
			return RulesNarrowResult{}, fmt.Errorf("this rule does not hold at %s: it holds at %s=%s, and %s sits at %s=%s", to, axis, held, to, axis, value)
		default:
			merged[axis] = value
		}
	}
	if maps.Equal(merged, narrowed.Coordinates) && !narrowed.AllProfiles {
		if maps.Equal(coordinates, st.places.Root) {
			return RulesNarrowResult{}, fmt.Errorf("%s sits at the project's own point: it has no profile, channel or coordinate of its own, and a rule holds at a set of coordinates, so nothing tells it apart from the rest of the project. Give it a profile or coordinates in kapi.yaml, or narrow to a collection that has them", to)
		}
		return RulesNarrowResult{}, fmt.Errorf("this rule already holds only where %s sits", to)
	}
	narrowed.Coordinates = merged
	narrowed.AllProfiles = false
	narrowed.Level = contextop.LevelProject

	actor, note, err := s.actorFor(ctx, req.Actor, req.Note)
	if err != nil {
		return RulesNarrowResult{}, err
	}
	written, err := s.ledger.Append(ctx, contextop.Record{
		Actor:   actor,
		Kind:    contextop.KindKeep,
		Target:  target.ID,
		Project: target.Project,
		Scope:   narrowed,
		Note:    note,
	})
	if err != nil {
		return RulesNarrowResult{}, teachRefusal(err)
	}
	target.Scope = narrowed
	if profile := rc.Ref().Profile; profile != "" {
		// The concept lands at the profile the narrowed point sits under.
		target.Basis.Profile = profile
	}
	landed, err := s.landAt(ctx, target, true)
	if err != nil {
		return RulesNarrowResult{}, err
	}
	applies, _ := st.applies(narrowed)
	return RulesNarrowResult{
		Operation: ContextOperation{Record: written, Landed: landed},
		To:        to, Collection: collection, Applies: applies,
	}, nil
}

// narrowPoint resolves what `--to` names: a collection the recipe declares,
// or a folder of the project, read at the point its source files sit at.
func (a *App) narrowPoint(_ context.Context, st *rulesState, to string) (project.GovernancePoint, error) {
	for _, c := range st.proj.Collections {
		if c.Name == to {
			return project.GovernancePoint{Collection: to, At: st.at}, nil
		}
	}
	info, err := os.Stat(filepath.Join(st.root, filepath.FromSlash(to)))
	if err != nil || !info.IsDir() {
		var names []string
		for _, c := range st.places.Collections {
			names = append(names, c.Name)
		}
		if len(names) == 0 {
			return project.GovernancePoint{}, fmt.Errorf("%s is neither a collection of this project nor a folder in it", to)
		}
		return project.GovernancePoint{}, fmt.Errorf("%s is neither a collection of this project nor a folder in it; its collections are %s", to, strings.Join(names, ", "))
	}
	// Every source file in the folder has to sit at one point, or no one
	// scope covers the folder alone.
	a.InitRegistries()
	pctx := project.NewProjectContext(st.proj, st.recipe)
	resolved, rerr := pctx.ResolveContent(a.FormatReg)
	if rerr != nil {
		return project.GovernancePoint{}, rerr
	}
	var first project.GovernancePoint
	var at map[string]string
	for _, rf := range resolved {
		rel := filepath.ToSlash(rf.Relative)
		if !strings.HasPrefix(rel, to+"/") {
			continue
		}
		point := project.GovernancePoint{Path: rel, At: st.at}
		_, _, coordinates, perr := governedPoint(st.proj, point)
		if perr != nil {
			continue
		}
		if at == nil {
			first, at = point, coordinates
			continue
		}
		if !maps.Equal(at, coordinates) {
			return project.GovernancePoint{}, fmt.Errorf("the files in %s sit at more than one point: narrow the rule to one of the collections there", to)
		}
	}
	if at == nil {
		return project.GovernancePoint{Path: to + "/" + rulesRootSample, At: st.at}, nil
	}
	return first, nil
}
