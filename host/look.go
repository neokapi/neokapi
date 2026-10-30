package host

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
)

// `kapi look`: the project, and one file, for a person.
//
// The Map (LookProject) is the project at a glance: the files kapi reads, the
// blocks in them, the rules in force, the findings a check reports and the
// suggestions waiting for a decision. The Reader (LookFile) is one file's
// blocks in order with each finding drawn under the words it is about, and the
// rules that hold there.
//
// Neither computes anything of its own. The findings are the report the check
// command produces, the rules are the by-location context answer, and the
// suggestions are the context digest review reads, so every number here is a
// number another command prints. The assembly lives here, cobra-free, so the
// desktop can draw the same Map.

// LookProject is the Map: what a project holds and where it stands.
type LookProject struct {
	// Project is the project's name.
	Project string `json:"project"`
	// Files are the content files the recipe declares, project-relative and
	// slash-separated, with what each holds.
	Files []LookFileSummary `json:"files"`
	// Blocks counts the blocks read from Files.
	Blocks int `json:"blocks"`
	// Rules counts the rules in force across the project's places: the terms,
	// the voice's pattern rules, and the rules a person kept.
	Rules int `json:"rules"`
	// Findings is the number of findings `kapi check` reports over the
	// project's content.
	Findings int `json:"findings"`
	// Failing is how many of Findings fail the check.
	Failing int `json:"failing"`
	// Suggests counts what waits for a person in `kapi context review`:
	// suggestions, and disagreements between them.
	Suggests int `json:"suggests"`
	// Suggestions are those, one line each.
	Suggestions []LookSuggestion `json:"suggestions,omitempty"`
	// Languages is the multilingual standing, present only when the recipe
	// declares a target language.
	Languages *LookLanguages `json:"languages,omitempty"`
	// Source is set for a project with no content read: the source line
	// `kapi status` prints for it.
	Source *SourceCoverage `json:"source,omitempty"`
	// Next is the command to run next.
	Next string `json:"next"`
}

// LookFileSummary is one file on the Map.
type LookFileSummary struct {
	Path     string `json:"path"`
	Blocks   int    `json:"blocks"`
	Findings int    `json:"findings"`
	// Unread names why the file could not be read, empty when it was.
	Unread string `json:"unread,omitempty"`
}

// LookSuggestion is one suggestion waiting for a person.
type LookSuggestion struct {
	// ID is what `kapi context review --keep` takes.
	ID string `json:"id"`
	// Kind is the mark the line carries: Name, Word, Note, Memory, or
	// Disagreement.
	Kind string `json:"kind"`
	// Text is the rule, the note or the disagreement.
	Text string `json:"text"`
	// Evidence is what the suggestion stands on, as review counts it.
	Evidence string `json:"evidence,omitempty"`
}

// LookLanguages is the one line of language standing the Map carries.
type LookLanguages struct {
	// Languages counts the source language and the target languages.
	Languages int `json:"languages"`
	// AwaitingReview counts the blocks `kapi status --review` lists.
	AwaitingReview int `json:"awaiting_review"`
}

// lookMapFiles caps the files the Map lists, and lookMapSuggestions the
// suggestions. The counts above them are always whole.
const (
	lookMapFiles       = 20
	lookMapSuggestions = 10
)

// LookProject assembles the Map for the project cmd resolves.
func (a *App) LookProject(cmd Command) (*LookProject, error) {
	a.InitRegistries()
	ctx := CmdContext(cmd)
	recipe, err := RequireProjectPath(cmd)
	if err != nil {
		return nil, err
	}
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}
	root := filepath.Dir(recipe)
	pcmd := lookCommand(ctx, recipe)
	a.applyProjectSourceLang(pcmd)

	out := &LookProject{Project: proj.Name, Files: []LookFileSummary{}}
	files, err := a.projectSourceFiles(proj, root)
	if err != nil {
		return nil, err
	}
	err = a.withParseCache(root, func() error {
		formats, ferr := a.newCheckFormats(pcmd)
		if ferr != nil {
			return ferr
		}
		var unreadable []string
		for _, file := range files {
			fs := LookFileSummary{Path: projectRel(root, file)}
			name, cfg := formats.forFile(a, file)
			blocks, rerr := a.readBlocksAs(ctx, file, name, cfg, a.SourceLang)
			if rerr != nil {
				fs.Unread = lookUnreadReason(a, file, name, rerr)
				if a.NoReaderFor(file) && name == "" {
					unreadable = appendUnique(unreadable, filepath.Ext(file))
				}
			}
			fs.Blocks = len(lookBlocks(blocks))
			out.Blocks += fs.Blocks
			out.Files = append(out.Files, fs)
		}
		if out.Blocks == 0 {
			out.Source = &SourceCoverage{Unreadable: unreadable}
			return nil
		}

		report, cerr := a.ComputeCheck(pcmd, nil)
		if cerr != nil {
			return fmt.Errorf("check the project's content: %w", cerr)
		}
		out.Findings, out.Failing = report.Summary.Findings, report.Summary.Failing
		byFile := map[string]int{}
		for _, d := range report.Findings {
			if d.Location.File != "" {
				byFile[projectRel(root, cwdAbs(d.Location.File))]++
			}
		}
		for i := range out.Files {
			out.Files[i].Findings = byFile[out.Files[i].Path]
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if out.Source == nil {
		out.Rules = a.lookRuleCount(pcmd, proj, root, files)
		if digest, derr := a.ContextDigest(ctx, ContextDigestRequest{Project: recipe}); derr == nil {
			out.Suggestions = lookSuggestions(digest)
			out.Suggests = len(out.Suggestions)
		}
		if proj.DeclaresTargetLanguages() {
			out.Languages = a.lookLanguages(ctx, proj, root, recipe)
		}
	}

	slices.SortStableFunc(out.Files, func(x, y LookFileSummary) int {
		if c := cmp.Compare(y.Findings, x.Findings); c != 0 {
			return c
		}
		return cmp.Compare(strings.ToLower(x.Path), strings.ToLower(y.Path))
	})
	switch {
	case out.Source != nil:
		out.Next = "kapi add <pattern>"
	case out.Suggests > 0:
		out.Next = "kapi context review"
	default:
		out.Next = "kapi check"
	}
	return out, nil
}

// lookCommand is the command the Map and the Reader run the check and the
// context answer through: the project at recipe, its output discarded.
func lookCommand(ctx context.Context, recipe string) *EnvCommand {
	cmd := NewEnvCommand(ctx, "look")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.Flags().String(projectFlagName, recipe, "")
	return cmd
}

// lookRuleCount counts the rules in force across the places the project's
// files sit at: one answer per point, as the rules files read them, with each
// rule counted once however many points hold it.
func (a *App) lookRuleCount(cmd Command, proj *project.KapiProject, root string, files []string) int {
	at := a.GovernanceInstant()
	points := map[string]bool{}
	rules := map[string]bool{}
	samples := append([]string{filepath.Join(root, rulesRootSample)}, files...)
	for _, file := range samples {
		key := rulesPointKey(proj, projectRel(root, file), at)
		if points[key] {
			continue
		}
		points[key] = true
		req := ContextPointRequest{Path: file, Limit: 1 << 16, rulesOnly: true}
		src, release := a.ContextSourcesAt(cmd, req)
		res, err := ResolveContextAt(CmdContext(cmd), src, req)
		release()
		if err != nil {
			continue
		}
		for _, r := range res.Rules {
			rules["rule|"+ruleKey(r)] = true
		}
		for _, n := range res.EstablishedNotes {
			rules["note|"+n.Operation] = true
		}
		if v := src.Voice; v != nil {
			for _, p := range slices.Concat(v.Style.ProhibitedPatterns, v.Style.RequiredPatterns) {
				if strings.TrimSpace(p.Regex) != "" {
					rules["voice|"+v.Name+"|"+p.Regex] = true
				}
			}
		}
	}
	return len(rules)
}

// lookSuggestions lists what review asks a person about: each disagreement,
// then each suggestion a person can keep or drop.
func lookSuggestions(d ContextDigest) []LookSuggestion {
	var out []LookSuggestion
	seen := map[string]bool{}
	for _, c := range d.Conflicts {
		id := ""
		if len(c.Sides) > 0 {
			id = c.Sides[0].Short
		}
		for _, s := range c.Sides {
			seen[s.ID] = true
		}
		out = append(out, LookSuggestion{ID: id, Kind: "Disagreement", Text: strings.TrimSuffix(c.Reason, ".")})
	}
	for _, t := range d.Suggested {
		for _, g := range t.Groups {
			for _, it := range g.Items {
				if seen[it.ID] || !(it.Keepable || it.Droppable) {
					continue
				}
				seen[it.ID] = true
				out = append(out, LookSuggestion{
					ID:       it.Short,
					Kind:     lookThemeKind(it.Theme),
					Text:     lookSubjectText(it.Subject, it.Sentence),
					Evidence: it.Standing,
				})
			}
		}
	}
	return out
}

// lookThemeKind is the mark a digest theme carries on the Map and in the
// Reader, the kinds the desktop's KindIcon draws.
func lookThemeKind(theme string) string {
	switch theme {
	case DigestThemeNames:
		return "Name"
	case DigestThemeWords:
		return "Word"
	case DigestThemeWriting:
		return "Note"
	case DigestThemeMemory:
		return "Memory"
	}
	return capitalize(theme)
}

// capitalize upper-cases the first letter of s.
func capitalize(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return s
	}
	return strings.ToUpper(string(r)) + s[size:]
}

// lookSubjectText states a suggestion's subject as the Map's one line: the
// form to write and the forms to avoid, or the note.
func lookSubjectText(s contextop.Subject, sentence string) string {
	if rule, ok := s.Rule(); ok {
		avoid := append([]string{rule.Term}, rule.Forms...)
		if rule.Replacement == "" {
			return "avoid " + strings.Join(avoid, ", ")
		}
		return rule.Replacement + ", not " + strings.Join(avoid, ", ")
	}
	return strings.TrimSuffix(sentence, ".")
}

// lookLanguages counts the languages a multilingual project holds and the
// blocks waiting for review in them. A queue that cannot be read leaves the
// line out.
func (a *App) lookLanguages(ctx context.Context, proj *project.KapiProject, root, recipe string) *LookLanguages {
	units, err := a.UnitsFromProject(proj, root, "")
	if err != nil {
		return nil
	}
	// The languages the recipe declares, and any a unit resolves beyond them.
	locales := map[string]bool{}
	for _, l := range proj.Defaults.TargetLanguages {
		locales[string(l)] = true
	}
	for i := range proj.Collections {
		for _, item := range proj.Collections[i].EffectiveItems() {
			if item.Target != "" {
				for _, l := range item.TargetLanguages {
					locales[string(l)] = true
				}
			}
		}
	}
	for _, u := range units {
		if u.Locale != "" {
			locales[u.Locale] = true
		}
	}
	queue, err := a.ReviewQueue(ctx, recipe, "", ReviewQueueOptions{})
	if err != nil {
		return nil
	}
	return &LookLanguages{Languages: 1 + len(locales), AwaitingReview: len(queue.Pending)}
}

// FormatText renders the Map.
func (m *LookProject) FormatText(w io.Writer) error {
	s := output.Theme(w)
	if m.Source != nil {
		writeNoSourceLine(w, m.Source)
		return nil
	}
	read := 0
	for _, f := range m.Files {
		if f.Unread == "" {
			read++
		}
	}
	fmt.Fprintf(w, "%s %s\n\n", s.Title.Render(m.Project), s.Muted.Render("· read "+pluralUnit(read, "file", "files")+" just now"))

	counts := []string{
		pluralUnit(len(m.Files), "file", "files"),
		pluralUnit(m.Blocks, "block", "blocks"),
		pluralUnit(m.Rules, "rule", "rules"),
		pluralUnit(m.Findings, "finding", "findings"),
	}
	if m.Suggests > 0 {
		counts = append(counts, fmt.Sprintf("kapi suggests %d", m.Suggests))
	}
	fmt.Fprintf(w, "  %s\n", strings.Join(counts, s.Muted.Render(" · ")))
	if l := m.Languages; l != nil {
		fmt.Fprintf(w, "  %s · %s waiting for review · %s\n",
			pluralUnit(l.Languages, "language", "languages"),
			pluralUnit(l.AwaitingReview, "block", "blocks"),
			s.Muted.Render("kapi status for coverage"))
	}

	if len(m.Suggestions) > 0 {
		fmt.Fprintf(w, "\n%s\n", s.Title.Render("What your files do"))
		shown := m.Suggestions[:min(len(m.Suggestions), lookMapSuggestions)]
		kindW, textW := 0, 0
		for _, sg := range shown {
			kindW = max(kindW, len(sg.Kind))
			textW = max(textW, utf8.RuneCountInString(sg.Text))
		}
		for _, sg := range shown {
			line := "  " + s.Muted.Render(padRight(sg.Kind, kindW)) + "  " + padRight(sg.Text, textW)
			if sg.Evidence != "" {
				line += "  " + s.Muted.Render(sg.Evidence)
			}
			fmt.Fprintln(w, strings.TrimRight(line, " "))
		}
		if rest := len(m.Suggestions) - len(shown); rest > 0 {
			fmt.Fprintf(w, "  %s\n", s.Muted.Render(fmt.Sprintf("%d more in kapi context review", rest)))
		}
	}

	if len(m.Files) > 0 {
		fmt.Fprintf(w, "\n%s\n", s.Title.Render("Files"))
		shown := m.Files[:min(len(m.Files), lookMapFiles)]
		pathW := 0
		for _, f := range shown {
			pathW = max(pathW, utf8.RuneCountInString(f.Path))
		}
		for _, f := range shown {
			var cells []string
			switch {
			case f.Unread != "":
				cells = append(cells, "not read: "+f.Unread)
			default:
				cells = append(cells, pluralUnit(f.Blocks, "block", "blocks"))
				if f.Findings > 0 {
					cells = append(cells, pluralUnit(f.Findings, "finding", "findings"))
				}
			}
			fmt.Fprintf(w, "  %s  %s\n", padRight(f.Path, pathW), s.Muted.Render(strings.Join(cells, " · ")))
		}
		if rest := len(m.Files) - len(shown); rest > 0 {
			fmt.Fprintf(w, "  %s\n", s.Muted.Render(fmt.Sprintf("%d more files in kapi ls", rest)))
		}
	}

	fmt.Fprintf(w, "\n%s\n", m.NextLine())
	return nil
}

// NextLine is the Map's last line: the next step and how to see one file. A
// project with no content prints its source line in place of it, because that
// line names the step.
func (m *LookProject) NextLine() string {
	if m.Next == "kapi context review" {
		return "Decide with `kapi context review`. See a file with `kapi look <path>`."
	}
	return "Check with `kapi check`. See a file with `kapi look <path>`."
}

// LookReader is one file as the Reader draws it.
type LookReader struct {
	// Path is the file, project-relative when it sits in the project.
	Path string `json:"path"`
	// Declared reports a file one of the project's collections reads.
	Declared bool `json:"declared"`
	// Note is the line the Reader opens with when the file is not one the
	// project declares.
	Note string `json:"note,omitempty"`
	// Blocks are the file's blocks in file order.
	Blocks []LookBlock `json:"blocks"`
	// Findings counts the findings the check reports for the file, and
	// Document holds the ones that sit on no block of it.
	Findings int           `json:"findings"`
	Document []LookFinding `json:"document,omitempty"`
	// Rules are what the context answer for the file holds, in its order.
	Rules []LookRule `json:"rules,omitempty"`

	width int
}

// LookBlock is one block with the findings on it.
type LookBlock struct {
	// ID is the block's reference as kapi inspect and kapi check name it.
	ID string `json:"id"`
	// Text is the block's text as the Reader draws it: inline codes as their
	// display form, line breaks as spaces.
	Text     string        `json:"text"`
	Findings []LookFinding `json:"findings,omitempty"`
}

// LookFinding is one finding, placed in its block's text.
type LookFinding struct {
	check.Diagnostic
	// Span is the rune range of Text the finding is about. Absent when the
	// check located the finding at the block alone.
	Span *LookSpan `json:"span,omitempty"`
	// Label is the line drawn under it.
	Label string `json:"label"`
}

// LookSpan is a half-open rune range [Start, End) of a block's Text.
type LookSpan struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// LookRule is one rule that holds at the file.
type LookRule struct {
	// Kind is the mark the line carries: Name, Word, Tone, Note, or a
	// suggestion's kind.
	Kind string `json:"kind"`
	// Rule is the rule as a line.
	Rule string `json:"rule"`
	// Status is established or suggested.
	Status string `json:"status"`
	// From is where the rule is held, or who recorded it.
	From string `json:"from,omitempty"`
	// At is when it was recorded, when the answer carries it.
	At string `json:"at,omitempty"`

	// avoid are the forms a word rule avoids, which mark a finding on one of
	// them with the rule's kind.
	avoid []string
}

// LookFile assembles the Reader for the file at path, relative to the working
// directory. A file the project's collections do not read is read anyway, by
// its extension, and drawn without rules.
func (a *App) LookFile(cmd Command, path string) (*LookReader, error) {
	a.InitRegistries()
	ctx := CmdContext(cmd)
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if info, serr := os.Stat(abs); errors.Is(serr, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s: no such file", path)
	} else if serr != nil {
		return nil, serr
	} else if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory: kapi look <path> draws one file, and kapi look the whole project", path)
	}
	recipe, err := ResolveProjectPath(cmd)
	if err != nil {
		return nil, err
	}
	pcmd := lookCommand(ctx, recipe)
	if recipe == "" {
		pcmd = NewEnvCommand(ctx, "look")
		pcmd.SetOut(io.Discard)
		pcmd.SetErr(io.Discard)
		pcmd.Flags().Bool(detachedFlagName, true, "")
	}
	a.applyProjectSourceLang(pcmd)

	out := &LookReader{Path: path, Blocks: []LookBlock{}, width: output.TerminalWidth(cmd.OutOrStdout())}
	root := ""
	if recipe != "" {
		root = filepath.Dir(recipe)
		if spelled, inside := rootSpelled(root, abs); inside {
			abs = spelled
			out.Path, _ = underRoot(root, abs)
		}
	}

	err = a.withParseCache(cmp.Or(root, filepath.Dir(abs)), func() error {
		formats, ferr := a.newCheckFormats(pcmd)
		if ferr != nil {
			return ferr
		}
		_, out.Declared = formats.lookup(abs)
		name, cfg := formats.forFile(a, abs)
		blocks, rerr := a.readBlocksAs(ctx, abs, name, cfg, a.SourceLang)
		if rerr != nil {
			return NoReaderError(rerr, DisplayName(path), name, a.discoveredPlugins()...)
		}
		var report check.Report
		var cerr error
		if out.Declared {
			report, cerr = a.ComputeDeclaredCheck(pcmd, []string{abs})
		} else {
			report, cerr = a.ComputeCheck(pcmd, []string{abs})
		}
		if cerr != nil {
			return fmt.Errorf("check %s: %w", path, cerr)
		}
		out.Findings = report.Summary.Findings
		out.place(lookBlocks(blocks), report.Findings)
		return nil
	})
	if err != nil {
		return nil, err
	}
	switch {
	case recipe == "":
		out.Note = "not in a kapi project"
	case !out.Declared:
		out.Note = "not in the project's collections"
	default:
		out.Rules = a.lookRulesAt(pcmd, abs)
		out.markTermFindings()
	}
	return out, nil
}

// markTermFindings gives a finding on a word the kind of the rule that avoids
// it, so a finding and the rule it comes from carry one mark.
func (r *LookReader) markTermFindings() {
	kinds := map[string]string{}
	for _, rule := range r.Rules {
		for _, form := range rule.avoid {
			if _, held := kinds[fold(form)]; !held {
				kinds[fold(form)] = rule.Kind
			}
		}
	}
	for i := range r.Blocks {
		for j := range r.Blocks[i].Findings {
			f := &r.Blocks[i].Findings[j]
			kind, ok := kinds[fold(f.Metadata["term"])]
			if f.Check != "terms" || !ok {
				continue
			}
			if old := lookCheckKind(f.Diagnostic); strings.HasPrefix(f.Label, old+": ") {
				f.Label = kind + ": " + strings.TrimPrefix(f.Label, old+": ")
			}
		}
	}
}

// place sets each finding on the block it names, at the range it names.
func (r *LookReader) place(blocks []*model.Block, findings []check.Diagnostic) {
	index := map[string]int{}
	runs := make([][]model.Run, len(blocks))
	for i, b := range blocks {
		runs[i] = b.SourceRuns()
		index[blockKey(b)] = i
		r.Blocks = append(r.Blocks, LookBlock{ID: blockKey(b), Text: lookDisplayText(runs[i])})
	}
	for _, d := range findings {
		f := LookFinding{Diagnostic: d, Label: lookFindingLabel(d)}
		i, ok := index[d.Location.Block]
		if !ok {
			if d.Location.Block != "" {
				f.Label = d.Location.Block + ": " + f.Label
			}
			r.Document = append(r.Document, f)
			continue
		}
		f.Span = lookSpan(runs[i], d.Location.Anchor)
		r.Blocks[i].Findings = append(r.Blocks[i].Findings, f)
	}
	for i := range r.Blocks {
		slices.SortStableFunc(r.Blocks[i].Findings, func(x, y LookFinding) int {
			return cmp.Compare(spanStart(x.Span), spanStart(y.Span))
		})
	}
}

// spanStart orders findings with a range by where it starts, and the ones
// without after them.
func spanStart(s *LookSpan) int {
	if s == nil {
		return 1 << 30
	}
	return s.Start
}

// lookBlocks keeps the blocks that hold content, as kapi inspect lists them.
func lookBlocks(blocks []*model.Block) []*model.Block {
	out := make([]*model.Block, 0, len(blocks))
	for _, b := range blocks {
		if model.RunsHygieneText(b.SourceRuns()) != "" {
			out = append(out, b)
		}
	}
	return out
}

// lookDisplayText is a block's text with each inline code in its display
// form and every line break a space, so one rune is one column.
func lookDisplayText(runs []model.Run) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, model.FlattenRuns(runs))
}

// lookSpan projects a finding's anchor onto the display text: the columns of
// the runs before it, plus its offset into its own run. An anchor into a
// plural or a select, or one the runs no longer hold, has no span.
func lookSpan(runs []model.Run, a *model.Anchor) *LookSpan {
	if a == nil || a.Kind != model.AnchorRange || len(a.Path) > 0 || !a.InBounds(runs) {
		return nil
	}
	col := func(p model.RunPos) int {
		return utf8.RuneCountInString(model.FlattenRuns(runs[:p.Run])) + p.Offset
	}
	start, end := col(a.Start), col(a.End)
	if end <= start {
		return nil
	}
	return &LookSpan{Start: start, End: end}
}

// lookFindingLabel is the line under a finding: its kind, then what to write
// instead or what the check says.
func lookFindingLabel(d check.Diagnostic) string {
	kind := lookCheckKind(d)
	text := d.Message
	if replacement, term := d.Metadata["replacement"], d.Metadata["term"]; d.Check == "terms" && replacement != "" && term != "" {
		text = replacement + ", not " + term
	}
	if d.Suggested {
		text += " (suggested)"
	}
	return kind + ": " + text
}

// lookCheckKind marks a finding by the check that raised it: a term about a
// spelling is a Name, any other term a Word, a voice rule Tone, and every
// other check its family.
func lookCheckKind(d check.Diagnostic) string {
	switch d.Check {
	case "terms":
		if r, t := d.Metadata["replacement"], d.Metadata["term"]; r != "" && spellingKey(r) == spellingKey(t) {
			return "Name"
		}
		return "Word"
	case "voice":
		return "Tone"
	}
	return capitalize(d.Check)
}

// lookRulesAt is the context answer for the file at abs as the Reader's rule
// lines, in the order `kapi context <path>` prints them: the voice, the rules
// to say and avoid, the notes a person established, and the suggestions not
// already stated as a rule.
func (a *App) lookRulesAt(cmd Command, abs string) []LookRule {
	req := ContextPointRequest{Path: abs, Limit: DefaultContextTermsLimit}
	src, release := a.ContextSourcesAt(cmd, req)
	defer release()
	res, err := ResolveContextAt(CmdContext(cmd), src, req)
	if err != nil {
		return nil
	}
	var out []LookRule
	if v := src.Voice; v != nil {
		from := "voice " + v.Name
		for _, p := range slices.Concat(v.Style.ProhibitedPatterns, v.Style.RequiredPatterns) {
			if strings.TrimSpace(p.Regex) == "" {
				continue
			}
			out = append(out, LookRule{Kind: "Tone", Rule: cmp.Or(p.Description, p.Regex), Status: string(contextop.StatusEstablished), From: from})
		}
	}
	for _, r := range res.Rules {
		kind := "Word"
		for _, n := range r.Not {
			if r.Say != "" && spellingKey(n) == spellingKey(r.Say) {
				kind = "Name"
			}
		}
		text := r.Say
		switch {
		case r.Say != "" && len(r.Not) > 0:
			text += ", not " + strings.Join(r.Not, ", ")
		case r.Say == "":
			text = "avoid " + strings.Join(r.Not, ", ")
		}
		out = append(out, LookRule{Kind: kind, Rule: text, Status: string(contextop.StatusEstablished), From: strings.Join(r.From, ", "), avoid: r.Not})
	}
	for _, n := range res.EstablishedNotes {
		out = append(out, LookRule{Kind: "Note", Rule: n.Text, Status: n.Status, From: n.SuggestedBy, At: n.At})
	}
	stated := map[string]bool{}
	for _, r := range res.Rules {
		for _, n := range r.Not {
			stated[fold(n)] = true
		}
	}
	for _, c := range res.Suggestions {
		if c.Kind != string(contextop.SubjectNote) && stated[fold(c.Term)] {
			continue
		}
		rule := LookRule{Status: c.Status, From: c.SuggestedBy, At: c.At, avoid: append([]string{c.Term}, c.Forms...)}
		subject := contextop.Subject{Kind: contextop.SubjectKind(c.Kind), Text: c.Text}
		if c.Kind == string(contextop.SubjectTerm) {
			subject.Term = &coreprofile.TermRule{Term: c.Term, Forms: c.Forms, Replacement: c.Replacement}
		}
		rule.Kind = lookThemeKind(digestTheme(subject))
		rule.Rule = lookSubjectText(subject, c.Text)
		out = append(out, rule)
	}
	return out
}

// FormatText renders the Reader.
func (r *LookReader) FormatText(w io.Writer) error {
	s := output.Theme(w)
	if r.Note != "" {
		fmt.Fprintln(w, s.Muted.Render(r.Note))
	}
	fmt.Fprintf(w, "%s %s\n", s.Title.Render(r.Path),
		s.Muted.Render("· "+pluralUnit(len(r.Blocks), "block", "blocks")+" · "+pluralUnit(r.Findings, "finding", "findings")))
	for _, f := range r.Document {
		fmt.Fprintf(w, "  %s\n", lookLabelStyle(s, f).Render(f.Label))
	}
	fmt.Fprintln(w)

	width := r.width
	if width <= 0 {
		width = lookDefaultWidth
	}
	gutter := 0
	for _, b := range r.Blocks {
		gutter = max(gutter, utf8.RuneCountInString(b.ID))
	}
	gutter = min(gutter, lookGutterMax) + 2
	textW := max(width-gutter, lookMinTextWidth)
	indent := strings.Repeat(" ", gutter)

	for _, b := range r.Blocks {
		lead := s.Muted.Render(padRight(b.ID, gutter))
		if utf8.RuneCountInString(b.ID) > gutter-2 {
			fmt.Fprintln(w, s.Muted.Render(b.ID))
			lead = indent
		}
		text := []rune(b.Text)
		for i, ln := range wrapRunes(text, textW) {
			if i > 0 {
				lead = indent
			}
			fmt.Fprintln(w, strings.TrimRight(lead+string(text[ln.start:ln.end]), " "))
			writeCarets(w, s, indent, ln, b.Findings, textW)
		}
		for _, f := range b.Findings {
			if f.Span == nil {
				fmt.Fprintf(w, "%s%s\n", indent, lookLabelStyle(s, f).Render(f.Label))
			}
		}
	}

	if len(r.Rules) > 0 {
		fmt.Fprintf(w, "\n%s\n", s.Title.Render("Rules here"))
		kindW, ruleW := 0, 0
		for _, rule := range r.Rules {
			kindW = max(kindW, len(rule.Kind))
			ruleW = max(ruleW, utf8.RuneCountInString(rule.Rule))
		}
		ruleW = min(ruleW, lookRuleWidth)
		for _, rule := range r.Rules {
			line := "  " + s.Muted.Render(padRight(rule.Kind, kindW)) + "  " + padRight(rule.Rule, ruleW)
			if who := lookWho(rule); who != "" {
				line += "  " + s.Muted.Render(who)
			}
			fmt.Fprintln(w, strings.TrimRight(line, " "))
		}
	}

	fmt.Fprintln(w)
	quoted := shellQuote(r.Path)
	if r.Declared {
		fmt.Fprintf(w, "`kapi check %s` for the report. `kapi context %s` for an assistant.\n", quoted, quoted)
	} else {
		fmt.Fprintf(w, "`kapi check %s` for the report.\n", quoted)
	}
	return nil
}

// Layout of the Reader.
const (
	// lookDefaultWidth is the width a Reader wraps at when its output is not a
	// terminal.
	lookDefaultWidth = 80
	// lookGutterMax caps the block id column; a longer id takes its own line.
	lookGutterMax = 24
	// lookMinTextWidth is the narrowest a wrapped text column gets.
	lookMinTextWidth = 30
	// lookRuleWidth caps the rule column of "Rules here".
	lookRuleWidth = 48
)

// lookWho says who a rule comes from and when: the store or voice that holds
// it, or the actor that recorded it and the day.
func lookWho(r LookRule) string {
	var parts []string
	if r.Status == string(contextop.StatusSuggested) || r.Status == string(contextop.StatusContested) {
		parts = append(parts, r.Status)
	}
	switch {
	case r.From != "" && r.Status == string(contextop.StatusEstablished) && r.At != "":
		parts = append(parts, "kept, recorded by "+r.From)
	case r.From != "" && r.At != "":
		parts = append(parts, "by "+r.From)
	case r.From != "":
		parts = append(parts, "from "+r.From)
	}
	if r.At != "" {
		parts = append(parts, lookDay(r.At))
	}
	return strings.Join(parts, ", ")
}

// lookDay renders an RFC 3339 instant as the day it fell on, or "today".
func lookDay(at string) string {
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return at
	}
	if t.Local().Format(time.DateOnly) == time.Now().Format(time.DateOnly) {
		return "today"
	}
	return t.Local().Format(time.DateOnly)
}

// lookLabelStyle inks a finding that fails as an error and one that reports
// as a warning.
func lookLabelStyle(s *output.Styles, f LookFinding) styleRenderer {
	if f.Fails {
		return s.Error
	}
	return s.Warn
}

// styleRenderer is the part of a lipgloss style the Reader draws with.
type styleRenderer interface{ Render(...string) string }

// runeLine is one wrapped line of a block's text: the half-open rune range
// [start, end) it shows.
type runeLine struct{ start, end int }

// wrapRunes breaks text into lines of at most width runes, at the last space
// that fits, or mid-word when a word is wider than the line. The space a line
// breaks at belongs to neither line.
func wrapRunes(text []rune, width int) []runeLine {
	if len(text) == 0 {
		return []runeLine{{}}
	}
	var lines []runeLine
	start := 0
	for start < len(text) {
		if len(text)-start <= width {
			lines = append(lines, runeLine{start, len(text)})
			break
		}
		end := start + width
		brk := -1
		for i := end; i > start; i-- {
			if text[i] == ' ' {
				brk = i
				break
			}
		}
		if brk < 0 {
			lines = append(lines, runeLine{start, end})
			start = end
			continue
		}
		lines = append(lines, runeLine{start, brk})
		start = brk + 1
	}
	return lines
}

// writeCarets draws, under one wrapped line, a caret under every rune a
// finding covers, then the label of each finding that starts on the line,
// beginning at its caret. Labels share a row when they do not touch.
func writeCarets(w io.Writer, s *output.Styles, indent string, ln runeLine, findings []LookFinding, textW int) {
	carets := make([]rune, ln.end-ln.start)
	for i := range carets {
		carets[i] = ' '
	}
	fails := false
	type placed struct {
		col   int
		label string
		f     LookFinding
	}
	var labels []placed
	for _, f := range findings {
		if f.Span == nil || f.Span.End <= ln.start || f.Span.Start >= ln.end+1 {
			continue
		}
		for i := max(f.Span.Start, ln.start); i < min(f.Span.End, ln.end); i++ {
			carets[i-ln.start] = '^'
		}
		fails = fails || f.Fails
		if f.Span.Start >= ln.start && f.Span.Start <= ln.end {
			col := f.Span.Start - ln.start
			col = max(0, min(col, textW-utf8.RuneCountInString(f.Label)))
			labels = append(labels, placed{col: col, label: f.Label, f: f})
		}
	}
	if !strings.ContainsRune(string(carets), '^') {
		return
	}
	caretStyle := styleRenderer(s.Warn)
	if fails {
		caretStyle = s.Error
	}
	fmt.Fprintln(w, indent+caretStyle.Render(strings.TrimRight(string(carets), " ")))

	// Pack the labels into rows, left to right, two columns apart.
	var rows [][]placed
	for _, p := range labels {
		placedRow := false
		for i, row := range rows {
			last := row[len(row)-1]
			if p.col >= last.col+utf8.RuneCountInString(last.label)+2 {
				rows[i] = append(row, p)
				placedRow = true
				break
			}
		}
		if !placedRow {
			rows = append(rows, []placed{p})
		}
	}
	for _, row := range rows {
		var b strings.Builder
		at := 0
		for _, p := range row {
			b.WriteString(strings.Repeat(" ", p.col-at))
			b.WriteString(lookLabelStyle(s, p.f).Render(p.label))
			at = p.col + utf8.RuneCountInString(p.label)
		}
		fmt.Fprintln(w, indent+b.String())
	}
}

// padRight pads s with spaces to width runes.
func padRight(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-utf8.RuneCountInString(s)))
}

// projectRel names an absolute path inside root by its project-relative,
// slash-separated form, and any other path as it is.
func projectRel(root, path string) string {
	if spelled, inside := rootSpelled(root, path); inside {
		rel, _ := underRoot(root, spelled)
		return rel
	}
	return filepath.ToSlash(path)
}

// rootSpelled spells an absolute path inside root the way root spells it, and
// reports whether it is inside. The two can name one directory differently
// when one passed through a symbolic link (macOS reaches /var through
// /private/var), so a path that is not under root as written is compared
// again with the links resolved.
func rootSpelled(root, abs string) (string, bool) {
	if rel, inside := underRoot(root, abs); inside {
		return filepath.Join(root, filepath.FromSlash(rel)), true
	}
	realRoot, rerr := filepath.EvalSymlinks(root)
	realPath, perr := filepath.EvalSymlinks(abs)
	if rerr != nil || perr != nil {
		return abs, false
	}
	if rel, inside := underRoot(realRoot, realPath); inside {
		return filepath.Join(root, filepath.FromSlash(rel)), true
	}
	return abs, false
}

// cwdAbs makes a path the check reported relative to the working directory
// absolute.
func cwdAbs(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// lookUnreadReason says in a few words why a file could not be read.
func lookUnreadReason(a *App, file, name string, err error) string {
	if a.NoReaderFor(file) && name == "" {
		return "no reader for " + cmp.Or(filepath.Ext(file), filepath.Base(file))
	}
	return err.Error()
}

// appendUnique appends s to list unless list holds it.
func appendUnique(list []string, s string) []string {
	if s == "" || slices.Contains(list, s) {
		return list
	}
	return append(list, s)
}
