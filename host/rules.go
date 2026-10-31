package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/axis"
	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/rulesview"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/terms"
)

// `kapi rules`: the rules a project keeps, in a person's words.
//
// A rule is what the project holds its files to: a name or a word (a term
// rule, in the terms store or the operation log), the voice's measures and
// pattern rules, and the notes a person kept. Nothing here is stored apart:
// the listing reads the answer `kapi context <path>` gives at the project's
// own point, adds the rules the operation log holds elsewhere in the project,
// and names the suggestions waiting for a decision. Every decision goes
// through DecideContextReview, the call `kapi context review` makes.

// The groups a rules listing sorts its lines into.
const (
	RulesGroupNames     = "names"
	RulesGroupWords     = "words"
	RulesGroupTone      = "tone"
	RulesGroupSuggested = "suggested"
)

// rulesGroups are the groups in the order a listing prints them, with their
// headings.
var rulesGroups = []struct{ id, title string }{
	{RulesGroupNames, "Names"},
	{RulesGroupWords, "Words"},
	{RulesGroupTone, "Tone"},
	{RulesGroupSuggested, "Suggested"},
}

// RulesListing is every rule a project keeps, grouped, with the suggestions
// waiting for a decision.
type RulesListing struct {
	Project string      `json:"project"`
	Counts  RulesCounts `json:"counts"`
	// Groups holds every group that has a line, in the order Names, Words,
	// Tone, Suggested.
	Groups []RulesGroup `json:"groups"`
}

// RulesCounts counts a listing's lines: the rules in force (Rules), how many
// of those hold in only part of the project (Narrowed), and the suggestions
// waiting (Suggested).
type RulesCounts struct {
	Rules     int `json:"rules"`
	Suggested int `json:"suggested"`
	Narrowed  int `json:"narrowed"`
}

// Line renders the counts the way a listing opens: `4 rules · 1 suggested ·
// 0 narrowed`.
func (c RulesCounts) Line() string {
	rules := "rules"
	if c.Rules == 1 {
		rules = "rule"
	}
	return fmt.Sprintf("%d %s · %d suggested · %d narrowed", c.Rules, rules, c.Suggested, c.Narrowed)
}

// RulesGroup is one group of a listing.
type RulesGroup struct {
	Group string     `json:"group"`
	Title string     `json:"title"`
	Rules []RuleLine `json:"rules"`
}

// RuleLine is one rule or suggestion.
type RuleLine struct {
	// ID names the context operation the rule or suggestion was recorded as,
	// the id `kapi rules keep`, `drop` and `narrow` take. Empty for a rule held
	// only in the terms store or the voice profile.
	ID    string `json:"id,omitempty"`
	Short string `json:"short,omitempty"`
	Group string `json:"group"`
	// Sentence is the rule as a person reads it.
	Sentence string `json:"sentence"`
	// From names where the rule comes from: `terms`, `workspace`, `voice
	// <name>`, `kept by <who>, <day>`, `suggested by <who>`.
	From []string `json:"from"`
	// Applies says where the rule holds: "the whole project", "every project
	// in the workspace", or the collections and coordinates it is narrowed to.
	Applies string `json:"applies"`
	// Narrowed reports a rule that holds in only part of the project.
	Narrowed bool `json:"narrowed,omitempty"`
	// Fails reports a rule a check fails on. A suggestion, an advisory rule, a
	// note and a measure of the voice report and fail nothing.
	Fails bool `json:"fails"`
	// Status is the operation's status for a line found in the log: a rule
	// set aside reads "dropped". Empty for a rule in force.
	Status string `json:"status,omitempty"`

	// words are the wordings the line mentions, for `kapi rules find`.
	words []string
	// keeper and keptAt say who put a rule found in the log in force and when;
	// kept reports a line that has them. subject is the operation's subject kind.
	keeper  contextop.Actor
	keptAt  time.Time
	kept    bool
	subject contextop.SubjectKind
}

// Kept answers who put the rule in force and when, for a rule the operation
// log holds. A rule held only in the terms store or the voice profile, and a
// suggestion, answer false.
func (l RuleLine) Kept() (contextop.Actor, time.Time, bool) {
	return l.keeper, l.keptAt, l.kept
}

// SubjectKind is the kind of subject the operation behind the line is about:
// a term or a note. Empty for a line the log does not hold.
func (l RuleLine) SubjectKind() contextop.SubjectKind { return l.subject }

// FormatText renders a listing: the counts, then each group.
func (l RulesListing) FormatText(w io.Writer) error {
	if _, err := fmt.Fprintln(w, l.Counts.Line()); err != nil {
		return err
	}
	for _, g := range l.Groups {
		if _, err := fmt.Fprintf(w, "\n%s\n", g.Title); err != nil {
			return err
		}
		for _, r := range g.Rules {
			if _, err := fmt.Fprintln(w, "  "+r.text(r.Narrowed)); err != nil {
				return err
			}
		}
	}
	return nil
}

// text is one line as a listing prints it: the sentence, where it comes from,
// where it holds when asked, and the id a decision takes.
func (r RuleLine) text(withApplies bool) string {
	parts := []string{r.Sentence}
	if len(r.From) > 0 {
		parts = append(parts, strings.Join(r.From, " · "))
	}
	if withApplies && r.Applies != "" {
		parts = append(parts, "applies to "+r.Applies)
	}
	if r.Status != "" {
		parts = append(parts, r.Status)
	}
	if r.Short != "" {
		parts = append(parts, "#"+r.Short)
	}
	return strings.Join(parts, "  ")
}

// RulesRequest names the project a rules call is for: a recipe path, or empty
// to resolve the project the way every command does.
type RulesRequest struct {
	Project string
	// WithCounts fills a decide view's Counts while suggestions wait too, for
	// a surface that shows how many rules are kept beside the questions.
	WithCounts bool
	// Reuse is a decide view read earlier for the same project. Its blocks
	// are counted over again, rather than read from the files, while no
	// source file has changed since it read them.
	Reuse *RulesDecideView
}

// rulesState is what every rules call reads once: the project, its log as
// the person reading it sees it, and the answer at the project's own point.
type rulesState struct {
	cmd     *EnvCommand
	recipe  string
	root    string
	proj    *project.KapiProject
	key     workspace.ProjectKey
	space   axis.Space
	at      time.Time
	records []contextop.Record
	byID    map[string]contextop.Record
	acts    map[string][]contextop.Record
	// rootCoordinates are the coordinates of the project's own point, where
	// a rule that holds across the project answers.
	rootCoordinates map[string]string
	// collections are the coordinates of each named collection's point, in
	// recipe order.
	collections []rulesCollection
	// files are the project's source files, read only for a project whose
	// collections have no names to say where a rule applies with.
	files []rulesFile
}

// rulesFile is one source file of the project and the coordinates of the
// point it sits at.
type rulesFile struct {
	rel         string
	coordinates map[string]string
}

type rulesCollection struct {
	name        string
	coordinates map[string]string
}

// rulesState opens a project for a rules call.
func (a *App) rulesState(ctx context.Context, recipePath string) (*rulesState, error) {
	resolved, err := a.resolveRulesRecipe(ctx, recipePath)
	if err != nil {
		return nil, err
	}
	cmd, root, proj, err := a.rulesProject(ctx, resolved)
	if err != nil {
		return nil, err
	}
	s, err := a.contextOps(ctx, resolved)
	if err != nil {
		return nil, err
	}
	records, err := s.ledger.Records(ctx, contextop.Filter{})
	if err != nil {
		return nil, err
	}
	if reader, rerr := a.commandActor(); rerr == nil {
		records = seenBy(records, reader.Actor)
	}
	st := &rulesState{
		cmd:     cmd,
		recipe:  s.recipe,
		root:    root,
		proj:    proj,
		key:     s.key,
		space:   proj.AxisSpace(),
		at:      a.GovernanceInstant(),
		records: records,
		byID:    make(map[string]contextop.Record, len(records)),
	}
	for _, r := range records {
		st.byID[r.ID] = r
	}
	st.acts = rulesview.Acts(records, st.byID)
	if _, _, coordinates, perr := governedPoint(proj, project.GovernancePoint{Path: rulesRootSample, At: st.at}); perr == nil {
		st.rootCoordinates = coordinates
	}
	for _, c := range proj.Collections {
		if c.Name == "" {
			continue
		}
		_, _, coordinates, perr := governedPoint(proj, project.GovernancePoint{Collection: c.Name, At: st.at})
		if perr != nil {
			continue
		}
		st.collections = append(st.collections, rulesCollection{name: c.Name, coordinates: coordinates})
	}
	if len(st.collections) == 0 {
		a.InitRegistries()
		if files, rerr := project.NewProjectContext(proj, resolved).ResolveContent(a.FormatReg); rerr == nil {
			for _, rf := range files {
				rel := filepath.ToSlash(rf.Relative)
				if _, _, coordinates, perr := governedPoint(proj, project.GovernancePoint{Path: rel, At: st.at}); perr == nil {
					st.files = append(st.files, rulesFile{rel: rel, coordinates: coordinates})
				}
			}
		}
	}
	return st, nil
}

// applies says where a rule at scope holds, and whether that is less than the
// whole project.
func (st *rulesState) applies(scope contextop.Scope) (string, bool) {
	if scope.Level == contextop.LevelWorkspace {
		return "every project in the workspace", false
	}
	if scope.AllProfiles && len(scope.Coordinates) == 0 {
		return "the whole project", false
	}
	if len(st.collections) > 0 {
		var in []string
		for _, c := range st.collections {
			if scope.CoversIn(st.space, c.coordinates) {
				in = append(in, c.name)
			}
		}
		switch {
		case len(in) == len(st.collections) && scope.CoversIn(st.space, st.rootCoordinates):
			return "the whole project", false
		case len(in) == 0:
			return "no collection of this project", true
		}
		return strings.Join(in, ", "), true
	}
	if scope.CoversIn(st.space, st.rootCoordinates) {
		return "the whole project", false
	}
	if folders := st.coveredFolders(scope); len(folders) > 0 {
		return strings.Join(folders, ", "), true
	}
	return "no file of this project", true
}

// coveredFolders names the folders whose source files sit where scope holds,
// for a project that names no collection to say it with.
func (st *rulesState) coveredFolders(scope contextop.Scope) []string {
	var out []string
	for _, f := range st.files {
		if !scope.CoversIn(st.space, f.coordinates) {
			continue
		}
		dir := path.Dir(f.rel)
		if dir == "." {
			dir = "the project's root folder"
		}
		if !slices.Contains(out, dir) {
			out = append(out, dir)
		}
	}
	slices.Sort(out)
	return out
}

// established is every rule in force the project's log holds: term rules and
// notes a person kept or the evidence settled, newest first.
func (st *rulesState) established() []contextop.Record {
	var out []contextop.Record
	for _, r := range st.records {
		if r.Project != st.key || !r.Kind.Bears() || r.Kind == contextop.KindImport || !r.Established || !r.Status.Answers() {
			continue
		}
		if r.Subject.Kind != contextop.SubjectTerm && r.Subject.Kind != contextop.SubjectNote {
			continue
		}
		out = append(out, r)
	}
	return out
}

// keptBy says who put a rule in force and on which day: "kept by you,
// 2026-10-30", or what the evidence it settled on was.
func (st *rulesState) keptBy(r contextop.Record) string {
	at, how := rulesview.Establishment(r, st.acts[r.ID], st.byID)
	if len(how) == 0 {
		return ""
	}
	return strings.Join(how, ", ") + ", " + at.UTC().Format(time.DateOnly)
}

// markKept records on a line who put r in force and when, beside the phrase
// keptBy appends to its From.
func (st *rulesState) markKept(line *RuleLine, r contextop.Record) {
	at, how := rulesview.Establishment(r, st.acts[r.ID], st.byID)
	if len(how) == 0 {
		return
	}
	line.keeper, line.keptAt, line.kept = rulesview.EstablishedBy(r, st.acts[r.ID]), at.UTC(), true
	line.subject = r.Subject.Kind
}

// ListRules lists the rules a project keeps.
func (a *App) ListRules(ctx context.Context, req RulesRequest) (RulesListing, error) {
	st, err := a.rulesState(ctx, req.Project)
	if err != nil {
		return RulesListing{}, err
	}
	return a.listRules(ctx, st)
}

func (a *App) listRules(ctx context.Context, st *rulesState) (RulesListing, error) {
	answer, voice, concepts, err := a.rulesAnswer(st)
	if err != nil {
		return RulesListing{}, err
	}
	var lines []RuleLine
	matched := map[string]bool{}

	// The word rules in force at the project's own point, as `kapi context`
	// states them, each tied to the operation that put it in force where the
	// log holds one.
	established := st.established()
	for _, r := range answer.Rules {
		line := RuleLine{
			Group:    rulesGroupOf(r),
			Sentence: contextRuleSentence(r),
			From:     slices.Clone(r.From),
			Applies:  "the whole project",
			Fails:    answerRuleFails(r, concepts, voice),
			words:    append(append([]string{r.Say}, r.Not...), r.Also...),
		}
		if op, ok := matchEstablished(r, established, matched); ok {
			// The operation states the rule whole: a form the terms store
			// folds away with case is still one the check holds files to.
			line.ID, line.Short = op.ID, contextop.ShortID(op.ID)
			line.Sentence = rulesview.Sentence(op.Subject)
			line.words = append(line.words, subjectWords(op.Subject)...)
			line.Applies, line.Narrowed = st.applies(op.Scope)
			if op.Scope.Level != contextop.LevelWorkspace {
				// The answer labels a kept rule's case-only forms, which the
				// log enforces beside the store, as the workspace's.
				line.From = slices.DeleteFunc(line.From, func(f string) bool { return f == "workspace" })
			}
			if rule, rok := op.Rule(); rok {
				line.Fails = !rule.Advisory
			}
			if kept := st.keptBy(op); kept != "" {
				line.From = append(line.From, kept)
				st.markKept(&line, op)
			}
		} else if slices.Equal(r.From, []string{"workspace"}) {
			line.Applies = "every project in the workspace"
		}
		lines = append(lines, line)
	}

	// The rules the log holds that the answer at the project's point does not
	// state: a rule narrowed to a part of the project, and a rule the terms
	// store cannot hold, such as one whose forms differ only in case.
	for _, r := range established {
		if matched[r.ID] || r.Subject.Kind != contextop.SubjectTerm {
			continue
		}
		rule, _ := r.Rule()
		line := st.opLine(r)
		line.Fails = !rule.Advisory
		lines = append(lines, line)
	}

	// The voice: its measures and pattern rules.
	lines = append(lines, voiceLines(voice)...)

	// The notes a person kept, where they hold.
	noted := map[string]bool{}
	for _, n := range answer.EstablishedNotes {
		noted[n.Operation] = true
	}
	for _, r := range established {
		if r.Subject.Kind != contextop.SubjectNote {
			continue
		}
		line := st.opLine(r)
		if noted[r.ID] {
			line.Applies, line.Narrowed = "the whole project", false
		}
		lines = append(lines, line)
	}

	// The suggestions waiting for a decision.
	lines = append(lines, st.suggestionLines()...)

	return st.listing(lines), nil
}

// listing groups lines and counts them.
func (st *rulesState) listing(lines []RuleLine) RulesListing {
	out := RulesListing{Project: string(st.key), Groups: []RulesGroup{}}
	for _, g := range rulesGroups {
		group := RulesGroup{Group: g.id, Title: g.title, Rules: []RuleLine{}}
		for _, l := range lines {
			if l.Group == g.id {
				group.Rules = append(group.Rules, l)
			}
		}
		if len(group.Rules) == 0 {
			continue
		}
		out.Groups = append(out.Groups, group)
	}
	for _, l := range lines {
		switch {
		case l.Group == RulesGroupSuggested:
			out.Counts.Suggested++
		default:
			out.Counts.Rules++
			if l.Narrowed {
				out.Counts.Narrowed++
			}
		}
	}
	return out
}

// opLine renders one operation of the log as a line.
func (st *rulesState) opLine(r contextop.Record) RuleLine {
	line := RuleLine{
		ID:       r.ID,
		Short:    contextop.ShortID(r.ID),
		Group:    rulesSubjectGroup(r.Subject),
		Sentence: rulesview.Sentence(r.Subject),
		words:    subjectWords(r.Subject),
	}
	if r.Established {
		if kept := st.keptBy(r); kept != "" {
			line.From = append(line.From, kept)
			st.markKept(&line, r)
		}
	}
	line.Applies, line.Narrowed = st.applies(r.Scope)
	return line
}

// suggestionLines are the suggestions waiting for a decision, the ones `kapi
// rules` asks about, newest first.
func (st *rulesState) suggestionLines() []RuleLine {
	var out []RuleLine
	for _, r := range st.records {
		if r.Project != st.key || !r.Kind.Bears() || r.Kind == contextop.KindImport || r.Established ||
			!r.Status.Advises() || r.Subject.Kind == contextop.SubjectNone {
			continue
		}
		line := st.opLine(r)
		line.Group = RulesGroupSuggested
		line.From = []string{"suggested by " + actorPhrase(r.Actor, false)}
		line.Narrowed = false
		if r.Status == contextop.StatusContested {
			line.Status = "contested"
		}
		out = append(out, line)
	}
	return out
}

// rulesAnswer reads the answer `kapi context` gives at the project's own
// point, and the voice profile in force there.
func (a *App) rulesAnswer(st *rulesState) (*ContextAnswer, *coreprofile.VoiceProfile, []terms.Concept, error) {
	req := ContextPointRequest{Path: filepath.Join(st.root, rulesRootSample), Limit: 1 << 16, rulesOnly: true}
	src, release := a.ContextSourcesAt(st.cmd, req)
	defer release()
	res, err := ResolveContextAt(CmdContext(st.cmd), src, req)
	if err != nil {
		return nil, nil, nil, err
	}
	return res, src.Voice, src.Concepts, nil
}

// answerRuleFails reports whether a check fails on a word rule of the answer:
// a rule that avoids a wording fails unless the concept or the voice file
// holding it marks it advisory. A rule that only names the wording to use
// avoids nothing, so nothing fails on it.
func answerRuleFails(r ContextRule, concepts []terms.Concept, voice *coreprofile.VoiceProfile) bool {
	if len(r.Not) == 0 {
		return false
	}
	avoided := func(word string) bool {
		return slices.ContainsFunc(r.Not, func(n string) bool { return fold(n) == fold(word) })
	}
	if slices.Contains(r.From, "terms") {
		for _, c := range concepts {
			for _, t := range c.Terms {
				if avoided(t.Text) {
					return !c.Advisory
				}
			}
		}
	}
	for _, t := range voice.CarriedTerms().Rules {
		if avoided(t.Term) {
			return !t.Advisory
		}
	}
	return true
}

// matchEstablished finds the operation in the log that put a word rule of the
// answer in force: a term rule saying the same wording, avoiding a wording the
// line avoids. Each operation matches one line.
func matchEstablished(r ContextRule, established []contextop.Record, matched map[string]bool) (contextop.Record, bool) {
	for _, op := range established {
		rule, ok := op.Rule()
		if !ok || matched[op.ID] || fold(rule.Replacement) != fold(r.Say) {
			continue
		}
		avoided := append([]string{rule.Term}, rule.Forms...)
		if r.Say != "" && len(r.Not) == 0 && rule.Replacement == "" {
			matched[op.ID] = true
			return op, true
		}
		for _, n := range r.Not {
			if slices.ContainsFunc(avoided, func(f string) bool { return fold(f) == fold(n) }) {
				matched[op.ID] = true
				return op, true
			}
		}
	}
	return contextop.Record{}, false
}

// rulesGroupOf sorts a word rule of the answer: a name when the wordings it
// avoids are spellings of the one it uses, a word otherwise.
func rulesGroupOf(r ContextRule) string {
	if r.Say == "" || len(r.Not) == 0 {
		return RulesGroupWords
	}
	want := rulesview.SpellingKey(r.Say)
	for _, n := range r.Not {
		if rulesview.SpellingKey(n) == want {
			return RulesGroupNames
		}
	}
	return RulesGroupWords
}

// rulesSubjectGroup sorts a subject of the log the same way.
func rulesSubjectGroup(s contextop.Subject) string {
	switch rulesview.ThemeOf(s) {
	case DigestThemeNames:
		return RulesGroupNames
	case DigestThemeWriting:
		return RulesGroupTone
	}
	return RulesGroupWords
}

// contextRuleSentence states a word rule of the answer as a sentence.
func contextRuleSentence(r ContextRule) string {
	switch {
	case r.Say != "" && len(r.Not) > 0:
		return "Write " + r.Say + ", not " + rulesview.OrList(r.Not) + "."
	case r.Say != "":
		return "Write " + r.Say + "."
	}
	return "Avoid " + rulesview.OrList(r.Not) + "."
}

// subjectWords are the wordings a subject mentions.
func subjectWords(s contextop.Subject) []string {
	if rule, ok := s.Rule(); ok {
		return append([]string{rule.Term, rule.Replacement}, rule.Forms...)
	}
	if s.Memory != nil {
		return []string{s.Memory.Source, s.Memory.Target}
	}
	return []string{s.Text}
}

// voiceLines are a voice profile's measures and pattern rules, one line each.
func voiceLines(v *coreprofile.VoiceProfile) []RuleLine {
	if v == nil {
		return nil
	}
	from := []string{"voice " + v.Name}
	var out []RuleLine
	add := func(sentence string, fails bool, words ...string) {
		out = append(out, RuleLine{
			Group: RulesGroupTone, Sentence: sentence, From: from, Applies: "the whole project",
			Fails: fails, words: append(words, sentence),
		})
	}
	if v.Tone.Formality != "" {
		add("Formality: "+v.Tone.Formality+".", false)
	}
	if len(v.Tone.Personality) > 0 {
		add("Personality: "+strings.Join(v.Tone.Personality, ", ")+".", false, v.Tone.Personality...)
	}
	if v.Style.SentenceLength != "" {
		add("Sentence length: "+v.Style.SentenceLength+".", false)
	}
	if v.Style.PersonPOV != "" {
		add("Point of view: "+strings.ReplaceAll(v.Style.PersonPOV, "_", " ")+".", false)
	}
	if v.Style.Contractions != "" {
		add("Contractions: "+v.Style.Contractions+".", false)
	}
	if v.Style.ActiveVoice {
		add("Active voice.", false)
	}
	for _, p := range v.Style.ProhibitedPatterns {
		add("Avoid: "+patternText(p)+".", p.Failing(), p.Regex)
	}
	for _, p := range v.Style.RequiredPatterns {
		add("Require: "+patternText(p)+".", p.Failing(), p.Regex)
	}
	return out
}

// patternText names a pattern rule by its description, or by its expression
// when it has none.
func patternText(p coreprofile.Pattern) string {
	if d := strings.TrimSuffix(strings.TrimSpace(p.Description), "."); d != "" {
		return d
	}
	return "/" + p.Regex + "/"
}

// RulesFound is what `kapi rules find` answers: the rules, suggestions and
// decisions that mention a word.
type RulesFound struct {
	Query string `json:"query"`
	// Rules are the rules in force that mention it, Suggested the suggestions
	// waiting, and Decided the suggestions and rules a person set aside.
	Rules     []RuleLine `json:"rules"`
	Suggested []RuleLine `json:"suggested"`
	Decided   []RuleLine `json:"decided"`
}

// FormatText renders what was found, each line with where it applies.
func (f RulesFound) FormatText(w io.Writer) error {
	if len(f.Rules)+len(f.Suggested)+len(f.Decided) == 0 {
		_, err := fmt.Fprintf(w, "No rule, suggestion or decision mentions %q.\n", f.Query)
		return err
	}
	for _, sec := range []struct {
		title string
		lines []RuleLine
	}{{"Rules", f.Rules}, {"Suggested", f.Suggested}, {"Decided against", f.Decided}} {
		if len(sec.lines) == 0 {
			continue
		}
		if _, err := fmt.Fprintln(w, sec.title); err != nil {
			return err
		}
		for _, l := range sec.lines {
			if _, err := fmt.Fprintln(w, "  "+l.text(true)); err != nil {
				return err
			}
		}
	}
	return nil
}

// RulesFindRequest asks which rules mention a word.
type RulesFindRequest struct {
	Project string
	Word    string
}

// FindRules lists the rules, suggestions and recorded decisions that mention
// a word, matching with case folded, as `kapi context search` matches.
func (a *App) FindRules(ctx context.Context, req RulesFindRequest) (RulesFound, error) {
	q := fold(req.Word)
	out := RulesFound{Query: req.Word, Rules: []RuleLine{}, Suggested: []RuleLine{}, Decided: []RuleLine{}}
	if q == "" {
		return out, errors.New("name a word to find")
	}
	st, err := a.rulesState(ctx, req.Project)
	if err != nil {
		return out, err
	}
	listing, err := a.listRules(ctx, st)
	if err != nil {
		return out, err
	}
	for _, g := range listing.Groups {
		for _, l := range g.Rules {
			if !lineMentions(l, q) {
				continue
			}
			if l.Group == RulesGroupSuggested {
				out.Suggested = append(out.Suggested, l)
			} else {
				out.Rules = append(out.Rules, l)
			}
		}
	}
	for _, r := range st.records {
		if r.Project != st.key || !r.Kind.Bears() || r.Status != contextop.StatusDropped || !mentions(r, q) {
			continue
		}
		line := st.opLine(r)
		line.Status = "not a rule"
		for _, act := range slices.Backward(st.acts[r.ID]) {
			if act.Kind == contextop.KindDrop {
				line.From = []string{"decided by " + actorPhrase(act.Actor, false) + ", " + act.At.UTC().Format(time.DateOnly)}
				break
			}
		}
		out.Decided = append(out.Decided, line)
	}
	return out, nil
}

// lineMentions reports whether a line names a folded query.
func lineMentions(l RuleLine, q string) bool {
	for _, w := range append(slices.Clone(l.words), l.Sentence) {
		if w != "" && strings.Contains(fold(w), q) {
			return true
		}
	}
	return false
}
