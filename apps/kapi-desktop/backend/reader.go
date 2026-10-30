package backend

// The Reader: one content file as a person reads it. Its blocks are drawn in
// file order with each finding on the words it is about, and the rules that
// hold at the file run down the margin.
//
// Every part of the answer is one another surface gives for the same file:
// the blocks and findings are a checks run over that file alone (runChecks),
// the same run the Checks page makes, and the rules are the answer
// `kapi context <path>` prints (ContextAt), in its order. A fix is the
// finding's own change operation (check.Fix), applied through the change
// service as the person at the keyboard, so the format's writer keeps the
// rest of the file as it was.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host"
)

// ReaderFileDTO is one content file as the Reader draws it.
type ReaderFileDTO struct {
	// Path is the file, project-relative and slash-separated; Abs is the same
	// file on disk.
	Path string `json:"path"`
	Abs  string `json:"abs"`
	// Format is the format the recipe reads the file as.
	Format string `json:"format,omitempty"`
	// Blocks are the file's source blocks in file order, each with the
	// findings on it.
	Blocks []ReaderBlockDTO `json:"blocks"`
	// Document holds the findings that name no block of the file.
	Document []ReaderFindingDTO `json:"document"`
	// Findings counts every finding the check reports for the file, and
	// Fixable those with a fix.
	Findings int `json:"findings"`
	Fixable  int `json:"fixable"`
	// Verdict is the checks run's: "passed", "failed" or "did_not_run". A run
	// that did not run found nothing because it checked nothing.
	Verdict string `json:"verdict"`
	// Rules are what the context answer for the file holds, in the order
	// `kapi context <path>` prints them. RulesTotal counts every rule the
	// answer holds when the list is capped.
	Rules      []ReaderRuleDTO `json:"rules"`
	RulesTotal int             `json:"rules_total"`
	// Attention is what the context answer says to act on before relying on
	// it, such as a binding that failed to load.
	Attention []string `json:"attention"`
	// Refused names the fixes the change service refused in the last fix,
	// each with the service's reason. A refused fix changed nothing.
	Refused []ReaderRefusalDTO `json:"refused"`
}

// ReaderBlockDTO is one block of the file.
type ReaderBlockDTO struct {
	// ID is the block's id as the check names it.
	ID string `json:"id"`
	// Runs are the block's source runs, which the Reader projects for display.
	Runs     []model.Run        `json:"runs"`
	Findings []ReaderFindingDTO `json:"findings"`
}

// ReaderFindingDTO is one finding, placed on its block.
type ReaderFindingDTO struct {
	// ID names the finding for FixFinding. It is the same in every read of
	// the file while the finding stands.
	ID      string `json:"id"`
	BlockID string `json:"block_id,omitempty"`
	// Position is the run range the finding is about, over the block's
	// source runs; Span is that range in code points of the block's own text,
	// in which an inline code has no width. Span is absent for a finding the
	// check located at the block alone or inside a plural.
	Position model.Anchor `json:"position,omitzero"`
	Span     *ReaderSpan  `json:"span,omitempty"`
	// Kind is the mark the finding carries: "name" for a term rule about a
	// spelling, "word" for any other term rule, "tone" for a voice rule, and
	// otherwise the check that raised it (placeholder, dnt, …).
	Kind string `json:"kind"`
	// Label is the line under the block: the wording a term rule asks for and
	// the word it avoids, or the check's message.
	Label string `json:"label"`
	// Fails says whether the finding fails the check; Suggested marks one a
	// rule nobody has established raised, which never fails.
	Fails     bool `json:"fails"`
	Suggested bool `json:"suggested,omitempty"`
	// Fixable says the finding carries a fix.
	Fixable bool `json:"fixable"`
	// Locale is the language of the translation a target-side finding is
	// about; empty for a finding on the source.
	Locale string `json:"locale,omitempty"`
}

// ReaderSpan is a half-open code-point range [Start, End).
type ReaderSpan struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// ReaderRuleDTO is one rule that holds at the file.
type ReaderRuleDTO struct {
	// Kind is the mark: "tone" for the voice, "name" or "word" for a term
	// rule, "keep" for a wording a rule elsewhere leaves correct here, "note"
	// for a note.
	Kind string `json:"kind"`
	// Rule is the rule as one line.
	Rule string `json:"rule"`
	// Status is "established", "suggested" or "contested".
	Status string `json:"status"`
	// From is where the rule is held: terms, a pack, the workspace, or the
	// voice profile.
	From string `json:"from,omitempty"`
	// By is who recorded the rule and At when, RFC 3339, when the answer
	// carries them.
	By string `json:"by,omitempty"`
	At string `json:"at,omitempty"`
}

// ReaderRefusalDTO is one fix the change service refused.
type ReaderRefusalDTO struct {
	FindingID string `json:"finding_id"`
	Reason    string `json:"reason"`
}

// readerRead is one read of a file for the Reader, with the fixes its
// findings carry.
type readerRead struct {
	dto *ReaderFileDTO
	// fixes maps a finding's id to its fix, kapi.change/v1 JSON; order lists
	// the fixable findings in file order.
	fixes map[string]string
	order []string
}

// ReaderFile reads one content file of the project a tab holds for the
// Reader: its blocks, the findings a checks run over that file reports on
// each, and the rules the context answer for it holds. relPath is
// project-relative. A file the project keeps a translation in is refused:
// the Reader reads the source.
func (a *App) ReaderFile(tabID, relPath string) (*ReaderFileDTO, error) {
	r, err := a.readerRead(tabID, relPath)
	if err != nil {
		return nil, err
	}
	return r.dto, nil
}

// FixFinding applies one finding's fix through the change service, as the
// person using the desktop, and answers with the file read again. A refusal
// is in the answer's Refused, with the service's reason, and changes nothing.
func (a *App) FixFinding(tabID, relPath, findingID string) (*ReaderFileDTO, error) {
	r, err := a.readerRead(tabID, relPath)
	if err != nil {
		return nil, err
	}
	if _, ok := r.fixes[findingID]; !ok {
		if !r.holds(findingID) {
			r.dto.Refused = []ReaderRefusalDTO{{FindingID: findingID, Reason: "This finding is no longer reported."}}
			return r.dto, nil
		}
		return nil, fmt.Errorf("finding %s has no fix", findingID)
	}
	return a.applyReaderFixes(tabID, relPath, r, []string{findingID}, "Fix from the Reader")
}

// FixFile applies every fix the file's findings carry as one change set and
// answers with the file read again. A fix the service refuses is set aside
// with its reason, and the rest are applied without it.
func (a *App) FixFile(tabID, relPath string) (*ReaderFileDTO, error) {
	r, err := a.readerRead(tabID, relPath)
	if err != nil {
		return nil, err
	}
	if len(r.order) == 0 {
		return r.dto, nil
	}
	note := fmt.Sprintf("Fix %d in %s from the Reader", len(r.order), r.dto.Path)
	return a.applyReaderFixes(tabID, relPath, r, r.order, note)
}

// holds reports whether the read has a finding with this id.
func (r *readerRead) holds(id string) bool {
	for _, b := range r.dto.Blocks {
		for _, f := range b.Findings {
			if f.ID == id {
				return true
			}
		}
	}
	return slices.ContainsFunc(r.dto.Document, func(f ReaderFindingDTO) bool { return f.ID == id })
}

// applyReaderFixes applies the fixes of ids, in that order, as one change set.
// When the service refuses some, it applies the others again without them, so
// one fix that does not fit leaves the rest to land, and records each refusal
// with the service's reason. It answers with the file read again.
func (a *App) applyReaderFixes(tabID, relPath string, r *readerRead, ids []string, note string) (*ReaderFileDTO, error) {
	ctx, cancel := context.WithTimeout(context.Background(), changeTimeout)
	defer cancel()
	svc, err := a.changeServiceFor(ctx, tabID, nil)
	if err != nil {
		return nil, err
	}
	var refused []ReaderRefusalDTO
	pending := slices.Clone(ids)
	for len(pending) > 0 {
		ops := make([]change.Op, 0, len(pending))
		for _, id := range pending {
			var op change.Op
			if err := json.Unmarshal([]byte(r.fixes[id]), &op); err != nil {
				return nil, fmt.Errorf("read the fix for %s: %w", id, err)
			}
			// A fix is addressed to the source's own edition of this file. One
			// addressed anywhere else is not the Reader's to send.
			if op.At.Doc != r.dto.Path || !op.At.Edition.IsZero() {
				return nil, fmt.Errorf("the fix for %s is addressed to %s, not to the source of %s", id, op.At.Doc, r.dto.Path)
			}
			ops = append(ops, op)
		}
		res, err := svc.Apply(ctx, change.Set{Note: note, Ops: ops}, desktopActor)
		if err != nil {
			if ce, ok := errors.AsType[*change.Error](err); ok && ce != nil {
				for _, id := range pending {
					refused = append(refused, ReaderRefusalDTO{FindingID: id, Reason: ce.Message})
				}
				break
			}
			return nil, err
		}
		if res.Status == change.SetPartial {
			return nil, fmt.Errorf("the fixes to %s were only partly written", r.dto.Path)
		}
		if res.Status != change.SetRefused {
			break
		}
		if res.Error != nil {
			for _, id := range pending {
				refused = append(refused, ReaderRefusalDTO{FindingID: id, Reason: res.Error.Message})
			}
			break
		}
		var again []string
		for _, o := range res.Ops {
			if o.I < 0 || o.I >= len(pending) {
				continue
			}
			switch o.Status {
			case change.OpRefused:
				reason := "The change service refused the fix."
				if o.Error != nil && o.Error.Message != "" {
					reason = o.Error.Message
				}
				refused = append(refused, ReaderRefusalDTO{FindingID: pending[o.I], Reason: reason})
			case change.OpNotApplied:
				again = append(again, pending[o.I])
			}
		}
		if len(again) == len(pending) {
			// Nothing was refused on its own, so nothing would change by
			// sending the rest again.
			for _, id := range again {
				refused = append(refused, ReaderRefusalDTO{FindingID: id, Reason: "The change service refused the change set."})
			}
			break
		}
		pending = again
	}

	out, err := a.readerRead(tabID, relPath)
	if err != nil {
		return nil, err
	}
	out.dto.Refused = append(out.dto.Refused, refused...)
	return out.dto, nil
}

// readerRead reads relPath for the Reader.
func (a *App) readerRead(tabID, relPath string) (*readerRead, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.contextOnly || op.Project == nil || op.Path == "" {
		return nil, errors.New("the tab has no project recipe on disk")
	}
	rel := path.Clean(filepath.ToSlash(strings.TrimSpace(relPath)))
	if rel == "." || rel == "" || strings.HasPrefix(rel, "../") || path.IsAbs(rel) {
		return nil, fmt.Errorf("%q is not a file in the project", relPath)
	}
	rf, err := a.readerSource(tabID, op, rel)
	if err != nil {
		return nil, err
	}

	targets, err := canonicalLocales(localeStrings(op.Project.Defaults.TargetLanguages))
	if err != nil {
		return nil, err
	}
	var blocks []*model.Block
	res, err := a.runChecks(tabID, ProjectFilter{Languages: targets}, checkScope{file: rel, blocks: &blocks})
	if err != nil {
		return nil, err
	}
	var findings []DesktopFinding
	for _, f := range res.Files {
		if f.Path == rf.Path {
			findings = append(findings, f.Findings...)
		}
	}

	out := &readerRead{
		dto: &ReaderFileDTO{
			Path:      rel,
			Abs:       rf.Path,
			Format:    rf.Format,
			Blocks:    make([]ReaderBlockDTO, 0, len(blocks)),
			Document:  []ReaderFindingDTO{},
			Findings:  len(findings),
			Verdict:   res.Verdict,
			Rules:     []ReaderRuleDTO{},
			Attention: []string{},
			Refused:   []ReaderRefusalDTO{},
		},
		fixes: map[string]string{},
	}
	out.place(blocks, findings)

	answer, err := a.ContextAt(tabID, rel, host.DefaultContextTermsLimit)
	if err != nil {
		return nil, fmt.Errorf("read the rules for %s: %w", rel, err)
	}
	out.dto.Rules, out.dto.RulesTotal = readerRules(answer)
	out.dto.Attention = append(out.dto.Attention, answer.Attention...)
	return out, nil
}

// readerSource finds the content file rel names among the files the recipe
// declares. A translation the recipe keeps is refused by name: the Reader
// reads and fixes the source, and the context answer for the file names the
// source it is an edition of, as `kapi context <path>` does.
func (a *App) readerSource(tabID string, op *openProject, rel string) (project.ResolvedFile, error) {
	pctx := project.NewProjectContext(op.Project, op.Path)
	resolved, err := pctx.ResolveContent(a.formatReg)
	if err != nil {
		return project.ResolvedFile{}, fmt.Errorf("resolve content: %w", err)
	}
	for _, rf := range resolved {
		if filepath.ToSlash(rf.Relative) == rel {
			return rf, nil
		}
	}
	if ans, aerr := a.ContextAt(tabID, rel, 1); aerr == nil && ans.EditionOf != nil {
		return project.ResolvedFile{}, fmt.Errorf("%s is the %s translation of %s: the Reader reads and fixes the source",
			rel, ans.EditionOf.Edition, ans.EditionOf.Source)
	}
	return project.ResolvedFile{}, fmt.Errorf("%s is not one of the project's content files", rel)
}

// place sets each finding on the block it names, in the order the run
// reported them, and keeps each one's fix.
func (r *readerRead) place(blocks []*model.Block, findings []DesktopFinding) {
	index := make(map[string]int, len(blocks))
	for _, b := range blocks {
		index[b.ID] = len(r.dto.Blocks)
		r.dto.Blocks = append(r.dto.Blocks, ReaderBlockDTO{ID: b.ID, Runs: b.SourceRuns(), Findings: []ReaderFindingDTO{}})
	}
	seen := map[string]int{}
	for _, df := range findings {
		f := ReaderFindingDTO{
			BlockID:   df.BlockID,
			Position:  df.Position,
			Kind:      readerKind(df),
			Label:     readerLabel(df),
			Fails:     df.Fails,
			Suggested: df.Suggested,
			Fixable:   df.Fix != "",
		}
		if df.Field == "target" {
			f.Locale = df.Locale
		}
		key := findingKey(df)
		seen[key]++
		f.ID = key + "-" + strconv.Itoa(seen[key])
		if f.Fixable {
			r.fixes[f.ID] = df.Fix
			r.dto.Fixable++
		}
		i, ok := index[df.BlockID]
		if !ok || df.BlockID == "" {
			r.dto.Document = append(r.dto.Document, f)
			continue
		}
		if df.Field != "target" {
			f.Span = readerSpan(r.dto.Blocks[i].Runs, df.Position)
		}
		r.dto.Blocks[i].Findings = append(r.dto.Blocks[i].Findings, f)
	}
	for i := range r.dto.Blocks {
		slices.SortStableFunc(r.dto.Blocks[i].Findings, func(x, y ReaderFindingDTO) int {
			return spanStart(x.Span) - spanStart(y.Span)
		})
		for _, f := range r.dto.Blocks[i].Findings {
			if f.Fixable {
				r.order = append(r.order, f.ID)
			}
		}
	}
}

// findingKey identifies a finding by what it is about, not by where in the
// block it sits, so the same finding keeps its id while a fix elsewhere in
// the block moves its words.
func findingKey(df DesktopFinding) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		df.BlockID, df.Field, df.Locale, df.Check, df.Category, df.Rule, df.OriginalText, df.Message,
	}, "\x00")))
	return hex.EncodeToString(sum[:6])
}

// spanStart orders findings with a span by where it starts, and the others
// after them.
func spanStart(s *ReaderSpan) int {
	if s == nil {
		return 1 << 30
	}
	return s.Start
}

// readerSpan is the code-point range of the block's own text that a run range
// over its top-level runs covers. A range inside a plural or a select, or one
// the runs do not hold, has none.
func readerSpan(runs []model.Run, a model.Anchor) *ReaderSpan {
	if a.Kind != model.AnchorRange || len(a.Path) > 0 || !a.InBounds(runs) {
		return nil
	}
	start, end := textOffset(runs, a.Start), textOffset(runs, a.End)
	if end <= start {
		return nil
	}
	return &ReaderSpan{Start: start, End: end}
}

// textOffset is the code-point offset of p into the text the runs hold
// themselves, in which every run but a text run has no width.
func textOffset(runs []model.Run, p model.RunPos) int {
	n := 0
	for i := 0; i < p.Run && i < len(runs); i++ {
		if runs[i].Text != nil {
			n += utf8.RuneCountInString(runs[i].Text.Text)
		}
	}
	if p.Run < len(runs) && runs[p.Run].Text != nil {
		n += p.Offset
	}
	return n
}

// readerKind marks a finding by the check that raised it: a term rule about a
// spelling is a name, any other term rule a word, a voice rule tone, and every
// other check its own family.
func readerKind(df DesktopFinding) string {
	switch df.Check {
	case "terms":
		if df.Replacement != "" && df.Term != "" && spellingKey(df.Replacement) == spellingKey(df.Term) {
			return "name"
		}
		return "word"
	case "voice":
		return "tone"
	case "":
		return df.Category
	}
	return df.Check
}

// readerLabel is the line under a finding: the wording a term rule asks for
// and the word it found, or the check's message.
func readerLabel(df DesktopFinding) string {
	if df.Check == "terms" && df.Replacement != "" {
		found := df.OriginalText
		if found == "" {
			found = df.Term
		}
		if found != "" {
			return df.Replacement + ", not " + found
		}
	}
	return df.Message
}

// spellingKey folds a form to compare spellings: lower case, with spaces,
// hyphens, underscores and dots taken out.
func spellingKey(form string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(form) {
		switch r {
		case ' ', '-', '_', '.', ' ':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// readerRules turns the context answer for a file into the margin's rows, in
// the order `kapi context <path>` prints the answer: the voice, what to say
// and what not, the wordings a rule elsewhere leaves correct here, the notes
// established at the file, and the suggestions not already stated as a rule.
func readerRules(ans *host.ContextAnswer) ([]ReaderRuleDTO, int) {
	out := []ReaderRuleDTO{}
	if ans == nil {
		return out, 0
	}
	if brief := strings.TrimSpace(ans.VoiceBrief); brief != "" {
		row := ReaderRuleDTO{Kind: "tone", Rule: brief, Status: "established"}
		if ans.Voice != nil && ans.Voice.Name != "" {
			row.From = "voice " + ans.Voice.Name
		}
		out = append(out, row)
	}
	avoided := map[string]bool{}
	said := map[string]bool{}
	for _, rule := range ans.Rules {
		kind := "word"
		for _, n := range rule.Not {
			avoided[strings.ToLower(strings.TrimSpace(n))] = true
			if rule.Say != "" && spellingKey(n) == spellingKey(rule.Say) {
				kind = "name"
			}
		}
		said[strings.ToLower(strings.TrimSpace(rule.Say))] = true
		text := rule.Say
		switch {
		case rule.Say != "" && len(rule.Not) > 0:
			text += ", not " + strings.Join(rule.Not, ", ")
		case rule.Say == "":
			text = "Avoid " + strings.Join(rule.Not, ", ")
		}
		if len(rule.Also) > 0 {
			text += " (also " + strings.Join(rule.Also, ", ") + ")"
		}
		out = append(out, ReaderRuleDTO{Kind: kind, Rule: text, Status: "established", From: strings.Join(rule.From, ", ")})
	}
	for _, e := range ans.Elsewhere {
		out = append(out, ReaderRuleDTO{
			Kind:   "keep",
			Rule:   strings.Join(e.Keep, ", ") + ", not " + e.Instead,
			Status: "established",
			From:   strings.Join(e.HeldIn, ", "),
		})
	}
	for _, n := range ans.EstablishedNotes {
		out = append(out, ReaderRuleDTO{Kind: "note", Rule: n.Text, Status: n.Status, By: n.SuggestedBy, At: n.At})
	}
	for _, s := range ans.Suggestions {
		if s.Kind != "note" && avoided[strings.ToLower(strings.TrimSpace(s.Term))] &&
			(s.Replacement == "" || said[strings.ToLower(strings.TrimSpace(s.Replacement))]) {
			continue
		}
		row := ReaderRuleDTO{Status: s.Status, By: s.SuggestedBy, At: s.At}
		switch {
		case s.Kind == "note":
			row.Kind, row.Rule = "note", s.Text
		case s.Replacement != "" && s.Replacement != s.Term:
			row.Kind = "word"
			if spellingKey(s.Replacement) == spellingKey(s.Term) {
				row.Kind = "name"
			}
			row.Rule = s.Replacement + ", not " + strings.Join(append([]string{s.Term}, s.Forms...), ", ")
		default:
			row.Kind, row.Rule = "word", "Avoid "+strings.Join(append([]string{s.Term}, s.Forms...), ", ")
		}
		out = append(out, row)
	}
	total := len(out)
	if ans.RulesTotal > len(ans.Rules) {
		total += ans.RulesTotal - len(ans.Rules)
	}
	return out, total
}
