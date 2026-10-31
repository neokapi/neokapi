package backend

// The Reader: one content file as a person reads it, its blocks in file order
// with each finding on its words and the rules that hold there in a margin.
//
// The blocks and findings are a checks run over that file alone (runChecks),
// and the rules are the answer `kapi context <path>` prints (ContextAt). A fix
// is the finding's own change operation (check.Fix), applied through the
// change service as the person at the keyboard, so the format's writer keeps
// the rest of the file as it was.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/rulesview"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/screens"
)

// readerRead is one read of a file for the Reader, with the fixes its
// findings carry.
type readerRead struct {
	dto *screens.ReaderFileDTO
	// fixes maps a finding's id to its fix, kapi.change/v1 JSON; order lists
	// the fixable findings in file order.
	fixes map[string]string
	order []string
	// run is the checks run over the file the read was drawn from.
	run *CheckRunResult
}

// ReaderFile reads one content file of the project a tab holds for the
// Reader: its blocks, the findings a checks run over that file reports on
// each, and the rules the context answer for it holds. relPath is
// project-relative, or absolute inside the project. A file the project keeps
// a translation in is refused: the Reader reads the source.
func (a *App) ReaderFile(tabID, relPath string) (*screens.ReaderFileDTO, error) {
	r, err := a.readerRead(tabID, relPath)
	if err != nil {
		return nil, err
	}
	return r.dto, nil
}

// FixFinding applies one finding's fix through the change service, as the
// person using the desktop, and answers with the file read again. A refusal
// is in the answer's Refused, with the service's reason, and changes nothing.
func (a *App) FixFinding(tabID, relPath, findingID string) (*screens.ReaderFileDTO, error) {
	r, err := a.readerRead(tabID, relPath)
	if err != nil {
		return nil, err
	}
	if _, ok := r.fixes[findingID]; !ok {
		if !r.holds(findingID) {
			r.dto.Refused = []screens.ReaderRefusalDTO{{FindingID: findingID, Reason: "This finding is no longer reported."}}
			return r.dto, nil
		}
		return nil, fmt.Errorf("finding %s has no fix", findingID)
	}
	return a.applyReaderFixes(tabID, relPath, r, []string{findingID}, "Fix from the Reader")
}

// FixFile applies every fix the file's findings carry as one change set and
// answers with the file read again. A fix the service refuses is set aside
// with its reason, and the rest are applied without it.
func (a *App) FixFile(tabID, relPath string) (*screens.ReaderFileDTO, error) {
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
	return slices.ContainsFunc(r.dto.Document, func(f screens.ReaderFindingDTO) bool { return f.ID == id })
}

// applyReaderFixes applies the fixes of ids, in that order, as one change set.
// When the service refuses some, it applies the others again without them, so
// one fix that does not fit leaves the rest to land, and records each refusal
// with the service's reason. It answers with the file read again.
func (a *App) applyReaderFixes(tabID, relPath string, r *readerRead, ids []string, note string) (*screens.ReaderFileDTO, error) {
	ctx, cancel := context.WithTimeout(context.Background(), changeTimeout)
	defer cancel()
	svc, err := a.changeServiceFor(ctx, tabID, nil)
	if err != nil {
		return nil, err
	}
	var refused []screens.ReaderRefusalDTO
	// wrote is set once a change set lands.
	wrote := false
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
					refused = append(refused, screens.ReaderRefusalDTO{FindingID: id, Reason: ce.Message})
				}
				break
			}
			return nil, err
		}
		if res.Status == change.SetPartial {
			return nil, fmt.Errorf("the fixes to %s were only partly written", r.dto.Path)
		}
		if res.Status != change.SetRefused {
			wrote = res.Status == change.SetApplied
			break
		}
		if res.Error != nil {
			for _, id := range pending {
				refused = append(refused, screens.ReaderRefusalDTO{FindingID: id, Reason: res.Error.Message})
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
				refused = append(refused, screens.ReaderRefusalDTO{FindingID: pending[o.I], Reason: reason})
			case change.OpNotApplied:
				again = append(again, pending[o.I])
			}
		}
		if len(again) == len(pending) {
			// Nothing was refused on its own, so nothing would change by
			// sending the rest again.
			for _, id := range again {
				refused = append(refused, screens.ReaderRefusalDTO{FindingID: id, Reason: "The change service refused the change set."})
			}
			break
		}
		pending = again
	}

	out, err := a.readerRead(tabID, relPath)
	if err != nil {
		return nil, err
	}
	// The read ran the file's checks in every target language the
	// whole-project run checks, so its findings take the file's place in that
	// run, which the Map counts and the Checks page opens on.
	if wrote {
		if op := a.getOpenProject(tabID); op != nil {
			op.replaceCheckedFile(out.dto.Abs, out.run)
		}
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
	name := strings.TrimSpace(relPath)
	// The Checks page names a file by its absolute path; one inside the
	// project reads as the project-relative path it is.
	if filepath.IsAbs(name) {
		if inner, rerr := filepath.Rel(filepath.Dir(op.Path), name); rerr == nil {
			name = inner
		}
	}
	rel := path.Clean(filepath.ToSlash(name))
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
		dto: &screens.ReaderFileDTO{
			Path:      rel,
			Abs:       rf.Path,
			Format:    rf.Format,
			Blocks:    make([]screens.ReaderBlockDTO, 0, len(blocks)),
			Document:  []screens.ReaderFindingDTO{},
			Findings:  len(findings),
			Verdict:   res.Verdict,
			Rules:     []screens.ReaderRuleDTO{},
			Attention: []string{},
			Refused:   []screens.ReaderRefusalDTO{},
		},
		fixes: map[string]string{},
		run:   res,
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
		if filepath.ToSlash(rf.Relative) != rel {
			continue
		}
		// A file declared for its comments holds no block the Reader draws:
		// its findings sit on comments, which the preview shows by line.
		if rf.CommentsOnly() {
			return project.ResolvedFile{}, fmt.Errorf("%s is read for its comments alone, which the Reader does not draw", rel)
		}
		return rf, nil
	}
	if ans, aerr := a.ContextAt(tabID, rel, 1); aerr == nil && ans.EditionOf != nil {
		return project.ResolvedFile{}, fmt.Errorf("%s is the %s translation of %s: the Reader reads and fixes the source",
			rel, ans.EditionOf.Edition, ans.EditionOf.Source)
	}
	return project.ResolvedFile{}, fmt.Errorf("%s is not one of the project's content files", rel)
}

// place sets each finding on the block it names, in the order the run
// reported them, and keeps each one's fix (screens.PlaceFindings).
func (r *readerRead) place(blocks []*model.Block, findings []DesktopFinding) {
	checked := make([]screens.CheckedFinding, len(findings))
	for i, df := range findings {
		checked[i] = checkedFinding(df)
	}
	p := screens.PlaceFindings(blocks, checked)
	r.dto.Blocks, r.dto.Document, r.dto.Fixable = p.Blocks, p.Document, p.Fixable
	r.fixes, r.order = p.Fixes, p.Order
}

// checkedFinding is a desktop finding as the Reader places it.
func checkedFinding(df DesktopFinding) screens.CheckedFinding {
	return screens.CheckedFinding{
		BlockID: df.BlockID, Field: df.Field, Locale: df.Locale, Check: df.Check,
		Category: df.Category, Rule: df.Rule, OriginalText: df.OriginalText, Message: df.Message,
		Replacement: df.Replacement, Term: df.Term, Fails: df.Fails, Suggested: df.Suggested,
		Position: df.Position, Fix: df.Fix,
	}
}

// readerLabel is the line under a finding (screens.ReaderLabel).
func readerLabel(df DesktopFinding) string { return screens.ReaderLabel(checkedFinding(df)) }

// spellingKey folds a form to compare spellings (screens.SpellingKey).
func spellingKey(form string) string { return screens.SpellingKey(form) }

// readerRules turns the context answer for a file into the margin's rows, in
// the order `kapi context <path>` prints it. The voice comes first, then what
// to say and what not, and the wordings a rule elsewhere leaves correct here.
// The notes established at the file follow, and last the suggestions not
// already stated as a rule.
func readerRules(ans *host.ContextAnswer) ([]screens.ReaderRuleDTO, int) {
	out := []screens.ReaderRuleDTO{}
	if ans == nil {
		return out, 0
	}
	voiceName := ""
	if ans.Voice != nil {
		voiceName = ans.Voice.Name
	}
	if row, ok := screens.ReaderToneRule(ans.VoiceBrief, voiceName); ok {
		out = append(out, row)
	}
	avoided := map[string]bool{}
	said := map[string]bool{}
	for _, rule := range ans.Rules {
		for _, n := range rule.Not {
			avoided[host.FoldWording(n)] = true
		}
		said[host.FoldWording(rule.Say)] = true
		out = append(out, screens.ReaderWordRule(rulesview.WordRule{
			Say: rule.Say, Also: rule.Also, Not: rule.Not, Note: rule.Note, Locale: rule.Locale, From: rule.From, Advisory: rule.Advisory,
		}))
	}
	for _, e := range ans.Elsewhere {
		out = append(out, screens.ReaderRuleDTO{
			Kind:   "keep",
			Rule:   strings.Join(e.Keep, ", ") + ", not " + e.Instead,
			Status: "established",
			From:   strings.Join(e.HeldIn, ", "),
		})
	}
	for _, n := range ans.EstablishedNotes {
		out = append(out, screens.ReaderRuleDTO{Kind: "note", Rule: n.Text, Status: n.Status, By: n.SuggestedBy, At: n.At})
	}
	for _, s := range ans.Suggestions {
		if s.Kind != "note" && avoided[host.FoldWording(s.Term)] &&
			(s.Replacement == "" || said[host.FoldWording(s.Replacement)]) {
			continue
		}
		row := screens.ReaderRuleDTO{Status: s.Status, By: s.SuggestedBy, At: s.At}
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
