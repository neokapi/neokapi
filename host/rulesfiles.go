package host

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/agentrules"
	"github.com/neokapi/neokapi/core/project"
)

// The rules files: a project's context, written where agents load it without a
// tool call (AD C-06, S-03).
//
// kapi writes the rules that hold in a folder into the AGENTS.md and CLAUDE.md
// there: the project root for the rules that hold everywhere, and one folder
// per collection whose rules differ from the root's. Each file states only what
// holds in its folder: the voice in force, what to write and what not, the
// wording a rule held elsewhere leaves as it is here, and one line pointing to
// `kapi check`. The context stays the source; the files are a projection of
// it, refreshed when it changes, and `kapi check` stays the gate.
//
// kapi owns one delimited section of each file (core/agentrules) and leaves
// the rest to the person who wrote it. A file kapi created and no longer needs
// is removed only when nothing but its section is in it. The output is
// deterministic and budgeted, so a refresh that changes nothing changes no
// byte and the files stay short enough to load on every session.

const (
	// rulesFileRules caps the rules one section lists, rulesFileKeep the
	// wordings it says to keep, and rulesFileFolders the folders the root
	// file names. What does not fit is counted, with the command that lists
	// it all.
	rulesFileRules   = 20
	rulesFileKeep    = 10
	rulesFileFolders = 15
	// rulesFileVoiceLines caps the lines of the voice brief.
	rulesFileVoiceLines = 16
	// rulesFileLineMax caps the length of one line of a section, in
	// characters, so stored text cannot fill an agent's context.
	rulesFileLineMax = 1000
	// rulesRootSample is the path the root's answer is read for. The rules
	// files are never content (core/ignore), so it resolves at the project's
	// default point whatever the collections claim.
	rulesRootSample = "AGENTS.md"
)

// RulesFileAction is what writing the rules did to one file.
type RulesFileAction string

const (
	// RulesFileCreated: the file did not exist and now holds kapi's section.
	RulesFileCreated RulesFileAction = "created"
	// RulesFileUpdated: kapi's section was added to the file or rewritten.
	RulesFileUpdated RulesFileAction = "updated"
	// RulesFileUnchanged: the file already held this section.
	RulesFileUnchanged RulesFileAction = "unchanged"
	// RulesFileRemoved: the folder has no rules of its own any more, so the
	// section was taken out and the person's own text left in place.
	RulesFileRemoved RulesFileAction = "removed"
	// RulesFileDeleted: the folder has no rules of its own any more and the
	// file held nothing but kapi's section, so the file was deleted.
	RulesFileDeleted RulesFileAction = "deleted"
)

// RulesFile is one file the rules were written to.
type RulesFile struct {
	// Path is project-relative and slash-separated.
	Path   string          `json:"path"`
	Action RulesFileAction `json:"action"`
}

// RulesFilesResult reports what writing the rules files did.
type RulesFilesResult struct {
	// Files are every rules file kapi wrote or considered, by path.
	Files []RulesFile `json:"files"`
}

// Changed lists the files a write changed.
func (r *RulesFilesResult) Changed() []RulesFile {
	if r == nil {
		return nil
	}
	var out []RulesFile
	for _, f := range r.Files {
		if f.Action != RulesFileUnchanged {
			out = append(out, f)
		}
	}
	return out
}

// FormatText lists every file and what happened to it.
func (r *RulesFilesResult) FormatText(w io.Writer) error {
	if r == nil || len(r.Files) == 0 {
		_, err := fmt.Fprintln(w, "No rules files: the project's context holds no rules yet.")
		return err
	}
	for _, f := range r.Files {
		if _, err := fmt.Fprintf(w, "  rules:  %s (%s)\n", f.Path, f.Action); err != nil {
			return err
		}
	}
	return nil
}

// rulesSection is one point's rules, as a section of a folder's file states
// them.
type rulesSection struct {
	// Pattern heads the section when a folder holds files at more than one
	// point. Empty for a folder's only section.
	Pattern string
	// Voice is the voice brief, empty when the folder keeps the root's voice.
	Voice string
	// VoiceReplaces says the voice here takes the place of the root's.
	VoiceReplaces bool
	// NoVoice says no voice is bound here although the root binds one.
	NoVoice bool
	// Rules are what to write and what not.
	Rules []ContextRule
	// Keep are the wordings a rule held elsewhere leaves as they are.
	Keep []ContextElsewhere
}

func (s rulesSection) empty() bool {
	return s.Voice == "" && !s.NoVoice && len(s.Rules) == 0 && len(s.Keep) == 0
}

// rulesFolder is what one folder's files say.
type rulesFolder struct {
	Dir      string
	Sections []rulesSection
}

// rulesPlan is every folder's rules for one project.
type rulesPlan struct {
	root     rulesSection
	folders  []rulesFolder
	comments bool
}

// rulesPointAnswer is the answer at one point, as the rules files read it.
type rulesPointAnswer struct {
	voiceName string
	voice     string
	rules     []ContextRule
	elsewhere []ContextElsewhere
}

// rulesFilesMu serializes writes to a project's rules files, so two decisions
// made at once cannot interleave their writes.
var rulesFilesMu sync.Mutex

// WriteRulesFiles writes the project's rules files: the root's, and one per
// folder whose rules differ from the root's. It removes kapi's section from a
// rules file in a folder that no longer has rules of its own, and deletes such
// a file when nothing else is in it. recipePath names the project; empty
// resolves it the way every command does.
func (a *App) WriteRulesFiles(ctx context.Context, recipePath string) (*RulesFilesResult, error) {
	cmd, root, proj, err := a.rulesProject(ctx, recipePath)
	if err != nil {
		return nil, err
	}
	plan, err := a.planRulesFiles(cmd, root, proj)
	if err != nil {
		return nil, err
	}
	rulesFilesMu.Lock()
	defer rulesFilesMu.Unlock()
	return writeRulesPlan(root, plan)
}

// RefreshRulesFiles rewrites the rules files of a project that has them: one
// whose root AGENTS.md or CLAUDE.md holds kapi's section. A project that has
// none was set up without them, and is left alone. It reports nil when it
// wrote nothing for that reason.
func (a *App) RefreshRulesFiles(ctx context.Context, recipePath string) (*RulesFilesResult, error) {
	resolved, err := a.resolveRulesRecipe(ctx, recipePath)
	if err != nil {
		return nil, err
	}
	if !HasRulesFiles(filepath.Dir(resolved)) {
		return nil, nil
	}
	return a.WriteRulesFiles(ctx, resolved)
}

// HasRulesFiles reports whether a project root's AGENTS.md or CLAUDE.md holds
// kapi's section, which is what makes a refresh keep the rules files current.
func HasRulesFiles(root string) bool {
	for _, name := range agentrules.Names {
		if doc, err := os.ReadFile(filepath.Join(root, name)); err == nil && agentrules.Holds(doc) {
			return true
		}
	}
	return false
}

// refreshRulesFilesQuietly refreshes a project's rules files after its context
// changed, and leaves a failure unreported: the decision that changed the
// context stands, and the next refresh writes the files again.
func (a *App) refreshRulesFilesQuietly(ctx context.Context, recipePath string) {
	if a.rulesFilesHeld(recipePath) {
		return
	}
	_, _ = a.RefreshRulesFiles(ctx, recipePath)
}

// holdRulesFiles defers the refreshes a run of decisions would make to its
// end, so a review that keeps ten rules writes the files once. The returned
// function releases the hold and refreshes, once, the projects whose context
// changed while it was held.
func (a *App) holdRulesFiles() func(ctx context.Context) {
	a.rulesHold.Lock()
	a.rulesHeld++
	a.rulesHold.Unlock()
	return func(ctx context.Context) {
		a.rulesHold.Lock()
		a.rulesHeld--
		var pending []string
		if a.rulesHeld == 0 {
			for recipe := range a.rulesPending {
				pending = append(pending, recipe)
			}
			a.rulesPending = nil
		}
		a.rulesHold.Unlock()
		slices.Sort(pending)
		for _, recipe := range pending {
			_, _ = a.RefreshRulesFiles(ctx, recipe)
		}
	}
}

// rulesFilesHeld reports whether refreshes are held, noting the project for
// the refresh the hold makes when it ends.
func (a *App) rulesFilesHeld(recipePath string) bool {
	a.rulesHold.Lock()
	defer a.rulesHold.Unlock()
	if a.rulesHeld == 0 {
		return false
	}
	if a.rulesPending == nil {
		a.rulesPending = map[string]bool{}
	}
	a.rulesPending[recipePath] = true
	return true
}

// resolveRulesRecipe resolves the recipe a rules write is for.
func (a *App) resolveRulesRecipe(ctx context.Context, recipePath string) (string, error) {
	cmd := NewEnvCommand(ctx, "rules")
	cmd.Flags().String(projectFlagName, recipePath, "")
	resolved, err := ResolveProjectPath(cmd)
	if err != nil {
		return "", err
	}
	if resolved == "" {
		return "", errors.New("no kapi project: the rules files belong to a project")
	}
	return resolved, nil
}

// rulesProject opens the project a rules write is for, with the command the
// answers are read through.
func (a *App) rulesProject(ctx context.Context, recipePath string) (*EnvCommand, string, *project.KapiProject, error) {
	resolved, err := a.resolveRulesRecipe(ctx, recipePath)
	if err != nil {
		return nil, "", nil, err
	}
	proj, err := project.LoadWithOptions(resolved, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, "", nil, fmt.Errorf("load project for the rules files: %w", err)
	}
	cmd := NewEnvCommand(ctx, "rules")
	cmd.Flags().String(projectFlagName, resolved, "")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd, filepath.Dir(resolved), proj, nil
}

// planRulesFiles reads the answer at the root and at every place the
// collections read, and decides what each folder's files say.
func (a *App) planRulesFiles(cmd Command, root string, proj *project.KapiProject) (rulesPlan, error) {
	plan := rulesPlan{comments: proj.DeclaresCommentPoint()}
	at := a.GovernanceInstant()

	// One answer per point: places that resolve to the same point say the
	// same thing, and a project with many collections has few points.
	answers := map[string]rulesPointAnswer{}
	answerFor := func(sample string) (rulesPointAnswer, error) {
		key := rulesPointKey(proj, sample, at)
		if ans, ok := answers[key]; ok {
			return ans, nil
		}
		ans, err := a.rulesAnswerAt(cmd, filepath.Join(root, filepath.FromSlash(sample)))
		if err != nil {
			return rulesPointAnswer{}, err
		}
		answers[key] = ans
		return ans, nil
	}

	base, err := answerFor(rulesRootSample)
	if err != nil {
		return plan, err
	}
	plan.root = rulesSection{Voice: base.voice, Rules: base.rules}

	byDir := map[string][]rulesSection{}
	seen := map[string]map[string]bool{}
	for _, place := range a.rulePlaces(proj, root) {
		if place.Dir != "" {
			if info, serr := os.Stat(filepath.Join(root, filepath.FromSlash(place.Dir))); serr != nil || !info.IsDir() {
				continue
			}
		}
		key := rulesPointKey(proj, place.Sample, at)
		if seen[place.Dir][key] {
			continue
		}
		if seen[place.Dir] == nil {
			seen[place.Dir] = map[string]bool{}
		}
		seen[place.Dir][key] = true
		ans, aerr := answerFor(place.Sample)
		if aerr != nil {
			return plan, aerr
		}
		section := deltaSection(base, ans)
		section.Pattern = place.Pattern
		byDir[place.Dir] = append(byDir[place.Dir], section)
	}

	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		var sections []rulesSection
		for _, s := range byDir[dir] {
			if !s.empty() && !slices.ContainsFunc(sections, func(o rulesSection) bool { return sameSection(o, s) }) {
				sections = append(sections, s)
			}
		}
		if len(sections) == 0 {
			continue
		}
		sort.SliceStable(sections, func(i, j int) bool { return sections[i].Pattern < sections[j].Pattern })
		if len(sections) == 1 && dir != "" {
			sections[0].Pattern = ""
		}
		// An agent loads the rules files of every folder above the one it
		// works in, so a folder that would repeat the nearest folder above it
		// with rules of its own needs no file.
		if parent := nearestRulesFolder(plan.folders, dir); parent != nil && sameSections(parent.Sections, sections) {
			continue
		}
		plan.folders = append(plan.folders, rulesFolder{Dir: dir, Sections: sections})
	}
	return plan, nil
}

// nearestRulesFolder is the closest folder above dir that has rules of its
// own, among folders sorted by path.
func nearestRulesFolder(folders []rulesFolder, dir string) *rulesFolder {
	for i := len(folders) - 1; i >= 0; i-- {
		f := &folders[i]
		if f.Dir != "" && strings.HasPrefix(dir, f.Dir+"/") {
			return f
		}
	}
	return nil
}

// sameSections reports whether two folders state the same sections.
func sameSections(a, b []rulesSection) bool {
	return slices.EqualFunc(a, b, func(x, y rulesSection) bool {
		return x.Pattern == y.Pattern && sameSection(x, y)
	})
}

// rulesPointKey names the point a path resolves to, so places at one point
// share one answer.
func rulesPointKey(proj *project.KapiProject, sample string, at time.Time) string {
	point := project.GovernancePoint{Path: sample, At: at}
	rc, err := proj.ResolveGovernanceFor(point)
	if err != nil || rc == nil {
		return "path:" + sample
	}
	coordinates := pointCoordinates(proj, rc, point)
	axes := make([]string, 0, len(coordinates))
	for axis, value := range coordinates {
		axes = append(axes, axis+"="+value)
	}
	sort.Strings(axes)
	return rc.Ref().String() + "|" + rc.VoiceField + "|" + rc.TermStore + "|" + strings.Join(axes, ",")
}

// rulesAnswerAt reads the rules in force for the file at abs.
func (a *App) rulesAnswerAt(cmd Command, abs string) (rulesPointAnswer, error) {
	req := ContextPointRequest{Path: abs, Limit: 1 << 16, rulesOnly: true}
	src, release := a.ContextSourcesAt(cmd, req)
	defer release()
	res, err := ResolveContextAt(CmdContext(cmd), src, req)
	if err != nil {
		return rulesPointAnswer{}, err
	}
	// Only rules in force reach the files: the answer's rules are the terms
	// store, the rules a person established or widened, and the voice, never
	// the suggestions nobody has decided about (rulesOnly leaves those out).
	// An agent's note stays a suggestion, answered by `kapi context <path>`
	// and review, until a person's signal establishes it.
	out := rulesPointAnswer{voice: strings.TrimSpace(res.VoiceBrief), elsewhere: res.Elsewhere, rules: orderRules(res.Rules)}
	if res.Voice != nil {
		out.voiceName = res.Voice.Name
	}
	return neutraliseAnswer(out), nil
}

// neutraliseAnswer neutralises every stored value an answer carries into the
// rules files.
func neutraliseAnswer(in rulesPointAnswer) rulesPointAnswer {
	out := rulesPointAnswer{voiceName: in.voiceName, voice: neutraliseBlock(in.voice)}
	for _, e := range in.elsewhere {
		e.Keep = neutraliseAll(e.Keep)
		e.Instead = neutraliseLine(e.Instead)
		e.HeldIn = neutraliseAll(e.HeldIn)
		out.elsewhere = append(out.elsewhere, e)
	}
	for _, r := range in.rules {
		r.Say = neutraliseLine(r.Say)
		r.Also = neutraliseAll(r.Also)
		r.Not = neutraliseAll(r.Not)
		r.Note = neutraliseLine(r.Note)
		out.rules = append(out.rules, r)
	}
	return out
}

// The rules files are instructions every agent in the tree loads, and what
// they state is text somebody stored: a term, a note, a voice brief. Stored
// text is written as plain lines inside kapi's section, so it cannot close the
// section, open an HTML comment or a code block, or start a heading that reads
// as an instruction of its own, and no line runs past rulesFileLineMax.

// neutraliseLine renders a stored value as part of one line: whitespace,
// newlines included, collapsed to single spaces.
func neutraliseLine(s string) string {
	return neutralise(strings.Join(strings.Fields(s), " "))
}

// neutraliseAll neutralises each of a list of stored values.
func neutraliseAll(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = neutraliseLine(s)
	}
	return out
}

// neutraliseBlock neutralises stored text line by line, keeping its lines.
func neutraliseBlock(text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = neutralise(l)
	}
	return strings.Join(lines, "\n")
}

// neutralise removes the markup that could change how a line reads: comment
// delimiters, code fences and a leading heading marker.
func neutralise(line string) string {
	for {
		next := line
		for _, m := range []string{"<!--", "-->", "```", "~~~"} {
			next = strings.ReplaceAll(next, m, "")
		}
		if next == line {
			break
		}
		line = next
	}
	if t := strings.TrimLeft(line, " \t"); strings.HasPrefix(t, "#") {
		line = strings.TrimLeft(t, "# \t")
	}
	return line
}

// capLineLength shortens every line of a rendered section to rulesFileLineMax
// characters.
func capLineLength(section string) string {
	lines := strings.Split(section, "\n")
	for i, l := range lines {
		if r := []rune(l); len(r) > rulesFileLineMax {
			lines[i] = string(r[:rulesFileLineMax-1]) + "…"
		}
	}
	return strings.Join(lines, "\n")
}

// orderRules puts the rules that rule a wording out ahead of the conventions,
// keeping the answer's order within each, so a capped list keeps the rules an
// agent must not break.
func orderRules(rules []ContextRule) []ContextRule {
	out := make([]ContextRule, 0, len(rules))
	for _, r := range rules {
		if len(r.Not) > 0 {
			out = append(out, r)
		}
	}
	for _, r := range rules {
		if len(r.Not) == 0 {
			out = append(out, r)
		}
	}
	return out
}

// deltaSection is what a place's files hold beyond the root's: the rules the
// root does not state, the root's rules that do not hold here as wording to
// keep, the rules held elsewhere that leave a wording as it is here, and the
// voice when it is not the root's.
func deltaSection(base, here rulesPointAnswer) rulesSection {
	var s rulesSection
	baseKeys := map[string]bool{}
	for _, r := range base.rules {
		baseKeys[ruleKey(r)] = true
	}
	hereKeys := map[string]bool{}
	mentioned := map[string]bool{}
	for _, r := range here.rules {
		hereKeys[ruleKey(r)] = true
		for _, w := range append(append([]string{r.Say}, r.Not...), r.Also...) {
			mentioned[fold(w)] = true
		}
		if !baseKeys[ruleKey(r)] {
			s.Rules = append(s.Rules, r)
		}
	}
	for _, r := range base.rules {
		if hereKeys[ruleKey(r)] || r.Say == "" || len(r.Not) == 0 || mentioned[fold(r.Say)] {
			continue
		}
		var keep []string
		for _, n := range oldWordings(r.Not, r.Say) {
			if !mentioned[fold(n)] {
				keep = append(keep, n)
			}
		}
		if len(keep) > 0 {
			s.Keep = append(s.Keep, ContextElsewhere{Keep: keep, Instead: r.Say, HeldIn: []string{"the rest of the project"}, Locale: r.Locale})
		}
	}
	s.Keep = mergeKeep(append(s.Keep, here.elsewhere...))
	switch {
	case here.voice != "" && here.voice != base.voice:
		s.Voice = here.voice
		s.VoiceReplaces = base.voice != ""
	case here.voice == "" && base.voice != "":
		s.NoVoice = true
	}
	return s
}

// mergeKeep states each wording to keep once: two rules held at different
// places that would rename the same wording to the same thing are one line,
// naming every place.
func mergeKeep(in []ContextElsewhere) []ContextElsewhere {
	var out []ContextElsewhere
	index := map[string]int{}
	for _, e := range in {
		key := strings.ToLower(strings.Join(e.Keep, "\x00")) + "|" + fold(e.Instead) + "|" + e.Locale
		if i, ok := index[key]; ok {
			out[i].HeldIn = dedupeStrings(append(out[i].HeldIn, e.HeldIn...))
			continue
		}
		index[key] = len(out)
		e.HeldIn = slices.Clone(e.HeldIn)
		out = append(out, e)
	}
	return out
}

// ruleKey identifies a rule by its wordings.
func ruleKey(r ContextRule) string {
	not := make([]string, len(r.Not))
	for i, n := range r.Not {
		not[i] = fold(n)
	}
	sort.Strings(not)
	return fold(r.Say) + "|" + strings.Join(not, ",") + "|" + r.Locale
}

// sameSection reports whether two sections state the same rules, so a folder
// whose places all agree states them once.
func sameSection(a, b rulesSection) bool {
	a.Pattern, b.Pattern = "", ""
	return renderSectionBody(a, "") == renderSectionBody(b, "")
}

// renderRootSection renders the root file's section.
func renderRootSection(plan rulesPlan) string {
	var b strings.Builder
	b.WriteString(agentrules.StartLine + "\n")
	b.WriteString("## Writing rules\n\n")
	b.WriteString("These rules hold for every file in this project. Follow them as you write; you do not need to ask " +
		"kapi first. kapi writes this section from the project's context and replaces it when the context changes, " +
		"so edit outside it.\n")
	body := renderSectionBody(plan.root, "<path>")
	if body == "" {
		b.WriteString("\nNo writing rules are recorded for this project yet. Write as the surrounding files do.\n")
	} else {
		b.WriteString(body)
	}
	if plan.comments {
		b.WriteString("\nCode comments follow a voice of their own: run `kapi context <path> --comments` before you write one.\n")
	}
	// The root's own folder can hold files at points of their own, which no
	// folder of their own states.
	for _, f := range plan.folders {
		if f.Dir != "" {
			continue
		}
		for _, s := range f.Sections {
			fmt.Fprintf(&b, "\n### Files matching `%s`\n", s.Pattern)
			b.WriteString(renderSectionBody(s, "<path>"))
		}
	}
	var scoped []rulesFolder
	for _, f := range plan.folders {
		if f.Dir != "" {
			scoped = append(scoped, f)
		}
	}
	if len(scoped) > 0 {
		b.WriteString("\nSome folders have rules of their own, in the AGENTS.md and CLAUDE.md there. " +
			"Read that file before you edit a file in one of them:\n")
		for i, f := range scoped {
			if i == rulesFileFolders {
				fmt.Fprintf(&b, "- %d more folders have rules of their own.\n", len(scoped)-i)
				break
			}
			fmt.Fprintf(&b, "- %s/: %s\n", f.Dir, folderSummary(f))
		}
	}
	b.WriteString("\n" + checkLine("<path>"))
	b.WriteString("When the files keep to a name or a word this section does not list, record it with " +
		"`kapi context note --term <used> --instead-of <avoided> --seen-in <file>`. A person decides what becomes a rule.\n")
	b.WriteString(agentrules.End + "\n")
	return b.String()
}

// renderFolderSection renders one folder's section.
func renderFolderSection(f rulesFolder, rootFile string) string {
	var b strings.Builder
	b.WriteString(agentrules.StartLine + "\n")
	fmt.Fprintf(&b, "## Writing rules for %s/\n\n", f.Dir)
	if rootFile != "" {
		fmt.Fprintf(&b, "These rules hold for the files in %s/, beside the project's rules in the root %s. "+
			"Where the two differ, follow this file. kapi writes this section from the project's context, so edit outside it.\n",
			f.Dir, rootFile)
	} else {
		fmt.Fprintf(&b, "These rules hold for the files in %s/. kapi writes this section from the project's context, so edit outside it.\n", f.Dir)
	}
	sample := f.Dir + "/<file>"
	for _, s := range f.Sections {
		if s.Pattern != "" {
			fmt.Fprintf(&b, "\n### Files matching `%s`\n", s.Pattern)
		}
		b.WriteString(renderSectionBody(s, sample))
	}
	b.WriteString("\n" + checkLine(sample))
	b.WriteString(agentrules.End + "\n")
	return b.String()
}

// renderSectionBody renders a section's voice, rules and wordings to keep,
// each list capped, with the command that answers in full for what the cap
// left out.
func renderSectionBody(s rulesSection, sample string) string {
	var b strings.Builder
	switch {
	case s.Voice != "":
		b.WriteString("\n" + capLines(s.Voice, rulesFileVoiceLines, sample) + "\n")
		if s.VoiceReplaces {
			b.WriteString("This voice takes the place of the project's voice for these files.\n")
		}
	case s.NoVoice:
		b.WriteString("\nNo voice is bound here: the project's voice does not apply to these files.\n")
	}
	if len(s.Rules) > 0 {
		b.WriteString("\nSay this, not that:\n")
		// A list that mixes languages names each rule's own, so a rule for
		// a translation is not read as one for the source.
		showLocale := slices.ContainsFunc(s.Rules, func(r ContextRule) bool { return r.Locale != s.Rules[0].Locale })
		for i, r := range s.Rules {
			if i == rulesFileRules {
				fmt.Fprintf(&b, "- %d more rules hold here: `kapi context %s` lists them all.\n", len(s.Rules)-i, sample)
				break
			}
			b.WriteString(ruleLine(r, showLocale) + "\n")
		}
	}
	if len(s.Keep) > 0 {
		b.WriteString("\nKeep as it is:\n")
		for i, e := range s.Keep {
			if i == rulesFileKeep {
				fmt.Fprintf(&b, "- %d more wordings stay as they are here: `kapi context %s` lists them all.\n", len(s.Keep)-i, sample)
				break
			}
			b.WriteString(elsewhereLine(e) + "\n")
		}
	}
	return b.String()
}

// capLines keeps the first n lines of a text and says where the rest is.
func capLines(text string, n int, sample string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:n], "\n") + fmt.Sprintf("\n(The voice continues: `kapi context %s` gives it in full.)", sample)
}

// checkLine names the gate, and the full answer for the case the section
// does not cover. An agent reading the section has the rules already, so the
// line asks for `kapi context` only when it needs more than they state.
func checkLine(sample string) string {
	return "Before you finish, run `kapi check <file>` on each file you changed and fix what it reports. " +
		"`kapi context " + sample + "` gives the full answer for one file when you need more than this section.\n"
}

// folderSummary renders what a folder's rules change, for the root's list.
func folderSummary(f rulesFolder) string {
	var parts []string
	for _, s := range f.Sections {
		if s.Voice != "" {
			parts = append(parts, "a voice of its own")
		}
		if s.NoVoice {
			parts = append(parts, "no voice")
		}
		for _, r := range s.Rules {
			if r.Say != "" && len(r.Not) > 0 {
				parts = append(parts, fmt.Sprintf("write %q, not %s", r.Say, quotedAlternatives(r.Not)))
			}
		}
		for _, e := range s.Keep {
			pronoun := "it"
			if len(e.Keep) > 1 {
				pronoun = "them"
			}
			parts = append(parts, fmt.Sprintf("keep %s (do not rename %s to %q)", quotedAll(e.Keep), pronoun, e.Instead))
		}
	}
	parts = dedupeStrings(parts)
	switch {
	case len(parts) == 0:
		return "rules of its own."
	case len(parts) > 3:
		return strings.Join(parts[:3], "; ") + "; and more."
	}
	return strings.Join(parts, "; ") + "."
}

func dedupeStrings(in []string) []string {
	var out []string
	for _, s := range in {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// writeRulesPlan writes every folder's files and takes kapi's section out of
// the files in folders that no longer need one.
func writeRulesPlan(root string, plan rulesPlan) (*RulesFilesResult, error) {
	res := &RulesFilesResult{}
	want := map[string]string{"": capLineLength(renderRootSection(plan))}
	for _, f := range plan.folders {
		if f.Dir == "" {
			continue
		}
		want[f.Dir] = capLineLength(renderFolderSection(f, "AGENTS.md"))
	}

	dirs := make([]string, 0, len(want))
	for dir := range want {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		files, err := writeFolderRules(root, dir, want[dir])
		if err != nil {
			return res, err
		}
		res.Files = append(res.Files, files...)
	}

	stale, err := staleRulesFiles(root, want)
	if err != nil {
		return res, err
	}
	for _, rel := range stale {
		action, err := removeRulesSection(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return res, err
		}
		if action != "" {
			res.Files = append(res.Files, RulesFile{Path: rel, Action: action})
		}
	}
	sort.SliceStable(res.Files, func(i, j int) bool { return res.Files[i].Path < res.Files[j].Path })
	return res, nil
}

// writeFolderRules writes a folder's section into its AGENTS.md and CLAUDE.md.
// A CLAUDE.md that imports AGENTS.md, or is the same file, reaches the section
// through it and is given none of its own.
func writeFolderRules(root, dir, section string) ([]RulesFile, error) {
	var out []RulesFile
	agents := filepath.Join(root, filepath.FromSlash(dir), "AGENTS.md")
	claude := filepath.Join(root, filepath.FromSlash(dir), "CLAUDE.md")
	for _, path := range []string{agents, claude} {
		rel := relSlash(root, path)
		if path == claude {
			if sameInode(agents, claude) {
				continue
			}
			if doc, err := os.ReadFile(claude); err == nil && agentrules.ImportsAgents(doc) {
				action, rerr := removeRulesSection(claude)
				if rerr != nil {
					return out, rerr
				}
				if action != "" {
					out = append(out, RulesFile{Path: rel, Action: action})
				}
				continue
			}
		}
		action, err := upsertRulesSection(path, section)
		if err != nil {
			return out, fmt.Errorf("%s: %w", rel, err)
		}
		out = append(out, RulesFile{Path: rel, Action: action})
	}
	return out, nil
}

// upsertRulesSection writes section into the file at path, creating it when
// it does not exist.
func upsertRulesSection(path, section string) (RulesFileAction, error) {
	doc, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if werr := os.WriteFile(path, []byte(section), 0o644); werr != nil {
			return "", werr
		}
		return RulesFileCreated, nil
	}
	if err != nil {
		return "", err
	}
	out, err := agentrules.Upsert(doc, section)
	if err != nil {
		return "", err
	}
	if bytes.Equal(out, doc) {
		return RulesFileUnchanged, nil
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return "", err
	}
	return RulesFileUpdated, nil
}

// removeRulesSection takes kapi's section out of the file at path, and deletes
// the file when nothing else is in it. It reports "" when the file holds no
// section.
func removeRulesSection(path string) (RulesFileAction, error) {
	doc, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	out, removed, err := agentrules.Remove(doc)
	if err != nil || !removed {
		return "", err
	}
	if agentrules.Empty(out) {
		if err := os.Remove(path); err != nil {
			return "", err
		}
		return RulesFileDeleted, nil
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return "", err
	}
	return RulesFileRemoved, nil
}

// staleRulesFiles finds the rules files under root that hold kapi's section in
// a folder the plan writes nothing to. It skips hidden folders, dependency
// trees, the folders the project ignores and nested projects.
func staleRulesFiles(root string, want map[string]string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			return nil
		}
		rel := relSlash(root, path)
		if d.IsDir() {
			if path == root {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "testdata" || ProjectIgnores(root, rel) {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, project.RecipeFileName)); err == nil {
				return filepath.SkipDir
			}
			return nil
		}
		if !slices.Contains(agentrules.Names, d.Name()) {
			return nil
		}
		dir := relSlash(root, filepath.Dir(path))
		if dir == "." {
			dir = ""
		}
		if _, wanted := want[dir]; wanted {
			return nil
		}
		if doc, rerr := os.ReadFile(path); rerr == nil && agentrules.Holds(doc) {
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// sameInode reports whether two paths name one file, as a CLAUDE.md linked to
// AGENTS.md does.
func sameInode(a, b string) bool {
	ia, err := os.Stat(a)
	if err != nil {
		return false
	}
	ib, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ia, ib)
}

// refreshRulesFilesAfterUp refreshes a project's rules files at the end of
// `kapi up`, and names the files that changed on stderr, where a `--json`
// stream is not disturbed. A failure is reported and fails nothing.
func (a *App) refreshRulesFilesAfterUp(cmd Command, projectPath string) {
	res, err := a.RefreshRulesFiles(CmdContext(cmd), projectPath)
	w := cmd.ErrOrStderr()
	if err != nil {
		fmt.Fprintf(w, "warning: rules files: %v\n", err)
		return
	}
	for _, f := range res.Changed() {
		fmt.Fprintf(w, "rules: %s (%s)\n", f.Path, f.Action)
	}
}
