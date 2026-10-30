package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
)

// Deciding about suggestions, as `kapi rules` asks a person.
//
// Each suggestion becomes one question with the evidence as its options: for
// a name the files write more than one way, every form with the blocks and
// files that write it; for a disagreement, each side. Beside each option is
// what keeping it does: how many blocks a check then reports, and where the
// rule applies. The counts come from one read of the project's source files,
// made before the first question, with the matcher a check's usage count
// uses (formMatcher). Every answer is a ContextReviewRequest, the request
// `kapi context review` records its decisions with.

// The kinds of decision a person is asked about.
const (
	RuleDecisionName         = "name"
	RuleDecisionWord         = "word"
	RuleDecisionNote         = "note"
	RuleDecisionMemory       = "memory"
	RuleDecisionDisagreement = "disagreement"
)

// The questions a decision asks.
const (
	questionWhichForm = "Which one do your files use?"
	questionKeep      = "Keep this as a rule?"
	questionWhichSide = "Which one holds?"
)

// RulesDecideView is every suggestion waiting for a person, as questions.
type RulesDecideView struct {
	Project   string         `json:"project"`
	Decisions []RuleDecision `json:"decisions"`
	// Counts are the rules listing's counts, for the line a project with
	// nothing to decide prints.
	Counts RulesCounts `json:"counts"`
	// Hint is the command that records a suggestion, for a project with none
	// waiting.
	Hint string `json:"hint,omitempty"`

	st     *rulesState
	blocks []ruleBlock
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

	subject contextop.Subject
	scope   contextop.Scope
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

	subject contextop.Subject
	scope   contextop.Scope
}

// ruleBlock is one block of the project's source as the counts read it.
type ruleBlock struct {
	file        string
	coordinates map[string]string
	text        string
}

// DecideRules reads the suggestions waiting in a project as questions. It
// records nothing.
func (a *App) DecideRules(ctx context.Context, req RulesRequest) (*RulesDecideView, error) {
	st, err := a.rulesState(ctx, req.Project)
	if err != nil {
		return nil, err
	}
	digest, err := a.ContextDigest(ctx, ContextDigestRequest{Project: st.recipe})
	if err != nil {
		return nil, err
	}
	v := &RulesDecideView{Project: string(st.key), Decisions: []RuleDecision{}, st: st}

	seen := map[string]bool{}
	var items []DigestItem
	for _, c := range digest.Conflicts {
		for _, side := range c.Sides {
			seen[side.ID] = true
		}
		if len(c.Sides) > 1 {
			v.Decisions = append(v.Decisions, v.disagreement(c))
			continue
		}
		items = append(items, c.Sides...)
	}
	for _, t := range digest.Suggested {
		for _, g := range t.Groups {
			for _, it := range g.Items {
				if !seen[it.ID] && (it.Keepable || it.Droppable) {
					items = append(items, it)
					seen[it.ID] = true
				}
			}
		}
	}
	if needsBlocks(v.Decisions, items) {
		v.blocks = a.ruleBlocks(ctx, st)
	}
	// A disagreement's counts are read now the blocks are.
	for i := range v.Decisions {
		v.fill(&v.Decisions[i])
	}
	for _, it := range items {
		d := v.suggestion(it)
		v.fill(&d)
		v.Decisions = append(v.Decisions, d)
	}

	if len(v.Decisions) == 0 {
		listing, lerr := a.listRules(ctx, st)
		if lerr != nil {
			return nil, lerr
		}
		v.Counts = listing.Counts
		v.Hint = "kapi context note"
	}
	return v, nil
}

// needsBlocks reports whether any question counts uses of a wording.
func needsBlocks(decisions []RuleDecision, items []DigestItem) bool {
	if len(decisions) > 0 {
		return true
	}
	for _, it := range items {
		if it.Subject.Kind == contextop.SubjectTerm {
			return true
		}
	}
	return false
}

// disagreement asks which side of a disagreement holds.
func (v *RulesDecideView) disagreement(c DigestConflict) RuleDecision {
	d := RuleDecision{Kind: RuleDecisionDisagreement, Question: questionWhichSide, Reason: c.Reason}
	names := true
	for _, side := range c.Sides {
		r := v.st.byID[side.ID]
		if digestTheme(side.Subject) != DigestThemeNames {
			names = false
		}
		d.Options = append(d.Options, RuleOption{
			ID: side.ID, Short: side.Short, Sentence: side.Sentence,
			subject: side.Subject, scope: r.Scope,
		})
		if d.Seen == nil && side.Quote != nil {
			d.Seen = side.Quote
		}
		if d.scope.Level == "" {
			d.scope = r.Scope
		}
	}
	if names {
		d.Question = questionWhichForm
	}
	return d
}

// suggestion asks about one suggestion: which of its forms the files use,
// for a name, and whether to keep it otherwise.
func (v *RulesDecideView) suggestion(it DigestItem) RuleDecision {
	r := v.st.byID[it.ID]
	d := RuleDecision{
		ID: it.ID, Short: it.Short, Sentence: it.Sentence, Seen: it.Quote,
		Question: questionKeep, SuggestedBy: actorPhrase(it.NoticedBy, false),
		subject: it.Subject, scope: r.Scope,
	}
	switch {
	case it.Subject.Kind == contextop.SubjectMemory:
		d.Kind = RuleDecisionMemory
	case it.Subject.Kind == contextop.SubjectNote || it.Subject.Kind == contextop.SubjectNone:
		d.Kind = RuleDecisionNote
	case digestTheme(it.Subject) == DigestThemeNames:
		d.Kind, d.Question = RuleDecisionName, questionWhichForm
		rule, _ := it.Subject.Rule()
		for _, form := range ruleForms(rule) {
			d.Options = append(d.Options, RuleOption{Form: form, scope: r.Scope})
		}
	default:
		d.Kind = RuleDecisionWord
	}
	return d
}

// ruleForms are a name's forms: the one the rule says to write first, then
// the ones it avoids, each once.
func ruleForms(rule coreprofile.TermRule) []string {
	var out []string
	for _, f := range append([]string{rule.Replacement, rule.Term}, rule.Forms...) {
		if f = strings.TrimSpace(f); f != "" && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

// fill counts each option's uses and states what keeping it does.
func (v *RulesDecideView) fill(d *RuleDecision) {
	d.Applies, _ = v.st.applies(d.scope)
	switch d.Kind {
	case RuleDecisionName:
		rule, _ := d.subject.Rule()
		caseSensitive := rule.MatchesCase()
		for i := range d.Options {
			o := &d.Options[i]
			o.Blocks, o.Files = v.uses(d.scope, []string{o.Form}, caseSensitive)
			o.Consequence = v.Consequence(*d, o.Form)
		}
	case RuleDecisionDisagreement:
		best := -1
		for i := range d.Options {
			o := &d.Options[i]
			if rule, ok := o.subject.Rule(); ok && rule.Replacement != "" {
				o.Blocks, o.Files = v.uses(o.scope, []string{rule.Replacement}, rule.MatchesCase())
			}
			o.Consequence = v.keepConsequence(o.subject, o.scope, "")
			if best < 0 || o.Blocks > d.Options[best].Blocks {
				best = i
			}
		}
		d.Chosen = max(best, 0)
	default:
		d.Consequence = v.keepConsequence(d.subject, d.scope, "")
	}
}

// Consequence states what keeping a name with form as the one to write does.
// The form may be one the person typed rather than one of the options.
func (v *RulesDecideView) Consequence(d RuleDecision, form string) string {
	subject := d.subject
	if form != "" {
		subject, _ = editSubject(subject, form, nil)
	}
	return v.keepConsequence(subject, d.scope, form)
}

// keepConsequence says what keeping subject at scope does: the blocks a check
// then reports, and where it applies. label is the form a name is kept as.
func (v *RulesDecideView) keepConsequence(subject contextop.Subject, scope contextop.Scope, label string) string {
	applies, _ := v.st.applies(scope)
	where := " Applies to " + applies + "."
	switch subject.Kind {
	case contextop.SubjectNote, contextop.SubjectNone:
		return "If you keep it: writers and assistants read it where it applies. No check fails on it." + where
	case contextop.SubjectMemory:
		return "If you keep it: the content memory holds this wording." + where
	}
	rule, _ := subject.Rule()
	avoided := slices.DeleteFunc(append([]string{rule.Term}, rule.Forms...), func(f string) bool {
		return strings.TrimSpace(f) == "" || f == rule.Replacement
	})
	blocks, _ := v.uses(scope, avoided, rule.MatchesCase())
	lead := "If you keep it: "
	if label != "" {
		lead = "If you keep " + strconv.Quote(label) + ": "
	}
	var what string
	switch {
	case label != "" && blocks == 0:
		what = "no block uses " + otherForms(len(avoided)) + "."
	case label != "":
		what = otherForms(len(avoided)) + " " + become(len(avoided)) + " in " + blockCount(blocks) + "."
	case blocks == 0:
		what = "no block uses " + quotedAlternatives(avoided) + "."
	default:
		what = quotedAlternatives(avoided) + " " + become(len(avoided)) + " in " + blockCount(blocks) + "."
	}
	if rule.Advisory {
		what += " They report and fail nothing."
	}
	return lead + what + where
}

// otherForms names the forms a kept name avoids: "the other form", "the other
// 4 forms".
func otherForms(n int) string {
	if n == 1 {
		return "the other form"
	}
	return "the other " + strconv.Itoa(n) + " forms"
}

// become is the verb for n avoided forms turning into findings.
func become(n int) string {
	if n == 1 {
		return "becomes a finding"
	}
	return "become findings"
}

// blockCount renders a count of blocks.
func blockCount(n int) string {
	if n == 1 {
		return "1 block"
	}
	return strconv.Itoa(n) + " blocks"
}

// uses counts the blocks at scope that write any of forms, and the files
// they sit in.
func (v *RulesDecideView) uses(scope contextop.Scope, forms []string, caseSensitive bool) (blocks, files int) {
	re := formMatcher(forms, caseSensitive)
	if re == nil {
		return 0, 0
	}
	return countBlocks(v.blocks, scope, v.st, re)
}

func countBlocks(blocks []ruleBlock, scope contextop.Scope, st *rulesState, re *regexp.Regexp) (int, int) {
	n := 0
	inFiles := map[string]bool{}
	for _, b := range blocks {
		if !scope.CoversIn(st.space, b.coordinates) || !re.MatchString(b.text) {
			continue
		}
		n++
		inFiles[b.file] = true
	}
	return n, len(inFiles)
}

// ruleBlocks reads every block of the project's source once, with the
// coordinates of the point each file sits at. A file that will not read is
// left out: a count is advice, and not worth failing the question over.
func (a *App) ruleBlocks(ctx context.Context, st *rulesState) []ruleBlock {
	pctx := project.NewProjectContext(st.proj, st.recipe)
	resolved, err := pctx.ResolveContent(a.FormatReg)
	if err != nil {
		return nil
	}
	formats, err := a.newCheckFormats(st.cmd)
	if err != nil {
		return nil
	}
	var out []ruleBlock
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
			out = append(out, ruleBlock{file: rel, coordinates: coordinates, text: b.SourceText()})
		}
	}
	return out
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

// FormatText lists every question without asking it: what a terminal-less
// `kapi rules` prints, with the id each decision takes.
func (v *RulesDecideView) FormatText(w io.Writer) error {
	if len(v.Decisions) == 0 {
		_, err := fmt.Fprintf(w, "No suggestions to decide. %s.\nRecord one with: %s\n", v.Counts.Line(), v.Hint)
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
	_, err := fmt.Fprintln(w, "\nDecide with: kapi rules --keep <id>, --drop <id> or --later <id>")
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
	if len(d.Options) == 0 || d.Kind != RuleDecisionName {
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
		fmt.Fprintf(&b, "  %s %d) %s  %s in %s\n", mark, i+1, label, blockCount(o.Blocks), fileCount(o.Files))
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
	// Applies is where the rule holds now.
	Applies string `json:"applies"`
}

// FormatText says where the rule holds now.
func (r RulesNarrowResult) FormatText(w io.Writer) error {
	line := fmt.Sprintf("Narrowed #%s: it applies to %s now", contextop.ShortID(r.Operation.Target), r.Applies)
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
		return RulesNarrowResult{}, errors.New("a content-memory pair holds across the project: it has no scope to narrow")
	case target.Scope.Level == contextop.LevelWorkspace:
		return RulesNarrowResult{}, errors.New("this rule holds across the workspace, in the workspace's rule store, which holds no collections: drop it and keep it in this project to narrow it")
	}

	point, err := a.narrowPoint(ctx, st, to)
	if err != nil {
		return RulesNarrowResult{}, err
	}
	rc, _, coordinates, err := governedPoint(st.proj, point)
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
		if maps.Equal(coordinates, st.rootCoordinates) {
			return RulesNarrowResult{}, fmt.Errorf("%s sits at the project's own point: it has no profile, channel or coordinate of its own, and a rule's scope is a set of coordinates, so no scope tells it apart from the rest of the project. Give it a profile or coordinates in kapi.yaml, or narrow to a collection that has them", to)
		}
		return RulesNarrowResult{}, fmt.Errorf("this rule already holds only where %s sits", to)
	}
	narrowed.Coordinates = merged
	narrowed.AllProfiles = false
	narrowed.Level = contextop.LevelProject

	actor, note, err := s.actorFor(ctx, contextop.Actor{}, req.Note)
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
	return RulesNarrowResult{Operation: ContextOperation{Record: written, Landed: landed}, Applies: applies}, nil
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
		for _, c := range st.collections {
			names = append(names, c.name)
		}
		if len(names) == 0 {
			return project.GovernancePoint{}, fmt.Errorf("%s is neither a collection of this project nor a folder in it", to)
		}
		return project.GovernancePoint{}, fmt.Errorf("%s is neither a collection of this project nor a folder in it; its collections are %s", to, strings.Join(names, ", "))
	}
	// Every source file in the folder has to sit at one point, or no one
	// scope covers the folder alone.
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
